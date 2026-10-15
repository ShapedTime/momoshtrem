package hdrezka

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestInitRe(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantKind   string
		wantPost   string
		wantTransl string
	}{
		{"movie", `sof.tv.initCDNMoviesEvents(2259, 56, 0, 0, 0, 'rezka.ag', false, false, {"id":"cdnplayer","streams":"[360p]x","hlsconfig":{"maxBufferLength":180}});`, "Movies", "2259", "56"},
		{"series", `sof.tv.initCDNSeriesEvents(646, 565, 1, 1, false, 'rezka.ag', false, true, {"id":"cdnplayer"});`, "Series", "646", "565"},
		{"no space after the comma", `initCDNMoviesEvents(1311,110, 0, 0, 0, 'rezka.ag', false, false, {"id":"cdnplayer"});`, "Movies", "1311", "110"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := initRe.FindStringSubmatch(tt.in)
			if m == nil {
				t.Fatal("no match")
			}
			if m[1] != tt.wantKind || m[2] != tt.wantPost || m[3] != tt.wantTransl {
				t.Errorf("kind %q post %q translator %q, want %q %q %q", m[1], m[2], m[3], tt.wantKind, tt.wantPost, tt.wantTransl)
			}
			if !strings.HasPrefix(m[4], "{") || !strings.HasSuffix(m[4], "}") {
				t.Errorf("player config %q isn't an object", m[4])
			}
		})
	}
	if initRe.MatchString(`sof.tv.initCDNMoviesEvents = function(a, b) {}`) {
		t.Error("matched the function definition")
	}
}

func TestParseTitle_MovieWithTranslators(t *testing.T) {
	path := "/films/fiction/2259-interstellar-2014.html"
	title, err := parseTitle(path, fixture(t, "film_interstellar.html"))
	if err != nil {
		t.Fatalf("parseTitle: %v", err)
	}
	checkTitle(t, title, &Title{
		Path: path, PostID: 2259, Kind: KindMovie, Category: "films",
		Title: "Интерстеллар", OriginalTitle: "Interstellar", Year: 2014,
	})
	wantIDs := []int{56, 62, 267, 57, 63, 72, 358, 580, 469, 238}
	if got := translatorIDs(title); !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("translator ids = %v, want %v", got, wantIDs)
	}
	if tr, ok := title.DefaultTranslator(); !ok || tr.ID != 56 || tr.Name != "Дубляж" {
		t.Errorf("default translator = %+v, %v; want 56 Дубляж", tr, ok)
	}
	original, ok := title.Translator(238)
	want := Translator{ID: 238, Name: "Оригинал (+субтитры)"}
	if !ok || original != want {
		t.Errorf("translator 238 = %+v, want %+v", original, want)
	}
	// The flag's language is already in the name.
	if tr, _ := title.Translator(358); tr.Name != "Украинский дубляж" {
		t.Errorf("translator 358 name = %q", tr.Name)
	}
	if title.Seasons != nil {
		t.Errorf("a movie has seasons: %+v", title.Seasons)
	}
}

func TestParseTitle_MovieWithoutTranslators(t *testing.T) {
	path := "/films/drama/1311-moskva-slezam-ne-verit-1979.html"
	title, err := parseTitle(path, fixture(t, "film_moskva.html"))
	if err != nil {
		t.Fatalf("parseTitle: %v", err)
	}
	// The page dates it 1980 (the release); the slug says 1979.
	checkTitle(t, title, &Title{
		Path: path, PostID: 1311, Kind: KindMovie, Category: "films",
		Title: "Москва слезам не верит", Year: 1980,
	})
	want := []Translator{{ID: 110, Default: true}}
	if !reflect.DeepEqual(title.Translators, want) {
		t.Errorf("translators = %+v, want %+v (the init call's, unnamed)", title.Translators, want)
	}
}

