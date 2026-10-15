package hdrezka

import (
	"reflect"
	"testing"
)

func TestParseQuickSearch(t *testing.T) {
	got, err := parseQuickSearch(fixture(t, "quick_search_interstellar.html"))
	if err != nil {
		t.Fatalf("parseQuickSearch: %v", err)
	}
	want := []SearchResult{
		{
			URL: "https://rezka.ag/films/fiction/2259-interstellar-2014.html", Path: "/films/fiction/2259-interstellar-2014.html",
			PostID: 2259, Title: "Интерстеллар", OriginalTitle: "Interstellar", Year: 2014, Category: "films", Kind: KindMovie, Rating: 8.68,
		},
		{
			URL: "https://rezka.ag/films/fiction/22194-mezhzvezdnye-voyny-2016.html", Path: "/films/fiction/22194-mezhzvezdnye-voyny-2016.html",
			PostID: 22194, Title: "Межзвездные войны", OriginalTitle: "Interstellar Wars", Year: 2016, Category: "films", Kind: KindMovie, Rating: 3.9,
		},
		{
			// A cartoon with a year range is a series.
			URL: "https://rezka.ag/cartoons/fiction/92343-kosmicheskaya-ella-2022.html", Path: "/cartoons/fiction/92343-kosmicheskaya-ella-2022.html",
			PostID: 92343, Title: "Космическая Элла", OriginalTitle: "Interstellar Ella", Year: 2022, Category: "cartoons", Kind: KindSeries,
		},
		{
			// Anime with one year could be either.
			URL: "https://rezka.ag/animation/comedy/83397-mezhzvezdnoe-puteshestvie-2022.html", Path: "/animation/comedy/83397-mezhzvezdnoe-puteshestvie-2022.html",
			PostID: 83397, Title: "Межзвёздное путешествие", OriginalTitle: "Xing ji yi you / Interstellar Wanderer", Year: 2022, Category: "animation",
		},
		{
			URL: "https://rezka.ag/films/documentary/87713-nauka-interstellar-2014.html", Path: "/films/documentary/87713-nauka-interstellar-2014.html",
			PostID: 87713, Title: "Наука «Интерстеллар»", OriginalTitle: "The Science of Interstellar", Year: 2014, Category: "films", Kind: KindMovie, Rating: 7.04,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseQuickSearch =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseQuickSearch_RussianTitlesAndSeries(t *testing.T) {
	got, err := parseQuickSearch(fixture(t, "quick_search_moskva.html"))
	if err != nil {
		t.Fatalf("parseQuickSearch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	// "(1979)": no original title.
	if r := got[0]; r.Title != "Москва слезам не верит" || r.OriginalTitle != "" || r.Year != 1979 || r.Kind != KindMovie {
		t.Errorf("film = %+v", r)
	}
	// "(сериал, 2025 - ...)": the label isn't a title.
	if r := got[1]; r.OriginalTitle != "" || r.Year != 2025 || r.Kind != KindSeries {
		t.Errorf("series = %+v", r)
	}

	bb, err := parseQuickSearch(fixture(t, "quick_search_breaking_bad.html"))
	if err != nil {
		t.Fatalf("parseQuickSearch: %v", err)
	}
	if r := bb[0]; r.PostID != 646 || r.OriginalTitle != "Breaking Bad" || r.Year != 2008 || r.Kind != KindSeries {
		t.Errorf("Breaking Bad = %+v", r)
	}
}

func TestParseSearch(t *testing.T) {
	got, err := parseSearch(fixture(t, "search_interstellar.html"))
	if err != nil {
		t.Fatalf("parseSearch: %v", err)
	}
	if len(got) < 5 {
		t.Fatalf("got %d results, want at least 5", len(got))
	}
	want := SearchResult{
		URL: "https://rezka.ag/films/fiction/2259-interstellar-2014.html", Path: "/films/fiction/2259-interstellar-2014.html",
		PostID: 2259, Title: "Интерстеллар", Year: 2014, Category: "films", Kind: KindMovie,
	}
	if got[0] != want {
		t.Errorf("first result = %+v, want %+v", got[0], want)
	}
	for _, r := range got {
		if r.PostID == 92343 && (r.Kind != KindSeries || r.Year != 2022) {
			t.Errorf("cartoon series %+v: want kind series, year 2022", r)
		}
		if r.Title == "" || r.Path == "" || r.Year == 0 {
			t.Errorf("incomplete result %+v", r)
		}
	}
}

func TestParseSearch_YearFromTheInfoLine(t *testing.T) {
	// The year comes from the line under the link, not the title.
	page := `<div class="b-content__inline_items"><div class="b-content__inline_item" data-id="33333">
<div class="b-content__inline_item-link"><a href="https://rezka.ag/films/war/33333-1917-2019.html">1917</a> <div>2019, Великобритания, Военные</div></div>
</div></div>`
	got, err := parseSearch([]byte(page))
	if err != nil {
		t.Fatalf("parseSearch: %v", err)
	}
	if len(got) != 1 || got[0].Title != "1917" || got[0].Year != 2019 {
		t.Errorf("parseSearch = %+v, want 1917 from 2019", got)
	}
}

func TestParseTail(t *testing.T) {
	tests := []struct {
		in   string
		want tail
	}{
		{"(Interstellar, 2014)", tail{original: "Interstellar", year: 2014}},
		{"(1979)", tail{year: 1979}},
		{"(Breaking Bad, сериал, 2008-2013)", tail{original: "Breaking Bad", year: 2008, series: true}},
		{"(сериал, 2025 - ...)", tail{year: 2025, series: true}},
		{"(The Bad Guys: Breaking In, мультфильм, 2025 - ...)", tail{original: "The Bad Guys: Breaking In", year: 2025, series: true}},
		{"(Xing ji yi you / Interstellar Wanderer, аниме, 2022)", tail{original: "Xing ji yi you / Interstellar Wanderer", year: 2022}},
		{"(Crouching Tiger, Hidden Dragon, 2000)", tail{original: "Crouching Tiger, Hidden Dragon", year: 2000}},
		{"(мини-сериал, 2019)", tail{year: 2019, series: true}},
		{"", tail{}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseTail(tt.in); got != tt.want {
				t.Errorf("parseTail = %+v, want %+v", got, tt.want)
			}
		})
	}
}
