package catalog

import (
	"context"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Watch state (B5): what the Trakt sync imported, read for the title pages
// (PLAN §6 "Watch state rules"). Without Trakt it's always null.

// movieWatchState reads a movie's watch state: one query.
func (s *Service) movieWatchState(ctx context.Context, tmdbID int) (*WatchState, error) {
	ws, err := s.store.WatchState(ctx, string(library.JobMovie), tmdbID)
	if err != nil || ws == nil {
		return nil, err
	}
	state := watchStateOf(ws)
	return &state, nil
}

// showWatchState reads a show's watch state: one query.
func (s *Service) showWatchState(ctx context.Context, tmdbID int) (*ShowWatchState, error) {
	ws, err := s.store.WatchState(ctx, string(library.JobTV), tmdbID)
	if err != nil || ws == nil {
		return nil, err
	}
	state := &ShowWatchState{WatchState: watchStateOf(ws)}
	if p := ws.Progress; p != nil {
		state.Progress = &WatchProgress{Completed: p.Completed, Aired: p.Aired}
	}
	if n := ws.Next; n != nil {
		state.NextEpisode = &NextEpisode{Season: n.Season, Episode: n.Episode, AbsoluteNumber: copyInt(n.AbsoluteNumber),
			Name: strPtr(n.Name), AirDate: strPtr(n.AirDate)}
	}
	return state, nil
}

func watchStateOf(ws *library.TraktWatchState) WatchState {
	state := WatchState{Plays: ws.Plays, Rating: copyInt(ws.Rating)}
	if ws.LastWatchedAt != nil {
		at := *ws.LastWatchedAt
		state.WatchedAt = &at
	}
	return state
}

// preferNextSeason opens the page on the season of the episode Up next
// names, when the page lists that season (PLAN §6 default_season).
func preferNextSeason(title *ShowTitle) {
	if title.WatchState == nil || title.WatchState.NextEpisode == nil {
		return
	}
	next := title.WatchState.NextEpisode.Season
	for _, s := range title.Seasons {
		if s.SeasonNumber == next {
			title.DefaultSeason = intPtr(next)
			return
		}
	}
}
