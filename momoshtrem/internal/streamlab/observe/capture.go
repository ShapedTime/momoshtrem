package observe

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The anacrolix messages of its read path (reader.go, v1.60.0): a storage
// read that failed ("error reading", with the piece), then each retry that
// failed: after resetting the storage reader, then after resyncing the
// piece's completion, when the read gives up.
const (
	MsgReadError     = "error reading"
	MsgInitialFailed = "initial read failed"
	MsgResetFailed   = "read failed after reader reset"
	MsgResyncFailed  = "read failed after completion resync"
)

// DiagnosedComponents are the components whose DEBUG records --diagnose
// captures: the streaming path's priorities and the activity manager.
var DiagnosedComponents = []string{"prioritizer", "priority-reader", "activity-manager"}

// maxEvents caps what a capture keeps; past it, records are counted as
// dropped.
const maxEvents = 200_000

// Capture is a slog.Handler that records log records as lab events and
// passes them on to the handler it wraps. Install it as slog's default
// before the stack starts: production's loggers take the default when
// they're built (slog.With("component", ...)).
//
// It records WARN and above from everyone, anacrolix's (through the lab's
// bridge, swarm.Logger) included: its read path logs at ERROR, which the
// known-issue registry needs (F11's proof) and #219 counts. In measure
// mode that's all, and a DEBUG call costs what it costs without the
// capture: Enabled says no. With diagnose, the DiagnosedComponents' DEBUG
// and INFO records are recorded too; they go on to the wrapped handler
// only when it's enabled for them.
type Capture struct {
	inner     slog.Handler
	diagnose  bool
	component string
	prefix    string // the open groups, "a.b."
	attrs     []Attr // the handler's own, flattened
	log       *eventLog
}

// Attr is a captured attribute: groups flattened into the key
// ("torrent.ih"), the value as text.
type Attr struct {
	Key, Value string
}

// LogEvent is a captured record.
type LogEvent struct {
	T         time.Time
	Level     slog.Level
	Component string
	Msg       string
	Attrs     []Attr
}

// Get returns the attribute's value, "" when absent.
func (e LogEvent) Get(key string) string {
	for _, a := range e.Attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return ""
}

// eventLog is what every handler derived from one Capture records into.
type eventLog struct {
	mu      sync.Mutex
	events  []LogEvent
	dropped int
	busy    time.Duration
}

var _ slog.Handler = (*Capture)(nil)

// NewCapture wraps inner; diagnose also records the DiagnosedComponents'
// DEBUG records.
func NewCapture(inner slog.Handler, diagnose bool) *Capture {
	return &Capture{inner: inner, diagnose: diagnose, log: &eventLog{}}
}

// Diagnose reports whether the capture records DEBUG records.
func (c *Capture) Diagnose() bool { return c.diagnose }

func (c *Capture) diagnosed() bool {
	return c.diagnose && slices.Contains(DiagnosedComponents, c.component)
}

// records reports whether a record at level is recorded.
func (c *Capture) records(level slog.Level) bool {
	return level >= slog.LevelWarn || (c.diagnosed() && level >= slog.LevelDebug)
}

// Enabled implements slog.Handler.
func (c *Capture) Enabled(ctx context.Context, level slog.Level) bool {
	return c.records(level) || c.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (c *Capture) Handle(ctx context.Context, r slog.Record) error {
	if c.records(r.Level) {
		start := time.Now()
		ev := LogEvent{T: r.Time, Level: r.Level, Component: c.component, Msg: r.Message,
			Attrs: make([]Attr, len(c.attrs), len(c.attrs)+r.NumAttrs())}
		if ev.T.IsZero() {
			ev.T = start
		}
		copy(ev.Attrs, c.attrs)
		r.Attrs(func(a slog.Attr) bool {
			if c.prefix == "" && a.Key == "component" {
				ev.Component = a.Value.String()
			}
			ev.Attrs = flatten(ev.Attrs, c.prefix, a)
			return true
		})
		c.log.add(ev, time.Since(start))
	}
	if c.inner.Enabled(ctx, r.Level) {
		return c.inner.Handle(ctx, r)
	}
	return nil
}

// WithAttrs implements slog.Handler.
func (c *Capture) WithAttrs(as []slog.Attr) slog.Handler {
	d := *c
	d.inner = c.inner.WithAttrs(as)
	d.attrs = slices.Clip(c.attrs)
	for _, a := range as {
		if c.prefix == "" && a.Key == "component" {
			d.component = a.Value.String()
		}
		d.attrs = flatten(d.attrs, c.prefix, a)
	}
	return &d
}

// WithGroup implements slog.Handler.
func (c *Capture) WithGroup(name string) slog.Handler {
	if name == "" {
		return c
	}
	d := *c
	d.inner = c.inner.WithGroup(name)
	d.prefix = c.prefix + name + "."
	return &d
}

func flatten(out []Attr, prefix string, a slog.Attr) []Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, g := range v.Group() {
			out = flatten(out, p, g)
		}
		return out
	}
	if a.Key == "" {
		return out
	}
	return append(out, Attr{Key: prefix + a.Key, Value: v.String()})
}

func (l *eventLog) add(ev LogEvent, cost time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.busy += cost
	if len(l.events) >= maxEvents {
		l.dropped++
		return
	}
	l.events = append(l.events, ev)
}

// Events returns the records captured at or after since, in the order
// they were recorded (records of concurrent goroutines can be a few
// microseconds out of time order).
func (c *Capture) Events(since time.Time) []LogEvent {
	c.log.mu.Lock()
	defer c.log.mu.Unlock()
	var out []LogEvent
	for _, e := range c.log.events {
		if !e.T.Before(since) {
			out = append(out, e)
		}
	}
	return out
}

// cost is the time spent recording, how many records were kept and how
// many dropped.
func (c *Capture) cost() (busy time.Duration, kept, dropped int) {
	c.log.mu.Lock()
	defer c.log.mu.Unlock()
	return c.log.busy, len(c.log.events), c.log.dropped
}

// proofWindow is how far before a break anacrolix's failed read may have
// been logged for the break to be its: a read of lost data fails within
// milliseconds (three tries), and the body breaks right after.
const proofWindow = 3 * time.Second

// ReadFailed reports whether the capture holds F11's log proof for a body
// that broke at piece of the torrent ih (hex) at the time at: anacrolix
// logged MsgReadError for that piece, then MsgResyncFailed, both within
// proofWindow before the break. The read gave up on data anacrolix
// believed it had (complete, or chunks it had written): the storage lost
// it.
func (c *Capture) ReadFailed(ih string, piece int, at time.Time) bool {
	if c == nil {
		return false
	}
	evs := c.Events(at.Add(-proofWindow))
	p := strconv.Itoa(piece)
	for i, r := range evs {
		if r.Msg != MsgResyncFailed || r.T.After(at.Add(time.Second)) || !sameTorrent(r, ih) {
			continue
		}
		for _, e := range evs[:i] {
			if e.Msg == MsgReadError && e.Get("piece") == p && sameTorrent(e, ih) && !e.T.Before(r.T.Add(-proofWindow)) {
				return true
			}
		}
	}
	return false
}

// sameTorrent reports whether an anacrolix record is of the torrent ih:
// its "torrent.ih" attribute, when it has one.
func sameTorrent(e LogEvent, ih string) bool {
	got := e.Get("torrent.ih")
	return got == "" || ih == "" || strings.EqualFold(got, ih)
}
