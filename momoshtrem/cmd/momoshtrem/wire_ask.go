package main

import "github.com/shapedtime/momoshtrem/internal/api"

// askWiring is Ask momoshtrem (R3, #158): turns over SSE, and threads.
type askWiring struct{}

// wireAsk serves /recommendations/ask and /recommendations/threads
// (Options.Ask), on the AI core and R1's tools. At startup a turn a crash
// left streaming is marked failed. #158 fills it in; until then those
// routes answer 503.
func wireAsk(d aiDeps, core *aiCore, recs *recsWiring, opts *api.Options) *askWiring {
	_, _, _, _ = d, core, recs, opts
	return &askWiring{}
}

// Close cancels the turns in flight (each ends cancelled, its usage
// recorded, its stream closed) and refuses new ones. main registers it
// with the REST server's RegisterOnShutdown, so it runs, in its own
// goroutine, as shutdown starts: http.Server.Shutdown doesn't cancel
// request contexts, and a stream would otherwise hold it for 10 s.
func (w *askWiring) Close() {}
