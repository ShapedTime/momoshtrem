package main

import (
	"context"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
	"github.com/shapedtime/momoshtrem/internal/recommend/schedule"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// recsWiring is the picks: R1's taste profile, candidates, tools and
// feedback (#153), and R2's runs, scheduler and shelves (#157).
//
// R1's parts, for R2's runs and R3's Ask (#158 gets them through
// wireAsk's recs):
//   - profiles.CheckHistory: refresh's no_history check (the database
//     only, never TMDB);
//   - profiles.Build, then profiles.Save: a run's profile, its id for
//     rec_runs.profile_id; profiles.Current: the newest stored one,
//     rebuilt when stale (Ask);
//   - pools.Build(ctx, profile): the candidate pool;
//   - taste.TitleStates with candidates.Exclusion: validating a run's
//     items (watched, rated, hidden, negative feedback);
//   - tools(inputs): the custom tools, over tools.Fixed{profile, pool}
//     for a run or a tools.Cached for Ask.
//
// R2's: runner executes a run (recommend-run uses it directly), and
// scheduler queues and executes them and serves the picks.
type recsWiring struct {
	profiles  *profile.Builder
	pools     *candidates.Builder
	taste     *library.TasteRepository
	tmdb      *tmdb.Cached
	runs      *library.RecRunRepository
	runner    *runs.Runner
	scheduler *schedule.Scheduler
}

// wireRecs sets up the picks. Feedback (Options.RecFeedback) works with or
// without a key; the dev-only debug route (Options.RecDebug) only with
// ai.recs_debug. The picks (Options.Recommendations: /home, refresh, runs)
// are served without a key too: /home then answers configured: false and
// refresh 503.
//
// It starts the scheduler (rec_schedule, 03:00 server-local; a slot
// missed while the server was down runs once, a minute after startup;
// runs a crash left active are marked failed first) and installs its
// hooks with setters, since their owners are built in main before this
// runs: the first_sync trigger on the Trakt sync (SetOnImported), the
// title pages' reasons on the catalog (SetReasons), the usage page's part
// (SetRecs) and rescheduling on a settings change (SetOnChange).
func wireRecs(d aiDeps, core *aiCore, opts *api.Options) *recsWiring {
	var trakt profile.Connection
	if d.traktAuth != nil {
		trakt = d.traktAuth
	}
	w := newRecsWiring(d, core, trakt)
	opts.RecFeedback = recommend.NewFeedbackService(library.NewRecFeedbackRepository(d.db))
	if d.cfg.AI.RecsDebug {
		opts.RecDebug = &recommend.Debug{Profiles: w.profiles, Pools: w.pools}
		slog.Warn("Recommendations debug route on: GET /api/recommendations/debug/profile (RECS_DEBUG); keep it off in production")
	}

	opts.Recommendations = w.scheduler
	core.settings.SetRecs(w.scheduler)
	core.settings.SetOnChange(w.scheduler.Reschedule)
	if d.catalog != nil {
		d.catalog.SetReasons(w.scheduler)
	}
	if d.traktSync != nil {
		d.traktSync.SetOnImported(w.scheduler.FirstSync)
	}
	if err := w.scheduler.Start(context.Background()); err != nil {
		slog.Error("Failed to start the picks' scheduler; runs left active stay so until the next start", "error", err)
	}
	return w
}

// newRecsWiring builds the picks' parts without serving or starting
// anything (recommend-run). trakt says whether Trakt is connected; nil:
// never.
func newRecsWiring(d aiDeps, core *aiCore, trakt profile.Connection) *recsWiring {
	taste := library.NewTasteRepository(d.db)
	w := &recsWiring{
		profiles: profile.New(profile.Deps{
			Store:    taste,
			Profiles: library.NewTasteProfileRepository(d.db),
			Settings: d.settings, // Trakt's use_ratings
			TMDB:     d.tmdb,
			Trakt:    trakt,
		}),
		pools: candidates.New(candidates.Deps{TMDB: d.tmdb, Store: taste}),
		taste: taste,
		tmdb:  d.tmdb,
		runs:  library.NewRecRunRepository(d.db),
	}
	// A nil *aicore.Client in the interface would look set.
	var ai runs.AI
	if core.client != nil {
		ai = core.client
	}
	w.runner = runs.NewRunner(runs.Deps{
		AI:       ai,
		Budget:   core.ledger,
		Settings: d.settings,
		Profiles: w.profiles,
		Pools:    w.pools,
		Tools:    w.tools,
		TMDB:     d.tmdb,
		Titles:   taste,
		Store:    w.runs,
	})
	reader := runs.NewReader(runs.ReaderDeps{
		Store:      w.runs,
		Settings:   d.settings,
		Titles:     taste,
		Library:    d.libraryStates(),
		Configured: ai != nil,
	})
	w.scheduler = schedule.New(schedule.Deps{
		Store:      w.runs,
		Runner:     w.runner,
		Reader:     reader,
		Settings:   d.settings,
		Budget:     core.ledger,
		History:    w.profiles,
		Configured: ai != nil,
	})
	return w
}

// libraryStates joins library state onto rec cards; nil without a
// catalog (recommend-run doesn't read /home).
func (d aiDeps) libraryStates() runs.LibraryStates {
	if d.catalog == nil {
		return nil
	}
	return d.catalog
}

// tools are R1's custom tools over inputs: tools.Fixed for a run,
// tools.Cached for Ask.
func (w *recsWiring) tools(inputs tools.Inputs) []agent.Tool {
	return tools.All(tools.Deps{Inputs: inputs, TMDB: w.tmdb, Titles: w.taste})
}

// Close stops the scheduler, waiting a while for a run in flight to stop
// (it ends failed).
func (w *recsWiring) Close() {
	if w.scheduler != nil {
		w.scheduler.Close()
	}
}