func TestParseTitle_Series(t *testing.T) {
	path := "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html"
	title, err := parseTitle(path, fixture(t, "series_breaking_bad.html"))
	if err != nil {
		t.Fatalf("parseTitle: %v", err)
	}
	checkTitle(t, title, &Title{
		Path: path, PostID: 646, Kind: KindSeries, Category: "series",
		Title: "Во все тяжкие", OriginalTitle: "Breaking Bad", Year: 2008,
	})
	// The hidden b-translator__items copies don't count twice.
	wantIDs := []int{56, 12, 565, 6, 1, 80, 63, 359, 238}
	if got := translatorIDs(title); !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("translator ids = %v, want %v", got, wantIDs)
	}
	if tr, ok := title.DefaultTranslator(); !ok || tr.ID != 565 || tr.Name != "TVShows" || tr.Premium {
		t.Errorf("default translator = %+v, %v; want 565 TVShows", tr, ok)
	}
	wantCounts := map[int]int{1: 7, 2: 13, 3: 13, 4: 13, 5: 17}
	if len(title.Seasons) != len(wantCounts) {
		t.Fatalf("seasons = %+v, want 5", title.Seasons)
	}
	for i, s := range title.Seasons {
		if s.Number != i+1 || len(s.Episodes) != wantCounts[s.Number] {
			t.Errorf("season %d: number %d with %d episodes, want %d with %d", i, s.Number, len(s.Episodes), i+1, wantCounts[i+1])
		}
		if s.Episodes[0] != 1 {
			t.Errorf("season %d starts at episode %d", s.Number, s.Episodes[0])
		}
	}
}

func TestParseTitle_SeriesWithPremiumTranslators(t *testing.T) {
	path := "/series/fiction/91924-kvest-vizhna-2026-latest.html"
	title, err := parseTitle(path, fixture(t, "series_visionquest.html"))
	if err != nil {
		t.Fatalf("parseTitle: %v", err)
	}
	checkTitle(t, title, &Title{
		Path: path, PostID: 91924, Kind: KindSeries, Category: "series",
		Title: "Квест Вижна / Вижн в поиске себя", OriginalTitle: "VisionQuest", Year: 2026,
	})
	want := []Translator{
		{ID: 672, Name: "Дубляж", Default: true, Premium: true},
		{ID: 111, Name: "HDrezka Studio", Premium: true},
		{ID: 35, Name: "колдфильм (Coldfilm)"},
		{ID: 316, Name: "октопус (Octopus/Ultradox)"},
		{ID: 238, Name: "Оригинал (+субтитры)", Premium: true},
	}
	if !reflect.DeepEqual(title.Translators, want) {
		t.Errorf("translators =\n%+v\nwant\n%+v", title.Translators, want)
	}
	if want := []Season{{Number: 1, Episodes: []int{1, 2}}}; !reflect.DeepEqual(title.Seasons, want) {
		t.Errorf("seasons = %+v, want %+v", title.Seasons, want)
	}
}

func TestParseTitle_Errors(t *testing.T) {
	const path = "/films/drama/1-x-2000.html"
	restricted := `<html><body><h1 itemprop="name">X</h1><div class="b-player__restricted__block_message">Недоступно в вашем регионе</div></body></html>`
	_, err := parseTitle(path, []byte(restricted))
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "Недоступно") {
		t.Errorf("restricted page: error = %v, want ErrNotFound with the site's message", err)
	}

	var perr *ParseError
	_, err = parseTitle(path, []byte(`<html><body><h1 itemprop="name">X</h1></body></html>`))
	if !errors.As(err, &perr) || perr.What != "player init call" {
		t.Errorf("page without a player: error = %v, want a ParseError", err)
	}

	noTitle := `<html><body><script>sof.tv.initCDNMoviesEvents(1, 2, 0, 0, 0, 'rezka.ag', false, false, {"id":"cdnplayer"});</script></body></html>`
	_, err = parseTitle(path, []byte(noTitle))
	if !errors.As(err, &perr) || perr.What != "title" {
		t.Errorf("page without a title: error = %v, want a ParseError", err)
	}
}

