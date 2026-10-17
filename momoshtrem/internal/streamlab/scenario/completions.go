package scenario

import (
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"
)

// completions remembers every piece anacrolix had data for, per torrent,
// from the moment the torrent loads: complete, or partial (dirty chunks,
// which the PriorityReader reads before the piece is verified: it's
// responsive). F11's signature needs a piece the leecher had downloaded
// before its read broke, since anacrolix resyncs the piece's completion
// when the read fails; a piece never downloaded isn't evicted.
type completions struct {
	mu   sync.Mutex
	done map[string]map[int]bool
	stop chan struct{}
	wg   sync.WaitGroup
}

// watchCompletions follows the titles' torrents until stopCompletions.
func (e *Env) watchCompletions() {
	c := &completions{done: map[string]map[int]bool{}, stop: make(chan struct{})}
	e.completions = c
	seen := map[string]bool{}
	for _, t := range e.Titles {
		if seen[t.Hash] {
			continue
		}
		seen[t.Hash] = true
		c.done[t.Hash] = map[int]bool{}
		c.wg.Add(1)
		go func(hash string) {
			defer c.wg.Done()
			c.follow(e, hash)
		}(t.Hash)
	}
}

// follow waits for the torrent to load, then records its completions.
func (c *completions) follow(e *Env, hash string) {
	var tor *atorrent.Torrent
	for {
		if t, ok := e.Stack.Torrent(hash); ok && t.Info() != nil {
			tor = t
			break
		}
		select {
		case <-c.stop:
			return
		case <-time.After(5 * time.Millisecond):
		}
	}
	sub := tor.SubscribePieceStateChanges()
	defer sub.Close()
	// What completed before the subscription.
	i := 0
	for _, r := range tor.PieceStateRuns() {
		for range r.Length {
			if r.Complete || r.Partial {
				c.mark(hash, i)
			}
			i++
		}
	}
	for {
		select {
		case <-c.stop:
			return
		case v, ok := <-sub.Values:
			if !ok {
				return
			}
			if v.Complete || v.Partial {
				c.mark(hash, v.Index)
			}
		}
	}
}

func (c *completions) mark(hash string, i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done[hash][i] = true
}

// was reports whether the leecher ever had data of piece i: complete or
// partial.
func (c *completions) was(hash string, i int) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.done[hash][i]
}

func (c *completions) stopWatching() {
	if c == nil {
		return
	}
	close(c.stop)
	c.wg.Wait()
}
