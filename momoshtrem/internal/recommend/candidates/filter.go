package candidates

import "strings"

// Filter narrows a pool: get_candidates' input. Zero fields don't filter;
// a bound on runtime or year leaves out candidates without one.
type Filter struct {
	MediaType  string   // movie or tv
	Genres     []string // any of them, by name, case-insensitively
	MinRuntime int      // minutes: a movie's runtime, a show's episode runtime
	MaxRuntime int
	YearFrom   int
	YearTo     int
	Language   string // original language, ISO 639-1
	Anime      *bool  // true: anime only; false: no anime
	Limit      int    // at most this many; DefaultLimit when 0
}

// Get_candidates' limits.
const (
	DefaultLimit = 30
	MaxLimit     = 60
)

// tvGenres are TMDB's TV genres that cover two movie genres: a filter on
// either finds them.
var tvGenres = map[string]string{
	"science fiction": "sci-fi & fantasy",
	"fantasy":         "sci-fi & fantasy",
	"action":          "action & adventure",
	"adventure":       "action & adventure",
	"war":             "war & politics",
}

// Filter returns the candidates f lets through, best first, up to its
// limit, and how many matched in all.
func (p *Pool) Filter(f Filter) (items []Candidate, matched int) {
	all := p.Match(f)
	return all[:min(len(all), f.EffectiveLimit())], len(all)
}

// Match returns every candidate f lets through, best first, whatever its
// limit.
func (p *Pool) Match(f Filter) []Candidate {
	items := []Candidate{}
	for _, c := range p.Items {
		if f.matches(&c) {
			items = append(items, c)
		}
	}
	return items
}

// EffectiveLimit is f's limit within 1–MaxLimit, DefaultLimit when unset.
func (f Filter) EffectiveLimit() int {
	if f.Limit <= 0 {
		return DefaultLimit
	}
	return min(f.Limit, MaxLimit)
}

func (f Filter) matches(c *Candidate) bool {
	switch {
	case f.MediaType != "" && c.MediaType != f.MediaType:
		return false
	case f.Language != "" && !strings.EqualFold(c.Language, f.Language):
		return false
	case f.Anime != nil && c.Anime != *f.Anime:
		return false
	case (f.MinRuntime > 0 || f.MaxRuntime > 0) && c.Runtime == 0:
		return false
	case f.MinRuntime > 0 && c.Runtime < f.MinRuntime:
		return false
	case f.MaxRuntime > 0 && c.Runtime > f.MaxRuntime:
		return false
	case (f.YearFrom > 0 || f.YearTo > 0) && c.year == 0:
		return false
	case f.YearFrom > 0 && c.year < f.YearFrom:
		return false
	case f.YearTo > 0 && c.year > f.YearTo:
		return false
	}
	return len(f.Genres) == 0 || hasGenre(c.Genres, f.Genres)
}

func hasGenre(have, want []string) bool {
	for _, w := range want {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		for _, h := range have {
			h = strings.ToLower(h)
			if h == w || tvGenres[w] == h || strings.Contains(h, w) {
				return true
			}
		}
	}
	return false
}
