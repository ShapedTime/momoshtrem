// Package runs is the picks' nightly run (PLAN §7 R2, #157): the agent on
// the AI core's tool loop that writes For You's shelves and hero picks,
// and the reads that serve them (/recommendations/home, the runs, the
// title pages' reason, the usage page's part).
//
// A run (Runner.Execute):
//   - checks the request, the monthly budget (skipped: budget_exceeded)
//     and the history (skipped: no_history) before it starts;
//   - builds and stores the taste profile, and the candidate pool (R1);
//   - runs the model from settings (rec_model) with R1's tools, web search
//     and fetch, and the strict submit_shelf and submit_hero_picks. The
//     system prompt and the tools are byte-stable, so they're cached; the
//     date, the profile and the pool's best go in the first user turn;
//   - validates every title as it's submitted (TMDB through the cache, the
//     exclusion rule) and tells the model what it dropped, so it can
//     replace it;
//   - gives a run that stops short one nudge turn;
//   - publishes its shelves and hero picks only when it succeeds, so /home
//     keeps the last run's until then.
//
// Its progress (steps, shelves done) and usage (summed from ai_usage) are
// written to rec_runs as it goes.
package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

// A run's request and limits. #160's live run checks them against real
// usage.
const (
	// Effort is output_config.effort: Opus 5.5's default, set explicitly.
	Effort = "medium"
	// WebSearchMaxUses and WebFetchMaxUses are the server tools' max_uses
	// per API call; the prompt asks for a few searches in all.
	WebSearchMaxUses = 8
	WebFetchMaxUses  = 4
	// Timeout bounds a run, its pre-checks and profile included.
	Timeout = 20 * time.Minute
	// KeepRuns is how many runs are kept (the newest succeeded one always
	// is).
	KeepRuns = 30
	// storeTimeout bounds a write made after the run's context ended.
	storeTimeout = 10 * time.Second
)

// Caps bound one run (its nudge turn included). A design run on Opus is
// about $0.36; the cost cap leaves room for web searches.
var Caps = aicore.Caps{Calls: 30, Tokens: 3_000_000, CostUSD: 1.50, ToolCalls: 80}

// AI runs tool loops (aicore.Client).
type AI interface {
	Run(ctx context.Context, req aicore.Request) (*aicore.Result, error)
}

// Settings reads the AI settings (library.SettingsRepository).
type Settings interface {
	AISettings(ctx context.Context) (library.AISettings, error)
}

// Profiles builds and stores taste profiles (profile.Builder).
type Profiles interface {
	CheckHistory(ctx context.Context) (profile.Readiness, error)
	Build(ctx context.Context) (*profile.Profile, error)
	Save(ctx context.Context, p *profile.Profile) (*profile.Stored, error)
}

// Pools builds candidate pools (candidates.Builder).
type Pools interface {
	Build(ctx context.Context, p *profile.Profile) (*candidates.Pool, error)
}

// Store writes a run as it goes (library.RecRunRepository).
type Store interface {
	StartRecRun(ctx context.Context, id int64, progress json.RawMessage) error
	SetRecRunProfile(ctx context.Context, id, profileID int64) error
	SetRecRunProgress(ctx context.Context, id int64, progress json.RawMessage) error
	SyncRecRunUsage(ctx context.Context, id int64) error
	PublishRecRun(ctx context.Context, id int64, progress json.RawMessage, shelves []library.RecShelfRow, picks []library.RecHeroPickRow) error
	FailRecRun(ctx context.Context, id int64, message string, progress json.RawMessage) error
	SkipRecRun(ctx context.Context, id int64, reason string) error
	PruneRecRuns(ctx context.Context, keep int) (int64, error)
}

var _ Store = (*library.RecRunRepository)(nil)

// Deps are what a Runner needs.
type Deps struct {
	// AI runs the loop; nil without ANTHROPIC_API_KEY (a run then fails).
	AI       AI
	Budget   aicore.Budget
	Settings Settings
	Profiles Profiles
	Pools    Pools
	// Tools are R1's custom tools over a run's inputs (recsWiring.tools).
	Tools  func(tools.Inputs) []agent.Tool
	TMDB   Details
	Titles Titles
	Store  Store
	Now    func() time.Time // nil: time.Now; its location is server-local
}

// Runner executes runs.
type Runner struct {
	d   Deps
	log *slog.Logger
}

// NewRunner creates a Runner.
func NewRunner(d Deps) *Runner {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Runner{d: d, log: slog.With("component", "recs-run")}
}

// Outcome is how a run ended.
type Outcome struct {
	Status     string // succeeded, failed or skipped
	SkipReason string
	Error      string // for the user, when failed
	Shelves    int
	HeroPicks  int
	Usage      aicore.Usage
}

