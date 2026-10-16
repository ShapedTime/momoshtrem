package torrent

import (
	"math"
	"slices"
	"sync"
	"time"
)

// The speed sampler turns the client's cumulative byte counters into
// speeds. Every interval it reads each loaded torrent's data bytes read from
// and written to peers (CollectStats), smooths each torrent's rate with an
// exponentially weighted moving average, and keeps the last minute of the
// totals for the Activity summary (PLAN §6 "Torrents and activity (B6)").

const (
	// SpeedInterval is how often the sampler reads the counters.
	SpeedInterval = 2 * time.Second
	// SpeedHistoryLen is how many totals the history keeps: a minute at
	// SpeedInterval.
	SpeedHistoryLen = 30
	// speedSmoothing is the EWMA's time constant: a step in speed shows
	// about two thirds of the way after this long.
	speedSmoothing = 5 * time.Second
)

// StatsSource is where the sampler reads byte counters. torrent.Service
// implements it.
type StatsSource interface {
	CollectStats() []FullStats
}

var _ StatsSource = (Service)(nil)

// Rate is a download and an upload speed, in bytes per second.
type Rate struct {
	Down int64
	Up   int64
}

// SpeedSampler keeps each loaded torrent's smoothed speed and a short
// history of the totals. It's safe for concurrent use.
type SpeedSampler struct {
	interval time.Duration
	now      func() time.Time

	mu      sync.RWMutex
	hashes  map[string]*hashSpeed
	total   Rate
	history []Rate // oldest first, at most SpeedHistoryLen
}

// hashSpeed is one torrent's last reading and smoothed speed.
type hashSpeed struct {
	read, written int64
	at            time.Time
	down, up      float64 // smoothed bytes per second
	rated         bool    // two readings so far: down and up mean something
}

// SpeedOption configures a SpeedSampler.
type SpeedOption func(*SpeedSampler)

// WithSpeedClock replaces the sampler's clock (tests).
func WithSpeedClock(now func() time.Time) SpeedOption {
	return func(s *SpeedSampler) { s.now = now }
}

// NewSpeedSampler creates a sampler that reads every SpeedInterval.
func NewSpeedSampler(opts ...SpeedOption) *SpeedSampler {
	s := &SpeedSampler{
		interval: SpeedInterval,
		now:      time.Now,
		hashes:   map[string]*hashSpeed{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Interval is how often the sampler reads the counters.
func (s *SpeedSampler) Interval() time.Duration { return s.interval }

// Run reads src every interval until stop is closed.
func (s *SpeedSampler) Run(src StatsSource, stop <-chan struct{}) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.Sample(src.CollectStats())
		}
	}
}

// Sample records one reading of every loaded torrent's counters. A torrent
// missing from stats (it was dropped) is forgotten. A new one, or one whose
// counters went back (it was dropped and added again), gets a speed from
// its next reading.
func (s *SpeedSampler) Sample(stats []FullStats) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]bool, len(stats))
	for _, st := range stats {
		seen[st.InfoHash] = true
		h, ok := s.hashes[st.InfoHash]
		if !ok || st.BytesReadData < h.read || st.BytesWrittenData < h.written {
			s.hashes[st.InfoHash] = &hashSpeed{read: st.BytesReadData, written: st.BytesWrittenData, at: now}
			continue
		}
		dt := now.Sub(h.at).Seconds()
		if dt <= 0 {
			continue
		}
		down := float64(st.BytesReadData-h.read) / dt
		up := float64(st.BytesWrittenData-h.written) / dt
		if h.rated {
			alpha := 1 - math.Exp(-dt/speedSmoothing.Seconds())
			h.down += alpha * (down - h.down)
			h.up += alpha * (up - h.up)
		} else {
			h.down, h.up, h.rated = down, up, true
		}
		h.read, h.written, h.at = st.BytesReadData, st.BytesWrittenData, now
	}

	var total Rate
	for hash, h := range s.hashes {
		if !seen[hash] {
			delete(s.hashes, hash)
			continue
		}
		r := h.rate()
		total.Down += r.Down
		total.Up += r.Up
	}
	s.total = total
	if len(s.history) == SpeedHistoryLen {
		copy(s.history, s.history[1:])
		s.history = s.history[:SpeedHistoryLen-1]
	}
	s.history = append(s.history, total)
}

// Rate returns a torrent's smoothed speed; zero before its second reading
// and for a torrent that isn't loaded.
func (s *SpeedSampler) Rate(hash string) Rate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if h, ok := s.hashes[hash]; ok {
		return h.rate()
	}
	return Rate{}
}

// Total returns the sum of every loaded torrent's smoothed speed at the
// last reading.
func (s *SpeedSampler) Total() Rate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.total
}

// History returns the totals of the last SpeedHistoryLen readings, oldest
// first: fewer right after startup.
func (s *SpeedSampler) History() []Rate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.history)
}

func (h *hashSpeed) rate() Rate {
	if !h.rated {
		return Rate{}
	}
	return Rate{Down: int64(math.Round(h.down)), Up: int64(math.Round(h.up))}
}
