package profile

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// The rules (PLAN §7 R1), pure: History in, Profile out. Every time
// boundary is computed from now.

// Rule constants.
const (
	lovedRating    = 8 // rated this or more: loved
	aversionRating = 4 // rated this or less: an aversion
	droppedShare   = 0.3
	droppedIdle    = 60 * 24 * time.Hour
	recentWindow   = 30 * 24 * time.Hour
	halfLife       = 365 * 24 * time.Hour // recency weight halves each year…
	recencyFloor   = 0.25                 // …down to this
	// DetailTitles is how many of the heaviest titles TMDB describes for
	// the genres, keywords, people, languages, countries, anime share and
	// runtimes.
	DetailTitles = 40
)

// List caps, before the byte budget trims them further.
const (
	maxGenres    = 12
	maxKeywords  = 20
	maxPeople    = 10
	maxDecades   = 8
	maxLanguages = 6
	maxCountries = 6
	maxLoved     = 30
	maxDropped   = 15
	maxRecent    = 20
	maxAversions = 20
	maxTitleLen  = 80 // characters
	maxNameLen   = 60
)

// ratingWeight multiplies a title's weight by its rating (with
// use_ratings); 4 or less is an aversion and weighs nothing.
var ratingWeight = map[int]float64{5: 0.7, 6: 1.0, 7: 1.15, 8: 1.5, 9: 1.75, 10: 2.0}

const likeWeight = 1.5 // a like weighs like a rating of 8

// Feedback signals, as rec_feedback stores them.
const (
	signalLike          = "like"
	signalDislike       = "dislike"
	signalNotInterested = "not_interested"
	signalSeen          = "seen"
)

// History is everything the profile reads, as the store answers it.
type History struct {
	Movies   []library.WatchedMovie
	Shows    []library.WatchedShow
	Ratings  []library.TasteRating
	Hidden   []library.HiddenShow
	Feedback []library.TasteFeedback
	Library  []library.LibraryTitle
}

// Person is a creator, director or cast member.
type Person struct {
	ID   int
	Name string
}

// Facts is what TMDB (or, without it, the library) says about a title.
type Facts struct {
	Title     string
	Year      int
	Genres    []tmdb.Genre
	Keywords  []tmdb.Keyword
	Creators  []Person // directors, or a show's creators
	Cast      []Person // top billed
	Language  string
	Countries []string
	// Anime is known only when TMDB described the title or the library
	// has it as a show.
	Anime, AnimeKnown bool
	Runtime           int // a movie's, or a show's episode runtime; 0 unknown
}

const factsCast = 5

// MovieFacts reads a movie's details.
func MovieFacts(d *tmdb.MovieDetails) Facts {
	f := Facts{Title: d.Title, Year: d.Year(), Genres: d.Genres, Keywords: d.Keywords.Keywords, Language: d.OriginalLanguage,
		Countries: d.OriginCountry, Runtime: d.Runtime, AnimeKnown: true,
		Anime: metadata.IsAnime(d.Genres, d.OriginalLanguage, d.OriginCountry, d.Keywords.Keywords)}
	for _, m := range d.Credits.Crew {
		if m.Job == "Director" {
			f.Creators = append(f.Creators, Person{m.ID, m.Name})
		}
	}
	for _, c := range d.Credits.Cast {
		if len(f.Cast) == factsCast {
			break
		}
		f.Cast = append(f.Cast, Person{c.ID, c.Name})
	}
	return f
}

// ShowFacts reads a show's details.
func ShowFacts(d *tmdb.ShowDetails) Facts {
	f := Facts{Title: d.Name, Year: d.Year(), Genres: d.Genres, Keywords: d.Keywords.Keywords, Language: d.OriginalLanguage,
		Countries: d.OriginCountry, Runtime: d.EpisodeRuntime(), AnimeKnown: true,
		Anime: metadata.IsAnime(d.Genres, d.OriginalLanguage, d.OriginCountry, d.Keywords.Keywords)}
	for _, c := range d.CreatedBy {
		f.Creators = append(f.Creators, Person{c.ID, c.Name})
	}
	for _, c := range d.AggregateCredits.Cast {
		if len(f.Cast) == factsCast {
			break
		}
		f.Cast = append(f.Cast, Person{c.ID, c.Name})
	}
	if len(f.Cast) == 0 {
		for _, c := range d.Credits.Cast {
			if len(f.Cast) == factsCast {
				break
			}
			f.Cast = append(f.Cast, Person{c.ID, c.Name})
		}
	}
	return f
}

