// Package me serves a household member's own lists (PLAN §6 "Trakt (B5)"):
// Up next, new episodes, the watchlist and the history. They come from
// what the member's Trakt sync imported and what the library holds, and
// only read: without Trakt (nothing imported) they're empty, except new
// episodes, which the library's shows still fill. Each list is the
// context's member's (member.Require).
package me

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// Store is the database. *library.CatalogRepository implements it.
type Store interface {
	UpNextCandidates(ctx context.Context, memberID int64, today string) ([]library.UpNextCandidate, error)
	NewEpisodeCandidates(ctx context.Context, memberID int64, from, now time.Time, fromDate, today string) ([]library.NewEpisodeCandidate, error)
	EpisodesByKeys(ctx context.Context, keys []library.TraktEpisodeKey) ([]library.KeyedEpisode, error)
	TraktWatchlist(ctx context.Context, memberID int64, mediaType string) ([]library.TraktWatchlistItem, error)
	TraktHistory(ctx context.Context, memberID int64, limit, offset int) ([]library.TraktPlay, int, error)
}

var _ Store = (*library.CatalogRepository)(nil)

// Settings reads a member's Trakt settings. *library.MemberRepository
// implements it.
type Settings interface {
	TraktSettings(ctx context.Context, memberID int64) (library.TraktSettings, error)
}

var _ Settings = (*library.MemberRepository)(nil)

// Catalog is what the title pages know about library titles and files.
// *catalog.Service implements it.
type Catalog interface {
	LibraryStates(ctx context.Context, ids library.TMDBIDs) (map[catalog.TitleRef]*catalog.SimilarLibrary, error)
	Downloads() *catalog.Downloads
}

var _ Catalog = (*catalog.Service)(nil)

// Deps are the Service's dependencies.
type Deps struct {
	Store    Store
	Settings Settings
	Catalog  Catalog
	// WebDAVPublicURL is the address Infuse reaches WebDAV at
	// (WEBDAV_PUBLIC_URL); webdav_url is it plus the escaped path.
	WebDAVPublicURL string
	Now             func() time.Time // nil: time.Now
}

// Service serves /me.
type Service struct {
	store     Store
	settings  Settings
	catalog   Catalog
	publicURL string
	now       func() time.Time
}

// New creates a Service.
func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: d.Store, settings: d.Settings, catalog: d.Catalog, publicURL: d.WebDAVPublicURL, now: now}
}

// ShowRef is the show on an episode card.
type ShowRef struct {
	TMDBID       int     `json:"tmdb_id"`
	Title        string  `json:"title"`
	PosterPath   *string `json:"poster_path"`
	BackdropPath *string `json:"backdrop_path"`
	LibraryID    *int64  `json:"library_id"` // null when the show isn't in the library
}

// EpisodeRef is the episode on a card: the landscape card's still, name
// and absolute number.
type EpisodeRef struct {
	catalog.NextEpisode
	StillPath *string `json:"still_path"`
	TMDBID    *int    `json:"tmdb_id"` // TMDB's episode id; null when unknown
}

// Availability is what an episode card offers.
type Availability string

const (
	AvailabilityReady        Availability = "ready"
	AvailabilityDownloading  Availability = "downloading"
	AvailabilityNeedsReview  Availability = "needs_review"
	AvailabilityNeedsTorrent Availability = "needs_torrent"
	AvailabilityNotInLibrary Availability = "not_in_library"
	AvailabilityAirsOn       Availability = "airs_on" // Up next only
)

// EpisodeCard is what a card's action needs; the fields that don't apply
// to its availability are null.
type EpisodeCard struct {
	Availability Availability `json:"availability"`
	// WebDAVPath and WebDAVURL are the episode's file as Infuse sees it,
	// while ready or downloading.
	WebDAVPath  *string                  `json:"webdav_path"`
	WebDAVURL   *string                  `json:"webdav_url"`
	Download    *catalog.LibraryDownload `json:"download"`      // while downloading
	ReviewJobID *int64                   `json:"review_job_id"` // while needs_review
}

// cards builds episode cards for one request.
type cards struct {
	// progress reports whether an assigned file is downloading (a download
	// wants it and it isn't all on disk, #77), and what's on disk of it
	// (catalog.Downloads.Progress, the title pages' rule).
	progress  func(a *library.TorrentAssignment) (downloading bool, pct int)
	publicURL string
}

func (s *Service) cards() cards {
	return cards{progress: s.catalog.Downloads().Progress, publicURL: s.publicURL}
}

