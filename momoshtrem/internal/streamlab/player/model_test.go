package player

import (
	"testing"
	"time"
)

// The model is driven with a fake clock: every number below is exact.

const mib = 1 << 20

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

func cfg() Config {
	return Config{Bitrate: mib, MaxBuffer: 8 * mib}.WithDefaults()
}

func TestConfigDefaults(t *testing.T) {
	c := Config{Bitrate: mib}.WithDefaults()
	if c.Playable != 2*mib || c.Resume != 2*mib || c.MaxBuffer != 16*mib || c.Chunk != 32<<10 {
		t.Errorf("defaults = %+v", c)
	}
	c = Config{Bitrate: mib, Playable: 3 * mib, MaxBuffer: mib}.WithDefaults()
	if c.Resume != 3*mib || c.MaxBuffer != 3*mib {
		t.Errorf("resume follows playable, max buffer holds it: %+v", c)
	}
}

// Playback starts at 2 s of the bitrate buffered; TTFB is the first byte.
func TestStart(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(300*time.Millisecond), 512<<10)
	if m.State() != Buffering {
		t.Fatalf("state %v after half a MiB, want buffering", m.State())
	}
	m.Receive(at(time.Second), 3*mib/2)
	if m.State() != Playing {
		t.Fatalf("state %v at 2 MiB, want playing", m.State())
	}
	s := m.End(at(time.Second))
	if s.TTFB != 300*time.Millisecond || s.TimeToPlayable != time.Second || !s.Playable {
		t.Errorf("ttfb %v, time to playable %v, playable %v", s.TTFB, s.TimeToPlayable, s.Playable)
	}
}

// A buffer that runs empty stalls at the moment it emptied, where the
// playhead is; the stall ends once Resume bytes are back.
func TestStall(t *testing.T) {
	m := NewModel(cfg(), 10*mib, t0)
	m.Receive(at(0), 2*mib) // playable at once
	// Nothing arrives for 5 s: the 2 MiB last 2 s at 1 MiB/s.
	m.Receive(at(5*time.Second), mib)
	if m.State() != Stalled {
		t.Fatalf("state %v, want stalled", m.State())
	}
	m.Receive(at(6*time.Second), mib) // 2 MiB buffered again: resumes
	if m.State() != Playing {
		t.Fatalf("state %v after the resume bytes, want playing", m.State())
	}
	s := m.End(at(6 * time.Second))
	if len(s.Stalls) != 1 {
		t.Fatalf("stalls = %+v, want one", s.Stalls)
	}
	st := s.Stalls[0]
	if st.At != 2*time.Second || st.Duration != 4*time.Second || st.Position != 12*mib || st.Open {
		t.Errorf("stall = %+v, want at 2s for 4s at %d", st, 12*mib)
	}
	if s.StallTime != 4*time.Second || s.PlayTime != 2*time.Second {
		t.Errorf("stall time %v, play time %v; want 4s and 2s", s.StallTime, s.PlayTime)
	}
	if s.Consumed != 2*mib || s.OverRead != 2*mib {
		t.Errorf("consumed %d, over-read %d; want 2 MiB each", s.Consumed, s.OverRead)
	}
}

// A stall still going when the playback ends counts up to the end.
func TestOpenStall(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(0), 2*mib)
	s := m.End(at(10 * time.Second))
	if len(s.Stalls) != 1 || !s.Stalls[0].Open || s.Stalls[0].Duration != 8*time.Second {
		t.Fatalf("stalls = %+v, want one open for 8s", s.Stalls)
	}
	again := m.End(at(20 * time.Second))
	if len(again.Stalls) != 1 {
		t.Errorf("End twice counted the stall twice: %+v", again.Stalls)
	}
}

// The end of the file isn't a stall: the playback ends.
func TestEOF(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(0), mib)
	m.SetEOF(at(0)) // a short tail: playable at EOF
	if m.State() != Playing {
		t.Fatalf("state %v, want playing a short tail", m.State())
	}
	m.Advance(at(2 * time.Second))
	if m.State() != Ended {
		t.Fatalf("state %v, want ended", m.State())
	}
	s := m.End(at(3 * time.Second))
	if len(s.Stalls) != 0 || !s.EOF || s.PlayTime != time.Second || s.OverRead != 0 {
		t.Errorf("stats = %+v", s)
	}
}

