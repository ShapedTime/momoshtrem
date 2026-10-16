package release

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// coverage is what a name's markers say it covers.
type coverage struct {
	se       []token.Token // SxxEyy and NxNN markers
	seasons  []int         // "S01", "Season 1-4", "Сезон: 2", "Specials" (0)
	spans    []Span        // episodes without a season of their own: "- 29", "(29-38)", "E1088", "Серии: 1-10"
	total    int           // "Серии: 1-10 из 10": the season's episode count
	half     string        // "- 28.5": a recap between episodes
	date     string        // a dated episode
	complete bool          // "Complete", "Batch"
	series   bool          // "Complete Series", "Complete Collection": the whole show even with seasons
}

// rangeWords make a following range something other than episodes:
// "Part 1-2", "CD1-2".
var rangeWords = map[string]bool{
	"part": true, "pt": true, "vol": true, "volume": true, "cd": true, "disc": true, "disk": true, "chapter": true,
}

var seasonWordsRU = map[string]bool{"сезон": true, "сезоны": true, "сезона": true}
var episodeWordsRU = map[string]bool{"серии": true, "серия": true, "серий": true}

// pack reads the tag zone's markers into a Pack, or nil when there are
// none.
func (p *parser) pack() *Pack {
	var c coverage
	dashNum := false
	for i := p.end; i < len(p.toks); i++ {
		t := p.toks[i]
		switch t.Kind {
		case token.SxxEyy, token.NxNN:
			c.se = append(c.se, t)
		case token.Season:
			for s := t.Season; s <= t.LastSeason; s++ {
				c.seasons = append(c.seasons, s)
			}
		case token.Episode:
			c.spans = append(c.spans, Span{t.Eps[0], t.Eps[len(t.Eps)-1]})
		case token.Range:
			if !isYearRange(t) && !rangeWords[p.norm(i-1)] {
				c.spans = append(c.spans, Span{t.Lo, t.Hi})
			}
		case token.Num:
			// The first number after a dash ("Frieren - 29"), or the
			// fansub-style one before the tags.
			afterDash := i > 0 && p.toks[i-1].Kind == token.Dash && !t.Bracket
			if i == p.absNum || afterDash && !dashNum {
				dashNum = true
				switch {
				case t.Half:
					c.half = t.Text
				case t.N > 0:
					c.spans = append(c.spans, Span{t.N, t.N})
				}
			}
		case token.Date:
			if c.date == "" {
				c.date = t.Date
			}
		case token.Word:
			i += p.packWord(&c, i)
		}
	}
	return c.pack()
}

// packWord reads a pack word at i and returns how many tokens after it it
// used.
func (p *parser) packWord(c *coverage, i int) int {
	n, next := p.toks[i].Norm, p.tok(i+1)
	switch {
	case n == "complete" || n == "batch":
		c.complete = true
		c.series = c.series || p.norm(i+1) == "series" || p.norm(i+1) == "collection"
	case n == "specials":
		c.seasons = append(c.seasons, 0)
	case seasonWordsRU[n] && next.Kind == token.Num:
		c.seasons = append(c.seasons, next.N)
		return 1
	case seasonWordsRU[n] && next.Kind == token.Range:
		for s := next.Lo; s <= next.Hi; s++ {
			c.seasons = append(c.seasons, s)
		}
		return 1
	case seasonWordsRU[n] && i > 0 && p.toks[i-1].Kind == token.Num: // "1 сезон"
		c.seasons = append(c.seasons, p.toks[i-1].N)
	case episodeWordsRU[n] && (next.Kind == token.Num || next.Kind == token.Range):
		span := Span{next.N, next.N}
		if next.Kind == token.Range {
			span = Span{next.Lo, next.Hi}
		}
		c.spans = append(c.spans, span)
		if p.norm(i+2) == "из" && p.tok(i+3).Kind == token.Num { // "Серии: 1-10 из 10"
			c.total = p.toks[i+3].N
			return 3
		}
		return 1
	}
	return 0
}

// tok is token i, or a zero token past the end.
func (p *parser) tok(i int) token.Token {
	if i < len(p.toks) {
		return p.toks[i]
	}
	return token.Token{}
}

