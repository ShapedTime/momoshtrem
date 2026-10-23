package main

import (
	"context"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
	"github.com/shapedtime/momoshtrem/internal/recommend/schedule"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// recsWiring is the picks: R1's taste profile, candidates, tools and
// feedback (#153), and R2's runs, scheduler and shelves (#157), each
// household member's (#245).
//
// R1's parts are a member's, bound to them (member(id), built cheaply on
// each call), for R2's runs and R3's Ask (#158 gets them through
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
	d aiDeps
	// trakt is a member's Trakt connection; nil: never connected.
	trakt     func(memberID int64) profile.Connection
	runs      *library.RecRunRepository
	runner    *runs.Runner
	scheduler *schedule.Scheduler
}

// recsMember is one member's R1 parts, bound to them: their Trakt rows,
// feedback, profiles and use_ratings.
type recsMember struct {
	profiles *profile.Builder
	pools    *candidates.Builder
	taste    *library.TasteRepository
	tools    func(tools.Inputs) []agent.Tool
}

// wireRecs sets up the picks. Feedback (Options.RecFeedback) works with or
// without a key; the dev-only debug route (Options.RecDebug) only with
// ai.recs_debug. The picks (Options.Recommendations: /home, refresh, runs)
// are served without a key too: /home then answers configured: false and
// refresh 503. Each is the request's member's.
//
// It starts the scheduler (rec_schedule, 03:00 server-local, a run for
// every member with Trakt connected; a slot missed while the server was
// down runs once, a minute after startup; runs a crash left active are
// marked failed first) and installs its hooks with setters, since their
// owners are built in main before this runs: the first_sync trigger on
// each member's Trakt sync (SetOnImported), the title pages' reasons on
// the catalog (SetReasons), the usage page's part (SetRecs), rescheduling
// on a settings change (SetOnChange), and stopping a removed member's run
// (OnRemoving, after their Trakt is forgotten).
func wireRecs(d aiDeps, core *aiCore, opts *api.Options) *recsWiring {
	connection := func(memberID int64) profile.Connection {
		if d.trakt == nil {
			return nil
		}
		return d.trakt.Connection(memberID)
	}
	w := newRecsWiring(d, core, connection)
	opts.RecFeedback = recommend.NewFeedbackService(library.NewRecFeedbackRepository(d.db))
	if d.cfg.AI.RecsDebug {
		opts.RecDebug = &recommend.Debug{Builders: func(memberID int64) (*profile.Builder, *candidates.Builder) {
			m := w.member(memberID)
			return m.profiles, m.pools
		}}
		slog.Warn("Recommendations debug route on: GET /api/recommendations/debug/profile (RECS_DEBUG); keep it off in production")
	}

	opts.Recommendations = w.scheduler
	core.settings.SetRecs(w.scheduler)
	core.settings.SetOnChange(w.scheduler.Reschedule)
	if d.catalog != nil {
		d.catalog.SetReasons(w.scheduler)
	}
	if d.trakt != nil {
		d.trakt.SetOnImported(w.scheduler.FirstSync)
	}
	if d.members != nil {
		d.members.OnRemoving(w.scheduler.ForgetMember)
	}
	if err := w.scheduler.Start(context.Background()); err != nil {
		slog.Error("Failed to start the picks' scheduler; runs left active stay so until the next start", "error", err)
	}
	return w
}

// newRecsWiring builds the picks' parts without serving or starting
// anything (recommend-run). connection says whether a member's Trakt is
// connected.
func newRecsWiring(d aiDeps, core *aiCore, connection func(memberID int64) profile.Connection) *recsWiring {
	w := &recsWiring{d: d, trakt: connection, runs: library.NewRecRunRepository(d.db)}
	// A nil *aicore.Client in the interface would look set.
	var ai runs.AI
	if core.client != nil {
		ai = core.client
	}
	w.runner = runs.NewRunner(runs.Deps{
		AI:       ai,
		Budget:   core.ledger,
		Settings: d.settings,
		Members: func(memberID int64) runs.Member {
			m := w.member(memberID)
			return runs.Member{Profiles: m.profiles, Pools: m.pools, Titles: m.taste, Tools: m.tools}
		},
		TMDB:  d.tmdb,
		Store: w.runs,
	})
	reader := runs.NewReader(runs.ReaderDeps{
		Store:    w.runs,
		Settings: d.settings,
		Titles: func(memberID int64) runs.Titles {
			return library.NewTasteRepository(d.db).ForMember(memberID)
		},
		Library:    d.libraryStates(),
		Configured: ai != nil,
	})
	w.scheduler = schedule.New(schedule.Deps{
		Store:    w.runs,
		Runner:   w.runner,
		Reader:   reader,
		Settings: d.settings,
		Budget:   core.ledger,
		History: func(memberID int64) schedule.History {
			return w.member(memberID).profiles
		},
		Members:    recsMembers{members: d.members, trakt: d.trakt},
		Configured: ai != nil,
	})
	return w
}

// member is the member's R1 parts, bound to them.
func (w *recsWiring) member(id int64) recsMember {
	var connection profile.Connection
	if w.trakt != nil {
		connection = w.trakt(id)
	}
	taste := library.NewTasteRepository(w.d.db).ForMember(id)
	return recsMember{
		profiles: profile.New(profile.Deps{
			Store:    taste,
			Profiles: library.NewTasteProfileRepository(w.d.db).ForMember(id),
			Settings: library.NewMemberRepository(w.d.db).TraktSettingsFor(id), // Trakt's use_ratings
			TMDB:     w.d.tmdb,
			Trakt:    connection,
		}),
		pools: candidates.New(candidates.Deps{TMDB: w.d.tmdb, Store: taste}),
		taste: taste,
		// R1's custom tools over inputs: tools.Fixed for a run,
		// tools.Cached for Ask.
		tools: func(inputs tools.Inputs) []agent.Tool {
			return tools.All(tools.Deps{Inputs: inputs, TMDB: w.d.tmdb, Titles: taste})
		},
	}
}

// recsMembers are the members the schedule queues runs for: every member,
// eligible while their Trakt is connected.
type recsMembers struct {
	members *member.Service
	trakt   *trakt.Registry // nil: nobody's is
}

func (m recsMembers) IDs(ctx context.Context) ([]int64, error) { return m.members.IDs(ctx) }

func (m recsMembers) Eligible(memberID int64) bool {
	return m.trakt != nil && m.trakt.Connected(memberID)
}

// libraryStates joins library state onto rec cards; nil without a
// catalog (recommend-run doesn't read /home).
func (d aiDeps) libraryStates() runs.LibraryStates {
	if d.catalog == nil {
		return nil
	}
	return d.catalog
}

// Close stops the scheduler, waiting a while for a run in flight to stop
// (it ends failed).
func (w *recsWiring) Close() {
	if w.scheduler != nil {
		w.scheduler.Close()
	}
}
