package activity

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Facts are what a torrent's state is computed from.
type Facts struct {
	Paused   bool // the user paused it, loaded or not
	Loaded   bool // the torrent client has it
	Readers  int  // open readers of its files (#99: streaming sessions)
	Active   bool // its network is on: not idled by the activity manager
	Complete bool // every piece is on disk
}

// StateOf is a torrent's state: the first rule that matches wins.
//   - paused: the user paused it;
//   - streaming: loaded, with a reader open;
//   - downloading: loaded, its network on, and not complete;
//   - idle: anything else, including a torrent that isn't loaded.
func StateOf(f Facts) State {
	switch {
	case f.Paused:
		return StatePaused
	case f.Loaded && f.Readers > 0:
		return StateStreaming
	case f.Loaded && f.Active && !f.Complete:
		return StateDownloading
	default:
		return StateIdle
	}
}

// stateRank orders the list: streaming, downloading, paused, idle.
func stateRank(s State) int {
	switch s {
	case StateStreaming:
		return 0
	case StateDownloading:
		return 1
	case StatePaused:
		return 2
	default:
		return 3
	}
}

// Scope names the episodes of one show a torrent fills:
//   - whole seasons: "Season 1", or "Seasons 1–3" for a run of them, and
//     "Specials" for all of season 0;
//   - a run of episodes in one season: "S01E29–E38", or "S01E05" alone;
//   - anything more: the first of those and how many other episodes,
//     "S01E01–E05 +2".
//
// A season is whole when every episode it has (seasonSizes) is there.
// Seasons go in number order with Specials last. nil without episodes.
func Scope(episodes []library.EpisodeNumber, seasonSizes map[int]int) *string {
	eps := slices.Clone(episodes)
	slices.SortFunc(eps, func(x, y library.EpisodeNumber) int {
		return cmp.Or(cmp.Compare(seasonOrder(x.Season), seasonOrder(y.Season)), cmp.Compare(x.Episode, y.Episode))
	})
	eps = slices.Compact(eps)
	if len(eps) == 0 {
		return nil
	}

	var segs []segment
	for i := 0; i < len(eps); {
		season := eps[i].Season
		j := i
		for j < len(eps) && eps[j].Season == season {
			j++
		}
		inSeason := eps[i:j]
		i = j

		if size := seasonSizes[season]; size > 0 && len(inSeason) >= size {
			last := len(segs) - 1
			if season > 0 && last >= 0 && segs[last].whole && segs[last].to > 0 && segs[last].to == season-1 {
				segs[last].to = season
				segs[last].count += len(inSeason)
				continue
			}
			segs = append(segs, segment{whole: true, season: season, from: season, to: season, count: len(inSeason)})
			continue
		}
		for k := 0; k < len(inSeason); {
			l := k + 1
			for l < len(inSeason) && inSeason[l].Episode == inSeason[l-1].Episode+1 {
				l++
			}
			segs = append(segs, segment{season: season, from: inSeason[k].Episode, to: inSeason[l-1].Episode, count: l - k})
			k = l
		}
	}

	label := segs[0].label()
	if rest := len(eps) - segs[0].count; rest > 0 {
		label = fmt.Sprintf("%s +%d", label, rest)
	}
	return &label
}

// segment is whole seasons from–to, or episodes from–to of one season.
type segment struct {
	whole    bool
	season   int
	from, to int
	count    int // episodes in it
}

func (s segment) label() string {
	switch {
	case s.whole && s.from == 0:
		return "Specials"
	case s.whole && s.from == s.to:
		return fmt.Sprintf("Season %d", s.from)
	case s.whole:
		return fmt.Sprintf("Seasons %d–%d", s.from, s.to)
	case s.from == s.to:
		return fmt.Sprintf("S%02dE%02d", s.season, s.from)
	default:
		return fmt.Sprintf("S%02dE%02d–E%02d", s.season, s.from, s.to)
	}
}

// seasonOrder puts Specials (season 0) after the regular seasons.
func seasonOrder(season int) int {
	if season == 0 {
		return 1 << 30
	}
	return season
}
