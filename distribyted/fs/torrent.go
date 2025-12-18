package fs

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/anacrolix/missinggo/v2"
	"github.com/anacrolix/torrent"
	"github.com/rs/zerolog/log"

	"github.com/distribyted/distribyted/episode"
	"github.com/distribyted/distribyted/iio"
	tloader "github.com/distribyted/distribyted/torrent/loader"
)

var _ Filesystem = &Torrent{}

type Torrent struct {
	mu              sync.RWMutex
	ts              map[string]*torrent.Torrent
	s               *storage
	loaded          bool
	readTimeout     int
	identifications map[string]*episode.IdentificationResult // keyed by infohash
	metadata        map[string]*tloader.TMDBMetadata          // keyed by infohash
	virtualMapper   *VirtualPathMapper
}

func NewTorrent(readTimeout int) *Torrent {
	return &Torrent{
		s:               newStorage(SupportedFactories),
		ts:              make(map[string]*torrent.Torrent),
		readTimeout:     readTimeout,
		identifications: make(map[string]*episode.IdentificationResult),
		metadata:        make(map[string]*tloader.TMDBMetadata),
		virtualMapper:   NewVirtualPathMapper(),
	}
}

func (fs *Torrent) AddTorrent(t *torrent.Torrent) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.loaded = false
	fs.ts[t.InfoHash().HexString()] = t
}

func (fs *Torrent) RemoveTorrent(h string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.s.Clear()

	fs.loaded = false

	// Clean up identification results
	if _, exists := fs.identifications[h]; exists {
		log.Debug().
			Str("hash", h).
			Msg("fs: removing identification results")
	}
	delete(fs.identifications, h)

	// Clean up metadata
	if _, exists := fs.metadata[h]; exists {
		log.Debug().
			Str("hash", h).
			Msg("fs: removing metadata")
	}
	delete(fs.metadata, h)

	delete(fs.ts, h)

	// Clear and rebuild virtual mappings for remaining torrents
	fs.virtualMapper.Clear()
	// Note: virtual mappings will be rebuilt on next load()
}

// SetIdentification stores identification results for a torrent
func (fs *Torrent) SetIdentification(hash string, result *episode.IdentificationResult) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.identifications[hash] = result

	// Debug: Log when identification is stored in FS layer
	log.Debug().
		Str("hash", hash).
		Int("identified_count", result.IdentifiedCount).
		Int("unidentified_count", len(result.UnidentifiedFiles)).
		Msg("fs: stored identification results")
}

// GetIdentification retrieves identification results for a torrent
func (fs *Torrent) GetIdentification(hash string) *episode.IdentificationResult {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	result := fs.identifications[hash]

	// Debug: Log retrieval from FS layer
	if result != nil {
		log.Debug().
			Str("hash", hash).
			Int("identified_count", result.IdentifiedCount).
			Msg("fs: retrieved identification results")
	} else {
		log.Debug().
			Str("hash", hash).
			Msg("fs: no identification results for hash")
	}

	return result
}

// SetMetadata stores TMDB metadata for a torrent
func (fs *Torrent) SetMetadata(hash string, metadata *tloader.TMDBMetadata) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.metadata[hash] = metadata

	// Trigger rebuild of virtual mappings on next load
	fs.loaded = false

	log.Debug().
		Str("hash", hash).
		Str("title", metadata.Title).
		Str("type", string(metadata.Type)).
		Int("year", metadata.Year).
		Msg("fs: stored metadata for torrent")
}

// GetMetadata retrieves TMDB metadata for a torrent
func (fs *Torrent) GetMetadata(hash string) *tloader.TMDBMetadata {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.metadata[hash]
}

