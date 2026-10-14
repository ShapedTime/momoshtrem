package catalog

import (
	"sort"
	"strconv"
	"time"

	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

const (
	// rangeMinEpisodes: only seasons with more episodes get ranges.
	rangeMinEpisodes = 20
	// rangeBlockSize is the most episodes in a block.
	rangeBlockSize = 10
	// rangeGapDays: air dates further apart than this start a new block.
	rangeGapDays = 45
	// rangeArcCoverage: story arcs are used only when they cover at least
	// this percentage of the season's episodes.
	rangeArcCoverage = 80
)

// Arc is one group of a TMDB story-arc episode group: its name and the
// in-season numbers of the episodes it lists in one season, ascending.
type Arc struct {
	Name     string
	Episodes []int
}

// ArcsForSeason returns the arcs of a story-arc episode group (TMDB type 5)
// that list episodes of season, in group order.
func ArcsForSeason(group *tmdb.EpisodeGroup, season int) []Arc {
	if group == nil {
		return nil
	}
	groups := append([]tmdb.EpisodeGroupGroup(nil), group.Groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Order < groups[j].Order })

	var arcs []Arc
	for _, g := range groups {
		arc := Arc{Name: g.Name}
		for _, ep := range g.Episodes {
			if ep.SeasonNumber == season {
				arc.Episodes = append(arc.Episodes, ep.EpisodeNumber)
			}
		}
		if len(arc.Episodes) > 0 {
			sort.Ints(arc.Episodes)
			arcs = append(arcs, arc)
		}
	}
	return arcs
}

// Ranges cuts a season of more than 20 episodes (in episode order) into
// jump targets. Ranges never overlap: every episode is in exactly one, so
// it counts towards one range's missing. Shorter seasons get none.
//
// A regular season uses its TMDB story arcs when they share no episode and
// cover at least 80% of it. An arc becomes one range per run of consecutive
// episodes it lists; when other episodes interrupt it, the later runs are
// labelled "Name (2)", "Name (3)" and so on. The episodes no arc covers (arc
// groups lag behind airing shows), and seasons without usable arcs, are
// split where air dates are more than 45 days apart, then cut into blocks of
// up to 10. Specials never use arcs and aren't split at long breaks: they
// air years apart.
func Ranges(seasonNumber int, episodes []EpisodeSummary, arcs []Arc) []EpisodeRange {
	ranges := []EpisodeRange{}
	if len(episodes) <= rangeMinEpisodes {
		return ranges
	}

	var owners []int
	if seasonNumber > 0 {
		owners = arcOwners(episodes, arcs)
	}
	owner := func(i int) int {
		if owners == nil {
			return -1
		}
		return owners[i]
	}

	runsOf := make([]int, len(arcs)) // runs emitted so far, per arc
	start := 0
	for i := 1; i <= len(episodes); i++ {
		if i < len(episodes) && owner(i) == owner(start) &&
			(owner(i) >= 0 || seasonNumber == 0 || !longBreak(episodes[i-1].AirDate, episodes[i].AirDate)) {
			continue
		}
		run := episodes[start:i]
		if a := owner(start); a >= 0 {
			runsOf[a]++
			label := arcs[a].Name
			if runsOf[a] > 1 {
				label += " (" + strconv.Itoa(runsOf[a]) + ")"
			}
			ranges = append(ranges, rangeOf(label, "arc", run))
		} else {
			for b := 0; b < len(run); b += rangeBlockSize {
				chunk := run[b:min(b+rangeBlockSize, len(run))]
				label := blockLabel(chunk[0].EpisodeNumber, chunk[len(chunk)-1].EpisodeNumber)
				ranges = append(ranges, rangeOf(label, "block", chunk))
			}
		}
		start = i
	}
	return ranges
}

// arcOwners maps each episode to the index of the arc that lists it, or -1
// when none does. It returns nil, so blocks are used instead, when two arcs
// list the same episode or the arcs cover less than rangeArcCoverage percent
// of the season. Arc episodes the season lacks are ignored.
func arcOwners(episodes []EpisodeSummary, arcs []Arc) []int {
	if len(arcs) == 0 {
		return nil
	}
	index := make(map[int]int, len(episodes))
	owners := make([]int, len(episodes))
	for i, ep := range episodes {
		index[ep.EpisodeNumber] = i
		owners[i] = -1
	}
	covered := 0
	for a, arc := range arcs {
		for _, n := range arc.Episodes {
			i, ok := index[n]
			switch {
			case !ok || owners[i] == a:
				// Not in the season, or listed twice by the same arc.
			case owners[i] >= 0:
				return nil // two arcs share an episode
			default:
				owners[i] = a
				covered++
			}
		}
	}
	if covered*100 < len(episodes)*rangeArcCoverage {
		return nil
	}
	return owners
}

// rangeOf is the range over run, a non-empty slice of consecutive episodes:
// its in-season and absolute bounds and its missing count.
func rangeOf(label, kind string, run []EpisodeSummary) EpisodeRange {
	r := EpisodeRange{Label: label, Kind: kind, From: run[0].EpisodeNumber, To: run[len(run)-1].EpisodeNumber}
	for _, ep := range run {
		if ep.AbsoluteNumber != nil {
			if r.AbsoluteFrom == nil {
				r.AbsoluteFrom = copyInt(ep.AbsoluteNumber)
			}
			r.AbsoluteTo = copyInt(ep.AbsoluteNumber)
		}
		if ep.State == EpisodeMissing {
			r.Missing++
		}
	}
	return r
}

// longBreak reports whether two air dates are more than rangeGapDays apart.
// Unknown dates never break.
func longBreak(prev, next *string) bool {
	if prev == nil || next == nil {
		return false
	}
	a, errA := time.Parse(time.DateOnly, *prev)
	b, errB := time.Parse(time.DateOnly, *next)
	if errA != nil || errB != nil {
		return false
	}
	return b.Sub(a) > rangeGapDays*24*time.Hour
}

// blockLabel is "21–28", or "31" for a single episode.
func blockLabel(from, to int) string {
	if from == to {
		return strconv.Itoa(from)
	}
	return strconv.Itoa(from) + "–" + strconv.Itoa(to)
}
