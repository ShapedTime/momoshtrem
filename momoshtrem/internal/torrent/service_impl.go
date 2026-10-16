package torrent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/anacrolix/torrent/types"
	"golang.org/x/sync/singleflight"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

// Ensure service implements Service interface
var _ Service = (*service)(nil)

// service implements the Service interface using anacrolix/torrent.
//
// A torrent's lifecycle is serialised per info hash: one add, one wait for
// its metadata and one drop decision at a time, and RemoveTorrent takes the
// same lock. Concurrent adds of one hash (a job and a WebDAV read, say)
// share a single attempt, so a timeout in one can't drop a torrent the
// other is adding.
type service struct {
	mu     sync.RWMutex
	client *torrent.Client
	// am idles torrents and holds the user's pauses. Without idle mode it's
	// a manager that only pauses (newPauseManager): never nil.
	am *ActivityManager

	// Loaded torrents by info hash (lowercase hex), and when each loaded
	torrents map[string]*torrent.Torrent
	loadedAt map[string]time.Time

	// File index cache: infoHash -> filePath -> *torrent.File
	// Built after metadata arrives, enables O(1) file lookup.
	fileIndexes map[string]map[string]*torrent.File

	// Configuration
	addTimeout  time.Duration
	readTimeout time.Duration

	// hashLocks serialises add, wait, drop and remove per hash; adds shares
	// one add among concurrent callers.
	hashLocks keyedMutex
	adds      singleflight.Group
	// addSpec adds a torrent to the client: client.AddTorrentSpec, swapped
	// in tests.
	addSpec func(*torrent.TorrentSpec) (*torrent.Torrent, error)

	// metainfo stores each torrent's .torrent file; nil keeps none.
	metainfo *MetainfoStore
	// pieces is the client's piece storage, for deleting the data of a
	// torrent that isn't loaded; nil skips that.
	pieces storage.ClientImpl
	// speeds smooths each torrent's speed from its byte counters; nil
	// reports 0.
	speeds *SpeedSampler

	log *slog.Logger
}

// Option configures the torrent service.
type Option func(*service)

// WithMetainfoStore keeps each torrent's metainfo in store: written when its
// metadata arrives, loaded before a magnet is added (no wait for peers) and
// deleted with the torrent.
func WithMetainfoStore(store *MetainfoStore) Option {
	return func(s *service) { s.metainfo = store }
}

// WithPieceStorage gives the service the client's piece storage, so
// RemoveTorrent can delete the data of a torrent that isn't loaded, from
// its stored metainfo.
func WithPieceStorage(impl storage.ClientImpl) Option {
	return func(s *service) { s.pieces = impl }
}

// WithSpeedSampler reports each torrent's speed from sampler, which the
// caller runs over the service (sampler.Run).
func WithSpeedSampler(sampler *SpeedSampler) Option {
	return func(s *service) { s.speeds = sampler }
}

// NewService creates a new torrent service. am is nil without idle mode:
// torrents then stay active, and only Pause turns their network off.
func NewService(
	client *torrent.Client,
	am *ActivityManager,
	addTimeout, readTimeout time.Duration,
	opts ...Option,
) Service {
	return newService(client, am, addTimeout, readTimeout, opts...)
}

