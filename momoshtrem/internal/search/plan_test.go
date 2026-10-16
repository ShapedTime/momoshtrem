package search

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/corpus"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

var (
	fixturesOnce sync.Once
	fixtures     map[string]*corpus.Show
	fixturesErr  error
)

// fixture is a show from the classifier's fixtures
// (internal/identify/testdata/shows), as a catalog. Each call returns a
// fresh copy.
func fixture(t *testing.T, name string) *identify.ShowCatalog {
	t.Helper()
	fixturesOnce.Do(func() { fixtures, fixturesErr = corpus.LoadShows(filepath.Join("..", "identify", "testdata")) })
	if fixturesErr != nil {
		t.Fatal(fixturesErr)
	}
	s, ok := fixtures[name]
	if !ok {
		t.Fatalf("no show fixture %q", name)
	}
	cat := &identify.ShowCatalog{
		Title: s.Title, OriginalTitle: s.OriginalTitle, Aliases: s.Aliases, Year: s.Year,
		IsAnime: s.IsAnime, Ordering: identify.Ordering(s.Ordering), Countries: s.Countries,
	}
	for _, e := range s.Episodes {
		cat.Episodes = append(cat.Episodes, identify.Episode(e))
	}
	for _, a := range s.AltOrderings {
		ao := identify.AltOrdering{Name: a.Name}
		for _, g := range a.Groups {
			ao.Groups = append(ao.Groups, identify.AltGroup(g))
		}
		cat.AltOrderings = append(cat.AltOrderings, ao)
	}
	return cat
}

// seasons is a catalog of regular seasons with these episode counts,
// numbered absolutely across them.
func seasons(title string, year int, counts ...int) *identify.ShowCatalog {
	cat := &identify.ShowCatalog{Title: title, Year: year, Ordering: identify.OrderingAired}
	abs := 1
	for i, count := range counts {
		for e := 1; e <= count; e++ {
			cat.Episodes = append(cat.Episodes, identify.Episode{ID: corpus.FixtureID(i+1, e), Season: i + 1, Number: e, Absolute: abs})
			abs++
		}
	}
	return cat
}

// showContext is a show's search context, as TitleContexts builds it.
func showContext(cat *identify.ShowCatalog, tmdbID int, imdbID string, tvdbID int, romaji string) *service.TitleContext {
	return &service.TitleContext{
		MediaType: library.JobTV, TMDBID: tmdbID, Title: cat.Title, OriginalTitle: cat.OriginalTitle,
		Aliases: cat.Aliases, Romaji: romaji, Year: cat.Year, IMDbID: imdbID, TVDBID: tvdbID,
		IsAnime: cat.IsAnime, Catalog: cat, EpisodeIDs: service.EpisodeIDsTMDB,
	}
}

func intp(n int) *int { return &n }

