package streaming

import (
	"log/slog"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types"
)

// Prioritizer manages piece priorities for a torrent file to optimize streaming.
// It prioritizes header and footer bytes on file open, and adjusts priorities
// dynamically based on seek position during playback.
type Prioritizer struct {
	mu          sync.Mutex
	t           *torrent.Torrent
	file        *torrent.File
	cfg         Config
	formatInfo  *FormatInfo
	pieceLength int64

	// File's piece range (inclusive begin, exclusive end)
	beginPiece int
	endPiece   int

	// File's byte offset within torrent
	fileOffset int64
	fileLength int64

	// Tracking
	initialized bool

	// Debouncing: track last seek position to avoid redundant priority updates
	lastSeekOffset int64

	log *slog.Logger
}

// NewPrioritizer creates a prioritizer for a file within a torrent.
func NewPrioritizer(t *torrent.Torrent, file *torrent.File, cfg Config) *Prioritizer {
	info := t.Info()
	if info == nil {
		return nil
	}

	return &Prioritizer{
		t:           t,
		file:        file,
		cfg:         cfg,
		pieceLength: info.PieceLength,
		beginPiece:  file.BeginPieceIndex(),
		endPiece:    file.EndPieceIndex(),
		fileOffset:  file.Offset(),
		fileLength:  file.Length(),
		log:         slog.With("component", "prioritizer", "file", file.Path()),
	}
}

// InitialPrioritize sets header and footer pieces to HIGH priority.
// Should be called when file is first opened for streaming.
func (p *Prioritizer) InitialPrioritize() {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return
	}

	// Prioritize header
	headerEnd := min(p.cfg.HeaderPriorityBytes, p.fileLength)
	p.setPieceRangePriority(0, headerEnd, types.PiecePriorityHigh)

	// Prioritize footer (for MP4 moov-at-end cases)
	if p.fileLength > p.cfg.FooterPriorityBytes {
		footerStart := p.fileLength - p.cfg.FooterPriorityBytes
		p.setPieceRangePriority(footerStart, p.fileLength, types.PiecePriorityHigh)
	} else {
		// File is smaller than footer size, entire file is high priority
		p.setPieceRangePriority(0, p.fileLength, types.PiecePriorityHigh)
	}

	p.initialized = true
	p.log.Debug("initial prioritization complete",
		"header_bytes", headerEnd,
		"footer_bytes", min(p.cfg.FooterPriorityBytes, p.fileLength),
		"piece_range", []int{p.beginPiece, p.endPiece},
	)
}

// SetFormatInfo updates prioritization based on detected format.
// For example, if MP4 moov atom is at end of file, those pieces get HIGH priority.
func (p *Prioritizer) SetFormatInfo(info *FormatInfo) {
	if p == nil || info == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.formatInfo = info

	// For MP4 with moov at end, ensure those pieces are HIGH priority
	if info.Format == FormatMP4 && info.MoovOffset > 0 && info.MoovSize > 0 {
		p.setPieceRangePriority(info.MoovOffset, info.MoovOffset+info.MoovSize, types.PiecePriorityHigh)
		p.log.Debug("prioritized moov atom",
			"offset", info.MoovOffset,
			"size", info.MoovSize,
		)
	}
}

// UpdateForSeek updates priorities based on seek position.
// Sets pieces around current position to NOW priority, and ahead to READAHEAD.
// Debounces updates - skips if position changed by less than piece length.
func (p *Prioritizer) UpdateForSeek(offset int64) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Clamp offset to valid range
	if offset < 0 {
		offset = 0
	}
	if offset > p.fileLength {
		offset = p.fileLength
	}

	// Debounce: skip if position changed by less than piece length
	// This avoids excessive priority updates during normal sequential reading
	// and when the media player makes frequent position checks while paused
	if p.pieceLength > 0 {
		diff := offset - p.lastSeekOffset
		if diff < 0 {
			diff = -diff
		}
		if diff < p.pieceLength {
			return
		}
	}
	p.lastSeekOffset = offset

	// Urgent: immediate position + buffer
	urgentEnd := min(offset+p.cfg.UrgentBufferBytes, p.fileLength)
	p.setPieceRangePriority(offset, urgentEnd, types.PiecePriorityNow)

	// Readahead: next chunk after urgent buffer
	readaheadEnd := min(urgentEnd+p.cfg.ReadaheadBytes, p.fileLength)
	if readaheadEnd > urgentEnd {
		p.setPieceRangePriority(urgentEnd, readaheadEnd, types.PiecePriorityReadahead)
	}

	// Only log significant priority updates (not every call)
	p.log.Debug("updated seek priorities",
		"offset", offset,
		"urgent_end", urgentEnd,
		"readahead_end", readaheadEnd,
	)
}

// setPieceRangePriority sets priority for pieces covering a byte range within the file.
// startByte and endByte are relative to file start (not torrent start).
func (p *Prioritizer) setPieceRangePriority(startByte, endByte int64, priority types.PiecePriority) {
	if startByte >= endByte {
		return
	}

	// Convert file-relative offsets to torrent-absolute offsets
	absStart := p.fileOffset + startByte
	absEnd := p.fileOffset + endByte

	// Convert to piece indices
	startPiece := p.byteToPiece(absStart)
	endPiece := p.byteToPiece(absEnd-1) + 1 // +1 because we need inclusive end byte

	// Clamp to file's piece range
	if startPiece < p.beginPiece {
		startPiece = p.beginPiece
	}
	if endPiece > p.endPiece {
		endPiece = p.endPiece
	}

	// Set priority for each piece
	for i := startPiece; i < endPiece; i++ {
		piece := p.t.Piece(i)
		piece.SetPriority(priority)
	}
}

// byteToPiece converts absolute byte offset (torrent-relative) to piece index.
func (p *Prioritizer) byteToPiece(offset int64) int {
	if p.pieceLength == 0 {
		return 0
	}
	return int(offset / p.pieceLength)
}

// PieceLength returns the torrent's piece length in bytes.
func (p *Prioritizer) PieceLength() int64 {
	if p == nil {
		return 0
	}
	return p.pieceLength
}

// FilePieceRange returns the begin and end piece indices for the file.
func (p *Prioritizer) FilePieceRange() (begin, end int) {
	if p == nil {
		return 0, 0
	}
	return p.beginPiece, p.endPiece
}
