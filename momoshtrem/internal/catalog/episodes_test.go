package catalog

import (
	"reflect"
	"testing"
)

const today = "2026-10-14"

const (
	past   = "2020-01-01"
	future = "2099-01-01"
)

// season builds SeasonFacts from a compact spec, one rune per episode:
// 'f' file, 'd' file downloading, 'm' aired without file, 'u' upcoming,
// '?' no air date.
func season(n int, spec string, absoluteFrom int) SeasonFacts {
	s := SeasonFacts{Number: n}
	for i, r := range spec {
		ep := EpisodeFact{Number: i + 1}
		if absoluteFrom > 0 {
			ep.Absolute = intPtr(absoluteFrom + i)
		}
		switch r {
		case 'f':
			ep.HasFile, ep.AirDate = true, past
		case 'd':
			ep.HasFile, ep.Downloading, ep.AirDate = true, true, past
		case 'm':
			ep.AirDate = past
		case 'u':
			ep.AirDate = future
		case '?':
		}
		s.Episodes = append(s.Episodes, ep)
	}
	return s
}

func run(seasonNumber, from, to, count int, absFrom, absTo *int) *MissingRun {
	return &MissingRun{SeasonNumber: seasonNumber, From: from, To: to, Count: count, AbsoluteFrom: absFrom, AbsoluteTo: absTo}
}

func TestFirstMissingRun(t *testing.T) {
	frieren := season(1, "ffffffffffffffffffffffffffffmmmmmmmmmm", 1) // 28 files, 29–38 aired

	tests := []struct {
		name         string
		seasons      []SeasonFacts
		wantFrontier bool
		want         *MissingRun
	}{
		{"Frieren: 29–38 after the last file", []SeasonFacts{frieren}, true, run(1, 29, 38, 10, intPtr(29), intPtr(38))},
		{"nothing assigned: from the first episode", []SeasonFacts{season(1, "mmmu", 0)}, true, run(1, 1, 3, 3, nil, nil)},
		{"gap before the last file doesn't count", []SeasonFacts{season(1, "fmmfu", 0)}, false, nil},
		{"all files", []SeasonFacts{season(1, "fff", 0)}, false, nil},
		{"only upcoming after the last file", []SeasonFacts{season(1, "ffuu", 0)}, false, nil},
		{"no air date after the last file", []SeasonFacts{season(1, "ff?", 0)}, false, nil},
		{"an upcoming episode ends the run", []SeasonFacts{season(1, "fmmum", 0)}, true, run(1, 2, 3, 2, nil, nil)},
		{"the run stays in one season", []SeasonFacts{season(1, "fmm", 1), season(2, "mm", 4)}, true, run(1, 2, 3, 2, intPtr(2), intPtr(3))},
		{"frontier in a later season", []SeasonFacts{season(2, "mmu", 0), season(1, "fff", 0)}, true, run(2, 1, 2, 2, nil, nil)},
		{"specials never count", []SeasonFacts{season(0, "mmm", 0), season(1, "ff", 0)}, false, nil},
		{"a special file isn't the last file", []SeasonFacts{season(0, "f", 0), season(1, "mm", 0)}, true, run(1, 1, 2, 2, nil, nil)},
		{"downloading counts as a file", []SeasonFacts{season(1, "fdm", 0)}, true, run(1, 3, 3, 1, nil, nil)},
		{"no seasons", nil, false, nil},
		{"no aired episodes", []SeasonFacts{season(1, "uu", 0)}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontier, got := FirstMissingRun(tt.seasons, today)
			if frontier != tt.wantFrontier || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FirstMissingRun = %v %+v, want %v %+v", frontier, got, tt.wantFrontier, tt.want)
			}
		})
	}
}

func TestDefaultSeason(t *testing.T) {
	tests := []struct {
		name    string
		seasons []SeasonFacts
		missing *MissingRun
		want    *int
	}{
		{"the frontier's season", []SeasonFacts{season(1, "f", 0), season(2, "f", 0)}, run(1, 2, 2, 1, nil, nil), intPtr(1)},
		{"latest season with a file", []SeasonFacts{season(1, "f", 0), season(2, "f", 0), season(3, "uu", 0)}, nil, intPtr(2)},
		{"specials files don't count", []SeasonFacts{season(0, "f", 0), season(1, "u", 0), season(2, "u", 0)}, nil, intPtr(1)},
		{"first regular season", []SeasonFacts{season(0, "m", 0), season(2, "u", 0), season(1, "u", 0)}, nil, intPtr(1)},
		{"specials only", []SeasonFacts{season(0, "m", 0)}, nil, intPtr(0)},
		{"no seasons", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultSeason(tt.seasons, tt.missing); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefaultSeason = %v, want %v", deref2(got), deref2(tt.want))
			}
		})
	}
}

func deref2(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestSummarizeShow(t *testing.T) {
	specials := season(0, "fmu", 0)
	specials.Name = "Specials"
	s1 := season(1, "ffdfmmu?", 1)
	s1.Name = "Season 1"
	s2 := season(2, "uu", 9)

	got := SummarizeShow([]SeasonFacts{specials, s2, s1}, today)

	wantSeasons := []SeasonSummary{
		{SeasonNumber: 1, Name: "Season 1", EpisodesTotal: 8, Ready: 4, Missing: 2, Upcoming: 2, AbsoluteFrom: intPtr(1), AbsoluteTo: intPtr(8)},
		{SeasonNumber: 2, Name: "Season 2", EpisodesTotal: 2, Upcoming: 2, AbsoluteFrom: intPtr(9), AbsoluteTo: intPtr(10)},
		{SeasonNumber: 0, Name: "Specials", EpisodesTotal: 3, Ready: 1, Missing: 1, Upcoming: 1},
	}
	if !reflect.DeepEqual(got.Seasons, wantSeasons) {
		t.Errorf("seasons =\n%+v\nwant\n%+v", got.Seasons, wantSeasons)
	}
	// Regular episodes that aired or have a file: S1E1–E6.
	if got.EpisodesTotal != 6 || got.EpisodesReady != 4 {
		t.Errorf("episodes = %d/%d, want 4/6", got.EpisodesReady, got.EpisodesTotal)
	}
	if !got.AnyFile || !got.Downloading || !got.Frontier {
		t.Errorf("flags = file %v downloading %v frontier %v", got.AnyFile, got.Downloading, got.Frontier)
	}
	if want := run(1, 5, 6, 2, intPtr(5), intPtr(6)); !reflect.DeepEqual(got.Missing, want) {
		t.Errorf("missing = %+v, want %+v", got.Missing, want)
	}
	if got.DefaultSeason == nil || *got.DefaultSeason != 1 {
		t.Errorf("default season = %v, want 1", deref2(got.DefaultSeason))
	}

	empty := SummarizeShow(nil, today)
	if empty.Seasons == nil || len(empty.Seasons) != 0 || empty.DefaultSeason != nil || empty.AnyFile {
		t.Errorf("empty show = %+v", empty)
	}
}

func TestSeasonName(t *testing.T) {
	if got := SeasonName(0, ""); got != "Specials" {
		t.Errorf("SeasonName(0) = %q", got)
	}
	if got := SeasonName(3, ""); got != "Season 3" {
		t.Errorf("SeasonName(3) = %q", got)
	}
	if got := SeasonName(1, "Part 1"); got != "Part 1" {
		t.Errorf("SeasonName(1, Part 1) = %q", got)
	}
}
