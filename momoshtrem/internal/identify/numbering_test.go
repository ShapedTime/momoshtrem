package identify

import (
	"fmt"
	"strings"
	"testing"
)

// numbersText writes release numbers as "S01E29", "#29" and
// "S02E01 (scene numbering, remap_cour)".
func numbersText(nums []ReleaseNumber) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		s := fmt.Sprintf("S%02dE%02d", n.Season, n.Episode)
		if n.Method == MethodAbsolute {
			s = fmt.Sprintf("#%d", n.Episode)
		}
		if n.Label != "" && n.Method != MethodAbsolute {
			s += fmt.Sprintf(" (%s, %s)", n.Label, n.Method)
		}
		parts[i] = s
	}
	return strings.Join(parts, " ")
}

func TestReleaseNumbers(t *testing.T) {
	tests := []struct {
		name            string
		show            string
		season, episode int
		want            string
	}{
		{"Frieren's 2nd cour is release S02", "frieren", 1, 29, "S01E29 #29 S02E01 (scene numbering, remap_cour)"},
		{"Frieren's last episode", "frieren", 1, 38, "S01E38 #38 S02E10 (scene numbering, remap_cour)"},
		{"Frieren's 1st cour has no scene numbering", "frieren", 1, 28, "S01E28 #28"},
		{"specials have only their own number", "frieren", 0, 3, "S00E03"},
		{"an episode group season", "fullmetal-alchemist-brotherhood", 1, 14, "S01E14 #14 S02E01 (scene numbering, remap_group)"},
		{"the group's 1st season is TMDB's numbering", "fullmetal-alchemist-brotherhood", 1, 13, "S01E13 #13"},
		{"the group's last season", "fullmetal-alchemist-brotherhood", 1, 64, "S01E64 #64 S05E12 (scene numbering, remap_group)"},
		{"long absolute numbers", "one-piece", 21, 79, "S21E79 #1071"},
		{"a season TMDB has is never a scene season", "spy-x-family", 2, 1, "S02E01 #26"},
		{"no such episode", "frieren", 1, 39, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := numbersText(ReleaseNumbers(show(t, tt.show), tt.season, tt.episode)); got != tt.want {
				t.Errorf("ReleaseNumbers(S%02dE%02d) = %q, want %q", tt.season, tt.episode, got, tt.want)
			}
		})
	}
}

func TestReleaseNumbersCumulativeFallback(t *testing.T) {
	// A show synced before absolute numbers existed has none; the index
	// counts them over the regular seasons, as the classifier does.
	cat := catalogOf(run{1, 10, "2020-01-01", 7}, run{2, 10, "2021-01-01", 7})
	for i := range cat.Episodes {
		cat.Episodes[i].Absolute = 0
	}
	if got := numbersText(ReleaseNumbers(cat, 2, 3)); got != "S02E03 #13" {
		t.Errorf("ReleaseNumbers(S02E03) = %q", got)
	}
}