func (fs *Torrent) load() {
	if fs.loaded {
		return
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	for _, t := range fs.ts {
		<-t.GotInfo()
		for _, file := range t.Files() {
			fs.s.Add(&torrentFile{
				readerFunc: file.NewReader,
				len:        file.Length(),
				timeout:    fs.readTimeout,
			}, file.Path())
		}
	}

	// Build virtual path mappings after loading all files
	fs.buildVirtualMappings()

	fs.loaded = true
}

// buildVirtualMappings creates virtual paths for all identified files
func (fs *Torrent) buildVirtualMappings() {
	// Clear existing mappings first
	fs.virtualMapper.Clear()

	totalMappings := 0

	for hash, t := range fs.ts {
		metadata := fs.metadata[hash]
		identification := fs.identifications[hash]

		// Skip if no metadata or identification
		if metadata == nil {
			log.Debug().
				Str("hash", hash).
				Msg("fs: skipping virtual mapping - no metadata")
			continue
		}

		if identification == nil {
			log.Debug().
				Str("hash", hash).
				Msg("fs: skipping virtual mapping - no identification")
			continue
		}

		log.Debug().
			Str("hash", hash).
			Str("title", metadata.Title).
			Int("identified_count", identification.IdentifiedCount).
			Msg("fs: building virtual mappings for torrent")

		// Get torrent name for context
		torrentName := ""
		if t.Info() != nil {
			torrentName = t.Info().Name
		}

		// Create mappings for each identified file
		for _, identified := range identification.IdentifiedFiles {
			virtualPath := GenerateVirtualPath(metadata, &identified)
			if virtualPath == "" {
				log.Debug().
					Str("hash", hash).
					Str("realPath", identified.FilePath).
					Msg("fs: failed to generate virtual path")
				continue
			}

			// The real path in storage is just the file path from the torrent
			realPath := "/" + identified.FilePath

			// Add mapping with conflict resolution
			actualVirtualPath := fs.virtualMapper.AddMappingWithConflictResolution(
				virtualPath,
				realPath,
				identified.Quality,
				hash,
			)

			log.Debug().
				Str("hash", hash).
				Str("realPath", realPath).
				Str("virtualPath", actualVirtualPath).
				Int("season", identified.Season).
				Ints("episodes", identified.Episodes).
				Str("torrentName", torrentName).
				Msg("fs: virtual path generated")

			totalMappings++
		}
	}

	log.Info().
		Int("total_mappings", totalMappings).
		Int("torrents_with_mappings", len(fs.ts)).
		Msg("fs: virtual mappings complete")
}

func (fs *Torrent) Open(filename string) (File, error) {
	fs.load()

	// Try virtual → real translation first
	if realPath, found := fs.virtualMapper.ToReal(filename); found {
		log.Debug().
			Str("virtual", filename).
			Str("real", realPath).
			Msg("fs: opening via virtual path")
		return fs.s.Get(realPath)
	}

	// Check if it's a virtual directory
	if fs.virtualMapper.IsVirtualDir(filename) {
		log.Debug().
			Str("path", filename).
			Msg("fs: opening virtual directory")
		return &Dir{}, nil
	}

	// Fall back to real path (backward compatibility)
	log.Debug().
		Str("path", filename).
		Msg("fs: opening via real path")
	return fs.s.Get(filename)
}

func (fs *Torrent) ReadDir(dirPath string) (map[string]File, error) {
	fs.load()

	result := make(map[string]File)

	// Get virtual children at this path
	virtualChildren := fs.virtualMapper.VirtualChildren(dirPath)
	for _, childName := range virtualChildren {
		childPath := cleanPath(dirPath + "/" + childName)

		// Check if it's a virtual directory or file
		if fs.virtualMapper.IsVirtualDir(childPath) {
			result[childName] = &Dir{}
			log.Debug().
				Str("parent", dirPath).
				Str("child", childName).
				Msg("fs: virtual dir listing - directory")
		} else if realPath, found := fs.virtualMapper.ToReal(childPath); found {
			// It's a file - get the actual file for size info
			if file, err := fs.s.Get(realPath); err == nil {
				result[childName] = file
				log.Debug().
					Str("parent", dirPath).
					Str("child", childName).
					Str("realPath", realPath).
					Msg("fs: virtual dir listing - file")
			}
		}
	}

	// Also include real paths that don't have virtual mappings (backward compat)
	realChildren, err := fs.s.Children(dirPath)
	if err == nil {
		for name, file := range realChildren {
			childPath := cleanPath(dirPath + "/" + name)
			// Only add if not already in result and doesn't have a virtual mapping
			if _, exists := result[name]; !exists {
				if _, hasVirtual := fs.virtualMapper.ToVirtual(childPath); !hasVirtual {
					result[name] = file
					log.Debug().
						Str("parent", dirPath).
						Str("child", name).
						Msg("fs: real path without virtual mapping")
				}
			}
		}
	}

	log.Debug().
		Str("path", dirPath).
		Int("children_count", len(result)).
		Msg("fs: virtual dir listing complete")

	return result, nil
}


type reader interface {
	iio.Reader
	missinggo.ReadContexter
}

type readAtWrapper struct {
	timeout int
	mu      sync.Mutex

	torrent.Reader
	io.ReaderAt
	io.Closer
}

func newReadAtWrapper(r torrent.Reader, timeout int) reader {
	return &readAtWrapper{Reader: r, timeout: timeout}
}

func (rw *readAtWrapper) ReadAt(p []byte, off int64) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	_, err := rw.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}

	return readAtLeast(rw, rw.timeout, p, len(p))
}

func readAtLeast(r missinggo.ReadContexter, timeout int, buf []byte, min int) (n int, err error) {
	if len(buf) < min {
		return 0, io.ErrShortBuffer
	}
	for n < min && err == nil {
		var nn int

		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(
			time.Duration(timeout)*time.Second,
			func() {
				cancel()
			},
		)

		nn, err = r.ReadContext(ctx, buf[n:])
		n += nn

		timer.Stop()
	}
	if n >= min {
		err = nil
	} else if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (rw *readAtWrapper) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.Reader.Close()
}

var _ File = &torrentFile{}

type torrentFile struct {
	readerFunc func() torrent.Reader
	reader     reader
	len        int64
	timeout    int
}

func (d *torrentFile) load() {
	if d.reader != nil {
		return
	}
	d.reader = newReadAtWrapper(d.readerFunc(), d.timeout)
}

func (d *torrentFile) Size() int64 {
	return d.len
}

func (d *torrentFile) IsDir() bool {
	return false
}

func (d *torrentFile) Close() error {
	var err error
	if d.reader != nil {
		err = d.reader.Close()
	}

	d.reader = nil

	return err
}

func (d *torrentFile) Read(p []byte) (n int, err error) {
	d.load()
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(
		time.Duration(d.timeout)*time.Second,
		func() {
			cancel()
		},
	)

	defer timer.Stop()

	return d.reader.ReadContext(ctx, p)
}

func (d *torrentFile) ReadAt(p []byte, off int64) (n int, err error) {
	d.load()
	return d.reader.ReadAt(p, off)
}
