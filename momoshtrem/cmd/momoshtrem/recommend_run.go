package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aisettings"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// recommendRunCommand runs one household member's picks run in the
// foreground (#157, #160's live check), or with -dry-run builds its first
// request and counts its tokens for free.
const recommendRunCommand = "recommend-run"

// recommendRunFlags are the parsed `recommend-run` flags.
type recommendRunFlags struct {
	dryRun bool
	member int64 // 0: the lowest member
}

// parseRecommendRunFlags parses the subcommand's flags before anything
// touches the database. ok is false when the process should exit with
// code (0 after -h, 2 for bad flags).
func parseRecommendRunFlags(args []string) (f recommendRunFlags, code int, ok bool) {
	fs := flag.NewFlagSet(recommendRunCommand, flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "build the run's first request and count its tokens (free); store nothing, call no model")
	fs.Func("member", "the household member whose picks run, by id (default: the lowest)", func(v string) error {
		id, err := member.ParseID(v)
		if err != nil {
			return errors.New("want a member's id, a positive integer")
		}
		f.member = id
		return nil
	})
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return f, 0, false
		}
		return f, 2, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", fs.Args())
		return f, 2, false
	}
	f.dryRun = *dryRun
	return f, 0, true
}

// storedConnection is Trakt's connection as the database has it: connected
// unless it needs a new connect. The subcommand doesn't decrypt tokens; it
// only reads Trakt's imported tables.
type storedConnection bool

func (c storedConnection) Connected() bool { return bool(c) }

// runRecommend runs `momoshtrem recommend-run` and returns the exit code:
// 0 when the run succeeded (or the dry run built its request), 1
// otherwise.
func runRecommend(cfg *config.Config, db *library.DB, f recommendRunFlags) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.TMDB.APIKey == "" {
		slog.Error("TMDB_API_KEY is required: the profile and the candidates come from TMDB")
		return 1
	}

	// -member's picks, else the lowest member's.
	who, err := recommendMember(ctx, member.NewService(library.NewMemberRepository(db)), f.member)
	if err != nil {
		slog.Error("Couldn't choose the household member", "error", err)
		return 1
	}
	memberID := who.ID
	settingsRepo := library.NewSettingsRepository(db)
	aiStore := library.NewAIStore(db)
	ledger := aicore.NewLedger(aiStore, settingsRepo)
	client := aicore.New(aicore.Config{APIKey: cfg.AI.AnthropicAPIKey, Recorder: ledger})
	core := &aiCore{client: client, ledger: ledger, settings: aisettings.New(settingsRepo, aiStore, client != nil)}
	tmdbClient := tmdb.NewClient(cfg.TMDB.APIKey, tmdb.WithLanguage(cfg.TMDB.Language), tmdb.WithRateLimit(float64(cfg.TMDB.RPS)))
	d := aiDeps{cfg: cfg, db: db, settings: settingsRepo, aiStore: aiStore,
		tmdb: tmdb.NewCached(tmdbClient, library.NewTMDBCacheRepository(db))}

	auth, err := library.NewTraktAuthRepository(db).ForMember(memberID).Get(ctx)
	if err != nil {
		slog.Error("Couldn't read the Trakt connection", "error", err)
		return 1
	}
	connected := storedConnection(auth != nil && !auth.NeedsReauth)
	w := newRecsWiring(d, core, func(int64) profile.Connection { return connected })
	settings, err := settingsRepo.AISettings(ctx)
	if err != nil {
		slog.Error("Couldn't read the AI settings", "error", err)
		return 1
	}

	if f.dryRun {
		return recommendDryRun(ctx, os.Stdout, w, client, who, settings.RecModel, settings.RecEnabled)
	}
	if client == nil {
		slog.Error("ANTHROPIC_API_KEY is required for a run (try -dry-run without one)")
		return 1
	}
	if !settings.RecEnabled {
		slog.Error(`Personal picks are off (rec_enabled): turn them on in Settings › AI & picks, or PUT /api/settings/ai {"rec_enabled": true}`)
		return 1
	}
	id, queued, err := w.runs.QueueRecRun(ctx, memberID, recommend.KindManual, settings.RecModel)
	if err != nil {
		slog.Error("Couldn't queue the run", "error", err)
		return 1
	}
	if !queued {
		slog.Error("Another picks run of this member's is active; wait for it (GET /api/recommendations/runs/{id})",
			"run", id, "member", memberID)
		return 1
	}
	run, err := w.runs.GetRecRun(ctx, id)
	if err != nil || run == nil {
		slog.Error("Couldn't read the queued run", "run", id, "error", err)
		return 1
	}
	slog.Info("Running picks", "run", id, "member", memberID, "name", who.Name, "model", run.Model)
	started := time.Now()
	if _, err := w.runner.Execute(ctx, *run); err != nil {
		slog.Error("Couldn't record the run", "run", id, "error", err)
	}
	return printRun(context.WithoutCancel(ctx), os.Stdout, w.runs, who, id, time.Since(started))
}

