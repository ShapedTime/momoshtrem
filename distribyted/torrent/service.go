package torrent

import (
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/distribyted/distribyted/fs"
	"github.com/distribyted/distribyted/torrent/loader"
)

type Service struct {
	c *torrent.Client

	s *Stats

	mu  sync.Mutex
	fss map[string]fs.Filesystem

	loaders []loader.Loader
	db      loader.LoaderAdder

	// In-memory metadata cache for fast access
	metadataMu sync.RWMutex
	metadata   map[string]*loader.TMDBMetadata // keyed by infohash

	log                     zerolog.Logger
	addTimeout, readTimeout int
	continueWhenAddTimeout  bool
}

func NewService(loaders []loader.Loader, db loader.LoaderAdder, stats *Stats, c *torrent.Client, addTimeout, readTimeout int, continueWhenAddTimeout bool) *Service {
	l := log.Logger.With().Str("component", "torrent-service").Logger()
	return &Service{
		log:                    l,
		s:                      stats,
		c:                      c,
		fss:                    make(map[string]fs.Filesystem),
		loaders:                loaders,
		db:                     db,
		metadata:               make(map[string]*loader.TMDBMetadata),
		addTimeout:             addTimeout,
		readTimeout:            readTimeout,
		continueWhenAddTimeout: continueWhenAddTimeout,
	}
}

func (s *Service) Load() (map[string]fs.Filesystem, error) {
	// Load from config
	s.log.Info().Msg("adding torrents from configuration")
	for _, loader := range s.loaders {
		if err := s.load(loader); err != nil {
			return nil, err
		}
	}

	// Load from DB
	s.log.Info().Msg("adding torrents from database")
	return s.fss, s.load(s.db)
}

func (s *Service) load(l loader.Loader) error {
	list, err := l.ListMagnets()
	if err != nil {
		return err
	}
	for r, twms := range list {
		s.addRoute(r)
		for _, twm := range twms {
			if err := s.addMagnet(r, twm.MagnetURI); err != nil {
				return err
			}
			// Cache metadata if present
			if twm.Metadata != nil {
				spec, parseErr := metainfo.ParseMagnetUri(twm.MagnetURI)
				if parseErr == nil && spec.InfoHash.HexString() != "" {
					s.metadataMu.Lock()
					s.metadata[spec.InfoHash.HexString()] = twm.Metadata
					s.metadataMu.Unlock()
				}
			}
		}
	}

	list2, err := l.ListTorrentPaths()
	if err != nil {
		return err
	}
	for r, ms := range list2 {
		s.addRoute(r)
		for _, p := range ms {
			if err := s.addTorrentPath(r, p); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Service) AddMagnet(r, m string, metadata *loader.TMDBMetadata) error {
	if err := s.addMagnet(r, m); err != nil {
		return err
	}

	// Cache metadata in memory if provided
	if metadata != nil {
		spec, parseErr := metainfo.ParseMagnetUri(m)
		if parseErr == nil && spec.InfoHash.HexString() != "" {
			s.metadataMu.Lock()
			s.metadata[spec.InfoHash.HexString()] = metadata
			s.metadataMu.Unlock()
		}
	}

	// Add to db
	return s.db.AddMagnet(r, m, metadata)
}

func (s *Service) addTorrentPath(r, p string) error {
	// Add to client
	t, err := s.c.AddTorrentFromFile(p)
	if err != nil {
		return err
	}

	return s.addTorrent(r, t)
}

func (s *Service) addMagnet(r, m string) error {
	// Add to client
	t, err := s.c.AddMagnet(m)
	if err != nil {
		return err
	}

	return s.addTorrent(r, t)

}

func (s *Service) addRoute(r string) {
	s.s.AddRoute(r)

	// Add to filesystems
	folder := path.Join("/", r)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.fss[folder]
	if !ok {
		s.fss[folder] = fs.NewTorrent(s.readTimeout)
	}
}

func (s *Service) addTorrent(r string, t *torrent.Torrent) error {
	// only get info if name is not available
	if t.Info() == nil {
		s.log.Info().Str("hash", t.InfoHash().String()).Msg("getting torrent info")
		select {
		case <-time.After(time.Duration(s.addTimeout) * time.Second):
			s.log.Warn().Str("hash", t.InfoHash().String()).Msg("timeout getting torrent info")
			if !s.continueWhenAddTimeout {
				return errors.New("timeout getting torrent info")
			} else {
				s.log.Info().Str("hash", t.InfoHash().String()).Msg("ignoring timeout error")
				return nil
			}
		case <-t.GotInfo():
			s.log.Info().Str("hash", t.InfoHash().String()).Msg("obtained torrent info")
		}

	}

	// Add to stats
	s.s.Add(r, t)

	// Add to filesystems
	folder := path.Join("/", r)
	s.mu.Lock()
	defer s.mu.Unlock()

	tfs, ok := s.fss[folder].(*fs.Torrent)
	if !ok {
		return errors.New("error adding torrent to filesystem")
	}

	tfs.AddTorrent(t)
	s.log.Info().Str("name", t.Info().Name).Str("route", r).Msg("torrent added")

	return nil
}

func (s *Service) RemoveFromHash(r, h string) error {
	// Remove from db
	deleted, err := s.db.RemoveFromHash(r, h)
	if err != nil {
		return err
	}

	if !deleted {
		return fmt.Errorf("element with hash %v on route %v cannot be removed", h, r)
	}

	// Remove from stats
	s.s.Del(r, h)

	// Remove from fs
	folder := path.Join("/", r)

	tfs, ok := s.fss[folder].(*fs.Torrent)
	if !ok {
		return errors.New("error removing torrent from filesystem")
	}

	tfs.RemoveTorrent(h)

	// Remove from client
	var mh metainfo.Hash
	if err := mh.FromHexString(h); err != nil {
		return err
	}

	t, ok := s.c.Torrent(metainfo.NewHashFromHex(h))
	if ok {
		t.Drop()
	}

	// Remove from metadata cache
	s.metadataMu.Lock()
	delete(s.metadata, h)
	s.metadataMu.Unlock()

	return nil
}

// GetTorrentMetadata returns cached metadata for a torrent hash
func (s *Service) GetTorrentMetadata(hash string) *loader.TMDBMetadata {
	s.metadataMu.RLock()
	defer s.metadataMu.RUnlock()
	return s.metadata[hash]
}

// GetTorrentInfo retrieves full torrent info from database
func (s *Service) GetTorrentInfo(route, hash string) (*loader.TorrentWithMetadata, error) {
	return s.db.GetTorrentInfo(route, hash)
}

// UpdateMetadata updates metadata for an existing torrent
func (s *Service) UpdateMetadata(route, hash string, metadata *loader.TMDBMetadata) error {
	// Update in database
	if err := s.db.UpdateMetadata(route, hash, metadata); err != nil {
		return err
	}

	// Update in-memory cache
	s.metadataMu.Lock()
	s.metadata[hash] = metadata
	s.metadataMu.Unlock()

	return nil
}
