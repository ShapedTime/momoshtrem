// Package player is streamlab's player model (docs/streamlab/PLAN.md §5):
// what a viewer of a stream feels. A Model is the buffer of one playback:
// bytes arrive from the network, a playhead consumes them at the bitrate,
// and the viewer sees a start delay and stalls. A Session drives Models
// over real WebDAV requests (client.Stream), reading as fast as the buffer
// allows and checking every byte against the media.
//
// The model is pure: every method takes the time, so tests drive it with
// a fake clock and the driver with the wall clock.
package player

import (
	"time"
)

// Config is the player's buffer model.
type Config struct {
	// Bitrate is how fast the playhead consumes the stream, in bytes per
	// second.
	Bitrate int64
	// Playable is how much must be buffered before playback starts:
	// DefaultPlayableSeconds of the bitrate when 0.
	Playable int64
	// Resume is how much must be buffered again to end a stall: Playable
	// when 0.
	Resume int64
	// MaxBuffer caps the buffer: the reader stops reading while it holds
	// this much. DefaultMaxBufferSeconds of the bitrate when 0.
	MaxBuffer int64
	// Chunk is the size of each read: http.ServeContent's copy buffer
	// when 0.
	Chunk int
}

// Defaults of Config.
const (
	DefaultPlayableSeconds  = 2
	DefaultMaxBufferSeconds = 16
	DefaultChunk            = 32 << 10
)

// WithDefaults fills the fields left 0.
func (c Config) WithDefaults() Config {
	if c.Bitrate <= 0 {
		c.Bitrate = 1 << 20
	}
	if c.Playable <= 0 {
		c.Playable = DefaultPlayableSeconds * c.Bitrate
	}
	if c.Resume <= 0 {
		c.Resume = c.Playable
	}
	if c.MaxBuffer <= 0 {
		c.MaxBuffer = DefaultMaxBufferSeconds * c.Bitrate
	}
	c.MaxBuffer = max(c.MaxBuffer, c.Playable, c.Resume)
	if c.Chunk <= 0 {
		c.Chunk = DefaultChunk
	}
	return c
}

// State is where a playback is.
type State int

const (
	// Buffering: not yet playable, the playhead waits.
	Buffering State = iota
	// Playing: the playhead consumes the buffer at the bitrate.
	Playing
	// Stalled: the buffer ran empty while playing; the playhead waits for
	// Resume bytes.
	Stalled
	// Paused: the viewer paused; the playhead waits.
	Paused
	// Ended: the playhead reached the end of the file.
	Ended
)

func (s State) String() string {
	switch s {
	case Buffering:
		return "buffering"
	case Playing:
		return "playing"
	case Stalled:
		return "stalled"
	case Paused:
		return "paused"
	case Ended:
		return "ended"
	}
	return "unknown"
}

// Stall is one time the buffer ran empty during playback.
type Stall struct {
	// At is when it began, from the playback's start.
	At time.Duration
	// Duration is how long it lasted; up to the playback's end when Open.
	Duration time.Duration
	// Position is the file offset of the playhead.
	Position int64
	// Open is set when the playback ended stalled.
	Open bool
}

// Model is the buffer of one playback from a file offset: what was
// received, where the playhead is, and when it waited. It isn't safe for
// concurrent use.
type Model struct {
	cfg   Config
	from  int64
	start time.Time

	received int64
	consumed float64 // bytes; a float so a fraction of a byte carries over
	last     time.Time
	state    State
	eof      bool
	resumeTo State // the state a pause returns to

	firstByte  time.Time
	playableAt time.Time
	endedAt    time.Time

	stallStart time.Time
	stallPos   int64
	stalls     []Stall
	stallTime  time.Duration
	playTime   time.Duration
	pauseTime  time.Duration
	pauseStart time.Time
}

// NewModel is a playback from the file offset from, requested at start.
func NewModel(cfg Config, from int64, start time.Time) *Model {
	return &Model{cfg: cfg.WithDefaults(), from: from, start: start, last: start}
}

// Config is the model's config, defaults filled.
func (m *Model) Config() Config { return m.cfg }

// State is the playback's state as of the last call.
func (m *Model) State() State { return m.state }

// Buffer is how many bytes are buffered and not yet played.
func (m *Model) Buffer() int64 { return m.received - int64(m.consumed) }

// Received is how many bytes arrived.
func (m *Model) Received() int64 { return m.received }

// Consumed is how many bytes the playhead played.
func (m *Model) Consumed() int64 { return int64(m.consumed) }

// From is the file offset the playback started at.
func (m *Model) From() int64 { return m.from }

// EOF reports whether the stream reached the end of the file.
func (m *Model) EOF() bool { return m.eof }

// Advance moves the playhead to now. A buffer that runs empty while
// playing starts a stall at the moment it emptied, or ends the playback
// when the stream had reached the end of the file.
func (m *Model) Advance(now time.Time) {
	if now.Before(m.last) {
		return
	}
	if m.state == Playing {
		dt := now.Sub(m.last)
		want := m.consumed + dt.Seconds()*float64(m.cfg.Bitrate)
		if want < float64(m.received) {
			m.consumed = want
			m.playTime += dt
		} else {
			// The buffer emptied before now: at last + what was left.
			left := float64(m.received) - m.consumed
			empty := m.last.Add(time.Duration(left / float64(m.cfg.Bitrate) * float64(time.Second)))
			m.playTime += empty.Sub(m.last)
			m.consumed = float64(m.received)
			if m.eof {
				m.state = Ended
				m.endedAt = empty
			} else {
				m.state = Stalled
				m.stallStart = empty
				m.stallPos = m.from + m.received
			}
		}
	}
	m.last = now
}

