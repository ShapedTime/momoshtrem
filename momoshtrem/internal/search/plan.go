// Package search is torrent search across Jackett's indexers (PLAN §6
// "Search"). Plan decides what to ask for; it's pure, so a plan is the
// same for the same title and request.
package search

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// Scope is what a TV search keeps: releases with the episode, releases
// with the season, or anything naming the show.
type Scope string

const (
	ScopeEpisode Scope = "episode"
	ScopeSeason  Scope = "season"
	ScopeAny     Scope = "any"
)

// Query labels (SearchQuery.label). "" is null: the primary query, the ID
// query, and an anime episode's TMDB SxxEyy when no scene numbering
// replaces it.
const (
	LabelOriginalTitle = "original title"
	LabelAbsolute      = identify.LabelAbsolute
	LabelScene         = identify.LabelScene
	LabelWholeShow     = "whole show"
)

// SearchType is a Torznab search function (t=).
type SearchType string

const (
	TypeSearch   SearchType = "search"
	TypeTVSearch SearchType = "tvsearch"
	TypeMovie    SearchType = "movie"
)

// ErrBadRequest is a request Plan can't make a plan for (400 bad_request):
// a scope without its season or episode, an episode without a season, an
// unknown scope, a negative season or an episode below 1.
var ErrBadRequest = errors.New("bad search request")

// Request is a search's context besides the title. Season, Episode and
// Scope are for TV and ignored for movies.
type Request struct {
	Season  *int
	Episode *int // needs Season
	// Scope defaults to episode with an episode, season with only a season,
	// else any.
	Scope Scope
	// Q is the user's own query, sent only when they edited the field. It
	// replaces the generated text queries and the ID query.
	Q string
}

// Params are one Torznab request's parameters, before the indexer's caps
// are applied: an indexer gets only the ids it supports, and the ID query
// is skipped for one that supports none.
type Params struct {
	Type   SearchType
	Q      string
	Season *int
	Ep     *int
	IMDbID string // "tt0816692"; ID queries only
	TVDBID int
	TMDBID int
}

// Query is one query of a plan.
type Query struct {
	// Text is what the UI lists (SearchQuery.q); "" for the ID query,
	// which it doesn't.
	Text string
	// Label is "" (null) or one of the Label constants.
	Label  string
	Params Params
}

// IsID reports whether q searches by ids rather than text.
func (q Query) IsID() bool {
	return q.Params.IMDbID != "" || q.Params.TVDBID != 0 || q.Params.TMDBID != 0
}

// QueryPlan is what a search asks the indexers.
type QueryPlan struct {
	// Key caches the search: media type, TMDB id, scope, season, episode
	// and q. Filters, sort and paging aren't part of it.
	Key string
	// Scope, Season and Episode are the request's, with the scope's
	// default applied; all empty for movies.
	Scope   Scope
	Season  *int
	Episode *int
	Q       string // the user's query, trimmed; "" when the plan generated its own
	// Queries are sent first: the primary text query (the field shows it),
	// then the ID query, then the other text queries.
	Queries []Query
	// Fallback is sent when Queries find nothing in scope: the show by name
	// ("whole show"), its results filtered by coverage. Empty for movies,
	// scope any and a user query.
	Fallback []Query
}

// TextQueries are Queries without the ID query, primary first, as the
// response lists them (plus the fallback once it's sent).
func (p *QueryPlan) TextQueries() []Query {
	var out []Query
	for _, q := range p.Queries {
		if !q.IsID() {
			out = append(out, q)
		}
	}
	return out
}

// Plan makes the query plan of a search for tc (PLAN §6 "Search"):
//   - an ID query: every id tc knows (IMDb, TVDB, TMDB), with the season
//     and episode for TV;
//   - normalised text queries: "Title Year" for a movie, "Title S01E05" for
//     an episode, "Title S01" for a season, "Title" for scope any, each
//     again with the original title when it's in Latin letters ("original
//     title");
//   - anime uses its short title (before ":"), numbers an episode by its
//     absolute number ("Frieren 29", the primary the field pre-fills),
//     uses the romaji title for "original title" ("Sousou no Frieren 29"),
//     adds scene numbering ("Frieren S02E01"; for a season, the scene
//     seasons it holds) and, without scene numbering, TMDB's SxxEyy;
//   - a show numbered absolutely that isn't anime adds "Title 17"
//     ("absolute number");
//   - for the episode and season scopes, a "whole show" fallback.
//
// With q, the plan is q alone.
func Plan(tc *service.TitleContext, req Request) (*QueryPlan, error) {
	if tc == nil {
		return nil, fmt.Errorf("%w: no title", ErrBadRequest)
	}
	p := &QueryPlan{Q: strings.Join(strings.Fields(req.Q), " ")}
	switch tc.MediaType {
	case library.JobMovie:
	case library.JobTV:
		scope, err := scopeOf(req)
		if err != nil {
			return nil, err
		}
		p.Scope, p.Season, p.Episode = scope, copyInt(req.Season), copyInt(req.Episode)
	default:
		return nil, fmt.Errorf("%w: unknown media type %q", ErrBadRequest, tc.MediaType)
	}
	p.Key = fmt.Sprintf("%s:%d:%s:%s:%s:%s", tc.MediaType, tc.TMDBID, p.Scope, number(p.Season), number(p.Episode), p.Q)

	if p.Q != "" {
		p.Queries = []Query{{Text: p.Q, Params: Params{Type: TypeSearch, Q: p.Q}}}
		return p, nil
	}
	b := newBuilder(tc)
	switch {
	case tc.MediaType == library.JobMovie:
		b.movie()
	case p.Scope == ScopeEpisode:
		b.episode(*p.Season, *p.Episode)
	case p.Scope == ScopeSeason:
		b.season(*p.Season)
	default:
		b.show()
	}
	p.Queries, p.Fallback = b.queries, b.fallback
	return p, nil
}

