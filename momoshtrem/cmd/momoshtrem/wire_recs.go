package main

import "github.com/shapedtime/momoshtrem/internal/api"

// recsWiring is the picks: R1's taste profile, candidates and feedback
// (#153), and R2's runs, scheduler and shelves (#157).
type recsWiring struct{}

// wireRecs sets up the picks; #153 and #157 fill it in, and until then
// their routes answer 503.
//   - #153 sets Options.RecFeedback (/recommendations/feedback), which
//     works without a key.
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
	_, _, _ = d, core, opts
	return &recsWiring{}
}

// Close stops the scheduler, waiting for a run in flight to stop.
func (w *recsWiring) Close() {}
