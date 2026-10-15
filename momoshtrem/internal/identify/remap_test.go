package identify

import (
	"testing"
	"time"
)

// catalogOf builds a catalog from season runs: each run is (season, count,
// first air date, days between episodes).
type run struct {
	season, count int
	first         string
	every         int
}

func catalogOf(runs ...run) *ShowCatalog {
	cat := &ShowCatalog{Title: "Test Show"}
	next := map[int]int{}
	abs := 1
	for _, r := range runs {
		day, _ := time.Parse("2006-01-02", r.first)
		for i := 0; i < r.count; i++ {
			next[r.season]++
			ep := Episode{ID: fixtureID(r.season, next[r.season]), Season: r.season, Number: next[r.season]}
			if r.first != "" {
				ep.AirDate = day.AddDate(0, 0, i*r.every).Format("2006-01-02")
			}
			if r.season > 0 {
				ep.Absolute = abs
				abs++
			}
			cat.Episodes = append(cat.Episodes, ep)
		}
	}
	return cat
}

func TestCours(t *testing.T) {
	tests := []struct {
		name string
		cat  *ShowCatalog
		want []cour
	}{
		{"one run", catalogOf(run{1, 12, "2020-01-01", 7}),
			[]cour{{season: 1, first: 1, last: 12}}},
		{"a break of 300 days", catalogOf(run{1, 12, "2020-01-01", 7}, run{1, 12, "2021-01-01", 7}),
			[]cour{{season: 1, first: 1, last: 12}, {season: 1, first: 13, last: 24, gapDays: 289}}},
		{"a two-week break is one cour", catalogOf(run{1, 6, "2020-01-01", 7}, run{1, 6, "2020-02-19", 7}),
			[]cour{{season: 1, first: 1, last: 12}}},
		{"no air dates", catalogOf(run{1, 10, "", 0}),
			[]cour{{season: 1, first: 1, last: 10}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newIndex(tt.cat, time.Now()).coursOf(1)
			if len(got) != len(tt.want) {
				t.Fatalf("cours = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("cour %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}

	// TMDB's mid_season marker ends a cour even without a long break.
	cat := catalogOf(run{1, 10, "2020-01-01", 7})
	cat.Episodes[4].Type = "mid_season"
	if got := newIndex(cat, time.Now()).coursOf(1); len(got) != 2 || got[1].first != 6 {
		t.Errorf("mid_season: cours = %+v", got)
	}
}

func TestPlanRemap(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	split := catalogOf(run{1, 28, "2023-09-29", 7}, run{1, 10, "2026-01-16", 7})
	shortBreak := catalogOf(run{1, 12, "2023-01-01", 7}, run{1, 12, "2023-05-15", 7})
	twoSeasons := catalogOf(run{1, 24, "2020-10-03", 7}, run{2, 23, "2023-07-06", 7})
	grouped := catalogOf(run{1, 26, "2009-04-05", 7})
	grouped.AltOrderings = []AltOrdering{{Name: "Blu-ray", Groups: []AltGroup{
		{Number: 1, EpisodeIDs: idsOf(1, 1, 13)},
		{Number: 2, EpisodeIDs: idsOf(1, 14, 26)},
	}}}

	tests := []struct {
		name       string
		cat        *ShowCatalog
		season     int
		maxE       int
		e          int
		wantMethod Method
		wantStrong bool
		want       string // "" when the episode doesn't map
	}{
		{"cour after a long break", split, 2, 10, 1, MethodRemapCour, true, "S01E29"},
		{"cour, last episode", split, 2, 10, 10, MethodRemapCour, true, "S01E38"},
		{"cour, past its end", split, 2, 10, 11, MethodRemapCour, true, ""},
		{"pack bigger than the cour", split, 2, 12, 1, MethodRemapCour, false, "S01E29"},
		{"cour after a short break", shortBreak, 2, 12, 1, MethodRemapCour, false, "S01E13"},
		{"no third cour", split, 3, 5, 1, "", false, ""},
		{"episode group", grouped, 2, 13, 1, MethodRemapGroup, true, "S01E14"},
		{"episode group, last", grouped, 2, 13, 13, MethodRemapGroup, true, "S01E26"},
		{"overflow into the next season", twoSeasons, 1, 25, 25, MethodRemapOverflow, false, "S02E01"},
		{"overflow keeps episodes in range unmapped", twoSeasons, 1, 25, 24, MethodRemapOverflow, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newIndex(tt.cat, now).planRemap(tt.season, tt.maxE)
			if tt.wantMethod == "" {
				if p != nil {
					t.Fatalf("plan = %+v, want none", p)
				}
				return
			}
			if p == nil {
				t.Fatal("no plan")
			}
			if p.method != tt.wantMethod || p.strong != tt.wantStrong {
				t.Errorf("plan %s strong=%v, want %s strong=%v", p.method, p.strong, tt.wantMethod, tt.wantStrong)
			}
			got := ""
			if ep := p.lookup(tt.e); ep != nil {
				got = seLabel([]*Episode{ep})
			}
			if got != tt.want {
				t.Errorf("E%02d → %q, want %q", tt.e, got, tt.want)
			}
		})
	}
}

// An S/E that exists is never remapped, even when a remap would read it
// differently.
func TestExistingEpisodesAreNotRemapped(t *testing.T) {
	cat := catalogOf(run{1, 12, "2020-01-01", 7}, run{1, 12, "2021-06-01", 7}, run{2, 12, "2022-01-01", 7})
	res := Classify(Input{TorrentName: "x", Files: []TorrentFile{{Path: "Test.Show.S02E01.mkv", Size: 1 << 30}}, Show: cat, Now: corpusNow})
	if got := targetLabel(res.Proposals[0].Targets); got != "S02E01" {
		t.Errorf("S02E01 → %s", got)
	}
}

func idsOf(season, from, to int) []int64 {
	var ids []int64
	for e := from; e <= to; e++ {
		ids = append(ids, fixtureID(season, e))
	}
	return ids
}
