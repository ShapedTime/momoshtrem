package torrent

import (
	"errors"
	"sync"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
)

// fakeNet records whether a torrent's network is allowed.
type fakeNet struct {
	mu       sync.Mutex
	download bool
	upload   bool
	peers    int
}

func newFakeNet() *fakeNet { return &fakeNet{download: true, upload: true} }

func (f *fakeNet) AllowDataDownload()    { f.set(&f.download, true) }
func (f *fakeNet) DisallowDataDownload() { f.set(&f.download, false) }
func (f *fakeNet) AllowDataUpload()      { f.set(&f.upload, true) }
func (f *fakeNet) DisallowDataUpload()   { f.set(&f.upload, false) }

func (f *fakeNet) Stats() atorrent.TorrentStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return atorrent.TorrentStats{TorrentGauges: atorrent.TorrentGauges{ActivePeers: f.peers}}
}

func (f *fakeNet) set(field *bool, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*field = v
}

func (f *fakeNet) on() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.download != f.upload {
		panic("download and upload disagree")
	}
	return f.download
}

// fakeClock is a settable clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestManager(startPaused bool) (*ActivityManager, *fakeClock) {
	am := NewActivityManager(time.Minute, startPaused)
	clock := newFakeClock()
	am.now = clock.Now
	return am, clock
}

const amHash = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"

func TestActivityManager_PauseRules(t *testing.T) {
	tests := []struct {
		name        string
		startPaused bool
		run         func(t *testing.T, am *ActivityManager, clock *fakeClock, net *fakeNet)
		wantState   TorrentState
		wantNet     bool
		wantPaused  bool
	}{
		{"a read wakes an idle torrent", true, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.MarkActive(amHash)
		}, StateActive, true, false},
		{"pausing a loaded torrent turns its network off", false, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.Pause(amHash, time.Now())
		}, StatePaused, false, true},
		{"a read doesn't wake a paused torrent", true, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.MarkActive(amHash)
			am.Pause(amHash, time.Now())
			am.MarkActive(amHash)
		}, StatePaused, false, true},
		{"the idle check leaves a paused torrent alone", false, func(t *testing.T, am *ActivityManager, clock *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.Pause(amHash, time.Now())
			clock.Advance(time.Hour)
			am.checkIdleTorrents()
		}, StatePaused, false, true},
		{"a pause applies when the torrent registers", false, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.Pause(amHash, time.Now())
			am.register(amHash, net)
		}, StatePaused, false, true},
		{"restored pauses apply when the torrent registers", true, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.RestorePauses(map[string]time.Time{amHash: time.Now()})
			am.register(amHash, net)
			am.MarkActive(amHash)
		}, StatePaused, false, true},
		{"resume turns a paused torrent's network back on", true, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.Pause(amHash, time.Now())
			am.Resume(amHash)
		}, StateActive, true, false},
		{"resume leaves a torrent that isn't paused alone", true, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.Resume(amHash)
		}, StateIdle, false, false},
		{"a pause outlives an unregister", false, func(t *testing.T, am *ActivityManager, _ *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.Pause(amHash, time.Now())
			am.Unregister(amHash)
			am.register(amHash, net)
		}, StatePaused, false, true},
		{"a resumed torrent idles after the timeout", false, func(t *testing.T, am *ActivityManager, clock *fakeClock, net *fakeNet) {
			am.register(amHash, net)
			am.Pause(amHash, time.Now())
			clock.Advance(time.Hour)
			am.Resume(amHash)
			clock.Advance(30 * time.Second)
			am.checkIdleTorrents() // resumed 30 s ago: stays on
			if am.GetState(amHash) != StateActive {
				t.Fatalf("idled %s after resume", am.GetState(amHash))
			}
			clock.Advance(time.Minute)
			am.checkIdleTorrents()
		}, StateIdle, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			am, clock := newTestManager(tt.startPaused)
			net := newFakeNet()
			tt.run(t, am, clock, net)
			if got := am.GetState(amHash); got != tt.wantState {
				t.Errorf("state = %s, want %s", got, tt.wantState)
			}
			if got := net.on(); got != tt.wantNet {
				t.Errorf("network on = %v, want %v", got, tt.wantNet)
			}
			if _, got := am.PausedAt(amHash); got != tt.wantPaused {
				t.Errorf("paused = %v, want %v", got, tt.wantPaused)
			}
			// IsPaused is "network off": idle or paused.
			if got := am.IsPaused(amHash); got != !tt.wantNet {
				t.Errorf("IsPaused = %v, want %v", got, !tt.wantNet)
			}
		})
	}
}

func TestActivityManager_WaitForActivationWhilePaused(t *testing.T) {
	am, _ := newTestManager(true)
	net := newFakeNet()
	am.register(amHash, net)
	am.Pause(amHash, time.Now())

	start := time.Now()
	if err := am.WaitForActivation(amHash, 5*time.Second); !errors.Is(err, ErrPaused) {
		t.Fatalf("WaitForActivation = %v, want ErrPaused", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("WaitForActivation waited %s; want an answer at once", time.Since(start))
	}
	if net.on() || am.GetState(amHash) != StatePaused {
		t.Errorf("waiting woke the torrent: state %s, network %v", am.GetState(amHash), net.on())
	}

	// Paused before it loads: the same.
	if err := am.WaitForActivation("0123456789abcdef0123456789abcdef01234567", time.Second); err != nil {
		t.Fatalf("WaitForActivation of an unknown torrent = %v", err)
	}
	am.Pause("0123456789abcdef0123456789abcdef01234567", time.Now())
	if err := am.WaitForActivation("0123456789abcdef0123456789abcdef01234567", time.Second); !errors.Is(err, ErrPaused) {
		t.Fatalf("WaitForActivation of a paused unloaded torrent = %v, want ErrPaused", err)
	}
}

func TestActivityManager_PauseTimesAndLastAccess(t *testing.T) {
	am, clock := newTestManager(true)
	net := newFakeNet()
	first := clock.Now()
	am.register(amHash, net)
	if at, ok := am.LastAccess(amHash); !ok || !at.Equal(first) {
		t.Fatalf("LastAccess after register = %v %v, want %v", at, ok, first)
	}

	clock.Advance(time.Minute)
	am.MarkActive(amHash)
	read := clock.Now()
	am.Pause(amHash, read)
	clock.Advance(time.Minute)
	am.Pause(amHash, clock.Now()) // idempotent: keeps the first time
	am.MarkActive(amHash)         // ignored while paused
	if at, _ := am.PausedAt(amHash); !at.Equal(read) {
		t.Errorf("PausedAt = %v, want the first pause %v", at, read)
	}
	if at, _ := am.LastAccess(amHash); !at.Equal(read) {
		t.Errorf("LastAccess = %v, want the last read %v", at, read)
	}

	clock.Advance(time.Minute)
	am.Resume(amHash)
	if at, _ := am.LastAccess(amHash); !at.Equal(clock.Now()) {
		t.Errorf("LastAccess after resume = %v, want %v", at, clock.Now())
	}
	if pauses := am.Pauses(); len(pauses) != 0 {
		t.Errorf("Pauses = %v, want none", pauses)
	}

	am.Unregister(amHash)
	if _, ok := am.LastAccess(amHash); ok {
		t.Error("LastAccess of an unregistered torrent should be unknown")
	}
}
