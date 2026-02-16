package streaming

import "sync"

// ReaderTracker maintains a concurrent-safe registry of active PriorityReaders.
// It allows the debug endpoint to query active reader positions per torrent.
type ReaderTracker struct {
	mu      sync.RWMutex
	readers map[string][]*PriorityReader // infoHash -> active readers
}

// NewReaderTracker creates a new ReaderTracker.
func NewReaderTracker() *ReaderTracker {
	return &ReaderTracker{
		readers: make(map[string][]*PriorityReader),
	}
}

// Register adds a reader to tracking.
func (rt *ReaderTracker) Register(infoHash string, r *PriorityReader) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.readers[infoHash] = append(rt.readers[infoHash], r)
}

// Unregister removes a reader from tracking.
func (rt *ReaderTracker) Unregister(infoHash string, r *PriorityReader) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	readers := rt.readers[infoHash]
	for i, rr := range readers {
		if rr == r {
			rt.readers[infoHash] = append(readers[:i], readers[i+1:]...)
			break
		}
	}
	if len(rt.readers[infoHash]) == 0 {
		delete(rt.readers, infoHash)
	}
}

// GetReaders returns snapshots of active reader positions for a torrent.
func (rt *ReaderTracker) GetReaders(infoHash string) []ReaderSnapshot {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	readers := rt.readers[infoHash]
	snapshots := make([]ReaderSnapshot, 0, len(readers))
	for _, r := range readers {
		snapshots = append(snapshots, r.Snapshot())
	}
	return snapshots
}

// ReaderSnapshot contains a point-in-time snapshot of a reader's position.
type ReaderSnapshot struct {
	FilePath   string
	Position   int64
	FileLength int64
}
