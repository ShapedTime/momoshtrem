package recommend

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
)

// FeedbackStore keeps each member's feedback on titles
// (library.RecFeedbackRepository).
type FeedbackStore interface {
	Set(ctx context.Context, memberID int64, ref library.TitleRef, signal, source string) error
	Clear(ctx context.Context, memberID int64, ref library.TitleRef) error
}

// FeedbackService is POST and DELETE /recommendations/feedback (R1,
// #153), for the context's member (member.Require). It works without a
// key: feedback is saved at once, and the member's profile, candidate
// pools and every AI surface read it when they next run.
type FeedbackService struct {
	store FeedbackStore
}

// NewFeedbackService creates a FeedbackService.
func NewFeedbackService(store FeedbackStore) *FeedbackService {
	return &FeedbackService{store: store}
}

// SetFeedback upserts the member's signal on a title; the newest wins.
func (s *FeedbackService) SetFeedback(ctx context.Context, f Feedback) error {
	memberID, err := member.Require(ctx)
	if err != nil {
		return err
	}
	return s.store.Set(ctx, memberID, library.TitleRef{MediaType: f.MediaType, TMDBID: f.TMDBID}, f.Signal, f.Source)
}

// ClearFeedback removes the member's signal on a title; idempotent.
func (s *FeedbackService) ClearFeedback(ctx context.Context, mediaType string, tmdbID int) error {
	memberID, err := member.Require(ctx)
	if err != nil {
		return err
	}
	return s.store.Clear(ctx, memberID, library.TitleRef{MediaType: mediaType, TMDBID: tmdbID})
}
