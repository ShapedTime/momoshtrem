package player_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/player"
)

// The driver against a plain HTTP server of lab media, no swarm. Nothing
// is timed tightly: the machine may be loaded.

const mib = 1 << 20

// file serves the layout's only file with http.ServeContent. hook sees
// each request's reads: it may sleep, or fail one.
type file struct {
	layout *media.Layout
	hook   func(req int, off int64) error
	reqs   atomic.Int32
}

func (f *file) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := int(f.reqs.Add(1))
	rs := &hooked{r: f.layout.FileReader(0), hook: func(off int64) error {
		if f.hook == nil {
			return nil
		}
		return f.hook(n, off)
	}}
	http.ServeContent(w, r, "f.bin", time.Time{}, rs)
}

type hooked struct {
	r    *io.SectionReader
	pos  int64
	hook func(off int64) error
}

func (h *hooked) Read(p []byte) (int, error) {
	if err := h.hook(h.pos); err != nil {
		return 0, err
	}
	n, err := h.r.ReadAt(p, h.pos)
	h.pos += int64(n)
	return n, err
}

func (h *hooked) Seek(off int64, whence int) (int64, error) {
	pos, err := h.r.Seek(off, whence)
	h.pos = pos
	return pos, err
}

func setup(t *testing.T, f *file) (client.Device, *checks.Integrity) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return client.Device{BaseURL: srv.URL, UserAgent: "player-test"}, checks.NewIntegrity(f.layout)
}

func session(cfg player.Config, l *media.Layout, integrity *checks.Integrity, onError func(int64, time.Duration, error)) *player.Session {
	return player.NewSession(cfg, player.Options{
		Path: "/f.bin", Size: l.Size(),
		Check:   func(off int64, p []byte) { integrity.Check(0, off, p) },
		OnError: onError, Retries: 1,
	})
}

