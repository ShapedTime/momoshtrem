package scenario

import (
	"net/http"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/cdn"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
)

// #231's signature is the corrupt seeder's flipped bytes and nothing
// else: a mismatch anywhere else, or another value, stays unexplained.
func TestExplainUnverified(t *testing.T) {
	l := media.MustNew(media.SingleFile("Film.mkv", 3, 64<<10, 1<<20, media.MKV))
	ts := &torrentSwarm{layout: l, peers: []*Peer{{Spec: SeederSpec{Missing: []int{4}}}, {Spec: SeederSpec{Corrupt: []int{4}}}}}
	integrity := checks.NewIntegrity(l)
	explainUnverified(ts, integrity)

	read := func(off int64, n int, flip ...int64) {
		b := make([]byte, n)
		if _, err := l.ReadAt(b, off); err != nil {
			t.Fatal(err)
		}
		for _, at := range flip {
			b[at-off] ^= 0xff
		}
		integrity.Check(0, off, b)
	}
	piece := int64(4 * 64 << 10)
	read(piece, 64<<10, piece, piece+swarm.ChunkSize) // two chunk starts flipped
	if r := integrity.Result(); r.Mismatched != 2 || r.Unexplained() != 0 || r.Explained[UnverifiedReads] != 2 {
		t.Fatalf("flipped chunk starts: %+v", r)
	}
	if got := integrityKnown(integrity.Result().Unexplained(), integrity.Result().Explained); got != UnverifiedReads {
		t.Errorf("integrityKnown = %q", got)
	}
	read(piece+1, 10, piece+1) // not a chunk start
	read(0, 10, 0)             // a chunk start of another piece
	b := make([]byte, 1)       // the bad byte, but another wrong value
	integrity.Check(0, piece+2*swarm.ChunkSize, b)
	r := integrity.Result()
	if r.Mismatched < 4 || r.Unexplained() < 2 {
		t.Fatalf("other mismatches were explained: %+v", r)
	}
	if got := integrityKnown(r.Unexplained(), r.Explained); got != "" {
		t.Errorf("with unexplained bytes, integrityKnown = %q", got)
	}

	// A torrent with no corrupt seeder explains nothing.
	plain := checks.NewIntegrity(l)
	explainUnverified(&torrentSwarm{layout: l, peers: []*Peer{{}}}, plain)
	bad := make([]byte, 1)
	if _, err := l.ReadAt(bad, piece); err != nil {
		t.Fatal(err)
	}
	bad[0] ^= 0xff
	plain.Check(0, piece, bad)
	if r := plain.Result(); r.Unexplained() != 1 {
		t.Errorf("a plain torrent explained a mismatch: %+v", r)
	}
}

// An expectation takes up to its count of its title's bodies that broke
// before their first byte at its offset, while it's open, and nothing
// else.
func TestExpectation(t *testing.T) {
	a, b := &Title{Name: "a"}, &Title{Name: "b"}
	const at = 1 << 20
	e := &Env{}
	x := e.Expect(a, at, 1, "a burst")
	for _, f := range []failure{
		{title: b, offset: at, broke: true},        // another title
		{title: a, offset: at + 1, broke: true},    // past the first byte
		{title: a, offset: at - 4096, broke: true}, // another request
		{title: a, offset: at},                     // a request that failed, not a body that broke
	} {
		if why := e.rec.expected(f); why != "" {
			t.Errorf("%+v was expected: %q", f, why)
		}
	}
	if why := e.rec.expected(failure{title: a, offset: at, broke: true}); why != "a burst" {
		t.Errorf("the break: %q", why)
	}
	if why := e.rec.expected(failure{title: a, offset: at, broke: true}); why != "" {
		t.Errorf("a second break past the count: %q", why)
	}
	if n := e.End(x); n != 1 {
		t.Errorf("End = %d, want 1", n)
	}
	y := e.Expect(a, at, 5, "later")
	e.End(y)
	if why := e.rec.expected(failure{title: a, offset: at, broke: true}); why != "" {
		t.Errorf("a break after End: %q", why)
	}
	fs, note := unexpected([]failure{{expected: "a burst"}, {err: "plain"}})
	if len(fs) != 1 || fs[0].err != "plain" || note == "" {
		t.Errorf("unexpected = %v, %q", fs, note)
	}
}

// A remote handler's lag is #232's within the remote read timeout; a
// torrent's is F12's within the read timeout (each plus a second); a
// handler still running, or a longer lag, is neither.
func TestLagKnown(t *testing.T) {
	remote := &Title{Path: "/Movies/R (2014)/R (2014).mp4", Remote: &cdn.File{}}
	tor := &Title{Path: "/Movies/T (2020)/T (2020).mkv"}
	c := &collector{e: &Env{Titles: []*Title{remote, tor}, Stack: &stack.Stack{HDRezkaReadTimeout: 3 * time.Second}}}
	const timeout = 10 * time.Second
	tests := []struct {
		l    abortLag
		want string
	}{
		{abortLag{path: remote.Path, lag: 2750 * time.Millisecond, done: true}, RemoteSlots},
		{abortLag{path: remote.Path, lag: 3900 * time.Millisecond, done: true}, RemoteSlots},
		{abortLag{path: remote.Path, lag: 4100 * time.Millisecond, done: true}, ""},
		{abortLag{path: remote.Path, lag: 9900 * time.Millisecond, done: true}, ""},
		{abortLag{path: remote.Path, lag: 2 * time.Second}, ""},
		{abortLag{path: tor.Path, lag: 5 * time.Second, done: true}, "F12"},
		{abortLag{path: tor.Path, lag: 12 * time.Second, done: true}, ""},
	}
	for _, tt := range tests {
		if got := c.lagKnown(tt.l, timeout); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.l, got, tt.want)
		}
	}
}

// A stalled or dropped body counts as reopened when it ended within the
// limit and the next request is a 206 from no further than the bytes it
// sent.
func TestReopened(t *testing.T) {
	at := time.Now()
	old := cdn.Request{At: at.Add(-time.Second), From: 1000, Status: http.StatusPartialContent, Sent: 5000, End: at.Add(3 * time.Second)}
	good := cdn.Request{At: at.Add(3 * time.Second), From: 5500, Status: http.StatusPartialContent}
	tests := []struct {
		name string
		log  []cdn.Request
		from int
		ok   bool
	}{
		{"reopened where it stopped", []cdn.Request{old, good}, 1, true},
		{"reopened at the last byte sent", []cdn.Request{old, {From: 6000, Status: http.StatusPartialContent}}, 1, true},
		{"reopened past what was sent", []cdn.Request{old, {From: 6001, Status: http.StatusPartialContent}}, 1, false},
		{"the reopen was refused", []cdn.Request{old, {From: 5500, Status: http.StatusForbidden}}, 1, false},
		{"nothing reopened", []cdn.Request{old}, 1, false},
		{"the body never ended", []cdn.Request{{From: 1000, Sent: 10}, good}, 1, false},
		{"no body was open", []cdn.Request{good}, 0, false},
	}
	for _, tt := range tests {
		if msg, ok := reopened(tt.log, tt.from, at, 5*time.Second); ok != tt.ok {
			t.Errorf("%s: %v (%s), want %v", tt.name, ok, msg, tt.ok)
		}
	}
	if _, ok := reopened([]cdn.Request{old, good}, 1, at, time.Second); ok {
		t.Error("a body that ended past the limit counted")
	}
}
