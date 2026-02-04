package vfs

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Ensure TorrentFile implements File interface
var _ File = (*TorrentFile)(nil)

// readResult holds the outcome of a read goroutine.
type readResult struct {
	n   int
	err error
}

// TorrentFile wraps a torrent file handle with VFS File interface.
// It provides lazy reader initialization, timeout handling, activity tracking,
// and intelligent piece prioritization for optimal streaming performance.
type TorrentFile struct {
	mu     sync.Mutex
	handle torrent.TorrentFileHandle
	reader *streaming.PriorityReader

	name        string
	hash        string
	readTimeout time.Duration

	// Streaming optimization config
	streamingCfg streaming.Config

	// Activity callback for idle mode tracking
	onActivity func(hash string)

	// Track if this is the first read (for retry logic)
	firstRead bool

	// pendingRead is non-nil when a timed-out read goroutine is still
	// running. We drain it before spawning a new goroutine, bounding
	// the leak to at most one goroutine per TorrentFile.
	pendingRead chan readResult
}

// NewTorrentFile creates a new TorrentFile.
func NewTorrentFile(
	handle torrent.TorrentFileHandle,
	name string,
	hash string,
	readTimeout time.Duration,
	onActivity func(hash string),
	streamingCfg streaming.Config,
) *TorrentFile {
	return &TorrentFile{
		handle:       handle,
		name:         name,
		hash:         hash,
		readTimeout:  readTimeout,
		onActivity:   onActivity,
		streamingCfg: streamingCfg,
		firstRead:    true,
	}
}

// ensureReader lazily initializes the reader with priority-aware streaming.
func (f *TorrentFile) ensureReader() {
	if f.reader != nil {
		return
	}

	// Create activity callback that includes the hash
	onActivity := func() {
		f.markActivity()
	}

	// Create priority-aware reader for optimized streaming
	f.reader = streaming.NewPriorityReader(
		f.handle.Torrent(),
		f.handle.File(),
		f.streamingCfg,
		onActivity,
	)
}

// markActivity notifies the activity manager that this torrent is being accessed.
func (f *TorrentFile) markActivity() {
	if f.onActivity != nil && f.hash != "" {
		f.onActivity(f.hash)
	}
}

// Name returns the file name.
func (f *TorrentFile) Name() string {
	return f.name
}

// IsDir returns false (torrent files are never directories).
func (f *TorrentFile) IsDir() bool {
	return false
}

// Size returns the file size in bytes.
func (f *TorrentFile) Size() int64 {
	return f.handle.Length()
}

// Stat returns file info.
func (f *TorrentFile) Stat() (os.FileInfo, error) {
	return common.NewFileInfo(f.name, f.handle.Length(), false, time.Now()), nil
}

// Read reads up to len(p) bytes into p with timeout.
func (f *TorrentFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ensureReader()
	f.markActivity()

	// Use retry logic for first read to handle start_paused race condition
	if f.firstRead {
		f.firstRead = false
		return f.firstReadWithRetry(p)
	}

	return f.readWithTimeout(p)
}

// ReadAt reads len(p) bytes at offset off with timeout.
func (f *TorrentFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ensureReader()
	f.markActivity()

	// Seek to offset
	if _, err := f.reader.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}

	return f.readAtLeast(p, len(p))
}

// Close closes the reader.
func (f *TorrentFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.reader != nil {
		err := f.reader.Close()
		f.reader = nil
		return err
	}
	return nil
}

// readWithTimeout reads with context timeout.
func (f *TorrentFile) readWithTimeout(p []byte) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.readTimeout)
	defer cancel()

	return f.readContext(ctx, p)
}

// firstReadWithRetry performs the first read with retry logic.
// This handles the race condition where the torrent may start paused
// (start_paused: true) and needs time to wake up via the activity callback.
func (f *TorrentFile) firstReadWithRetry(p []byte) (int, error) {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		n, err := f.readWithTimeout(p)
		if err == nil || err == io.EOF {
			return n, err
		}
		lastErr = err

		// Exponential backoff: 100ms, 200ms, 300ms
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}

	return 0, lastErr
}

// readAtLeast reads at least min bytes with timeout.
// Uses a single context for the entire operation to avoid overhead of
// creating new contexts per iteration.
func (f *TorrentFile) readAtLeast(buf []byte, min int) (n int, err error) {
	if len(buf) < min {
		return 0, io.ErrShortBuffer
	}

	// Create a single context for the entire read operation
	ctx, cancel := context.WithTimeout(context.Background(), f.readTimeout)
	defer cancel()

	for n < min && err == nil {
		var nn int
		nn, err = f.readContext(ctx, buf[n:])
		n += nn
	}

	if n >= min {
		err = nil
	} else if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}

	return
}

// readContext reads using a context for timeout/cancellation.
//
// A private buffer is used instead of the caller's slice to prevent a data
// race: on timeout the goroutine may still be writing to the buffer while the
// caller has already returned or reused it (e.g. firstReadWithRetry retries
// with the same slice).
//
// Before spawning a new goroutine we drain any outstanding one from a previous
// timeout.  PriorityReader.mu serialises reads anyway, so waiting here does
// not change the observable concurrency.  This bounds the goroutine leak to at
// most one per TorrentFile instead of growing without limit.
func (f *TorrentFile) readContext(ctx context.Context, p []byte) (int, error) {
	// Drain a pending goroutine from a previous timeout.
	if f.pendingRead != nil {
		select {
		case <-f.pendingRead:
			f.pendingRead = nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	buf := make([]byte, len(p))
	done := make(chan readResult, 1)

	go func() {
		n, err := f.reader.Read(buf)
		done <- readResult{n, err}
	}()

	select {
	case r := <-done:
		copy(p[:r.n], buf[:r.n])
		return r.n, r.err
	case <-ctx.Done():
		f.pendingRead = done
		return 0, ctx.Err()
	}
}
