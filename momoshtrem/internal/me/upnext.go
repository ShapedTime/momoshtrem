package me

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
)

// Up next's limits (PLAN §6 "Trakt (B5)").
const (
	MaxUpNext          = 40
	maxWatchlistStarts = 3
	// idleAfter: a show last watched longer ago is paused, and left out
	// unless include_paused.
	idleAfter = 180 * 24 * time.Hour
)

// UpNextItem is one card of GET /me/up-next.
type UpNextItem struct {
	Show        ShowRef    `json:"show"`
	NextEpisode EpisodeRef `json:"next_episode"`
	EpisodeCard
	AirsAt        *time.Time            `json:"airs_at"` // only while airs_on; null when unknown
	Progress      catalog.WatchProgress `json:"progress"`
	LastWatchedAt *time.Time            `json:"last_watched_at"` // null for a watchlist start
	Source        string                `json:"source"`          // progress or watchlist
}

// UpNext lists GET /me/up-next, the context's member's: the shows to carry
// on with by last watched, then watchlist starts, then what airs next.
// Queries: the settings, the candidates and their episodes in the library.
func (s *Service) UpNext(ctx context.Context) ([]UpNextItem, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	shows, episodes, err := s.readUpNext(ctx, memberID, now)
	if err != nil || len(shows) == 0 {
		return []UpNextItem{}, err
	}
	return upNextItems(shows, episodes, s.cards(), now, s.today()), nil
}

// readUpNext reads the member's Up next shows (upNextShows) with what the
// library knows of the episode each names, one each, in order.
func (s *Service) readUpNext(ctx context.Context, memberID int64, now time.Time) ([]library.UpNextCandidate, []library.KeyedEpisode, error) {
	settings, err := s.settings.TraktSettings(ctx, memberID)
	if err != nil {
		slog.Warn("Trakt settings unreadable; Up next uses the defaults", "component", "me", "member", memberID, "error", err)
	}
	candidates, err := s.store.UpNextCandidates(ctx, memberID, s.today())
	if err != nil {
		return nil, nil, err
	}
	shows := upNextShows(candidates, settings, now)
	if len(shows) == 0 {
		return nil, nil, nil
	}
	keys := make([]library.TraktEpisodeKey, len(shows))
	for i, c := range shows {
		keys[i] = c.Next.TraktEpisodeKey
	}
	episodes, err := s.store.EpisodesByKeys(ctx, keys)
	if err != nil {
		return nil, nil, err
	}
	return shows, episodes, nil
}

// UpNextEpisode is a library show Up next lists, with the library episode
// it names next: what smart downloads start from (#203).
type UpNextEpisode struct {
	ShowID int64 // the library show's
	// EpisodeID, SeasonNumber and EpisodeNumber are the library episode
	// the matching rule finds, in the library's numbering.
	EpisodeID     int64
	SeasonNumber  int
	EpisodeNumber int
	// LastWatchedAt is the show's last Trakt play; nil for a watchlist
	// start.
	LastWatchedAt *time.Time
}

// UpNextEpisodes lists the context's member's Up next shows the library
// has with the episode each names next, when the library lists it: the
// shows upNextShows picks (as GET /me/up-next does, without its cap or its
// order), in no particular order. Empty without Trakt. Queries: the
// settings, the candidates and their episodes.
func (s *Service) UpNextEpisodes(ctx context.Context) ([]UpNextEpisode, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	shows, episodes, err := s.readUpNext(ctx, memberID, s.now())
	if err != nil {
		return nil, err
	}
	var out []UpNextEpisode
	for i, k := range episodes {
		if k.Show == nil || k.Episode == nil {
			continue
		}
		out = append(out, UpNextEpisode{ShowID: k.Show.ID, EpisodeID: k.Episode.ID,
			SeasonNumber: k.SeasonNumber, EpisodeNumber: k.Episode.EpisodeNumber, LastWatchedAt: shows[i].LastWatchedAt})
	}
	return out, nil
}

