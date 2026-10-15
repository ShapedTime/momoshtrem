package hdrezka

import (
	"reflect"
	"testing"
)

func TestRank(t *testing.T) {
	interstellar, err := parseQuickSearch(fixture(t, "quick_search_interstellar.html"))
	if err != nil {
		t.Fatal(err)
	}
	breakingBad, err := parseQuickSearch(fixture(t, "quick_search_breaking_bad.html"))
	if err != nil {
		t.Fatal(err)
	}
	moskva, err := parseQuickSearch(fixture(t, "quick_search_moskva.html"))
	if err != nil {
		t.Fatal(err)
	}

	type ranked struct {
		PostID int
		Grade  Grade
	}
	tests := []struct {
		name       string
		candidates []SearchResult
		want       Want
		top        []ranked // the first len(top) results
	}{
		{
			"movie by original title",
			interstellar,
			Want{Title: "Interstellar", OriginalTitle: "Interstellar", Year: 2014, Kind: KindMovie},
			// The documentary contains the title and agrees on the rest;
			// "Interstellar Wars" is two years off.
			[]ranked{{2259, GradeExact}, {87713, GradeLikely}, {22194, GradeWeak}},
		},
		{
			"series; the minisodes and the movie spin-off rank lower",
			breakingBad,
			Want{Title: "Breaking Bad", OriginalTitle: "Breaking Bad", Year: 2008, Kind: KindSeries},
			[]ranked{{646, GradeExact}, {14806, GradeLikely}, {32282, GradeWeak}},
		},
		{
			"Russian title, year one off",
			moskva,
			Want{Title: "Moscow Does Not Believe in Tears", OriginalTitle: "Москва слезам не верит", Year: 1980, Kind: KindMovie},
			[]ranked{{1311, GradeExact}, {81849, GradeWeak}},
		},
		{
			"same title, wrong kind",
			breakingBad,
			Want{Title: "Breaking Bad", Year: 2008, Kind: KindMovie},
			[]ranked{{646, GradeLikely}},
		},
		{
			"cartoons don't conflict with either kind",
			interstellar,
			Want{Title: "Interstellar Wanderer", Year: 2022, Kind: KindMovie},
			[]ranked{{83397, GradeExact}},
		},
		{
			"a title alone, as the probe searches",
			interstellar,
			Want{Title: "interstellar"},
			[]ranked{{2259, GradeExact}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Rank(tt.candidates, tt.want)
			if len(got) != len(tt.candidates) {
				t.Fatalf("Rank returned %d of %d candidates", len(got), len(tt.candidates))
			}
			var top []ranked
			for _, m := range got[:len(tt.top)] {
				top = append(top, ranked{m.PostID, m.Grade})
			}
			if !reflect.DeepEqual(top, tt.top) {
				t.Errorf("top = %+v, want %+v", top, tt.top)
			}
		})
	}
}

func TestRank_KeepsSiteOrderForTies(t *testing.T) {
	candidates := []SearchResult{{PostID: 1, Title: "Other"}, {PostID: 2, Title: "Another"}}
	got := Rank(candidates, Want{Title: "Something else"})
	if got[0].PostID != 1 || got[1].PostID != 2 || got[0].Grade != GradeWeak {
		t.Errorf("Rank = %+v, want the site's order, both weak", got)
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := map[string]string{
		"Interstellar":     "interstellar",
		"Law & Order: SVU": "law and order svu",
		"Amélie":           "amelie",
		"Ёлки":             "елки",
		"Наука «Интерстеллар»": "наука интерстеллар",
		"Journey's End":                   "journeys end",
		"  WALL·E ":                       "wall e",
		"El Camino: A Breaking Bad Movie": "el camino a breaking bad movie",
	}
	for in, want := range tests {
		if got := normalizeTitle(in); got != want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