// scopeOf checks a TV request and applies the scope's default.
func scopeOf(req Request) (Scope, error) {
	switch {
	case req.Season != nil && *req.Season < 0:
		return "", fmt.Errorf("%w: season must be 0 or more", ErrBadRequest)
	case req.Episode != nil && *req.Episode < 1:
		return "", fmt.Errorf("%w: episode must be 1 or more", ErrBadRequest)
	case req.Episode != nil && req.Season == nil:
		return "", fmt.Errorf("%w: an episode needs a season", ErrBadRequest)
	}
	scope := req.Scope
	if scope == "" {
		switch {
		case req.Episode != nil:
			scope = ScopeEpisode
		case req.Season != nil:
			scope = ScopeSeason
		default:
			scope = ScopeAny
		}
	}
	switch scope {
	case ScopeEpisode:
		if req.Episode == nil {
			return "", fmt.Errorf("%w: scope episode needs an episode", ErrBadRequest)
		}
	case ScopeSeason:
		if req.Season == nil {
			return "", fmt.Errorf("%w: scope season needs a season", ErrBadRequest)
		}
	case ScopeAny:
	default:
		return "", fmt.Errorf("%w: unknown scope %q", ErrBadRequest, scope)
	}
	return scope, nil
}

// builder collects a plan's queries, each text once.
type builder struct {
	tc        *service.TitleContext
	anime     bool
	main, alt string // the title queries lead with, and the original (or romaji) one; alt is "" without one
	queries   []Query
	fallback  []Query
	seen      map[string]bool
}

func newBuilder(tc *service.TitleContext) *builder {
	b := &builder{tc: tc, anime: tc.IsAnime && tc.MediaType == library.JobTV, seen: map[string]bool{}}
	short := func(s string) string { return s }
	if b.anime {
		short = shortTitle
	}
	b.main = queryTitle(short(tc.Title))
	switch {
	case b.anime && tc.Romaji != "":
		b.alt = queryTitle(short(tc.Romaji))
	case hasLatinLetter(tc.OriginalTitle):
		b.alt = queryTitle(short(tc.OriginalTitle))
	}
	if strings.EqualFold(b.alt, b.main) {
		b.alt = ""
	}
	return b
}

func (b *builder) text(text, label string) {
	key := strings.ToLower(text)
	if text == "" || b.seen[key] {
		return
	}
	b.seen[key] = true
	b.queries = append(b.queries, Query{Text: text, Label: label, Params: Params{Type: TypeSearch, Q: text}})
}

func (b *builder) id(t SearchType, season, episode *int) {
	b.queries = append(b.queries, Query{Params: Params{
		Type: t, Season: copyInt(season), Ep: copyInt(episode),
		IMDbID: b.tc.IMDbID, TVDBID: b.tc.TVDBID, TMDBID: b.tc.TMDBID,
	}})
}

// withAlt adds the original-title form of a query: the alternative title
// followed by suffix.
func (b *builder) withAlt(suffix string) {
	if b.alt != "" {
		b.text(join(b.alt, suffix), LabelOriginalTitle)
	}
}

func (b *builder) wholeShow() {
	for _, title := range []string{b.main, b.alt} {
		if title != "" {
			b.fallback = append(b.fallback, Query{Text: title, Label: LabelWholeShow, Params: Params{Type: TypeSearch, Q: title}})
		}
	}
}

func (b *builder) movie() {
	year := ""
	if b.tc.Year > 0 {
		year = strconv.Itoa(b.tc.Year)
	}
	b.text(join(b.main, year), "")
	b.id(TypeMovie, nil, nil)
	b.withAlt(year)
}