// errTooFewShelves: the agent stopped, nudge and all, short of MinShelves.
var errTooFewShelves = errors.New("too few shelves")

// Execute runs a queued run to its end and stores how it ended: skipped
// before it starts (the budget, the history), failed, or succeeded with
// its shelves and hero picks published. Its error is only the store's,
// when even that couldn't be written.
func (r *Runner) Execute(parent context.Context, run library.RecRun) (Outcome, error) {
	ctx, cancel := context.WithTimeout(parent, Timeout)
	defer cancel()
	log := r.log.With("run", run.ID, "kind", run.Kind, "model", run.Model)

	fail := func(msg string, err error, progress json.RawMessage) (Outcome, error) {
		log.Warn("Picks run failed", "error", err, "message", msg)
		wctx, done := context.WithTimeout(context.WithoutCancel(parent), storeTimeout)
		defer done()
		if serr := r.d.Store.FailRecRun(wctx, run.ID, msg, progress); serr != nil {
			return Outcome{Status: recommend.StatusFailed, Error: msg}, serr
		}
		r.prune(wctx)
		return Outcome{Status: recommend.StatusFailed, Error: msg}, nil
	}
	skip := func(reason string) (Outcome, error) {
		log.Info("Picks run skipped", "reason", reason)
		if err := r.d.Store.SkipRecRun(ctx, run.ID, reason); err != nil {
			return Outcome{Status: recommend.StatusSkipped, SkipReason: reason}, err
		}
		r.prune(ctx)
		return Outcome{Status: recommend.StatusSkipped, SkipReason: reason}, nil
	}

	// Before it starts: the request, the budget, the history.
	if r.d.AI == nil {
		return fail("AI isn't set up: set ANTHROPIC_API_KEY.", recommend.ErrUnavailable, nil)
	}
	settings, err := r.d.Settings.AISettings(ctx)
	if err != nil {
		return fail("Couldn't read the AI settings.", err, nil)
	}
	if !settings.RecEnabled {
		return fail("Picks were switched off before the run started.", recommend.ErrDisabled, nil)
	}
	skeleton := r.request(run.Model, aicore.UserText("."), nil, nil, 0)
	if err := skeleton.Validate(); err != nil {
		return fail("The run's request isn't valid.", err, nil)
	}
	if err := r.d.Budget.Check(ctx); err != nil {
		if errors.Is(err, aicore.ErrBudgetExceeded) {
			return skip(recommend.SkipBudgetExceeded)
		}
		return fail("Couldn't check the AI budget.", err, nil)
	}
	ready, err := r.d.Profiles.CheckHistory(ctx)
	if err != nil {
		return fail("Couldn't read the watch history.", err, nil)
	}
	if !ready.Enough() {
		return skip(recommend.SkipNoHistory)
	}

	track := newTracker(ShelvesTarget, nil)
	track.reach(recommend.StepHistory)
	if err := r.d.Store.StartRecRun(ctx, run.ID, mustJSON(track.progress())); err != nil {
		return Outcome{Status: recommend.StatusFailed}, err
	}
	track.save = func(p Progress) {
		if err := r.d.Store.SetRecRunProgress(ctx, run.ID, mustJSON(p)); err != nil && ctx.Err() == nil {
			log.Warn("Failed to save the run's progress", "error", err)
		}
	}
	log.Info("Picks run started", "prompt", PromptVersion)

	// The inputs: the profile (stored, for the run to reference) and the
	// pool.
	p, err := r.d.Profiles.Build(ctx)
	if err != nil {
		return fail(stoppedOr(ctx, parent, "Couldn't build the taste profile."), err, mustJSON(track.finish()))
	}
	stored, err := r.d.Profiles.Save(ctx, p)
	if err != nil {
		return fail(stoppedOr(ctx, parent, "Couldn't store the taste profile."), err, mustJSON(track.finish()))
	}
	if err := r.d.Store.SetRecRunProfile(ctx, run.ID, stored.ID); err != nil {
		log.Warn("Failed to link the run's profile", "error", err)
	}
	track.detail(recommend.StepHistory, historyDetail(p.Counts))
	track.reach(recommend.StepTMDB)
	pool, err := r.d.Pools.Build(ctx, p)
	switch {
	case err == nil:
		track.detail(recommend.StepTMDB, fmt.Sprintf("%s · %s already seen",
			plural(pool.Stats.Kept, "candidate", "candidates"), thousands(pool.Stats.Excluded)))
	case ctx.Err() == nil && errors.Is(err, candidates.ErrNoSources):
		log.Warn("No candidate pool tonight: TMDB answered none of its sources")
		pool = nil
	default:
		return fail(stoppedOr(ctx, parent, "Couldn't build the candidate pool."), err, mustJSON(track.finish()))
	}

	// The loop, and one nudge when it stops short.
	b := newBoard(&validator{titles: r.d.Titles, details: r.d.TMDB, now: r.d.Now}, pool, track.shelves)
	first, err := firstTurn(r.d.Now(), p, pool)
	if err != nil {
		return fail("Couldn't write the run's first message.", err, mustJSON(track.finish()))
	}
	req := r.request(run.Model, aicore.UserText(first), append(r.d.Tools(tools.Fixed{P: p, C: pool}), b.tools()...), track, run.ID)
	req.Hooks.Call = func(aicore.CallInfo) {
		wctx, done := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
		defer done()
		if err := r.d.Store.SyncRecRunUsage(wctx, run.ID); err != nil {
			log.Warn("Failed to sum the run's usage", "error", err)
		}
	}
	res, err := r.d.AI.Run(ctx, req)
	usage := res.Usage
	if err == nil && (b.done() < MinShelves || len(b.picks) == 0) {
		log.Info("Picks run stopped short; nudging once", "shelves", b.done(), "hero_picks", len(b.picks))
		next := req
		next.Messages = append(append(append([]aicore.Message{}, req.Messages...), res.Messages...), aicore.UserText(nudge(b.done(), len(b.picks) > 0)))
		var reached string
		if next.Caps, reached = remaining(Caps, res); reached != "" {
			err = &aicore.CapError{Cap: reached}
		} else {
			var more *aicore.Result
			more, err = r.d.AI.Run(ctx, next)
			usage = usage.Add(more.Usage)
		}
	}
	if err == nil && b.done() < MinShelves {
		err = errTooFewShelves
	}
	out := Outcome{Shelves: b.done(), HeroPicks: len(b.picks), Usage: usage}
	if serr := r.d.Store.SyncRecRunUsage(context.WithoutCancel(ctx), run.ID); serr != nil {
		log.Warn("Failed to sum the run's usage", "error", serr)
	}

	// Publish what passed, or fail.
	switch {
	case ctx.Err() != nil:
		o, serr := fail(stoppedOr(ctx, parent, ""), ctx.Err(), mustJSON(track.finish()))
		o.Shelves, o.HeroPicks, o.Usage = out.Shelves, out.HeroPicks, usage
		return o, serr
	case b.done() < MinShelves:
		o, serr := fail(runError(err, b.done()), err, mustJSON(track.finish()))
		o.Shelves, o.HeroPicks, o.Usage = out.Shelves, out.HeroPicks, usage
		return o, serr
	case err != nil:
		// Enough shelves passed before the loop stopped: they're worth
		// publishing.
		log.Warn("Picks run stopped early; publishing what it saved", "error", err, "shelves", b.done())
	}
	shelves, picks, err := b.rows()
	if err != nil {
		return fail("Couldn't store the shelves.", err, mustJSON(track.finish()))
	}
	if err := r.d.Store.PublishRecRun(ctx, run.ID, mustJSON(track.finish()), shelves, picks); err != nil {
		if errors.Is(err, library.ErrRecRunNotActive) {
			return Outcome{Status: recommend.StatusFailed}, err
		}
		return fail("Couldn't store the shelves.", err, nil)
	}
	r.prune(ctx)
	out.Status = recommend.StatusSucceeded
	log.Info("Picks run succeeded", "shelves", out.Shelves, "hero_picks", out.HeroPicks, "api_calls", usage.Calls,
		"cost_usd", usage.CostUSD)
	return out, nil
}

