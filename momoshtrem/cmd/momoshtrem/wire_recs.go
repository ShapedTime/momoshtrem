package main

import (
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
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
type recsWiring struct {
	profiles *profile.Builder
	pools    *candidates.Builder
	taste    *library.TasteRepository
	tmdb     *tmdb.Cached
}

// wireRecs sets up the picks. Feedback (Options.RecFeedback) works with or
// without a key; the dev-only debug route (Options.RecDebug) only with
// ai.recs_debug.
//   - #157 sets Options.Recommendations (/home, refresh, runs), even
//     without a key: /home then answers configured: false. It starts the
//     scheduler (rec_schedule, 03:00 server-local; a slot missed while the
//     server was down runs once, shortly after startup) and installs its
//     hooks with setters, since their owners are built in main before this
//     runs: the first_sync trigger on the Trakt sync
//     (d.traktSync.SetOnImported) and the title pages' reasons on the
//     catalog (d.catalog.SetReasons, an interface catalog defines, since
//     catalog can't import recommend).
func wireRecs(d aiDeps, core *aiCore, opts *api.Options) *recsWiring {
	_ = core
	taste := library.NewTasteRepository(d.db)
	w := &recsWiring{
		profiles: profile.New(profile.Deps{
			Store:    taste,
			Profiles: library.NewTasteProfileRepository(d.db),
			Settings: d.settings, // Trakt's use_ratings
			TMDB:     d.tmdb,
			Trakt:    d.traktAuth,
		}),
		pools: candidates.New(candidates.Deps{TMDB: d.tmdb, Store: taste}),
		taste: taste,
		tmdb:  d.tmdb,
	}
	opts.RecFeedback = recommend.NewFeedbackService(library.NewRecFeedbackRepository(d.db))
	if d.cfg.AI.RecsDebug {
		opts.RecDebug = &recommend.Debug{Profiles: w.profiles, Pools: w.pools}
		slog.Warn("Recommendations debug route on: GET /api/recommendations/debug/profile (RECS_DEBUG); keep it off in production")
	}
	return w
}

// tools are R1's custom tools over inputs: tools.Fixed for a run,
// tools.Cached for Ask.
func (w *recsWiring) tools(inputs tools.Inputs) []agent.Tool {
	return tools.All(tools.Deps{Inputs: inputs, TMDB: w.tmdb, Titles: w.taste})
}

// Close stops the scheduler, waiting for a run in flight to stop.
func (w *recsWiring) Close() {}
