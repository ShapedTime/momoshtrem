package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Inspect (PLAN §6 "Search" › inspect): a dry run previews how a torrent's
// files would map before it's added. A library title runs the normal
// pipeline (metadata refresh and AI included); a title outside the library
// is read from TMDB, classified by the rules alone, and its targets carry
// TMDB episode ids. Dry runs are read-only, never applied, and deleted
// after dryRunTTL with their torrent unless something else uses it.

// inspectReuse: inspecting the same title and torrent again within this
// long answers the earlier dry run.
const inspectReuse = time.Hour

var (
	// ErrInvalidInspect is an Inspect request without a known media type
	// or a TMDB id (400).
	ErrInvalidInspect = errors.New("invalid inspect request")
	// ErrReleaseNotFound is a search result's release id that isn't
	// indexed (any more): search again (404). A release resolver answers
	// it.
	ErrReleaseNotFound = errors.New("release not found")
)

// errNoTMDB: without a TMDB client, titles outside the library can't be
// read.
var errNoTMDB = fmt.Errorf("%w: %w", library.ErrTMDBFetch, tmdb.ErrNoAPIKey)

// InspectRequest is POST /search/torrents/inspect, its release resolved to
// a magnet.
type InspectRequest struct {
	MediaType library.JobMediaType
	TMDBID    int
	MagnetURI string
	Indexer   string           // the release's indexer; "" for a pasted magnet
	Hint      *library.JobHint // shows: the search's season and episode
}

// Validate checks what needs no network: the media type, the TMDB id and
// the hint (ErrInvalidInspect, ErrInvalidHint).
func (r InspectRequest) Validate() error {
	switch {
	case r.MediaType != library.JobMovie && r.MediaType != library.JobTV:
		return fmt.Errorf(`%w: media_type is "movie" or "tv"`, ErrInvalidInspect)
	case r.TMDBID <= 0:
		return fmt.Errorf("%w: tmdb_id must be a positive number", ErrInvalidInspect)
	}
	return validateHint(r.Hint)
}

// Inspect creates a dry run for the torrent on a title, found in the
// library by its TMDB id or else read from TMDB, and wakes a worker. When
// the title, in the same library state, had the torrent inspected within
// inspectReuse by a dry run that wasn't cancelled and didn't fail, it
// returns that one with created false (the new hint is ignored).
//
// Errors: ErrInvalidInspect, ErrInvalidHint, library.ErrInvalidMagnet (no
// BitTorrent info hash), ErrTitleNotFound (TMDB doesn't know a title
// that isn't in the library) and library.ErrTMDBFetch (TMDB failed).
func (s *JobService) Inspect(ctx context.Context, req InspectRequest) (jobID int64, created bool, err error) {
	if err := req.Validate(); err != nil {
		return 0, false, err
	}
	hash := normalizeInfoHash(torrent.ExtractInfoHash(req.MagnetURI))
	if hash == "" {
		return 0, false, library.ErrInvalidMagnet
	}
	job := &library.AssignmentJob{
		MediaType:   req.MediaType,
		TMDBID:      req.TMDBID,
		InfoHash:    hash,
		MagnetURI:   req.MagnetURI,
		TorrentName: magnetName(req.MagnetURI, hash),
		Indexer:     strings.TrimSpace(req.Indexer),
		DryRun:      true,
	}
	switch req.MediaType {
	case library.JobTV:
		job.Hint = normalizeHint(req.Hint)
		show, err := s.shows.GetByTMDBID(req.TMDBID)
		if err != nil {
			return 0, false, err
		}
		if show != nil {
			job.ShowID = &show.ID
		}
	case library.JobMovie:
		movie, err := s.movies.GetByTMDBID(req.TMDBID)
		if err != nil {
			return 0, false, err
		}
		if movie != nil {
			job.MovieID = &movie.ID
		}
	}
	if job.LibraryID() == nil {
		if _, err := s.tmdbTitle(ctx, req.MediaType, req.TMDBID); err != nil {
			return 0, false, err
		}
	}

	created, err = s.jobs.CreateDryRun(ctx, job, inspectReuse)
	if err != nil {
		return 0, false, err
	}
	if created {
		s.log.Info("Dry run queued", "job_id", job.ID, "media_type", job.MediaType, "tmdb_id", job.TMDBID,
			"in_library", job.LibraryID() != nil, "info_hash", hash)
		s.notify()
	}
	return job.ID, created, nil
}

// outsideLibrary reports a dry run on a title that isn't in the library.
func outsideLibrary(job *library.AssignmentJob) bool {
	return job.DryRun && job.LibraryID() == nil
}

// processTMDBShow runs a dry run on a show outside the library: the files
// are classified against the show's TMDB catalog (TMDB episode ids), read
// while the file list loads, by the rules alone. Nothing is refreshed, the
// AI isn't asked and nothing is applied.
func (s *JobService) processTMDBShow(ctx context.Context, job *library.AssignmentJob) error {
	var (
		wg      sync.WaitGroup
		show    *TMDBShow
		tmdbErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		show, tmdbErr = s.tmdbShow(ctx, job.TMDBID)
	}()
	info, addErr := s.addTorrent(ctx, job.MagnetURI, job.InfoHash)
	wg.Wait()
	if addErr != nil {
		return s.addFailed(ctx, job, addErr)
	}
	if tmdbErr != nil {
		return s.tmdbFailed(ctx, job, tmdbErr)
	}
	if !hasVideo(info.Files) {
		return s.fail(ctx, job, failNoVideos)
	}

	res := identify.Classify(identify.Input{
		TorrentName: info.Name,
		Files:       info.Files,
		Show:        show.Catalog,
		Hint:        identifyHint(job.Hint),
		Now:         s.now(),
	})
	drafts := showDrafts(res.Proposals)
	if err := s.setFileList(ctx, job, info, drafts); err != nil {
		return err
	}
	err := s.jobs.Locked(ctx, job.ID, func(t *library.JobTx) error {
		if t.Job.Status != library.JobClassifying {
			return errJobMoved
		}
		if err := t.ReplaceProposals(drafts); err != nil {
			return err
		}
		t.Job.ReleaseGroup = res.Pack.Group
		t.Job.SuggestedHints = capHints(res.SuggestedHints)
		t.Job.Status = settledStatus(listed(t.Proposals))
		return t.SaveJob()
	})
	if err != nil {
		return err
	}
	s.log.Info("Dry run classified", "job_id", job.ID, "files", len(drafts))
	return nil
}