// libraryFacts is what the library knows, for a title TMDB didn't
// describe: genres by name, and a show's language, countries and anime.
func libraryFacts(l *library.LibraryTitle) Facts {
	f := Facts{Title: l.Title, Year: l.Year, Language: l.OriginalLanguage, Countries: l.OriginCountry, Runtime: l.Runtime}
	for _, g := range l.Genres {
		f.Genres = append(f.Genres, tmdb.Genre{Name: g})
	}
	if l.MediaType == library.TitleTV {
		f.Anime, f.AnimeKnown = l.IsAnime, true
	}
	return f
}

// entry is one title and everything known about it.
type entry struct {
	ref          library.TitleRef
	title        string
	year         int
	plays        int
	watchedMovie bool
	show         *library.WatchedShow
	rating       int // 0: none, or ratings aren't used
	ratedAt      time.Time
	feedback     string
	feedbackAt   time.Time
	droppedTrakt bool
	lib          *library.LibraryTitle
	lastWatched  time.Time // zero: never

	aversion, completed, loved, dropped bool
	weight                              float64 // toward the features; 0 for dropped titles and aversions
}

// Plan is a profile being built: the titles weighed and ranked, waiting
// for TMDB to describe the ones WantDetails names.
type Plan struct {
	now         time.Time
	ratingsUsed bool
	entries     []*entry // sorted by ref
	counts      Counts
}

// NewPlan weighs h's titles as of now. useRatings is Trakt's use_ratings.
func NewPlan(h *History, useRatings bool, now time.Time) *Plan {
	p := &Plan{now: now, ratingsUsed: useRatings}
	byRef := map[library.TitleRef]*entry{}
	get := func(ref library.TitleRef) *entry {
		e := byRef[ref]
		if e == nil {
			e = &entry{ref: ref}
			byRef[ref] = e
		}
		return e
	}
	name := func(e *entry, title string, year int) {
		if e.title == "" && title != "" {
			e.title, e.year = title, year
		}
		if e.year == 0 {
			e.year = year
		}
	}

	for i := range h.Library {
		l := &h.Library[i]
		e := get(l.TitleRef)
		e.lib = l
	}
	for _, m := range h.Movies {
		e := get(library.TitleRef{MediaType: library.TitleMovie, TMDBID: m.TMDBID})
		name(e, m.Title, m.Year)
		e.watchedMovie, e.plays, e.lastWatched = true, m.Plays, m.LastWatchedAt
		p.counts.Plays += m.Plays
	}
	for i := range h.Shows {
		s := &h.Shows[i]
		e := get(library.TitleRef{MediaType: library.TitleTV, TMDBID: s.TMDBID})
		name(e, s.Title, s.Year)
		e.show, e.plays = s, s.Plays
		if s.LastWatchedAt != nil {
			e.lastWatched = *s.LastWatchedAt
		}
		p.counts.Plays += s.Plays
	}
	if useRatings {
		for _, r := range h.Ratings {
			e := get(r.TitleRef)
			name(e, r.Title, r.Year)
			e.rating, e.ratedAt = r.Rating, r.RatedAt
			p.counts.Rated++
		}
	}
	for _, hs := range h.Hidden {
		if hs.Dropped {
			get(library.TitleRef{MediaType: library.TitleTV, TMDBID: hs.TMDBID}).droppedTrakt = true
		}
	}
	for _, f := range h.Feedback {
		e := get(f.TitleRef)
		name(e, f.Title, f.Year)
		e.feedback, e.feedbackAt = f.Signal, f.UpdatedAt
	}
	for _, e := range byRef {
		if e.lib != nil {
			name(e, e.lib.Title, e.lib.Year)
		}
		p.classify(e)
		p.entries = append(p.entries, e)
	}
	slices.SortFunc(p.entries, func(a, b *entry) int {
		return cmp.Or(cmp.Compare(a.ref.MediaType, b.ref.MediaType), cmp.Compare(a.ref.TMDBID, b.ref.TMDBID))
	})
	return p
}

