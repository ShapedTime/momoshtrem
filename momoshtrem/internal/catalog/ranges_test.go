package catalog

import (
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// weekly builds n episodes from number `from`, a week apart from start
// (shifted by extra days at breakAfter), with absolute numbers from abs (0:
// none) and the given states.
func weekly(n, from int, start string, abs int, breakAfter, extraDays int, state func(int) EpisodeState) []EpisodeSummary {
	t0, _ := time.Parse(time.DateOnly, start)
	eps := make([]EpisodeSummary, n)
	shift := 0
	for i := range eps {
		if breakAfter > 0 && i == breakAfter {
			shift = extraDays
		}
		date := t0.AddDate(0, 0, i*7+shift).Format(time.DateOnly)
		eps[i] = EpisodeSummary{EpisodeNumber: from + i, AirDate: &date, State: state(from + i)}
		if abs > 0 {
			eps[i].AbsoluteNumber = intPtr(abs + i)
		}
	}
	return eps
}

func allReady(int) EpisodeState { return EpisodeReady }

func block(from, to, missing int, absFrom, absTo *int) EpisodeRange {
	return EpisodeRange{Label: blockLabel(from, to), Kind: "block", From: from, To: to, Missing: missing, AbsoluteFrom: absFrom, AbsoluteTo: absTo}
}

func TestRanges(t *testing.T) {
	frieren := weekly(38, 1, "2023-09-29", 0, 0, 0, func(n int) EpisodeState {
		if n >= 29 {
			return EpisodeMissing
		}
		return EpisodeReady
	})
	// 13 episodes, a 60-day break, then 13 more.
	cours := weekly(26, 1, "2024-01-05", 0, 13, 60, allReady)
	// One Piece season 22 from absolute 1086: arcs cover 1–40, the rest
	// (41–45) is newer than the arc group.
	onePiece := weekly(45, 1, "2023-12-03", 1086, 0, 0, func(n int) EpisodeState {
		if n > 43 {
			return EpisodeMissing
		}
		return EpisodeReady
	})
	arcs := []Arc{{Name: "Egghead", From: 1, To: 30}, {Name: "Elbaph", From: 31, To: 40}}

	tests := []struct {
		name   string
		season int
		eps    []EpisodeSummary
		arcs   []Arc
		want   []EpisodeRange
	}{
		{"20 episodes get none", 1, weekly(20, 1, "2020-01-01", 0, 0, 0, allReady), nil, []EpisodeRange{}},
		{"weekly 38 in blocks of 10", 1, frieren, nil, []EpisodeRange{
			block(1, 10, 0, nil, nil), block(11, 20, 0, nil, nil), block(21, 30, 2, nil, nil), block(31, 38, 8, nil, nil),
		}},
		{"split at a break of more than 45 days", 1, cours, nil, []EpisodeRange{
			block(1, 10, 0, nil, nil), block(11, 13, 0, nil, nil), block(14, 23, 0, nil, nil), block(24, 26, 0, nil, nil),
		}},
		{"specials ignore breaks", 0, cours, nil, []EpisodeRange{
			block(1, 10, 0, nil, nil), block(11, 20, 0, nil, nil), block(21, 26, 0, nil, nil),
		}},
		{"arcs, then blocks for what they don't cover", 22, onePiece, arcs, []EpisodeRange{
			{Label: "Egghead", Kind: "arc", From: 1, To: 30, AbsoluteFrom: intPtr(1086), AbsoluteTo: intPtr(1115)},
			{Label: "Elbaph", Kind: "arc", From: 31, To: 40, AbsoluteFrom: intPtr(1116), AbsoluteTo: intPtr(1125)},
			block(41, 45, 2, intPtr(1126), intPtr(1130)),
		}},
		{"a single leftover episode", 1, weekly(21, 1, "2020-01-01", 0, 0, 0, allReady), nil, []EpisodeRange{
			block(1, 10, 0, nil, nil), block(11, 20, 0, nil, nil), block(21, 21, 0, nil, nil),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ranges(tt.season, tt.eps, tt.arcs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Ranges =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestRanges_UnknownDatesDontBreak(t *testing.T) {
	eps := weekly(25, 1, "2020-01-01", 0, 0, 0, allReady)
	eps[12].AirDate = nil
	got := Ranges(1, eps, nil)
	if len(got) != 3 || got[1].From != 11 || got[1].To != 20 {
		t.Errorf("ranges = %+v", got)
	}
}

func TestBlockLabel(t *testing.T) {
	if got := blockLabel(21, 28); got != "21–28" {
		t.Errorf("blockLabel(21, 28) = %q", got)
	}
	if got := blockLabel(31, 31); got != "31" {
		t.Errorf("blockLabel(31, 31) = %q", got)
	}
}

func TestArcsForSeason(t *testing.T) {
	ep := func(season, number int) tmdb.GroupEpisode {
		return tmdb.GroupEpisode{SeasonNumber: season, EpisodeNumber: number}
	}
	group := &tmdb.EpisodeGroup{Type: tmdb.GroupStoryArc, Groups: []tmdb.EpisodeGroupGroup{
		{Name: "Wano", Order: 1, Episodes: []tmdb.GroupEpisode{ep(21, 90), ep(21, 91), ep(22, 1), ep(22, 2)}},
		{Name: "Egghead", Order: 2, Episodes: []tmdb.GroupEpisode{ep(22, 5), ep(22, 3), ep(22, 4)}},
		{Name: "East Blue", Order: 0, Episodes: []tmdb.GroupEpisode{ep(1, 1)}},
	}}
	got := ArcsForSeason(group, 22)
	want := []Arc{{Name: "Wano", From: 1, To: 2}, {Name: "Egghead", From: 3, To: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ArcsForSeason = %+v, want %+v", got, want)
	}
	if got := ArcsForSeason(nil, 1); got != nil {
		t.Errorf("nil group = %+v", got)
	}
}
