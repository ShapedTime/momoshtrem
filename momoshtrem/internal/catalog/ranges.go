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
)

// Arc is a TMDB story arc's span within one season, in in-season episode
// numbers.
type Arc struct {
	Name     string
	From, To int
}

// ArcsForSeason returns the arcs of a story-arc episode group (TMDB type 5)
// that overlap season, in group order.
func ArcsForSeason(group *tmdb.EpisodeGroup, season int) []Arc {
	if group == nil {
		return nil
	}
	groups := append([]tmdb.EpisodeGroupGroup(nil), group.Groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Order < groups[j].Order })

	var arcs []Arc
	for _, g := range groups {
		arc := Arc{Name: g.Name, From: -1}
		for _, ep := range g.Episodes {
			if ep.SeasonNumber != season {
				continue
			}
			if arc.From < 0 || ep.EpisodeNumber < arc.From {
				arc.From = ep.EpisodeNumber
			}
			if ep.EpisodeNumber > arc.To {
				arc.To = ep.EpisodeNumber
			}
		}
		if arc.From >= 0 {
			arcs = append(arcs, arc)
		}
	}
	return arcs
}

// Ranges cuts a season of more than 20 episodes (in episode order) into
// jump targets. TMDB story arcs that overlap the season come first;
// episodes no arc covers (arc groups lag behind airing shows) and seasons
// without arcs are split where air dates are more than 45 days apart (not
// for specials, which air years apart), then into blocks of up to 10. Each
// range counts its missing episodes. Shorter seasons get none.
func Ranges(seasonNumber int, episodes []EpisodeSummary, arcs []Arc) []EpisodeRange {
	ranges := []EpisodeRange{}
	if len(episodes) <= rangeMinEpisodes {
		return ranges
	}

	covered := func(n int) bool {
		for _, a := range arcs {
			if n >= a.From && n <= a.To {
				return true
			}
		}
		return false
	}
	for _, a := range arcs {
		ranges = append(ranges, summarizeRange(EpisodeRange{Label: a.Name, Kind: "arc", From: a.From, To: a.To}, episodes))
	}

	// Runs of consecutive uncovered episodes, split at long breaks.
	var runs [][]EpisodeSummary
	var run []EpisodeSummary
	for i, ep := range episodes {
		if covered(ep.EpisodeNumber) {
			if len(run) > 0 {
				runs, run = append(runs, run), nil
			}
			continue
		}
		if len(run) > 0 && seasonNumber > 0 && longBreak(episodes[i-1].AirDate, ep.AirDate) {
			runs, run = append(runs, run), nil
		}
		run = append(run, ep)
	}
	if len(run) > 0 {
		runs = append(runs, run)
	}

	for _, run := range runs {
		for start := 0; start < len(run); start += rangeBlockSize {
			chunk := run[start:min(start+rangeBlockSize, len(run))]
			from, to := chunk[0].EpisodeNumber, chunk[len(chunk)-1].EpisodeNumber
			ranges = append(ranges, summarizeRange(EpisodeRange{Label: blockLabel(from, to), Kind: "block", From: from, To: to}, episodes))
		}
	}

	sort.SliceStable(ranges, func(i, j int) bool { return ranges[i].From < ranges[j].From })
	return ranges
}

// summarizeRange fills r's absolute numbers and missing count from the
// episodes inside it.
func summarizeRange(r EpisodeRange, episodes []EpisodeSummary) EpisodeRange {
	for _, ep := range episodes {
		if ep.EpisodeNumber < r.From || ep.EpisodeNumber > r.To {
			continue
		}
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
