package service

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// TorrentClient is what the jobs need from the torrent client: the file
// list (adding the torrent) and dropping it when a discarded job leaves it
// unused.
type TorrentClient interface {
	AddTorrent(magnetURI string) (*torrent.TorrentInfo, error)
	RemoveTorrent(infoHash string, deleteData bool) error
}

var _ TorrentClient = (torrent.Service)(nil)

// TreeInvalidator rebuilds the WebDAV tree after Apply.
type TreeInvalidator interface {
	InvalidateTree()
}

// SubtitleCreator records the subtitles that came with a torrent.
type SubtitleCreator interface {
	CreateTorrentSubtitle(ctx context.Context, sub *subtitle.Subtitle) error
}

var _ SubtitleCreator = (*subtitle.Service)(nil)

// ShowRefresher refreshes a show's metadata from TMDB.
type ShowRefresher interface {
	RefreshShow(ctx context.Context, showID int64) (*metadata.ShowResult, error)
}

var _ ShowRefresher = (*metadata.Syncer)(nil)

// CatalogSource gives the classifier a show's alternative titles and
// episode groups. *tmdb.Cached implements it.
type CatalogSource interface {
	ShowAlternativeTitles(ctx context.Context, showID int) ([]tmdb.AlternativeTitle, error)
	EpisodeGroups(ctx context.Context, showID int) ([]tmdb.EpisodeGroupSummary, error)
	EpisodeGroup(ctx context.Context, id string) (*tmdb.EpisodeGroup, error)
}

var _ CatalogSource = (*tmdb.Cached)(nil)

// Errors the API maps to status codes.
var (
	// ErrInvalidStatus is an unknown status in a job list filter (400).
	ErrInvalidStatus = errors.New("unknown job status")
	// ErrInvalidHint is a hint with a negative season or episode, or text
	// that's too long (400).
	ErrInvalidHint = errors.New("invalid hint")
)

// 409 codes, in the order apply checks them.
const (
	CodeDryRun               = "dry_run"
	CodeJobClosed            = "job_closed"
	CodeJobWorking           = "job_working"
	CodeConflictingProposals = "conflicting_proposals"
	CodePendingProposals     = "pending_proposals"
)

// JobConflictError is a 409: the job can't do that now. Code tells why.
type JobConflictError struct {
	Code    string
	Message string
}

func (e *JobConflictError) Error() string { return e.Message }

func conflict(code, message string) error { return &JobConflictError{Code: code, Message: message} }

// JobFailedError is a legacy assign (?wait=true) whose job failed.
type JobFailedError struct {
	JobID   int64
	Message string
	Timeout bool // no peers sent the file list in time
}

func (e *JobFailedError) Error() string { return e.Message }

// Job failure messages, shown to the user.
const (
	msgMetadataTimeout = "No peers sent the torrent's file list in time."
	msgNoVideos        = "This torrent has no video files."
	msgNoTorrents      = "Torrents aren't available, so the file list can't be fetched."
	msgTitleGone       = "The title isn't in the library any more."
)

const (
	// maxHintText caps the free-text hint.
	maxHintText = 500
	// listLimit caps GET /assignment-jobs.
	listLimit = 200
	// defaultWorkers is how many jobs run at once.
	defaultWorkers = 2
)

// JobDeps are a JobService's dependencies. Torrents, Tree, Subtitles,
// Metadata, TMDB and Resolver are optional.
type JobDeps struct {
	Jobs        *library.AssignmentJobRepository
	Shows       *library.ShowRepository
	Movies      *library.MovieRepository
	Assignments *library.AssignmentRepository
	Settings    *library.SettingsRepository
	// Torrents fetches file lists. Without it, jobs fail.
	Torrents TorrentClient
	// Tree is rebuilt after Apply.
	Tree TreeInvalidator
	// Subtitles records the torrent's subtitles on Apply.
	Subtitles SubtitleCreator
	// Metadata refreshes a show whose metadata is stale before matching.
	Metadata ShowRefresher
	// TMDB gives the classifier aliases and alternative orderings.
	TMDB CatalogSource
	// Resolver is the AI resolver. Nil or identify.NoopResolver means AI
	// isn't available.
	Resolver identify.Resolver
	// Workers is how many jobs run at once (2 when 0).
	Workers int
}

