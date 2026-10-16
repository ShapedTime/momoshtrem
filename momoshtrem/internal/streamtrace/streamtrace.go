// Package streamtrace is the streaming path's flight recorder.
//
// It keeps the latest events of the streaming path in a fixed ring in
// memory: HTTP requests, VFS opens, torrent reads, piece priorities,
// torrents, sessions and HDRezka bodies. The debug endpoints read them in
// production, where the lab can't reach (docs/streamlab/PLAN.md §7).
//
// It must never change how production streams:
//   - At LevelOff a hook is one atomic load and a return: the check inlines
//     into the caller, and the ring isn't even allocated.
//   - Recording is lock-free and lossy. A writer claims a slot by sequence
//     number and never waits; a reader that falls behind loses the oldest
//     events, and Since says how many.
//   - Emitting doesn't allocate or format. Events carry numbers. Strings
//     (paths, User-Agents, info hashes) are interned once, when a request
//     or a file opens, into small IDs.
//   - It imports the standard library only.
//
// Hooks use the process-wide recorder:
//
//	tr := streamtrace.Default()
//	file := tr.NewID()                     // when the file opens
//	path := tr.Intern(name)                // likewise
//	tr.TFRead(file, off, n, dur, 0, flags) // in the read path
//
// A hook's arguments are evaluated even when tracing is off, so it passes
// values its code already computed. A hook that has to compute something
// (a duration, say) checks On first.
package streamtrace

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level is how much a recorder keeps.
type Level int32

const (
	// LevelOff records nothing: each hook is one atomic load.
	LevelOff Level = iota
	// LevelSummary records requests, opens and closes, priority changes,
	// torrent and session changes, HDRezka bodies, and the notable reads
	// (slow, timed out, drained, first byte, seek, failed): under ~50
	// events/s during playback.
	LevelSummary
	// LevelDetailed adds every read and every debounced priority update,
	// for the lab.
	LevelDetailed
)

var levelNames = [...]string{LevelOff: "off", LevelSummary: "summary", LevelDetailed: "detailed"}

func (l Level) String() string {
	if l >= 0 && int(l) < len(levelNames) {
		return levelNames[l]
	}
	return "level(" + strconv.Itoa(int(l)) + ")"
}

// ParseLevel reads a level name: off, summary or detailed (any case,
// surrounding spaces ignored). Anything else is an error, the empty string
// included: an unset STREAM_TRACE is for the caller to default.
func ParseLevel(s string) (Level, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	for l, n := range levelNames {
		if name == n {
			return Level(l), nil
		}
	}
	return LevelOff, errors.New("streamtrace: unknown level " + strconv.Quote(s) + " (want off, summary or detailed)")
}

// DefaultSize is the default ring's capacity: 64Ki events, 4 MiB.
const DefaultSize = 1 << 16

// cacheLine separates the fields every emit reads from those it writes.
// 128 bytes covers arm64's lines as well as amd64's.
const cacheLine = 128

// Recorder is a flight recorder: a fixed ring of events and the strings
// they refer to. Its methods are safe for concurrent use. The zero value
// isn't usable: use New, or Default.
type Recorder struct {
	// Read by every emit, written only by the first SetLevel that turns it
	// on (under mu, before its level store, which orders it before every
	// emit that sees a level above off).
	level atomic.Int32
	mask  uint64
	slots []slot
	drops []atomic.Uint64 // per slot, the newest seq its writer dropped
	start time.Time
	_     [cacheLine]byte

	// head is the last sequence number claimed. Every emit adds to it.
	head atomic.Uint64
	_    [cacheLine]byte

	ids     atomic.Uint64 // NewID's counter
	dropped atomic.Uint64 // events their writer dropped
	size    int
	mu      sync.Mutex // SetLevel's allocation; Since reads the ring under it
	strs    strtab
}

// std is the process-wide recorder. A value, not a pointer: Default()
// inlines to its address, so a hook's check is a single load.
var std Recorder

func init() { std.init(DefaultSize) }

// Default returns the process-wide recorder. It starts at LevelOff, with
// its clock at the process start (package initialisation).
func Default() *Recorder { return &std }

// New returns a recorder for size events, rounded up to a power of two
// (DefaultSize when size ≤ 0), at LevelOff. Production uses Default; New
// is for tests.
func New(size int) *Recorder {
	r := &Recorder{}
	r.init(size)
	return r
}

func (r *Recorder) init(size int) {
	if size <= 0 {
		size = DefaultSize
	}
	n := 1
	for n < size {
		n <<= 1
	}
	r.size = n
	r.start = time.Now()
}

// SetLevel changes what the recorder keeps. The first level above off
// allocates the ring. Turning it off keeps the ring and its events.
// Values outside off…detailed are clamped.
func (r *Recorder) SetLevel(l Level) {
	l = max(LevelOff, min(l, LevelDetailed))
	r.mu.Lock()
	defer r.mu.Unlock()
	if l != LevelOff && r.slots == nil {
		r.slots = make([]slot, r.size)
		r.drops = make([]atomic.Uint64, r.size)
		r.mask = uint64(r.size - 1)
	}
	r.level.Store(int32(l))
}

// Level returns the current level.
func (r *Recorder) Level() Level { return Level(r.level.Load()) }

// On reports whether events at level l are recorded: a guard for a hook
// that must compute an argument.
func (r *Recorder) On(l Level) bool { return r.level.Load() >= int32(l) }

// NewID returns a new ID for a request, file, reader or session, unique
// in this recorder; 0 while off. The events of one file or request carry
// its ID.
func (r *Recorder) NewID() uint64 {
	if r.level.Load() == int32(LevelOff) {
		return 0
	}
	return r.ids.Add(1)
}

// Stats describes a recorder, for the debug endpoints.
type Stats struct {
	Level Level
	// Start is when the recorder's clock started: an event's Time is
	// relative to it.
	Start time.Time
	// Size is the ring's capacity in events.
	Size int
	// Head is the newest sequence number claimed: Head events were emitted.
	Head uint64
	// Dropped counts the events their writer gave up, because their slot
	// was still being filled by a writer a whole lap behind.
	Dropped uint64
	// Strings is how many strings are interned, and StringsRefused how many
	// Intern calls returned 0 because the table was full.
	Strings        int
	StringsRefused uint64
}

// Stats returns the recorder's counters.
func (r *Recorder) Stats() Stats {
	n, refused := r.strs.stats()
	return Stats{
		Level:          r.Level(),
		Start:          r.start,
		Size:           r.size,
		Head:           r.head.Load(),
		Dropped:        r.dropped.Load(),
		Strings:        n,
		StringsRefused: refused,
	}
}
