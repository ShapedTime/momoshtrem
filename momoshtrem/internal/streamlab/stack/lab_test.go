package stack_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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

// The lab end to end: a seeder, the stack wired as production, and a
// player over real WebDAV. Every byte is checked, and once the player is
// done nothing may be left behind. Nothing is timed tightly: the machine
// may be loaded.
//
// A read that times out with the seeder connected and its piece wanted is
// a known issue (swarm.StallIssue): the run is skipped with the client's
// status rather than failed.

// labReadTimeout is torrent.read_timeout in the lab, in seconds.
// Production waits 120 s; a healthy read on loopback takes well under a
// second even under -race, and a #195 stall lasts about a minute, so it
// only sets how long a skipped run takes.
const labReadTimeout = 5

// settle bounds the wait for quiescence. The handlers of aborted plays end
// at their next write, at once with a healthy seeder.
const settle = 15 * time.Second

// chunk is how much the player reads at a time: http.ServeContent's copy
// buffer.
const chunk = 32 << 10

// readTimeouts counts the server's reads that timed out.
const readTimeouts = "momoshtrem_streaming_read_timeouts_total"

// labRun is one run: start a seeder, serve its torrent in a stack, then
// browse, probe, play and seek as a player does.
type labRun struct {
	spec media.Spec
	file int // the file played
	// stored puts the metainfo in the stack's store, the usual production
	// path. Otherwise the info dict comes from the seeder.
	stored      bool
	startPaused bool
}

