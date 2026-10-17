package scenario

import (
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/observe"
)

// Faults a script injects on purpose declare themselves (Env.Fault), per
// title: a paused seeder, a vanished proxy, a stalled CDN. A declared fault
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
	resumes []func() // PauseSeeder's
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

// Fault declares a fault starting now that starves t (a paused seeder, a
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

// PauseSeeder pauses t's seeder (swarm.Seeder.Pause) as a declared fault,
// until the returned func resumes it; calling it again does nothing. A
// pause the script doesn't resume is resumed when the script ends.
func (e *Env) PauseSeeder(t *Title) (resume func()) {
	end := e.Fault("seeder-pause", t)
	t.Seeder.Pause()
	var once sync.Once
	resume = func() {
		once.Do(func() {
			t.Seeder.Resume()
			end()
		})
	}
	e.faults.mu.Lock()
	e.faults.resumes = append(e.faults.resumes, resume)
	e.faults.mu.Unlock()
	return resume
}

// faultedDuring reports whether a fault declared on t (or on every title)
// ran at some point of [a, b].
func (e *Env) faultedDuring(t *Title, a, b time.Time) bool {
	key := faultKey(t)
	e.faults.mu.Lock()
	defer e.faults.mu.Unlock()
	for _, f := range e.faults.list {
		if (f.key == "" || f.key == key) && !f.start.After(b) && (f.end.IsZero() || !f.end.Before(a)) {
			return true
		}
	}
	return false
}

// endFaults, once the script has ended, notes the faults it left running,
// ends them and resumes the seeders it paused, so the teardown and the
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
