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
	// members are whose Up next the picks read: every member's.
	members *member.Service
	trakt   *trakt.Registry
}

// wireSmart sets up smart downloads (#77 P5, #203): the next episodes of
// the shows being watched (every member's Trakt Up next, and what streams
// now), picked through the download manager. A pass runs every 5
// minutes, after each member's Trakt sync, after each storage PUT
// (smart.enabled, episodes_ahead and the caps: the manager's
// StorageChanged, which wireDownloads installs on the governor, nudges
// it), and when an episode starts streaming (the manager's session
// reads). Start it after the manager.
func wireSmart(d smartDeps) *downloads.SmartPicker {
	picker := downloads.NewSmartPicker(d.downloads, downloads.SmartDeps{
		Store:  library.NewDownloadsRepository(d.db),
		UpNext: upNextShows{me: d.me, members: d.members},
	})
	d.trakt.SetOnSynced(picker.Nudge)
	return picker
}

// upNextReader is each member's Up next, the member in the context.
// *me.Service implements it.
type upNextReader interface {
	UpNextEpisodes(ctx context.Context) ([]me.UpNextEpisode, error)
}

// memberIDs are the household's members. *member.Service implements it.
type memberIDs interface {
	IDs(ctx context.Context) ([]int64, error)
}

// upNextShows is Up next as the smart picks read it: the library shows
// every member's /me/up-next lists, with the episode each names next, by
// member id. A show on two members' lists is there twice; the picks keep
// the one watched last (downloads.smartAnchors).
type upNextShows struct {
	me      upNextReader
	members memberIDs
}

func (u upNextShows) UpNextShows(ctx context.Context) ([]downloads.UpNextShow, error) {
	ids, err := u.members.IDs(ctx)
	if err != nil {
		return nil, err
	}
	var out []downloads.UpNextShow
	for _, id := range ids {
		eps, err := u.me.UpNextEpisodes(member.WithID(ctx, id))
		if err != nil {
			return nil, err
		}
		for _, e := range eps {
			out = append(out, downloads.UpNextShow{ShowID: e.ShowID, SeasonNumber: e.SeasonNumber, EpisodeNumber: e.EpisodeNumber,
				LastWatchedAt: e.LastWatchedAt})
		}
	}
	return out, nil
}
