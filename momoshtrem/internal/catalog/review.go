package catalog

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// review is a library title's mapping review (PLAN §6 "Review rules"): its
// open, non-dry-run jobs past classification.
type review struct {
	state    *library.ReviewState
	episodes map[int64]bool // targeted by a file under review
}

// reviewOf reads a library title's review: one query. mediaType is "tv" or
// "movie".
func (s *Service) reviewOf(ctx context.Context, mediaType string, libraryID int64) (*review, error) {
	state, err := s.store.ReviewState(ctx, mediaType, libraryID)
	if err != nil {
		return nil, err
	}
	r := &review{state: state, episodes: make(map[int64]bool, len(state.EpisodeIDs))}
	for _, id := range state.EpisodeIDs {
		r.episodes[id] = true
	}
	return r, nil
}

// underReview reports whether a file under review targets the episode.
func (r *review) underReview(episodeID int64) bool {
	return r != nil && r.episodes[episodeID]
}

// setOn sets the title's needs_review and review_job_id.
func (r *review) setOn(state *TitleLibraryState) {
	if r == nil {
		return
	}
	state.NeedsReview = r.state.NeedsReview
	if r.state.JobID != nil {
		id := *r.state.JobID
		state.ReviewJobID = &id
	}
}