func TestCover(t *testing.T) {
	split := catalogOf(run{1, 28, "2023-09-29", 7}, run{1, 10, "2026-01-16", 7}) // Frieren's shape, not anime
	twoSeasons := catalogOf(run{1, 24, "2020-10-03", 7}, run{2, 23, "2023-07-06", 7})
	shows := map[string]*ShowCatalog{"split": split, "two-seasons": twoSeasons}
	catalog := func(t *testing.T, name string) *ShowCatalog {
		if cat, ok := shows[name]; ok {
			return cat
		}
		return show(t, name)
	}

	tests := []struct {
		name string
		show string
		span Span
		want string
	}{
		{"scene season 2 is the 2nd cour", "frieren", Span{Seasons: []int{2}}, "S01E29-E38"},
		{"the same run by absolute numbers", "frieren", Span{Absolute: &Range{29, 38}}, "S01E29-E38"},
		{"scene episodes", "frieren", Span{Seasons: []int{2}, Episodes: &Range{1, 3}}, "S01E29-E31"},
		{"anime: season 1 stops where scene season 2 starts", "frieren", Span{Seasons: []int{1}}, "S01E01-E28"},
		{"both scene seasons", "frieren", Span{Seasons: []int{1, 2}}, "S01E01-E38"},
		{"an S/E TMDB has is never remapped", "frieren", Span{Seasons: []int{1}, Episodes: &Range{29, 29}}, "S01E29"},
		{"bare numbers read as absolute", "frieren", Span{Episodes: &Range{29, 30}}, "S01E29-E30"},
		{"specials", "frieren", Span{Seasons: []int{0}}, "S00E01-E03"},
		{"a pack past TMDB's end covers what TMDB has", "frieren", Span{Seasons: []int{2}, Episodes: &Range{9, 12}}, "S01E37-E38"},
		{"no third scene season", "frieren", Span{Seasons: []int{3}}, ""},
		{"episode group season", "fullmetal-alchemist-brotherhood", Span{Seasons: []int{2}}, "S01E14-E26"},
		{"episode group season 1", "fullmetal-alchemist-brotherhood", Span{Seasons: []int{1}}, "S01E01-E13"},
		{"complete episode group", "fullmetal-alchemist-brotherhood", Span{Seasons: []int{1, 2, 3, 4, 5}}, "S01E01-E64"},
		{"long absolute numbers", "one-piece", Span{Absolute: &Range{1071, 1071}}, "S21E79"},
		{"absolute run across seasons", "spy-x-family", Span{Absolute: &Range{24, 27}}, "S01E24+S02E02"},
		{"non-anime: a whole season is TMDB's", "split", Span{Seasons: []int{1}}, "S01E01-E38"},
		{"non-anime: a later cour still maps", "split", Span{Seasons: []int{2}}, "S01E29-E38"},
		{"episodes past a season's end count on", "two-seasons", Span{Seasons: []int{1}, Episodes: &Range{23, 26}}, "S01E23+S02E02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cover(catalog(t, tt.show), tt.span)
			if label := coverLabel(got); label != tt.want {
				t.Errorf("Cover = %s, want %s", label, tt.want)
			}
		})
	}
}

// coverLabel writes covered episodes like seLabel, after checking they're
// consecutive within each season.
func coverLabel(targets []Target) string {
	if len(targets) == 0 {
		return ""
	}
	for i := 1; i < len(targets); i++ {
		a, b := targets[i-1], targets[i]
		if a.Season == b.Season && b.Episode != a.Episode+1 {
			return fmt.Sprintf("gap after S%02dE%02d", a.Season, a.Episode)
		}
	}
	first, last := targets[0], targets[len(targets)-1]
	s := fmt.Sprintf("S%02dE%02d", first.Season, first.Episode)
	switch {
	case len(targets) == 1:
		return s
	case first.Season == last.Season:
		return s + fmt.Sprintf("-E%02d", last.Episode)
	}
	return s + fmt.Sprintf("+S%02dE%02d", last.Season, last.Episode)
}

// Every numbering ReleaseNumbers gives reads back as its episode.
func TestReleaseNumbersRoundTrip(t *testing.T) {
	for _, name := range []string{"frieren", "fullmetal-alchemist-brotherhood", "spy-x-family", "one-piece", "breaking-bad"} {
		cat := show(t, name)
		n := NewNumbering(cat)
		for _, ep := range cat.Episodes {
			for _, rn := range n.ReleaseNumbers(ep.Season, ep.Number) {
				span := Span{Seasons: []int{rn.Season}, Episodes: &Range{rn.Episode, rn.Episode}}
				if rn.Method == MethodAbsolute {
					span = Span{Absolute: &Range{rn.Episode, rn.Episode}}
				}
				got := n.Cover(span)
				if len(got) != 1 || got[0].EpisodeID != ep.ID {
					t.Errorf("%s S%02dE%02d: %s reads back as %v", name, ep.Season, ep.Number, numbersText([]ReleaseNumber{rn}), got)
				}
			}
		}
	}
}
