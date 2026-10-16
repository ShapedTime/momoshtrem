package profile

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

var now = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }

func daysAgoPtr(n int) *time.Time {
	t := daysAgo(n)
	return &t
}

func movie(id int) library.TitleRef {
	return library.TitleRef{MediaType: library.TitleMovie, TMDBID: id}
}
func show(id int) library.TitleRef { return library.TitleRef{MediaType: library.TitleTV, TMDBID: id} }

// fixture is a history with one title for each rule.
func fixture() *History {
	return &History{
		Movies: []library.WatchedMovie{
			{TMDBID: 1, Title: "Dune: Part Two", Year: 2024, Plays: 3, LastWatchedAt: daysAgo(2)}, // rated 9
			{TMDBID: 2, Title: "Arrival", Year: 2016, Plays: 1, LastWatchedAt: daysAgo(400)},      // completed, unrated
			{TMDBID: 3, Title: "Cats", Year: 2019, Plays: 1, LastWatchedAt: daysAgo(40)},          // rated 3
			{TMDBID: 4, Title: "Old Watch", Year: 1994, Plays: 1, LastWatchedAt: daysAgo(31)},     // just outside recent
			{TMDBID: 5, Title: "Paddington 2", Year: 2017, Plays: 1, LastWatchedAt: daysAgo(29)},  // recent
		},
		Shows: []library.WatchedShow{
			{TMDBID: 10, Title: "Severance", Year: 2022, Plays: 19, EpisodesWatched: 19, Aired: 19, Completed: 19, HasProgress: true,
				LastWatchedAt: daysAgoPtr(5)}, // completed
			{TMDBID: 11, Title: "Frieren", Year: 2023, Plays: 26, EpisodesWatched: 26, Aired: 38, Completed: 26, HasProgress: true,
				LastWatchedAt: daysAgoPtr(1)}, // rated 10, not finished
			{TMDBID: 12, Title: "Game of Thrones", Year: 2011, Plays: 5, EpisodesWatched: 5, Aired: 73, Completed: 5, HasProgress: true,
				LastWatchedAt: daysAgoPtr(70)}, // dropped: 7%, idle 70 days
			{TMDBID: 13, Title: "Slow Horses", Year: 2022, Plays: 5, EpisodesWatched: 5, Aired: 30, Completed: 5, HasProgress: true,
				LastWatchedAt: daysAgoPtr(10)}, // under 30% but not idle
			{TMDBID: 14, Title: "The Wire", Year: 2002, Plays: 20, EpisodesWatched: 20, Aired: 60, Completed: 20, HasProgress: true,
				LastWatchedAt: daysAgoPtr(100)}, // idle but a third watched
			{TMDBID: 15, Title: "Lost", Year: 2004, Plays: 30, EpisodesWatched: 30, Aired: 118, Completed: 30, HasProgress: true,
				LastWatchedAt: daysAgoPtr(20)}, // dropped on Trakt
		},
		Ratings: []library.TasteRating{
			{TitleRef: movie(1), Rating: 9, RatedAt: daysAgo(2)},
			{TitleRef: movie(3), Rating: 3, RatedAt: daysAgo(40)},
			{TitleRef: show(11), Rating: 10, RatedAt: daysAgo(1)},
			{TitleRef: movie(6), Title: "Rated Unseen", Year: 2020, Rating: 9, RatedAt: daysAgo(50)}, // rated without a play
		},
		Hidden: []library.HiddenShow{{TMDBID: 15, Dropped: true}, {TMDBID: 16, FromProgress: true}},
		Feedback: []library.TasteFeedback{
			{TitleRef: show(20), Signal: "like", Title: "The Bear", Year: 2022, UpdatedAt: daysAgo(3)},
			{TitleRef: movie(21), Signal: "dislike", Title: "Transformers", Year: 2007, UpdatedAt: daysAgo(4)},
			{TitleRef: show(22), Signal: "not_interested", Title: "Love Island", Year: 2015, UpdatedAt: daysAgo(6)},
			{TitleRef: movie(23), Signal: "seen", Title: "Heat", Year: 1995, UpdatedAt: daysAgo(7)},
			{TitleRef: movie(24), Signal: "dislike", UpdatedAt: daysAgo(8)}, // nothing names it
		},
		Library: []library.LibraryTitle{
			{TitleRef: show(11), Title: "Frieren", Year: 2023, Genres: []string{"Animation", "Fantasy"}, OriginalLanguage: "ja",
				OriginCountry: []string{"JP"}, IsAnime: true, Runtime: 25},
		},
	}
}

