package torznab

import (
	"sync"
	"time"
)

// Status is an indexer's state: in health, its latest search; in a fan-out,
// its requests so far.
type Status string

const (
	StatusPending Status = "pending" // fan-out only: a request is still running
	StatusOK      Status = "ok"
	StatusTimeout Status = "timeout"
	StatusError   Status = "error"
	StatusUnknown Status = "unknown" // health only: no search since startup
)

// IndexerHealth is one configured indexer's latest search outcome since
// startup.
type IndexerHealth struct {
	ID     string
	Name   string
	Status Status     // ok, timeout, error or unknown
	MS     *int64     // how long the latest search took; nil when unknown
	Since  *time.Time // when the current timeout or error streak began; nil unless failing
	Error  string     // why the latest search failed; safe to show
}

// healthTracker keeps each indexer's latest outcome.
type healthTracker struct {
	mu   sync.Mutex
	byID map[string]healthEntry
}

type healthEntry struct {
	status Status
	ms     int64
	since  time.Time // zero when ok
	err    string
}

func newHealthTracker() *healthTracker {
	return &healthTracker{byID: map[string]healthEntry{}}
}

// record stores an outcome. A failure after a failure (a timeout after an
// error too) continues the streak and keeps its start.
func (h *healthTracker) record(id string, status Status, elapsed time.Duration, errText string, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := healthEntry{status: status, ms: elapsed.Milliseconds(), err: errText}
	if status != StatusOK {
		e.since = now
		if prev, ok := h.byID[id]; ok && prev.status != StatusOK {
			e.since = prev.since
		}
	}
	h.byID[id] = e
}

// Snapshot is the health of each configured indexer, in Jackett's order,
// from the list Indexers last fetched (nil before the first fetch).
// Indexers with no search since startup are StatusUnknown.
func (c *Client) Snapshot() []IndexerHealth {
	c.mu.Lock()
	list := c.indexers
	c.mu.Unlock()
	if list == nil {
		return nil
	}

	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	out := make([]IndexerHealth, 0, len(list))
	for _, ix := range list {
		h := IndexerHealth{ID: ix.ID, Name: ix.Title, Status: StatusUnknown}
		if e, ok := c.health.byID[ix.ID]; ok {
			ms := e.ms
			h.Status, h.MS, h.Error = e.status, &ms, e.err
			if !e.since.IsZero() {
				since := e.since
				h.Since = &since
			}
		}
		out = append(out, h)
	}
	return out
}
