package torrent

import (
	"encoding/base64"
	"log/slog"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

// Ensure service implements Service interface
var _ Service = (*service)(nil)

// service implements the Service interface using anacrolix/torrent.
type service struct {
	mu     sync.RWMutex
	client *torrent.Client
	am     *ActivityManager

	// Loaded torrents by info hash (lowercase hex)
	torrents map[string]*torrent.Torrent

	// File index cache: infoHash -> filePath -> *torrent.File
	// Built after metadata arrives, enables O(1) file lookup.
	fileIndexes map[string]map[string]*torrent.File

	// Configuration
	addTimeout  time.Duration
	readTimeout time.Duration

	log *slog.Logger
}

// NewService creates a new torrent service.
func NewService(
	client *torrent.Client,
	am *ActivityManager,
	addTimeout, readTimeout time.Duration,
) Service {
	return &service{
		client:      client,
		am:          am,
		torrents:    make(map[string]*torrent.Torrent),
		fileIndexes: make(map[string]map[string]*torrent.File),
		addTimeout:  addTimeout,
		readTimeout: readTimeout,
		log:         slog.With("component", "torrent-service"),
	}
}

// AddTorrent adds a torrent by magnet URI and waits for metadata.
func (s *service) AddTorrent(magnetURI string) (*TorrentInfo, error) {
	// Parse magnet URI
	spec, err := metainfo.ParseMagnetUri(magnetURI)
	if err != nil {
		s.log.Warn("invalid magnet URI", "error", err)
		return nil, ErrInvalidMagnet
	}

	hash := spec.InfoHash.HexString()

	// Check if already loaded
	s.mu.RLock()
	existing, exists := s.torrents[hash]
	s.mu.RUnlock()

	if exists {
		s.log.Debug("torrent already loaded", "hash", hash)
		return s.torrentToInfo(existing), nil
	}

	// Add to client
	t, err := s.client.AddMagnet(magnetURI)
	if err != nil {
		s.log.Error("failed to add magnet", "hash", hash, "error", err)
		return nil, err
	}

	s.log.Info("waiting for torrent metadata", "hash", hash)

	// Wait for metadata with strict timeout
	select {
	case <-time.After(s.addTimeout):
		s.log.Warn("timeout waiting for torrent metadata", "hash", hash)
		t.Drop() // Clean up failed torrent
		return nil, ErrMetadataTimeout
	case <-t.GotInfo():
		s.log.Info("obtained torrent metadata",
			"hash", hash,
			"name", t.Info().Name,
			"files", len(t.Files()),
		)
	}

	// Suppress all piece downloads until a file is opened for streaming.
	// Each file's Prioritizer will raise its own pieces on open.
	for i := 0; i < t.NumPieces(); i++ {
		t.Piece(i).SetPriority(types.PiecePriorityNone)
	}

	// Build file index for O(1) lookup
	idx := make(map[string]*torrent.File, len(t.Files()))
	for _, f := range t.Files() {
		idx[f.Path()] = f
	}

	// Register with activity manager for idle tracking
	if s.am != nil {
		s.am.Register(hash, t)
	}

	// Store in map
	s.mu.Lock()
	s.torrents[hash] = t
	s.fileIndexes[hash] = idx
	s.mu.Unlock()

	return s.torrentToInfo(t), nil
}

// GetTorrent returns information about an already-added torrent.
func (s *service) GetTorrent(infoHash string) (*TorrentInfo, error) {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTorrentNotFound
	}

	return s.torrentToInfo(t), nil
}

// GetOrAddTorrent returns an existing torrent or adds it if not present.
func (s *service) GetOrAddTorrent(magnetURI string) (*TorrentInfo, error) {
	hash := ExtractInfoHash(magnetURI)
	if hash == "" {
		return nil, ErrInvalidMagnet
	}

	// Check if already exists
	s.mu.RLock()
	existing, exists := s.torrents[hash]
	s.mu.RUnlock()

	if exists {
		return s.torrentToInfo(existing), nil
	}

	// Add it
	return s.AddTorrent(magnetURI)
}

// GetFile returns a file handle for streaming a specific file from a torrent.
func (s *service) GetFile(infoHash string, filePath string) (TorrentFileHandle, error) {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	idx := s.fileIndexes[infoHash]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTorrentNotFound
	}

	// Use cached index for O(1) lookup
	if idx != nil {
		f, ok := idx[filePath]
		if !ok {
			return nil, ErrFileNotFound
		}
		// Find the next file in torrent order for pre-fetching
		var next *torrent.File
		files := t.Files()
		for i, tf := range files {
			if tf.Path() == filePath && i+1 < len(files) {
				next = files[i+1]
				break
			}
		}
		return &fileHandle{file: f, nextFile: next}, nil
	}

	// Fallback: linear scan (should not happen with proper init)
	var found *torrent.File
	var next *torrent.File
	files := t.Files()
	for i, f := range files {
		if f.Path() == filePath {
			found = f
			if i+1 < len(files) {
				next = files[i+1]
			}
			break
		}
	}
	if found == nil {
		return nil, ErrFileNotFound
	}
	return &fileHandle{file: found, nextFile: next}, nil
}

