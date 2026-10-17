package cdn

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
)

// Links is a fake hdrezka.LinkResolver over a CDN. Like hdrezka.Resolver,
// it caches each stream's link until Invalidate (the real one also expires
// them after hours, which a lab run never reaches), and a miss "resolves"
// a fresh link: one the CDN accepts until its next Expire. It counts Link,
// the resolves and Invalidate. Safe for concurrent use.
type Links struct {
	cdn *CDN

	mu sync.Mutex
	// cache holds each stream's links, by streamID, then quality.
	cache map[hdrezka.StreamKey]map[string]string
	stats LinkStats
	probe *slotProbe
}

var _ hdrezka.LinkResolver = (*Links)(nil)

// LinkStats is what a Links did.
type LinkStats struct {
	// Links are the Link calls, Resolves the ones that had no cached link
	// and resolved one, Invalidations the Invalidate calls.
	Links, Resolves, Invalidations int
}

// NewLinks resolves the files of cdn.
func NewLinks(cdn *CDN) *Links {
	return &Links{cdn: cdn, cache: map[hdrezka.StreamKey]map[string]string{}}
}

// streamID is a key's identity, as the resolver caches it: everything but
// the Referer (Path).
func streamID(key hdrezka.StreamKey) hdrezka.StreamKey {
	key.Path = ""
	return key
}

// Link returns the link of the stream's quality: from the stream's cached
// links, or from a fresh resolve of them all. It's hdrezka.ErrNoQuality
// when the CDN has no file for the stream in quality.
func (l *Links) Link(ctx context.Context, key hdrezka.StreamKey, quality string) (string, error) {
	id := streamID(key)
	if id == probeKey {
		return l.probed(ctx)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stats.Links++
	links, ok := l.cache[id]
	if !ok {
		l.stats.Resolves++
		links = l.cdn.mintAll(id)
		l.cache[id] = links
	}
	link, ok := links[quality]
	if !ok {
		return "", hdrezka.ErrNoQuality
	}
	return link, nil
}

// Invalidate forgets the stream's link: the next Link resolves a fresh one.
func (l *Links) Invalidate(key hdrezka.StreamKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stats.Invalidations++
	delete(l.cache, streamID(key))
}

// Stats is what the Links did so far.
func (l *Links) Stats() LinkStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stats
}

// StreamerSlots is how many bodies an hdrezka.Streamer keeps open at once
// (its unexported defaultMaxBodies). TestFreeSlots fails when it changes.
const StreamerSlots = 8

// probeKey names the slot probe's file: no stream has a negative post.
var probeKey = hdrezka.StreamKey{PostID: -1}

// errProbe ends a slot probe's open: it reaches no CDN.
var errProbe = errors.New("cdn: slot probe")

// slotProbe is a FreeSlots run: n opens wait in Link until all n have
// arrived.
type slotProbe struct {
	n, arrived int
	all        chan struct{}
}

// FreeSlots counts the streamer's free body slots, up to n, without a
// request to the CDN. It opens n probe files at once through s: each takes
// a slot, then waits in Link until all n have arrived or wait has passed,
// and fails. The opens that got a slot are the free slots. It takes no
// time when all n are free, and wait otherwise. Run it when nothing else
// opens files through s, and through a streamer that resolves with l.
func (l *Links) FreeSlots(s *hdrezka.Streamer, n int, wait time.Duration) int {
	p := &slotProbe{n: n, all: make(chan struct{})}
	l.mu.Lock()
	l.probe = p
	l.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Open(ctx, hdrezka.File{Key: probeKey, Quality: "probe"}, 0, 1)
		}()
	}
	wg.Wait()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.probe = nil
	return p.arrived
}

// probed is a slot probe's Link: it holds the slot until every probe has
// one, or the probe's wait is over.
func (l *Links) probed(ctx context.Context) (string, error) {
	l.mu.Lock()
	p := l.probe
	// A probe that got its slot once the wait was over (one an earlier
	// probe freed) doesn't count.
	if p == nil || ctx.Err() != nil {
		l.mu.Unlock()
		return "", errProbe
	}
	p.arrived++
	if p.arrived == p.n {
		close(p.all)
	}
	l.mu.Unlock()
	select {
	case <-p.all:
	case <-ctx.Done():
	}
	return "", errProbe
}