// card is what an episode card offers (PLAN §6 "Trakt (B5)"), first match:
// the episode has a file (ready, or downloading while a download fetches
// it; a file nothing keeps streams on demand, so it's ready); a
// mapping job wants it (needs_review); it hasn't aired (airs_on); the show
// is in the library (needs_torrent, an episode the library doesn't list
// yet too); else not_in_library. It reads files and reviews, never plays.
func (c cards) card(k library.KeyedEpisode, aired bool) EpisodeCard {
	ep := k.Episode
	switch {
	case ep != nil && ep.Assignment != nil:
		season := append([]library.Episode{*ep}, k.FileMates...)
		p := catalog.EpisodePaths(k.Show.Title, k.Show.Year, k.SeasonNumber, season)[ep.ID]
		u := vfs.PublicURL(c.publicURL, p)
		card := EpisodeCard{Availability: AvailabilityReady, WebDAVPath: &p, WebDAVURL: &u}
		if downloading, pct := c.progress(ep.Assignment); downloading {
			card.Availability = AvailabilityDownloading
			card.Download = &catalog.LibraryDownload{Pct: pct, InfoHash: ep.Assignment.InfoHash}
		}
		return card
	case ep != nil && k.ReviewJobID != nil:
		id := *k.ReviewJobID
		return EpisodeCard{Availability: AvailabilityNeedsReview, ReviewJobID: &id}
	case !aired:
		return EpisodeCard{Availability: AvailabilityAirsOn}
	case k.Show != nil:
		return EpisodeCard{Availability: AvailabilityNeedsTorrent}
	}
	return EpisodeCard{Availability: AvailabilityNotInLibrary}
}

// traktShow is a show as Trakt's rows name it.
type traktShow struct {
	title, posterPath, backdropPath string
}

// showRef is the card's show: the library's when it's there (its artwork
// falling back to Trakt's), else Trakt's.
func showRef(k library.KeyedEpisode, t traktShow) ShowRef {
	ref := ShowRef{TMDBID: k.Key.ShowTMDBID, Title: t.title, PosterPath: strPtr(t.posterPath), BackdropPath: strPtr(t.backdropPath)}
	if sh := k.Show; sh != nil {
		id := sh.ID
		ref.LibraryID = &id
		ref.Title = firstOf(sh.Title, t.title)
		ref.PosterPath = strPtr(firstOf(sh.PosterPath, t.posterPath))
		ref.BackdropPath = strPtr(firstOf(sh.BackdropPath, t.backdropPath))
	}
	return ref
}

// traktEpisode is an episode as Trakt's rows name it.
type traktEpisode struct {
	name, stillPath string
	firstAired      *time.Time // the calendar's
}

// episodeRef is the card's episode: its numbers, name and still are the
// library's when the library lists it, else Trakt's. The absolute number is
// set only for a library show with absolute ordering, the rule the title
// page labels its rows by ("#1124 · S22E39"); otherwise it's null.
func episodeRef(k library.KeyedEpisode, t traktEpisode) EpisodeRef {
	ref := EpisodeRef{
		NextEpisode: catalog.NextEpisode{Season: k.Key.Season, Episode: k.Key.Episode, Name: strPtr(t.name),
			AirDate: strPtr(utcDate(t.firstAired))},
		StillPath: strPtr(t.stillPath),
		TMDBID:    positive(k.Key.EpisodeTMDBID),
	}
	if ep := k.Episode; ep != nil {
		ref.Season, ref.Episode = k.SeasonNumber, ep.EpisodeNumber
		if ep.AbsoluteNumber != nil && k.Show != nil && k.Show.EpisodeOrdering == library.OrderingAbsolute {
			n := *ep.AbsoluteNumber
			ref.AbsoluteNumber = &n
		}
		ref.Name = strPtr(firstOf(ep.Name, t.name))
		ref.AirDate = strPtr(firstOf(ep.AirDate, utcDate(t.firstAired)))
		ref.StillPath = strPtr(firstOf(ep.StillPath, t.stillPath))
		if ep.TMDBEpisodeID > 0 {
			ref.TMDBID = positive(ep.TMDBEpisodeID)
		}
	}
	return ref
}

// airsAt is when an episode airs: the calendar's first airing, else the
// library's air date at 00:00 UTC; nil when neither is known.
func airsAt(k library.KeyedEpisode, firstAired *time.Time) *time.Time {
	if firstAired != nil {
		at := *firstAired
		return &at
	}
	if k.Episode == nil || k.Episode.AirDate == "" {
		return nil
	}
	at, err := time.Parse(time.DateOnly, k.Episode.AirDate)
	if err != nil {
		return nil
	}
	return &at
}

// today is the date episodes count as aired by, as the title pages count.
func (s *Service) today() string {
	return s.now().Format(time.DateOnly)
}

func utcDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func positive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}
