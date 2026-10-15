package service

import (
	"context"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// applyMode is who applies a job.
type applyMode int

const (
	// applyReview is POST /assignment-jobs/{id}/apply: a ready job with
	// nothing pending and no conflict.
	applyReview applyMode = iota
	// applyAuto is the worker applying a freshly classified job on its
	// own: only when autoApplies holds; otherwise nothing happens.
	applyAuto
	// applyLegacy is ?wait=true: what's accepted is applied, conflicting
	// and pending files are left out.
	applyLegacy
)

// applyResult is what an apply wrote.
type applyResult struct {
	applied   bool
	write     library.AssignmentWrite
	subtitles []*subtitle.Subtitle // created after the commit
}

// applyJob applies the job in one transaction (the only writer of
// assignments), then records its subtitles and rebuilds the WebDAV tree.
func (s *JobService) applyJob(ctx context.Context, id int64, mode applyMode) (*applyResult, error) {
	var res *applyResult
	err := s.jobs.Locked(ctx, id, func(t *library.JobTx) error {
		var err error
		res, err = s.applyLocked(t, mode)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterApply(ctx, res)
	return res, nil
}

// applyChecks answers why a job can't be applied, in the contract's order:
// dry_run, job_closed, job_working, conflicting_proposals,
// pending_proposals. The legacy path skips the last two: it leaves those
// files out.
func applyChecks(job *library.AssignmentJob, listedProposals []*library.AssignmentProposal, mode applyMode) error {
	switch {
	case job.DryRun:
		return conflict(CodeDryRun, "A dry run is never applied.")
	case !job.Status.Open():
		return conflict(CodeJobClosed, fmt.Sprintf("This mapping job is already %s.", job.Status))
	case job.Status.Working():
		return conflict(CodeJobWorking, "The files are still being matched.")
	}
	if mode == applyLegacy {
		return nil
	}
	c := countsOf(listedProposals)
	if c.Conflicts > 0 {
		return conflict(CodeConflictingProposals, "Two files point at the same episode. Keep one.")
	}
	if c.Pending > 0 {
		return conflict(CodePendingProposals, fmt.Sprintf("Decide on %d %s first.", c.Pending, plural(c.Pending, "file", "files")))
	}
	return nil
}

// applyLocked applies the locked job: one row per accepted target episode
// (or the accepted movie file), upserted with its confidence and source;
// the episodes' other rows deactivated; the torrent's rows on the title
// that the job no longer maps deactivated. Episode ids are re-validated:
// one a refresh removed is left out. The job is moved to applied, guarded
// by its status.
func (s *JobService) applyLocked(t *library.JobTx, mode applyMode) (*applyResult, error) {
	job := t.Job
	shown := listed(t.Proposals)
	if err := applyChecks(job, shown, mode); err != nil {
		return nil, err
	}
	if mode == applyAuto && !autoApplies(job, shown) {
		return &applyResult{}, nil
	}

	res, err := s.planApply(t)
	if err != nil {
		return nil, err
	}
	if err := t.WriteAssignments(res.write); err != nil {
		return nil, err
	}
	from := []library.JobStatus{library.JobReady}
	if mode == applyLegacy {
		from = append(from, library.JobNeedsReview)
	}
	ok, err := t.MarkApplied(from)
	if err != nil {
		return nil, err
	}
	if !ok {
		if job.Status == library.JobNeedsReview {
			return nil, conflict(CodePendingProposals, "Some files still need a look.")
		}
		return nil, conflict(CodeJobClosed, fmt.Sprintf("This mapping job is already %s.", job.Status))
	}
	res.applied = true
	s.log.Info("Assignment job applied", "job_id", job.ID, "rows", len(res.write.Rows), "subtitles", len(res.subtitles))
	return res, nil
}

// planApply works out the rows and subtitles of the locked job's accepted
// proposals.
func (s *JobService) planApply(t *library.JobTx) (*applyResult, error) {
	job := t.Job
	res := &applyResult{write: library.AssignmentWrite{InfoHash: job.InfoHash, MagnetURI: job.MagnetURI}}
	byID := make(map[int64]*library.AssignmentProposal, len(t.Proposals))
	for _, p := range t.Proposals {
		byID[p.ID] = p
	}
	accepted := func(p *library.AssignmentProposal) bool {
		return p.Listed() && p.Decision == library.DecisionAccepted
	}

	switch job.MediaType {
	case library.JobTV:
		if job.ShowID == nil {
			return nil, fmt.Errorf("job %d has no show", job.ID)
		}
		res.write.ItemType, res.write.ShowID = library.ItemTypeEpisode, *job.ShowID

		var ids []int64
		for _, p := range t.Proposals {
			if accepted(p) && (p.Kind == library.KindEpisode || p.Kind == library.KindSubtitle) {
				ids = append(ids, p.EpisodeIDs...)
			}
		}
		refs, err := t.EpisodeRefs(*job.ShowID, ids)
		if err != nil {
			return nil, err
		}

		claimed := map[int64]bool{}
		first := map[int64]int64{} // proposal → the first episode it was written to
		for _, p := range t.Proposals {
			if !accepted(p) || p.Kind != library.KindEpisode {
				continue
			}
			quality := identify.ParseQuality(p.FilePath)
			for _, id := range p.EpisodeIDs {
				if refs[id] == nil {
					s.log.Warn("Episode isn't the show's any more; left out", "job_id", job.ID, "episode_id", id, "file_path", p.FilePath)
					continue
				}
				if claimed[id] {
					continue
				}
				claimed[id] = true
				if first[p.ID] == 0 {
					first[p.ID] = id
				}
				res.write.Rows = append(res.write.Rows, library.TorrentAssignment{
					ItemID: id, FilePath: p.FilePath, FileSize: p.FileSize,
					Resolution: quality.Resolution, Source: quality.Source,
					Confidence: p.Confidence, MatchSource: p.Source,
				})
			}
		}
		for _, p := range t.Proposals {
			if p.Kind != library.KindSubtitle {
				continue
			}
			var episode int64
			switch {
			case p.LinkedProposalID != nil:
				episode = first[*p.LinkedProposalID]
			case accepted(p) && len(p.EpisodeIDs) > 0 && refs[p.EpisodeIDs[0]] != nil:
				episode = p.EpisodeIDs[0]
			}
			if episode != 0 {
				res.subtitles = append(res.subtitles, torrentSubtitle(job, p, subtitle.ItemTypeEpisode, episode))
			}
		}

	case library.JobMovie:
		if job.MovieID == nil {
			return nil, fmt.Errorf("job %d has no movie", job.ID)
		}
		res.write.ItemType, res.write.MovieID = library.ItemTypeMovie, *job.MovieID
		var film *library.AssignmentProposal
		for _, p := range t.Proposals {
			if accepted(p) && p.Kind == library.KindMovie {
				film = p
				break
			}
		}
		if film == nil {
			break
		}
		quality := identify.ParseQuality(film.FilePath)
		res.write.Rows = append(res.write.Rows, library.TorrentAssignment{
			ItemID: *job.MovieID, FilePath: film.FilePath, FileSize: film.FileSize,
			Resolution: quality.Resolution, Source: quality.Source,
			Confidence: film.Confidence, MatchSource: film.Source,
		})
		for _, p := range t.Proposals {
			if p.Kind == library.KindSubtitle && p.LinkedProposalID != nil && byID[*p.LinkedProposalID] == film {
				res.subtitles = append(res.subtitles, torrentSubtitle(job, p, subtitle.ItemTypeMovie, *job.MovieID))
			}
		}
	}
	return res, nil
}

// torrentSubtitle is a subtitle file of the job's torrent, for an item.
func torrentSubtitle(job *library.AssignmentJob, p *library.AssignmentProposal, itemType subtitle.ItemType, itemID int64) *subtitle.Subtitle {
	code, name, _ := subtitle.DetectLanguage(p.FilePath)
	return &subtitle.Subtitle{
		ItemType:     itemType,
		ItemID:       itemID,
		LanguageCode: code,
		LanguageName: name,
		Format:       subtitle.ParseFormat(p.FilePath),
		FilePath:     p.FilePath,
		FileSize:     p.FileSize,
		Source:       subtitle.SourceTorrent,
		InfoHash:     job.InfoHash,
		ReleaseName:  subtitle.AssignedRelease(job.MagnetURI, p.FilePath),
	}
}

// afterApply records the applied job's subtitles (each replaces the item's
// subtitles from an earlier torrent) and rebuilds the WebDAV tree, which
// renames grouped multi-episode files and drops the earlier torrent's
// files. It outlives a cancelled request: the rows are committed.
func (s *JobService) afterApply(ctx context.Context, res *applyResult) {
	if res == nil || !res.applied {
		return
	}
	ctx = context.WithoutCancel(ctx)
	if s.subtitles != nil {
		for _, sub := range res.subtitles {
			if err := s.subtitles.CreateTorrentSubtitle(ctx, sub); err != nil {
				s.log.Error("Failed to record a torrent subtitle", "item_id", sub.ItemID, "file_path", sub.FilePath, "error", err)
			}
		}
	}
	if s.tree != nil {
		s.tree.InvalidateTree()
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
