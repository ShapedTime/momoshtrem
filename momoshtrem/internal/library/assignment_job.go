package library

import (
	"errors"
	"time"
)

// ErrJobNotFound is returned for an assignment job that doesn't exist (or
// went with its title).
var ErrJobNotFound = errors.New("assignment job not found")

// JobStatus is where an assignment job is (PLAN §6 "Assignment"):
// queued → fetching_metadata → classifying → [resolving] → needs_review ⇄
// ready → applied; any open status can end in failed or cancelled.
type JobStatus string

const (
	JobQueued           JobStatus = "queued"
	JobFetchingMetadata JobStatus = "fetching_metadata"
	JobClassifying      JobStatus = "classifying"
	JobResolving        JobStatus = "resolving"
	JobNeedsReview      JobStatus = "needs_review"
	JobReady            JobStatus = "ready"
	JobApplied          JobStatus = "applied"
	JobFailed           JobStatus = "failed"
	JobCancelled        JobStatus = "cancelled"
)

// Job status sets: working (the worker has it), open (working or waiting
// for review) and closed.
var (
	WorkingJobStatuses = []JobStatus{JobQueued, JobFetchingMetadata, JobClassifying, JobResolving}
	OpenJobStatuses    = []JobStatus{JobQueued, JobFetchingMetadata, JobClassifying, JobResolving, JobNeedsReview, JobReady}
	AllJobStatuses     = []JobStatus{JobQueued, JobFetchingMetadata, JobClassifying, JobResolving,
		JobNeedsReview, JobReady, JobApplied, JobFailed, JobCancelled}
)

// Valid reports whether s is a known status.
func (s JobStatus) Valid() bool { return s.in(AllJobStatuses) }

// Working reports whether the worker still has the job (queued…resolving).
func (s JobStatus) Working() bool { return s.in(WorkingJobStatuses) }

// Open reports whether the job is working or waiting for review.
func (s JobStatus) Open() bool { return s.in(OpenJobStatuses) }

// Classified reports whether the job's proposals exist: from resolving
// on (a job cancelled before that has none).
func (s JobStatus) Classified() bool {
	return s != JobQueued && s != JobFetchingMetadata && s != JobClassifying
}

func (s JobStatus) in(set []JobStatus) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

// JobMediaType is what an assignment job maps a torrent to.
type JobMediaType string

const (
	JobMovie JobMediaType = "movie"
	JobTV    JobMediaType = "tv"
)

// JobHint is what the user said about a torrent: the season bare numbers
// belong to, the episode of a single file, and free text for the AI.
type JobHint struct {
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
	Text    string `json:"text,omitempty"`
}

// AssignmentJob is a torrent being mapped to a title's files.
type AssignmentJob struct {
	ID        int64
	MediaType JobMediaType
	TMDBID    int
	ShowID    *int64 // tv; nil for a dry run on a title not in the library
	MovieID   *int64 // movie; likewise

	InfoHash     string
	MagnetURI    string
	TorrentName  string // the magnet's dn (or the hash) until the file list arrives
	Indexer      string
	ReleaseGroup string
	SizeBytes    *int64 // nil until the file list arrives
	FileCount    *int
	VideoCount   *int   // video files that aren't extras
	FilePath     string // movies: the file the request picked up front

	Status         JobStatus
	DryRun         bool
	Hint           *JobHint
	SuggestedHints []string
	AIFiles        *int
	Error          string
	AIError        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AppliedAt      *time.Time

	// Loaded with the job from its title.
	Title          string
	Year           int
	PosterPath     string
	ClassifierMode ClassifierMode // shows
	SeasonCount    int            // shows: seasons, Specials included
	EpisodeCount   int            // shows: episodes, Specials included
}

// LibraryID is the movie's or show's library id; nil for a dry run on a
// title that isn't in the library.
func (j *AssignmentJob) LibraryID() *int64 {
	if j.MediaType == JobTV {
		return j.ShowID
	}
	return j.MovieID
}

// ProposalKind is what a file is.
type ProposalKind string

const (
	KindEpisode  ProposalKind = "episode"
	KindMovie    ProposalKind = "movie"
	KindExtra    ProposalKind = "extra"
	KindSubtitle ProposalKind = "subtitle"
)

// ProposalGroup is a proposal's review group, set at classification.
type ProposalGroup string

const (
	GroupMatched     ProposalGroup = "matched"
	GroupNeedsReview ProposalGroup = "needs_review"
	GroupSpecials    ProposalGroup = "specials"
	GroupExtras      ProposalGroup = "extras"
	GroupUnmatched   ProposalGroup = "unmatched"
)

// DefaultDecision is the decision a group starts with: matched and
// specials accepted, extras ignored, the others pending.
func (g ProposalGroup) DefaultDecision() Decision {
	switch g {
	case GroupMatched, GroupSpecials:
		return DecisionAccepted
	case GroupExtras:
		return DecisionIgnored
	}
	return DecisionPending
}

// Decision is what happens to a proposal on Apply.
type Decision string

const (
	DecisionPending  Decision = "pending"
	DecisionAccepted Decision = "accepted"
	DecisionIgnored  Decision = "ignored"
)

// Who decided a proposal (and an assignment's match_source).
const (
	SourceRules  = "rules"
	SourceAI     = "ai"
	SourceManual = "manual"
)

// AssignmentProposal is the job's answer for one file of the torrent.
type AssignmentProposal struct {
	ID        int64
	JobID     int64
	FilePath  string
	FileSize  int64
	Kind      ProposalKind
	Group     ProposalGroup
	ExtraType string // extras only

	EpisodeIDs []int64 // targets, in order
	Confidence string  // high, medium, low; "" with no target and for manual picks
	Source     string
	Reason     string

	SuggestedEpisodeIDs []int64
	SuggestedConfidence string
	SuggestedSource     string
	SuggestedReason     string

	Decision    Decision
	UserDecided bool
	Updated     bool
	Hinted      bool

	SupersededBy     *int64
	LinkedProposalID *int64 // a subtitle's video: it isn't listed

	// FirstSeason is the season of EpisodeIDs[0], loaded with the proposal;
	// nil with no target.
	FirstSeason *int
}

// Maps reports whether Apply would write the proposal: it has targets, or
// it's a movie candidate.
func (p *AssignmentProposal) Maps() bool {
	return p.Kind == KindMovie || len(p.EpisodeIDs) > 0
}

// Listed reports whether the API lists the proposal: subtitles that belong
// to a video are counted in its subtitle_count instead.
func (p *AssignmentProposal) Listed() bool { return p.LinkedProposalID == nil }

// ProposalDraft is a proposal before it's stored: links name files by path.
type ProposalDraft struct {
	FilePath   string
	FileSize   int64
	Kind       ProposalKind
	Group      ProposalGroup
	ExtraType  string
	EpisodeIDs []int64
	Confidence string
	Source     string
	Reason     string
	Decision   Decision
	// LinkedPath is the video a subtitle belongs to; SupersededByPath the
	// newer version that replaces this file.
	LinkedPath       string
	SupersededByPath string
}

// EpisodeRef is an episode a proposal can target, with its active
// assignment (for `replaces`).
type EpisodeRef struct {
	ID             int64
	ShowID         int64
	Season         int
	Number         int
	AbsoluteNumber *int
	Name           string
	Active         *TorrentAssignment // nil when the episode has no file
}
