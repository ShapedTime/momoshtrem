package trakt

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// What the sync reads from Trakt (developer.trakt.tv, checked 2026-10-16),
// and the rows it makes of it. Items without a TMDB id are skipped: every
// row maps to TMDB.

// lastActivities is the part of GET /sync/last_activities the incremental
// sync compares. A field Trakt leaves out stays zero.
type lastActivities struct {
	Movies struct {
		WatchedAt     time.Time `json:"watched_at"`
		RatedAt       time.Time `json:"rated_at"`
		WatchlistedAt time.Time `json:"watchlisted_at"`
	} `json:"movies"`
	Episodes struct {
		WatchedAt time.Time `json:"watched_at"`
		RatedAt   time.Time `json:"rated_at"`
	} `json:"episodes"`
	Shows struct {
		RatedAt       time.Time `json:"rated_at"`
		WatchlistedAt time.Time `json:"watchlisted_at"`
		HiddenAt      time.Time `json:"hidden_at"`
		DroppedAt     time.Time `json:"dropped_at"`
	} `json:"shows"`
	Seasons struct {
		RatedAt  time.Time `json:"rated_at"`
		HiddenAt time.Time `json:"hidden_at"`
	} `json:"seasons"`
	Watchlist struct {
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"watchlist"`
}

type ids struct {
	Trakt int64 `json:"trakt"`
	TMDB  int   `json:"tmdb"` // 0 when Trakt has none (null)
}

// title is a movie or a show.
type title struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
	IDs   ids    `json:"ids"`
}

type episode struct {
	Season int    `json:"season"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	IDs    ids    `json:"ids"`
}

type season struct {
	Number int `json:"number"`
	IDs    ids `json:"ids"`
}

// historyItem is one play of GET /sync/history/{movies|episodes}.
type historyItem struct {
	ID        int64     `json:"id"`
	WatchedAt time.Time `json:"watched_at"`
	Movie     *title    `json:"movie"`
	Show      *title    `json:"show"`
	Episode   *episode  `json:"episode"`
}

// ratingItem is one rating of GET /sync/ratings/{type}.
type ratingItem struct {
	RatedAt time.Time `json:"rated_at"`
	Rating  int       `json:"rating"`
	Movie   *title    `json:"movie"`
	Show    *title    `json:"show"`
	Season  *season   `json:"season"`
	Episode *episode  `json:"episode"`
}

// watchlistItem is one title of GET /sync/watchlist/{movies|shows}/….
type watchlistItem struct {
	Rank     int       `json:"rank"`
	ListedAt time.Time `json:"listed_at"`
	Movie    *title    `json:"movie"`
	Show     *title    `json:"show"`
}

// progressItem is one show of the bulk GET /sync/progress/watched.
type progressItem struct {
	Show     title `json:"show"`
	Progress struct {
		Aired         int        `json:"aired"`
		Completed     int        `json:"completed"`
		LastWatchedAt *time.Time `json:"last_watched_at"`
		ResetAt       *time.Time `json:"reset_at"`
		NextEpisode   *episode   `json:"next_episode"`
		LastEpisode   *episode   `json:"last_episode"`
	} `json:"progress"`
}

// hiddenItem is one show of GET /users/hidden/{section}.
type hiddenItem struct {
	Show *title `json:"show"`
}

// calendarItem is one episode of GET /calendars/my/shows/….
type calendarItem struct {
	FirstAired time.Time `json:"first_aired"`
	Episode    episode   `json:"episode"`
	Show       title     `json:"show"`
}

func (h historyItem) play() (library.TraktPlayRow, bool) {
	p := library.TraktPlayRow{ID: h.ID, WatchedAt: h.WatchedAt}
	switch {
	case h.Movie != nil && h.Movie.IDs.TMDB != 0:
		p.MediaType, p.TMDBID, p.TraktID, p.Title, p.Year = "movie", h.Movie.IDs.TMDB, h.Movie.IDs.Trakt, h.Movie.Title, h.Movie.Year
	case h.Show != nil && h.Episode != nil && h.Show.IDs.TMDB != 0:
		p.MediaType, p.TMDBID, p.TraktID, p.Title, p.Year = "episode", h.Show.IDs.TMDB, h.Show.IDs.Trakt, h.Show.Title, h.Show.Year
		p.Season, p.Episode = ptr(h.Episode.Season), ptr(h.Episode.Number)
		p.EpisodeTMDBID, p.EpisodeTitle = h.Episode.IDs.TMDB, h.Episode.Title
	default:
		return p, false
	}
	return p, true
}

// rating maps a rating of mediaType (movie|show|season|episode).
func (r ratingItem) rating(mediaType string) (library.TraktRatingRow, bool) {
	out := library.TraktRatingRow{MediaType: mediaType, Rating: r.Rating, RatedAt: r.RatedAt}
	if r.Rating < 1 || r.Rating > 10 {
		return out, false
	}
	switch {
	case mediaType == "movie" && r.Movie != nil && r.Movie.IDs.TMDB != 0:
		out.TraktID, out.TMDBID = r.Movie.IDs.Trakt, r.Movie.IDs.TMDB
	case mediaType == "show" && r.Show != nil && r.Show.IDs.TMDB != 0:
		out.TraktID, out.TMDBID = r.Show.IDs.Trakt, r.Show.IDs.TMDB
	case mediaType == "season" && r.Season != nil && r.Show != nil && r.Show.IDs.TMDB != 0:
		out.TraktID, out.TMDBID, out.ShowTMDBID = r.Season.IDs.Trakt, r.Season.IDs.TMDB, r.Show.IDs.TMDB
		out.Season = ptr(r.Season.Number)
	case mediaType == "episode" && r.Episode != nil && r.Show != nil && r.Show.IDs.TMDB != 0:
		out.TraktID, out.TMDBID, out.ShowTMDBID = r.Episode.IDs.Trakt, r.Episode.IDs.TMDB, r.Show.IDs.TMDB
		out.Season, out.Episode = ptr(r.Episode.Season), ptr(r.Episode.Number)
	default:
		return out, false
	}
	return out, true
}

func (w watchlistItem) item() (library.TraktWatchlistRow, bool) {
	out := library.TraktWatchlistRow{Rank: w.Rank, ListedAt: w.ListedAt}
	t := w.Movie
	out.MediaType = "movie"
	if t == nil {
		t, out.MediaType = w.Show, "tv"
	}
	if t == nil || t.IDs.TMDB == 0 {
		return out, false
	}
	out.TraktID, out.TMDBID, out.Title, out.Year = t.IDs.Trakt, t.IDs.TMDB, t.Title, t.Year
	return out, true
}

func (p progressItem) progress() (library.TraktShowProgressRow, bool) {
	out := library.TraktShowProgressRow{
		ShowTraktID: p.Show.IDs.Trakt, ShowTMDBID: p.Show.IDs.TMDB, Title: p.Show.Title, Year: p.Show.Year,
		Aired: p.Progress.Aired, Completed: p.Progress.Completed,
		LastWatchedAt: p.Progress.LastWatchedAt, ResetAt: p.Progress.ResetAt,
	}
	if last := p.Progress.LastEpisode; last != nil {
		out.LastSeason, out.LastEpisode = ptr(last.Season), ptr(last.Number)
	}
	if next := p.Progress.NextEpisode; next != nil {
		out.NextSeason, out.NextEpisode = ptr(next.Season), ptr(next.Number)
		out.NextEpisodeTMDBID, out.NextTitle = next.IDs.TMDB, next.Title
	}
	return out, out.ShowTMDBID != 0
}

func (c calendarItem) entry() (library.TraktCalendarRow, bool) {
	return library.TraktCalendarRow{
		ShowTraktID: c.Show.IDs.Trakt, Season: c.Episode.Season, Episode: c.Episode.Number,
		ShowTMDBID: c.Show.IDs.TMDB, EpisodeTMDBID: c.Episode.IDs.TMDB,
		ShowTitle: c.Show.Title, Year: c.Show.Year, EpisodeTitle: c.Episode.Title, FirstAired: c.FirstAired,
	}, c.Show.IDs.TMDB != 0
}

func ptr[T any](v T) *T { return &v }
