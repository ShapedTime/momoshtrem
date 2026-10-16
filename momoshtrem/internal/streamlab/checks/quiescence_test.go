package checks_test

import (
	"context"
	"io"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
)

// startMovie serves one movie, its metainfo stored, from a seeder.
func startMovie(t *testing.T) (*stack.Stack, stack.Movie) {
	t.Helper()
	layout := media.MustNew(media.SingleFile("Film.2020.mkv", 41, 256<<10, 8<<20, media.MKV))
	mi, err := media.BuildMetainfo(layout)
	if err != nil {
		t.Fatal(err)
	}
	seeder, err := swarm.StartSeeder(swarm.SeederConfig{Layout: layout, Metainfo: mi})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { seeder.Close() })
	m := stack.Movie{Title: "Film", Year: 2020, Source: stack.Source{Magnet: seeder.Magnet(), Layout: layout}}
	s, err := stack.Start(stack.Options{
		Tune:   func(c *config.Config) { c.Torrent.ReadTimeout = 5 },
		Movies: []stack.Movie{m},
		Stored: []*metainfo.MetaInfo{mi},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s, m
}

// A play the client neither reads nor drops keeps its handler, its file and
// its reader: the check must see each, the handler's goroutine by its
// label. Once the client drops it, all of it goes.
func TestQuiescence_SeesARequestLeftRunning(t *testing.T) {
	s, m := startMovie(t)
	ctx := context.Background()
	conn := client.Device{BaseURL: s.URL, UserAgent: "streamlab-test"}.Conn()
	defer conn.Close()

	st, err := conn.Play(ctx, m.Path(), 0)
	if err != nil {
		t.Fatal(err)
	}
	// One chunk in: the handler has opened the file and is reading.
	if _, err := io.ReadFull(st, make([]byte, 32<<10)); err != nil {
		st.Abort()
		skipKnownStall(t, s, err)
		t.Fatal(err)
	}
	q := checks.Quiescence(ctx, s, 300*time.Millisecond)
	if q.Quiet() || q.Err() == nil {
		t.Fatalf("a running play reads as quiet: %+v", q)
	}
	t.Logf("while playing: %v", q.Err())
	if q.Requests != 1 || q.InFlight != 1 || q.OpenFiles != 1 || q.Readers != 1 {
		t.Errorf("requests %d, requests_inflight %v, open_files %v, readers %d; want 1 each",
			q.Requests, q.InFlight, q.OpenFiles, q.Readers)
	}
	if len(q.Leaked) == 0 {
		t.Fatal("no labelled goroutine for the running handler")
	}
	var handler bool
	for _, g := range q.Leaked {
		if !strings.HasPrefix(g.Request, s.ID+".") {
			t.Errorf("leaked goroutine of request %q, not this stack's (%s)", g.Request, s.ID)
		}
		handler = handler || strings.Contains(g.Stack, "internal/webdav")
	}
	if !handler {
		t.Errorf("no leaked goroutine runs the WebDAV handler:\n%v", q.Err())
	}

	st.Abort()
	if q := checks.Quiescence(ctx, s, 10*time.Second); !q.Quiet() {
		t.Errorf("after the client dropped the play: %v", q.Err())
	}
}

// skipKnownStall skips the test when the first read of the cold torrent
// broke on the known stall: a read timed out with the seeder connected and
// piece 0 wanted (swarm.Stalled).
func skipKnownStall(t *testing.T, s *stack.Stack, err error) {
	t.Helper()
	tor, ok := s.Torrent(s.Torrents()[0])
	if checks.Value(s.Registry, "momoshtrem_streaming_read_timeouts_total") > 0 && ok && swarm.Stalled(tor, 0) {
		t.Skipf("known issue %s, a stall with one seeder: %v", swarm.StallIssue, err)
	}
}

// A goroutine a request started on a library function (go io.Copy, go
// t.Drop) runs no momoshtrem code, but didn't start in anacrolix: once
// the requests are done, it's a leak too.
func TestQuiescence_SeesALoneSpawnedGoroutine(t *testing.T) {
	s, err := stack.Start(stack.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if q := checks.Quiescence(ctx, s, time.Second); !q.Quiet() {
		t.Fatalf("a fresh stack isn't quiet: %v", q.Err())
	}

	pr, pw := io.Pipe()
	defer pw.Close()
	pprof.Do(ctx, pprof.Labels(stack.RequestLabel, s.ID+".x"), func(context.Context) {
		go io.Copy(io.Discard, pr)
	})
	q := checks.Quiescence(ctx, s, 300*time.Millisecond)
	if len(q.Leaked) == 0 {
		t.Fatalf("the lone goroutine isn't a leak: %+v", q)
	}
	if q.Requests != 0 || q.InFlight != 0 || q.OpenFiles != 0 || q.Readers != 0 {
		t.Errorf("requests %d, requests_inflight %v, open_files %v, readers %d; want 0 each",
			q.Requests, q.InFlight, q.OpenFiles, q.Readers)
	}
	t.Logf("%v", q.Err())
}