func TestParseTranslators_FlagsAndUkrainianDuplicate(t *testing.T) {
	page := `<html><body><ul id="translators-list">
<li title="Дубляж" class="b-translator__item" data-translator_id="56" data-camrip="1" data-ads="1" data-director="0">Дубляж</li>
<li title="Режиссёрская" class="b-translator__item" data-translator_id="57" data-camrip="0" data-ads="0" data-director="1">Режиссёрская</li>
<li><a title="HDrezka Studio " class="b-translator__item" data-translator_id="376" href="/x">HDrezka Studio <img title="Украинский" src="ua.png"></a></li>
</ul></body></html>`
	doc, err := parseHTML([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	got := parseTranslators(doc, 57, nil)
	want := []Translator{
		{ID: 56, Name: "Дубляж", Camrip: true, Ads: true},
		{ID: 57, Name: "Режиссёрская", Default: true, Director: true},
		{ID: 376, Name: "HDrezka Studio (Украинский)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseTranslators =\n%+v\nwant\n%+v", got, want)
	}
}

// One translator id in two cuts is two entries; the init call's flags say
// which is the default.
func TestParseTranslators_Cuts(t *testing.T) {
	page := `<html><body><ul id="translators-list">
<li title="Дубляж" class="b-translator__item" data-translator_id="56" data-camrip="0" data-ads="0" data-director="0">Дубляж</li>
<li title="Дубляж" class="b-translator__item" data-translator_id="56" data-camrip="0" data-ads="0" data-director="1">Дубляж</li>
<li title="Дубляж" class="b-translator__item" data-translator_id="56" data-camrip="0" data-ads="0" data-director="1">Дубляж</li>
</ul><script>sof.tv.initCDNMoviesEvents(9, 56, 0, 0, 1, 'rezka.ag', false, false, {"id":"cdnplayer"});</script></body></html>`
	doc, err := parseHTML([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	got := parseTranslators(doc, 56, movieCut([]byte(page)))
	want := []Translator{
		{ID: 56, Name: "Дубляж"},
		{ID: 56, Name: "Дубляж", Default: true, Director: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseTranslators =\n%+v\nwant\n%+v", got, want)
	}

	// Without the flags, the id's first cut is the default.
	got = parseTranslators(doc, 56, nil)
	if !got[0].Default || got[1].Default {
		t.Errorf("default without flags = %+v", got)
	}
	if movieCut([]byte(`initCDNSeriesEvents(646, 565, 1, 1, false, 'rezka.ag', false, true, {});`)) != nil {
		t.Error("a series' init call read as a movie's cut")
	}
}

func TestTitlePath(t *testing.T) {
	tests := []struct {
		in      string
		want    string // "" means ErrInvalidPath
		wantCat string
		wantID  int
	}{
		{"/films/fiction/2259-interstellar-2014.html", "/films/fiction/2259-interstellar-2014.html", "films", 2259},
		{"https://rezka.ag/films/fiction/2259-interstellar-2014.html", "/films/fiction/2259-interstellar-2014.html", "films", 2259},
		{"https://hdrezka.me/series/thriller/646-vo-vse-tyazhkie-2008-latest.html?utm=1#player", "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html", "series", 646},
		{" https://rezka.ag/cartoons/fiction/92343-kosmicheskaya-ella-2022.html ", "/cartoons/fiction/92343-kosmicheskaya-ella-2022.html", "cartoons", 92343},
		{"/animation/comedy/83397-mezhzvezdnoe-puteshestvie-2022.html", "/animation/comedy/83397-mezhzvezdnoe-puteshestvie-2022.html", "animation", 83397},
		{"", "", "", 0},
		{"/", "", "", 0},
		{"/person/3929-kristofer-nolan/", "", "", 0},
		{"/films/fiction/", "", "", 0},
		{"/films/fiction/../../etc/passwd.html", "", "", 0},
		{"/series/thriller/646-vo-vse-tyazhkie-2008-latest/565-mnogogolosiy-tvshows.html", "", "", 0},
		{"/music/rock/1-x.html", "", "", 0},
		{"%zz", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := TitlePath(tt.in)
			if tt.want == "" {
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("TitlePath = %q, %v; want ErrInvalidPath", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("TitlePath = %q, %v; want %q", got, err, tt.want)
			}
			if cat, id := splitPath(got); cat != tt.wantCat || id != tt.wantID {
				t.Errorf("splitPath = %q, %d; want %q, %d", cat, id, tt.wantCat, tt.wantID)
			}
		})
	}
}

// checkTitle compares everything but the translators and seasons.
func checkTitle(t *testing.T, got, want *Title) {
	t.Helper()
	g, w := *got, *want
	g.Translators, g.Seasons, w.Translators, w.Seasons = nil, nil, nil, nil
	if !reflect.DeepEqual(g, w) {
		t.Errorf("title =\n%+v\nwant\n%+v", g, w)
	}
}

func translatorIDs(t *Title) []int {
	ids := make([]int, len(t.Translators))
	for i, tr := range t.Translators {
		ids[i] = tr.ID
	}
	return ids
}
