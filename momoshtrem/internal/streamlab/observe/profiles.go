package observe

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Profile rates. Measure mode samples lightly: a tenth of the contended
// unlocks, and blocking events of 100 µs or more always (shorter ones in
// proportion); --diagnose records every contention and samples from 10 µs.
const (
	mutexFraction         = 10
	blockRateNS           = 100_000
	diagnoseMutexFraction = 1
	diagnoseBlockRateNS   = 10_000
)

// topSites is how many contended sites the report lists per profile.
const topSites = 8

// profiles turns the runtime's mutex and block profiles on for the run.
// They're process-wide, which suits the lab's process per repetition.
type profiles struct {
	prevMutex int
}

func startProfiles(diagnose bool) *profiles {
	fraction, rate := mutexFraction, blockRateNS
	if diagnose {
		fraction, rate = diagnoseMutexFraction, diagnoseBlockRateNS
	}
	p := &profiles{prevMutex: runtime.SetMutexProfileFraction(fraction)}
	runtime.SetBlockProfileRate(rate)
	return p
}

func (p *profiles) stop() {
	runtime.SetMutexProfileFraction(p.prevMutex)
	runtime.SetBlockProfileRate(0)
}

// contention is what the profiles found.
type contention struct {
	mutex, block  []report.ContentionSite
	clientLockMS  float64
	setPriorityMS float64
	artifacts     map[string]string
}

// collectProfiles writes the mutex and block profiles to dir (pprof's
// format, for go tool pprof) and sums up their most contended sites. The
// block sites are those in momoshtrem's code (the lab's aside), but for
// goroutines idling in their own loop: where the streaming path waited, a
// read for its piece above all.
func collectProfiles(dir string) contention {
	c := contention{artifacts: map[string]string{}}
	for _, name := range []string{"mutex", "block"} {
		prof := pprof.Lookup(name)
		if prof == nil {
			continue
		}
		if dir != "" {
			file := name + ".pprof"
			if f, err := os.Create(filepath.Join(dir, file)); err == nil {
				if prof.WriteTo(f, 0) == nil {
					c.artifacts[name+"-profile"] = file
				}
				f.Close()
			}
		}
		var buf bytes.Buffer
		if prof.WriteTo(&buf, 1) != nil {
			continue
		}
		recs := parseContention(buf.String())
		if name == "mutex" {
			for _, r := range recs {
				if r.clientLock {
					c.clientLockMS += r.delayMS
				}
				if r.setPriority {
					c.setPriorityMS += r.delayMS
				}
			}
			c.mutex = top(recs, func(contentionRecord) bool { return true })
		} else {
			c.block = top(recs, func(r contentionRecord) bool { return r.ownCode && !r.idle() })
		}
	}
	return c
}

// contentionRecord is one stack of a debug=1 mutex or block profile.
type contentionRecord struct {
	count   int64
	delayMS float64
	frames  []string // functions, innermost first
	// clientLock: the anacrolix client lock; setPriority: under a piece
	// SetPriority (F7); ownCode: momoshtrem's code, not the lab's.
	clientLock, setPriority, ownCode bool
}

var (
	cyclesLine = regexp.MustCompile(`^cycles/second=(\d+)`)
	recordLine = regexp.MustCompile(`^(\d+) (\d+) @`)
)

// parseContention parses a debug=1 mutex or block profile: a header with
// cycles/second, then records "<cycles> <count> @ <pcs>" each followed by
// "#  <pc>  <func>+<off>  <file>:<line>" frames. The runtime has already
// scaled the values by their sampling (Go 1.23 and later).
func parseContention(text string) []contentionRecord {
	cps := 1.0
	var out []contentionRecord
	var cur *contentionRecord
	var cycles int64
	flush := func() {
		if cur != nil {
			cur.delayMS = float64(cycles) / cps * 1000
			classify(cur)
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case cyclesLine.MatchString(line):
			v, _ := strconv.ParseFloat(cyclesLine.FindStringSubmatch(line)[1], 64)
			cps = max(v, 1)
		case recordLine.MatchString(line):
			flush()
			m := recordLine.FindStringSubmatch(line)
			cycles, _ = strconv.ParseInt(m[1], 10, 64)
			n, _ := strconv.ParseInt(m[2], 10, 64)
			cur = &contentionRecord{count: n}
		case strings.HasPrefix(line, "#") && cur != nil:
			if fields := strings.Fields(line); len(fields) >= 3 {
				fn := fields[2]
				if i := strings.LastIndex(fn, "+0x"); i > 0 {
					fn = fn[:i]
				}
				cur.frames = append(cur.frames, fn)
			}
		case strings.TrimSpace(line) == "":
			flush()
		}
	}
	flush()
	return out
}