// A pause stops the playhead: no stall, the pause counted apart.
func TestPause(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(0), 3*mib)
	m.Advance(at(time.Second))
	m.Pause(at(time.Second))
	m.Advance(at(10 * time.Second))
	if m.Buffer() != 2*mib {
		t.Fatalf("buffer %d after a pause, want 2 MiB", m.Buffer())
	}
	m.Resume(at(10 * time.Second))
	m.Advance(at(11 * time.Second))
	s := m.End(at(11 * time.Second))
	if len(s.Stalls) != 0 || s.PauseTime != 9*time.Second || s.PlayTime != 2*time.Second || s.Consumed != 2*mib {
		t.Errorf("stats = %+v", s)
	}
}

// A pause during a stall ends it; resuming without the resume bytes
// stalls again.
func TestPauseWhileStalled(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(0), 2*mib)
	m.Advance(at(3 * time.Second)) // stalled since 2 s
	m.Pause(at(4 * time.Second))
	m.Resume(at(9 * time.Second))
	if m.State() != Stalled {
		t.Fatalf("state %v after resuming an empty buffer, want stalled", m.State())
	}
	m.Receive(at(10*time.Second), 2*mib)
	s := m.End(at(10 * time.Second))
	if len(s.Stalls) != 2 || s.StallTime != 3*time.Second || s.PauseTime != 5*time.Second {
		t.Errorf("stalls %+v, stall time %v, pause time %v; want 2 stalls, 3s, 5s", s.Stalls, s.StallTime, s.PauseTime)
	}
}

// A stall's resume bytes that arrive during a pause end it for good: no
// zero-length stall on resuming.
func TestPauseWhileStalledRefilled(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(0), 2*mib)
	m.Advance(at(3 * time.Second)) // stalled since 2 s
	m.Pause(at(4 * time.Second))
	m.Receive(at(6*time.Second), 2*mib) // refilled while paused
	m.Resume(at(9 * time.Second))
	if m.State() != Playing {
		t.Fatalf("state %v after resuming a refilled buffer, want playing", m.State())
	}
	s := m.End(at(9 * time.Second))
	if len(s.Stalls) != 1 || s.StallTime != 2*time.Second {
		t.Errorf("stalls %+v, stall time %v; want the one 2 s stall", s.Stalls, s.StallTime)
	}
}

func TestStallTimeSoFar(t *testing.T) {
	m := NewModel(cfg(), 0, t0)
	m.Receive(at(0), 2*mib)
	m.Advance(at(5 * time.Second))
	if got := m.StallTime(at(5 * time.Second)); got != 3*time.Second {
		t.Errorf("StallTime = %v during a stall from 2s, want 3s", got)
	}
}

func TestSummary(t *testing.T) {
	st := SessionStats{Plays: []PlayRecord{
		{Stats: Stats{TTFB: 100 * time.Millisecond, TimeToPlayable: time.Second, Playable: true,
			Stalls: []Stall{{Duration: time.Second}}, StallTime: time.Second, PlayTime: 3 * time.Second, OverRead: 10}},
		{Stats: Stats{Playable: true, PlayTime: 4 * time.Second, OverRead: 5}, Seek: true, SeekLatency: 700 * time.Millisecond},
		{Stats: Stats{}, Seek: true}, // never playable: no latency
	}}
	sum := st.Summary()
	if sum.TTFB != 100*time.Millisecond || sum.TimeToPlayable != time.Second || sum.Stalls != 1 ||
		len(sum.SeekLatencies) != 1 || sum.SeekLatencies[0] != 700*time.Millisecond || sum.OverRead != 15 {
		t.Errorf("summary = %+v", sum)
	}
	if sum.RebufferRatio != 1.0/8 {
		t.Errorf("rebuffer ratio %v, want 1/8", sum.RebufferRatio)
	}
}