// Receive records n bytes arriving at now.
func (m *Model) Receive(now time.Time, n int) {
	m.Advance(now)
	if n > 0 && m.firstByte.IsZero() {
		m.firstByte = now
	}
	m.received += int64(n)
	m.settle(now)
}

// SetEOF records the stream reaching the end of the file at now.
func (m *Model) SetEOF(now time.Time) {
	m.Advance(now)
	m.eof = true
	m.settle(now)
}

// settle starts playback or ends a stall once enough is buffered.
func (m *Model) settle(now time.Time) {
	buf := m.Buffer()
	switch m.state {
	case Buffering:
		if buf >= m.cfg.Playable || (m.eof && buf > 0) {
			m.state = Playing
			m.playableAt = now
		} else if m.eof {
			m.state = Ended
			m.endedAt = now
		}
	case Stalled:
		if buf >= m.cfg.Resume || m.eof {
			m.endStall(now, false)
			m.state = Playing
			if buf == 0 {
				m.state = Ended
				m.endedAt = now
			}
		}
	}
}

func (m *Model) endStall(now time.Time, open bool) {
	d := now.Sub(m.stallStart)
	m.stalls = append(m.stalls, Stall{At: m.stallStart.Sub(m.start), Duration: d, Position: m.stallPos, Open: open})
	m.stallTime += d
}

// Pause stops the playhead at now, as a viewer pausing does.
func (m *Model) Pause(now time.Time) {
	m.Advance(now)
	if m.state == Paused || m.state == Ended {
		return
	}
	if m.state == Stalled {
		// A pause ends the stall: the viewer isn't waiting any more.
		m.endStall(now, false)
		m.resumeTo = Buffering
	} else {
		m.resumeTo = m.state
	}
	m.state = Paused
	m.pauseStart = now
}

// Resume restarts the playhead at now after a Pause. A playback that was
// buffering, or stalled, waits for Playable bytes again.
func (m *Model) Resume(now time.Time) {
	if m.state != Paused {
		return
	}
	m.pauseTime += now.Sub(m.pauseStart)
	m.last = now
	m.state = m.resumeTo
	if m.state == Buffering && m.playableAt.IsZero() {
		m.settle(now)
		return
	}
	if m.state == Buffering {
		// Was stalled: playing again needs the resume threshold.
		m.state = Stalled
		m.stallStart = now
		m.stallPos = m.from + m.received
		m.settle(now)
	}
}

// Stats are a playback's numbers.
type Stats struct {
	From int64
	// TTFB is from the request to the first byte; 0 without one.
	TTFB time.Duration
	// TimeToPlayable is from the request to Playable bytes buffered; 0
	// when it never got there (Playable is false).
	TimeToPlayable time.Duration
	Playable       bool
	Stalls         []Stall
	// StallTime is the stalls' total, PlayTime the time the playhead
	// moved, PauseTime the time the viewer paused.
	StallTime time.Duration
	PlayTime  time.Duration
	PauseTime time.Duration
	Received  int64
	Consumed  int64
	// OverRead is what arrived and was never played: the buffer when the
	// playback ended.
	OverRead int64
	EOF      bool
	// Duration is the playback's wall time, from its request to its end.
	Duration time.Duration
	State    string
}

// End ends the playback at now and returns its numbers. A stall still
// going counts up to now, marked Open.
func (m *Model) End(now time.Time) Stats {
	m.Advance(now)
	switch m.state {
	case Stalled:
		m.endStall(now, true)
		m.state = Buffering // counted once, even if End is called again
		m.stallStart = now
	case Paused:
		m.pauseTime += now.Sub(m.pauseStart)
		m.pauseStart = now
	}
	s := Stats{
		From:      m.from,
		Playable:  !m.playableAt.IsZero(),
		Stalls:    append([]Stall(nil), m.stalls...),
		StallTime: m.stallTime,
		PlayTime:  m.playTime,
		PauseTime: m.pauseTime,
		Received:  m.received,
		Consumed:  int64(m.consumed),
		OverRead:  m.Buffer(),
		EOF:       m.eof,
		Duration:  now.Sub(m.start),
		State:     m.state.String(),
	}
	if !m.firstByte.IsZero() {
		s.TTFB = m.firstByte.Sub(m.start)
	}
	if s.Playable {
		s.TimeToPlayable = m.playableAt.Sub(m.start)
	}
	return s
}

// StallTime is the time stalled so far, a stall still going included.
func (m *Model) StallTime(now time.Time) time.Duration {
	if m.state == Stalled && now.After(m.stallStart) {
		return m.stallTime + now.Sub(m.stallStart)
	}
	return m.stallTime
}

// PlayableAt is when the playback became playable; zero until it did.
func (m *Model) PlayableAt() time.Time { return m.playableAt }

// FirstByte is when the first byte arrived; zero until one did.
func (m *Model) FirstByte() time.Time { return m.firstByte }
