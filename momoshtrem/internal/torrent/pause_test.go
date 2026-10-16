package torrent

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// The service's pauses, over a real client without idle mode (the manager
// that only pauses): a pause applies when the torrent loads, survives its
// removal, and Resume turns the network back on.
func TestService_PausesWithoutIdleMode(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	f := newFixture(t)
	if err := e.store.Save(f.hash, f.mi); err != nil {
		t.Fatal(err)
	}

	// Paused before it loads, by an uppercase hash.
	if err := e.svc.Pause(strings.ToUpper(f.hash)); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, ok := e.svc.PausedAt(f.hash); !ok {
		t.Fatal("PausedAt: not paused")
	}
	if _, err := e.svc.AddTorrent(f.magnet()); err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	st, err := e.svc.GetStatus(f.hash)
	if err != nil {
		t.Fatal(err)
	}
	if e.svc.IsActive(f.hash) || !st.Paused || !st.IsPaused || st.LoadedAt.IsZero() || st.LastActive.IsZero() {
		t.Fatalf("loaded paused torrent: active %v, status %+v", e.svc.IsActive(f.hash), st)
	}

	if err := e.svc.Resume(f.hash); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if st, _ = e.svc.GetStatus(f.hash); !e.svc.IsActive(f.hash) || st.Paused || st.IsPaused {
		t.Fatalf("resumed: active %v, status %+v", e.svc.IsActive(f.hash), st)
	}
	if err := e.svc.Resume(f.hash); err != nil || !e.svc.IsActive(f.hash) {
		t.Fatalf("Resume again: %v, active %v", err, e.svc.IsActive(f.hash))
	}

	// A pause outlives the torrent's removal: it's the caller's to clear.
	if err := e.svc.Pause(f.hash); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveTorrent(f.hash, false); err != nil {
		t.Fatal(err)
	}
	if pauses := e.svc.Pauses(); len(pauses) != 1 || pauses[f.hash].IsZero() {
		t.Fatalf("Pauses after removal = %v", pauses)
	}
	if err := e.svc.Resume(f.hash); err != nil {
		t.Fatal(err)
	}
	if pauses := e.svc.Pauses(); len(pauses) != 0 {
		t.Fatalf("Pauses after resume = %v", pauses)
	}

	for _, bad := range []string{"", "summary", strings.Repeat("z", 40)} {
		if err := e.svc.Pause(bad); !errors.Is(err, ErrInvalidInfoHash) {
			t.Errorf("Pause(%q) = %v, want ErrInvalidInfoHash", bad, err)
		}
		if err := e.svc.Resume(bad); !errors.Is(err, ErrInvalidInfoHash) {
			t.Errorf("Resume(%q) = %v, want ErrInvalidInfoHash", bad, err)
		}
	}
}

func TestService_RestorePauses(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	at := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	e.svc.RestorePauses(map[string]time.Time{
		"DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C": at, // stored by hand, uppercase
		"not a hash": at,
	})
	if got := e.svc.Pauses(); len(got) != 1 || !got["dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"].Equal(at) {
		t.Fatalf("Pauses = %v", got)
	}
}

// Status reports completion from the cache and the sampler's speed.
func TestService_StatusCompletionAndSpeed(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	sampler, clock := newTestSampler()
	e.svc.speeds = sampler
	f := newFixture(t)
	if err := e.store.Save(f.hash, f.mi); err != nil {
		t.Fatal(err)
	}
	e.fill(t, f)
	if _, err := e.svc.AddTorrent(f.magnet()); err != nil {
		t.Fatal(err)
	}

	sample(sampler, clock, reading{f.hash, 0, 0})
	sample(sampler, clock, reading{f.hash, 3_000_000, 1_000})
	st, err := e.svc.GetStatus(f.hash)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Complete || st.Progress != 1 || st.Downloaded != int64(len(f.data)) || st.Peers != 0 {
		t.Errorf("status = %+v, want complete", st)
	}
	if st.DownloadSpeed != 1_500_000 || st.UploadSpeed != 500 {
		t.Errorf("speeds = %d/%d, want the sampler's 1.5 MB/s and 500 B/s", st.DownloadSpeed, st.UploadSpeed)
	}

	// The sampler reads the service's own counters.
	stats := e.svc.CollectStats()
	if len(stats) != 1 || stats[0].InfoHash != f.hash {
		t.Errorf("CollectStats = %+v", stats)
	}
}
