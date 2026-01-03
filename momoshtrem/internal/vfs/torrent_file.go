package vfs

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Ensure TorrentFile implements File interface
var _ File = (*TorrentFile)(nil)

// TorrentFile wraps a torrent file handle with VFS File interface.
// It provides lazy reader initialization, timeout handling, and activity tracking.
type TorrentFile struct {
	mu     sync.Mutex
	handle torrent.TorrentFileHandle
	reader torrent.TorrentReader

	name        string
	hash        string
	readTimeout time.Duration

	// Activity callback for idle mode tracking
	onActivity func(hash string)
}

// NewTorrentFile creates a new TorrentFile.
func NewTorrentFile(
	handle torrent.TorrentFileHandle,
	name string,
	hash string,
	readTimeout time.Duration,
	onActivity func(hash string),
) *TorrentFile {
	return &TorrentFile{
		handle:      handle,
		name:        name,
		hash:        hash,
		readTimeout: readTimeout,
		onActivity:  onActivity,
	}
}

// ensureReader lazily initializes the reader.
func (f *TorrentFile) ensureReader() {
	if f.reader != nil {
		return
	}
	f.reader = f.handle.NewReader()
	f.reader.SetResponsive() // Prioritize current position for streaming
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

// readAtLeast reads at least min bytes with timeout.
func (f *TorrentFile) readAtLeast(buf []byte, min int) (n int, err error) {
	if len(buf) < min {
		return 0, io.ErrShortBuffer
	}

	for n < min && err == nil {
		var nn int

		ctx, cancel := context.WithTimeout(context.Background(), f.readTimeout)
		nn, err = f.readContext(ctx, buf[n:])
		n += nn
		cancel()
	}

	if n >= min {
		err = nil
	} else if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}

	return
}

// readContext reads using a context for timeout/cancellation.
// If the reader supports ReadContext, use it; otherwise fall back to goroutine.
func (f *TorrentFile) readContext(ctx context.Context, p []byte) (int, error) {
	// Check if reader supports ReadContext
	if rc, ok := f.reader.(interface {
		ReadContext(context.Context, []byte) (int, error)
	}); ok {
		return rc.ReadContext(ctx, p)
	}

	// Fallback: read in goroutine with context cancellation
	type result struct {
		n   int
		err error
	}

	done := make(chan result, 1)
	go func() {
		n, err := f.reader.Read(p)
		done <- result{n, err}
	}()

	select {
	case r := <-done:
		return r.n, r.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
