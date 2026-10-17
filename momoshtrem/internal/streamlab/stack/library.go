package stack

import (
	"errors"
	"fmt"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// Source is what a library entry plays: a file of a lab torrent, or an
// HDRezka stream (Stream). It's what its assignment points at.
type Source struct {
	// Magnet is the torrent's magnet. A seeder's (swarm.Seeder.Magnet)
	// carries its address as x.pe, so the leecher finds it through the
	// same service.add path as any magnet.
	Magnet string
	Layout *media.Layout // the torrent's content, or the stream's
	File   int           // the file's index in Layout.Files()
	// Stream, when set, makes the source an HDRezka stream: its file is
	// served by the fake CDN (cdn.CDN.Add with the same key and quality),
	// and Magnet is unused.
	Stream *library.StreamRef
}

// file is the source's file in its layout.
func (s Source) file() media.File {
	if s.Layout == nil || s.File < 0 || s.File >= len(s.Layout.Files()) {
		return media.File{}
	}
	return s.Layout.Files()[s.File]
}

// filePath is the file's path as the library stores it: in the torrent,
// or the stream's made-up one.
func (s Source) filePath() string {
	if s.Stream != nil {
		return s.Stream.FilePath()
	}
	return s.file().Path
}

// assignment is the source as the library stores it: an active torrent
// or HDRezka assignment of the file.
func (s Source) assignment() (*library.TorrentAssignment, error) {
	if s.Layout == nil {
		return nil, errors.New("stack: a source needs a layout")
	}
	if s.File < 0 || s.File >= len(s.Layout.Files()) {
		return nil, fmt.Errorf("stack: %q has no file %d", s.Layout.Name(), s.File)
	}
	if s.Stream != nil {
		ref := *s.Stream
		return &library.TorrentAssignment{
			Kind:     library.AssignmentKindHDRezka,
			FilePath: ref.FilePath(),
			FileSize: s.file().Size,
			Stream:   &ref,
			IsActive: true,
		}, nil
	}
	m, err := metainfo.ParseMagnetUri(s.Magnet)
	if err != nil {
		return nil, fmt.Errorf("stack: magnet of %q: %w", s.Layout.Name(), err)
	}
	f := s.file()
	return &library.TorrentAssignment{
		Kind:      library.AssignmentKindTorrent,
		InfoHash:  m.InfoHash.HexString(),
		MagnetURI: s.Magnet,
		FilePath:  f.Path,
		FileSize:  f.Size,
		IsActive:  true,
	}, nil
}

// Movie is a movie of the lab's library: /Movies/Title (Year)/.
type Movie struct {
	Title  string
	Year   int
	Source Source
}

// Dir is the movie's folder, the one Infuse lists.
func (m Movie) Dir() string { return vfs.MovieDir(m.Title, m.Year) }

// Path is the movie's video file.
func (m Movie) Path() string { return vfs.MovieFilePath(m.Title, m.Year, m.Source.filePath()) }

// Episode is an episode of the lab's library: /TV Shows/Show (Year)/Season
// NN/.
type Episode struct {
	Show    string
	Year    int
	Season  int
	Episode int
	Name    string
	Source  Source
}

// Dir is the episode's season folder.
func (e Episode) Dir() string { return vfs.SeasonDir(e.Show, e.Year, e.Season) }

// Path is the episode's video file.
func (e Episode) Path() string {
	return vfs.EpisodeFilePath(e.Show, e.Year, e.Season, e.Episode, e.Name, e.Source.filePath())
}

// staticLibrary is the library as vfs.NewStaticFS takes it, and the info
// hashes of its torrents, each once, in order.
func staticLibrary(movies []Movie, episodes []Episode) ([]vfs.StaticMovie, []vfs.StaticEpisode, []string, error) {
	var (
		sm     []vfs.StaticMovie
		se     []vfs.StaticEpisode
		hashes []string
		seen   = map[string]bool{}
	)
	add := func(a *library.TorrentAssignment) {
		if a.IsTorrent() && !seen[a.InfoHash] {
			seen[a.InfoHash] = true
			hashes = append(hashes, a.InfoHash)
		}
	}
	for _, m := range movies {
		a, err := m.Source.assignment()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("movie %q: %w", m.Title, err)
		}
		add(a)
		sm = append(sm, vfs.StaticMovie{Title: m.Title, Year: m.Year, Assignment: a})
	}
	for _, e := range episodes {
		a, err := e.Source.assignment()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("episode %q S%02dE%02d: %w", e.Show, e.Season, e.Episode, err)
		}
		add(a)
		se = append(se, vfs.StaticEpisode{Show: e.Show, Year: e.Year, Season: e.Season, Episode: e.Episode,
			Name: e.Name, Assignment: a})
	}
	return sm, se, hashes, nil
}
