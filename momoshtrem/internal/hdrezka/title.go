package hdrezka

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Title is a title page: what it is, its translations and, for a series,
// the seasons and episodes of the default translation (Episodes lists
// another translation's).
type Title struct {
	Path          string
	PostID        int
	Kind          Kind
	Category      string // films, series, cartoons or animation
	Title         string // the Russian title
	OriginalTitle string // "" for Russian titles
	Year          int    // 0 when the page doesn't say
	Translators   []Translator
	Seasons       []Season // nil for a movie
}

// Translator is one translation of a title: a dub, a voice-over, or the
// original audio ("Оригинал (+субтитры)").
type Translator struct {
	ID int
	// Name is the site's name, with the language of a flagged one in
	// brackets: "HDrezka Studio (Украинский)". "" when the page names none.
	Name     string
	Default  bool // the one the page plays first
	Premium  bool // needs a Premium account; the CDN serves a teaser without one
	Camrip   bool
	Ads      bool // has ads burned in
	Director bool // director's cut
}

// Season is a season number and its episode numbers, in site order.
type Season struct {
	Number   int
	Episodes []int
}

// Ref names a title in the CDN calls: its post id, and its page path for
// the Referer.
type Ref struct {
	PostID int
	Path   string
}

// Ref returns the title's Ref.
func (t *Title) Ref() Ref { return Ref{PostID: t.PostID, Path: t.Path} }

// DefaultTranslator returns the translation the page plays first.
func (t *Title) DefaultTranslator() (Translator, bool) {
	for _, tr := range t.Translators {
		if tr.Default {
			return tr, true
		}
	}
	return Translator{}, false
}

// Translator returns the translation with the given id.
func (t *Title) Translator(id int) (Translator, bool) {
	for _, tr := range t.Translators {
		if tr.ID == id {
			return tr, true
		}
	}
	return Translator{}, false
}

// titlePathRe matches a title page: /<category>/<genre>/<post id>-<slug>.html.
var titlePathRe = regexp.MustCompile(`^/(films|series|cartoons|animation)/[a-z0-9_-]+/(\d+)-[a-z0-9_-]+\.html$`)

// TitlePath returns the path of a title page from its URL on any mirror, or
// from a bare path: "/films/fiction/2259-interstellar-2014.html". The client
// only fetches paths under its own base URL, so a URL can't send it
// elsewhere.
func TitlePath(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !titlePathRe.MatchString(u.Path) {
		return "", ErrInvalidPath
	}
	return u.Path, nil
}

// splitPath returns a valid title path's category and post id.
func splitPath(path string) (category string, postID int) {
	m := titlePathRe.FindStringSubmatch(path)
	if m == nil {
		return "", 0
	}
	postID, _ = strconv.Atoi(m[2])
	return m[1], postID
}

// Title fetches and parses a title page, given its URL on any mirror or its
// path.
func (c *Client) Title(ctx context.Context, pathOrURL string) (*Title, error) {
	path, err := TitlePath(pathOrURL)
	if err != nil {
		return nil, err
	}
	body, err := c.fetch(ctx, siteRequest{path: path})
	if err != nil {
		return nil, err
	}
	return parseTitle(path, body)
}

// initRe matches the player's init call, e.g.
//
//	sof.tv.initCDNMoviesEvents(2259, 56, 0, 0, 0, 'rezka.ag', false, false, {...});
//	sof.tv.initCDNSeriesEvents(646, 565, 1, 1, false, 'rezka.ag', false, true, {...});
//
// capturing the kind, the post id and the default translator id.
var initRe = regexp.MustCompile(`initCDN(Series|Movies)Events\((\d+),\s*(\d+),.+?(\{.*?\})\);`)

// slugYearRe reads the year at the end of a title's slug.
var slugYearRe = regexp.MustCompile(`-((?:19|20)\d\d)(?:-latest)?\.html$`)

func parseTitle(path string, body []byte) (*Title, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}

	m := initRe.FindSubmatch(body)
	if m == nil {
		// Titles blocked for the viewer's region show a message instead of
		// a player.
		if msg := textOf(doc, withClass("b-player__restricted__block_message")); msg != "" {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, msg)
		}
		return nil, &ParseError{What: "player init call"}
	}
	postID, _ := strconv.Atoi(string(m[2]))
	defaultID, _ := strconv.Atoi(string(m[3]))

	category, _ := splitPath(path)
	t := &Title{
		Path:          path,
		PostID:        postID,
		Kind:          KindMovie,
		Category:      category,
		Title:         textOf(doc, func(n *html.Node) bool { return n.Data == "h1" && withAttr("itemprop", "name")(n) }),
		OriginalTitle: textOf(doc, withClass("b-post__origtitle")),
		Year:          pageYear(doc, path),
		Translators:   parseTranslators(doc, defaultID),
	}
	if t.Title == "" {
		return nil, &ParseError{What: "title"}
	}
	if string(m[1]) == "Series" {
		t.Kind = KindSeries
		t.Seasons = parseSeasons(doc)
	}
	return t, nil
}

