package main

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/me"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// smartDeps are what smart downloads are built from in main.
type smartDeps struct {
	db        *library.DB
	downloads *downloads.Service
	me        *me.Service
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
		UpNext: upNextShows{d.me},
	})
	d.traktSync.SetOnSynced(picker.Nudge)
	return picker
}

// upNextShows is Up next as the smart picks read it: the library shows
// /me/up-next lists, with the episode each names next.
type upNextShows struct{ me *me.Service }

func (u upNextShows) UpNextShows(ctx context.Context) ([]downloads.UpNextShow, error) {
	eps, err := u.me.UpNextEpisodes(ctx)
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
