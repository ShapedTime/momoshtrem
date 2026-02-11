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

	// Previous priority ranges for downgrading stale pieces
	lastUrgentStart  int64
	lastUrgentEnd    int64
	lastReadaheadEnd int64

	// Metrics callbacks (nil-safe)
	onSeek      func(forward bool) // called on each non-debounced seek
	onDowngrade func(count int)    // called with number of pieces downgraded

	log *slog.Logger
}

// NewPrioritizer creates a prioritizer for a file within a torrent.
func NewPrioritizer(t *torrent.Torrent, file *torrent.File, cfg Config) *Prioritizer {
	info := t.Info()
	if info == nil {
		return nil
	}

	return &Prioritizer{
		t:              t,
		file:           file,
		cfg:            cfg,
		pieceLength:    info.PieceLength,
		beginPiece:     file.BeginPieceIndex(),
		endPiece:       file.EndPieceIndex(),
		fileOffset:     file.Offset(),
		fileLength:     file.Length(),
		lastSeekOffset: -1, // sentinel: never seeked
		log:            slog.With("component", "prioritizer", "file", file.Path()),
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
// Downgrades pieces from previous ranges that are now behind the cursor.
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
	// and when the media player makes frequent position checks while paused.
	// Skip debounce when lastSeekOffset < 0 (sentinel: never seeked).
	if p.pieceLength > 0 && p.lastSeekOffset >= 0 {
		diff := offset - p.lastSeekOffset
		if diff < 0 {
			diff = -diff
		}
		if diff < p.pieceLength {
			return
		}
	}

	forward := offset >= p.lastSeekOffset
	p.lastSeekOffset = offset

	if p.onSeek != nil {
		p.onSeek(forward)
	}

	// Calculate new ranges
	urgentEnd := min(offset+p.cfg.UrgentBufferBytes, p.fileLength)
	readaheadEnd := min(urgentEnd+p.cfg.ReadaheadBytes, p.fileLength)

	// Downgrade pieces from previous ranges that are now behind the cursor.
	// Only downgrade pieces strictly before the new urgent start — the player
	// has already consumed them and won't seek back (and if it does, they
	// get re-prioritized immediately).
	if p.lastReadaheadEnd > 0 {
		downgradeEnd := min(p.lastReadaheadEnd, offset)
		if p.lastUrgentStart < downgradeEnd {
			downgraded := p.setPieceRangePriorityCount(p.lastUrgentStart, downgradeEnd, types.PiecePriorityNormal)
			if downgraded > 0 && p.onDowngrade != nil {
				p.onDowngrade(downgraded)
			}
		}
	}

	// Downgrade pieces from old range that are now beyond new readahead window.
	// This handles backward seeks and forward seeks near end of file that shrink the window.
	if p.lastReadaheadEnd > readaheadEnd {
		downgraded := p.setPieceRangePriorityCount(readaheadEnd, p.lastReadaheadEnd, types.PiecePriorityNormal)
		if downgraded > 0 && p.onDowngrade != nil {
			p.onDowngrade(downgraded)
		}
	}

	// Urgent: immediate position + buffer
	p.setPieceRangePriority(offset, urgentEnd, types.PiecePriorityNow)

	// Readahead: next chunk after urgent buffer
	if readaheadEnd > urgentEnd {
		p.setPieceRangePriority(urgentEnd, readaheadEnd, types.PiecePriorityReadahead)
	}

	// Track for next downgrade
	p.lastUrgentStart = offset
	p.lastUrgentEnd = urgentEnd
	p.lastReadaheadEnd = readaheadEnd

	p.log.Debug("updated seek priorities",
		"offset", offset,
		"urgent_end", urgentEnd,
		"readahead_end", readaheadEnd,
	)
}

// setPieceRangePriority sets priority for pieces covering a byte range within the file.
// startByte and endByte are relative to file start (not torrent start).
func (p *Prioritizer) setPieceRangePriority(startByte, endByte int64, priority types.PiecePriority) {
	p.setPieceRangePriorityCount(startByte, endByte, priority)
}

// setPieceRangePriorityCount sets priority for pieces covering a byte range
// and returns the number of pieces updated.
func (p *Prioritizer) setPieceRangePriorityCount(startByte, endByte int64, priority types.PiecePriority) int {
	if startByte >= endByte {
		return 0
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
	count := 0
	for i := startPiece; i < endPiece; i++ {
		piece := p.t.Piece(i)
		piece.SetPriority(priority)
		count++
	}
	return count
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
