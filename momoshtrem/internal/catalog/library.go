package catalog

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// Library page sizes.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	// MaxPage caps page so the offset can't overflow; it's far past the end
	// of any library.
	MaxPage = 1_000_000
	// MaxLibraryIDs caps GET /library?ids=: a page of TMDB results asks
	// about at most this many titles at once.
	MaxLibraryIDs = 200
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
	// IDs looks titles up by TMDB ID: "movie:603,tv:1399", at most
	// MaxLibraryIDs. With IDs the answer is one page of all of them: Page
	// and PageSize are ignored.
	IDs string
}

// Library lists the library, a page at a time, each item with its
// availability and mapping review, plus the filter counts and the
// library's summary. One query, whatever the page. Unknown values return
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
	switch p.Status {
	case "", StatusReady, StatusIncomplete, StatusNeedsReview, StatusWatching:
	default:
		return nil, fmt.Errorf("%w: status must be ready, incomplete, needs_review or watching", ErrInvalidQuery)
	}
	var ids *library.TMDBIDs
	if p.IDs != "" {
		var n int
		var err error
		if ids, n, err = ParseLibraryIDs(p.IDs); err != nil {
			return nil, err
		}
		p.Page, p.PageSize = 1, n
	}

	tv := s.torrentView()
	listing, err := s.store.ListLibrary(ctx, library.LibraryQuery{
		MediaType:   p.MediaType,
		Status:      p.Status, // watching matches nothing until B5
		Sort:        p.Sort,
		Search:      p.Query,
		Limit:       p.PageSize,
		Offset:      (p.Page - 1) * p.PageSize,
		Today:       s.today(),
		Downloading: tv.downloadingFiles(),
		IDs:         ids,
	})
	if err != nil {
		return nil, err
	}

	c := listing.Counts
	page := &LibraryPage{
		Items:    make([]LibraryItem, 0, len(listing.Items)),
		Total:    listing.Total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Counts: LibraryCounts{All: c.All, Movie: c.Movies, TV: c.Shows, Ready: c.Ready, Incomplete: c.Incomplete,
			NeedsReview: c.NeedsReview},
		Summary: LibrarySummary{Total: listing.Summary.Total, SizeBytes: listing.Summary.SizeBytes, Ready: listing.Summary.Ready},
	}
	streaming := s.streamingTitles(ctx, len(listing.Items))
	for _, row := range listing.Items {
		item := s.libraryItem(row, tv)
		item.Streaming = streaming[titleKey{mediaType: row.MediaType, id: row.ID}]
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// titleKey is a library title: a movie or a show, by library id.
type titleKey struct {
	mediaType string // movie or tv
	id        int64
}

// streamingTitles are the titles a streaming session plays now (a show
// for any of its episodes), read from memory over the one query's rows.
func (s *Service) streamingTitles(ctx context.Context, rows int) map[titleKey]bool {
	out := map[titleKey]bool{}
	if s.sessions == nil || rows == 0 {
		return out
	}
	for _, ss := range s.sessions.Sessions(ctx) {
		if ss.Item != nil {
			out[titleKey{mediaType: ss.Item.MediaType, id: ss.Item.LibraryID}] = true
		}
	}
	return out
}

// libraryItem is one row of GET /library: the title page's library state,
// read from the one query.
func (s *Service) libraryItem(row library.LibraryItem, tv *torrentView) LibraryItem {
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
		Runtime:           positive(row.Runtime),
		Resolution:        strPtr(row.Resolution),
	}
	if row.ReviewJobID != nil {
		id := *row.ReviewJobID
		item.ReviewJobID = &id
	}
	if row.HasFile {
		var p string
		if row.MediaType == "movie" {
			p = vfs.MovieFilePath(row.Title, row.Year, row.FilePath)
		} else {
			p = vfs.ShowDir(row.Title, row.Year) + "/"
		}
		item.WebDAVPath = &p
		item.WebDAVURL = s.webdavURL(&p)
	}
	if len(row.Downloading) > 0 {
		done, length := tv.filesProgress(row.Downloading)
		item.Download = &LibraryDownload{Pct: pct(done, length), InfoHash: row.Downloading[0].InfoHash}
	}
	if m := row.Missing; m != nil {
		item.Missing = &MissingRun{SeasonNumber: m.SeasonNumber, From: m.From, To: m.To,
			AbsoluteFrom: copyInt(m.AbsoluteFrom), AbsoluteTo: copyInt(m.AbsoluteTo), Count: m.Count}
	}
	if row.MediaType == "tv" {
		item.EpisodesWatched = intPtr(0) // B5 fills it in
	}
	return item
}

// ParseLibraryIDs reads GET /library?ids=: comma-separated
// "movie:<tmdb_id>" and "tv:<tmdb_id>", at most MaxLibraryIDs. It returns
// the IDs and how many distinct titles they name; anything malformed is
// ErrInvalidQuery.
func ParseLibraryIDs(raw string) (*library.TMDBIDs, int, error) {
	parts := strings.Split(raw, ",")
	if len(parts) > MaxLibraryIDs {
		return nil, 0, fmt.Errorf("%w: ids takes at most %d titles", ErrInvalidQuery, MaxLibraryIDs)
	}
	ids := &library.TMDBIDs{}
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		mediaType, rawID, ok := strings.Cut(part, ":")
		id, err := strconv.Atoi(rawID)
		if !ok || err != nil || id < 1 || (mediaType != "movie" && mediaType != "tv") {
			return nil, 0, fmt.Errorf("%w: ids must look like movie:603,tv:1399", ErrInvalidQuery)
		}
		key := mediaType + ":" + strconv.Itoa(id)
		if seen[key] {
			continue
		}
		seen[key] = true
		if mediaType == "movie" {
			ids.Movies = append(ids.Movies, id)
		} else {
			ids.Shows = append(ids.Shows, id)
		}
	}
	return ids, len(seen), nil
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
