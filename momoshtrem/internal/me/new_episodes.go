package me

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// New episodes' window and limit (PLAN §6 "Trakt (B5)").
const (
	DefaultNewEpisodeDays = 7
	MaxNewEpisodeDays     = 30
	MaxNewEpisodes        = 30
)

// NewEpisodeItem is one card of GET /me/new-episodes.
type NewEpisodeItem struct {
	Show    ShowRef    `json:"show"`
	Episode EpisodeRef `json:"episode"`
	AiredAt time.Time  `json:"aired_at"`
	EpisodeCard
}

// NewEpisodes lists GET /me/new-episodes: regular episodes that aired in
// the last days days (1–30) and haven't been watched, of the library's
// shows and the shows on the Trakt calendar, each once, newest first.
// Queries: the candidates and their episodes in the library.
func (s *Service) NewEpisodes(ctx context.Context, days int) ([]NewEpisodeItem, error) {
	if days < 1 || days > MaxNewEpisodeDays {
		return nil, fmt.Errorf("days must be from 1 to %d, not %d", MaxNewEpisodeDays, days)
	}
	now := s.now()
	today := s.today()
	fromDate := now.AddDate(0, 0, -days).Format(time.DateOnly)
	from, _ := time.Parse(time.DateOnly, fromDate) // midnight UTC
	candidates, err := s.store.NewEpisodeCandidates(ctx, from, now, fromDate, today)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []NewEpisodeItem{}, nil
	}
	keys := make([]library.TraktEpisodeKey, len(candidates))
	for i, c := range candidates {
		keys[i] = c.TraktEpisodeKey
	}
	episodes, err := s.store.EpisodesByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	return newEpisodeItems(candidates, episodes, s.cards(), from, now), nil
}

// newEpisodeItems builds the cards of candidates, whose episodes in the
// library are episodes (one each, in order). Each episode is one card: a
// calendar entry the library lists is that episode, aired when the
// calendar says. A card is kept when it aired from `from` until now; they
// come newest first, at most MaxNewEpisodes.
func newEpisodeItems(candidates []library.NewEpisodeCandidate, episodes []library.KeyedEpisode, c cards, from, now time.Time) []NewEpisodeItem {
	type merged struct {
		k    library.KeyedEpisode
		show traktShow
		ep   traktEpisode
	}
	byEpisode := map[string]*merged{}
	var all []*merged
	for i, cand := range candidates {
		k := episodes[i]
		id := fmt.Sprintf("trakt:%d:%d:%d", cand.ShowTMDBID, cand.Season, cand.Episode)
		if k.Episode != nil {
			id = fmt.Sprintf("episode:%d", k.Episode.ID)
		}
		m := byEpisode[id]
		if m == nil {
			m = &merged{k: k}
			byEpisode[id] = m
			all = append(all, m)
		}
		if cand.FromCalendar {
			m.show = traktShow{cand.ShowTitle, cand.PosterPath, cand.BackdropPath}
			m.ep = traktEpisode{cand.EpisodeTitle, cand.StillPath, cand.FirstAired}
		}
	}

	items := make([]NewEpisodeItem, 0, len(all))
	for _, m := range all {
		at := airsAt(m.k, m.ep.firstAired)
		if at == nil || at.Before(from) || at.After(now) {
			continue
		}
		items = append(items, NewEpisodeItem{
			Show:        showRef(m.k, m.show),
			Episode:     episodeRef(m.k, m.ep),
			AiredAt:     *at,
			EpisodeCard: c.card(m.k, true),
		})
	}
	slices.SortStableFunc(items, func(a, b NewEpisodeItem) int {
		return cmp.Or(b.AiredAt.Compare(a.AiredAt), cmp.Compare(a.Show.Title, b.Show.Title),
			cmp.Compare(a.Show.TMDBID, b.Show.TMDBID), cmp.Compare(a.Episode.Season, b.Episode.Season),
			cmp.Compare(a.Episode.Episode, b.Episode.Episode))
	})
	if len(items) > MaxNewEpisodes {
		items = items[:MaxNewEpisodes]
	}
	return items
}
