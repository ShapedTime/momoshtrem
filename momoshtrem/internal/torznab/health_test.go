package torznab

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealth_StreaksAndSince(t *testing.T) {
	j := newFakeJackett(t)
	var mode atomic.Value // "ok", "error" or "slow"
	okBody, errBody := fixture(t, "search_thepiratebay.xml"), fixture(t, "error_indexer.xml")
	j.handle("thepiratebay", func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case "error":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(errBody)
		case "slow":
			<-r.Context().Done()
		default:
			_, _ = w.Write(okBody)
		}
	})
	clock := newFakeClock()
	c := New(j.URL, testKey, WithClock(clock.Now), WithTimeouts(30*time.Millisecond, 0))
	ctx := context.Background()

	if c.Snapshot() != nil {
		t.Fatal("Snapshot before the first list fetch should be nil")
	}
	if _, err := c.Indexers(ctx); err != nil {
		t.Fatal(err)
	}
	for _, h := range c.Snapshot() {
		if h.Status != StatusUnknown || h.MS != nil || h.Since != nil || h.Error != "" {
			t.Fatalf("before any search: %+v, want unknown", h)
		}
	}

	step := func(name, m string, wantStatus Status, wantSince *time.Time) {
		t.Helper()
		mode.Store(m)
		_, _ = c.Search(ctx, "thepiratebay", Query{Q: "Frieren"})
		h := snapshotOf(t, c, "thepiratebay")
		if h.Status != wantStatus || h.MS == nil {
			t.Fatalf("%s: health = %+v, want %s with ms", name, h, wantStatus)
		}
		switch {
		case wantSince == nil && h.Since != nil:
			t.Fatalf("%s: since = %v, want nil", name, *h.Since)
		case wantSince != nil && (h.Since == nil || !h.Since.Equal(*wantSince)):
			t.Fatalf("%s: since = %v, want %v", name, h.Since, *wantSince)
		}
		if (wantStatus == StatusOK) != (h.Error == "") {
			t.Fatalf("%s: error = %q", name, h.Error)
		}
		clock.Advance(time.Hour)
	}

	step("ok", "ok", StatusOK, nil)
	firstFailure := clock.Now()
	step("an error starts a streak", "error", StatusError, &firstFailure)
	step("a timeout continues it", "slow", StatusTimeout, &firstFailure)
	step("another error still continues it", "error", StatusError, &firstFailure)
	step("ok ends it", "ok", StatusOK, nil)
	secondFailure := clock.Now()
	step("a new failure starts a new streak", "slow", StatusTimeout, &secondFailure)

	if h := snapshotOf(t, c, "eztv"); h.Status != StatusUnknown || h.Name != "EZTV" {
		t.Errorf("an indexer never searched = %+v, want unknown", h)
	}
}
