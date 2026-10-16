package torznab

import (
	"context"
	"errors"
	"sync"
)

const (
	defaultPerIndexer = 2
	defaultTotal      = 6
)

// Request is one search of a fan-out.
type Request struct {
	IndexerID string
	Query     Query
}

// Response is what one request of a fan-out returned. Err is
// ErrUnsupported when the indexer couldn't run the query and no request
// was made.
type Response struct {
	Request
	Results []Result
	Err     error
}

// Progress is one indexer's state during a fan-out.
type Progress struct {
	IndexerID   string
	IndexerName string
	// Status is pending until every request to the indexer has finished;
	// then ok, or the status of its first failure. Skipped requests
	// (ErrUnsupported) count as answered.
	Status    Status
	ElapsedMS int64  // since the fan-out started
	Count     int    // results so far, over all its requests
	Error     string // the first failure's reason; safe to show
	// RequestIndex is the index in reqs of the request that just finished,
	// and Results what it returned; -1 and nil in the first report.
	RequestIndex int
	Results      []Result
}

// FanoutOptions caps a fan-out's concurrency and reports its progress.
type FanoutOptions struct {
	PerIndexer int // concurrent requests per indexer; default 2
	Total      int // concurrent requests overall; default 6
	// OnProgress, when set, gets a pending report for each indexer first,
	// then one each time a request finishes. Calls are serialised.
	OnProgress func(Progress)
}

// Fanout runs reqs concurrently, each with the request timeout, and returns
// their responses in order once all have finished. Cancelling ctx stops
// requests that are waiting for a slot and aborts running ones.
func (c *Client) Fanout(ctx context.Context, reqs []Request, opts FanoutOptions) []Response {
	if opts.PerIndexer <= 0 {
		opts.PerIndexer = defaultPerIndexer
	}
	if opts.Total <= 0 {
		opts.Total = defaultTotal
	}
	start := c.now()

	names := map[string]string{}
	if list, err := c.Indexers(ctx); err == nil {
		for _, ix := range list {
			names[ix.ID] = ix.Title
		}
	}

	// One aggregate and one slot pool per indexer, in first-request order.
	type aggregate struct {
		progress  Progress
		remaining int
		failure   Status // the first failure's status; "" while none
	}
	var order []string
	aggs := map[string]*aggregate{}
	slots := map[string]chan struct{}{}
	for _, r := range reqs {
		a, ok := aggs[r.IndexerID]
		if !ok {
			name := names[r.IndexerID]
			if name == "" {
				name = r.IndexerID
			}
			a = &aggregate{progress: Progress{IndexerID: r.IndexerID, IndexerName: name, Status: StatusPending, RequestIndex: -1}}
			aggs[r.IndexerID] = a
			slots[r.IndexerID] = make(chan struct{}, opts.PerIndexer)
			order = append(order, r.IndexerID)
		}
		a.remaining++
	}

	report := func(p Progress) {
		if opts.OnProgress != nil {
			opts.OnProgress(p)
		}
	}
	for _, id := range order {
		report(aggs[id].progress)
	}

	var mu sync.Mutex // guards aggs and serialises report
	total := make(chan struct{}, opts.Total)
	responses := make([]Response, len(reqs))
	var wg sync.WaitGroup
	for i, r := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := c.runSlotted(ctx, slots[r.IndexerID], total, r)
			responses[i] = Response{Request: r, Results: results, Err: err}

			mu.Lock()
			defer mu.Unlock()
			a := aggs[r.IndexerID]
			a.remaining--
			a.progress.Count += len(results)
			if err != nil && !errors.Is(err, ErrUnsupported) && a.failure == "" {
				a.failure = outcomeOf(err)
				a.progress.Error = c.SafeError(err)
			}
			switch {
			case a.remaining > 0:
				a.progress.Status = StatusPending
			case a.failure != "":
				a.progress.Status = a.failure
			default:
				a.progress.Status = StatusOK
			}
			a.progress.ElapsedMS = c.now().Sub(start).Milliseconds()
			p := a.progress
			p.RequestIndex, p.Results = i, results
			report(p)
		}()
	}
	wg.Wait()
	return responses
}

// runSlotted waits for a slot on the indexer and one overall, then searches.
func (c *Client) runSlotted(ctx context.Context, perIndexer, total chan struct{}, r Request) ([]Result, error) {
	select {
	case perIndexer <- struct{}{}:
		defer func() { <-perIndexer }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case total <- struct{}{}:
		defer func() { <-total }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return c.Search(ctx, r.IndexerID, r.Query)
}