func notes(list []Title) map[string]string {
	out := map[string]string{}
	for _, t := range list {
		out[t.Title] = t.Note
	}
	return out
}

func titleNames(list []Title) []string {
	var out []string
	for _, t := range list {
		out = append(out, t.Title)
	}
	return out
}

func TestRules_Lists(t *testing.T) {
	p := NewPlan(fixture(), true, now).Build(nil)

	wantLoved := map[string]string{
		"Dune: Part Two": "watched 3× · rated 9",
		"Arrival":        "watched",
		"Old Watch":      "watched",
		"Paddington 2":   "watched",
		"Severance":      "completed",
		"Frieren":        "26 of 38 · rated 10",
		"Rated Unseen":   "rated 9",
		"The Bear":       "liked",
		"Heat":           "marked seen", // seen: completed, neutral
	}
	if got := notes(p.Loved); !reflect.DeepEqual(got, wantLoved) {
		t.Errorf("loved = %v\nwant %v", got, wantLoved)
	}
	// Heaviest first: the rewatched 9 and the 10 lead, the year-old watch
	// trails
	if p.Loved[0].Title != "Dune: Part Two" || p.Loved[1].Title != "Frieren" || p.Loved[len(p.Loved)-1].Title != "Arrival" {
		t.Errorf("loved order = %v", titleNames(p.Loved))
	}

	wantDropped := []Title{
		{MediaType: "tv", TMDBID: 15, Title: "Lost", Year: 2004, Note: "30 of 118 · dropped on Trakt"},
		{MediaType: "tv", TMDBID: 12, Title: "Game of Thrones", Year: 2011, Note: "5 of 73 · idle 70 days"},
	}
	if !reflect.DeepEqual(p.Dropped, wantDropped) {
		t.Errorf("dropped = %+v\nwant %+v", p.Dropped, wantDropped)
	}

	if got, want := titleNames(p.Recent), []string{"Frieren", "Dune: Part Two", "Severance", "Slow Horses", "Lost", "Paddington 2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recent = %v, want %v", got, want)
	}
	if n := notes(p.Recent); n["Dune: Part Two"] != "watched 2026-10-14 · rated 9" || n["Paddington 2"] != "watched 2026-09-17" {
		t.Errorf("recent notes = %v", n)
	}

	// Newest first; a title nothing names is left off the list but counted
	wantAversions := []Title{
		{MediaType: "movie", TMDBID: 21, Title: "Transformers", Year: 2007, Note: "disliked"},
		{MediaType: "tv", TMDBID: 22, Title: "Love Island", Year: 2015, Note: "not interested"},
		{MediaType: "movie", TMDBID: 3, Title: "Cats", Year: 2019, Note: "rated 3"},
	}
	if !reflect.DeepEqual(p.Aversions, wantAversions) {
		t.Errorf("aversions = %+v\nwant %+v", p.Aversions, wantAversions)
	}

	// Completed: 5 movies watched, Severance, Heat (seen). Loved: those but
	// Cats (rated 3), plus Frieren, the rated and the liked. Plays: 7 + 105.
	want := Counts{Loved: 9, Completed: 7, Dropped: 2, Rated: 4, Plays: 112}
	if p.Counts != want {
		t.Errorf("counts = %+v, want %+v", p.Counts, want)
	}
	if !p.RatingsUsed || !p.Counts.Enough() {
		t.Errorf("ratings used %v, enough %v", p.RatingsUsed, p.Counts.Enough())
	}
}

