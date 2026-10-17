package observe

import (
	"strings"
	"testing"
	"time"
)

// A fault covers its own torrent; one of every torrent covers all; a
// torrent without one is watched.
func TestFaultsPerTorrent(t *testing.T) {
	o, _ := fixture()
	both := map[string]bool{"aaaa": true, "bbbb": true}
	if o.coveredByFaults(both) {
		t.Fatal("covered without a fault")
	}
	endA := o.Fault("seeder-pause", "aaaa")
	if !o.coveredByFaults(map[string]bool{"aaaa": true}) {
		t.Error("aaaa's own fault doesn't cover it")
	}
	if o.coveredByFaults(both) {
		t.Error("aaaa's fault covers bbbb")
	}
	endAll := o.Fault("link-down", "")
	if !o.coveredByFaults(both) {
		t.Error("a fault of every torrent doesn't cover them")
	}
	endAll()
	endA()
	endA() // a second end is a no-op
	if o.coveredByFaults(map[string]bool{"aaaa": true}) {
		t.Error("an ended fault still covers")
	}
	o.Fault("forgotten", "bbbb")
	if open := o.EndFaults(); len(open) != 1 || open[0] != "forgotten" {
		t.Errorf("open = %v, want the forgotten one", open)
	}
	if open := o.EndFaults(); len(open) != 0 {
		t.Errorf("ended twice: %v", open)
	}
	var none *Observer
	none.Fault("x", "")()
	if none.EndFaults() != nil {
		t.Error("a nil observer has faults")
	}
}

// A trip lasts its whole stretch without progress, and keeps #195 only
// past #195's threshold.
func TestTripDuration(t *testing.T) {
	o, t0 := fixture()
	o.cfg.KnownStall = 5 * time.Second
	o.wd = watchdogState{since: t0, tripped: true}
	o.trips = []tripRecord{{at: t0.Add(2 * time.Second), stuck: 2 * time.Second, known: "#195", why: "#195's signature: wanted, incomplete pieces"}}
	o.endStretch(t0.Add(6 * time.Second))
	if o.trips[0].stuck != 6*time.Second || o.wd.tripped {
		t.Fatalf("trip = %+v, watchdog %+v: want the 6 s stretch", o.trips[0], o.wd)
	}
	o.trips = append(o.trips, tripRecord{stuck: 3 * time.Second, known: "#195", why: "#195's signature: wanted, incomplete pieces"})
	o.holdKnownStall()
	if o.trips[0].known != "#195" {
		t.Errorf("a 6 s stall lost #195: %+v", o.trips[0])
	}
	if o.trips[1].known != "" || !strings.Contains(o.trips[1].why, "under its 5s") {
		t.Errorf("a 3 s stall kept #195: %+v", o.trips[1])
	}
}
