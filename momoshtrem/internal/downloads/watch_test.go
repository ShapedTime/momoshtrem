package downloads

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
)

// Local watches over the real session registry, which keeps a session
// listed for sessions.Timeout after its last read: a watch is a session
// that reads on through the end, not one that sits past 90% because its
// last read was there. No Postgres: the store records the watches.

type watchStore struct {
	Store
	mu      sync.Mutex
	watches []library.ItemRef
}

func (w *watchStore) FileItems(context.Context, string, string) ([]library.ItemRef, error) {
	return []library.ItemRef{{Type: library.ItemTypeMovie, ID: 7}}, nil
}

func (w *watchStore) UpsertLocalWatch(_ context.Context, item library.ItemRef, _ time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.watches = append(w.watches, item)
	return nil
}

func (w *watchStore) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.watches)
}

// watchRig is a manager over a real registry, and one playback of a 4 GiB
// film by Infuse.
type watchRig struct {
	clock *fakeClock
	store *watchStore
	svc   *Service
	h     *sessions.Handle
}

const filmSize = 4 << 30

func newWatchRig() *watchRig {
	clock := &fakeClock{now: time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC)}
	reg := sessions.NewRegistry(sessions.WithClock(clock.Now))
	r := &watchRig{clock: clock, store: &watchStore{}}
	r.svc = New(Deps{Store: r.store, Sessions: reg, Clock: clock, Location: time.UTC})
	ctx := sessions.WithRequest(context.Background(), sessions.Request{Method: http.MethodGet, UserAgent: "Infuse-Direct/8.0", RemoteAddr: "10.0.0.5"})
	r.h = reg.Open(ctx, sessions.File{Path: "/movies/Film/Film.mp4", Name: "Film.mp4", Size: filmSize,
		Source: sessions.SourceTorrent, InfoHash: "aa", TorrentPath: "Film/Film.mp4"})
	return r
}

func (r *watchRig) read(off int64, n int) {
	r.h.Begin()
	r.h.End(off, n)
}

// polls runs n session polls, one every PollInterval, with play(i) before
// each (nil: nothing is read).
func (r *watchRig) polls(n int, play func(i int)) {
	for i := range n {
		if play != nil {
			play(i)
		}
		r.svc.passMu.Lock()
		r.svc.observeSessions(context.Background(), r.clock.Now())
		r.svc.passMu.Unlock()
		r.clock.advance(PollInterval)
	}
}

// Infuse reads the header, then the index at the end (2 MiB of moov), then
// nothing: the session sits at 100% until it expires. Not a watch.
func TestWatch_IndexReadThenSilence(t *testing.T) {
	r := newWatchRig()
	r.read(0, 128<<10)
	r.read(filmSize-(2<<20), 2<<20)
	r.polls(6, nil) // 25 s, all of it listed
	if n := r.store.count(); n != 0 {
		t.Errorf("an index read followed by silence recorded %d watches", n)
	}
}

// The index at the end read in two chunks with a poll between them (a cold
// torrent fetches the moov's pieces a few seconds apart), then nothing: the
// second chunk moves the offset on, but the session has stopped reading.
// Not a watch.
func TestWatch_IndexReadInChunksThenSilence(t *testing.T) {
	r := newWatchRig()
	r.read(0, 128<<10)
	r.read(filmSize-(2<<20), 1<<20) // the moov's first half
	r.polls(7, func(i int) {
		if i == 1 {
			r.read(filmSize-(1<<20), 1<<20) // its second half, 5 s later
		}
	})
	if n := r.store.count(); n != 0 {
		t.Errorf("an index read in two chunks, then silence, recorded %d watches", n)
	}
}

// A playback that reaches 92% and stops there isn't a watch either.
func TestWatch_ReadsStopPastNinety(t *testing.T) {
	r := newWatchRig()
	r.read(0, 128<<10)
	r.read(filmSize*92/100, 4<<20)
	r.polls(6, nil)
	if n := r.store.count(); n != 0 {
		t.Errorf("reads that stopped at 92%% recorded %d watches", n)
	}
}

// A playback that reads on through the end is a watch, recorded once.
func TestWatch_ReadsOnThroughTheEnd(t *testing.T) {
	r := newWatchRig()
	r.read(0, 128<<10)
	r.read(filmSize-(2<<20), 2<<20) // the index first, as Infuse does
	at := int64(filmSize * 90 / 100)
	r.polls(8, func(int) {
		r.read(at, 8<<20) // playback, 8 MiB a poll
		at += 8 << 20
	})
	if n := r.store.count(); n != 1 {
		t.Errorf("a playback through the end recorded %d watches, want 1", n)
	}
}
