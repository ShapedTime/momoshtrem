package recommend

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// FeedbackStore keeps feedback on titles (library.RecFeedbackRepository).
type FeedbackStore interface {
	Set(ctx context.Context, ref library.TitleRef, signal, source string) error
	Clear(ctx context.Context, ref library.TitleRef) error
}

// FeedbackService is POST and DELETE /recommendations/feedback (R1,
// #153). It works without a key: feedback is saved at once, and the
// profile, the candidate pools and every AI surface read it when they
// next run.
type FeedbackService struct {
	store FeedbackStore
}

// NewFeedbackService creates a FeedbackService.
func NewFeedbackService(store FeedbackStore) *FeedbackService {
	return &FeedbackService{store: store}
}

// SetFeedback upserts a title's signal; the newest wins.
func (s *FeedbackService) SetFeedback(ctx context.Context, f Feedback) error {
	return s.store.Set(ctx, library.TitleRef{MediaType: f.MediaType, TMDBID: f.TMDBID}, f.Signal, f.Source)
}

// ClearFeedback removes a title's signal; idempotent.
func (s *FeedbackService) ClearFeedback(ctx context.Context, mediaType string, tmdbID int) error {
	return s.store.Clear(ctx, library.TitleRef{MediaType: mediaType, TMDBID: tmdbID})
}