// JobService runs assignment jobs (PLAN §6 "Assignment"): assigning a
// torrent creates a job, a worker fetches its file list and classifies
// every file, and the job waits for review or applies on its own. Apply is
// the only writer of assignments.
type JobService struct {
	jobs        *library.AssignmentJobRepository
	shows       *library.ShowRepository
	movies      *library.MovieRepository
	assignments *library.AssignmentRepository
	settings    *library.SettingsRepository
	torrents    TorrentClient
	tree        TreeInvalidator
	subtitles   SubtitleCreator
	refresher   ShowRefresher
	tmdb        CatalogSource
	resolver    identify.Resolver
	workers     int
	now         func() time.Time
	log         *slog.Logger

	// The worker pool (assignment_worker.go).
	wake      chan struct{}
	stop      context.CancelFunc
	wg        sync.WaitGroup
	hashLocks keyedMutex
	// pollInterval is how often a waiting legacy request re-reads its job.
	pollInterval time.Duration
}

// NewJobService creates a JobService. Call Start to run its workers.
func NewJobService(d JobDeps) *JobService {
	workers := d.Workers
	if workers <= 0 {
		workers = defaultWorkers
	}
	return &JobService{
		jobs:         d.Jobs,
		shows:        d.Shows,
		movies:       d.Movies,
		assignments:  d.Assignments,
		settings:     d.Settings,
		torrents:     d.Torrents,
		tree:         d.Tree,
		subtitles:    d.Subtitles,
		refresher:    d.Metadata,
		tmdb:         d.TMDB,
		resolver:     d.Resolver,
		workers:      workers,
		now:          time.Now,
		log:          slog.With("component", "assignment-jobs"),
		wake:         make(chan struct{}, 1),
		pollInterval: 200 * time.Millisecond,
	}
}

// AssignRequest is POST /shows/{id}/assign-torrent and
// /movies/{id}/assign-torrent.
type AssignRequest struct {
	MagnetURI string
	Hint      *library.JobHint // shows
	Indexer   string
	FilePath  string // movies: the file to use, picked up front
}

// Assign creates a job for the torrent on a library title and wakes a
// worker. When the title already has an open job for the torrent it
// returns that one, with created false (the new hint is ignored).
func (s *JobService) Assign(ctx context.Context, mediaType library.JobMediaType, libraryID int64, req AssignRequest) (jobID int64, created bool, err error) {
	hash := normalizeInfoHash(torrent.ExtractInfoHash(req.MagnetURI))
	if hash == "" {
		return 0, false, library.ErrInvalidMagnet
	}
	if err := validateHint(req.Hint); err != nil {
		return 0, false, err
	}
	job := &library.AssignmentJob{
		MediaType:   mediaType,
		InfoHash:    hash,
		MagnetURI:   req.MagnetURI,
		TorrentName: magnetName(req.MagnetURI, hash),
		Indexer:     strings.TrimSpace(req.Indexer),
	}
	switch mediaType {
	case library.JobTV:
		show, err := s.shows.GetByID(libraryID)
		if err != nil {
			return 0, false, err
		}
		if show == nil {
			return 0, false, library.ErrShowNotFound
		}
		job.TMDBID, job.ShowID, job.Hint = show.TMDBID, &show.ID, normalizeHint(req.Hint)
	case library.JobMovie:
		movie, err := s.movies.GetByID(libraryID)
		if err != nil {
			return 0, false, err
		}
		if movie == nil {
			return 0, false, library.ErrMovieNotFound
		}
		job.TMDBID, job.MovieID, job.FilePath = movie.TMDBID, &movie.ID, strings.TrimSpace(req.FilePath)
	default:
		return 0, false, fmt.Errorf("unknown media type %q", mediaType)
	}

	created, err = s.jobs.Create(ctx, job)
	if err != nil {
		return 0, false, err
	}
	if created {
		s.log.Info("Assignment job queued", "job_id", job.ID, "media_type", mediaType, "library_id", libraryID, "info_hash", hash)
		s.notify()
	}
	return job.ID, created, nil
}

