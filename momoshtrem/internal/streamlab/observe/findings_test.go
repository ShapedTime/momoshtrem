package observe

import (
	"strings"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

const mib = 1 << 20

// fixture is an observer that ran 60 s over one 96 MiB movie in 512 KiB
// pieces, at a 1 MiB/s bitrate, with nothing recorded yet.
func fixture() (*Observer, time.Time) {
	t0 := time.Now()
	w := &pieceWatch{hash: "aaaa", loaded: true, pieceLen: 512 << 10,
		files: []fileSpan{{path: "Film.2020.1080p.mkv", size: 96 * mib}}}
	o := &Observer{
		cfg: Config{
			Stack:       &stack.Stack{Streaming: streaming.Config{UrgentBufferBytes: 8 * mib, ReadaheadBytes: 64 * mib}},
			Titles:      []Title{{Name: "Film (2020)", Path: "/Movies/Film (2020)/Film.mkv", Hash: "aaaa", Size: 96 * mib, PieceLength: 512 << 10}},
			Bitrate:     mib,
			AbortLagMax: time.Second,
		},
		start: t0, stopped: t0.Add(time.Minute), hashes: []string{"aaaa"}, watches: map[string]*pieceWatch{"aaaa": w},
	}
	return o, t0
}

// seeding records the seeder sending rate bytes per second, sampled every
// 500 ms over the first 30 s.
func (o *Observer) seeding(t0 time.Time, rate float64) {
	for i := range 60 {
		at := t0.Add(time.Duration(i) * 500 * time.Millisecond)
		o.seeds = append(o.seeds, seederSample{t: at, hash: "aaaa", sent: int64(rate * float64(i) / 2)})
		o.torrents = append(o.torrents, torrentSample{t: at, hash: "aaaa", useful: int64(rate * float64(i) / 2), state: "active"})
	}
}

// reading records the reader at pos with its piece at prio, every 250 ms
// over [from, to).
func (o *Observer) reading(t0 time.Time, from, to time.Duration, pos int64, prio types.PiecePriority) {
	for at := from; at < to; at += tick {
		o.readers = append(o.readers, readerSample{t: t0.Add(at), pos: pos, piece: o.cfg.Titles[0].piece(pos),
			state: atorrent.PieceState{Priority: prio}})
	}
}

func rulesOf(fs []report.Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return strings.Join(out, " ")
}

func TestStallFindings(t *testing.T) {
	stall := func(t0 time.Time) Stall {
		return Stall{Device: "infuse", Start: t0.Add(10 * time.Second), End: t0.Add(13 * time.Second), Position: 20 * mib}
	}
	tests := []struct {
		name  string
		rate  float64
		prio  types.PiecePriority
		rules string
	}{
		{"the seeder below the bitrate", 0.4 * mib, types.PiecePriorityNow, "starved"},
		{"the reader's piece below Now", 3 * mib, types.PiecePriorityReadahead, "stall-below-now"},
		{"both", 0.4 * mib, types.PiecePriorityHigh, "stall-below-now starved"},
		{"neither", 3 * mib, types.PiecePriorityNow, "stall-unexplained"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, t0 := fixture()
			o.seeding(t0, tt.rate)
			o.reading(t0, 10*time.Second, 13*time.Second, 20*mib, tt.prio)
			fs := o.stallFindings(stall(t0))
			if got := rulesOf(fs); got != tt.rules {
				t.Errorf("rules = %q, want %q: %+v", got, tt.rules, fs)
			}
			for _, f := range fs {
				if f.Evidence == "" || f.AtMS != 10000 || len(f.Refs) == 0 {
					t.Errorf("finding without its evidence: %+v", f)
				}
			}
		})
	}
	// One sample below Now is a reader between its seek and its read.
	o, t0 := fixture()
	o.seeding(t0, 3*mib)
	o.reading(t0, 10*time.Second, 10250*time.Millisecond, 20*mib, types.PiecePriorityNone)
	o.reading(t0, 10250*time.Millisecond, 13*time.Second, 20*mib, types.PiecePriorityNow)
	if got := rulesOf(o.stallFindings(stall(t0))); got != "stall-unexplained" {
		t.Errorf("one sample below Now: rules = %q", got)
	}
	// Another reader of the file, far away and below Now, isn't the
	// stalled reader; it shares the swarm.
	o, t0 = fixture()
	o.seeding(t0, 3*mib)
	o.reading(t0, 10*time.Second, 13*time.Second, 20*mib, types.PiecePriorityNow)
	o.reading(t0, 10*time.Second, 13*time.Second, 80*mib, types.PiecePriorityNormal)
	sortReaders(o)
	fs := o.stallFindings(stall(t0))
	if rulesOf(fs) != "stall-unexplained" || !strings.Contains(fs[0].Hint, "2 readers of the file shared it") {
		t.Errorf("a far reader: %+v", fs)
	}
	// A hiccup is too short to explain.
	short := stall(t0)
	short.End = short.Start.Add(80 * time.Millisecond)
	if fs := o.stallFindings(short); len(fs) != 0 {
		t.Errorf("an 80 ms stall: %+v", fs)
	}
}

