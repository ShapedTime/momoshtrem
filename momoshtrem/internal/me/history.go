package me

import (
	"context"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
)

// History page sizes (PLAN §6 "Trakt (B5)").
const (
	DefaultHistoryPageSize = 50
	MaxHistoryPageSize     = 200
	// maxHistoryPage caps page so the offset can't overflow; it's far past
	// the end of any history.
	maxHistoryPage = 1_000_000
)

// HistoryItem is one play of GET /me/history.
type HistoryItem struct {
	ID        int64  `json:"id"`         // Trakt's history event id
	MediaType string `json:"media_type"` // movie or tv
	TMDBID    int    `json:"tmdb_id"`    // the movie's or the show's
	Title     string `json:"title"`
	Year      *int   `json:"year"`
	// Season, Episode and EpisodeName are null for movies.
	Season      *int      `json:"season"`
	Episode     *int      `json:"episode"`
	EpisodeName *string   `json:"episode_name"`
	WatchedAt   time.Time `json:"watched_at"`
}

// HistoryPage is GET /me/history: a Paginated<HistoryItem>.
type HistoryPage struct {
	Items    []HistoryItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// History lists GET /me/history, the context's member's, newest first:
// page from 1, pageSize 1–MaxHistoryPageSize. One query.
func (s *Service) History(ctx context.Context, page, pageSize int) (*HistoryPage, error) {
	if page < 1 || pageSize < 1 || pageSize > MaxHistoryPageSize {
		return nil, fmt.Errorf("page %d, page_size %d: out of range", page, pageSize)
	}
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	page = min(page, maxHistoryPage)
	plays, total, err := s.store.TraktHistory(ctx, memberID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	out := &HistoryPage{Items: make([]HistoryItem, 0, len(plays)), Total: total, Page: page, PageSize: pageSize}
	for _, p := range plays {
		item := HistoryItem{ID: p.ID, MediaType: string(library.JobMovie), TMDBID: p.TMDBID, Title: p.Title,
			Year: positive(p.Year), WatchedAt: p.WatchedAt}
		if p.MediaType != string(library.JobMovie) {
			item.MediaType = string(library.JobTV)
			item.Season, item.Episode, item.EpisodeName = p.Season, p.Episode, strPtr(p.EpisodeTitle)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