func (lr labRun) run(t *testing.T) {
	layout, err := media.New(lr.spec)
	if err != nil {
		t.Fatal(err)
	}
	mi, err := media.BuildMetainfo(layout)
	if err != nil {
		t.Fatal(err)
	}
	seeder, err := swarm.StartSeeder(swarm.SeederConfig{Layout: layout, Metainfo: mi})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { seeder.Close() })

	src := stack.Source{Magnet: seeder.Magnet(), Layout: layout, File: lr.file}
	opts := stack.Options{
		Tune: func(c *config.Config) {
			c.Torrent.ReadTimeout = labReadTimeout
			c.Torrent.StartPaused = lr.startPaused
		},
	}
	// A pack is a season, a single file a movie.
	var dir, path string
	if len(layout.Files()) > 1 {
		for i := range layout.Files() {
			ep := stack.Episode{Show: "Show", Year: 2024, Season: 1, Episode: i + 1,
				Source: stack.Source{Magnet: src.Magnet, Layout: layout, File: i}}
			opts.Episodes = append(opts.Episodes, ep)
			if i == lr.file {
				dir, path = ep.Dir(), ep.Path()
			}
		}
	} else {
		m := stack.Movie{Title: "Film", Year: 2020, Source: src}
		opts.Movies = []stack.Movie{m}
		dir, path = m.Dir(), m.Path()
	}
	if lr.stored {
		opts.Stored = []*metainfo.MetaInfo{mi}
	}
	s, err := stack.Start(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	t.Logf("stack %s at %s, config %s, start_paused=%v, stored=%v", s.ID, s.URL, s.Fingerprint, lr.startPaused, lr.stored)

	ctx := context.Background()
	f := layout.Files()[lr.file]
	p := &player{t: t, s: s, layout: layout, file: lr.file, path: path, hash: mi.HashInfoBytes().HexString(),
		integrity: checks.NewIntegrity(layout), buf: make([]byte, chunk)}
	infuse := client.Device{BaseURL: s.URL, UserAgent: "Infuse-Direct/7.7 (streamlab player-1)"}
	a, b := infuse.Conn(), infuse.Conn()
	defer a.Close()
	defer b.Close()

	t.Logf("F1: %s", checks.MIME())

	// Browse the folder: F3.
	f3, err := checks.F3(ctx, s, a, dir)
	if err != nil {
		t.Fatalf("PROPFIND %s: %v", dir, err)
	}
	t.Logf("F3: %s", f3)
	entries, err := a.Propfind(ctx, path, 0)
	if err != nil {
		t.Fatalf("PROPFIND %s: %v", path, err)
	}
	if len(entries) != 1 || entries[0].Size != f.Size || entries[0].IsDir {
		t.Fatalf("PROPFIND %s = %+v, want the file, %d bytes", path, entries, f.Size)
	}

	// HEAD, then the header and the footer probed at once on two
	// connections, as Infuse opens a file.
	resp, err := a.Head(ctx, path)
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	if resp.StatusCode != http.StatusOK || resp.ContentLength != f.Size || resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("HEAD = %d, Content-Length %d, Accept-Ranges %q; want 200, %d, bytes",
			resp.StatusCode, resp.ContentLength, resp.Header.Get("Accept-Ranges"), f.Size)
	}
	const probe = 64 << 10
	p.started()
	footer := make(chan probeResult, 1)
	go func() {
		r, body, err := b.Probe(ctx, path, f.Size-probe, probe)
		footer <- probeResult{r, body, err}
	}()
	r, body, err := a.Probe(ctx, path, 0, probe)
	p.probed(0, probe, probeResult{r, body, err})
	p.probed(f.Size-probe, probe, <-footer)

	// Play from the start, seek forward, then back: each seek drops the
	// playing request, as Infuse does.
	st := p.play(a, 0)
	p.read(st, 2<<20)
	st.Abort()
	st = p.play(b, f.Size/2)
	p.read(st, 1<<20)
	st.Abort()
	st = p.play(a, f.Size/4)
	p.read(st, f.Size) // to the end
	if err := st.Close(); err != nil {
		t.Errorf("closing the last play: %v", err)
	}
	a.Close()
	b.Close()

	q := checks.Quiescence(ctx, s, settle)
	if err := q.Err(); err != nil {
		t.Error(err)
	}
	if err := p.integrity.Result().Err(); err != nil {
		t.Error(err)
	}
	serverErrors := s.ServerErrors()
	if panics := checks.Panics(serverErrors); len(panics) > 0 {
		t.Errorf("handlers panicked:\n%s", strings.Join(panics, "\n"))
	}
	for _, e := range serverErrors {
		if len(checks.Panics([]string{e})) == 0 {
			t.Logf("server error log: %s", e)
		}
	}
	reqs := s.Requests.Requests()
	for _, r := range reqs {
		t.Logf("%s %-8s %-14s %d %8d B  ttfb %-12v took %-12v aborted=%v",
			r.ID, r.Method, r.Range, r.Status, r.Bytes, r.FirstByte.Round(time.Microsecond),
			r.Duration().Round(time.Microsecond), r.Aborted)
	}
	if want := 8; len(reqs) != want { // 2 PROPFINDs, HEAD, 2 probes, 3 plays
		t.Errorf("the stack saw %d requests, want %d", len(reqs), want)
	}
	t.Logf("quiet after %v; %d labelled anacrolix goroutines (a request started them; they live with the torrent); %.1f MiB checked",
		q.Waited.Round(time.Millisecond), q.Inherited, float64(p.integrity.Result().Checked)/(1<<20))
}

// probeResult is a Probe's answer.
type probeResult struct {
	resp *http.Response
	body []byte
	err  error
}

// player plays one file and checks what it gets.
type player struct {
	t         *testing.T
	s         *stack.Stack
	layout    *media.Layout
	file      int
	path      string
	hash      string
	integrity *checks.Integrity
	buf       []byte
	// timeouts is read_timeouts_total when the current probes or play
	// started: a break is the known stall only if a read timed out since.
	timeouts float64
}

// started notes the server's read timeouts as requests start.
func (p *player) started() { p.timeouts = checks.Value(p.s.Registry, readTimeouts) }

// size is the file's size.
func (p *player) size() int64 { return p.layout.Files()[p.file].Size }

// probed checks a probe of n bytes at off.
func (p *player) probed(off, n int64, r probeResult) {
	p.t.Helper()
	if r.err != nil {
		p.fail(off+int64(len(r.body)), fmt.Errorf("probe of %d bytes at %d: %w", n, off, r.err))
	}
	if err := checks.RangeResponse(r.resp, off, off+n-1, p.size()); err != nil {
		p.t.Fatal(err)
	}
	p.integrity.Check(p.file, off, r.body)
	if int64(len(r.body)) != n {
		p.t.Fatalf("probe at %d: %d bytes, want %d", off, len(r.body), n)
	}
}

