package service

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
)

func TestShowCatalog(t *testing.T) {
	abs := func(n int) *int { return &n }
	show := &library.Show{
		ID: 7, Title: "Frieren: Beyond Journey's End", OriginalName: "葬送のフリーレン", Year: 2023,
		IsAnime: true, EpisodeOrdering: library.OrderingAbsolute, IncludeSpecials: true,
		Seasons: []library.Season{
			{SeasonNumber: 0, Episodes: []library.Episode{{ID: 100, EpisodeNumber: 1, Name: "Recap", AirDate: "2024-03-15"}}},
			{SeasonNumber: 1, Episodes: []library.Episode{
				{ID: 101, EpisodeNumber: 1, Name: "The Journey's End", AirDate: "2023-09-29", AbsoluteNumber: abs(1), EpisodeType: "standard"},
				{ID: 129, EpisodeNumber: 29, Name: "", AirDate: "", AbsoluteNumber: abs(29), EpisodeType: "mid_season"},
			}},
		},
	}

	cat := ShowCatalog(show)
	if cat.Title != show.Title || cat.OriginalTitle != "葬送のフリーレン" || cat.Year != 2023 || !cat.IsAnime || cat.Ordering != identify.OrderingAbsolute {
		t.Errorf("show fields = %+v", cat)
	}
	want := []identify.Episode{
		{ID: 100, Season: 0, Number: 1, Name: "Recap", AirDate: "2024-03-15"},
		{ID: 101, Season: 1, Number: 1, Absolute: 1, Name: "The Journey's End", AirDate: "2023-09-29", Type: "standard"},
		{ID: 129, Season: 1, Number: 29, Absolute: 29, Type: "mid_season"},
	}
	if len(cat.Episodes) != len(want) {
		t.Fatalf("episodes = %+v", cat.Episodes)
	}
	for i := range want {
		if cat.Episodes[i] != want[i] {
			t.Errorf("episode %d = %+v, want %+v", i, cat.Episodes[i], want[i])
		}
	}

	show.IncludeSpecials = false
	for _, ep := range ShowCatalog(show).Episodes {
		if ep.Season == 0 {
			t.Error("season 0 kept with specials off")
		}
	}

	show.OriginalName = show.Title
	if ShowCatalog(show).OriginalTitle != "" {
		t.Error("an original title equal to the title is kept")
	}
}

// The adapter's catalog feeds the classifier: a scene S02 file of a show
// TMDB keeps in one season maps through the cour split.
func TestShowCatalogClassifies(t *testing.T) {
	show := &library.Show{Title: "Frieren: Beyond Journey's End", IsAnime: true, EpisodeOrdering: library.OrderingAbsolute, IncludeSpecials: true}
	season := library.Season{SeasonNumber: 1}
	for n := 1; n <= 38; n++ {
		air := "2023-09-29"
		if n > 28 {
			air = "2026-01-16"
		}
		abs := n
		season.Episodes = append(season.Episodes, library.Episode{ID: int64(1000 + n), EpisodeNumber: n, AirDate: air, AbsoluteNumber: &abs})
	}
	show.Seasons = []library.Season{season}

	res := identify.Classify(identify.Input{
		TorrentName: "Frieren.S02E01.1080p.WEB.mkv",
		Files:       []identify.TorrentFile{{Path: "Frieren.S02E01.1080p.WEB.mkv", Size: 1 << 30}},
		Show:        ShowCatalog(show),
	})
	p := res.Proposals[0]
	if len(p.Targets) != 1 || p.Targets[0].EpisodeID != 1029 {
		t.Errorf("S02E01 → %+v (%s)", p.Targets, p.Reason)
	}
}
