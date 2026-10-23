package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/secret"
	"github.com/shapedtime/momoshtrem/internal/status"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// wireTrakt sets up Trakt (B5, read-only), one connection and sync per
// household member (#244): the device-code connect with its tokens sealed
// with SECRET_KEY, and the sync (#132: the initial import after a
// connect, a last_activities check every 30 minutes and a nightly
// reconcile), the members' first checks staggered at startup. A member
// added gets theirs, and a member being removed is disconnected first
// (member.Service's listeners). It serves /integrations/trakt and the
// /me/up-next nudge for the request's member, and /status's
// trakt.connected for the viewer.
//
// It's configured with TRAKT_CLIENT_ID and a SECRET_KEY of 64 hex
// characters (or at least 32); TRAKT_CLIENT_SECRET is optional (Trakt no
// longer issues one to new apps). Without them every member's state is
// off and connect answers 503. Artwork for titles outside the library
// comes through the TMDB cache (none without TMDB_API_KEY).
func wireTrakt(cfg *config.Config, db *library.DB, members *member.Service, cachedTMDB *tmdb.Cached,
	statusOpts *status.Options, opts *api.Options) (*trakt.Registry, error) {
	var client *trakt.Client
	var box *secret.Box
	if cfg.Trakt.ClientID != "" {
		if b, err := secret.New(cfg.Security.SecretKey); err != nil {
			slog.Warn("Trakt disabled: SECRET_KEY can't encrypt its tokens", "error", err)
		} else {
			client = trakt.NewClient(cfg.Trakt.ClientID, cfg.Trakt.ClientSecret, version)
			box = b
		}
	}
	var artwork trakt.Artwork
	if cfg.TMDB.APIKey != "" {
		artwork = cachedTMDB
	}
	auths, syncs, settings := library.NewTraktAuthRepository(db), library.NewTraktSyncStore(db), library.NewMemberRepository(db)
	registry := trakt.NewRegistry(trakt.RegistryDeps{
		Client:   client,
		Box:      box,
		Artwork:  artwork,
		Auths:    func(id int64) trakt.Store { return auths.ForMember(id) },
		Syncs:    func(id int64) trakt.SyncStore { return syncs.ForMember(id) },
		Settings: func(id int64) trakt.Settings { return settings.TraktSettingsFor(id) },
	})

	ids, err := members.IDs(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to read the household's members: %w", err)
	}
	registry.Start(context.Background(), ids)
	members.OnAdded(registry.Add)
	members.OnRemoving(registry.Forget)

	statusOpts.TraktConfigured = registry.Configured()
	statusOpts.TraktConnected = registry.ConnectedFor
	opts.Trakt = registry
	if registry.Configured() {
		opts.TraktNudge = registry // reading Up next syncs the member when their last check is 5 min old
		connected := 0
		for _, id := range ids {
			if registry.Connected(id) {
				connected++
			}
		}
		slog.Info("Trakt configured", "members", len(ids), "connected", connected)
	}
	return registry, nil
}
