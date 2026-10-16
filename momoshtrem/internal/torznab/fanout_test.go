package torznab

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// concurrency counts requests in flight, overall and per indexer, and keeps
// the highest counts seen.
type concurrency struct {
	mu        sync.Mutex
	now, peak map[string]int
}

func newConcurrency() *concurrency {
	return &concurrency{now: map[string]int{}, peak: map[string]int{}}
}

func (c *concurrency) enter(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range []string{id, "*"} {
		c.now[k]++
		c.peak[k] = max(c.peak[k], c.now[k])
	}
}

func (c *concurrency) leave(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now[id]--
	c.now["*"]--
}

func TestFanout(t *testing.T) {
	j := newFakeJackett(t)
	conc := newConcurrency()
	slowly := func(id string, h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			conc.enter(id)
			defer conc.leave(id)
			time.Sleep(30 * time.Millisecond)
			h(w, r)
		}
	}
	errBody := fixture(t, "error_indexer.xml")
	j.handle("thepiratebay", slowly("thepiratebay", serveFixture(t, "search_thepiratebay.xml")))
	j.handle("therarbg", slowly("therarbg", serveFixture(t, "movie_therarbg.xml")))
	j.handle("1337x", slowly("1337x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(errBody)
	}))
	c := New(j.URL, testKey)

	reqs := []Request{
		{"thepiratebay", Query{Q: "Interstellar 2014"}},
		{"therarbg", Query{Mode: ModeMovie, IMDbID: "tt0816692"}},
		{"thepiratebay", Query{Q: "Interstellar"}},
		{"1337x", Query{Q: "Interstellar 2014"}},
		{"eztv", Query{Mode: ModeMovie, Q: "Interstellar 2014"}}, // EZTV has no movies: skipped
		{"therarbg", Query{Q: "Interstellar 2014"}},
		{"thepiratebay", Query{Q: "Interstellar (2014)"}},
	}
	var reports []Progress // appended without a lock: calls must be serialised
	responses := c.Fanout(context.Background(), reqs, FanoutOptions{
		PerIndexer: 1, Total: 2,
		OnProgress: func(p Progress) { reports = append(reports, p) },
	})

	if len(responses) != len(reqs) {
		t.Fatalf("%d responses for %d requests", len(responses), len(reqs))
	}
	for i, r := range responses {
		if r.IndexerID != reqs[i].IndexerID {
			t.Errorf("response %d is for %s, want %s", i, r.IndexerID, reqs[i].IndexerID)
		}
	}
	if len(responses[0].Results) != 8 || responses[0].Err != nil {
		t.Errorf("response 0 = %d results, %v", len(responses[0].Results), responses[0].Err)
	}
	if !errors.Is(responses[4].Err, ErrUnsupported) {
		t.Errorf("eztv err = %v, want ErrUnsupported", responses[4].Err)
	}
	if conc.peak["*"] > 2 || conc.peak["thepiratebay"] > 1 || conc.peak["therarbg"] > 1 {
		t.Errorf("peak concurrency = %v, want at most 2 overall and 1 per indexer", conc.peak)
	}

	// First a pending report per indexer, in first-request order.
	order := []string{"thepiratebay", "therarbg", "1337x", "eztv"}
	for i, id := range order {
		p := reports[i]
		if p.IndexerID != id || p.Status != StatusPending || p.RequestIndex != -1 || p.Results != nil {
			t.Fatalf("report %d = %+v, want pending for %s", i, p, id)
		}
	}
	if reports[0].IndexerName != "The Pirate Bay" {
		t.Errorf("name = %q", reports[0].IndexerName)
	}
	// Then one per finished request.
	if len(reports) != len(order)+len(reqs) {
		t.Fatalf("%d reports, want %d", len(reports), len(order)+len(reqs))
	}
	last := map[string]Progress{}
	seen := map[int]bool{}
	for _, p := range reports[len(order):] {
		if seen[p.RequestIndex] {
			t.Errorf("request %d reported twice", p.RequestIndex)
		}
		seen[p.RequestIndex] = true
		if prev, ok := last[p.IndexerID]; ok && prev.Status != StatusPending {
			t.Errorf("%s reported after its final status: %+v", p.IndexerID, p)
		}
		last[p.IndexerID] = p
	}
	want := map[string]struct {
		status Status
		count  int
	}{
		"thepiratebay": {StatusOK, 24},
		"therarbg":     {StatusOK, 12},
		"1337x":        {StatusError, 0},
		"eztv":         {StatusOK, 0},
	}
	for id, w := range want {
		p := last[id]
		if p.Status != w.status || p.Count != w.count {
			t.Errorf("%s final = %s, %d results; want %s, %d", id, p.Status, p.Count, w.status, w.count)
		}
	}
	if last["1337x"].Error != "jackett error: Challenge detected but FlareSolverr is not configured" {
		t.Errorf("1337x error = %q", last["1337x"].Error)
	}
	if last["thepiratebay"].ElapsedMS < 60 {
		t.Errorf("thepiratebay elapsed = %d ms; three serial 30 ms requests take longer", last["thepiratebay"].ElapsedMS)
	}
}

func TestFanout_TimeoutAndPendingWhileRunning(t *testing.T) {
	j := newFakeJackett(t)
	j.handle("thepiratebay", serveFixture(t, "search_thepiratebay.xml"))
	j.handle("therarbg", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c := New(j.URL, testKey, WithTimeouts(50*time.Millisecond, 0))

	var rarbg []Progress // therarbg's reports after a request finished
	final := map[string]Progress{}
	c.Fanout(context.Background(), []Request{
		{"therarbg", Query{Q: "a"}},
		{"therarbg", Query{Q: "b"}},
		{"thepiratebay", Query{Q: "c"}},
	}, FanoutOptions{OnProgress: func(p Progress) {
		if p.IndexerID == "therarbg" && p.RequestIndex >= 0 {
			rarbg = append(rarbg, p)
		}
		final[p.IndexerID] = p
	}})

	if len(rarbg) != 2 || rarbg[0].Status != StatusPending || rarbg[1].Status != StatusTimeout {
		t.Fatalf("therarbg reports = %+v, want pending then timeout", rarbg)
	}
	if p := rarbg[1]; p.Error != "timed out after 50ms" || p.ElapsedMS < 50 {
		t.Errorf("therarbg = %+v", p)
	}
	if p := final["thepiratebay"]; p.Status != StatusOK || p.Count != 8 {
		t.Errorf("thepiratebay = %+v", p)
	}
}

func TestFanout_Cancel(t *testing.T) {
	j := newFakeJackett(t)
	var started atomic.Int32
	j.handle("thepiratebay", func(w http.ResponseWriter, r *http.Request) {
		started.Add(1)
		<-r.Context().Done()
	})
	c := New(j.URL, testKey)
	if _, err := c.Indexers(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	start := time.Now()
	responses := c.Fanout(ctx, []Request{
		{"thepiratebay", Query{Q: "a"}},
		{"thepiratebay", Query{Q: "b"}},
		{"thepiratebay", Query{Q: "c"}}, // waits for a slot, then gives up
	}, FanoutOptions{PerIndexer: 2})

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Fanout took %s after cancel", elapsed)
	}
	for i, r := range responses {
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("response %d err = %v, want context.Canceled", i, r.Err)
		}
	}
	if n := started.Load(); n != 2 {
		t.Errorf("%d requests started, want 2 (the per-indexer cap)", n)
	}
	if h := snapshotOf(t, c, "thepiratebay"); h.Status != StatusUnknown {
		t.Errorf("cancelled searches changed health: %+v", h)
	}
}
