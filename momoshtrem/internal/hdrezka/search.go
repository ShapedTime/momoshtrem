package hdrezka

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Kind is what a title is: a movie or a series.
type Kind string

const (
	KindMovie  Kind = "movie"
	KindSeries Kind = "series"
)

// SearchResult is one search hit.
type SearchResult struct {
	URL           string  // the title page as the site linked it
	Path          string  // e.g. /films/fiction/2259-interstellar-2014.html
	PostID        int     // the site's id for the title
	Title         string  // the Russian title
	OriginalTitle string  // "" when the site gives none (Russian titles, full search)
	Year          int     // the first year; 0 when unknown
	Category      string  // films, series, cartoons or animation
	Kind          Kind    // "" when the category doesn't tell (cartoons, anime)
	Rating        float64 // Kinopoisk rating (quick search only); 0 when none
}

// QuickSearch runs the site's search-as-you-type: at most five hits, with
// original titles and years.
func (c *Client) QuickSearch(ctx context.Context, query string) ([]SearchResult, error) {
	body, err := c.fetch(ctx, siteRequest{
		path: "/engine/ajax/search.php",
		form: url.Values{"q": {query}},
		ajax: true,
	})
	if err != nil {
		return nil, err
	}
	return parseQuickSearch(body)
}

// Search runs the full search page: more hits than QuickSearch, but without
// original titles or ratings. Only the first page is read.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	q := url.Values{"do": {"search"}, "subaction": {"search"}, "q": {query}}
	body, err := c.fetch(ctx, siteRequest{path: "/search/?" + q.Encode()})
	if err != nil {
		return nil, err
	}
	return parseSearch(body)
}

// parseQuickSearch reads the hits of ul.b-search__section_list: links to
// the titles holding span.enty (the title), a tail like "(Interstellar,
// 2014)" and span.rating.
func parseQuickSearch(body []byte) ([]SearchResult, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var out []SearchResult
	for _, list := range findAll(doc, withClass("b-search__section_list")) {
		for _, li := range findAll(list, withTag("li")) {
			a := findFirst(li, withTag("a"))
			if a == nil {
				continue
			}
			href, _ := attr(a, "href")
			r, ok := resultFromURL(href)
			if !ok {
				continue
			}
			if enty := findFirst(a, withClass("enty")); enty != nil {
				r.Title = text(enty)
			}
			if rating := findFirst(a, withClass("rating")); rating != nil {
				r.Rating, _ = strconv.ParseFloat(text(rating), 64)
			}
			tail := parseTail(ownText(a))
			r.OriginalTitle, r.Year = tail.original, tail.year
			if r.Kind == "" && tail.series {
				r.Kind = KindSeries
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// parseSearch reads div.b-content__inline_item cards: the link in
// .b-content__inline_item-link and, below it, "2014, США, Фантастика".
func parseSearch(body []byte) ([]SearchResult, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	var out []SearchResult
	for _, item := range findAll(doc, withClass("b-content__inline_item")) {
		link := findFirst(item, withClass("b-content__inline_item-link"))
		if link == nil {
			continue
		}
		a := findFirst(link, withTag("a"))
		if a == nil {
			continue
		}
		href, _ := attr(a, "href")
		r, ok := resultFromURL(href)
		if !ok {
			continue
		}
		r.Title = text(a)
		if info := findFirst(link, withTag("div")); info != nil {
			years, _, _ := strings.Cut(text(info), ",")
			r.Year = firstYear(years)
			if r.Kind == "" && isYearRange(years) {
				r.Kind = KindSeries
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// resultFromURL starts a result from a hit's link. Links that aren't title
// pages (collections, people) are skipped.
func resultFromURL(href string) (SearchResult, bool) {
	path, err := TitlePath(href)
	if err != nil {
		return SearchResult{}, false
	}
	category, postID := splitPath(path)
	return SearchResult{
		URL:      href,
		Path:     path,
		PostID:   postID,
		Category: category,
		Kind:     categoryKind(category),
	}, true
}

// categoryKind is the kind a category implies: cartoons and animation hold
// both movies and series.
func categoryKind(category string) Kind {
	switch category {
	case "films":
		return KindMovie
	case "series":
		return KindSeries
	}
	return ""
}

// tail is what a quick-search hit says after its title.
type tail struct {
	original string // "" when there is none
	year     int
	series   bool // a year range ("2008-2013", "2025 - ...") or the label "сериал"
}

// yearRe finds a year in "2014", "2008-2013" or "2025 - ...".
var yearRe = regexp.MustCompile(`(?:19|20)\d\d`)

// parseTail reads "(Interstellar, 2014)", "(Breaking Bad, сериал,
// 2008-2013)", "(сериал, 2025 - ...)" or "(1979)". The last part is the
// year; a lowercase Cyrillic part before it is a label such as "сериал",
// "мультфильм" or "аниме"; the rest is the original title, which may itself
// contain commas.
func parseTail(s string) tail {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	parts := strings.Split(s, ", ")
	var t tail
	if last := parts[len(parts)-1]; yearRe.MatchString(last) {
		t.year = firstYear(last)
		t.series = isYearRange(last)
		parts = parts[:len(parts)-1]
	}
	if n := len(parts); n > 0 && isLabel(parts[n-1]) {
		t.series = t.series || strings.Contains(parts[n-1], "сериал")
		parts = parts[:n-1]
	}
	t.original = strings.TrimSpace(strings.Join(parts, ", "))
	return t
}

func firstYear(s string) int {
	y, _ := strconv.Atoi(yearRe.FindString(s))
	return y
}

func isYearRange(s string) bool {
	return strings.Contains(s, "-")
}

// isLabel reports whether s is a lowercase Cyrillic word or two, like
// "сериал" or "мини-сериал", rather than an original title.
func isLabel(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != ' ' && r != '-' && !(unicode.Is(unicode.Cyrillic, r) && unicode.IsLower(r)) {
			return false
		}
	}
	return true
}
