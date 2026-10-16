package service

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// The JSON the assignment endpoints answer (PLAN §6 "Assignment";
// web/src/lib/api/contract/assignment.ts mirrors it field for field).

// JobDetail is GET /assignment-jobs/{id}: the job and its listed proposals,
// empty until it's classified.
type JobDetail struct {
	Job       JobView        `json:"job"`
	Proposals []ProposalView `json:"proposals"`
}

// JobView is AssignmentJob.
type JobView struct {
	ID             int64             `json:"id"`
	Status         library.JobStatus `json:"status"`
	DryRun         bool              `json:"dry_run"`
	Item           JobItem           `json:"item"`
	Torrent        JobTorrent        `json:"torrent"`
	Progress       JobProgress       `json:"progress"`
	Hint           *library.JobHint  `json:"hint"`
	AIAvailable    bool              `json:"ai_available"`
	AIError        *string           `json:"ai_error"`
	SuggestedHints []string          `json:"suggested_hints"`
	Counts         JobCounts         `json:"counts"`
	Error          *string           `json:"error"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	AppliedAt      *time.Time        `json:"applied_at"`
}

// JobItem is the title a job maps to. ID is its library id.
type JobItem struct {
	ID         *int64  `json:"id"`
	MediaType  string  `json:"media_type"`
	TMDBID     int     `json:"tmdb_id"`
	Title      string  `json:"title"`
	Year       *int    `json:"year"`
	PosterPath *string `json:"poster_path"`
}

// JobTorrent is the torrent; sizes and counts are null until its file
// list is in.
type JobTorrent struct {
	InfoHash     string  `json:"info_hash"`
	Name         string  `json:"name"`
	SizeBytes    *int64  `json:"size_bytes"`
	FileCount    *int    `json:"file_count"`
	VideoCount   *int    `json:"video_count"`
	ReleaseGroup *string `json:"release_group"`
	Indexer      *string `json:"indexer"`
}

// JobProgress details the progress steps; each is null until its step has
// run.
type JobProgress struct {
	Seasons  *int `json:"seasons"`
	Episodes *int `json:"episodes"`
	Files    *int `json:"files"`
	Videos   *int `json:"videos"`
	AIFiles  *int `json:"ai_files"`
}

// ProposalView is Proposal.
type ProposalView struct {
	ID               int64             `json:"id"`
	FilePath         string            `json:"file_path"`
	FileSize         int64             `json:"file_size"`
	Kind             string            `json:"kind"`
	Group            string            `json:"group"`
	ExtraType        *string           `json:"extra_type"`
	Targets          []ProposalTarget  `json:"targets"`
	SuggestedTargets []ProposalTarget  `json:"suggested_targets"`
	Confidence       *string           `json:"confidence"`
	Source           string            `json:"source"`
	Reason           string            `json:"reason"`
	Decision         string            `json:"decision"`
	UserDecided      bool              `json:"user_decided"`
	Replaces         *ProposalReplaces `json:"replaces"`
	Updated          bool              `json:"updated"`
	Hinted           bool              `json:"hinted"`
	SubtitleCount    int               `json:"subtitle_count"`
	SupersededBy     *int64            `json:"superseded_by"`
}

// ProposalTarget is an episode a file maps to.
type ProposalTarget struct {
	EpisodeID      int64  `json:"episode_id"`
	Season         int    `json:"season"`
	Episode        int    `json:"episode"`
	AbsoluteNumber *int   `json:"absolute_number"`
	Name           string `json:"name"`
}

// ProposalReplaces is the file the first target has now, from elsewhere.
type ProposalReplaces struct {
	ReleaseName string `json:"release_name"`
	FilePath    string `json:"file_path"`
}

// jobView builds a job's view. ps are its listed proposals (none before
// it's classified).
func jobView(j *library.AssignmentJob, ps []*library.AssignmentProposal, aiAvailable bool) JobView {
	v := JobView{
		ID:     j.ID,
		Status: j.Status,
		DryRun: j.DryRun,
		Item: JobItem{
			ID:         j.LibraryID(),
			MediaType:  string(j.MediaType),
			TMDBID:     j.TMDBID,
			Title:      j.Title,
			Year:       positiveInt(j.Year),
			PosterPath: nonEmpty(j.PosterPath),
		},
		Torrent: JobTorrent{
			InfoHash:     j.InfoHash,
			Name:         j.TorrentName,
			SizeBytes:    j.SizeBytes,
			FileCount:    j.FileCount,
			VideoCount:   j.VideoCount,
			ReleaseGroup: nonEmpty(j.ReleaseGroup),
			Indexer:      nonEmpty(j.Indexer),
		},
		Progress: JobProgress{
			Files:   j.FileCount,
			Videos:  j.VideoCount,
			AIFiles: j.AIFiles,
		},
		Hint:           j.Hint,
		AIAvailable:    aiAvailable,
		AIError:        nonEmpty(j.AIError),
		SuggestedHints: j.SuggestedHints,
		Counts:         countsOf(ps),
		Error:          nonEmpty(j.Error),
		CreatedAt:      j.CreatedAt.UTC(),
		UpdatedAt:      j.UpdatedAt.UTC(),
	}
	// The show's size is known once fetching_metadata has run.
	if j.MediaType == library.JobTV && j.FileCount != nil {
		seasons, episodes := j.SeasonCount, j.EpisodeCount
		v.Progress.Seasons, v.Progress.Episodes = &seasons, &episodes
	}
	if v.SuggestedHints == nil {
		v.SuggestedHints = []string{}
	}
	if j.AppliedAt != nil {
		t := j.AppliedAt.UTC()
		v.AppliedAt = &t
	}
	return v
}

// proposalViews builds the listed proposals' views, sorted for the API.
// all are the job's proposals (hidden subtitles included, for
// subtitle_count); refs has their target episodes.
func proposalViews(j *library.AssignmentJob, all []*library.AssignmentProposal, refs map[int64]*library.EpisodeRef) []ProposalView {
	subtitles := map[int64]int{}
	for _, p := range all {
		if p.LinkedProposalID != nil {
			subtitles[*p.LinkedProposalID]++
		}
	}
	out := make([]ProposalView, 0, len(all))
	for _, p := range all {
		if !p.Listed() {
			continue
		}
		v := ProposalView{
			ID:               p.ID,
			FilePath:         p.FilePath,
			FileSize:         p.FileSize,
			Kind:             string(p.Kind),
			Group:            string(p.Group),
			ExtraType:        nonEmpty(p.ExtraType),
			Targets:          targetsOf(p.EpisodeIDs, refs),
			SuggestedTargets: targetsOf(p.SuggestedEpisodeIDs, refs),
			Confidence:       nonEmpty(p.Confidence),
			Source:           p.Source,
			Reason:           p.Reason,
			Decision:         string(p.Decision),
			UserDecided:      p.UserDecided,
			Updated:          p.Updated,
			Hinted:           p.Hinted,
			SubtitleCount:    subtitles[p.ID],
			SupersededBy:     p.SupersededBy,
			Replaces:         proposalReplaces(j, p, refs),
		}
		out = append(out, v)
	}
	sortProposals(out)
	return out
}

// targetsOf resolves episode ids; one that isn't the show's any more (a
// refresh removed it) is left out.
func targetsOf(ids []int64, refs map[int64]*library.EpisodeRef) []ProposalTarget {
	out := make([]ProposalTarget, 0, len(ids))
	for _, id := range ids {
		ref, ok := refs[id]
		if !ok {
			continue
		}
		out = append(out, ProposalTarget{
			EpisodeID:      ref.ID,
			Season:         ref.Season,
			Episode:        ref.Number,
			AbsoluteNumber: ref.AbsoluteNumber,
			Name:           ref.Name,
		})
	}
	return out
}

// proposalReplaces is a proposal's `replaces`: what its first target has
// now (replacesOf); nil without a target. Auto-apply waits on it too.
func proposalReplaces(j *library.AssignmentJob, p *library.AssignmentProposal, refs map[int64]*library.EpisodeRef) *ProposalReplaces {
	if len(p.EpisodeIDs) == 0 {
		return nil
	}
	return replacesOf(j, p, refs[p.EpisodeIDs[0]])
}

// replacesOf is the file the episode has now when it isn't this one: Apply
// replaces it. It's computed when read, so it never goes stale. An HDRezka
// stream is named by its label ("HDRezka · LostFilm · 1080p") and has no
// file path.
func replacesOf(j *library.AssignmentJob, p *library.AssignmentProposal, ref *library.EpisodeRef) *ProposalReplaces {
	if ref == nil || ref.Active == nil {
		return nil
	}
	a := ref.Active
	if !a.IsTorrent() {
		r := &ProposalReplaces{}
		if a.Stream != nil {
			r.ReleaseName = a.Stream.Label()
		}
		return r
	}
	if a.InfoHash == j.InfoHash && a.FilePath == p.FilePath {
		return nil
	}
	return &ProposalReplaces{ReleaseName: subtitle.AssignedRelease(a.MagnetURI, a.FilePath), FilePath: a.FilePath}
}

// episodeIDsOf collects the target and suggested episodes of proposals.
func episodeIDsOf(ps []*library.AssignmentProposal) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	for _, p := range ps {
		for _, list := range [][]int64{p.EpisodeIDs, p.SuggestedEpisodeIDs} {
			for _, id := range list {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func positiveInt(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}
