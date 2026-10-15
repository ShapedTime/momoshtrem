package identify

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Ordering is how a show's episodes are numbered (the library's
// episode_ordering).
type Ordering string

const (
	OrderingAired        Ordering = "aired"
	OrderingAbsolute     Ordering = "absolute"
	OrderingEpisodeGroup Ordering = "episode_group"
)

// ShowCatalog is what the classifier knows about a show: its names and
// every episode TMDB lists, season 0 included when specials are on.
type ShowCatalog struct {
	Title         string
	OriginalTitle string
	Aliases       []string
	Year          int
	IsAnime       bool
	Ordering      Ordering
	Episodes      []Episode
	// AltOrderings are other ways to split the episodes into seasons (TMDB
	// episode groups), for releases whose seasons TMDB doesn't have.
	AltOrderings []AltOrdering
	// Countries are the show's origin countries (ISO 3166-1, "US"): the
	// country tag a release may add ("The.Office.US").
	Countries []string
}

// Episode is one episode of a ShowCatalog.
type Episode struct {
	ID       int64
	Season   int // 0 for specials
	Number   int
	Absolute int    // Absolute number; 0 when unknown (and for specials)
	Name     string //
	AirDate  string // YYYY-MM-DD; "" when unknown
	Type     string // TMDB episode_type: "standard", "mid_season", "finale"
}

// AltOrdering splits episodes into seasons another way: Groups[i] is the
// release season Groups[i].Number.
type AltOrdering struct {
	Name   string
	Groups []AltGroup
}

// AltGroup is a season of an AltOrdering: its episodes, in order.
type AltGroup struct {
	Number     int
	EpisodeIDs []int64
}

type seKey struct{ season, episode int }

// catalogIndex is a ShowCatalog indexed for matching. It's built per
// Classify call; the catalog itself is never modified.
type catalogIndex struct {
	cat         *ShowCatalog
	now         time.Time
	bySE        map[seKey]*Episode
	byAbs       map[int]*Episode
	byDate      map[string][]*Episode
	byID        map[int64]*Episode
	seasonMax   map[int]int // highest episode number per season
	absStart    map[int]int // first absolute number per regular season
	regular     []int       // regular season numbers, ascending
	hasSpecials bool
	named       []namedEpisode
	titles      [][]string      // known show titles as words, longest first
	titleWords  map[string]bool // every word of the known titles
	countryTags map[string]bool // country tags allowed after a title
}

type namedEpisode struct {
	ep    *Episode
	words []string
}

func newIndex(cat *ShowCatalog, now time.Time) *catalogIndex {
	if cat == nil {
		cat = &ShowCatalog{}
	}
	ix := &catalogIndex{
		cat:       cat,
		now:       now,
		bySE:      make(map[seKey]*Episode, len(cat.Episodes)),
		byAbs:     make(map[int]*Episode, len(cat.Episodes)),
		byDate:    make(map[string][]*Episode),
		byID:      make(map[int64]*Episode, len(cat.Episodes)),
		seasonMax: make(map[int]int),
		absStart:  make(map[int]int),
	}

	eps := make([]*Episode, 0, len(cat.Episodes))
	for i := range cat.Episodes {
		eps = append(eps, &cat.Episodes[i])
	}
	sort.SliceStable(eps, func(i, j int) bool {
		if eps[i].Season != eps[j].Season {
			return eps[i].Season < eps[j].Season
		}
		return eps[i].Number < eps[j].Number
	})

	// Absolute numbers come from the library (TMDB's absolute group or
	// cumulative counts). Shows synced before they existed have none:
	// count them here the cumulative way.
	haveAbs := false
	for _, ep := range eps {
		if ep.Absolute > 0 {
			haveAbs = true
			break
		}
	}
	next := 1
	for _, ep := range eps {
		ix.bySE[seKey{ep.Season, ep.Number}] = ep
		ix.byID[ep.ID] = ep
		if ep.Number > ix.seasonMax[ep.Season] {
			ix.seasonMax[ep.Season] = ep.Number
		}
		if ep.AirDate != "" {
			ix.byDate[ep.AirDate] = append(ix.byDate[ep.AirDate], ep)
		}
		if ep.Season == 0 {
			ix.hasSpecials = true
		} else {
			abs := ep.Absolute
			if !haveAbs {
				abs = next
				next++
			}
			if abs > 0 {
				if _, taken := ix.byAbs[abs]; !taken {
					ix.byAbs[abs] = ep
				}
				if start, ok := ix.absStart[ep.Season]; !ok || abs < start {
					ix.absStart[ep.Season] = abs
				}
			}
		}
		if words := nameWords(ep.Name); len(words) > 0 && !genericName(words) {
			ix.named = append(ix.named, namedEpisode{ep: ep, words: words})
		}
	}
	for s := range ix.seasonMax {
		if s > 0 {
			ix.regular = append(ix.regular, s)
		}
	}
	sort.Ints(ix.regular)

	seen := map[string]bool{}
	for _, name := range append([]string{cat.Title, cat.OriginalTitle}, cat.Aliases...) {
		words := normWords(name)
		for _, variant := range [][]string{words, trimThe(words)} {
			key := strings.Join(variant, " ")
			if len(variant) > 0 && !seen[key] {
				seen[key] = true
				ix.titles = append(ix.titles, variant)
			}
		}
	}
	sort.SliceStable(ix.titles, func(i, j int) bool { return len(ix.titles[i]) > len(ix.titles[j]) })
	ix.titleWords = map[string]bool{}
	ix.countryTags = map[string]bool{}
	for _, title := range ix.titles {
		for _, w := range title {
			ix.titleWords[w] = true
			if countryWords[w] {
				ix.countryTags[w] = true // "The Office (US)"
			}
		}
	}
	for _, c := range cat.Countries {
		for _, tag := range countryCodes[strings.ToUpper(c)] {
			ix.countryTags[tag] = true
		}
	}
	return ix
}