// tmdbFailed fails a dry run whose title couldn't be read from TMDB (or
// returns ctx's error on shutdown, leaving the job to recovery).
func (s *JobService) tmdbFailed(ctx context.Context, job *library.AssignmentJob, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.log.Warn("TMDB unavailable for a dry run", "job_id", job.ID, "tmdb_id", job.TMDBID, "error", err)
	return s.fail(ctx, job, failTMDB)
}

// tmdbItem is a title outside the library as TMDB has it: what its job's
// item shows, and a film's runtime for ranking its files.
type tmdbItem struct {
	Title      string
	Year       int
	PosterPath string
	Runtime    int // films: minutes; 0 when unknown
}

// tmdbTitle reads a movie's or show's details from cached TMDB, within
// refreshBudget. Errors as TMDBShowCatalog's: ErrTitleNotFound, or
// library.ErrTMDBFetch.
func (s *JobService) tmdbTitle(ctx context.Context, mediaType library.JobMediaType, tmdbID int) (*tmdbItem, error) {
	if s.tmdb == nil {
		return nil, errNoTMDB
	}
	ctx, cancel := context.WithTimeout(ctx, refreshBudget)
	defer cancel()
	if mediaType == library.JobMovie {
		d, err := s.tmdb.MovieDetails(ctx, tmdbID)
		if err != nil {
			return nil, titleTMDBError(ctx, err)
		}
		return &tmdbItem{Title: d.Title, Year: d.Year(), PosterPath: d.PosterPath, Runtime: d.Runtime}, nil
	}
	d, err := s.tmdb.ShowDetails(ctx, tmdbID)
	if err != nil {
		return nil, titleTMDBError(ctx, err)
	}
	return &tmdbItem{Title: d.Name, Year: d.Year(), PosterPath: d.PosterPath}, nil
}

// tmdbShow builds a show's catalog from cached TMDB (TMDBShowCatalog),
// within refreshBudget.
func (s *JobService) tmdbShow(ctx context.Context, tmdbID int) (*TMDBShow, error) {
	if s.tmdb == nil {
		return nil, errNoTMDB
	}
	ctx, cancel := context.WithTimeout(ctx, refreshBudget)
	defer cancel()
	return TMDBShowCatalog(ctx, s.tmdb, tmdbID)
}

// tmdbView fills in what the library gives other jobs' views, for a dry
// run outside the library: the item's title, year and poster, a show's
// size once its file list is in, and its proposals' targets (TMDB episode
// ids, without files) with the season of each first target. Until there
// are targets or a file list, a show reads only its details.
func (s *JobService) tmdbView(ctx context.Context, job *library.AssignmentJob, ps []*library.AssignmentProposal) (map[int64]*library.EpisodeRef, error) {
	refs := map[int64]*library.EpisodeRef{}
	ids := episodeIDsOf(ps)
	if job.MediaType == library.JobMovie || (job.FileCount == nil && len(ids) == 0) {
		item, err := s.tmdbTitle(ctx, job.MediaType, job.TMDBID)
		if err != nil {
			return nil, err
		}
		job.Title, job.Year, job.PosterPath = item.Title, item.Year, item.PosterPath
		return refs, nil
	}

	show, err := s.tmdbShow(ctx, job.TMDBID)
	if err != nil {
		return nil, err
	}
	job.Title, job.Year, job.PosterPath = show.Details.Name, show.Details.Year(), show.Details.PosterPath
	wanted := make(map[int64]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	seasons := map[int]bool{}
	for _, ep := range show.Catalog.Episodes {
		seasons[ep.Season] = true
		if wanted[ep.ID] {
			refs[ep.ID] = &library.EpisodeRef{
				ID: ep.ID, Season: ep.Season, Number: ep.Number, AbsoluteNumber: positiveInt(ep.Absolute), Name: ep.Name,
			}
		}
	}
	job.SeasonCount, job.EpisodeCount = len(seasons), len(show.Catalog.Episodes)
	// The stored first season comes from library episodes, which these ids
	// aren't.
	for _, p := range ps {
		p.FirstSeason = nil
		if len(p.EpisodeIDs) > 0 && refs[p.EpisodeIDs[0]] != nil {
			season := refs[p.EpisodeIDs[0]].Season
			p.FirstSeason = &season
		}
	}
	return refs, nil
}

// sweepDryRuns deletes the dry runs older than dryRunTTL and drops each
// one's torrent unless something else uses it: an active assignment or
// another open job (a newer dry run of it included).
func (s *JobService) sweepDryRuns(ctx context.Context) {
	hashes, err := s.jobs.DeleteDryRuns(ctx, dryRunTTL)
	if err != nil {
		s.log.Warn("Failed to remove old dry runs", "error", err)
		return
	}
	if len(hashes) == 0 {
		return
	}
	s.log.Info("Removed old dry runs", "jobs", len(hashes))
	released := make(map[string]bool, len(hashes))
	for _, hash := range hashes {
		if !released[hash] {
			released[hash] = true
			s.releaseTorrent(ctx, hash, 0)
		}
	}
}