// parseTranslators reads the translation list. Movies list li elements with
// the flags; series list links and repeat them hidden as
// b-translator__items, which the exact class match skips. A page without a
// list has one translation, the init call's.
func parseTranslators(doc *html.Node, defaultID int) []Translator {
	var out []Translator
	seen := map[int]bool{}
	for _, n := range findAll(doc, withClass("b-translator__item")) {
		raw, _ := attr(n, "data-translator_id")
		id, err := strconv.Atoi(raw)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, Translator{
			ID:       id,
			Name:     translatorName(n),
			Default:  id == defaultID,
			Premium:  hasClass(n, "b-prem_translator"),
			Camrip:   flag(n, "data-camrip"),
			Ads:      flag(n, "data-ads"),
			Director: flag(n, "data-director"),
		})
	}
	if len(out) == 0 {
		out = append(out, Translator{ID: defaultID, Name: infoRow(doc, "В переводе"), Default: true})
	}
	return out
}

// translatorName is the item's title (or text) plus the language of its
// flag, so "HDrezka Studio" and its Ukrainian version differ.
func translatorName(n *html.Node) string {
	name, _ := attr(n, "title")
	if name = strings.TrimSpace(name); name == "" {
		name = text(n)
	}
	if img := findFirst(n, withTag("img")); img != nil {
		if lang, _ := attr(img, "title"); lang != "" && !strings.Contains(name, lang) {
			name += " (" + strings.TrimSpace(lang) + ")"
		}
	}
	return name
}

func flag(n *html.Node, key string) bool {
	v, _ := attr(n, key)
	return v == "1"
}

// parseSeasons reads season tabs (b-simple_season__item, data-tab_id) and
// episodes (b-simple_episode__item, data-season_id and data-episode_id),
// from a series page or a get_episodes answer. Seasons keep the order the
// site lists them in.
func parseSeasons(doc *html.Node) []Season {
	var out []Season
	index := map[int]int{}
	season := func(number int) *Season {
		i, ok := index[number]
		if !ok {
			i = len(out)
			index[number] = i
			out = append(out, Season{Number: number})
		}
		return &out[i]
	}
	for _, n := range findAll(doc, withClass("b-simple_season__item")) {
		if v, _ := attr(n, "data-tab_id"); v != "" {
			if number, err := strconv.Atoi(v); err == nil {
				season(number)
			}
		}
	}
	for _, n := range findAll(doc, withClass("b-simple_episode__item")) {
		s, _ := attr(n, "data-season_id")
		e, _ := attr(n, "data-episode_id")
		number, err1 := strconv.Atoi(s)
		episode, err2 := strconv.Atoi(e)
		if err1 != nil || err2 != nil {
			continue
		}
		ss := season(number)
		ss.Episodes = append(ss.Episodes, episode)
	}
	return out
}

// pageYear reads the release year from the info table's /year/ link, or
// else from the slug.
func pageYear(doc *html.Node, path string) int {
	if info := findFirst(doc, withClass("b-post__info")); info != nil {
		for _, a := range findAll(info, withTag("a")) {
			if href, _ := attr(a, "href"); strings.Contains(href, "/year/") {
				if y := firstYear(href); y != 0 {
					return y
				}
			}
		}
	}
	if m := slugYearRe.FindStringSubmatch(path); m != nil {
		y, _ := strconv.Atoi(m[1])
		return y
	}
	return 0
}

// infoRow returns the value of the info table's row whose heading starts
// with label ("В переводе"), or "".
func infoRow(doc *html.Node, label string) string {
	info := findFirst(doc, withClass("b-post__info"))
	if info == nil {
		return ""
	}
	for _, tr := range findAll(info, withTag("tr")) {
		tds := findAll(tr, withTag("td"))
		if len(tds) >= 2 && strings.HasPrefix(text(tds[0]), label) {
			return text(tds[1])
		}
	}
	return ""
}