// A server that pauses mid-stream makes the player stall; every byte is
// checked and the playback ends at the end of the file.
func TestWatchStalls(t *testing.T) {
	l := media.MustNew(media.SingleFile("f.bin", 7, 64<<10, 8*mib, media.Raw))
	var paused atomic.Bool
	f := &file{layout: l, hook: func(_ int, off int64) error {
		if off >= 4*mib && paused.CompareAndSwap(false, true) {
			time.Sleep(1500 * time.Millisecond)
		}
		return nil
	}}
	dev, integrity := setup(t, f)
	conn := dev.Conn()
	defer conn.Close()
	// 4 MiB/s: the 4 MiB sent before the pause play in 1 s, well inside it.
	s := session(player.Config{Bitrate: 4 * mib, Playable: 512 << 10, MaxBuffer: mib}, l, integrity, nil)
	p, err := s.Open(context.Background(), conn, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.UntilEOF(); err != nil {
		t.Fatal(err)
	}
	st := p.Stop()
	if !st.EOF || st.Received != 8*mib || st.State != "ended" {
		t.Errorf("stats = %+v, want the whole file played", st)
	}
	if len(st.Stalls) == 0 || st.StallTime <= 0 {
		t.Errorf("no stall across a 1.5 s server pause: %+v", st)
	}
	if r := integrity.Result(); r.Err() != nil || r.Checked != 8*mib {
		t.Errorf("integrity: %+v %v", r, r.Err())
	}
}

// Seeks drop the playing request: each is an abort with its Range, and a
// seek's latency runs from the seek to playable.
func TestSeekAndScrub(t *testing.T) {
	l := media.MustNew(media.SingleFile("f.bin", 8, 64<<10, 8*mib, media.Raw))
	dev, integrity := setup(t, &file{layout: l})
	conn := dev.Conn()
	defer conn.Close()
	s := session(player.Config{Bitrate: mib, Playable: 256 << 10, MaxBuffer: 2 * mib}, l, integrity, nil)
	ctx := context.Background()
	p, err := s.Open(ctx, conn, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Watch(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if p, err = s.Seek(ctx, p, conn, 4*mib); err != nil {
		t.Fatal(err)
	}
	if err := p.Scrub(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if p, err = s.Seek(ctx, p, conn, 5*mib); err != nil {
		t.Fatal(err)
	}
	if err := p.Watch(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	p.Stop()
	st := s.Stats()
	if len(st.Plays) != 3 || st.Plays[0].Seek || !st.Plays[1].Seek || !st.Plays[2].Seek {
		t.Fatalf("plays = %+v", st.Plays)
	}
	var ranges []string
	for _, a := range st.Aborts {
		ranges = append(ranges, a.Range)
	}
	if len(ranges) != 3 || ranges[0] != "bytes=0-" || ranges[1] != "bytes=4194304-" || ranges[2] != "bytes=5242880-" {
		t.Errorf("aborts = %v", ranges)
	}
	if st.Plays[2].Playable && st.Plays[2].SeekLatency <= 0 {
		t.Errorf("seek latency %v", st.Plays[2].SeekLatency)
	}
	if r := integrity.Result(); r.Err() != nil || r.Checked == 0 {
		t.Errorf("integrity: %+v %v", r, r.Err())
	}
}

// A body that breaks is reported at once with its offset, and the
// playback reconnects from where it broke, the buffer kept.
func TestBrokenBodyReconnects(t *testing.T) {
	l := media.MustNew(media.SingleFile("f.bin", 9, 64<<10, 4*mib, media.Raw))
	f := &file{layout: l, hook: func(req int, off int64) error {
		if req == 1 && off >= mib {
			return errors.New("lab: the read broke")
		}
		return nil
	}}
	dev, integrity := setup(t, f)
	conn := dev.Conn()
	defer conn.Close()
	var broke []int64
	s := session(player.Config{Bitrate: 8 * mib, Playable: 256 << 10, MaxBuffer: 8 * mib}, l, integrity,
		func(off int64, silence time.Duration, err error) {
			broke = append(broke, off)
			if silence < 0 || silence > 5*time.Second {
				t.Errorf("silence before the break = %v", silence)
			}
		})
	p, err := s.Open(context.Background(), conn, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.UntilEOF(); err != nil {
		t.Fatal(err)
	}
	st := s.Stats()
	if len(broke) != 1 || broke[0] < mib || broke[0] >= mib+64<<10 {
		t.Errorf("OnError offsets = %v, want one just past %d", broke, mib)
	}
	if len(st.ReadErrors) != 1 || len(st.Plays) != 1 || st.Plays[0].Requests != 2 || !st.Plays[0].EOF {
		t.Errorf("stats = %+v", st)
	}
	if r := integrity.Result(); r.Err() != nil || r.Checked != 4*mib {
		t.Errorf("integrity: %+v %v", r, r.Err())
	}
}

// A pause isn't silence: a body that breaks after one reports only the
// time since the resume.
func TestSilenceSkipsPauses(t *testing.T) {
	l := media.MustNew(media.SingleFile("f.bin", 11, 64<<10, 2*mib, media.Raw))
	f := &file{layout: l, hook: func(req int, off int64) error {
		if req == 1 && off >= mib {
			return errors.New("lab: the read broke")
		}
		return nil
	}}
	dev, integrity := setup(t, f)
	conn := dev.Conn()
	defer conn.Close()
	var silence time.Duration
	s := session(player.Config{Bitrate: 8 * mib, Playable: 64 << 10, MaxBuffer: 512 << 10}, l, integrity,
		func(off int64, d time.Duration, err error) { silence = d })
	p, err := s.Open(context.Background(), conn, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Fill(256 << 10); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	time.Sleep(1500 * time.Millisecond)
	if err := p.UntilEOF(); err != nil {
		t.Fatal(err)
	}
	if silence <= 0 || silence >= 1200*time.Millisecond {
		t.Errorf("silence before the break = %v, want under the 1.5 s pause", silence)
	}
}

// A wrong answer is an HTTP error and ends the playback.
func TestWrongAnswer(t *testing.T) {
	l := media.MustNew(media.SingleFile("f.bin", 10, 64<<10, mib, media.Raw))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not a range")) // 200, no Content-Range
	}))
	defer srv.Close()
	conn := client.Device{BaseURL: srv.URL}.Conn()
	defer conn.Close()
	s := session(player.Config{Bitrate: mib}, l, checks.NewIntegrity(l), nil)
	p, err := s.Open(context.Background(), conn, 0)
	if err == nil || p.Err() == nil {
		t.Fatal("a 200 answer to a Range GET was accepted")
	}
	if st := s.Stats(); len(st.HTTPErrors) != 1 {
		t.Errorf("HTTP errors = %v", st.HTTPErrors)
	}
}
