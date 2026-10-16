package main

import "github.com/shapedtime/momoshtrem/internal/api"

// recsWiring is the picks: R1's taste profile, candidates and feedback
// (#153), and R2's runs, scheduler and shelves (#157).
type recsWiring struct{}

// wireRecs serves /recommendations/home, refresh, runs and feedback
// (Options.Recommendations), starts the scheduler (rec_schedule, 03:00
// server-local) and hooks the first_sync run onto Trakt's initial import.
// It's set even without a key: /home then answers enabled: false. #153 and
// #157 fill it in; until then those routes answer 503.
func wireRecs(d aiDeps, core *aiCore, opts *api.Options) *recsWiring {
	_, _, _ = d, core, opts
	return &recsWiring{}
}

// Close stops the scheduler, waiting for a run in flight to stop.
func (w *recsWiring) Close() {}