// lockFrames are the frames of locking itself: the runtime's, sync's and
// anacrolix's lock wrappers. A record's site is its first frame past them.
var lockFrames = []string{
	"runtime.", "sync.", "internal/sync.", "github.com/anacrolix/sync.",
	"github.com/anacrolix/torrent.(*lockWithDeferreds).",
	"github.com/anacrolix/torrent.(*Client).lock", "github.com/anacrolix/torrent.(*Client).unlock",
	"github.com/anacrolix/torrent.(*Client).rLock", "github.com/anacrolix/torrent.(*Client).rUnlock",
}

const (
	momoshtremCode = "github.com/shapedtime/momoshtrem/internal/"
	labCode        = "github.com/shapedtime/momoshtrem/internal/streamlab/"
)

func classify(r *contentionRecord) {
	for _, f := range r.frames {
		switch {
		case strings.HasPrefix(f, "github.com/anacrolix/torrent.(*lockWithDeferreds)."),
			strings.HasPrefix(f, "github.com/anacrolix/torrent.(*Client).lock"),
			strings.HasPrefix(f, "github.com/anacrolix/torrent.(*Client).unlock"),
			strings.HasPrefix(f, "github.com/anacrolix/torrent.(*Client).rLock"),
			strings.HasPrefix(f, "github.com/anacrolix/torrent.(*Client).rUnlock"):
			r.clientLock = true
		case strings.HasSuffix(f, ".(*Piece).SetPriority"), strings.HasSuffix(f, ".(*File).SetPriority"):
			r.setPriority = true
		}
		if strings.HasPrefix(f, momoshtremCode) && !strings.HasPrefix(f, labCode) {
			r.ownCode = true
		}
	}
}

// site is the record's first frame past locking, and its next callers;
// ok is false when every frame is locking (the runtime's own locks).
func (r contentionRecord) site() (name string, callers []string, ok bool) {
	for i, f := range r.frames {
		if !slices.ContainsFunc(lockFrames, func(p string) bool { return strings.HasPrefix(f, p) }) {
			return f, slices.Clone(r.frames[i+1 : min(i+4, len(r.frames))]), true
		}
	}
	return "", nil, false
}

// idle reports whether a block record is a goroutine's own loop waiting
// for work (a ticker, a stop channel): nothing past its site but the
// goroutine's start.
func (r contentionRecord) idle() bool {
	_, callers, ok := r.site()
	if !ok {
		return true
	}
	for _, f := range callers {
		if !strings.Contains(f, ".gowrap") && !strings.HasPrefix(f, "runtime.") {
			return false
		}
	}
	return true
}

// top sums the records keep accepts by site and returns the topSites with
// the most delay.
func top(recs []contentionRecord, keep func(contentionRecord) bool) []report.ContentionSite {
	bySite := map[string]*report.ContentionSite{}
	for _, r := range recs {
		name, callers, ok := r.site()
		if !ok || !keep(r) {
			continue
		}
		s, ok := bySite[name]
		if !ok {
			s = &report.ContentionSite{Site: name, Stack: callers}
			bySite[name] = s
		}
		s.Count += r.count
		s.DelayMS += r.delayMS
		var notes []string
		if r.clientLock {
			notes = append(notes, "anacrolix client lock")
		}
		if r.setPriority {
			notes = append(notes, "SetPriority (F7)")
		}
		for _, n := range notes {
			if !strings.Contains(s.Note, n) {
				s.Note = strings.TrimPrefix(s.Note+", "+n, ", ")
			}
		}
	}
	out := make([]report.ContentionSite, 0, len(bySite))
	for _, s := range bySite {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b report.ContentionSite) int {
		return cmp.Or(cmp.Compare(b.DelayMS, a.DelayMS), cmp.Compare(a.Site, b.Site))
	})
	if len(out) > topSites {
		out = out[:topSites]
	}
	return out
}