// RemoveTorrent removes a torrent from the client.
func (s *service) RemoveTorrent(infoHash string, deleteData bool) error {
	s.mu.Lock()
	t, exists := s.torrents[infoHash]
	if !exists {
		s.mu.Unlock()
		return ErrTorrentNotFound
	}
	delete(s.torrents, infoHash)
	delete(s.fileIndexes, infoHash)
	s.mu.Unlock()

	// Unregister from activity manager
	if s.am != nil {
		s.am.Unregister(infoHash)
	}

	// Drop from client (stops downloading/uploading)
	t.Drop()

	s.log.Info("removed torrent", "hash", infoHash, "delete_data", deleteData)

	// Note: deleteData is currently ignored as we use a shared cache.
	// The filecache handles cleanup based on capacity limits.

	return nil
}

// ListTorrents returns status of all active torrents.
func (s *service) ListTorrents() ([]TorrentStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]TorrentStatus, 0, len(s.torrents))
	for _, t := range s.torrents {
		result = append(result, s.torrentToStatus(t))
	}

	return result, nil
}

// GetStatus returns detailed status of a specific torrent.
func (s *service) GetStatus(infoHash string) (*TorrentStatus, error) {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTorrentNotFound
	}

	status := s.torrentToStatus(t)
	return &status, nil
}

// Pause pauses downloading/uploading for a torrent.
func (s *service) Pause(infoHash string) error {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return ErrTorrentNotFound
	}

	t.DisallowDataDownload()
	t.DisallowDataUpload()

	s.log.Info("paused torrent", "hash", infoHash)
	return nil
}

// Resume resumes a paused torrent.
func (s *service) Resume(infoHash string) error {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return ErrTorrentNotFound
	}

	t.AllowDataDownload()
	t.AllowDataUpload()

	s.log.Info("resumed torrent", "hash", infoHash)
	return nil
}

// Close shuts down the torrent service.
func (s *service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Unregister all torrents from activity manager
	if s.am != nil {
		for hash := range s.torrents {
			s.am.Unregister(hash)
		}
	}

	// Clear maps (client shutdown is handled separately)
	s.torrents = make(map[string]*torrent.Torrent)
	s.fileIndexes = make(map[string]map[string]*torrent.File)

	s.log.Info("torrent service closed")
	return nil
}

// CollectStats returns complete statistics for all active torrents.
func (s *service) CollectStats() []FullStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]FullStats, 0, len(s.torrents))
	for _, t := range s.torrents {
		stats := t.Stats()

		var totalSize int64
		var name string
		if info := t.Info(); info != nil {
			totalSize = info.TotalLength()
			name = info.Name
		}

		result = append(result, FullStats{
			InfoHash:          t.InfoHash().HexString(),
			Name:              name,
			TotalSize:         totalSize,
			BytesCompleted:    t.BytesCompleted(),
			ActivePeers:       stats.ActivePeers,
			ConnectedSeeders:  stats.ConnectedSeeders,
			HalfOpenPeers:     stats.HalfOpenPeers,
			PiecesComplete:    stats.PiecesComplete,
			BytesReadData:     stats.BytesReadData.Int64(),
			BytesWrittenData:  stats.BytesWrittenData.Int64(),
			ChunksReadWasted:  stats.ChunksReadWasted.Int64(),
			PiecesDirtiedGood: stats.PiecesDirtiedGood.Int64(),
			PiecesDirtiedBad:  stats.PiecesDirtiedBad.Int64(),
		})
	}
	return result
}