// recommendMember is the member whose picks run: id's, else (0) the
// lowest.
func recommendMember(ctx context.Context, members *member.Service, id int64) (member.Member, error) {
	list, err := members.List(ctx)
	if err != nil {
		return member.Member{}, err
	}
	for _, m := range list {
		if id == 0 || m.ID == id {
			return m.Member, nil
		}
	}
	return member.Member{}, fmt.Errorf("no household member has id %d (GET /api/members lists them)", id)
}

// recommendDryRun builds the request the member's run would send first
// and reports its size, and with a key its tokens (count_tokens is free).
func recommendDryRun(ctx context.Context, out io.Writer, w *recsWiring, client *aicore.Client, who member.Member, model string, enabled bool) int {
	prep, err := w.runner.Prepare(ctx, who.ID, model)
	if err != nil {
		slog.Error("Couldn't build the run's request", "error", err)
		return 1
	}
	body, err := aicore.RequestBody(prep.Request)
	if err != nil {
		slog.Error("Couldn't encode the run's request", "error", err)
		return 1
	}
	profileJSON, _ := json.Marshal(prep.Profile)

	fmt.Fprintf(out, "recommend-run -dry-run: the first request of a %s run for %s (member %d; nothing stored, no model called)\n",
		model, who.Name, who.ID)
	ready := "no: a run would be skipped (no_history)"
	if prep.Readiness.Enough() {
		ready = "yes"
	}
	fmt.Fprintf(out, "  picks on:         %v (rec_enabled; a run needs them on)\n", enabled)
	fmt.Fprintf(out, "  trakt connected:  %v\n", prep.Readiness.Connected)
	fmt.Fprintf(out, "  loved titles:     %d (a run needs %d); goes ahead: %s\n", prep.Readiness.Counts.Loved, profile.MinLoved, ready)
	fmt.Fprintf(out, "  profile:          %d bytes (budget %d); %d plays, %d ratings, %d loved\n", len(profileJSON), profile.MaxBytes,
		prep.Profile.Counts.Plays, prep.Profile.Counts.Rated, prep.Profile.Counts.Loved)
	if prep.Pool != nil {
		fmt.Fprintf(out, "  candidate pool:   %d kept of %d considered (%d excluded); %d in the first turn\n",
			prep.Pool.Stats.Kept, prep.Pool.Stats.Considered, prep.Pool.Stats.Excluded, min(len(prep.Pool.Items), runs.InlineCandidates))
	} else {
		fmt.Fprintf(out, "  candidate pool:   none (TMDB answered none of its sources)\n")
	}
	fmt.Fprintf(out, "  tools:            %d custom, strict; web_search (max_uses %d), web_fetch (max_uses %d)\n",
		len(prep.Request.Tools), prep.Request.WebSearchMaxUses, prep.Request.WebFetchMaxUses)
	fmt.Fprintf(out, "  system prompt:    %d bytes (byte-stable, cached with the tools)\n", prep.SystemSize)
	fmt.Fprintf(out, "  first user turn:  %d bytes\n", len(prep.FirstTurn))
	fmt.Fprintf(out, "  request body:     %d bytes\n", len(body))
	fmt.Fprintf(out, "  effort:           %s; caps: %d calls, %d tokens, $%.2f, %d tool calls, %d web searches, %s\n",
		prep.Request.Effort, runs.Caps.Calls, runs.Caps.Tokens, runs.Caps.CostUSD, runs.Caps.ToolCalls, runs.Caps.WebSearches, runs.Timeout)
	if client == nil {
		fmt.Fprintf(out, "  input tokens:     not counted: count_tokens needs ANTHROPIC_API_KEY (it's free)\n")
		return 0
	}
	tokens, err := client.CountTokens(ctx, prep.Request)
	if err != nil {
		slog.Error("count_tokens failed", "error", err)
		return 1
	}
	write, _ := aicore.Cost(model, aicore.Tokens{CacheWrite: tokens})
	read, _ := aicore.Cost(model, aicore.Tokens{CacheRead: tokens})
	fmt.Fprintf(out, "  input tokens:     %d (count_tokens)\n", tokens)
	fmt.Fprintf(out, "  first call input: $%.4f written to the cache; $%.4f a later call reading it back\n", write, read)
	return 0
}