// episode returns the episode (season, number), or nil.
func (ix *catalogIndex) episode(season, number int) *Episode {
	return ix.bySE[seKey{season, number}]
}

func (ix *catalogIndex) seasonExists(season int) bool {
	_, ok := ix.seasonMax[season]
	return ok
}

func (ix *catalogIndex) lastRegular() int {
	if len(ix.regular) == 0 {
		return 0
	}
	return ix.regular[len(ix.regular)-1]
}

// absPreferred reports whether bare numbers are more likely absolute than
// per-season: anime, or a show the user numbers absolutely.
func (ix *catalogIndex) absPreferred() bool {
	return ix.cat.IsAnime || ix.cat.Ordering == OrderingAbsolute || ix.cat.Ordering == OrderingEpisodeGroup
}

// aired reports whether ep has aired by now. Unknown dates count as aired:
// specials often have none.
func (ix *catalogIndex) aired(ep *Episode) bool {
	if ep.AirDate == "" {
		return true
	}
	d, err := time.Parse("2006-01-02", ep.AirDate)
	if err != nil {
		return true
	}
	return !d.After(ix.now)
}

// cour is a run of a season's episodes aired without a long break.
type cour struct {
	season      int
	first, last int // episode numbers
	gapDays     int // days since the previous cour's last episode; 0 for the first
}

func (c cour) count() int { return c.last - c.first + 1 }

// coursOf splits a season into cours: a new cour starts after an air-date
// gap of more than 56 days, or after an episode TMDB marks mid_season.
func (ix *catalogIndex) coursOf(season int) []cour {
	var eps []*Episode
	for n := 1; n <= ix.seasonMax[season]; n++ {
		if ep := ix.episode(season, n); ep != nil {
			eps = append(eps, ep)
		}
	}
	if len(eps) == 0 {
		return nil
	}
	cours := []cour{{season: season, first: eps[0].Number, last: eps[0].Number}}
	for i := 1; i < len(eps); i++ {
		prev, ep := eps[i-1], eps[i]
		gap := daysBetween(prev.AirDate, ep.AirDate)
		if gap > 56 || prev.Type == "mid_season" {
			cours = append(cours, cour{season: season, first: ep.Number, last: ep.Number, gapDays: max(gap, 0)})
			continue
		}
		cours[len(cours)-1].last = ep.Number
	}
	return cours
}

// daysBetween returns the days from a to b, or -1 when either is unknown.
func daysBetween(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// normWords lowercases s and splits it into words of letters and digits.
// "&" reads as "and" and apostrophes vanish ("Journey's" → "journeys"), so
// "Law & Order: SVU" and "Law.and.Order.SVU" give the same words.
func normWords(s string) []string {
	s = foldDiacritics(strings.ToLower(s))
	s = strings.NewReplacer("&", " and ", "'", "", "’", "", "`", "").Replace(s)
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// latinFolds maps lowercase Latin letters with diacritics to plain ones,
// so "Shōgun" and "Élite" match "Shogun" and "Elite" in release names.
var latinFolds = func() map[rune]string {
	m := map[rune]string{'ß': "ss", 'æ': "ae", 'œ': "oe", 'ø': "o", 'đ': "d", 'ł': "l", 'ħ': "h", 'ı': "i", 'þ': "th"}
	for base, letters := range map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ď", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥ",
		"i": "ìíîïĩīĭį", "j": "ĵ", "k": "ķ", "l": "ĺļľŀ", "n": "ñńņň", "o": "òóôõöōŏő",
		"r": "ŕŗř", "s": "śŝşšș", "t": "ţťț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
	} {
		for _, r := range letters {
			m[r] = base
		}
	}
	return m
}()

// foldDiacritics replaces the diacritics latinFolds knows in a lowercase
// string. ASCII strings come back as they are.
func foldDiacritics(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if f, ok := latinFolds[r]; ok {
			b.WriteString(f)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// nameWords are an episode name's words for comparing names: normWords
// without "part"/"pt", so "No Regrets: Part 1" and "No.Regrets.Part.1"
// and "No Regrets 1" agree.
func nameWords(s string) []string {
	words := normWords(s)
	out := words[:0]
	for _, w := range words {
		if w != "part" && w != "pt" {
			out = append(out, w)
		}
	}
	return out
}

var reGenericName = regexp.MustCompile(`^(episode|ep|chapter|folge|episodio|capitulo)\s*\d+$`)

// genericName reports names TMDB gives unnamed episodes ("Episode 5"),
// which say nothing about which episode a file is.
func genericName(words []string) bool {
	return reGenericName.MatchString(strings.Join(words, " "))
}

func trimThe(words []string) []string {
	if len(words) > 1 && words[0] == "the" {
		return words[1:]
	}
	return nil
}

// stopwords don't count when comparing a file's title with the show's.
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "and": true, "no": true, "to": true, "in": true,
	"on": true, "with": true, "x": true, "de": true, "la": true, "le": true, "wa": true,
}

func significant(w string) bool {
	return !stopwords[w] && (len(w) >= 3 || containsDigit(w))
}

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