// GetDebugInfo returns detailed piece-level debug information for a torrent.
func (s *service) GetDebugInfo(infoHash string) (*DebugInfo, error) {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTorrentNotFound
	}

	info := t.Info()
	if info == nil {
		return &DebugInfo{
			InfoHash: infoHash,
			Name:     "Waiting for metadata...",
		}, nil
	}

	numPieces := t.NumPieces()
	pieceLength := info.PieceLength

	// Build piece bitmap: bit 7 = complete, bit 6 = partial, bits 0-2 = priority
	pieceBuf := make([]byte, numPieces)
	piecesComplete := 0
	for i := 0; i < numPieces; i++ {
		ps := t.Piece(i).State()
		var b byte
		if ps.Completion.Complete {
			b |= 0x80
			piecesComplete++
		}
		if ps.Partial {
			b |= 0x40
		}
		pri := int(ps.Priority)
		if pri > 7 {
			pri = 7
		}
		b |= byte(pri)
		pieceBuf[i] = b
	}

	piecesB64 := base64.StdEncoding.EncodeToString(pieceBuf)

	// Build file info
	files := t.Files()
	fileInfos := make([]DebugFileInfo, len(files))
	for i, f := range files {
		bc := f.BytesCompleted()
		var progress float64
		if f.Length() > 0 {
			progress = float64(bc) / float64(f.Length())
		}
		beginPiece := int(f.Offset() / pieceLength)
		endPiece := int((f.Offset() + f.Length() - 1) / pieceLength)
		if f.Length() == 0 {
			endPiece = beginPiece
		}
		fileInfos[i] = DebugFileInfo{
			Path:           f.Path(),
			Length:         f.Length(),
			BytesCompleted: bc,
			Progress:       progress,
			BeginPiece:     beginPiece,
			EndPiece:       endPiece,
		}
	}

	// Build stats
	stats := t.Stats()
	totalSize := info.TotalLength()

	return &DebugInfo{
		InfoHash:    infoHash,
		Name:        info.Name,
		PieceLength: pieceLength,
		NumPieces:   numPieces,
		Pieces:      piecesB64,
		Files:       fileInfos,
		Readers:     []DebugReaderInfo{},
		Stats: DebugStats{
			PiecesComplete:   piecesComplete,
			PiecesTotal:      numPieces,
			ActivePeers:      stats.ActivePeers,
			ConnectedSeeders: stats.ConnectedSeeders,
			BytesCompleted:   t.BytesCompleted(),
			TotalSize:        totalSize,
		},
	}, nil
}

// torrentToInfo converts a torrent.Torrent to TorrentInfo.
func (s *service) torrentToInfo(t *torrent.Torrent) *TorrentInfo {
	info := t.Info()
	if info == nil {
		return &TorrentInfo{
			InfoHash: t.InfoHash().HexString(),
		}
	}

	files := make([]identify.TorrentFile, 0, len(t.Files()))
	for _, f := range t.Files() {
		files = append(files, identify.TorrentFile{
			Path: f.Path(),
			Size: f.Length(),
		})
	}

	return &TorrentInfo{
		InfoHash:  t.InfoHash().HexString(),
		Name:      info.Name,
		Files:     files,
		TotalSize: info.TotalLength(),
		AddedAt:   time.Now(), // Note: could track actual add time separately
	}
}

// torrentToStatus converts a torrent.Torrent to TorrentStatus.
func (s *service) torrentToStatus(t *torrent.Torrent) TorrentStatus {
	hash := t.InfoHash().HexString()
	stats := t.Stats()

	var totalSize int64
	var name string
	if info := t.Info(); info != nil {
		totalSize = info.TotalLength()
		name = info.Name
	}

	// Calculate download progress
	var progress float64
	if totalSize > 0 {
		progress = float64(t.BytesCompleted()) / float64(totalSize)
	}

	// Check if paused via activity manager
	isPaused := false
	if s.am != nil {
		isPaused = s.am.IsPaused(hash)
	}

	return TorrentStatus{
		InfoHash:      hash,
		Name:          name,
		TotalSize:     totalSize,
		Downloaded:    t.BytesCompleted(),
		Progress:      progress,
		Seeders:       stats.ConnectedSeeders,
		Leechers:      stats.ActivePeers - stats.ConnectedSeeders,
		DownloadSpeed: 0, // Would need rate tracking
		UploadSpeed:   0, // Would need rate tracking
		IsPaused:      isPaused,
	}
}

// fileHandle wraps a torrent.File to implement TorrentFileHandle.
type fileHandle struct {
	file     *torrent.File
	nextFile *torrent.File // next file in torrent order, for pre-fetching
}

func (f *fileHandle) Path() string {
	return f.file.Path()
}

func (f *fileHandle) Length() int64 {
	return f.file.Length()
}

func (f *fileHandle) NewReader() TorrentReader {
	reader := f.file.NewReader()
	return &readerWrapper{reader: reader}
}

func (f *fileHandle) Torrent() *torrent.Torrent {
	return f.file.Torrent()
}

func (f *fileHandle) File() *torrent.File {
	return f.file
}

func (f *fileHandle) NextFile() *torrent.File {
	return f.nextFile
}

// readerWrapper wraps a torrent.Reader to implement TorrentReader.
type readerWrapper struct {
	reader torrent.Reader
}

func (r *readerWrapper) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

func (r *readerWrapper) Seek(offset int64, whence int) (int64, error) {
	return r.reader.Seek(offset, whence)
}

func (r *readerWrapper) Close() error {
	return r.reader.Close()
}

func (r *readerWrapper) SetResponsive() {
	r.reader.SetResponsive()
}