// render writes queries compactly: "Frieren S02E01 (scene numbering)" for
// a text query, "id tvsearch s=1 ep=29 imdb=… tvdb=… tmdb=…" for the ID
// query.
func render(queries []Query) []string {
	out := make([]string, 0, len(queries))
	for _, q := range queries {
		p := q.Params
		if !q.IsID() {
			s := q.Text
			if q.Label != "" {
				s += " (" + q.Label + ")"
			}
			if p != (Params{Type: TypeSearch, Q: q.Text}) {
				s += fmt.Sprintf(" [params %+v]", p)
			}
			out = append(out, s)
			continue
		}
		parts := []string{"id", string(p.Type)}
		if p.Season != nil {
			parts = append(parts, fmt.Sprintf("s=%d", *p.Season))
		}
		if p.Ep != nil {
			parts = append(parts, fmt.Sprintf("ep=%d", *p.Ep))
		}
		if p.IMDbID != "" {
			parts = append(parts, "imdb="+p.IMDbID)
		}
		if p.TVDBID != 0 {
			parts = append(parts, fmt.Sprintf("tvdb=%d", p.TVDBID))
		}
		parts = append(parts, fmt.Sprintf("tmdb=%d", p.TMDBID))
		if q.Text != "" || q.Label != "" || p.Q != "" {
			parts = append(parts, fmt.Sprintf("[text %q label %q q %q]", q.Text, q.Label, p.Q))
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func TestPlan(t *testing.T) {
	frieren := showContext(fixture(t, "frieren"), 209867, "tt22248376", 424536, "Sousou no Frieren")
	fma := showContext(fixture(t, "fullmetal-alchemist-brotherhood"), 31911, "tt1355642", 85249, "Hagane no Renkinjutsushi: Fullmetal Alchemist")
	severance := showContext(seasons("Severance", 2022, 9, 10), 95396, "tt11280740", 371980, "")
	goodPlace := showContext(seasons("The Good Place", 2016, 13, 13, 13, 14), 66573, "tt4955642", 311711, "")
	svu := showContext(fixture(t, "law-and-order-svu"), 2734, "tt0203259", 75692, "")
	moneyHeist := seasons("Money Heist", 2017, 9)
	moneyHeist.OriginalTitle = "La casa de papel"
	absolute := seasons("Numbered Show", 2010, 10, 10)
	absolute.Ordering = identify.OrderingAbsolute

	// TMDB's season 22 starts after the fixture's 1,122 episodes, so #1124
	// is S22E02.
	onePieceCat := fixture(t, "one-piece")
	for e := 1; e <= 10; e++ {
		onePieceCat.Episodes = append(onePieceCat.Episodes, identify.Episode{ID: corpus.FixtureID(22, e), Season: 22, Number: e, Absolute: 1122 + e})
	}
	onePiece := showContext(onePieceCat, 37854, "tt0388629", 81797, "")

	perfectDays := &service.TitleContext{
		MediaType: library.JobMovie, TMDBID: 976893, Title: "Perfect Days", OriginalTitle: "PERFECT DAYS",
		Year: 2023, IMDbID: "tt27503384", Runtime: 124,
	}

	tests := []struct {
		name         string
		tc           *service.TitleContext
		req          Request
		want         []string
		wantFallback []string
		wantKey      string
	}{
		{
			name: "anime episode: absolute number, romaji, scene numbering",
			tc:   frieren, req: Request{Season: intp(1), Episode: intp(29)},
			want: []string{
				"Frieren 29",
				"id tvsearch s=1 ep=29 imdb=tt22248376 tvdb=424536 tmdb=209867",
				"Sousou no Frieren 29 (original title)",
				"Frieren S02E01 (scene numbering)",
			},
			wantFallback: []string{"Frieren (whole show)", "Sousou no Frieren (whole show)"},
			wantKey:      "tv:209867:episode:1:29:",
		},
		{
			name: "anime episode without scene numbering adds TMDB's SxxEyy",
			tc:   frieren, req: Request{Season: intp(1), Episode: intp(5)},
			want: []string{
				"Frieren 05",
				"id tvsearch s=1 ep=5 imdb=tt22248376 tvdb=424536 tmdb=209867",
				"Sousou no Frieren 05 (original title)",
				"Frieren S01E05",
			},
			wantFallback: []string{"Frieren (whole show)", "Sousou no Frieren (whole show)"},
			wantKey:      "tv:209867:episode:1:5:",
		},
		{
			name: "anime season holds a scene season",
			tc:   frieren, req: Request{Season: intp(1)},
			want: []string{
				"Frieren S01",
				"id tvsearch s=1 imdb=tt22248376 tvdb=424536 tmdb=209867",
				"Sousou no Frieren S01 (original title)",
				"Frieren S02 (scene numbering)",
			},
			wantFallback: []string{"Frieren (whole show)", "Sousou no Frieren (whole show)"},
			wantKey:      "tv:209867:season:1:-:",
		},
		{
			name: "anime special has no absolute number",
			tc:   frieren, req: Request{Season: intp(0), Episode: intp(3)},
			want: []string{
				"Frieren S00E03",
				"id tvsearch s=0 ep=3 imdb=tt22248376 tvdb=424536 tmdb=209867",
				"Sousou no Frieren S00E03 (original title)",
			},
			wantFallback: []string{"Frieren (whole show)", "Sousou no Frieren (whole show)"},
			wantKey:      "tv:209867:episode:0:3:",
		},
		{
			name: "anime episode-group season",
			tc:   fma, req: Request{Season: intp(1), Episode: intp(14)},
			want: []string{
				"Fullmetal Alchemist 14",
				"id tvsearch s=1 ep=14 imdb=tt1355642 tvdb=85249 tmdb=31911",
				"Hagane no Renkinjutsushi 14 (original title)",
				"Fullmetal Alchemist S02E01 (scene numbering)",
			},
			wantFallback: []string{"Fullmetal Alchemist (whole show)", "Hagane no Renkinjutsushi (whole show)"},
			wantKey:      "tv:31911:episode:1:14:",
		},
		{
			name: "long absolute number",
			tc:   onePiece, req: Request{Season: intp(22), Episode: intp(2)},
			want: []string{
				"One Piece 1124",
				"id tvsearch s=22 ep=2 imdb=tt0388629 tvdb=81797 tmdb=37854",
				"One Piece S22E02",
			},
			wantFallback: []string{"One Piece (whole show)"},
			wantKey:      "tv:37854:episode:22:2:",
		},
		{
			name: "episode",
			tc:   severance, req: Request{Season: intp(2), Episode: intp(8)},
			want: []string{
				"Severance S02E08",
				"id tvsearch s=2 ep=8 imdb=tt11280740 tvdb=371980 tmdb=95396",
			},
			wantFallback: []string{"Severance (whole show)"},
			wantKey:      "tv:95396:episode:2:8:",
		},
		{
			name: "season",
			tc:   goodPlace, req: Request{Season: intp(1)},
			want: []string{
				"The Good Place S01",
				"id tvsearch s=1 imdb=tt4955642 tvdb=311711 tmdb=66573",
			},
			wantFallback: []string{"The Good Place (whole show)"},
			wantKey:      "tv:66573:season:1:-:",
		},
		{
			name: "scope any is the title alone",
			tc:   severance, req: Request{},
			want: []string{
				"Severance",
				"id tvsearch imdb=tt11280740 tvdb=371980 tmdb=95396",
			},
			wantKey: "tv:95396:any:-:-:",
		},
		{
			name: "scope any keeps the season in the key",
			tc:   severance, req: Request{Season: intp(2), Scope: ScopeAny},
			want: []string{
				"Severance",
				"id tvsearch imdb=tt11280740 tvdb=371980 tmdb=95396",
			},
			wantKey: "tv:95396:any:2:-:",
		},
		{
			name: "& reads as and, the colon goes",
			tc:   svu, req: Request{Season: intp(1), Episode: intp(1)},
			want: []string{
				"Law and Order Special Victims Unit S01E01",
				"id tvsearch s=1 ep=1 imdb=tt0203259 tvdb=75692 tmdb=2734",
			},
			wantFallback: []string{"Law and Order Special Victims Unit (whole show)"},
			wantKey:      "tv:2734:episode:1:1:",
		},
		{
			name: "a Latin original title",
			tc:   showContext(moneyHeist, 71446, "tt6468322", 327417, ""), req: Request{Season: intp(1), Episode: intp(3)},
			want: []string{
				"Money Heist S01E03",
				"id tvsearch s=1 ep=3 imdb=tt6468322 tvdb=327417 tmdb=71446",
				"La casa de papel S01E03 (original title)",
			},
			wantFallback: []string{"Money Heist (whole show)", "La casa de papel (whole show)"},
			wantKey:      "tv:71446:episode:1:3:",
		},
		{
			name: "a show numbered absolutely",
			tc:   showContext(absolute, 1, "", 0, ""), req: Request{Season: intp(2), Episode: intp(3)},
			want: []string{
				"Numbered Show S02E03",
				"id tvsearch s=2 ep=3 tmdb=1",
				"Numbered Show 13 (absolute number)",
			},
			wantFallback: []string{"Numbered Show (whole show)"},
			wantKey:      "tv:1:episode:2:3:",
		},
		{
			name: "movie: Title Year; an original title differing only in case is the title",
			tc:   perfectDays, req: Request{},
			want: []string{
				"Perfect Days 2023",
				"id movie imdb=tt27503384 tmdb=976893",
			},
			wantKey: "movie:976893::-:-:",
		},
		{
			name: "movie: season, episode and scope don't apply",
			tc:   perfectDays, req: Request{Season: intp(1), Episode: intp(2), Scope: "bogus"},
			want: []string{
				"Perfect Days 2023",
				"id movie imdb=tt27503384 tmdb=976893",
			},
			wantKey: "movie:976893::-:-:",
		},
		{
			name:    "a user query replaces the generated ones and the ID query",
			tc:      frieren,
			req:     Request{Season: intp(1), Episode: intp(29), Q: "  Sousou no Frieren  S2 - 01 "},
			want:    []string{"Sousou no Frieren S2 - 01"},
			wantKey: "tv:209867:episode:1:29:Sousou no Frieren S2 - 01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Plan(tt.tc, tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if got := render(p.Queries); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("queries:\n got  %q\n want %q", got, tt.want)
			}
			if got := render(p.Fallback); !reflect.DeepEqual(got, append([]string{}, tt.wantFallback...)) {
				t.Errorf("fallback:\n got  %q\n want %q", got, tt.wantFallback)
			}
			if p.Key != tt.wantKey {
				t.Errorf("key = %q, want %q", p.Key, tt.wantKey)
			}
			// Deterministic: the same request makes the same plan.
			if again, _ := Plan(tt.tc, tt.req); !reflect.DeepEqual(again, p) {
				t.Error("a second plan differs")
			}
		})
	}
}

func TestPlanScopeDefaults(t *testing.T) {
	tc := showContext(seasons("Severance", 2022, 9, 10), 95396, "", 0, "")
	tests := []struct {
		req  Request
		want Scope
	}{
		{Request{Season: intp(2), Episode: intp(8)}, ScopeEpisode},
		{Request{Season: intp(2)}, ScopeSeason},
		{Request{}, ScopeAny},
		{Request{Season: intp(2), Episode: intp(8), Scope: ScopeSeason}, ScopeSeason},
	}
	for _, tt := range tests {
		p, err := Plan(tc, tt.req)
		if err != nil || p.Scope != tt.want {
			t.Errorf("Plan(%+v) scope = %q, %v; want %q", tt.req, p.Scope, err, tt.want)
		}
	}
	p, _ := Plan(tc, Request{Season: intp(2), Episode: intp(8)})
	if got := render(p.TextQueries()); !reflect.DeepEqual(got, []string{"Severance S02E08"}) {
		t.Errorf("text queries = %q", got)
	}
}

func TestPlanErrors(t *testing.T) {
	tc := showContext(seasons("Severance", 2022, 9, 10), 95396, "", 0, "")
	tests := []struct {
		name string
		tc   *service.TitleContext
		req  Request
	}{
		{"scope episode without an episode", tc, Request{Season: intp(1), Scope: ScopeEpisode}},
		{"scope season without a season", tc, Request{Scope: ScopeSeason}},
		{"an episode without a season", tc, Request{Episode: intp(3)}},
		{"an unknown scope", tc, Request{Scope: "show"}},
		{"a negative season", tc, Request{Season: intp(-1)}},
		{"episode 0", tc, Request{Season: intp(1), Episode: intp(0)}},
		{"no title", nil, Request{}},
		{"an unknown media type", &service.TitleContext{MediaType: "person"}, Request{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Plan(tt.tc, tt.req); !errors.Is(err, ErrBadRequest) {
				t.Errorf("err = %v, want ErrBadRequest", err)
			}
		})
	}
}

func TestQueryTitle(t *testing.T) {
	tests := map[string]string{
		"Law & Order: Special Victims Unit":    "Law and Order Special Victims Unit",
		"Frieren: Beyond Journey's End":        "Frieren Beyond Journeys End",
		"Shōgun":                               "Shogun",
		"Marvel's Agents of S.H.I.E.L.D.":      "Marvels Agents of S H I E L D",
		"Love, Death & Robots":                 "Love Death and Robots",
		"What If...?":                          "What If",
		"Mr. Robot":                            "Mr Robot",
		"9-1-1: Lone Star":                     "9-1-1 Lone Star",
		"Spider-Man: Across the Spider-Verse":  "Spider-Man Across the Spider-Verse",
		"SPY×FAMILY":                           "SPY FAMILY",
		"  It’s Always Sunny in Philadelphia ": "Its Always Sunny in Philadelphia",
		"Star Wars - Andor":                    "Star Wars Andor",
	}
	for in, want := range tests {
		if got := queryTitle(in); got != want {
			t.Errorf("queryTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortTitle(t *testing.T) {
	tests := map[string]string{
		"Frieren: Beyond Journey's End":           "Frieren",
		"Demon Slayer: Kimetsu no Yaiba":          "Demon Slayer",
		"One Piece":                               "One Piece",
		"Re:ZERO -Starting Life in Another World": "Re:ZERO -Starting Life in Another World",
	}
	for in, want := range tests {
		if got := shortTitle(in); got != want {
			t.Errorf("shortTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
