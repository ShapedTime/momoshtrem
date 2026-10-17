package scenario

import (
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"
)

// completions remembers every piece anacrolix reported complete, per
// torrent, from the moment the torrent loads: F11's signature needs a
// piece that was complete before its read broke (anacrolix resyncs the
// piece's completion when the read fails). It also remembers the pieces
// anacrolix reported partial: their chunks were written to the cache's
// incompleted/ (anacrolix publishes Partial after the write), so a chunk
// missing there later was evicted, not never written (F11's forward-play
// signature).
type completions struct {
	mu      sync.Mutex
	done    map[string]map[int]bool
	partial map[string]map[int]bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

// watchCompletions follows the titles' torrents until stopCompletions.
func (e *Env) watchCompletions() {
	c := &completions{done: map[string]map[int]bool{}, partial: map[string]map[int]bool{}, stop: make(chan struct{})}
	e.completions = c
	seen := map[string]bool{}
	for _, t := range e.Titles {
		if seen[t.Hash] {
			continue
		}
		seen[t.Hash] = true
		c.done[t.Hash] = map[int]bool{}
		c.partial[t.Hash] = map[int]bool{}
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
			if r.Complete {
				c.mark(hash, i)
			}
			if r.Partial {
				c.markPartial(hash, i)
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
			if v.Complete {
				c.mark(hash, v.Index)
			}
			if v.Partial {
				c.markPartial(hash, v.Index)
			}
		}
	}
}

func (c *completions) markPartial(hash string, i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partial[hash][i] = true
}

// wasPartial reports whether piece i of the torrent was ever partial.
func (c *completions) wasPartial(hash string, i int) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.partial[hash][i]
}

func (c *completions) mark(hash string, i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done[hash][i] = true
}

// was reports whether piece i of the torrent was ever complete.
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
