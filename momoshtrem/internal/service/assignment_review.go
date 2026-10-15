package service

import (
	"context"
	"fmt"
	"path"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// Review (PLAN §6 "Assignment", PATCH): the user's changes to a job's
// proposals, and the episode list the picker chooses from.

// reasonManual is a manual pick's reason.
const reasonManual = "You picked this episode"

// ValidationError is a well-formed request that can't apply (422
// validation_error). Message is shown to the user.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// ProposalChange is one item of PATCH /assignment-jobs/{id}/proposals: a
// new decision, new targets, or both. A nil field stays as it is.
type ProposalChange struct {
	ID       int64
	Decision *library.Decision
	// EpisodeIDs replaces the targets; never empty when set (to leave a
	// file out, ignore it).
	EpisodeIDs []int64
}

// PatchProposals applies changes to a job's proposals, all or nothing,
// and returns the job's detail with its status and counts recomputed
// (during resolving the AI run's end recomputes the status).
//   - EpisodeIDs equal to the suggestion restore it (targets, confidence,
//     source and reason); any others make a manual pick: source manual, no
//     confidence, reason "You picked this episode".
//   - Accepting needs targets, or a movie candidate. Accepting a movie
//     candidate ignores the job's other candidates.
//   - user_decided is false again once a proposal is back on its suggestion
//     with its group's default decision.
//
// It answers job_closed or job_working (409) when the job can't change,
// and a ValidationError for an unknown proposal, an episode that isn't the
// show's, episode ids on a movie job, accepting a file that maps to
// nothing, or episodes for a file that isn't a video (or a subtitle).
func (s *JobService) PatchProposals(ctx context.Context, jobID int64, changes []ProposalChange) (*JobDetail, error) {
	err := s.jobs.Locked(ctx, jobID, func(t *library.JobTx) error {
		job := t.Job
		switch {
		case !job.Status.Open():
			return conflict(CodeJobClosed, fmt.Sprintf("This mapping job is already %s.", job.Status))
		case !job.Status.Classified():
			return conflict(CodeJobWorking, "The files are still being matched.")
		}
		changed, err := s.planChanges(t, changes)
		if err != nil {
			return err
		}
		if err := t.UpdateProposals(changed); err != nil {
			return err
		}
		if job.Status != library.JobResolving {
			job.Status = settledStatus(listed(t.Proposals))
		}
		return t.SaveJob()
	})
	if err != nil {
		return nil, err
	}
	return s.Detail(ctx, jobID)
}

// planChanges works out the proposals changes make on the locked job, in
// the order they were sent (an id may come twice: the later change builds
// on the earlier one).
func (s *JobService) planChanges(t *library.JobTx, changes []ProposalChange) ([]*library.AssignmentProposal, error) {
	job := t.Job
	shown := listed(t.Proposals)
	byID := make(map[int64]*library.AssignmentProposal, len(shown))
	for _, p := range shown {
		byID[p.ID] = p
	}

	var episodeIDs []int64
	for _, c := range changes {
		if c.EpisodeIDs == nil {
			continue
		}
		if job.MediaType == library.JobMovie {
			return nil, invalid("Movie files have no episodes: accept the file instead.")
		}
		if len(c.EpisodeIDs) == 0 {
			return nil, invalid("Proposal %d: pick an episode, or ignore the file.", c.ID)
		}
		episodeIDs = append(episodeIDs, c.EpisodeIDs...)
	}
	refs := map[int64]*library.EpisodeRef{}
	if len(episodeIDs) > 0 && job.ShowID != nil {
		var err error
		if refs, err = t.EpisodeRefs(*job.ShowID, episodeIDs); err != nil {
			return nil, err
		}
	}

	next := map[int64]*library.AssignmentProposal{}
	var order []int64
	for _, c := range changes {
		p := next[c.ID]
		if p == nil {
			current, ok := byID[c.ID]
			if !ok {
				return nil, invalid("This job has no proposal %d.", c.ID)
			}
			copied := *current
			p = &copied
			next[c.ID] = p
			order = append(order, c.ID)
		}
		if c.EpisodeIDs != nil {
			ids := uniqueIDs(c.EpisodeIDs)
			for _, id := range ids {
				if refs[id] == nil {
					return nil, invalid("Episode %d isn't one of this show's.", id)
				}
			}
			p.EpisodeIDs = ids
			if equalIDs(ids, p.SuggestedEpisodeIDs) {
				p.Confidence, p.Source, p.Reason = p.SuggestedConfidence, p.SuggestedSource, p.SuggestedReason
			} else {
				p.Confidence, p.Source, p.Reason = "", library.SourceManual, reasonManual
			}
		}
		if c.Decision != nil {
			switch *c.Decision {
			case library.DecisionPending, library.DecisionAccepted, library.DecisionIgnored:
			default:
				return nil, invalid("Proposal %d: unknown decision %q.", c.ID, *c.Decision)
			}
			p.Decision = *c.Decision
		}
		if p.Decision == library.DecisionAccepted && !p.Maps() {
			return nil, invalid("Proposal %d: pick an episode before accepting it.", c.ID)
		}
		if len(p.EpisodeIDs) > 0 && (c.EpisodeIDs != nil || p.Decision == library.DecisionAccepted) && !takesEpisodes(p) {
			return nil, invalid("%q isn't a video, so it can't play an episode.", path.Base(p.FilePath))
		}
	}

	// A movie job maps one file: the first candidate accepted here wins
	// and the others are ignored.
	for _, id := range order {
		picked := next[id]
		if picked.Kind != library.KindMovie || picked.Decision != library.DecisionAccepted {
			continue
		}
		for _, p := range shown {
			if p.Kind != library.KindMovie || p.ID == picked.ID {
				continue
			}
			other := next[p.ID]
			if other == nil {
				if p.Decision == library.DecisionIgnored {
					continue
				}
				copied := *p
				other = &copied
				next[p.ID] = other
				order = append(order, p.ID)
			}
			other.Decision = library.DecisionIgnored
		}
		break
	}

	out := make([]*library.AssignmentProposal, 0, len(order))
	for _, id := range order {
		p := next[id]
		p.UserDecided = !(equalIDs(p.EpisodeIDs, p.SuggestedEpisodeIDs) && p.Decision == p.Group.DefaultDecision())
		out = append(out, p)
	}
	return out, nil
}

// uniqueIDs drops repeated ids, keeping the first of each in order.
func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// ShowEpisodesSeason is one season of GET /shows/{id}/episodes.
type ShowEpisodesSeason struct {
	SeasonNumber int           `json:"season_number"`
	Name         string        `json:"name"`
	Episodes     []ShowEpisode `json:"episodes"`
}

// ShowEpisode is one episode the picker offers.
type ShowEpisode struct {
	ID             int64   `json:"id"`
	EpisodeNumber  int     `json:"episode_number"`
	AbsoluteNumber *int    `json:"absolute_number"`
	Name           string  `json:"name"`
	AirDate        *string `json:"air_date"`
	HasFile        bool    `json:"has_file"` // it has an active assignment ("In library")
}

// ShowEpisodes lists every season of a library show in number order, so
// Specials (season 0) come first whenever its rows exist, even with
// include_specials off; has_file marks episodes with an active assignment.
// It returns library.ErrShowNotFound for an unknown show. One query.
func (s *JobService) ShowEpisodes(ctx context.Context, showID int64) ([]ShowEpisodesSeason, error) {
	seasons, err := s.shows.EpisodeList(ctx, showID)
	if err != nil {
		return nil, err
	}
	out := make([]ShowEpisodesSeason, 0, len(seasons))
	for _, season := range seasons {
		v := ShowEpisodesSeason{
			SeasonNumber: season.Number,
			Name:         catalog.SeasonName(season.Number, season.Name),
			Episodes:     make([]ShowEpisode, 0, len(season.Episodes)),
		}
		for _, ep := range season.Episodes {
			v.Episodes = append(v.Episodes, ShowEpisode{
				ID:             ep.ID,
				EpisodeNumber:  ep.Number,
				AbsoluteNumber: ep.AbsoluteNumber,
				Name:           ep.Name,
				AirDate:        nonEmpty(ep.AirDate),
				HasFile:        ep.HasFile,
			})
		}
		out = append(out, v)
	}
	return out, nil
}