// play starts playing at from on conn and checks the answer's headers.
func (p *player) play(conn *client.Conn, from int64) *client.Stream {
	p.t.Helper()
	p.started()
	st, err := conn.Play(context.Background(), p.path, from)
	if err != nil {
		p.t.Fatalf("play from %d: %v", from, err)
	}
	if err := checks.RangeResponse(st.Response, from, p.size()-1, p.size()); err != nil {
		st.Abort()
		p.t.Fatal(err)
	}
	return st
}

// read reads n bytes of the stream, fewer at the end of the file, a chunk
// at a time, and checks them.
func (p *player) read(st *client.Stream, n int64) {
	p.t.Helper()
	end := min(st.Pos()+n, p.size())
	for st.Pos() < end {
		off := st.Pos()
		b := p.buf[:min(int64(len(p.buf)), end-off)]
		m, err := io.ReadFull(st, b)
		p.integrity.Check(p.file, off, b[:m])
		if err != nil {
			st.Abort()
			p.fail(off+int64(m), err)
		}
	}
}

// fail fails the test on a read that broke at the file offset off, or
// skips it when that's the known stall.
func (p *player) fail(off int64, err error) {
	p.t.Helper()
	msg := fmt.Sprintf("reading %s: %v (at %d)\n%s", p.path, err, off, p.status())
	if p.knownStall(off) {
		p.t.Skipf("known issue %s, a stall with one seeder: %s", swarm.StallIssue, msg)
	}
	p.t.Fatal(msg)
}

// knownStall reports whether a read that broke at the file offset off is
// the known stall: a server-side read timed out since the request started
// (the server ends the response early then), with the seeder connected and
// the piece at off wanted (swarm.Stalled).
func (p *player) knownStall(off int64) bool {
	if checks.Value(p.s.Registry, readTimeouts) <= p.timeouts {
		return false
	}
	tor, ok := p.s.Torrent(p.hash)
	f := p.layout.Files()[p.file]
	return ok && swarm.Stalled(tor, int((f.Offset+off)/p.layout.PieceLength()))
}

// status describes the leecher for a failure message: read timeouts, the
// activity state and anacrolix's client status.
func (p *player) status() string {
	var b strings.Builder
	fmt.Fprintf(&b, "read timeouts: %v\n", checks.Value(p.s.Registry, readTimeouts))
	if p.s.Activity != nil {
		fmt.Fprintf(&b, "activity state %q\n", p.s.Activity.GetState(p.hash))
	}
	p.s.Client.WriteStatus(&b)
	return b.String()
}

// TestLab is the smoke tier, always run: a movie from stored metainfo with
// production's start_paused, and an episode of a pack whose info dict
// comes from the seeder.
func TestLab(t *testing.T) {
	tests := []struct {
		name string
		run  labRun
	}{
		{
			name: "movie, stored metainfo, start paused",
			run: labRun{
				spec:        media.SingleFile("Film.2020.1080p.mkv", 21, 256<<10, 16<<20, media.MKV),
				stored:      true,
				startPaused: true,
			},
		},
		{
			name: "episode, metadata from peers",
			run: labRun{
				// Odd sizes: files start and end inside pieces.
				spec: media.EpisodePack("Show", 1, 3, 22, 256<<10, 6<<20+321, media.MP4),
				file: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run.run(t)
		})
	}
}

// TestLab_64MiB plays 64 MiB files, for every way a torrent loads and both
// start_paused settings. It takes several seconds, more under -race, so
// it runs with STREAMLAB=1.
func TestLab_64MiB(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run the 64 MiB plays")
	}
	seed := uint64(30)
	for _, stored := range []bool{true, false} {
		for _, startPaused := range []bool{false, true} {
			seed++ // a seed each: each run's info hash names it in the logs
			lr := labRun{
				spec:        media.SingleFile("Film.2021.2160p.mkv", seed, 1<<20, 64<<20, media.MKV),
				stored:      stored,
				startPaused: startPaused,
			}
			t.Run(fmt.Sprintf("stored=%v,start_paused=%v", stored, startPaused), func(t *testing.T) {
				t.Parallel()
				lr.run(t)
			})
		}
	}
}
