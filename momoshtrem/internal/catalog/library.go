package catalog

import (
	"context"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
)

// Library page sizes.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	// MaxPage caps page so the offset can't overflow; it's far past the end
	// of any library.
	MaxPage = 1_000_000
)

// Library status filters (GET /library?status=).
const (
	StatusReady       = "ready"        // availability_stage ready
	StatusIncomplete  = "incomplete"   // anything else: needs a torrent or downloading
	StatusNeedsReview = "needs_review" // files under mapping review (needs_review > 0)
	StatusWatching    = "watching"     // B5: empty until then
)

// LibraryParams are GET /library's query parameters.
type LibraryParams struct {
	MediaType string // "", "movie" or "tv"
	Status    string // "" or a Status*
	Sort      string // "" (added), "added", "title", "last_watched" or "size"
	Query     string
	Page      int // From 1; capped at MaxPage
	PageSize  int // 0 is DefaultPageSize; capped at MaxPageSize
}

// Library lists the library, a page at a time, each item with its
// availability and mapping review. One query (two when the page is past the
// end); none for watching, which has no data yet. Unknown values return
// ErrInvalidQuery.
func (s *Service) Library(ctx context.Context, p LibraryParams) (*LibraryPage, error) {
	if p.Page == 0 {
		p.Page = 1
	}
	if p.PageSize == 0 {
		p.PageSize = DefaultPageSize
	}
	switch {
	case p.Page < 1:
		return nil, fmt.Errorf("%w: page must be at least 1", ErrInvalidQuery)
	case p.PageSize < 1:
		return nil, fmt.Errorf("%w: page_size must be at least 1", ErrInvalidQuery)
	}
	p.PageSize = min(p.PageSize, MaxPageSize)
	p.Page = min(p.Page, MaxPage)
	switch p.MediaType {
	case "", "movie", "tv":
	default:
		return nil, fmt.Errorf("%w: type must be movie or tv", ErrInvalidQuery)
	}
	switch p.Sort {
	case "", library.LibrarySortAdded, library.LibrarySortTitle, library.LibrarySortLastWatched, library.LibrarySortSize:
	default:
		return nil, fmt.Errorf("%w: sort must be added, title, last_watched or size", ErrInvalidQuery)
	}

	page := &LibraryPage{Items: []LibraryItem{}, Page: p.Page, PageSize: p.PageSize}
	switch p.Status {
	case "", StatusReady, StatusIncomplete, StatusNeedsReview:
	case StatusWatching:
		return page, nil // no watch history until B5
	default:
		return nil, fmt.Errorf("%w: status must be ready, incomplete, needs_review or watching", ErrInvalidQuery)
	}

	rows, total, err := s.store.LibraryItems(ctx, library.LibraryQuery{
		MediaType:   p.MediaType,
		Status:      p.Status,
		Sort:        p.Sort,
		Search:      p.Query,
		Limit:       p.PageSize,
		Offset:      (p.Page - 1) * p.PageSize,
		Today:       s.today(),
		Downloading: s.torrentView().downloadingFiles(),
	})
	if err != nil {
		return nil, err
	}

	page.Total = total
	for _, row := range rows {
		item := LibraryItem{
			ID:                row.ID,
			MediaType:         row.MediaType,
			TMDBID:            row.TMDBID,
			Title:             row.Title,
			Year:              yearPtr(row.Year),
			PosterPath:        strPtr(row.PosterPath),
			AvailabilityStage: Stage(row.Stage),
			EpisodesTotal:     copyInt(row.EpisodesTotal),
			EpisodesReady:     copyInt(row.EpisodesReady),
			NeedsReview:       row.NeedsReview,
			SizeBytes:         row.SizeBytes,
			AddedAt:           row.CreatedAt,
		}
		if row.MediaType == "tv" {
			item.EpisodesWatched = intPtr(0) // B5 fills it in
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// PatchShow applies show settings (PATCH /shows/{id}) and returns the
// updated title. Invalid settings return library.ErrInvalidShowSettings; an
// unknown show library.ErrShowNotFound.
func (s *Service) PatchShow(ctx context.Context, showID int64, patch metadata.SettingsPatch) (*ShowTitle, error) {
	if s.settings == nil {
		return nil, ErrSettingsUnavailable
	}
	show, err := s.settings.ApplySettings(ctx, showID, patch)
	if err != nil {
		return nil, err
	}
	return s.ShowTitle(ctx, show.TMDBID)
}

// EpisodeGroups lists a library show's TMDB episode groups, for the ordering
// picker (GET /shows/{id}/episode-groups).
func (s *Service) EpisodeGroups(ctx context.Context, showID int64) ([]EpisodeGroup, error) {
	show, err := s.store.ShowByID(ctx, showID)
	if err != nil {
		return nil, err
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}
	groups, err := s.tmdb.EpisodeGroups(ctx, show.TMDBID)
	if err != nil {
		return nil, tmdbError(err)
	}
	items := make([]EpisodeGroup, 0, len(groups))
	for _, g := range groups {
		items = append(items, EpisodeGroup{
			ID:           g.ID,
			Name:         g.Name,
			Type:         g.Type,
			GroupCount:   g.GroupCount,
			EpisodeCount: g.EpisodeCount,
		})
	}
	return items, nil
}
