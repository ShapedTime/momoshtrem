package main

import (
	"context"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// downloadsDeps are what the downloads core is built from in main.
type downloadsDeps struct {
	db       *library.DB
	governor *storagecap.Governor
	cache    *piececache.Cache
	metainfo *torrent.MetainfoStore
	torrents torrent.Service
	// activity is nil without idle mode: torrents then stay active, and
	// MarkActive is a no-op.
	activity *torrent.ActivityManager
	sessions *sessions.Registry
}

// wireDownloads sets up the downloads core (#77 P4, #202): Keeps and smart
// picks pinned in the piece cache, the manager that fetches them in the
// background, and /downloads (Options.Downloads). It installs the usage
// and the commit guard on the storage governor (the PUT's 409, its usage),
// and tells the manager (and the smart picks, wireSmart) about each
// storage PUT (StorageChanged).
//
// Nothing runs yet: startDownloads restores the pins and starts the
// manager, once the stored pauses are restored. Until then the cache holds
// its trims (DeferTrim, right after InitStorage).
func wireDownloads(d downloadsDeps, opts *api.Options) *downloads.Service {
	var mark func(string)
	if d.activity != nil {
		mark = d.activity.MarkActive
	}
	svc := downloads.New(downloads.Deps{
		Store:    library.NewDownloadsRepository(d.db),
		Limits:   d.governor,
		Cache:    d.cache,
		Metainfo: d.metainfo,
		Client:   downloads.ServiceClient{Torrents: d.torrents, Mark: mark},
		Sessions: d.sessions,
		// Once the pins are back, the cache may trim again; the governor's
		// pass then steps the excess down 2 GiB at a time, instead of the
		// cache's next write evicting it in one go.
		OnRestored: func() {
			d.cache.Ready()
			d.governor.Wake()
		},
	})
	d.governor.SetDownloads(svc)
	d.governor.SetOnChange(svc.StorageChanged)
	opts.Downloads = svc
	return svc
}

// startDownloads restores the pins (nothing is loaded yet), then runs the
// manager. When the pins can't be restored, the manager's first pass that
// can does it; the cache's fallback lets trims resume after two minutes
// regardless.
func startDownloads(svc *downloads.Service) {
	if err := svc.RestorePins(context.Background()); err != nil {
		slog.Error("Failed to restore the downloads' pins; the manager tries again", "error", err)
	}
	svc.Start()
}

// wakingTree is the WebDAV tree as the assignment writers see it: every
// rebuild after an Apply, an override or an HDRezka assign also wakes the
// download manager, so a show's or season's Keep takes a new episode at
// once.
type wakingTree struct {
	tree service.TreeInvalidator
	wake func()
}

func (t wakingTree) InvalidateTree() {
	t.tree.InvalidateTree()
	t.wake()
}