// Detail returns a job with its listed proposals (none before it's
// classified). Three queries, besides the settings.
func (s *JobService) Detail(ctx context.Context, id int64) (*JobDetail, error) {
	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var all []*library.AssignmentProposal
	if job.Status.Classified() {
		byJob, err := s.jobs.Proposals(ctx, []int64{id})
		if err != nil {
			return nil, err
		}
		all = byJob[id]
	}
	refs := map[int64]*library.EpisodeRef{}
	if job.ShowID != nil {
		if refs, err = s.jobs.EpisodeRefs(ctx, *job.ShowID, episodeIDsOf(all)); err != nil {
			return nil, err
		}
	}
	settings := s.aiSettings(ctx)
	return &JobDetail{
		Job:       jobView(job, listed(all), s.aiAvailable(settings, job)),
		Proposals: proposalViews(job, all, refs),
	}, nil
}

// List returns the jobs in statuses (the open ones when empty), newest
// first, without dry runs. Two queries, besides the settings.
func (s *JobService) List(ctx context.Context, statuses []library.JobStatus) ([]JobView, error) {
	if len(statuses) == 0 {
		statuses = library.OpenJobStatuses
	}
	for _, st := range statuses {
		if !st.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, st)
		}
	}
	jobs, err := s.jobs.List(ctx, statuses, listLimit)
	if err != nil {
		return nil, err
	}
	var classified []int64
	for _, j := range jobs {
		if j.Status.Classified() {
			classified = append(classified, j.ID)
		}
	}
	proposals, err := s.jobs.Proposals(ctx, classified)
	if err != nil {
		return nil, err
	}
	settings := s.aiSettings(ctx)
	out := make([]JobView, len(jobs))
	for i, j := range jobs {
		out[i] = jobView(j, listed(proposals[j.ID]), s.aiAvailable(settings, j))
	}
	return out, nil
}

// ParseStatuses reads a job list filter: "open" (or empty) for the open
// statuses, else a comma-separated list.
func ParseStatuses(filter string) ([]library.JobStatus, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == "open" {
		return library.OpenJobStatuses, nil
	}
	var out []library.JobStatus
	for _, part := range strings.Split(filter, ",") {
		st := library.JobStatus(strings.TrimSpace(part))
		if !st.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, part)
		}
		out = append(out, st)
	}
	return out, nil
}

// Apply applies a reviewed job and returns it, applied. It answers a
// JobConflictError when the job is a dry run, closed, still working, or
// has conflicting or pending proposals (in that order).
func (s *JobService) Apply(ctx context.Context, id int64) (*JobDetail, error) {
	if _, err := s.applyJob(ctx, id, applyReview); err != nil {
		return nil, err
	}
	return s.Detail(ctx, id)
}

// Discard cancels an open job (a cancelled one stays cancelled) and drops
// the torrent when nothing else uses it: no active assignment and no other
// open job. An applied or failed job answers job_closed.
func (s *JobService) Discard(ctx context.Context, id int64) (*JobDetail, error) {
	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status != library.JobCancelled {
		ok, err := s.jobs.Cancel(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			if job, err = s.jobs.Get(ctx, id); err != nil {
				return nil, err
			}
			if job.Status != library.JobCancelled {
				return nil, conflict(CodeJobClosed, fmt.Sprintf("This mapping job is already %s.", job.Status))
			}
		} else {
			s.log.Info("Assignment job discarded", "job_id", id)
			s.releaseTorrent(ctx, job.InfoHash, id)
		}
	}
	return s.Detail(ctx, id)
}