// printRun prints how a run ended, its shelves and hero picks, and its
// usage; the exit code is 0 when it succeeded.
func printRun(ctx context.Context, out io.Writer, store *library.RecRunRepository, who member.Member, id int64, took time.Duration) int {
	run, err := store.GetRecRun(ctx, id)
	if err != nil || run == nil {
		slog.Error("Couldn't read the run", "run", id, "error", err)
		return 1
	}
	fmt.Fprintf(out, "\nRun %d (%s, %s) for %s (member %d): %s in %s\n", run.ID, run.Kind, run.Model, who.Name, who.ID,
		run.Status, took.Round(time.Second))
	switch run.Status {
	case recommend.StatusSkipped:
		fmt.Fprintf(out, "  skipped: %s\n", deref(run.SkipReason))
	case recommend.StatusFailed:
		fmt.Fprintf(out, "  error: %s\n", deref(run.Error))
	}
	if run.Status == recommend.StatusSucceeded {
		shelves, err := store.RecRunShelves(ctx, id)
		if err != nil {
			slog.Error("Couldn't read the shelves", "error", err)
		}
		for i, s := range shelves {
			fmt.Fprintf(out, "\n%d. %s [%s, %s]", i+1, s.Title, s.Kind, s.Slug)
			if s.Subtitle != nil {
				fmt.Fprintf(out, " — %s", *s.Subtitle)
			}
			fmt.Fprintln(out)
			for _, item := range s.Items {
				fmt.Fprintf(out, "   - %s: %s\n", describeRec(item.TitleRef, item.Meta), item.Reason)
			}
		}
		picks, err := store.RecRunHeroPicks(ctx, id)
		if err != nil {
			slog.Error("Couldn't read the hero picks", "error", err)
		}
		fmt.Fprintf(out, "\nTonight's pick (%d):\n", len(picks))
		for _, p := range picks {
			fmt.Fprintf(out, "   - %s — %s: %s\n", p.Kicker, describeRec(p.TitleRef, p.Meta), p.Reason)
		}
	}
	fmt.Fprintf(out, "\nUsage: %d API calls · %d input tokens (cache writes included) · %d output · %d cache reads · %d web searches · $%.4f\n",
		run.APICalls, run.InputTokens, run.OutputTokens, run.CacheReadTokens, run.WebSearches, run.CostUSD)
	if run.Status != recommend.StatusSucceeded {
		return 1
	}
	return 0
}

// describeRec is a picked title: "Severance (2022, tv)".
func describeRec(ref library.TitleRef, raw json.RawMessage) string {
	var meta recommend.ItemMeta
	_ = json.Unmarshal(raw, &meta)
	name := meta.Title
	if name == "" {
		name = fmt.Sprintf("#%d", ref.TMDBID)
	}
	if meta.Year != nil {
		return fmt.Sprintf("%s (%d, %s)", name, *meta.Year, ref.MediaType)
	}
	return fmt.Sprintf("%s (%s)", name, ref.MediaType)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
