package observe

import (
	"sync"
	"time"
)

// fault is a fault a scenario declared: a seeder paused, a link dropped.
// hash is the torrent it starves; "" is every torrent.
type fault struct {
	name, hash string
	start, end time.Time // end zero: still running
}

// Fault declares a fault on the torrent hash ("" for every torrent)
// starting now, which the returned func ends. The watchdog doesn't trip
// for that torrent while it runs, and its no-progress time starts over
// when it ends. A fault never ended is one the script forgot: EndFaults
// names it.
func (o *Observer) Fault(name, hash string) (end func()) {
	if o == nil {
		return func() {}
	}
	o.faultMu.Lock()
	defer o.faultMu.Unlock()
	o.faults = append(o.faults, fault{name: name, hash: hash, start: time.Now()})
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

// coveredByFaults reports whether running faults cover every torrent of
// hashes (at least one): each has one of its own, or one of every
// torrent runs.
func (o *Observer) coveredByFaults(hashes map[string]bool) bool {
	if len(hashes) == 0 {
		return false
	}
	o.faultMu.Lock()
	defer o.faultMu.Unlock()
	for h := range hashes {
		covered := false
		for _, f := range o.faults {
			if f.end.IsZero() && (f.hash == "" || f.hash == h) {
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