func (b *builder) episode(season, episode int) {
	se := sxxeyy(season, episode)
	nums := identify.ReleaseNumbers(b.tc.Catalog, season, episode)
	abs := 0
	var scenes []identify.ReleaseNumber
	for _, n := range nums {
		switch n.Label {
		case identify.LabelAbsolute:
			abs = n.Episode
		case identify.LabelScene:
			scenes = append(scenes, n)
		}
	}

	if !b.anime {
		b.text(join(b.main, se), "")
		b.id(TypeTVSearch, &season, &episode)
		b.withAlt(se)
		if abs > 0 && numberedAbsolutely(b.tc.Catalog) {
			b.text(join(b.main, absText(abs)), LabelAbsolute)
		}
		b.wholeShow()
		return
	}

	num := se
	if abs > 0 {
		num = absText(abs)
	}
	b.text(join(b.main, num), "")
	b.id(TypeTVSearch, &season, &episode)
	b.withAlt(num)
	for _, n := range scenes {
		b.text(join(b.main, sxxeyy(n.Season, n.Episode)), LabelScene)
	}
	if abs > 0 && len(scenes) == 0 {
		b.text(join(b.main, se), "")
	}
	b.wholeShow()
}

func (b *builder) season(season int) {
	s := fmt.Sprintf("S%02d", season)
	b.text(join(b.main, s), "")
	b.id(TypeTVSearch, &season, nil)
	b.withAlt(s)
	if b.anime {
		for _, rs := range sceneSeasons(b.tc.Catalog, season) {
			b.text(join(b.main, fmt.Sprintf("S%02d", rs)), LabelScene)
		}
	}
	b.wholeShow()
}

func (b *builder) show() {
	b.text(b.main, "")
	b.id(TypeTVSearch, nil, nil)
	b.withAlt("")
}

// sceneSeasons are the release seasons scene numbering puts TMDB season
// s's episodes in, other than s: Frieren's TMDB S01 holds release S02.
func sceneSeasons(cat *identify.ShowCatalog, s int) []int {
	if cat == nil || s == 0 {
		return nil
	}
	n := identify.NewNumbering(cat)
	found := map[int]bool{}
	for _, ep := range cat.Episodes {
		if ep.Season != s {
			continue
		}
		for _, rn := range n.ReleaseNumbers(ep.Season, ep.Number) {
			if rn.Label == identify.LabelScene && rn.Season != s {
				found[rn.Season] = true
			}
		}
	}
	out := make([]int, 0, len(found))
	for rs := range found {
		out = append(out, rs)
	}
	sort.Ints(out)
	return out
}

// numberedAbsolutely reports a show the library numbers absolutely (its
// ordering is absolute or an episode group).
func numberedAbsolutely(cat *identify.ShowCatalog) bool {
	return cat != nil && (cat.Ordering == identify.OrderingAbsolute || cat.Ordering == identify.OrderingEpisodeGroup)
}

// shortTitle is the part of a title before ":" ("Frieren" of "Frieren:
// Beyond Journey's End"), when it has at least three letters or digits.
func shortTitle(title string) string {
	before, _, found := strings.Cut(title, ":")
	if !found {
		return title
	}
	n := 0
	for _, r := range before {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	if n < 3 {
		return title // "Re:Zero"
	}
	return before
}

// queryTitle normalises a title the way release names write it: marks
// dropped ("Shōgun" is "Shogun"), "&" as "and", apostrophes gone
// ("Journey's" is "Journeys"), other punctuation (": ! , . ?") a space. A
// hyphen inside a word stays ("Spider-Man", "9-1-1").
func queryTitle(s string) string {
	foldMarks := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC) // not safe to share
	if folded, _, err := transform.String(foldMarks, s); err == nil {
		s = folded
	}
	s = strings.NewReplacer("&", " and ", "'", "", "’", "", "‘", "", "`", "", "´", "").Replace(s)
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	rs := []rune(s)
	var out strings.Builder
	for i, r := range rs {
		switch {
		case word(r):
			out.WriteRune(r)
		case r == '-' && i > 0 && i+1 < len(rs) && word(rs[i-1]) && word(rs[i+1]):
			out.WriteRune(r)
		default:
			out.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func hasLatinLetter(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func join(title, suffix string) string {
	if suffix == "" {
		return title
	}
	return title + " " + suffix
}

func sxxeyy(season, episode int) string { return fmt.Sprintf("S%02dE%02d", season, episode) }

// absText writes an absolute number as releases do: at least two digits.
func absText(n int) string { return fmt.Sprintf("%02d", n) }

func number(p *int) string {
	if p == nil {
		return "-"
	}
	return strconv.Itoa(*p)
}

func copyInt(p *int) *int {
	if p == nil {
		return nil
	}
	n := *p
	return &n
}