// request is a run's request: the model, the byte-stable system prompt and
// tools, the first turn, the caps; track follows the steps.
func (r *Runner) request(model string, first aicore.Message, list []agent.Tool, track *tracker, runID int64) aicore.Request {
	req := aicore.Request{
		Model:            model,
		System:           systemPrompt,
		Tools:            list,
		WebSearchMaxUses: WebSearchMaxUses,
		WebFetchMaxUses:  WebFetchMaxUses,
		Messages:         []aicore.Message{first},
		Effort:           Effort,
		Caps:             Caps,
		Feature:          library.AIFeatureRecs,
		Ref:              runID,
	}
	if list == nil {
		req.Tools = append(r.d.Tools(tools.Fixed{}), newBoard(nil, nil, nil).tools()...)
	}
	if track != nil {
		req.Hooks = aicore.Hooks{
			BlockStart: func(b aicore.Block) {
				if b.Type == "server_tool_use" {
					track.web(b.Name)
				}
			},
			ToolStart: func(c aicore.ToolCall) { track.reach(stepOf(c.Name)) },
		}
	}
	return req
}

// Prepared is a run's request as the dry run builds it, with what went in.
type Prepared struct {
	Request    aicore.Request
	Readiness  profile.Readiness
	Profile    *profile.Profile
	Pool       *candidates.Pool // nil when TMDB answered none of its sources
	FirstTurn  string
	SystemSize int
}

