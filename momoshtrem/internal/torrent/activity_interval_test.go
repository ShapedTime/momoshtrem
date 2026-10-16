package torrent

import (
	"testing"
	"time"
)

// TestActivityManager_SetCheckInterval runs the real idle loop on a short
// interval: a torrent past its idle timeout goes idle at the next check.
func TestActivityManager_SetCheckInterval(t *testing.T) {
	am, clock := newTestManager(false)
	if am.checkInterval != 30*time.Second {
		t.Fatalf("default check interval = %s, want 30s", am.checkInterval)
	}
	net := newFakeNet()
	am.register(amHash, net)
	clock.Advance(2 * time.Minute) // past the 1 min idle timeout

	am.SetCheckInterval(10 * time.Millisecond)
	am.Start()
	t.Cleanup(am.Stop)

	deadline := time.Now().Add(5 * time.Second)
	for am.GetState(amHash) != StateIdle {
		if time.Now().After(deadline) {
			t.Fatalf("state = %s after 5s of 10ms checks, want idle", am.GetState(amHash))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if net.on() {
		t.Error("idle torrent's network is still on")
	}
}
