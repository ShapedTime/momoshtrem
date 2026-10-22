package me

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
)

// WatchlistItem is one title of GET /me/watchlist.
type WatchlistItem struct {
	MediaType  string    `json:"media_type"`
	TMDBID     int       `json:"tmdb_id"`
	Title      string    `json:"title"`
	Year       *int      `json:"year"`
	PosterPath *string   `json:"poster_path"`
	ListedAt   time.Time `json:"listed_at"`
	// Library is the title's place in the library, as "More like this"
	// shows it; null when it isn't there.
	Library *catalog.SimilarLibrary `json:"library"`
}

// Watchlist lists GET /me/watchlist, the context's member's, newest listed
// first: mediaType "movie" or "tv", or "" for both. Queries: the watchlist
// and its titles' library state.
func (s *Service) Watchlist(ctx context.Context, mediaType string) ([]WatchlistItem, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.TraktWatchlist(ctx, memberID, mediaType)
	if err != nil || len(rows) == 0 {
		return []WatchlistItem{}, err
	}
	var ids library.TMDBIDs
	for _, w := range rows {
		if w.MediaType == string(library.JobMovie) {
			ids.Movies = append(ids.Movies, w.TMDBID)
		} else {
			ids.Shows = append(ids.Shows, w.TMDBID)
		}
	}
	states, err := s.catalog.LibraryStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]WatchlistItem, 0, len(rows))
	for _, w := range rows {
		items = append(items, WatchlistItem{
			MediaType:  w.MediaType,
			TMDBID:     w.TMDBID,
			Title:      w.Title,
			Year:       positive(w.Year),
			PosterPath: strPtr(w.PosterPath),
			ListedAt:   w.ListedAt,
			Library:    states[catalog.TitleRef{MediaType: w.MediaType, TMDBID: w.TMDBID}],
		})
	}
	return items, nil
}
