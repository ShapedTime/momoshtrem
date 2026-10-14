package metadata

import (
	"sort"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// NumberedEpisode is a library episode, as absolute numbering sees it.
type NumberedEpisode struct {
	ID            int64
	SeasonNumber  int
	EpisodeNumber int
	TMDBEpisodeID int // 0 when unknown
}

// NumberingGroup returns the episode group a show's absolute numbers come
// from, or "" for cumulative numbering:
//   - episode_group: the chosen group;
//   - absolute: the chosen group, else the absolute-type group (TMDB type 2)
//     with the most episodes;
//   - aired: none.
func NumberingGroup(ordering library.EpisodeOrdering, chosen string, groups []tmdb.EpisodeGroupSummary) string {
	switch ordering {
	case library.OrderingEpisodeGroup:
		return chosen
	case library.OrderingAbsolute:
		if chosen != "" {
			return chosen
		}
		return PickAbsoluteGroup(groups)
	}
	return ""
}

// PickAbsoluteGroup returns the ID of the absolute-type group with the most
// episodes (the first on a tie), or "" when there is none.
func PickAbsoluteGroup(groups []tmdb.EpisodeGroupSummary) string {
	best, bestCount := "", -1
	for _, g := range groups {
		if g.Type == tmdb.GroupAbsolute && g.EpisodeCount > bestCount {
			best, bestCount = g.ID, g.EpisodeCount
		}
	}
	return best
}

// AbsoluteNumbers numbers a show's episodes 1..N.
//
// With a group, the order is the group's: its groups by order, then their
// episodes by order. Library episodes are matched by TMDB episode ID, falling
// back to season and episode number. Episodes aired after the last one the
// group lists continue its numbering in aired order (groups lag airing
// shows); other episodes the group doesn't list get no number. Without a
// group, numbers are cumulative over the regular seasons in season and
// episode order.
//
// Specials (season 0) never get a number, and don't advance the count. The
// result maps episode IDs to numbers; unnumbered episodes are absent.
func AbsoluteNumbers(episodes []NumberedEpisode, group *tmdb.EpisodeGroup) map[int64]int {
	if group != nil {
		return numbersFromGroup(episodes, group)
	}
	return cumulativeNumbers(episodes)
}

func cumulativeNumbers(episodes []NumberedEpisode) map[int64]int {
	regular := make([]NumberedEpisode, 0, len(episodes))
	for _, ep := range episodes {
		if ep.SeasonNumber > 0 {
			regular = append(regular, ep)
		}
	}
	sort.SliceStable(regular, func(i, j int) bool {
		if regular[i].SeasonNumber != regular[j].SeasonNumber {
			return regular[i].SeasonNumber < regular[j].SeasonNumber
		}
		return regular[i].EpisodeNumber < regular[j].EpisodeNumber
	})

	numbers := make(map[int64]int, len(regular))
	for i, ep := range regular {
		numbers[ep.ID] = i + 1
	}
	return numbers
}

type seasonEpisode struct{ season, episode int }

func numbersFromGroup(episodes []NumberedEpisode, group *tmdb.EpisodeGroup) map[int64]int {
	byTMDB := make(map[int]int64, len(episodes))
	byNumber := make(map[seasonEpisode]int64, len(episodes))
	for _, ep := range episodes {
		if ep.SeasonNumber == 0 {
			continue
		}
		if ep.TMDBEpisodeID != 0 {
			byTMDB[ep.TMDBEpisodeID] = ep.ID
		}
		byNumber[seasonEpisode{ep.SeasonNumber, ep.EpisodeNumber}] = ep.ID
	}

	groups := append([]tmdb.EpisodeGroupGroup(nil), group.Groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Order < groups[j].Order })

	numbers := make(map[int64]int, len(episodes))
	counted := make(map[seasonEpisode]bool)
	var last seasonEpisode // Latest aired episode the group covers
	next := 1
	for _, g := range groups {
		eps := append([]tmdb.GroupEpisode(nil), g.Episodes...)
		sort.SliceStable(eps, func(i, j int) bool { return eps[i].Order < eps[j].Order })

		for _, ge := range eps {
			key := seasonEpisode{ge.SeasonNumber, ge.EpisodeNumber}
			if ge.SeasonNumber == 0 || counted[key] {
				continue // specials get no number; overlapping groups count once
			}
			counted[key] = true
			if key.after(last) {
				last = key
			}
			n := next
			next++

			id, ok := byTMDB[ge.ID]
			if !ok {
				id, ok = byNumber[key]
			}
			if ok {
				numbers[id] = n
			}
		}
	}

	// Groups lag behind airing shows (a long anime's absolute group may stop
	// dozens of episodes short). Episodes aired after the last one the group
	// covers carry on from where it stops, in aired order. Unlisted episodes
	// before that point stay unnumbered: the group left them out on purpose.
	var newer []NumberedEpisode
	for _, ep := range episodes {
		key := seasonEpisode{ep.SeasonNumber, ep.EpisodeNumber}
		if _, done := numbers[ep.ID]; ep.SeasonNumber > 0 && !done && key.after(last) {
			newer = append(newer, ep)
		}
	}
	sort.SliceStable(newer, func(i, j int) bool {
		return seasonEpisode{newer[j].SeasonNumber, newer[j].EpisodeNumber}.after(
			seasonEpisode{newer[i].SeasonNumber, newer[i].EpisodeNumber})
	})
	for _, ep := range newer {
		numbers[ep.ID] = next
		next++
	}
	return numbers
}

// after reports whether a airs after b in season and episode order.
func (a seasonEpisode) after(b seasonEpisode) bool {
	if a.season != b.season {
		return a.season > b.season
	}
	return a.episode > b.episode
}
