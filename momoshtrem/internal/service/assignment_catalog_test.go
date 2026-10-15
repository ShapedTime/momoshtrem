package service

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
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

	cat := ShowCatalog(show, CatalogExtras{})
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
	for _, ep := range ShowCatalog(show, CatalogExtras{}).Episodes {
		if ep.Season == 0 {
			t.Error("season 0 kept with specials off")
		}
	}

	show.OriginalName = show.Title
	if ShowCatalog(show, CatalogExtras{}).OriginalTitle != "" {
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
		Show:        ShowCatalog(show, CatalogExtras{}),
	})
	p := res.Proposals[0]
	if len(p.Targets) != 1 || p.Targets[0].EpisodeID != 1029 {
		t.Errorf("S02E01 → %+v (%s)", p.Targets, p.Reason)
	}
}

// Fansub names use TMDB's romaji alternative title, not its English or
// Japanese one: the alias is what lets the classifier trust them.
func TestShowCatalogAliasesReachHigh(t *testing.T) {
	show := &library.Show{Title: "The Apothecary Diaries", OriginalName: "薬屋のひとりごと", Year: 2023, IsAnime: true,
		EpisodeOrdering: library.OrderingAbsolute, IncludeSpecials: true, OriginCountry: []string{"JP"}}
	season := library.Season{SeasonNumber: 1}
	for n := 1; n <= 24; n++ {
		abs := n
		season.Episodes = append(season.Episodes, library.Episode{ID: int64(2000 + n), EpisodeNumber: n,
			AirDate: "2023-10-22", AbsoluteNumber: &abs})
	}
	show.Seasons = []library.Season{season}
	files := []identify.TorrentFile{{Path: "[SubsPlease] Kusuriya no Hitorigoto - 05 (1080p) [A1B2C3D4].mkv", Size: 1 << 30}}
	classify := func(extras CatalogExtras) identify.Proposal {
		return identify.Classify(identify.Input{TorrentName: files[0].Path, Files: files, Show: ShowCatalog(show, extras)}).Proposals[0]
	}

	without := classify(CatalogExtras{})
	with := classify(CatalogExtras{AlternativeTitles: []tmdb.AlternativeTitle{{Country: "JP", Title: "Kusuriya no Hitorigoto", Type: "Romaji"}}})
	if len(with.Targets) != 1 || with.Targets[0].EpisodeID != 2005 || with.Confidence != identify.ConfidenceHigh {
		t.Errorf("with the alias: %+v %s (%s)", with.Targets, with.Confidence, with.Reason)
	}
	if without.Confidence == identify.ConfidenceHigh {
		t.Errorf("without the alias the name isn't the show's, yet it's high (%s)", without.Reason)
	}
}
