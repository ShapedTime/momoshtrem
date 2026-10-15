package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Manual overrides (PUT /episodes/{id}/assignment and /movies/{id}/
// assignment) and a torrent's files (GET /torrents/{hash}/files). Both need
// the torrent's file list, which comes from a job that stored it, else the
// loaded torrent, else by adding the torrent again.

// fileListBudget bounds adding a torrent to read its file list during a
// request; under the web proxy's 30 s, so the user gets this 504 and not
// the proxy's. The add goes on in the background when it runs out, and
// the torrent is dropped again if nothing uses it by then.
const fileListBudget = 25 * time.Second

// Errors the torrent file endpoints map to status codes.
var (
	// ErrUnknownTorrent: no job, assignment or loaded torrent has this
	// info hash, so its files can't be listed (404).
	ErrUnknownTorrent = errors.New("unknown torrent")
	// ErrFileNotInTorrent: the torrent has no such file (404).
	ErrFileNotInTorrent = errors.New("no such file in the torrent")
	// ErrFileListTimeout: no peers sent the torrent's file list in time
	// (504).
	ErrFileListTimeout = errors.New("the torrent's file list didn't arrive in time")
)

// torrentLoader reads a loaded torrent's file list without adding it.
// torrent.Service implements it; a TorrentClient without it skips that
// source.
type torrentLoader interface {
	GetTorrent(infoHash string) (*torrent.TorrentInfo, error)
}

var _ torrentLoader = (torrent.Service)(nil)

// torrentFiles is a torrent's file list and how it was found.
type torrentFiles struct {
	name   string
	magnet string // "" when only the loaded torrent knows it
	files  []library.TorrentFile
	// subtitles are the subtitle files a job linked to each video, by the
	// video's path; nil when no job stored the file list.
	subtitles map[string][]library.TorrentFile
	added     bool // this request added the torrent to read its files
}

// fileList returns the torrent's files: the newest job's that stored them,
// else the loaded torrent's, else it adds the torrent with a magnet an
// assignment or a job has (under the torrent's lock, within
// fileListBudget). It answers ErrUnknownTorrent when nothing knows the
// torrent, ErrFileListTimeout when its file list doesn't arrive in time.
func (s *JobService) fileList(ctx context.Context, hash string) (*torrentFiles, error) {
	stored, err := s.jobs.StoredTorrent(ctx, hash)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		return &torrentFiles{name: stored.Name, magnet: stored.MagnetURI, files: stored.Files, subtitles: stored.Subtitles}, nil
	}
	if loader, ok := s.torrents.(torrentLoader); ok {
		if info, err := loader.GetTorrent(hash); err == nil && info != nil && len(info.Files) > 0 {
			return &torrentFiles{name: info.Name, files: libraryFiles(info.Files)}, nil
		}
	}
	magnet, err := s.jobs.TorrentMagnet(ctx, hash)
	if err != nil {
		return nil, err
	}
	if magnet == "" {
		return nil, ErrUnknownTorrent
	}

	addCtx, cancel := context.WithTimeout(ctx, fileListBudget)
	defer cancel()
	info, err := s.addForFileList(addCtx, magnet, hash)
	switch {
	case err == nil:
		return &torrentFiles{name: info.Name, magnet: magnet, files: libraryFiles(info.Files), added: true}, nil
	case ctx.Err() != nil:
		return nil, ctx.Err() // the client went away
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, torrent.ErrMetadataTimeout):
		return nil, ErrFileListTimeout
	case errors.Is(err, library.ErrTorrentServiceUnavailable):
		return nil, err
	}
	return nil, fmt.Errorf("failed to add the torrent: %w", err)
}

// addForFileList adds the torrent under its lock (like the worker's
// addTorrent) and waits for its file list until ctx ends. An add that
// finishes after that is dropped again when nothing uses the torrent:
// nobody is left to use the file list it was added for.
func (s *JobService) addForFileList(ctx context.Context, magnetURI, hash string) (*torrent.TorrentInfo, error) {
	if s.torrents == nil {
		return nil, library.ErrTorrentServiceUnavailable
	}
	type result struct {
		info *torrent.TorrentInfo
		err  error
	}
	client := s.torrents
	done := make(chan result, 1)
	go func() {
		unlock := s.hashLocks.Lock(hash)
		defer unlock()
		info, err := client.AddTorrent(magnetURI)
		done <- result{info, err}
	}()
	select {
	case r := <-done:
		if r.err == nil && r.info == nil {
			return nil, torrent.ErrTorrentNotFound
		}
		return r.info, r.err
	case <-ctx.Done():
		go func() {
			if r := <-done; r.err == nil {
				s.releaseTorrent(context.Background(), hash, 0)
			}
		}()
		return nil, ctx.Err()
	}
}

func libraryFiles(files []identify.TorrentFile) []library.TorrentFile {
	out := make([]library.TorrentFile, len(files))
	for i, f := range files {
		out[i] = library.TorrentFile{Path: f.Path, Size: f.Size}
	}
	return out
}

// TorrentFileView is one file of GET /torrents/{hash}/files.
type TorrentFileView struct {
	Path     string       `json:"path"`
	Size     int64        `json:"size"`
	MappedTo *FileMapping `json:"mapped_to"` // null when no active assignment uses it
}

// FileMapping is what a torrent file is assigned to: {episodes} or
// {movie_id}.
type FileMapping struct {
	Episodes []ProposalTarget `json:"episodes,omitempty"`
	MovieID  *int64           `json:"movie_id,omitempty"`
}

