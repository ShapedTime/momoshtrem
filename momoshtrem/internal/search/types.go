package search

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/release"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// Sort orders a search's results (sort=).
type Sort string

const (
	SortBest    Sort = "best"    // score, then seeders; the best match first on page 1
	SortSeeders Sort = "seeders" // most first
	SortSize    Sort = "size"    // smallest first
	SortNewest  Sort = "newest"  // by published time, unknown last
)

// Paging.
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

var (
	// ErrJackettUnavailable: the indexer list can't be fetched, or Jackett
	// rejects the key (502 jackett_unavailable). The error's text says why
	// without the key or a URL.
	ErrJackettUnavailable = errors.New("jackett is unavailable")
	// ErrReleaseNotFound: no release has this id in the last hour (404
	// not_found: search again). It's Inspect's error, so its route maps
	// both the same way.
	ErrReleaseNotFound = service.ErrReleaseNotFound
	// ErrTorrentUnavailable: the release's .torrent or magnet can't be
	// fetched or read (502 torrent_unavailable).
	ErrTorrentUnavailable = errors.New("torrent unavailable")
	// ErrResolveTimeout: fetching the .torrent took too long (504 timeout).
	// It wraps torznab.ErrTimeout.
	ErrResolveTimeout = fmt.Errorf("fetching the torrent %w", torznab.ErrTimeout)
)

// SearchRequest is GET /search/torrents.
type SearchRequest struct {
	MediaType string // "movie" or "tv"
	TMDBID    int
	// Season, Episode and Scope are for TV; Plan checks them.
	Season  *int
	Episode *int
	Scope   Scope
	// Q is the user's own query: it replaces the generated text queries,
	// and the scope still filters.
	Q       string
	Filters Filters
	Sort    Sort // "" is SortBest
	// Page is 1-based; 0 is 1. PageSize 0 is DefaultPageSize; it's capped
	// at MaxPageSize.
	Page, PageSize int
	// Refresh re-runs the search, at most once per refreshEvery.
	Refresh bool
}

// Filters narrow the results: OR within a filter, AND across them. A value
// a filter doesn't know matches nothing.
type Filters struct {
	Resolution, Source, Codec, HDR, Audio, Pack, Indexer, Group []string
	MinSeeders                                                  *int
	// MinSizeGB and MaxSizeGB compare per episode for TV (the size over
	// the episodes a release covers), the whole release for movies.
	MinSizeGB, MaxSizeGB *float64
}

// validate checks req, applies its defaults and returns its media type.
// TV's season, episode and scope are checked as Plan checks them.
func (req *SearchRequest) validate() (library.JobMediaType, error) {
	mt := library.JobMediaType(req.MediaType)
	switch {
	case mt != library.JobMovie && mt != library.JobTV:
		return "", fmt.Errorf("%w: media_type must be movie or tv", ErrBadRequest)
	case req.TMDBID < 1:
		return "", fmt.Errorf("%w: tmdb_id must be a positive number", ErrBadRequest)
	}
	if mt == library.JobTV {
		if _, err := scopeOf(req.planRequest()); err != nil {
			return "", err
		}
	}
	switch req.Sort {
	case "":
		req.Sort = SortBest
	case SortBest, SortSeeders, SortSize, SortNewest:
	default:
		return "", fmt.Errorf("%w: unknown sort %q", ErrBadRequest, req.Sort)
	}
	switch {
	case req.Page < 0:
		return "", fmt.Errorf("%w: page must be 1 or more", ErrBadRequest)
	case req.PageSize < 0:
		return "", fmt.Errorf("%w: page_size must be 1 or more", ErrBadRequest)
	case req.Filters.MinSeeders != nil && *req.Filters.MinSeeders < 0:
		return "", fmt.Errorf("%w: min_seeders must be 0 or more", ErrBadRequest)
	case !validSize(req.Filters.MinSizeGB):
		return "", fmt.Errorf("%w: min_size_gb must be 0 or more", ErrBadRequest)
	case !validSize(req.Filters.MaxSizeGB):
		return "", fmt.Errorf("%w: max_size_gb must be 0 or more", ErrBadRequest)
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = DefaultPageSize
	}
	req.PageSize = min(req.PageSize, MaxPageSize)
	return mt, nil
}

