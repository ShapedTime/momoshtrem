package catalog

import (
	"sort"
	"strconv"
)

// EpisodeFact is what availability needs to know about one episode.
type EpisodeFact struct {
	Number      int
	Absolute    *int
	AirDate     string // YYYY-MM-DD; "" when unknown
	HasFile     bool
	Downloading bool // HasFile and its file is being fetched
	// UnderReview: a file of an open mapping job targets it (PLAN §6
	// "Review rules"). It's needs_review only without a file of its own.
	UnderReview bool
	// Watched: Trakt has a play of it (B5). It wins over the rest.
	Watched bool
}

// SeasonFacts is one season of a library show and its episodes.
type SeasonFacts struct {
	Number   int // 0 holds specials
	Name     string
	Episodes []EpisodeFact
}

// ShowSummary is what a show's title page derives from its episodes.
type ShowSummary struct {
	Seasons []SeasonSummary // regular seasons in order, specials last
	// EpisodesTotal counts regular episodes that aired or have a file;
	// EpisodesReady those with a file.
	EpisodesTotal int
	EpisodesReady int
	AnyFile       bool // any episode, specials included, has a file
	Downloading   bool // any episode with a file is being fetched
	Frontier      bool // a regular episode aired after the last one with a file or a play
	Missing       *MissingRun
	DefaultSeason *int
}

// SummarizeShow derives the title page's season summaries and availability
// from a library show's visible seasons (without season 0 while specials are
// off), in any order. A season's ready counts episodes with a file, watched
// ones too; missing, needs_review and upcoming leave watched ones out.
func SummarizeShow(seasons []SeasonFacts, today string) ShowSummary {
	ordered := orderSeasons(seasons)

	var sum ShowSummary
	sum.Seasons = make([]SeasonSummary, 0, len(ordered))
	for _, season := range ordered {
		s := SeasonSummary{
			SeasonNumber:  season.Number,
			Name:          SeasonName(season.Number, season.Name),
			EpisodesTotal: len(season.Episodes),
		}
		for _, ep := range season.Episodes {
			if ep.HasFile {
				s.Ready++
			}
			switch EpisodeStateOf(ep.Watched, ep.HasFile, ep.Downloading, ep.UnderReview, ep.AirDate, today) {
			case EpisodeNeedsReview:
				s.NeedsReview++
			case EpisodeMissing:
				s.Missing++
			case EpisodeUpcoming:
				s.Upcoming++
			}
			if ep.Absolute != nil {
				if s.AbsoluteFrom == nil || *ep.Absolute < *s.AbsoluteFrom {
					s.AbsoluteFrom = intPtr(*ep.Absolute)
				}
				if s.AbsoluteTo == nil || *ep.Absolute > *s.AbsoluteTo {
					s.AbsoluteTo = intPtr(*ep.Absolute)
				}
			}
			sum.AnyFile = sum.AnyFile || ep.HasFile
			sum.Downloading = sum.Downloading || (ep.HasFile && ep.Downloading)
			if season.Number > 0 && (ep.HasFile || Aired(ep.AirDate, today)) {
				sum.EpisodesTotal++
			}
			if season.Number > 0 && ep.HasFile {
				sum.EpisodesReady++
			}
		}
		sum.Seasons = append(sum.Seasons, s)
	}

	sum.Frontier, sum.Missing = FirstMissingRun(ordered, today)
	sum.DefaultSeason = DefaultSeason(ordered, sum.Missing)
	return sum
}

// FirstMissingRun finds the frontier: aired regular episodes after the last
// one with a file or a play (all of them when there's none). Specials never
// count, nor do gaps before that episode. It's about files: an episode
// under review still belongs to it. It returns whether the frontier is
// non-empty and its first run of consecutive aired episodes within one
// season.
func FirstMissingRun(seasons []SeasonFacts, today string) (frontier bool, run *MissingRun) {
	type position struct {
		season int
		ep     EpisodeFact
	}
	var regular []position
	for _, season := range orderSeasons(seasons) {
		if season.Number == 0 {
			continue
		}
		eps := append([]EpisodeFact(nil), season.Episodes...)
		sort.SliceStable(eps, func(i, j int) bool { return eps[i].Number < eps[j].Number })
		for _, ep := range eps {
			regular = append(regular, position{season.Number, ep})
		}
	}

	last := -1
	for i, p := range regular {
		if p.ep.HasFile || p.ep.Watched {
			last = i
		}
	}
	rest := regular[last+1:]

	start := -1
	for i, p := range rest {
		if Aired(p.ep.AirDate, today) {
			start = i
			break
		}
	}
	if start < 0 {
		return false, nil
	}
	end := start
	for end+1 < len(rest) && rest[end+1].season == rest[start].season && Aired(rest[end+1].ep.AirDate, today) {
		end++
	}

	first, final := rest[start], rest[end]
	return true, &MissingRun{
		SeasonNumber: first.season,
		From:         first.ep.Number,
		To:           final.ep.Number,
		AbsoluteFrom: copyInt(first.ep.Absolute),
		AbsoluteTo:   copyInt(final.ep.Absolute),
		Count:        end - start + 1,
	}
}

// DefaultSeason is the season a show's page opens on, unless Trakt names a
// next episode in a season the page lists (ShowTitle prefers that one): the
// frontier's, else the latest regular season with a file, else the first
// regular season, else specials. Nil when the show has no seasons.
func DefaultSeason(seasons []SeasonFacts, missing *MissingRun) *int {
	if missing != nil {
		return intPtr(missing.SeasonNumber)
	}
	ordered := orderSeasons(seasons)
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].Number == 0 {
			continue
		}
		for _, ep := range ordered[i].Episodes {
			if ep.HasFile {
				return intPtr(ordered[i].Number)
			}
		}
	}
	if len(ordered) > 0 {
		return intPtr(ordered[0].Number) // the first regular one, or specials alone
	}
	return nil
}

// orderSeasons returns regular seasons in ascending order, then specials.
func orderSeasons(seasons []SeasonFacts) []SeasonFacts {
	out := append([]SeasonFacts(nil), seasons...)
	sort.SliceStable(out, func(i, j int) bool { return seasonLess(out[i].Number, out[j].Number) })
	return out
}

// seasonLess orders regular seasons ascending and season 0 (specials) last.
func seasonLess(a, b int) bool {
	if (a == 0) != (b == 0) {
		return b == 0
	}
	return a < b
}

// SeasonName is the stored season name, or TMDB's default when there is
// none.
func SeasonName(number int, name string) string {
	switch {
	case name != "":
		return name
	case number == 0:
		return "Specials"
	}
	return "Season " + strconv.Itoa(number)
}

func intPtr(n int) *int { return &n }

func copyInt(p *int) *int {
	if p == nil {
		return nil
	}
	return intPtr(*p)
}
