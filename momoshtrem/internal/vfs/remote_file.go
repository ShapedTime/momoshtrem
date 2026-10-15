package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
)

var _ File = (*RemoteFile)(nil)

// RemoteOpener opens a remote file's bytes from off to its end. ctx bounds
// the opening only: the body must stay readable after ctx is done, until
// it's closed. Closing the body must end a Read blocked on it.
type RemoteOpener func(ctx context.Context, off int64) (io.ReadCloser, error)

const (
	// maxSkip is the largest forward gap a read discards from the open body
	// instead of opening it again at the new offset. Players read a little
	// ahead of where they last were (headers, indexes); a request costs
	// more than 256 KiB.
	maxSkip = 256 << 10
	// defaultRemoteReadTimeout bounds opening and each idle stretch of a
	// read when no timeout is configured.
	defaultRemoteReadTimeout = 30 * time.Second
)

// ErrReadTimeout means the remote body sent nothing for the read timeout.
var ErrReadTimeout = errors.New("remote read timed out")

// RemoteFile is a file streamed over the network (an HDRezka stream). It
// does no I/O until the first read, since WebDAV opens every file it
// lists. It keeps one body open at its position: a read there continues
// it, a short skip forward discards the gap, and anything else opens a new
// body at the offset. A read error midway reopens at the current position
// once. A read that receives nothing for the read timeout fails.
//
// WebDAV gives each request its own handle; a mutex guards it anyway.
type RemoteFile struct {
	name        string
	size        int64
	open        RemoteOpener
	readTimeout time.Duration

	mu      sync.Mutex
	body    io.ReadCloser
	pos     int64 // the body's position in the file
	readPos int64 // Read's position
	closed  bool
}

// NewRemoteFile creates a RemoteFile of size bytes that opens its bytes
// with open. readTimeout bounds each open and each idle stretch of a read
// (30 s when 0).
func NewRemoteFile(name string, size int64, open RemoteOpener, readTimeout time.Duration) *RemoteFile {
	if readTimeout <= 0 {
		readTimeout = defaultRemoteReadTimeout
	}
	return &RemoteFile{name: name, size: size, open: open, readTimeout: readTimeout}
}

func (f *RemoteFile) Name() string { return f.name }
func (f *RemoteFile) IsDir() bool  { return false }
func (f *RemoteFile) Size() int64  { return f.size }

func (f *RemoteFile) Stat() (os.FileInfo, error) {
	return common.NewFileInfo(f.name, f.size, false, time.Now()), nil
}

// Read reads from its own position, through ReadAt.
func (f *RemoteFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.readAt(p, f.readPos)
	f.readPos += int64(n)
	return n, err
}

// ReadAt reads len(p) bytes at off: it returns n == len(p), or an error.
// A read that reaches the end of the file returns the bytes up to it and
// io.EOF.
func (f *RemoteFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readAt(p, off)
}

// Close closes the open body, if any.
func (f *RemoteFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.closeBody()
	return nil
}

func (f *RemoteFile) readAt(p []byte, off int64) (int, error) {
	switch {
	case f.closed:
		return 0, os.ErrClosed
	case off < 0:
		return 0, fmt.Errorf("negative offset %d", off)
	case off >= f.size:
		return 0, io.EOF
	}
	want := p[:min(int64(len(p)), f.size-off)]

	n, retried := 0, false
	for n < len(want) {
		if err := f.seek(off + int64(n)); err != nil {
			return n, err
		}
		m, err := f.readBody(want[n:])
		n += m
		if err == nil {
			continue
		}
		// The body broke midway (a reset, a timeout): reopen where it
		// stopped, once.
		f.closeBody()
		if retried {
			return n, err
		}
		retried = true
		slog.Warn("Remote read failed; reopening", "file", f.name, "offset", off+int64(n), "error", err)
	}
	if len(want) < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// seek puts the body at off: the open body when it's there or a short skip
// ahead, otherwise a new one.
func (f *RemoteFile) seek(off int64) error {
	if f.body != nil {
		if gap := off - f.pos; gap > 0 && gap <= maxSkip {
			n, err := f.discard(gap)
			f.pos += n
			if err != nil {
				f.closeBody() // reopen at off instead
			}
		}
		if f.body != nil && f.pos == off {
			return nil
		}
		f.closeBody()
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.readTimeout)
	defer cancel()
	body, err := f.open(ctx, off)
	if err != nil {
		return err
	}
	f.body, f.pos = body, off
	return nil
}

// readBody fills p from the body, failing when it sends nothing for the
// read timeout.
func (f *RemoteFile) readBody(p []byte) (int, error) {
	n := 0
	err := f.watch(func(progress func()) error {
		for n < len(p) {
			m, err := f.body.Read(p[n:])
			n += m
			if m > 0 {
				progress()
			}
			switch {
			case err == io.EOF && n == len(p):
				return nil // the last bytes of the file
			case err == io.EOF:
				return io.ErrUnexpectedEOF // the body ended before the file
			case err != nil:
				return err
			}
		}
		return nil
	})
	f.pos += int64(n)
	return n, err
}

// discard skips n bytes of the body.
func (f *RemoteFile) discard(n int64) (int64, error) {
	var skipped int64
	err := f.watch(func(progress func()) error {
		buf := make([]byte, min(n, 32<<10))
		for skipped < n {
			m, err := f.body.Read(buf[:min(int64(len(buf)), n-skipped)])
			skipped += int64(m)
			if m > 0 {
				progress()
			}
			if err != nil {
				if err == io.EOF {
					return io.ErrUnexpectedEOF
				}
				return err
			}
		}
		return nil
	})
	return skipped, err
}

// watch runs read with an idle watchdog: when read makes no progress for
// the read timeout, the body is closed, which ends a blocked Read, and
// watch returns ErrReadTimeout.
func (f *RemoteFile) watch(read func(progress func()) error) error {
	body := f.body
	var fired atomic.Bool
	timer := time.AfterFunc(f.readTimeout, func() {
		fired.Store(true)
		body.Close()
	})
	err := read(func() { timer.Reset(f.readTimeout) })
	// Stop fails once the watchdog has started; a Reset after it fired
	// schedules it again, which fired tells apart.
	if stopped := timer.Stop(); stopped && !fired.Load() {
		return err
	}
	f.closeBody() // the watchdog closed it
	if err == nil {
		return nil // the read finished as the watchdog fired
	}
	return ErrReadTimeout
}

func (f *RemoteFile) closeBody() {
	if f.body != nil {
		f.body.Close()
		f.body = nil
	}
}