func (c coverage) pack() *Pack {
	switch {
	case len(c.se) > 0:
		return c.episodePack()
	case len(c.seasons) > 0:
		return c.seasonPack()
	case len(c.spans) > 0:
		s := c.span()
		if s.From == s.To {
			return &Pack{Type: PackEpisode, Label: fmt.Sprintf("Episode %d", s.From), Seasons: []int{}, Episodes: &s, Absolute: true}
		}
		return &Pack{Type: PackMultiEpisode, Label: fmt.Sprintf("Episodes %d–%d", s.From, s.To), Seasons: []int{}, Episodes: &s, Absolute: true}
	case c.half != "":
		return &Pack{Type: PackUnknown, Label: "Episode " + c.half, Seasons: []int{}}
	case c.date != "":
		return &Pack{Type: PackEpisode, Label: c.date, Seasons: []int{}, Date: c.date}
	case c.complete:
		return &Pack{Type: PackComplete, Label: "Complete", Seasons: []int{}}
	}
	return nil
}

// episodePack is the pack of SxxEyy markers: one season's episodes, or
// unknown when they span seasons.
func (c coverage) episodePack() *Pack {
	var seasons, eps []int
	for _, t := range c.se {
		seasons = append(seasons, t.Season)
		eps = append(eps, t.Eps...)
	}
	seasons = sortedUnique(seasons)
	if len(seasons) > 1 {
		first, last := c.se[0], c.se[len(c.se)-1]
		label := seLabel(first.Season, first.Eps[0]) + "–" + seLabel(last.Season, last.Eps[len(last.Eps)-1])
		return &Pack{Type: PackUnknown, Label: label, Seasons: seasons}
	}
	return seasonEpisodes(seasons[0], Span{slices.Min(eps), slices.Max(eps)})
}

// seasonPack is the pack of season markers, maybe with episodes: "S2
// (01-04)" is S02E01–E04, but "Сезон: 1 / Серии: 1-10 из 10" is the whole
// season.
func (c coverage) seasonPack() *Pack {
	seasons := sortedUnique(c.seasons)
	if len(seasons) == 1 && len(c.spans) > 0 {
		if s := c.span(); !(c.total > 0 && s.From == 1 && s.To >= c.total) {
			return seasonEpisodes(seasons[0], s)
		}
	}
	switch {
	case c.series:
		return &Pack{Type: PackComplete, Label: "Complete", Seasons: seasons}
	case len(seasons) == 1:
		return &Pack{Type: PackSeason, Label: seasonLabel(seasons[0]), Seasons: seasons}
	}
	return &Pack{Type: PackMultiSeason, Label: seasonsLabel(seasons), Seasons: seasons}
}

// seasonEpisodes is an episode or a run of them in one season.
func seasonEpisodes(season int, s Span) *Pack {
	p := &Pack{Type: PackEpisode, Label: seLabel(season, s.From), Seasons: []int{season}, Episodes: &s}
	if s.To > s.From {
		p.Type = PackMultiEpisode
		p.Label += fmt.Sprintf("–E%02d", s.To)
	}
	return p
}

// span covers every span read.
func (c coverage) span() Span {
	s := c.spans[0]
	for _, o := range c.spans[1:] {
		s.From, s.To = min(s.From, o.From), max(s.To, o.To)
	}
	return s
}

func seLabel(season, episode int) string { return fmt.Sprintf("S%02dE%02d", season, episode) }

func seasonLabel(s int) string {
	if s == 0 {
		return "Specials"
	}
	return fmt.Sprintf("Season %d", s)
}

// seasonsLabel writes "Seasons 1–4", "Seasons 1, 3, 5" or "Season 1 +
// Specials".
func seasonsLabel(seasons []int) string {
	specials := seasons[0] == 0
	if specials {
		seasons = seasons[1:]
	}
	var label string
	switch first, last := seasons[0], seasons[len(seasons)-1]; {
	case len(seasons) == 1:
		label = seasonLabel(first)
	case last-first == len(seasons)-1:
		label = fmt.Sprintf("Seasons %d–%d", first, last)
	default:
		parts := make([]string, len(seasons))
		for i, s := range seasons {
			parts[i] = strconv.Itoa(s)
		}
		label = "Seasons " + strings.Join(parts, ", ")
	}
	if specials {
		label += " + Specials"
	}
	return label
}

func sortedUnique(s []int) []int {
	s = slices.Clone(s)
	slices.Sort(s)
	return slices.Compact(s)
}