// classify decides what e is and what it weighs.
func (p *Plan) classify(e *entry) {
	e.aversion = (e.rating > 0 && e.rating <= aversionRating) || e.feedback == signalDislike || e.feedback == signalNotInterested
	switch {
	case e.watchedMovie, e.feedback == signalSeen:
		e.completed = true
	case e.show != nil && e.show.HasProgress && e.show.Aired > 0 && e.show.Completed >= e.show.Aired:
		e.completed = true
	}
	liked := e.feedback == signalLike
	e.loved = !e.aversion && (e.rating >= lovedRating || liked || e.completed)
	if e.ref.MediaType == library.TitleTV && !e.completed {
		switch {
		case e.droppedTrakt:
			e.dropped = true
		case e.show != nil && e.show.HasProgress && e.show.Aired > 0 &&
			float64(e.show.Completed)/float64(e.show.Aired) < droppedShare && p.idle(e) >= droppedIdle:
			e.dropped = true
		}
	}
	if e.loved {
		p.counts.Loved++
	}
	if e.completed {
		p.counts.Completed++
	}
	if e.dropped {
		p.counts.Dropped++
	}
	if e.aversion || e.dropped {
		return
	}

	var base float64
	switch {
	case e.watchedMovie:
		base = 1 + min(0.5, 0.25*float64(e.plays-1)) // rewatches count a little
	case e.show != nil:
		done := 0.5 // plays without progress: unknown
		if e.show.HasProgress && e.show.Aired > 0 {
			done = min(1, float64(e.show.Completed)/float64(e.show.Aired))
		}
		base = 0.3 + 0.7*done
	case e.feedback == signalSeen:
		base = 1
	case e.rating > 0 || liked:
		base = 0.8 // rated or liked without a play
	default:
		return // in the library, nothing more
	}
	if e.feedback == signalSeen {
		base = max(base, 1)
	}
	mult := 1.0
	if e.rating > 0 {
		mult = ratingWeight[e.rating]
	}
	if liked {
		mult = max(mult, likeWeight)
	}
	e.weight = base * mult * p.recency(e)
}

// lastActivity is when you last watched, rated or gave feedback on e.
func (p *Plan) lastActivity(e *entry) time.Time {
	t := e.lastWatched
	if e.rating > 0 && e.ratedAt.After(t) {
		t = e.ratedAt
	}
	if e.feedbackAt.After(t) {
		t = e.feedbackAt
	}
	return t
}

// idle is how long since e was last watched (forever when never).
func (p *Plan) idle(e *entry) time.Duration {
	if e.lastWatched.IsZero() {
		return time.Duration(math.MaxInt64)
	}
	return p.now.Sub(e.lastWatched)
}

// recency halves a title's weight each year since its last activity,
// down to recencyFloor.
func (p *Plan) recency(e *entry) float64 {
	last := p.lastActivity(e)
	if last.IsZero() {
		return recencyFloor
	}
	age := max(0, p.now.Sub(last))
	return recencyFloor + (1-recencyFloor)*math.Pow(0.5, float64(age)/float64(halfLife))
}

// Counts are the plan's totals.
func (p *Plan) Counts() Counts { return p.counts }

// featured are the heaviest titles, the ones whose facts make the
// features.
func (p *Plan) featured() []*entry {
	var out []*entry
	for _, e := range p.entries {
		if e.weight > 0 {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b *entry) int { return cmp.Compare(b.weight, a.weight) })
	if len(out) > DetailTitles {
		out = out[:DetailTitles]
	}
	return out
}

// WantDetails names the titles TMDB should describe: the heaviest, and
// any on a list that nothing names yet.
func (p *Plan) WantDetails() []library.TitleRef {
	var refs []library.TitleRef
	seen := map[library.TitleRef]bool{}
	for _, e := range p.featured() {
		refs = append(refs, e.ref)
		seen[e.ref] = true
	}
	lists := p.lists()
	for _, list := range [][]*entry{lists.loved, lists.dropped, lists.recent, lists.aversions} {
		for _, e := range list {
			if e.title == "" && !seen[e.ref] {
				refs = append(refs, e.ref)
				seen[e.ref] = true
			}
		}
	}
	return refs
}

type planLists struct {
	loved, dropped, recent, aversions []*entry
}

// lists are the profile's title lists, capped, before the names are
// checked.
func (p *Plan) lists() planLists {
	var l planLists
	since := p.now.Add(-recentWindow)
	for _, e := range p.entries {
		if e.loved {
			l.loved = append(l.loved, e)
		}
		if e.dropped {
			l.dropped = append(l.dropped, e)
		}
		if !e.lastWatched.IsZero() && !e.lastWatched.Before(since) {
			l.recent = append(l.recent, e)
		}
		if e.aversion {
			l.aversions = append(l.aversions, e)
		}
	}
	slices.SortStableFunc(l.loved, func(a, b *entry) int { return cmp.Compare(b.weight, a.weight) })
	byLast := func(a, b *entry) int { return p.lastActivity(b).Compare(p.lastActivity(a)) }
	slices.SortStableFunc(l.dropped, byLast)
	slices.SortStableFunc(l.recent, func(a, b *entry) int { return b.lastWatched.Compare(a.lastWatched) })
	slices.SortStableFunc(l.aversions, byLast)
	return l
}