// With use_ratings off, Trakt ratings make nothing loved or an aversion,
// weigh nothing and aren't counted; feedback still counts.
func TestRules_RatingsOff(t *testing.T) {
	p := NewPlan(fixture(), false, now).Build(nil)
	loved := notes(p.Loved)
	if _, ok := loved["Rated Unseen"]; ok {
		t.Error("a rating made a title loved with use_ratings off")
	}
	if loved["Cats"] != "watched" || loved["Dune: Part Two"] != "watched 3×" || loved["Frieren"] != "" {
		t.Errorf("loved = %v", loved)
	}
	if got := titleNames(p.Aversions); !reflect.DeepEqual(got, []string{"Transformers", "Love Island"}) {
		t.Errorf("aversions = %v, want feedback only", got)
	}
	if p.Counts.Rated != 0 || p.RatingsUsed {
		t.Errorf("rated = %d, ratings used = %v", p.Counts.Rated, p.RatingsUsed)
	}
	// Frieren and the rated-but-unseen title aren't loved; Cats now is
	if want := (Counts{Loved: 8, Completed: 7, Dropped: 2, Rated: 0, Plays: 112}); p.Counts != want {
		t.Errorf("counts = %+v, want %+v", p.Counts, want)
	}
}

func TestRules_Enough(t *testing.T) {
	h := &History{}
	for i := 1; i <= 4; i++ {
		h.Movies = append(h.Movies, library.WatchedMovie{TMDBID: i, Title: fmt.Sprint(i), Plays: 1, LastWatchedAt: daysAgo(i)})
	}
	// A completed title you rated badly isn't loved
	h.Movies = append(h.Movies, library.WatchedMovie{TMDBID: 5, Title: "5", Plays: 1, LastWatchedAt: daysAgo(5)})
	h.Ratings = []library.TasteRating{{TitleRef: movie(5), Rating: 2, RatedAt: daysAgo(5)}}
	if c := NewPlan(h, true, now).Counts(); c.Loved != 4 || c.Completed != 5 || c.Enough() {
		t.Errorf("4 loved = %+v, enough %v", c, c.Enough())
	}
	// A show you liked makes five
	h.Feedback = []library.TasteFeedback{{TitleRef: show(9), Signal: "like", UpdatedAt: daysAgo(1)}}
	c := NewPlan(h, true, now).Counts()
	if !c.Enough() {
		t.Errorf("5 loved = %+v, not enough", c)
	}
	for _, tt := range []struct {
		connected bool
		want      bool
	}{{true, true}, {false, false}} {
		if got := (Readiness{Connected: tt.connected, Counts: c}).Enough(); got != tt.want {
			t.Errorf("connected %v: enough = %v", tt.connected, got)
		}
	}
	if (Readiness{Connected: true}).Enough() {
		t.Error("no history is enough")
	}
}

// Recency halves a title's weight each year since you last touched it.
func TestRules_Recency(t *testing.T) {
	h := &History{Movies: []library.WatchedMovie{
		{TMDBID: 1, Title: "Old", Plays: 1, LastWatchedAt: daysAgo(3 * 365)},
		{TMDBID: 2, Title: "New", Plays: 1, LastWatchedAt: daysAgo(1)},
		{TMDBID: 3, Title: "Mid", Plays: 1, LastWatchedAt: daysAgo(365)},
	}}
	plan := NewPlan(h, true, now)
	w := map[int]float64{}
	for _, e := range plan.entries {
		w[e.ref.TMDBID] = e.weight
	}
	if !(w[2] > w[3] && w[3] > w[1]) {
		t.Errorf("weights = %v, want newest heaviest", w)
	}
	if mid := w[3]; mid < 0.62 || mid > 0.63 { // 0.25 + 0.75/2
		t.Errorf("a year-old watch weighs %.3f, want 0.625", mid)
	}
	if got := titleNames(plan.Build(nil).Loved); !reflect.DeepEqual(got, []string{"New", "Mid", "Old"}) {
		t.Errorf("loved = %v", got)
	}
}

