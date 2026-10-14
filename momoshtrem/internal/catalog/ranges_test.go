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
	arcs := []Arc{arc("Egghead", 1, 30), arc("Elbaph", 31, 40)}

	// Doctor Who's specials in TMDB's "The Doctor Order": a sparse
	// "Specials" group whose span (1–40) wraps every Doctor's group. Its
	// min–max span used to overlap them and count E1, E19 and E40 twice.
	doctorWho := weekly(40, 1, "2005-03-26", 0, 0, 0, func(n int) EpisodeState {
		switch n {
		case 1, 19, 25, 40:
			return EpisodeMissing
		}
		return EpisodeReady
	})
	doctors := []Arc{
		{Name: "Specials", Episodes: []int{1, 18, 19, 20, 30, 31, 32, 33, 40}},
		arc("Tenth Doctor", 2, 17),
		arc("Eleventh Doctor", 21, 29),
		arc("Twelfth Doctor", 34, 39),
	}
	arcRange := func(label string, from, to, missing int) EpisodeRange {
		return EpisodeRange{Label: label, Kind: "arc", From: from, To: to, Missing: missing}
	}

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
		{"arcs covering exactly 80% win", 22, onePiece, []Arc{arc("Egghead", 1, 36)}, []EpisodeRange{
			{Label: "Egghead", Kind: "arc", From: 1, To: 36, AbsoluteFrom: intPtr(1086), AbsoluteTo: intPtr(1121)},
			block(37, 45, 2, intPtr(1122), intPtr(1130)),
		}},
		{"arcs covering less than 80% fall back to blocks", 22, onePiece, []Arc{arc("Egghead", 1, 35)}, []EpisodeRange{
			block(1, 10, 0, intPtr(1086), intPtr(1095)), block(11, 20, 0, intPtr(1096), intPtr(1105)),
			block(21, 30, 0, intPtr(1106), intPtr(1115)), block(31, 40, 0, intPtr(1116), intPtr(1125)),
			block(41, 45, 2, intPtr(1126), intPtr(1130)),
		}},
		{"arcs sharing an episode fall back to blocks", 22, onePiece, []Arc{arc("Egghead", 1, 30), arc("Elbaph", 30, 45)}, []EpisodeRange{
			block(1, 10, 0, intPtr(1086), intPtr(1095)), block(11, 20, 0, intPtr(1096), intPtr(1105)),
			block(21, 30, 0, intPtr(1106), intPtr(1115)), block(31, 40, 0, intPtr(1116), intPtr(1125)),
			block(41, 45, 2, intPtr(1126), intPtr(1130)),
		}},
		{"a hole between arcs gets a block", 22, onePiece, []Arc{arc("Egghead", 1, 20), arc("Elbaph", 25, 45)}, []EpisodeRange{
			{Label: "Egghead", Kind: "arc", From: 1, To: 20, AbsoluteFrom: intPtr(1086), AbsoluteTo: intPtr(1105)},
			block(21, 24, 0, intPtr(1106), intPtr(1109)),
			{Label: "Elbaph", Kind: "arc", From: 25, To: 45, Missing: 2, AbsoluteFrom: intPtr(1110), AbsoluteTo: intPtr(1130)},
		}},
		{"an interrupted arc gets one range per run", 1, doctorWho, doctors, []EpisodeRange{
			arcRange("Specials", 1, 1, 1),
			arcRange("Tenth Doctor", 2, 17, 0),
			arcRange("Specials (2)", 18, 20, 1),
			arcRange("Eleventh Doctor", 21, 29, 1),
			arcRange("Specials (3)", 30, 33, 0),
			arcRange("Twelfth Doctor", 34, 39, 0),
			arcRange("Specials (4)", 40, 40, 1),
		}},
		{"specials never use arcs", 0, doctorWho, doctors, []EpisodeRange{
			block(1, 10, 1, nil, nil), block(11, 20, 1, nil, nil), block(21, 30, 1, nil, nil), block(31, 40, 1, nil, nil),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ranges(tt.season, tt.eps, tt.arcs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Ranges =\n%+v\nwant\n%+v", got, tt.want)
			}
			assertPartition(t, tt.eps, got)
		})
	}
}

// arc lists episodes from–to.
func arc(name string, from, to int) Arc {
	a := Arc{Name: name}
	for n := from; n <= to; n++ {
		a.Episodes = append(a.Episodes, n)
	}
	return a
}

// assertPartition checks that ranges, when there are any, take every
// episode exactly once and in order, so no two overlap and each missing
// episode is counted once.
func assertPartition(t *testing.T, eps []EpisodeSummary, ranges []EpisodeRange) {
	t.Helper()
	if len(ranges) == 0 {
		return
	}
	i := 0
	for _, r := range ranges {
		if i == len(eps) || eps[i].EpisodeNumber != r.From {
			t.Errorf("range %q doesn't start at the next episode", r.Label)
			return
		}
		missing := 0
		for ; i < len(eps) && eps[i].EpisodeNumber <= r.To; i++ {
			if eps[i].State == EpisodeMissing {
				missing++
			}
		}
		if eps[i-1].EpisodeNumber != r.To || missing != r.Missing {
			t.Errorf("range %q = %d–%d missing %d; its episodes end at %d with %d missing",
				r.Label, r.From, r.To, r.Missing, eps[i-1].EpisodeNumber, missing)
		}
	}
	if i != len(eps) {
		t.Errorf("ranges take %d of %d episodes", i, len(eps))
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
	want := []Arc{{Name: "Wano", Episodes: []int{1, 2}}, {Name: "Egghead", Episodes: []int{3, 4, 5}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ArcsForSeason = %+v, want %+v", got, want)
	}
	if got := ArcsForSeason(nil, 1); got != nil {
		t.Errorf("nil group = %+v", got)
	}
}
