package main

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/me"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// smartDeps are what smart downloads are built from in main.
type smartDeps struct {
	db        *library.DB
	downloads *downloads.Service
	me        *me.Service
	// member is whose Up next the picks read: the one Trakt connection's,
	// until #244 reads every member's.
	member    int64
	traktSync *trakt.Syncer
}

// wireSmart sets up smart downloads (#77 P5, #203): the next episodes of
// the shows being watched (Trakt's Up next, and what streams now), picked
// through the download manager. A pass runs every 5 minutes, after each
// Trakt sync, after each storage PUT (smart.enabled, episodes_ahead and
// the caps: the manager's StorageChanged, which wireDownloads installs on
// the governor, nudges it), and when an episode starts streaming (the
// manager's session reads). Start it after the manager.
func wireSmart(d smartDeps) *downloads.SmartPicker {
	picker := downloads.NewSmartPicker(d.downloads, downloads.SmartDeps{
		Store:  library.NewDownloadsRepository(d.db),
		UpNext: upNextShows{me: d.me, member: d.member},
	})
	d.traktSync.SetOnSynced(picker.Nudge)
	return picker
}

// upNextShows is Up next as the smart picks read it: the library shows
// the member's /me/up-next lists, with the episode each names next.
type upNextShows struct {
	me     *me.Service
	member int64
}

func (u upNextShows) UpNextShows(ctx context.Context) ([]downloads.UpNextShow, error) {
	eps, err := u.me.UpNextEpisodes(member.WithID(ctx, u.member))
	if err != nil {
		return nil, err
	}
	out := make([]downloads.UpNextShow, len(eps))
	for i, e := range eps {
		out[i] = downloads.UpNextShow{ShowID: e.ShowID, SeasonNumber: e.SeasonNumber, EpisodeNumber: e.EpisodeNumber,
			LastWatchedAt: e.LastWatchedAt}
	}
	return out, nil
}
