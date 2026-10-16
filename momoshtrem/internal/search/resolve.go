package search

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

const (
	// indexTTL is how long resolve and Inspect accept a release id after
	// the last search that returned it.
	indexTTL = time.Hour
	// sweepEvery is how often the index drops expired ids.
	sweepEvery = time.Minute
)

// indexed is what resolve needs of a release. link carries the Jackett API
// key: it stays here and goes only to torznab.Download.
type indexed struct {
	link    string
	magnet  string // "" until resolved, when the indexer gave none
	indexer string // its name
	expires time.Time
}

func indexedOf(r torznab.Result, it *item) indexed {
	return indexed{link: r.Link, magnet: it.magnet, indexer: it.indexer}
}

// releaseIndex maps release ids to their links for an hour.
type releaseIndex struct {
	mu    sync.Mutex
	byID  map[string]indexed
	swept time.Time
}

func newReleaseIndex() *releaseIndex {
	return &releaseIndex{byID: map[string]indexed{}}
}

// put indexes a release for indexTTL from now, keeping a magnet resolved
// earlier.
func (x *releaseIndex) put(id string, v indexed, now time.Time) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if now.Sub(x.swept) >= sweepEvery {
		for k, old := range x.byID {
			if !now.Before(old.expires) {
				delete(x.byID, k)
			}
		}
		x.swept = now
	}
	if old, ok := x.byID[id]; ok && v.magnet == "" {
		v.magnet = old.magnet
	}
	v.expires = now.Add(indexTTL)
	x.byID[id] = v
}

func (x *releaseIndex) get(id string, now time.Time) (indexed, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	v, ok := x.byID[id]
	if !ok || !now.Before(v.expires) {
		return indexed{}, false
	}
	return v, true
}

func (x *releaseIndex) setMagnet(id, magnet string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if v, ok := x.byID[id]; ok {
		v.magnet = magnet
		x.byID[id] = v
	}
}

// Resolve turns a release id from the last hour into its magnet and the
// name of the indexer that found it (POST /search/torrents/resolve, and
// api.ReleaseResolver for Inspect). A release without a magnet has its
// .torrent fetched through Jackett (torznab.Download: only from the
// configured Jackett, at most 5 MB, within 15 s) and turned into a magnet
// with its name and trackers.
//
// Errors: ErrReleaseNotFound (unknown or expired), ErrResolveTimeout (it
// wraps torznab.ErrTimeout), the caller's cancellation, and
// ErrTorrentUnavailable for anything else.
func (s *Service) Resolve(ctx context.Context, releaseID string) (magnetURI, indexer string, err error) {
	rec, ok := s.index.get(releaseID, s.now())
	if !ok {
		return "", "", ErrReleaseNotFound
	}
	if rec.magnet != "" {
		return rec.magnet, rec.indexer, nil
	}
	if rec.link == "" {
		return "", "", fmt.Errorf("%w: the indexer gave no download link", ErrTorrentUnavailable)
	}

	magnet, data, err := s.jackett.Download(ctx, rec.link)
	switch {
	case errors.Is(err, context.Canceled):
		return "", "", context.Canceled
	case errors.Is(err, torznab.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "", "", ErrResolveTimeout
	case err != nil:
		return "", "", fmt.Errorf("%w: %s", ErrTorrentUnavailable, s.jackett.SafeError(err))
	}
	if magnet == "" {
		if magnet, err = magnetFromTorrent(data); err != nil {
			return "", "", err
		}
	}
	if torrent.NormalizeInfoHash(torrent.ExtractInfoHash(magnet)) == "" {
		return "", "", fmt.Errorf("%w: the magnet link has no BitTorrent info hash", ErrTorrentUnavailable)
	}
	s.index.setMagnet(releaseID, magnet)
	return magnet, rec.indexer, nil
}

// magnetFromTorrent reads a .torrent file into a magnet link with its info
// hash, name (dn) and trackers.
func magnetFromTorrent(data []byte) (string, error) {
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: the indexer's file isn't a valid .torrent", ErrTorrentUnavailable)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return "", fmt.Errorf("%w: the .torrent file has no valid info", ErrTorrentUnavailable)
	}
	m := metainfo.Magnet{
		InfoHash:    mi.HashInfoBytes(),
		Trackers:    mi.UpvertedAnnounceList().DistinctValues(),
		DisplayName: info.BestName(),
	}
	return m.String(), nil
}
