package scenario

import (
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/observe"
)

// Faults a script injects on purpose declare themselves (Env.Fault), per
// title: a held seeder, a vanished proxy, a stalled CDN. A declared fault
// excuses only the watchdog: it doesn't trip for that title while one
// runs. It never excuses a broken body: play-ok's excuse for those is
// Env.Expect, narrower, and a body silent during a declared fault isn't
// #195's lost wakeup either. So nothing is excused twice, and a fault
// makes #195 stricter, never wider. A fault the script leaves running
// fails the watchdog check: it would hide any hang after it.

// envFaults are the faults a script declared.
type envFaults struct {
	mu      sync.Mutex
	list    []envFault
	resumes []func() // HoldSeeder's releases
	// open are the faults still running when the script ended.
	open []string
}

type envFault struct {
	name, key  string
	start, end time.Time // end zero: still running
}

// faultKey is the key of a fault on t (observe.Title.FaultKey): its
// torrent, or its path for an HDRezka movie; "" for every title (t nil).
func faultKey(t *Title) string {
	if t == nil {
		return ""
	}
	return observe.Title{Hash: t.Hash, Path: t.Path, Remote: t.Remote != nil}.FaultKey()
}

// Fault declares a fault starting now that starves t (a held seeder, a
// dropped link, a stalled CDN), or every title when t is nil. Call the
// returned func when it ends; calling it again does nothing.
func (e *Env) Fault(name string, t *Title) (end func()) {
	key := faultKey(t)
	endObs := e.obs.Fault(name, key)
	e.faults.mu.Lock()
	e.faults.list = append(e.faults.list, envFault{name: name, key: key, start: time.Now()})
	i := len(e.faults.list) - 1
	e.faults.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			e.faults.mu.Lock()
			if e.faults.list[i].end.IsZero() { // endFaults may have ended it
				e.faults.list[i].end = time.Now()
			}
			e.faults.mu.Unlock()
			endObs()
		})
	}
}

// HoldSeeder holds t's seeder's uploads (swarm.Seeder.HoldUploads) as a
// fault called name, until the returned func releases them; calling it
// again does nothing. A hold the script doesn't release is released when
// the script ends, and fails the watchdog check.
func (e *Env) HoldSeeder(t *Title, name string) (release func()) {
	end := e.Fault(name, t)
	t.Seeder.HoldUploads()
	var once sync.Once
	release = func() {
		once.Do(func() {
			t.Seeder.ReleaseUploads()
			end()
		})
	}
	e.faults.mu.Lock()
	e.faults.resumes = append(e.faults.resumes, release)
	e.faults.mu.Unlock()
	return release
}

// faultExplains reports whether faults declared on t (or on every title)
// explain a body silent from since to at: one overlapped the silence, and
// what's left of it after the last of them ended (nothing, if one still
// ran at at) is shorter than knownStall, #195's threshold. A longer
// silence after the release is a stall of its own: the release is itself
// a writer wakeup #195 can lose.
func (e *Env) faultExplains(t *Title, since, at time.Time, knownStall time.Duration) bool {
	key := faultKey(t)
	e.faults.mu.Lock()
	defer e.faults.mu.Unlock()
	var last time.Time
	overlap := false
	for _, f := range e.faults.list {
		if (f.key != "" && f.key != key) || f.start.After(at) {
			continue
		}
		end := f.end
		if end.IsZero() || end.After(at) {
			end = at
		}
		if end.Before(since) {
			continue
		}
		overlap = true
		if end.After(last) {
			last = end
		}
	}
	return overlap && at.Sub(last) < knownStall
}

// endFaults, once the script has ended, notes the faults it left running,
// ends them and releases the seeders it held, so the teardown and the
// quiescence check run on a healthy swarm.
func (e *Env) endFaults() {
	e.faults.mu.Lock()
	var open []string
	now := time.Now()
	for i := range e.faults.list {
		if e.faults.list[i].end.IsZero() {
			open = append(open, e.faults.list[i].name)
			e.faults.list[i].end = now
		}
	}
	e.faults.open = open
	resumes := e.faults.resumes
	e.faults.mu.Unlock()
	e.obs.EndFaults()
	for _, r := range resumes {
		r()
	}
}
