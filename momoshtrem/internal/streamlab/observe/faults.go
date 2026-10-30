package observe

import (
	"sync"
	"time"
)

// fault is a fault a scenario declared: a seeder paused, a link dropped, a
// CDN stalled. key is the title it starves (Title.FaultKey: its torrent,
// or an HDRezka movie); "" is every title.
type fault struct {
	name, key  string
	start, end time.Time // end zero: still running
}

// Fault declares a fault on the title key (Title.FaultKey; "" for every
// title) starting now, which the returned func ends. The watchdog doesn't
// trip for that key while it runs, and its no-progress time starts over
// when it ends. A fault never ended is one the script forgot: EndFaults
// names it.
func (o *Observer) Fault(name, key string) (end func()) {
	if o == nil {
		return func() {}
	}
	o.faultMu.Lock()
	defer o.faultMu.Unlock()
	o.faults = append(o.faults, fault{name: name, key: key, start: time.Now()})
	i := len(o.faults) - 1
	var once sync.Once
	return func() {
		once.Do(func() {
			o.faultMu.Lock()
			defer o.faultMu.Unlock()
			o.faults[i].end = time.Now()
		})
	}
}

// coveredByFaults reports whether running faults cover every key (at
// least one): each has one of its own, or one of every title runs.
func (o *Observer) coveredByFaults(keys map[string]bool) bool {
	if len(keys) == 0 {
		return false
	}
	o.faultMu.Lock()
	defer o.faultMu.Unlock()
	for k := range keys {
		covered := false
		for _, f := range o.faults {
			if f.end.IsZero() && (f.key == "" || f.key == k) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// EndFaults ends the faults still running and returns their names: what
// the script left open when it ended.
func (o *Observer) EndFaults() []string {
	if o == nil {
		return nil
	}
	o.faultMu.Lock()
	defer o.faultMu.Unlock()
	var open []string
	now := time.Now()
	for i := range o.faults {
		if o.faults[i].end.IsZero() {
			open = append(open, o.faults[i].name)
			o.faults[i].end = now
		}
	}
	return open
}

// faultList is the declared faults, a running one ending at the stop.
func (o *Observer) faultList() []fault {
	o.faultMu.Lock()
	defer o.faultMu.Unlock()
	out := make([]fault, len(o.faults))
	copy(out, o.faults)
	for i := range out {
		if out[i].end.IsZero() {
			out[i].end = o.stopped
		}
	}
	return out
}
