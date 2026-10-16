// Package catalog builds the title pages and the library list (PLAN §6
// "Titles and library"): cached TMDB details merged with what the library
// holds and where each title is, from "not in the library" to "ready in
// Infuse".
package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

var (
	// ErrNotInLibrary: the show isn't in the library, so it has no seasons
	// to list (404 not_in_library).
	ErrNotInLibrary = errors.New("not in the library")
	// ErrSeasonNotFound: the show has no such season, or it's specials
	// while they're switched off (404 not_found).
	ErrSeasonNotFound = errors.New("season not found")
	// ErrInvalidQuery is an unknown /library filter or sort (400).
	ErrInvalidQuery = errors.New("invalid query")
	// ErrSettingsUnavailable: no settings service is configured (503).
	ErrSettingsUnavailable = errors.New("show settings not configured")
)

// Store is the library database. *library.CatalogRepository implements it.
type Store interface {
	MovieByTMDBID(ctx context.Context, tmdbID int) (*library.Movie, error)
	ShowByTMDBID(ctx context.Context, tmdbID int) (*library.Show, error)
	ShowByID(ctx context.Context, id int64) (*library.Show, error)
	ShowEpisodes(ctx context.Context, showID int64) ([]library.Season, error)
	SeasonByTMDBID(ctx context.Context, tmdbID, seasonNumber int) (*library.SeasonView, error)
	LibraryItems(ctx context.Context, q library.LibraryQuery) ([]library.LibraryItem, int, error)
	ListLibrary(ctx context.Context, q library.LibraryQuery) (*library.LibraryListing, error)
	ReviewState(ctx context.Context, mediaType string, libraryID int64) (*library.ReviewState, error)
	WatchState(ctx context.Context, mediaType string, tmdbID int) (*library.TraktWatchState, error)
}

var _ Store = (*library.CatalogRepository)(nil)

// TMDB is the cached TMDB client. *tmdb.Cached implements it.
type TMDB interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
	EpisodeGroups(ctx context.Context, showID int) ([]tmdb.EpisodeGroupSummary, error)
	EpisodeGroup(ctx context.Context, id string) (*tmdb.EpisodeGroup, error)
}

var _ TMDB = (*tmdb.Cached)(nil)

// Torrents is the live torrent client. torrent.Service implements it.
type Torrents interface {
	ListTorrents() ([]torrent.TorrentStatus, error)
	GetStatus(infoHash string) (*torrent.TorrentStatus, error)
	FilesProgress(infoHash string) (map[string]torrent.FileProgress, error)
	IsActive(infoHash string) bool
	// PausedAt reports whether the user paused the torrent, loaded or not.
	PausedAt(infoHash string) (time.Time, bool)
}

var _ Torrents = (torrent.Service)(nil)

// Colors computes a backdrop's hero colour, nil while it's being computed.
// *artwork.Extractor implements it.
type Colors interface {
	Color(ctx context.Context, backdropPath string) *string
}

// SettingsApplier stores a show's settings. *metadata.Syncer implements it.
type SettingsApplier interface {
	ApplySettings(ctx context.Context, showID int64, patch metadata.SettingsPatch) (*library.Show, error)
}

var _ SettingsApplier = (*metadata.Syncer)(nil)

// Sessions are the clients streaming over WebDAV. *sessions.Registry
// implements it.
type Sessions interface {
	Sessions(ctx context.Context) []sessions.Session
}

var _ Sessions = (*sessions.Registry)(nil)

// Deps are the Service's dependencies. Torrents, Sessions, Colors and
// Settings are optional.
type Deps struct {
	Store    Store
	TMDB     TMDB
	Torrents Torrents        // nil: nothing is ever loaded
	Sessions Sessions        // nil: nothing is ever streaming
	Colors   Colors          // nil: colours are always null
	Settings SettingsApplier // nil: PatchShow fails with ErrSettingsUnavailable
	// WebDAVPublicURL is the address Infuse reaches WebDAV at
	// (WEBDAV_PUBLIC_URL); webdav_url is it plus the escaped path.
	WebDAVPublicURL string
	// HDRezkaURL is the HDRezka mirror (HDREZKA_URL, no trailing slash);
	// a stream's url is it plus the title page's path. "": rezka.ag.
	HDRezkaURL string
	Now        func() time.Time // nil: time.Now
}

// Service builds title pages and the library list.
type Service struct {
	store      Store
	tmdb       TMDB
	torrents   Torrents
	sessions   Sessions
	colors     Colors
	settings   SettingsApplier
	publicURL  string
	hdrezkaURL string
	now        func() time.Time
	log        *slog.Logger
}

// New creates a Service.
func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	hdrezkaURL := strings.TrimRight(d.HDRezkaURL, "/")
	if hdrezkaURL == "" {
		hdrezkaURL = hdrezka.DefaultBaseURL
	}
	return &Service{
		store:      d.Store,
		tmdb:       d.TMDB,
		torrents:   d.Torrents,
		sessions:   d.Sessions,
		colors:     d.Colors,
		settings:   d.Settings,
		publicURL:  d.WebDAVPublicURL,
		hdrezkaURL: hdrezkaURL,
		now:        now,
		log:        slog.With("component", "catalog"),
	}
}

// today is the date episodes count as aired by.
func (s *Service) today() string {
	return s.now().Format(time.DateOnly)
}

// tmdbError marks err as a TMDB failure: it matches library.ErrTMDBFetch
// (502 tmdb_unavailable) as well as tmdb.ErrNoAPIKey (503) and
// tmdb.ErrNotFound (404).
func tmdbError(err error) error {
	return fmt.Errorf("%w: %w", library.ErrTMDBFetch, err)
}

// fallBackToLibrary reports whether a TMDB failure for a title already in the
// library should be served from stored metadata instead of failing. Only a
// missing API key fails regardless: that's configuration, not an outage.
func fallBackToLibrary(err error) bool {
	return !errors.Is(err, tmdb.ErrNoAPIKey)
}

// colorAsync looks up the backdrop colour while the caller does other work.
func (s *Service) colorAsync(ctx context.Context, backdropPath string) func() *string {
	if s.colors == nil || backdropPath == "" {
		return func() *string { return nil }
	}
	ch := make(chan *string, 1)
	go func() { ch <- s.colors.Color(ctx, backdropPath) }()
	return func() *string { return <-ch }
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func yearPtr(year int) *int {
	if year <= 0 {
		return nil
	}
	return &year
}

func positive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

// nonNil keeps lists from encoding as JSON null.
func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}