func TestCompetingWindows(t *testing.T) {
	o, t0 := fixture()
	// 80 MiB apart, past the 72 MiB window, for 1 s; then 10 MiB apart.
	o.reading(t0, 0, time.Second, 0, types.PiecePriorityNow)
	o.reading(t0, 0, time.Second, 80*mib, types.PiecePriorityNow)
	o.reading(t0, time.Second, 2*time.Second, 0, types.PiecePriorityNow)
	o.reading(t0, time.Second, 2*time.Second, 10*mib, types.PiecePriorityNow)
	sortReaders(o)
	fs := o.competingWindows()
	if len(fs) != 1 || fs[0].Rule != "competing-windows" || !strings.Contains(fs[0].Evidence, "80.0 MiB apart") {
		t.Fatalf("findings = %+v", fs)
	}
	// A single sample apart is a probe passing by.
	o, t0 = fixture()
	o.reading(t0, 0, tick, 0, types.PiecePriorityNow)
	o.reading(t0, 0, tick, 80*mib, types.PiecePriorityNow)
	sortReaders(o)
	if fs := o.competingWindows(); len(fs) != 0 {
		t.Errorf("one sample: %+v", fs)
	}
}

// sortReaders orders the samples by time, as the sampler records them.
func sortReaders(o *Observer) {
	rs := o.readers
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].t.Before(rs[j-1].t); j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func TestRateOver(t *testing.T) {
	t0 := time.Now()
	var pts []point
	for i := range 10 {
		pts = append(pts, point{t0.Add(time.Duration(i) * time.Second), float64(i * 100)})
	}
	if r, ok := rateOver(pts, t0.Add(2*time.Second), t0.Add(5*time.Second)); !ok || r != 100 {
		t.Errorf("rate = %v, %v", r, ok)
	}
	if r, ok := rateOver(pts, t0.Add(2500*time.Millisecond), t0.Add(2600*time.Millisecond)); !ok || r != 100 {
		t.Errorf("a window shorter than a sample: %v, %v", r, ok)
	}
	if _, ok := rateOver(pts[:1], t0, t0.Add(time.Second)); ok {
		t.Error("one point has a rate")
	}
}

func TestAbortLagsAndCap(t *testing.T) {
	o, _ := fixture()
	fs := o.abortLags([]Lag{{Request: "s1.2", Lag: 500 * time.Millisecond, Done: true}, {Request: "s1.3", Lag: 2900 * time.Millisecond, Done: true}})
	if len(fs) != 1 || fs[0].Known != "F12" || fs[0].Refs[0] != "s1.3" {
		t.Fatalf("findings = %+v", fs)
	}
	var many []report.Finding
	for range maxPerRule + 3 {
		many = append(many, fs[0])
	}
	capped := capPerRule(many)
	if len(capped) != maxPerRule+1 || !strings.Contains(capped[maxPerRule].Hint, "3 more") {
		t.Errorf("capped = %+v", capped)
	}
}

// F1: a reader prioritized at 0 while only a GET from 1000 is open.
func TestSniffs(t *testing.T) {
	o, t0 := fixture()
	path := o.cfg.Titles[0].Path
	at := t0.Add(5 * time.Second)
	logs := []LogEvent{{T: at, Component: "prioritizer", Msg: "updated seek priorities",
		Attrs: []Attr{{"file", "Film.2020.1080p.mkv"}, {"offset", "0"}}}}
	mid := []stack.Request{{ID: "s1.4", Method: "GET", Path: path, Range: "bytes=1000-", Start: at.Add(-time.Millisecond)}}
	fs := o.sniffs(logs, mid)
	if len(fs) != 1 || fs[0].Known != "F1" || !strings.Contains(fs[0].Evidence, "s1.4 GET bytes=1000-") {
		t.Fatalf("findings = %+v", fs)
	}
	atZero := append(mid, stack.Request{ID: "s1.5", Method: "GET", Path: path, Range: "bytes=0-65535", Start: at.Add(-time.Millisecond)})
	if fs := o.sniffs(logs, atZero); len(fs) != 0 {
		t.Errorf("a GET at 0 was open: %+v", fs)
	}
}

func TestStorageLoss(t *testing.T) {
	c, _ := newCapture(false)
	failRead(leecher(c, "aaaa"), 7, true)
	failRead(leecher(c, "aaaa"), 9, false)
	o, _ := fixture()
	o.start = time.Now().Add(-time.Second)
	fs := o.storageLoss(c.Events(time.Time{}))
	if len(fs) != 1 || fs[0].Known != "F11" || !strings.Contains(fs[0].Evidence, "on pieces 7") {
		t.Fatalf("findings = %+v", fs)
	}
}