func genre(id int, name string) tmdb.Genre { return tmdb.Genre{ID: id, Name: name} }

// The features come from TMDB's facts on the heaviest titles, or the
// library's when TMDB didn't describe one.
func TestRules_Features(t *testing.T) {
	h := &History{
		Movies: []library.WatchedMovie{
			{TMDBID: 1, Title: "Dune: Part Two", Year: 2024, Plays: 1, LastWatchedAt: daysAgo(1)},
			{TMDBID: 2, Title: "Arrival", Year: 2016, Plays: 1, LastWatchedAt: daysAgo(1)},
		},
		Shows: []library.WatchedShow{
			{TMDBID: 11, Title: "Frieren", Year: 2023, Aired: 28, Completed: 28, HasProgress: true, LastWatchedAt: daysAgoPtr(1)},
		},
		Library: []library.LibraryTitle{
			{TitleRef: show(11), Title: "Frieren", Year: 2023, Genres: []string{"Animation", "Science Fiction"}, OriginalLanguage: "ja",
				OriginCountry: []string{"JP"}, IsAnime: true, Runtime: 25},
		},
	}
	plan := NewPlan(h, true, now)
	if got := plan.WantDetails(); !reflect.DeepEqual(got, []library.TitleRef{movie(1), movie(2), show(11)}) {
		t.Errorf("want details = %v", got)
	}
	villeneuve := Person{137427, "Denis Villeneuve"}
	facts := map[library.TitleRef]Facts{
		movie(1): {Genres: []tmdb.Genre{genre(878, "Science Fiction"), genre(12, "Adventure")}, Keywords: []tmdb.Keyword{{ID: 4, Name: "desert"}},
			Creators: []Person{villeneuve}, Cast: []Person{{1190668, "Timothée Chalamet"}}, Language: "en", Countries: []string{"US"},
			AnimeKnown: true, Runtime: 166},
		movie(2): {Genres: []tmdb.Genre{genre(878, "Science Fiction"), genre(18, "Drama")}, Keywords: []tmdb.Keyword{{ID: 9, Name: "alien"}},
			Creators: []Person{villeneuve}, Language: "en", Countries: []string{"US"}, AnimeKnown: true, Runtime: 116},
		// Frieren: TMDB failed; the library describes it
	}
	p := plan.Build(facts)

	// Every title weighs 1 (watched yesterday, recency ~1): Science Fiction
	// is all three (the library's genre has no id, TMDB's does)
	if len(p.Genres) == 0 || p.Genres[0] != (Weighted{ID: 878, Name: "Science Fiction", W: 1}) {
		t.Errorf("genres = %+v", p.Genres)
	}
	if p.Creators[0] != (Weighted{ID: 137427, Name: "Denis Villeneuve", W: 0.67}) {
		t.Errorf("creators = %+v", p.Creators)
	}
	if len(p.Keywords) != 2 || p.Keywords[0].ID != 9 || p.Keywords[0].W != 0.33 {
		t.Errorf("keywords = %+v", p.Keywords)
	}
	if want := []Weighted{{Name: "en", W: 0.67}, {Name: "ja", W: 0.33}}; !reflect.DeepEqual(p.Languages, want) {
		t.Errorf("languages = %+v", p.Languages)
	}
	if want := (Mix{Movies: 2, Shows: 1, MovieShare: 0.67, AnimeShare: 0.33}); p.Mix != want {
		t.Errorf("mix = %+v, want %+v", p.Mix, want)
	}
	if want := (Runtime{MovieMedian: 116, MovieP90: 166, EpisodeMedian: 25}); p.Runtime != want {
		t.Errorf("runtime = %+v, want %+v", p.Runtime, want)
	}
	if want := []Weighted{{Name: "2020s", W: 0.67}, {Name: "2010s", W: 0.33}}; !reflect.DeepEqual(p.Decades, want) {
		t.Errorf("decades = %+v", p.Decades)
	}
}