// TorrentFiles lists a torrent's files, in its order, each with what its
// active assignments map it to: a movie, else its episodes in order. A
// torrent added just to list its files is removed again when nothing uses
// it. Errors: ErrUnknownTorrent, ErrFileListTimeout, and
// library.ErrTorrentServiceUnavailable when it would have to be added
// without a torrent client.
func (s *JobService) TorrentFiles(ctx context.Context, infoHash string) ([]TorrentFileView, error) {
	hash := normalizeInfoHash(infoHash)
	if hash == "" {
		return nil, ErrUnknownTorrent
	}
	src, err := s.fileList(ctx, hash)
	if err != nil {
		return nil, err
	}
	if src.added {
		defer s.releaseTorrent(context.WithoutCancel(ctx), hash, 0)
	}
	mappings, err := s.jobs.ActiveMappings(ctx, hash)
	if err != nil {
		return nil, err
	}
	byPath := map[string]*FileMapping{}
	for _, m := range mappings {
		fm := byPath[m.FilePath]
		if fm == nil {
			fm = &FileMapping{}
			byPath[m.FilePath] = fm
		}
		switch {
		case m.MovieID != nil:
			fm.MovieID, fm.Episodes = m.MovieID, nil
		case m.Episode != nil && fm.MovieID == nil:
			e := m.Episode
			fm.Episodes = append(fm.Episodes, ProposalTarget{
				EpisodeID: e.ID, Season: e.Season, Episode: e.Number, AbsoluteNumber: e.AbsoluteNumber, Name: e.Name,
			})
		}
	}
	out := make([]TorrentFileView, 0, len(src.files))
	for _, f := range src.files {
		out = append(out, TorrentFileView{Path: f.Path, Size: f.Size, MappedTo: byPath[f.Path]})
	}
	return out, nil
}

// OverrideEpisode makes a file of a torrent the episode's file: a manual
// assignment (match_source manual, no confidence) written like Apply's
// rows, then the WebDAV tree is rebuilt. The torrent's other rows on the
// show stay. Errors: library.ErrEpisodeNotFound, ErrUnknownTorrent,
// ErrFileNotInTorrent, a ValidationError for a file that isn't a video,
// ErrFileListTimeout.
func (s *JobService) OverrideEpisode(ctx context.Context, episodeID int64, infoHash, filePath string) error {
	showID, err := s.shows.EpisodeShowID(ctx, episodeID)
	if err != nil {
		return err
	}
	return s.override(ctx, library.AssignmentWrite{ItemType: library.ItemTypeEpisode, ShowID: showID}, episodeID, infoHash, filePath)
}

// OverrideMovie makes a file of a torrent the movie's file, like
// OverrideEpisode. Errors: library.ErrMovieNotFound and OverrideEpisode's.
func (s *JobService) OverrideMovie(ctx context.Context, movieID int64, infoHash, filePath string) error {
	movie, err := s.movies.GetByID(movieID)
	if err != nil {
		return err
	}
	if movie == nil {
		return library.ErrMovieNotFound
	}
	return s.override(ctx, library.AssignmentWrite{ItemType: library.ItemTypeMovie, MovieID: movieID}, movieID, infoHash, filePath)
}

// override checks the file is in the torrent, writes w's one row for
// itemID (which drops the item's torrent subtitles when its file changes),
// records the subtitles a job linked to the new file and rebuilds the
// tree. A torrent added only for this is dropped again when the override
// fails and nothing else uses it.
func (s *JobService) override(ctx context.Context, w library.AssignmentWrite, itemID int64, infoHash, filePath string) (err error) {
	hash := normalizeInfoHash(infoHash)
	if hash == "" {
		return ErrUnknownTorrent
	}
	if identify.ClassOf(filePath) != identify.ClassVideo {
		return invalid("Pick a video file: %q isn't one.", filePath)
	}
	src, err := s.fileList(ctx, hash)
	if err != nil {
		return err
	}
	if src.added {
		defer func() {
			if err != nil {
				s.releaseTorrent(context.WithoutCancel(ctx), hash, 0)
			}
		}()
	}
	var file *library.TorrentFile
	for i := range src.files {
		if src.files[i].Path == filePath {
			file = &src.files[i]
			break
		}
	}
	if file == nil {
		return ErrFileNotInTorrent
	}

	magnet := src.magnet
	if magnet == "" {
		if magnet, err = s.jobs.TorrentMagnet(ctx, hash); err != nil {
			return err
		}
	}
	if magnet == "" {
		magnet = "magnet:?xt=urn:btih:" + hash
		if src.name != "" {
			magnet += "&dn=" + url.QueryEscape(src.name)
		}
	}
	quality := identify.ParseQuality(file.Path)
	w.InfoHash, w.MagnetURI = hash, magnet
	w.Rows = []library.TorrentAssignment{{
		ItemID: itemID, FilePath: file.Path, FileSize: file.Size,
		Resolution: quality.Resolution, Source: quality.Source, MatchSource: library.SourceManual,
	}}
	if err := s.assignments.WriteOverride(ctx, w); err != nil {
		return err
	}
	s.log.Info("Manual assignment", "item_type", w.ItemType, "item_id", itemID, "info_hash", hash, "file_path", file.Path)

	// Committed: the rest outlives a cancelled request, like afterApply.
	ctx = context.WithoutCancel(ctx)
	if s.subtitles != nil {
		itemType := subtitle.ItemTypeEpisode
		if w.ItemType == library.ItemTypeMovie {
			itemType = subtitle.ItemTypeMovie
		}
		for _, f := range src.subtitles[file.Path] {
			if err := s.subtitles.CreateTorrentSubtitle(ctx, newTorrentSubtitle(hash, magnet, f, itemType, itemID)); err != nil {
				s.log.Error("Failed to record a torrent subtitle", "item_id", itemID, "file_path", f.Path, "error", err)
			}
		}
	}
	if s.tree != nil {
		s.tree.InvalidateTree()
	}
	return nil
}
