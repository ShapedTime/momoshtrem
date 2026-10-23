package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

// Ask reads a member's newest stored taste profile while it's younger than
// askProfileMaxAge (else builds one), and keeps the profile and the
// candidate pool it built for askInputsTTL between their turns.
const (
	askProfileMaxAge = 12 * time.Hour
	askInputsTTL     = time.Hour
)

// askWiring is Ask momoshtrem (R3, #158): turns over SSE, and threads.
type askWiring struct {
	svc *ask.Service
}

// wireAsk serves /recommendations/ask and /recommendations/threads
// (Options.Ask), on the AI core and R1's tools, each household member's
// (#246): their threads, limits and taste (askMember). Threads read
// without a key; asking needs one. At startup a turn a crash left
// streaming is marked failed, and Ask's part of /settings/ai/usage is
// installed. A removed member's turns are cancelled and their cache
// dropped (OnRemoving, after their picks run is stopped).
func wireAsk(d aiDeps, core *aiCore, recs *recsWiring, opts *api.Options) *askWiring {
	deps := ask.Deps{
		Budget:    core.ledger,
		Store:     library.NewAskRepository(d.db),
		Settings:  d.settings, // rec_model: new threads' model
		Library:   d.catalog,
		TMDB:      d.tmdb,
		ForMember: askMember(recs),
	}
	if core.client != nil {
		deps.Runner = core.client
	}
	svc := ask.New(deps)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if n, err := svc.Recover(ctx); err != nil {
		slog.Error("Failed to end the Ask turns left streaming", "component", "ask", "error", err)
	} else if n > 0 {
		slog.Warn("Ask turns left streaming by a restart are marked failed", "component", "ask", "turns", n)
	}

	opts.Ask = svc
	core.settings.SetAsk(svc)
	if d.members != nil {
		d.members.OnRemoving(svc.ForgetMember)
	}
	return &askWiring{svc: svc}
}

// askMember builds a member's Ask parts from their R1 parts: their taste,
// and R1's tools over a tools.Cached of their own, so one member's profile
// and pool are never another's. The service builds one per member and
// keeps it.
func askMember(recs *recsWiring) func(memberID int64) ask.Member {
	return func(memberID int64) ask.Member {
		m := recs.member(memberID)
		inputs := tools.NewCached(func(ctx context.Context) (*profile.Profile, error) {
			stored, err := m.profiles.Current(ctx, askProfileMaxAge)
			if err != nil {
				return nil, err
			}
			return stored.Profile, nil
		}, m.pools.Build, askInputsTTL)
		return ask.Member{Titles: m.taste, Tools: m.tools(inputs)}
	}
}

// Close cancels the turns in flight (each ends cancelled, its usage
// recorded, its stream closed) and refuses new ones. main registers it
// with the REST server's RegisterOnShutdown, so it runs, in its own
// goroutine, as shutdown starts: http.Server.Shutdown doesn't cancel
// request contexts, and a stream would otherwise hold it for 10 s.
func (w *askWiring) Close() { w.svc.Close() }