// Build assembles the profile from the plan and the facts TMDB gave for
// WantDetails' titles (missing ones fall back to the library's).
func (p *Plan) Build(facts map[library.TitleRef]Facts) *Profile {
	for _, e := range p.entries {
		if f, ok := facts[e.ref]; ok && e.title == "" {
			e.title, e.year = f.Title, f.Year
		}
	}
	prof := &Profile{BuiltAt: p.now, Counts: p.counts, RatingsUsed: p.ratingsUsed}

	var total, movieW float64
	for _, e := range p.entries {
		if e.weight <= 0 {
			continue
		}
		total += e.weight
		if e.ref.MediaType == library.TitleMovie {
			prof.Mix.Movies++
			movieW += e.weight
		} else {
			prof.Mix.Shows++
		}
	}
	if total > 0 {
		prof.Mix.MovieShare = round2(movieW / total)
	}
	prof.Decades = p.decades(total)

	genres, keywords, creators, cast := newTally(), newTally(), newTally(), newTally()
	languages, countries := newTally(), newTally()
	var described, animeW, animeKnownW float64
	var movieRuntimes, episodeRuntimes []int
	for _, e := range p.featured() {
		f, ok := facts[e.ref]
		if !ok {
			if e.lib == nil {
				continue
			}
			f = libraryFacts(e.lib)
		}
		w := e.weight
		described += w
		for _, g := range f.Genres {
			genres.add(g.ID, g.Name, w)
		}
		for _, k := range f.Keywords {
			keywords.add(k.ID, k.Name, w)
		}
		for _, c := range f.Creators {
			creators.add(c.ID, c.Name, w)
		}
		for _, c := range f.Cast {
			cast.add(c.ID, c.Name, w)
		}
		if f.Language != "" {
			languages.add(0, f.Language, w)
		}
		for _, c := range f.Countries {
			countries.add(0, c, w)
		}
		if f.AnimeKnown {
			animeKnownW += w
			if f.Anime {
				animeW += w
			}
		}
		if f.Runtime > 0 {
			if e.ref.MediaType == library.TitleMovie {
				movieRuntimes = append(movieRuntimes, f.Runtime)
			} else {
				episodeRuntimes = append(episodeRuntimes, f.Runtime)
			}
		}
	}
	if animeKnownW > 0 {
		prof.Mix.AnimeShare = round2(animeW / animeKnownW)
	}
	prof.Genres = genres.top(maxGenres, described)
	prof.Keywords = keywords.top(maxKeywords, described)
	prof.Creators = creators.top(maxPeople, described)
	prof.Cast = cast.top(maxPeople, described)
	prof.Languages = languages.top(maxLanguages, described)
	prof.Countries = countries.top(maxCountries, described)
	prof.Runtime = Runtime{MovieMedian: medianInt(movieRuntimes), MovieP90: percentile(movieRuntimes, 0.9),
		EpisodeMedian: medianInt(episodeRuntimes)}

	lists := p.lists()
	prof.Loved = p.titles(lists.loved, maxLoved, p.lovedNote)
	prof.Dropped = p.titles(lists.dropped, maxDropped, p.droppedNote)
	prof.Recent = p.titles(lists.recent, maxRecent, p.recentNote)
	prof.Aversions = p.titles(lists.aversions, maxAversions, p.aversionNote)
	fit(prof)
	return prof
}

// decades shares every weighed title by its decade.
func (p *Plan) decades(total float64) []Weighted {
	t := newTally()
	for _, e := range p.entries {
		if e.weight > 0 && e.year > 0 {
			t.add(0, strconv.Itoa(e.year/10*10)+"s", e.weight)
		}
	}
	return t.top(maxDecades, total)
}

// titles lists up to limit named titles of list with their notes.
func (p *Plan) titles(list []*entry, limit int, note func(*entry) string) []Title {
	out := []Title{}
	for _, e := range list {
		if len(out) == limit {
			break
		}
		if e.title == "" {
			continue // nothing names it: TMDB didn't either
		}
		out = append(out, Title{MediaType: e.ref.MediaType, TMDBID: e.ref.TMDBID, Title: clip(e.title, maxTitleLen), Year: e.year, Note: note(e)})
	}
	return out
}

func (p *Plan) lovedNote(e *entry) string {
	var parts []string
	switch {
	case e.watchedMovie && e.plays > 1:
		parts = append(parts, fmt.Sprintf("watched %d×", e.plays))
	case e.watchedMovie:
		parts = append(parts, "watched")
	case e.completed && e.show != nil:
		parts = append(parts, "completed")
	case e.show != nil && e.show.HasProgress:
		parts = append(parts, fmt.Sprintf("%d of %d", e.show.Completed, e.show.Aired))
	}
	if e.feedback == signalSeen && len(parts) == 0 {
		parts = append(parts, "marked seen")
	}
	if e.rating > 0 {
		parts = append(parts, fmt.Sprintf("rated %d", e.rating))
	}
	if e.feedback == signalLike {
		parts = append(parts, "liked")
	}
	return strings.Join(parts, " · ")
}

