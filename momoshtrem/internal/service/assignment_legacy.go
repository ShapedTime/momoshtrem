package service

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// The legacy synchronous assign (?wait=true, until H2): it creates a job,
// waits for it to stop working and applies it the old way, so the current
// web UI keeps working. It goes through the same apply writer as review.

// legacyWaitLimit bounds how long a legacy request waits for its job.
const legacyWaitLimit = 10 * time.Minute

// AssignmentSummary contains counts of the assignment operation.
type AssignmentSummary struct {
	TotalFiles     int `json:"total_files"` // videos and subtitles
	Matched        int `json:"matched"`     // assignment rows: one per episode
	Unmatched      int `json:"unmatched"`
	Skipped        int `json:"skipped"` // extras: samples, openings, trailers, older versions...
	SubtitlesFound int `json:"subtitles_found"`
}

// MatchedAssignment represents a successful episode-to-file match.
type MatchedAssignment struct {
	EpisodeID  int64  `json:"episode_id"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Resolution string `json:"resolution"`
	Confidence string `json:"confidence"`
}

// UnmatchedAssignment represents a file that couldn't be matched. Reason is
// the classifier's (or a conflict's) explanation; Season and Episode are -1.
type UnmatchedAssignment struct {
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
}

// ShowAssignmentResult is the legacy show assign's answer.
type ShowAssignmentResult struct {
	JobID     int64
	Matched   []MatchedAssignment
	Unmatched []UnmatchedAssignment
	Summary   AssignmentSummary
}

// MovieAssignmentResult is the legacy movie assign's answer: the movie's
// active assignment after the job applied.
type MovieAssignmentResult struct {
	JobID      int64
	Assignment *library.TorrentAssignment
}

// AssignShowAndWait assigns a torrent the legacy way: every file the
// classifier gives a target is applied, whatever its confidence, except
// files in a conflict, which are reported unmatched (their proposals stay
// pending). The job ends applied.
func (s *JobService) AssignShowAndWait(ctx context.Context, showID int64, req AssignRequest) (*ShowAssignmentResult, error) {
	jobID, _, err := s.Assign(ctx, library.JobTV, showID, req)
	if err != nil {
		return nil, err
	}
	if _, err := s.waitUntilClassified(ctx, jobID); err != nil {
		return nil, err
	}

	var out *ShowAssignmentResult
	var res *applyResult
	err = s.jobs.Locked(ctx, jobID, func(t *library.JobTx) error {
		if t.Job.Status != library.JobApplied {
			if err := t.UpdateProposals(legacyAcceptShow(listed(t.Proposals))); err != nil {
				return err
			}
			var err error
			if res, err = s.applyLocked(t, applyLegacy); err != nil {
				return err
			}
		}
		refs, err := t.EpisodeRefs(showID, episodeIDsOf(t.Proposals))
		if err != nil {
			return err
		}
		out = legacyShowResult(t.Proposals, refs)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.afterApply(ctx, res)
	out.JobID = jobID
	return out, nil
}

// AssignMovieAndWait assigns a torrent to a movie the legacy way: the
// top-ranked candidate (or the file picked up front) is applied. It
// returns library.ErrNoVideoFiles when no video can be the film.
func (s *JobService) AssignMovieAndWait(ctx context.Context, movieID int64, req AssignRequest) (*MovieAssignmentResult, error) {
	jobID, _, err := s.Assign(ctx, library.JobMovie, movieID, req)
	if err != nil {
		return nil, err
	}
	job, err := s.waitUntilClassified(ctx, jobID)
	if err != nil {
		return nil, err
	}
	movie, err := s.movies.GetByID(movieID)
	if err != nil {
		return nil, err
	}
	if movie == nil {
		return nil, library.ErrMovieNotFound
	}

	var res *applyResult
	err = s.jobs.Locked(ctx, jobID, func(t *library.JobTx) error {
		if t.Job.Status == library.JobApplied {
			return nil
		}
		changed, ok := legacyAcceptMovie(t.Proposals, movie)
		if !ok {
			return library.ErrNoVideoFiles
		}
		if err := t.UpdateProposals(changed); err != nil {
			return err
		}
		var err error
		res, err = s.applyLocked(t, applyLegacy)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterApply(ctx, res)

	a, err := s.assignments.GetActiveForItem(library.ItemTypeMovie, movieID)
	if err != nil {
		return nil, err
	}
	if a == nil || a.InfoHash != job.InfoHash {
		return nil, library.ErrNoVideoFiles
	}
	return &MovieAssignmentResult{JobID: jobID, Assignment: a}, nil
}

// waitUntilClassified waits for the job to stop working. A failed job is a
// JobFailedError, a cancelled one job_closed.
func (s *JobService) waitUntilClassified(ctx context.Context, id int64) (*library.AssignmentJob, error) {
	ctx, cancel := context.WithTimeout(ctx, legacyWaitLimit)
	defer cancel()
	for {
		job, err := s.jobs.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		switch {
		case job.Status == library.JobFailed:
			return nil, &JobFailedError{JobID: id, Message: job.Error, Timeout: job.Error == msgMetadataTimeout}
		case job.Status == library.JobCancelled:
			return nil, conflict(CodeJobClosed, "This mapping job was discarded.")
		case !job.Status.Working():
			return job, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.pollInterval):
		}
	}
}

// legacyAcceptShow accepts every listed proposal with targets that isn't
// in a conflict, and returns the changed ones.
func legacyAcceptShow(ps []*library.AssignmentProposal) []*library.AssignmentProposal {
	conflicts := conflictIDs(ps)
	var changed []*library.AssignmentProposal
	for _, p := range ps {
		if len(p.EpisodeIDs) == 0 || conflicts[p.ID] || p.Decision == library.DecisionAccepted {
			continue
		}
		next := *p
		next.Decision = library.DecisionAccepted
		next.UserDecided = next.Decision != p.Group.DefaultDecision()
		changed = append(changed, &next)
	}
	return changed
}

// legacyAcceptMovie keeps an accepted film, else accepts the top-ranked
// candidate; the other candidates are ignored. ok is false when there's
// no candidate.
func legacyAcceptMovie(ps []*library.AssignmentProposal, movie *library.Movie) (changed []*library.AssignmentProposal, ok bool) {
	for _, p := range ps {
		if p.Kind == library.KindMovie && p.Decision == library.DecisionAccepted {
			return nil, true
		}
	}
	files := make([]identify.TorrentFile, len(ps))
	for i, p := range ps {
		files[i] = identify.TorrentFile{Path: p.FilePath, Size: p.FileSize}
	}
	ranked := identify.RankMovieFiles(files, identify.MovieInfo{Title: movie.Title, Year: movie.Year, RuntimeMin: movie.Runtime})
	for _, c := range ranked {
		if changed := pickMovie(ps, c.Path); len(changed) > 0 {
			return changed, true
		}
	}
	return nil, false
}

// legacyShowResult reports a legacy show assign the way the synchronous
// path did: videos and subtitles counted, extras skipped, unmatched files
// and conflicts with their reasons, and one matched row per episode
// written.
func legacyShowResult(all []*library.AssignmentProposal, refs map[int64]*library.EpisodeRef) *ShowAssignmentResult {
	out := &ShowAssignmentResult{Matched: []MatchedAssignment{}, Unmatched: []UnmatchedAssignment{}}
	shown := listed(all)
	partners := conflictPartners(shown)
	byID := make(map[int64]*library.AssignmentProposal, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}
	unmatched := func(p *library.AssignmentProposal, reason string) {
		out.Unmatched = append(out.Unmatched, UnmatchedAssignment{FilePath: p.FilePath, Reason: reason, Season: -1, Episode: -1})
	}
	written := func(p *library.AssignmentProposal) bool {
		if p.Decision != library.DecisionAccepted {
			return false
		}
		for _, id := range p.EpisodeIDs {
			if refs[id] != nil {
				return true
			}
		}
		return false
	}

	for _, p := range all {
		class := identify.ClassOf(p.FilePath)
		if class != identify.ClassVideo && class != identify.ClassSubtitle {
			continue // fonts, images, info files: not counted
		}
		out.Summary.TotalFiles++
		switch {
		case p.Kind == library.KindExtra:
			out.Summary.Skipped++
		case len(p.EpisodeIDs) == 0:
			unmatched(p, p.Reason)
		case p.Kind == library.KindSubtitle:
			video := p
			if p.LinkedProposalID != nil {
				video = byID[*p.LinkedProposalID]
			}
			switch {
			case video != nil && len(partners[video.ID]) > 0:
				unmatched(p, "Its video conflicts with another file")
			case video != nil && written(video):
				out.Summary.SubtitlesFound++
			}
		case len(partners[p.ID]) > 0:
			other := byID[partners[p.ID][0]]
			unmatched(p, clipReason(fmt.Sprintf("Conflicts with %s: both map to %s", clipName(path.Base(other.FilePath)), targetLabel(p.EpisodeIDs, refs))))
		case p.Decision == library.DecisionAccepted:
			quality := identify.ParseQuality(p.FilePath)
			for _, id := range p.EpisodeIDs {
				ref := refs[id]
				if ref == nil {
					continue
				}
				out.Matched = append(out.Matched, MatchedAssignment{
					EpisodeID: id, Season: ref.Season, Episode: ref.Number,
					FilePath: p.FilePath, FileSize: p.FileSize,
					Resolution: quality.Resolution, Confidence: p.Confidence,
				})
			}
		}
	}
	out.Summary.Matched = len(out.Matched)
	out.Summary.Unmatched = len(out.Unmatched)
	return out
}

// conflictPartners maps each proposal in a conflict to the others sharing
// one of its episodes, in id order.
func conflictPartners(ps []*library.AssignmentProposal) map[int64][]int64 {
	in := conflictIDs(ps)
	out := map[int64][]int64{}
	for _, a := range ps {
		if !in[a.ID] {
			continue
		}
		for _, b := range ps {
			if a.ID != b.ID && in[b.ID] && b.Decision != library.DecisionIgnored && b.Kind != library.KindSubtitle && sharesEpisode(a, b) {
				out[a.ID] = append(out[a.ID], b.ID)
			}
		}
	}
	return out
}

func sharesEpisode(a, b *library.AssignmentProposal) bool {
	for _, x := range a.EpisodeIDs {
		for _, y := range b.EpisodeIDs {
			if x == y {
				return true
			}
		}
	}
	return false
}

// clipName shortens a file name to 40 characters for a reason, which must
// stay within identify.MaxReasonRunes.
func clipName(name string) string {
	const n = 40
	if r := []rune(name); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return name
}

// targetLabel writes episodes as "S01E05" or "S01E05-E06".
func targetLabel(ids []int64, refs map[int64]*library.EpisodeRef) string {
	var first, last *library.EpisodeRef
	for _, id := range ids {
		if ref := refs[id]; ref != nil {
			if first == nil {
				first = ref
			}
			last = ref
		}
	}
	if first == nil {
		return "the same episode"
	}
	label := fmt.Sprintf("S%02dE%02d", first.Season, first.Number)
	if last != first {
		label += fmt.Sprintf("-E%02d", last.Number)
	}
	return label
}