// releaseTorrent drops the torrent when nothing uses it any more.
func (s *JobService) releaseTorrent(ctx context.Context, infoHash string, jobID int64) {
	if s.torrents == nil {
		return
	}
	used, err := s.jobs.TorrentInUse(ctx, infoHash, jobID)
	if err != nil {
		s.log.Warn("Couldn't check whether the torrent is in use; keeping it", "info_hash", infoHash, "error", err)
		return
	}
	if used {
		return
	}
	if err := s.torrents.RemoveTorrent(infoHash, false); err != nil && !errors.Is(err, torrent.ErrTorrentNotFound) {
		s.log.Warn("Failed to remove an unused torrent", "info_hash", infoHash, "error", err)
	}
}

// aiSettings reads the AI settings; on a database error it logs and uses
// the defaults.
func (s *JobService) aiSettings(ctx context.Context) library.AISettings {
	if s.settings == nil {
		return library.DefaultAISettings()
	}
	settings, err := s.settings.AISettings(ctx)
	if err != nil {
		s.log.Warn("Failed to read the AI settings; using the defaults", "error", err)
		return library.DefaultAISettings()
	}
	return settings
}

// resolverConfigured reports whether a real AI resolver is wired in.
func (s *JobService) resolverConfigured() bool {
	if s.resolver == nil {
		return false
	}
	_, noop := s.resolver.(identify.NoopResolver)
	return !noop
}

// aiAvailable reports whether the AI may look at the job's files: a
// resolver is configured, the classifier is enabled, and the show doesn't
// use rules only.
func (s *JobService) aiAvailable(settings library.AISettings, job *library.AssignmentJob) bool {
	if !s.resolverConfigured() || !settings.ClassifierEnabled {
		return false
	}
	return job.MediaType != library.JobTV || job.ClassifierMode != library.ClassifierRulesOnly
}

// normalizeInfoHash lowercases a hex info hash and converts a base32 one
// (32 characters, used by some magnets) to hex, the form the torrent
// client keys torrents by. It returns "" for anything else.
func normalizeInfoHash(hash string) string {
	switch len(hash) {
	case 40:
		if _, err := hex.DecodeString(hash); err == nil {
			return strings.ToLower(hash)
		}
	case 32:
		if raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash)); err == nil && len(raw) == 20 {
			return hex.EncodeToString(raw)
		}
	}
	return ""
}

// magnetName is the magnet's display name (dn), else the hash: the job's
// torrent name until the file list arrives.
func magnetName(magnetURI, hash string) string {
	if u, err := url.Parse(magnetURI); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			return dn
		}
	}
	return hash
}

func validateHint(h *library.JobHint) error {
	if h == nil {
		return nil
	}
	switch {
	case h.Season != nil && *h.Season < 0:
		return fmt.Errorf("%w: the season can't be negative", ErrInvalidHint)
	case h.Episode != nil && *h.Episode < 1:
		return fmt.Errorf("%w: episodes start at 1", ErrInvalidHint)
	case len([]rune(h.Text)) > maxHintText:
		return fmt.Errorf("%w: keep the text under %d characters", ErrInvalidHint, maxHintText)
	}
	return nil
}

// normalizeHint trims the text and drops an empty hint.
func normalizeHint(h *library.JobHint) *library.JobHint {
	if h == nil {
		return nil
	}
	out := *h
	out.Text = strings.TrimSpace(out.Text)
	if out.Season == nil && out.Episode == nil && out.Text == "" {
		return nil
	}
	return &out
}

// identifyHint is the classifier's view of a job's hint.
func identifyHint(h *library.JobHint) *identify.Hint {
	if h == nil {
		return nil
	}
	return &identify.Hint{Season: h.Season, Episode: h.Episode, Text: h.Text}
}