func validSize(gb *float64) bool {
	return gb == nil || *gb >= 0 && !math.IsInf(*gb, 0) && !math.IsNaN(*gb)
}

// planRequest is req's part of the query plan.
func (req *SearchRequest) planRequest() Request {
	return Request{Season: req.Season, Episode: req.Episode, Scope: req.Scope, Q: req.Q}
}

// SearchResponse is PLAN §6 SearchTorrentsResponse.
type SearchResponse struct {
	Items    []Release `json:"items"`
	Total    int       `json:"total"` // after filters
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	// TotalUnfiltered is in scope, after the profile hides, before filters.
	TotalUnfiltered int `json:"total_unfiltered"`
	// Hidden is how many the profile dropped.
	Hidden int `json:"hidden"`
	// BestID is the best match among the filtered releases; nil for none.
	BestID   *string         `json:"best_id"`
	Facets   Facets          `json:"facets"`
	Queries  []SentQuery     `json:"queries"`
	Indexers []IndexerReport `json:"indexers"`
	Complete bool            `json:"complete"`
	Missing  *MissingRun     `json:"missing"`
	CachedAt time.Time       `json:"cached_at"` // when the search started
	Warnings []string        `json:"warnings"`  // problems that aren't about one indexer
}

// Release is one result (PLAN §6 Release). It's built field by field, so
// the Torznab link (it carries the API key) and attributes never reach it.
type Release struct {
	ID           string          `json:"id"`
	RawTitle     string          `json:"raw_title"`
	DisplayTitle string          `json:"display_title"`
	InfoHash     *string         `json:"info_hash"`
	MagnetURI    *string         `json:"magnet_uri"`
	SizeBytes    int64           `json:"size_bytes"`
	Seeders      int             `json:"seeders"`
	Leechers     int             `json:"leechers"`
	PublishedAt  *time.Time      `json:"published_at"`
	Indexer      string          `json:"indexer"`
	Attrs        release.Release `json:"attrs"`
	Score        int             `json:"score"`
	ScoreReasons []string        `json:"score_reasons"`
	Notes        []string        `json:"notes"`
	Warnings     []string        `json:"warnings"`
	Flags        []string        `json:"flags"`
}

// FlagNeedsTorrentFetch: the release has no magnet yet, so Add and Copy
// magnet resolve it first.
const FlagNeedsTorrentFetch = "needs_torrent_fetch"

// Facets are the disjunctive counts: each facet counted with every other
// active filter and its own left out.
type Facets struct {
	Resolution []FacetBucket `json:"resolution"`
	Source     []FacetBucket `json:"source"`
	Codec      []FacetBucket `json:"codec"`
	HDR        []FacetBucket `json:"hdr"`
	Audio      []FacetBucket `json:"audio"`
	Pack       []FacetBucket `json:"pack"`
	Indexer    []FacetBucket `json:"indexer"`
	Group      []FacetBucket `json:"group"`
}

// FacetBucket is what selecting Value would give under the other filters.
type FacetBucket struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// SentQuery is a text query sent to the indexers. Label is nil for the
// primary query.
type SentQuery struct {
	Q     string  `json:"q"`
	Label *string `json:"label"`
}

// IndexerReport is one indexer over all of a search's queries.
type IndexerReport struct {
	Name   string         `json:"name"`
	Status torznab.Status `json:"status"` // pending, ok, timeout or error
	Count  int            `json:"count"`  // results it returned
	MS     int64          `json:"ms"`     // time so far while pending
	Error  string         `json:"error,omitempty"`
}

// MissingRun is the title's missing run when the request falls in it.
type MissingRun struct {
	catalog.MissingRun
	// BestCovers: the best match has every episode of the run.
	BestCovers bool `json:"best_covers"`
}
