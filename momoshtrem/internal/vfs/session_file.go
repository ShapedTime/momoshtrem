package vfs

import (
	"context"
	"sync"

	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/streaming"
)

// SessionTracker opens a streaming session handle for a video a WebDAV GET
// opened: nil for anything else. *sessions.Registry implements it.
type SessionTracker interface {
	Open(ctx context.Context, f sessions.File) *sessions.Handle
}

var _ SessionTracker = (*sessions.Registry)(nil)

// SetSessions reports the reads of every video to tracker, so Activity
// knows who is streaming what.
func (fs *LibraryFS) SetSessions(tracker SessionTracker) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.sessions = tracker
}

// trackSession wraps a video's file so its reads reach its session: only
// when a GET opened it (the tracker hands out no handle otherwise).
// Called with fs.mu held.
func (fs *LibraryFS) trackSession(ctx context.Context, path string, pf *PlaceholderFile, f File) File {
	if fs.sessions == nil {
		return f
	}
	a := pf.assignment
	desc := sessions.File{Path: path, Name: pf.name, Size: f.Size(), Source: sessions.SourceTorrent}
	if a.ItemID != 0 {
		desc.Item = &sessions.ItemRef{Type: a.ItemType, ID: a.ItemID}
	}
	if a.IsTorrent() {
		desc.InfoHash, desc.TorrentPath = a.InfoHash, a.FilePath
		if tf, ok := f.(*TorrentFile); ok {
			desc.BufferedAhead = bufferedAhead(tf)
		}
	} else {
		desc.Source = sessions.SourceHDRezka
	}
	h := fs.sessions.Open(ctx, desc)
	if h == nil {
		return f
	}
	return &sessionFile{File: f, h: h}
}

// bufferedAhead is how much of a torrent file is complete from an offset.
func bufferedAhead(tf *TorrentFile) func(off int64) int64 {
	h := tf.handle
	return func(off int64) int64 {
		t, file := h.Torrent(), h.File()
		if t == nil || file == nil {
			return 0
		}
		return streaming.BufferedAhead(t, file, off)
	}
}

// sessionFile reports a file's reads to its streaming session.
type sessionFile struct {
	File
	h *sessions.Handle

	mu  sync.Mutex
	pos int64 // Read's offset
}

func (f *sessionFile) ReadAt(p []byte, off int64) (int, error) {
	f.h.Begin()
	n, err := f.File.ReadAt(p, off)
	f.h.End(off, n)
	return n, err
}

func (f *sessionFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.h.Begin()
	n, err := f.File.Read(p)
	f.h.End(f.pos, n)
	f.pos += int64(n)
	return n, err
}

func (f *sessionFile) Close() error {
	f.h.Close()
	return f.File.Close()
}