func (p *Plan) droppedNote(e *entry) string {
	var parts []string
	if e.show != nil && e.show.HasProgress {
		parts = append(parts, fmt.Sprintf("%d of %d", e.show.Completed, e.show.Aired))
	}
	if e.droppedTrakt {
		parts = append(parts, "dropped on Trakt")
	} else if !e.lastWatched.IsZero() {
		parts = append(parts, fmt.Sprintf("idle %d days", int(p.now.Sub(e.lastWatched)/(24*time.Hour))))
	}
	return strings.Join(parts, " · ")
}

func (p *Plan) recentNote(e *entry) string {
	note := "watched " + e.lastWatched.In(p.now.Location()).Format(time.DateOnly)
	if e.rating > 0 {
		note += fmt.Sprintf(" · rated %d", e.rating)
	}
	return note
}

func (p *Plan) aversionNote(e *entry) string {
	switch e.feedback {
	case signalDislike:
		return "disliked"
	case signalNotInterested:
		return "not interested"
	}
	return fmt.Sprintf("rated %d", e.rating)
}

// tally sums weights by name, case-insensitively (the library's genres
// have no id), keeping TMDB's id when a source has one.
type tally struct {
	w    map[string]float64
	name map[string]string
	id   map[string]int
}

func newTally() *tally {
	return &tally{w: map[string]float64{}, name: map[string]string{}, id: map[string]int{}}
}

func (t *tally) add(id int, name string, w float64) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	key := strings.ToLower(name)
	t.w[key] += w
	if _, ok := t.name[key]; !ok {
		t.name[key] = clip(name, maxNameLen)
	}
	if t.id[key] == 0 && id > 0 {
		t.id[key] = id
	}
}

// top is the limit heaviest, each as its share of total.
func (t *tally) top(limit int, total float64) []Weighted {
	out := []Weighted{}
	if total <= 0 {
		return out
	}
	keys := make([]string, 0, len(t.w))
	for k := range t.w {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if t.w[keys[i]] != t.w[keys[j]] {
			return t.w[keys[i]] > t.w[keys[j]]
		}
		return t.name[keys[i]] < t.name[keys[j]]
	})
	for _, k := range keys {
		if len(out) == limit {
			break
		}
		share := round2(t.w[k] / total)
		if share <= 0 {
			break
		}
		out = append(out, Weighted{ID: t.id[k], Name: t.name[k], W: share})
	}
	return out
}

// fit trims the longest lists until the profile is within MaxBytes.
func fit(p *Profile) {
	for size(p) > MaxBytes {
		lists := []struct {
			n    int
			trim func(int)
		}{
			{len(p.Keywords), func(n int) { p.Keywords = p.Keywords[:n] }},
			{len(p.Cast), func(n int) { p.Cast = p.Cast[:n] }},
			{len(p.Creators), func(n int) { p.Creators = p.Creators[:n] }},
			{len(p.Loved), func(n int) { p.Loved = p.Loved[:n] }},
			{len(p.Recent), func(n int) { p.Recent = p.Recent[:n] }},
			{len(p.Aversions), func(n int) { p.Aversions = p.Aversions[:n] }},
			{len(p.Dropped), func(n int) { p.Dropped = p.Dropped[:n] }},
			{len(p.Genres), func(n int) { p.Genres = p.Genres[:n] }},
		}
		longest := -1
		for i, l := range lists {
			if l.n > 0 && (longest < 0 || l.n > lists[longest].n) {
				longest = i
			}
		}
		if longest < 0 {
			return // nothing left to trim
		}
		n := lists[longest].n
		lists[longest].trim(n - max(1, n/4))
	}
}

func size(p *Profile) int {
	b, err := json.Marshal(p)
	if err != nil {
		return 0
	}
	return len(b)
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// clip cuts s to n characters.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func medianInt(v []int) int { return percentile(v, 0.5) }

// percentile is the nearest-rank percentile of v's positive values (0
// when there are none).
func percentile(v []int, q float64) int {
	var s []int
	for _, x := range v {
		if x > 0 {
			s = append(s, x)
		}
	}
	if len(s) == 0 {
		return 0
	}
	slices.Sort(s)
	i := int(math.Ceil(q*float64(len(s)))) - 1
	return s[max(0, min(i, len(s)-1))]
}