func newService(client *torrent.Client, am *ActivityManager, addTimeout, readTimeout time.Duration, opts ...Option) *service {
	if am == nil {
		am = newPauseManager()
	}
	s := &service{
		client:      client,
		am:          am,
		torrents:    make(map[string]*torrent.Torrent),
		loadedAt:    make(map[string]time.Time),
		fileIndexes: make(map[string]map[string]*torrent.File),
		addTimeout:  addTimeout,
		readTimeout: readTimeout,
		log:         slog.With("component", "torrent-service"),
	}
	s.addSpec = func(spec *torrent.TorrentSpec) (*torrent.Torrent, error) {
		t, _, err := s.client.AddTorrentSpec(spec)
		return t, err
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AddTorrent adds a torrent by magnet URI and waits for metadata. A torrent
// whose metainfo is stored loads at once; otherwise the info dict comes from
// peers, within addTimeout. Concurrent adds of one hash share one attempt
// and its result.
func (s *service) AddTorrent(magnetURI string) (*TorrentInfo, error) {
	m, err := metainfo.ParseMagnetUri(magnetURI)
	if err != nil {
		s.log.Warn("invalid magnet URI", "error", err)
		return nil, ErrInvalidMagnet
	}
	hash := m.InfoHash.HexString()

	if t := s.loaded(hash); t != nil {
		s.log.Debug("torrent already loaded", "hash", hash)
		return s.torrentToInfo(t), nil
	}
	v, err, _ := s.adds.Do(hash, func() (any, error) { return s.add(hash, magnetURI) })
	if err != nil {
		return nil, err
	}
	return v.(*TorrentInfo), nil
}

// add loads the torrent under its hash lock: from the stored metainfo when
// there is one, else from peers, dropping it when its metadata doesn't come
// in time.
func (s *service) add(hash, magnetURI string) (*TorrentInfo, error) {
	unlock := s.hashLocks.Lock(hash)
	defer unlock()
	if t := s.loaded(hash); t != nil {
		return s.torrentToInfo(t), nil
	}

	spec, err := torrent.TorrentSpecFromMagnetUri(magnetURI)
	if err != nil {
		s.log.Warn("invalid magnet URI", "error", err)
		return nil, ErrInvalidMagnet
	}
	stored := s.storedMetainfo(hash)
	if stored != nil {
		spec.InfoBytes = stored.InfoBytes
		spec.Trackers = append(spec.Trackers, stored.UpvertedAnnounceList()...)
	}

	t, err := s.addSpec(spec)
	if err != nil {
		s.log.Error("failed to add magnet", "hash", hash, "from_store", stored != nil, "error", err)
		if stored != nil {
			// The client rejected the stored info dict: fetch it from peers
			// next time.
			s.forgetMetainfo(hash)
		}
		return nil, err
	}

	if stored == nil {
		s.log.Info("waiting for torrent metadata", "hash", hash)
	}

	// Wait for metadata with strict timeout
	timer := time.NewTimer(s.addTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		s.log.Warn("timeout waiting for torrent metadata", "hash", hash)
		t.Drop() // Clean up failed torrent
		return nil, ErrMetadataTimeout
	case <-t.GotInfo():
		s.log.Info("obtained torrent metadata",
			"hash", hash,
			"name", t.Info().Name,
			"files", len(t.Files()),
			"from_store", stored != nil,
		)
	}
	if stored == nil {
		s.saveMetainfo(hash, t)
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

	// Register with activity manager for idle tracking and pauses
	s.am.Register(hash, t)

	// Store in map
	s.mu.Lock()
	s.torrents[hash] = t
	s.loadedAt[hash] = time.Now()
	s.fileIndexes[hash] = idx
	s.mu.Unlock()

	return s.torrentToInfo(t), nil
}

// loaded returns the loaded torrent with the hash, or nil.
func (s *service) loaded(hash string) *torrent.Torrent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.torrents[hash]
}

// storedMetainfo returns the torrent's stored metainfo, or nil when there's
// none (or no store). A corrupt file is removed, so the torrent loads from
// peers and is stored again.
func (s *service) storedMetainfo(hash string) *metainfo.MetaInfo {
	if s.metainfo == nil {
		return nil
	}
	mi, err := s.metainfo.Load(hash)
	if err != nil {
		s.log.Warn("ignoring stored metainfo", "hash", hash, "error", err)
		if errors.Is(err, ErrCorruptMetainfo) {
			s.forgetMetainfo(hash)
		}
		return nil
	}
	return mi
}

// saveMetainfo stores the metainfo of a torrent whose metadata just came
// from peers. A failure only costs the next load a wait for peers.
func (s *service) saveMetainfo(hash string, t *torrent.Torrent) {
	if s.metainfo == nil {
		return
	}
	mi := t.Metainfo()
	if err := s.metainfo.Save(hash, &mi); err != nil {
		s.log.Warn("failed to store torrent metainfo", "hash", hash, "error", err)
	}
}

// forgetMetainfo deletes the torrent's stored metainfo, if any.
func (s *service) forgetMetainfo(hash string) {
	if s.metainfo == nil {
		return
	}
	if err := s.metainfo.Remove(hash); err != nil {
		s.log.Warn("failed to remove torrent metainfo", "hash", hash, "error", err)
	}
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
// AddTorrent already answers a loaded torrent at once.
func (s *service) GetOrAddTorrent(magnetURI string) (*TorrentInfo, error) {
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

// RemoveTorrent removes a torrent from the client and deletes its stored
// metainfo. It waits for an add of the same hash in flight. With deleteData
// it also deletes the torrent's pieces from the cache (MarkNotComplete on
// each), using the live info dict or, for a torrent that isn't loaded, the
// stored metainfo. Partial pieces (their "incompleted" chunks) are left to
// the cache's LRU. ErrTorrentNotFound when the torrent is neither loaded nor
// stored: there's nothing to remove, and no way to name its pieces.
//
// Dropping the torrent closes it, so readers still open on its files fail.
// WebDAV readers belong to their request; there's no other handle to close.
func (s *service) RemoveTorrent(infoHash string, deleteData bool) error {
	hash := NormalizeInfoHash(infoHash)
	if hash == "" {
		return ErrTorrentNotFound
	}
	unlock := s.hashLocks.Lock(hash)
	defer unlock()

	s.mu.Lock()
	t, loaded := s.torrents[hash]
	delete(s.torrents, hash)
	delete(s.loadedAt, hash)
	delete(s.fileIndexes, hash)
	s.mu.Unlock()

	if loaded {
		// Unregister from activity manager (a pause stays)
		s.am.Unregister(hash)
		var pieces []storage.Piece
		if deleteData {
			pieces = livePieces(t)
		}
		// Drop from client (stops downloading/uploading)
		t.Drop()
		if deleteData {
			s.deletePieces(hash, pieces)
		}
		s.forgetMetainfo(hash)
		s.log.Info("removed torrent", "hash", hash, "delete_data", deleteData)
		return nil
	}

	stored := s.storedMetainfo(hash)
	if stored == nil {
		return ErrTorrentNotFound
	}
	if deleteData {
		pieces, err := s.storedPieces(hash, stored)
		if err != nil {
			s.log.Warn("couldn't delete the data of a torrent that isn't loaded", "hash", hash, "error", err)
		} else {
			s.deletePieces(hash, pieces)
		}
	}
	s.forgetMetainfo(hash)
	s.log.Info("removed a torrent that wasn't loaded", "hash", hash, "delete_data", deleteData)
	return nil
}

// livePieces is the storage of each piece of a loaded torrent.
func livePieces(t *torrent.Torrent) []storage.Piece {
	if t.Info() == nil {
		return nil
	}
	pieces := make([]storage.Piece, t.NumPieces())
	for i := range pieces {
		pieces[i] = t.Piece(i).Storage()
	}
	return pieces
}

// storedPieces opens the storage of each piece stored metainfo names,
// without adding the torrent to the client. Pieces without a v1 hash (pure
// v2 torrents) are skipped: their hashes are in piece layers.
func (s *service) storedPieces(hash string, mi *metainfo.MetaInfo) ([]storage.Piece, error) {
	if s.pieces == nil {
		return nil, errors.New("no piece storage configured")
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorruptMetainfo, err)
	}
	st, err := storage.NewClient(s.pieces).OpenTorrent(context.Background(), &info, metainfo.NewHashFromHex(hash))
	if err != nil {
		return nil, fmt.Errorf("failed to open the torrent's storage: %w", err)
	}
	if st.Close != nil {
		defer st.Close()
	}
	pieces := make([]storage.Piece, 0, info.NumPieces())
	skipped := 0
	for i := 0; i < info.NumPieces(); i++ {
		p := info.Piece(i)
		if !p.V1Hash().Ok {
			skipped++
			continue
		}
		pieces = append(pieces, st.Piece(p)) // it has a v1 hash, so Piece can't panic
	}
	if skipped > 0 {
		s.log.Warn("skipping pieces without a v1 hash", "hash", hash, "pieces", skipped)
	}
	return pieces, nil
}

// deletePieces marks every piece not complete, which deletes its file from
// the cache. Failures are logged: the torrent is gone either way.
func (s *service) deletePieces(hash string, pieces []storage.Piece) {
	failed := 0
	var firstErr error
	for _, p := range pieces {
		if err := p.MarkNotComplete(); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if failed > 0 {
		s.log.Warn("failed to delete some torrent pieces", "hash", hash, "failed", failed, "pieces", len(pieces), "error", firstErr)
		return
	}
	s.log.Info("deleted torrent data", "hash", hash, "pieces", len(pieces))
}

// ListTorrents returns status of all loaded torrents.
func (s *service) ListTorrents() ([]TorrentStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]TorrentStatus, 0, len(s.torrents))
	for hash, t := range s.torrents {
		result = append(result, s.torrentToStatus(hash, t))
	}

	return result, nil
}

// GetStatus returns detailed status of a specific torrent.
func (s *service) GetStatus(infoHash string) (*TorrentStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, exists := s.torrents[infoHash]
	if !exists {
		return nil, ErrTorrentNotFound
	}

	status := s.torrentToStatus(infoHash, t)
	return &status, nil
}

// Pause pauses a torrent through the activity manager, loaded or not.
func (s *service) Pause(infoHash string) error {
	hash := NormalizeInfoHash(infoHash)
	if hash == "" {
		return fmt.Errorf("%w: %q", ErrInvalidInfoHash, infoHash)
	}
	s.am.Pause(hash, time.Now())
	s.log.Info("paused torrent", "hash", hash, "loaded", s.loaded(hash) != nil)
	return nil
}

// Resume clears a torrent's pause through the activity manager.
func (s *service) Resume(infoHash string) error {
	hash := NormalizeInfoHash(infoHash)
	if hash == "" {
		return fmt.Errorf("%w: %q", ErrInvalidInfoHash, infoHash)
	}
	s.am.Resume(hash)
	s.log.Info("resumed torrent", "hash", hash, "loaded", s.loaded(hash) != nil)
	return nil
}

// PausedAt returns when the torrent was paused.
func (s *service) PausedAt(infoHash string) (time.Time, bool) {
	return s.am.PausedAt(NormalizeInfoHash(infoHash))
}

// Pauses returns every paused torrent.
func (s *service) Pauses() map[string]time.Time {
	return s.am.Pauses()
}

// RestorePauses applies pauses kept from before a restart. Keys that aren't
// info hashes are skipped.
func (s *service) RestorePauses(pauses map[string]time.Time) {
	restored := 0
	for hash, at := range pauses {
		if norm := NormalizeInfoHash(hash); norm != "" {
			s.am.Pause(norm, at)
			restored++
		}
	}
	if restored > 0 {
		s.log.Info("restored torrent pauses", "count", restored)
	}
}

// Close shuts down the torrent service.
func (s *service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Unregister all torrents from activity manager
	for hash := range s.torrents {
		s.am.Unregister(hash)
	}

	// Clear maps (client shutdown is handled separately)
	s.torrents = make(map[string]*torrent.Torrent)
	s.loadedAt = make(map[string]time.Time)
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
		var pieceLength int64
		if info := t.Info(); info != nil {
			totalSize = info.TotalLength()
			name = info.Name
			pieceLength = info.PieceLength
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
			PieceLength:       pieceLength,
			NumPieces:         t.NumPieces(),
		})
	}
	return result
}

