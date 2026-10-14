package metadata

import (
	"reflect"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// ep builds a library episode whose ID encodes its position: season*100+n.
func ep(season, n, tmdbID int) NumberedEpisode {
	return NumberedEpisode{ID: int64(season*100 + n), SeasonNumber: season, EpisodeNumber: n, TMDBEpisodeID: tmdbID}
}

func gep(season, n, tmdbID, order int) tmdb.GroupEpisode {
	return tmdb.GroupEpisode{ID: tmdbID, SeasonNumber: season, EpisodeNumber: n, Order: order}
}

func TestAbsoluteNumbers(t *testing.T) {
	tests := []struct {
		name     string
		episodes []NumberedEpisode
		group    *tmdb.EpisodeGroup
		want     map[int64]int
	}{
		{
			name:     "cumulative over regular seasons",
			episodes: []NumberedEpisode{ep(2, 2, 0), ep(1, 1, 0), ep(2, 1, 0), ep(1, 2, 0), ep(1, 3, 0)},
			want:     map[int64]int{101: 1, 102: 2, 103: 3, 201: 4, 202: 5},
		},
		{
			name:     "specials get no number and don't count",
			episodes: []NumberedEpisode{ep(0, 1, 0), ep(0, 2, 0), ep(1, 1, 0), ep(1, 2, 0), ep(2, 1, 0)},
			want:     map[int64]int{101: 1, 102: 2, 201: 3},
		},
		{
			name:     "only specials",
			episodes: []NumberedEpisode{ep(0, 1, 0)},
			want:     map[int64]int{},
		},
		{
			name:     "no episodes",
			episodes: nil,
			want:     map[int64]int{},
		},
		{
			name: "from a group, in group and episode order",
			// TMDB splits One Piece S1 into arcs; the group lists them out of
			// order and the library matches by TMDB episode ID.
			episodes: []NumberedEpisode{ep(1, 1, 11), ep(1, 2, 12), ep(1, 3, 13), ep(1, 4, 14)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 2, Episodes: []tmdb.GroupEpisode{gep(1, 4, 14, 1), gep(1, 3, 13, 0)}},
				{Order: 1, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(1, 2, 12, 1)}},
			}},
			want: map[int64]int{101: 1, 102: 2, 103: 3, 104: 4},
		},
		{
			name: "group reorders aired numbering",
			// The group puts S2E1 before S1E2 (e.g. a broadcast-order fix).
			episodes: []NumberedEpisode{ep(1, 1, 11), ep(1, 2, 12), ep(2, 1, 21)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 0, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(2, 1, 21, 1), gep(1, 2, 12, 2)}},
			}},
			want: map[int64]int{101: 1, 201: 2, 102: 3},
		},
		{
			name:     "falls back to season and episode when the TMDB ID is unknown",
			episodes: []NumberedEpisode{ep(1, 1, 0), ep(1, 2, 0)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 0, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(1, 2, 12, 1)}},
			}},
			want: map[int64]int{101: 1, 102: 2},
		},
		{
			name: "episodes missing from the library keep their place",
			// Library has episodes 1 and 3 of a 1086-start season: numbers
			// still follow the group.
			episodes: []NumberedEpisode{ep(1, 1, 11), ep(1, 3, 13)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 0, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(1, 2, 12, 1), gep(1, 3, 13, 2)}},
			}},
			want: map[int64]int{101: 1, 103: 3},
		},
		{
			name:     "episodes the group doesn't list get no number",
			episodes: []NumberedEpisode{ep(1, 1, 11), ep(1, 2, 12), ep(9, 1, 91)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 0, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(1, 2, 12, 1)}},
			}},
			want: map[int64]int{101: 1, 102: 2},
		},
		{
			name:     "specials in a group are skipped",
			episodes: []NumberedEpisode{ep(0, 1, 1), ep(1, 1, 11), ep(1, 2, 12)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 0, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(0, 1, 1, 1), gep(1, 2, 12, 2)}},
			}},
			want: map[int64]int{101: 1, 102: 2},
		},
		{
			name:     "an episode listed in two groups counts once",
			episodes: []NumberedEpisode{ep(1, 1, 11), ep(1, 2, 12), ep(1, 3, 13)},
			group: &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
				{Order: 0, Episodes: []tmdb.GroupEpisode{gep(1, 1, 11, 0), gep(1, 2, 12, 1)}},
				{Order: 1, Episodes: []tmdb.GroupEpisode{gep(1, 2, 12, 0), gep(1, 3, 13, 1)}},
			}},
			want: map[int64]int{101: 1, 102: 2, 103: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AbsoluteNumbers(tt.episodes, tt.group)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AbsoluteNumbers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPickAbsoluteGroup(t *testing.T) {
	tests := []struct {
		name   string
		groups []tmdb.EpisodeGroupSummary
		want   string
	}{
		{"none", nil, ""},
		{"no absolute group", []tmdb.EpisodeGroupSummary{{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 1100}}, ""},
		{"the one with the most episodes", []tmdb.EpisodeGroupSummary{
			{ID: "old", Type: tmdb.GroupAbsolute, EpisodeCount: 900},
			{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 2000},
			{ID: "new", Type: tmdb.GroupAbsolute, EpisodeCount: 1100},
		}, "new"},
		{"first on a tie", []tmdb.EpisodeGroupSummary{
			{ID: "a", Type: tmdb.GroupAbsolute, EpisodeCount: 10},
			{ID: "b", Type: tmdb.GroupAbsolute, EpisodeCount: 10},
		}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PickAbsoluteGroup(tt.groups); got != tt.want {
				t.Errorf("PickAbsoluteGroup = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNumberingGroup(t *testing.T) {
	groups := []tmdb.EpisodeGroupSummary{{ID: "abs", Type: tmdb.GroupAbsolute, EpisodeCount: 5}}
	tests := []struct {
		name     string
		ordering library.EpisodeOrdering
		chosen   string
		want     string
	}{
		{"aired ignores any group", library.OrderingAired, "chosen", ""},
		{"episode group uses the chosen one", library.OrderingEpisodeGroup, "chosen", "chosen"},
		{"episode group without a choice", library.OrderingEpisodeGroup, "", ""},
		{"absolute prefers the chosen one", library.OrderingAbsolute, "chosen", "chosen"},
		{"absolute falls back to the biggest absolute group", library.OrderingAbsolute, "", "abs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NumberingGroup(tt.ordering, tt.chosen, groups); got != tt.want {
				t.Errorf("NumberingGroup = %q, want %q", got, tt.want)
			}
		})
	}
	if got := NumberingGroup(library.OrderingAbsolute, "", nil); got != "" {
		t.Errorf("absolute without groups = %q, want cumulative", got)
	}
}