// Prepare builds the request a run on model would send first, storing
// nothing (recommend-run -dry-run): the profile and pool are built, not
// saved, and the submit tools validate against nothing.
func (r *Runner) Prepare(ctx context.Context, model string) (*Prepared, error) {
	ready, err := r.d.Profiles.CheckHistory(ctx)
	if err != nil {
		return nil, err
	}
	p, err := r.d.Profiles.Build(ctx)
	if err != nil {
		return nil, err
	}
	pool, err := r.d.Pools.Build(ctx, p)
	if err != nil {
		if !errors.Is(err, candidates.ErrNoSources) || ctx.Err() != nil {
			return nil, err
		}
		pool = nil
	}
	first, err := firstTurn(r.d.Now(), p, pool)
	if err != nil {
		return nil, err
	}
	b := newBoard(&validator{titles: r.d.Titles, details: r.d.TMDB, now: r.d.Now}, pool, nil)
	req := r.request(model, aicore.UserText(first), append(r.d.Tools(tools.Fixed{P: p, C: pool}), b.tools()...), nil, 0)
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return &Prepared{Request: req, Readiness: ready, Profile: p, Pool: pool, FirstTurn: first, SystemSize: len(systemPrompt)}, nil
}

func (r *Runner) prune(ctx context.Context) {
	if n, err := r.d.Store.PruneRecRuns(ctx, KeepRuns); err != nil {
		r.log.Warn("Failed to prune old runs", "error", err)
	} else if n > 0 {
		r.log.Info("Pruned old picks runs", "runs", n)
	}
}

// remaining is what's left of caps after res, for the nudge turn, and the
// cap already reached, if one is ("" when none: a zero cap would mean no
// cap at all).
func remaining(caps aicore.Caps, res *aicore.Result) (aicore.Caps, string) {
	left := aicore.Caps{
		Calls:     caps.Calls - res.Usage.Calls,
		Tokens:    caps.Tokens - res.Usage.Total(),
		CostUSD:   caps.CostUSD - res.Usage.CostUSD,
		ToolCalls: caps.ToolCalls - res.ToolCalls,
	}
	switch {
	case left.Calls <= 0:
		return left, aicore.CapCalls
	case left.Tokens <= 0:
		return left, aicore.CapTokens
	case left.CostUSD <= 0:
		return left, aicore.CapCost
	case left.ToolCalls <= 0:
		return left, aicore.CapToolCalls
	}
	return left, ""
}

// historyDetail is the history step's tally: "2,184 plays · 312 ratings",
// or the loved titles when ratings didn't count.
func historyDetail(c profile.Counts) string {
	if c.Rated > 0 {
		return plural(c.Plays, "play", "plays") + " · " + plural(c.Rated, "rating", "ratings")
	}
	return plural(c.Plays, "play", "plays") + " · " + thousands(c.Loved) + " loved"
}

// stoppedOr is the message for a run whose context ended (the server
// stopped, or it ran out of time), else msg.
func stoppedOr(ctx, parent context.Context, msg string) string {
	switch {
	case parent.Err() != nil:
		return "momoshtrem stopped during the run."
	case ctx.Err() != nil:
		return fmt.Sprintf("The run took longer than %d minutes.", int(Timeout.Minutes()))
	case msg == "":
		return "The run stopped."
	}
	return msg
}

// capNames name the caps for the user.
var capNames = map[string]string{
	aicore.CapCalls: "API calls", aicore.CapTokens: "tokens", aicore.CapCost: "cost",
	aicore.CapToolCalls: "tool calls", aicore.CapContinuations: "web search rounds",
}

// runError is the failed run's message for the user.
func runError(err error, shelves int) string {
	var (
		capErr *aicore.CapError
		apiErr *aicore.APIError
	)
	switch {
	case errors.Is(err, errTooFewShelves):
		return fmt.Sprintf("Noir saved %d of the %d shelves a run needs.", shelves, MinShelves)
	case errors.Is(err, aicore.ErrRefused):
		return "Claude declined to write the picks."
	case errors.As(err, &capErr):
		return fmt.Sprintf("The run reached its limit on %s with %d of %d shelves saved.", capNames[capErr.Cap], shelves, MinShelves)
	case errors.Is(err, aicore.ErrTruncated):
		return "Claude's answer was cut off."
	case errors.Is(err, aicore.ErrEmptyAnswer):
		return "Claude answered nothing."
	case errors.As(err, &apiErr):
		if errors.Is(err, aicore.ErrRejected) {
			return apiErr.Error() + "."
		}
		return apiErr.Error() + "; the next run tries again."
	}
	return "Something went wrong during the run."
}

func mustJSON(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("runs: marshal %T: %v", v, err)) // plain structs never fail
	}
	return out
}