// upNextShows picks Up next's shows (PLAN §6 "Trakt (B5)"), each with the
// episode it names: every Trakt progress row with a next episode (one per
// show, the last watched), except dropped and hidden shows and, unless
// include_paused, shows idle for over 180 days; then, with show_watchlist,
// the watchlisted library shows nobody has started, newest listed first.
func upNextShows(candidates []library.UpNextCandidate, settings library.TraktSettings, now time.Time) []library.UpNextCandidate {
	latest := map[int]int{} // show → its last watched progress row
	var starts []library.UpNextCandidate
	for i, c := range candidates {
		if c.Source == library.UpNextFromWatchlist {
			starts = append(starts, c)
			continue
		}
		if prev, ok := latest[c.ShowTMDBID]; !ok || newer(c.LastWatchedAt, candidates[prev].LastWatchedAt) {
			latest[c.ShowTMDBID] = i
		}
	}

	var out []library.UpNextCandidate
	for i, c := range candidates {
		if c.Source == library.UpNextFromWatchlist || latest[c.ShowTMDBID] != i {
			continue
		}
		idle := c.LastWatchedAt != nil && now.Sub(*c.LastWatchedAt) > idleAfter
		if c.Next == nil || c.Hidden || (idle && !settings.IncludePaused) {
			continue
		}
		out = append(out, c)
	}
	if !settings.ShowWatchlist {
		return out
	}
	slices.SortStableFunc(starts, func(a, b library.UpNextCandidate) int { return byNewest(a.ListedAt, b.ListedAt) })
	for _, c := range starts {
		if _, started := latest[c.ShowTMDBID]; started || c.Hidden || c.Next == nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// upNextItems builds the cards of shows, whose episodes in the library are
// episodes (one each, in order), and orders them: the shows that aren't
// airs_on by last watched, newest first; then at most three watchlist
// starts whose first episode has aired; then airs_on by when it airs,
// unknown last. At most MaxUpNext.
func upNextItems(shows []library.UpNextCandidate, episodes []library.KeyedEpisode, c cards, now time.Time, today string) []UpNextItem {
	var actionable, starts, upcoming []UpNextItem
	for i, show := range shows {
		k, next := episodes[i], show.Next
		item := UpNextItem{
			Show:          showRef(k, traktShow{show.Title, show.PosterPath, show.BackdropPath}),
			NextEpisode:   episodeRef(k, traktEpisode{next.Title, next.StillPath, next.FirstAired}),
			Progress:      catalog.WatchProgress{Completed: show.Completed, Aired: show.Aired},
			LastWatchedAt: show.LastWatchedAt,
			Source:        show.Source,
		}
		if show.Source == library.UpNextFromWatchlist {
			// Once it has aired, by the library's date.
			if k.Episode == nil || !catalog.Aired(k.Episode.AirDate, today) || len(starts) == maxWatchlistStarts {
				continue
			}
			item.EpisodeCard = c.card(k, true)
			starts = append(starts, item)
			continue
		}
		item.EpisodeCard = c.card(k, upNextAired(k, next.FirstAired, now, today))
		if item.Availability != AvailabilityAirsOn {
			actionable = append(actionable, item)
			continue
		}
		item.AirsAt = airsAt(k, next.FirstAired)
		upcoming = append(upcoming, item)
	}

	slices.SortStableFunc(actionable, func(a, b UpNextItem) int {
		return cmp.Or(byNewest(a.LastWatchedAt, b.LastWatchedAt), byShow(a, b))
	})
	slices.SortStableFunc(upcoming, func(a, b UpNextItem) int {
		return cmp.Or(bySoonest(a.AirsAt, b.AirsAt), byShow(a, b))
	})
	items := append(append(actionable, starts...), upcoming...)
	if len(items) > MaxUpNext {
		items = items[:MaxUpNext]
	}
	return nonNil(items)
}

// upNextAired reports whether a progress show's next episode has aired, at
// read time: by the calendar's first airing, else the library's air date.
// With neither it has: Trakt only names aired episodes next, and the
// calendar lists every one it names that airs soon.
func upNextAired(k library.KeyedEpisode, firstAired *time.Time, now time.Time, today string) bool {
	switch {
	case firstAired != nil:
		return !firstAired.After(now)
	case k.Episode != nil && k.Episode.AirDate != "":
		return catalog.Aired(k.Episode.AirDate, today)
	}
	return true
}

// newer reports whether a is later than b; nil is never later.
func newer(a, b *time.Time) bool {
	return a != nil && (b == nil || a.After(*b))
}

// byNewest orders times newest first, nil last.
func byNewest(a, b *time.Time) int {
	switch {
	case newer(a, b):
		return -1
	case newer(b, a):
		return 1
	}
	return 0
}

// bySoonest orders times soonest first, nil last.
func bySoonest(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return a.Compare(*b)
}

// byShow breaks ties: by show title, then TMDB id.
func byShow(a, b UpNextItem) int {
	return cmp.Or(cmp.Compare(a.Show.Title, b.Show.Title), cmp.Compare(a.Show.TMDBID, b.Show.TMDBID))
}

// nonNil keeps lists from encoding as JSON null.
func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}