// FilesProgress returns per-file progress of a loaded torrent.
func (s *service) FilesProgress(infoHash string) (map[string]FileProgress, error) {
	s.mu.RLock()
	t, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTorrentNotFound
	}
	if t.Info() == nil {
		return map[string]FileProgress{}, nil
	}

	files := t.Files()
	progress := make(map[string]FileProgress, len(files))
	for _, f := range files {
		progress[f.Path()] = FileProgress{Length: f.Length(), BytesCompleted: f.BytesCompleted()}
	}
	return progress, nil
}

// IsActive reports whether a torrent is loaded, not idle and not paused.
// Without idle mode, loaded torrents are active unless paused.
func (s *service) IsActive(infoHash string) bool {
	s.mu.RLock()
	_, exists := s.torrents[infoHash]
	s.mu.RUnlock()

	if !exists {
		return false
	}
	return !s.am.IsPaused(infoHash)
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

// torrentToStatus converts a loaded torrent to TorrentStatus. The caller
// holds s.mu.
func (s *service) torrentToStatus(hash string, t *torrent.Torrent) TorrentStatus {
	stats := t.Stats()
	done := t.BytesCompleted()

	var totalSize int64
	var name string
	info := t.Info()
	if info != nil {
		totalSize = info.TotalLength()
		name = info.Name
	}

	// Calculate download progress
	var progress float64
	if totalSize > 0 {
		progress = float64(done) / float64(totalSize)
	}

	var speed Rate
	if s.speeds != nil {
		speed = s.speeds.Rate(hash)
	}
	_, paused := s.am.PausedAt(hash)
	lastActive, _ := s.am.LastAccess(hash)

	return TorrentStatus{
		InfoHash:      hash,
		Name:          name,
		TotalSize:     totalSize,
		Downloaded:    done,
		Progress:      progress,
		Complete:      info != nil && done >= totalSize,
		Seeders:       stats.ConnectedSeeders,
		Leechers:      stats.ActivePeers - stats.ConnectedSeeders,
		Peers:         stats.ActivePeers,
		DownloadSpeed: speed.Down,
		UploadSpeed:   speed.Up,
		IsPaused:      s.am.IsPaused(hash),
		Paused:        paused,
		LoadedAt:      s.loadedAt[hash],
		LastActive:    lastActive,
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
