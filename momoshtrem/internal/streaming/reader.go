package streaming

import (
	"io"
	"log/slog"
	"sync"

	"github.com/anacrolix/torrent"
)

// PriorityReader wraps a torrent file reader with intelligent piece prioritization.
// It performs async format detection and updates piece priorities based on seek position.
type PriorityReader struct {
	mu          sync.Mutex
	t           *torrent.Torrent
	file        *torrent.File
	reader      torrent.Reader
	prioritizer *Prioritizer
	cfg         Config

	// Position tracking
	pos int64

	// Format detection state (async)
	formatDetecting sync.Once
	formatInfo      *FormatInfo

	// Callbacks
	onActivity func()

	log *slog.Logger
}

// NewPriorityReader creates a priority-aware reader for a torrent file.
// It immediately sets up initial prioritization and configures the underlying reader.
func NewPriorityReader(
	t *torrent.Torrent,
	file *torrent.File,
	cfg Config,
	onActivity func(),
) *PriorityReader {
	reader := file.NewReader()
	reader.SetReadahead(cfg.ReadaheadBytes)
	reader.SetResponsive()

	prioritizer := NewPrioritizer(t, file, cfg)
	prioritizer.InitialPrioritize()

	pr := &PriorityReader{
		t:           t,
		file:        file,
		reader:      reader,
		prioritizer: prioritizer,
		cfg:         cfg,
		onActivity:  onActivity,
		log:         slog.With("component", "priority-reader", "file", file.Path()),
	}

	pr.log.Debug("priority reader created",
		"file_size", file.Length(),
		"readahead", cfg.ReadaheadBytes,
	)

	return pr
}

// Read implements io.Reader.
func (r *PriorityReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.markActivity()
	r.startFormatDetection()

	n, err = r.reader.Read(p)
	r.pos += int64(n)

	return n, err
}

// ReadAt implements io.ReaderAt for seeking reads.
// It updates piece priorities around the seek position.
func (r *PriorityReader) ReadAt(p []byte, off int64) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.markActivity()
	r.startFormatDetection()

	// Update priorities if position changed significantly
	if off != r.pos {
		r.prioritizer.UpdateForSeek(off)
		if _, err := r.reader.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
	}

	// Read fully (ReadAt semantics require reading exactly len(p) bytes)
	n, err = io.ReadFull(r.reader, p)
	r.pos = off + int64(n)

	return n, err
}

// Seek implements io.Seeker.
func (r *PriorityReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	newPos, err := r.reader.Seek(offset, whence)
	if err != nil {
		return 0, err
	}

	r.prioritizer.UpdateForSeek(newPos)
	r.pos = newPos

	r.log.Debug("seek completed", "position", newPos)

	return newPos, nil
}

// Close implements io.Closer.
func (r *PriorityReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.log.Debug("reader closed", "final_position", r.pos)

	return r.reader.Close()
}

// SetResponsive delegates to underlying reader.
// This is called by the VFS layer to ensure responsive mode is set.
func (r *PriorityReader) SetResponsive() {
	r.reader.SetResponsive()
}

// startFormatDetection triggers async format detection on first read.
// Format detection reads header bytes to identify MP4/MKV structure.
func (r *PriorityReader) startFormatDetection() {
	r.formatDetecting.Do(func() {
		go r.detectFormat()
	})
}

// detectFormat performs format detection in background.
// It creates a separate reader to avoid interfering with streaming.
func (r *PriorityReader) detectFormat() {
	// Create a separate reader for detection to avoid disturbing playback
	detectionReader := r.file.NewReader()
	defer detectionReader.Close()

	adapter := &seekingReaderAt{
		reader:   detectionReader,
		fileSize: r.file.Length(),
	}

	info := DetectFormat(adapter, r.file.Length(), r.file.Path())

	r.mu.Lock()
	r.formatInfo = info
	r.prioritizer.SetFormatInfo(info)
	r.mu.Unlock()

	r.log.Debug("format detected",
		"format", info.Format.String(),
		"moov_offset", info.MoovOffset,
		"moov_size", info.MoovSize,
		"header_size", info.HeaderSize,
		"needs_footer", info.NeedsFooter,
	)
}

// markActivity signals file access for idle tracking.
func (r *PriorityReader) markActivity() {
	if r.onActivity != nil {
		r.onActivity()
	}
}

// seekingReaderAt adapts a torrent.Reader to io.ReaderAt for format detection.
type seekingReaderAt struct {
	reader   torrent.Reader
	fileSize int64
}

func (s *seekingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= s.fileSize {
		return 0, io.EOF
	}

	if _, err := s.reader.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}

	return s.reader.Read(p)
}