// A title only feedback or a rating names gets its name from TMDB.
func TestRules_NamesFromTMDB(t *testing.T) {
	h := &History{Feedback: []library.TasteFeedback{{TitleRef: movie(24), Signal: "dislike", UpdatedAt: daysAgo(1)}}}
	plan := NewPlan(h, true, now)
	if got := plan.WantDetails(); !reflect.DeepEqual(got, []library.TitleRef{movie(24)}) {
		t.Fatalf("want details = %v", got)
	}
	p := plan.Build(map[library.TitleRef]Facts{movie(24): {Title: "Morbius", Year: 2022}})
	if want := []Title{{MediaType: "movie", TMDBID: 24, Title: "Morbius", Year: 2022, Note: "disliked"}}; !reflect.DeepEqual(p.Aversions, want) {
		t.Errorf("aversions = %+v", p.Aversions)
	}
}

// A huge history still fits the byte budget, trimming the longest lists.
func TestRules_ByteBudget(t *testing.T) {
	h := &History{}
	facts := map[library.TitleRef]Facts{}
	long := strings.Repeat("Ü", 70)
	for i := 1; i <= 2000; i++ {
		h.Movies = append(h.Movies, library.WatchedMovie{TMDBID: i, Title: fmt.Sprintf("%s %d", long, i), Year: 1950 + i%75, Plays: 1 + i%3,
			LastWatchedAt: daysAgo(i % 40)})
		h.Ratings = append(h.Ratings, library.TasteRating{TitleRef: movie(i), Rating: 1 + i%10, RatedAt: daysAgo(i % 40)})
		f := Facts{Language: fmt.Sprintf("l%d", i%9), Countries: []string{fmt.Sprintf("C%d", i%11)}, Runtime: 80 + i%90, AnimeKnown: true}
		for k := 0; k < 30; k++ {
			f.Keywords = append(f.Keywords, tmdb.Keyword{ID: i*100 + k, Name: fmt.Sprintf("%s keyword %d", long, i*100+k)})
			f.Cast = append(f.Cast, Person{i*100 + k, fmt.Sprintf("%s actor %d", long, k)})
			f.Creators = append(f.Creators, Person{i*1000 + k, fmt.Sprintf("%s director %d", long, k)})
		}
		facts[movie(i)] = f
	}
	p := NewPlan(h, true, now).Build(facts)
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > MaxBytes {
		t.Errorf("profile is %d bytes, want at most %d", len(b), MaxBytes)
	}
	if len(p.Loved) == 0 || len(p.Keywords) == 0 || len(p.Aversions) == 0 || len(p.Recent) == 0 {
		t.Errorf("a list was emptied: loved %d, keywords %d, aversions %d, recent %d", len(p.Loved), len(p.Keywords), len(p.Aversions), len(p.Recent))
	}
	for _, title := range p.Loved {
		if n := len([]rune(title.Title)); n > maxTitleLen {
			t.Fatalf("title of %d characters", n)
		}
	}
}

// An empty history is an empty profile: lists are [], never null.
func TestRules_Empty(t *testing.T) {
	p := NewPlan(&History{}, true, now).Build(nil)
	b, _ := json.Marshal(p)
	for _, key := range []string{"genres", "keywords", "creators", "cast", "decades", "languages", "countries", "loved", "dropped", "recent", "aversions"} {
		if !strings.Contains(string(b), `"`+key+`":[]`) {
			t.Errorf("%s isn't [] in %s", key, b)
		}
	}
	if p.Counts.Enough() {
		t.Error("an empty history is enough")
	}
}
