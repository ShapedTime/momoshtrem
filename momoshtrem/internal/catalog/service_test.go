package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// golden compares v's JSON with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run go test -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file (go test ./internal/catalog -run %s -update to accept):\n%s", name, t.Name(), got)
	}
}

func TestMovieTitle_Golden(t *testing.T) {
	f := newFixture()
	title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "movie_title.json", title)

	lib := title.Library
	if lib.AvailabilityStage != StageReady || lib.Torrent == nil || lib.Torrent.State != TorrentIdle || lib.Download != nil {
		t.Errorf("an idle torrent with a partial file is ready, not downloading: %+v", lib)
	}
	if *lib.WebDAVPath != "/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mkv" ||
		*lib.WebDAVURL != "https://momoshtrem.local:36911/Movies/Dune-%20Part%20Two%20%282024%29/Dune-%20Part%20Two%20%282024%29.mkv" {
		t.Errorf("paths = %s %s", *lib.WebDAVPath, *lib.WebDAVURL)
	}
	if len(title.Similar) != 2 || title.Similar[0].Library == nil || title.Similar[0].Library.AvailabilityStage != StageDownloading ||
		title.Similar[1].Library != nil {
		t.Errorf("similar = %+v", title.Similar)
	}
	if f.store.lastQuery.OnlyTMDB == nil || !reflect.DeepEqual(f.store.lastQuery.OnlyTMDB.Movies, []int{157336, 438631}) {
		t.Errorf("annotation query = %+v", f.store.lastQuery.OnlyTMDB)
	}
}

func TestMovieTitle_TorrentStates(t *testing.T) {
	tests := []struct {
		name        string
		loaded      *fakeTorrent
		wantStage   Stage
		wantState   TorrentState
		wantSeeders *int
		wantPct     int
	}{
		{"not loaded", nil, StageReady, TorrentNotLoaded, nil, -1},
		{"loaded and idle", &fakeTorrent{active: false, seeders: 3}, StageReady, TorrentIdle, intPtr(3), -1},
		{"active, file complete", &fakeTorrent{active: true, seeders: 5, files: map[string]torrent.FileProgress{
			"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv": {Length: 100, BytesCompleted: 100}}},
			StageReady, TorrentActive, intPtr(5), -1},
		{"active, file partial", &fakeTorrent{active: true, seeders: 9, files: map[string]torrent.FileProgress{
			"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv": {Length: 1000, BytesCompleted: 429}}},
			StageDownloading, TorrentActive, intPtr(9), 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			delete(f.torrents.loaded, duneHash)
			if tt.loaded != nil {
				f.torrents.loaded[duneHash] = *tt.loaded
			}
			title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
			if err != nil {
				t.Fatal(err)
			}
			lib := title.Library
			if lib.AvailabilityStage != tt.wantStage || lib.Torrent.State != tt.wantState || !reflect.DeepEqual(lib.Torrent.Seeders, tt.wantSeeders) {
				t.Errorf("stage %s state %s seeders %v", lib.AvailabilityStage, lib.Torrent.State, deref2(lib.Torrent.Seeders))
			}
			if tt.wantPct < 0 && lib.Download != nil {
				t.Errorf("download = %+v, want nil", lib.Download)
			}
			if tt.wantPct >= 0 && (lib.Download == nil || lib.Download.Pct != tt.wantPct || lib.Download.ETASeconds != nil ||
				lib.Download.SpeedBps != 0 || lib.Download.Seeders != *tt.wantSeeders) {
				t.Errorf("download = %+v, want %d%%", lib.Download, tt.wantPct)
			}
		})
	}
}

func TestMovieTitle_NotInLibrary(t *testing.T) {
	f := newFixture()
	delete(f.store.movies, duneTMDB)
	title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	want := TitleLibraryState{AvailabilityStage: StageNone}
	if !reflect.DeepEqual(title.Library, want) {
		t.Errorf("library = %+v, want %+v", title.Library, want)
	}
	if title.Title != "Dune: Part Two" || title.OriginalTitle != nil || *title.Year != 2024 || title.Directors[0] != "Denis Villeneuve" {
		t.Errorf("title = %+v", title)
	}
}

func TestMovieTitle_MovieWithoutTorrent(t *testing.T) {
	f := newFixture()
	f.store.movies[duneTMDB].Assignment = nil
	title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	lib := title.Library
	if lib.AvailabilityStage != StageNeedsTorrent || lib.WebDAVPath != nil || lib.WebDAVURL != nil || lib.Torrent != nil ||
		!lib.InLibrary || *lib.LibraryID != 7 || !lib.AddedAt.Equal(addedAt) {
		t.Errorf("library = %+v", lib)
	}
}

func TestTitles_TMDBFailures(t *testing.T) {
	down := &tmdb.StatusError{StatusCode: 503}
	tests := []struct {
		name      string
		inLibrary bool
		err       error
		wantErr   error // nil: falls back to the library
	}{
		{"down, in library: stored metadata", true, down, nil},
		{"down, not in library", false, down, library.ErrTMDBFetch},
		{"unknown id, not in library", false, tmdb.ErrNotFound, tmdb.ErrNotFound},
		{"no key, even in library", true, tmdb.ErrNoAPIKey, tmdb.ErrNoAPIKey},
		{"no key, not in library", false, tmdb.ErrNoAPIKey, tmdb.ErrNoAPIKey},
	}
	for _, tt := range tests {
		t.Run("movie: "+tt.name, func(t *testing.T) {
			f := newFixture()
			f.tmdb.err = tt.err
			if !tt.inLibrary {
				delete(f.store.movies, duneTMDB)
			}
			title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || !errors.Is(err, library.ErrTMDBFetch) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if title.Title != "Dune: Part Two" || len(title.Cast) != 0 || len(title.Similar) != 0 || *title.Runtime != 167 ||
				title.Genres[0] != "Science Fiction" || *title.BackdropDominantColor != "#7a4a22" || title.Library.AvailabilityStage != StageReady {
				t.Errorf("fallback title = %+v", title)
			}
		})
		t.Run("show: "+tt.name, func(t *testing.T) {
			f := newFixture()
			f.tmdb.err = tt.err
			if !tt.inLibrary {
				delete(f.store.shows, frierenTMDB)
			}
			title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || !errors.Is(err, library.ErrTMDBFetch) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *title.OriginalTitle != "葬送のフリーレン" || len(title.Cast) != 0 || len(title.Creators) != 0 || !title.IsAnime ||
				*title.Status != "Returning Series" || *title.EpisodeRunTime != 24 || len(title.Seasons) != 2 ||
				title.Library.AvailabilityStage != StageNeedsTorrent {
				t.Errorf("fallback title = %+v", title)
			}
		})
	}
}

func TestShowTitle_Golden(t *testing.T) {
	f := newFixture()
	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "show_title.json", title)

	lib := title.Library
	want := &MissingRun{SeasonNumber: 1, From: 29, To: 38, AbsoluteFrom: intPtr(29), AbsoluteTo: intPtr(38), Count: 10}
	if lib.AvailabilityStage != StageNeedsTorrent || !reflect.DeepEqual(lib.Missing, want) || *lib.EpisodesTotal != 38 || *lib.EpisodesReady != 28 {
		t.Errorf("library = %+v (missing %+v)", lib, lib.Missing)
	}
	if lib.Download == nil || lib.Download.Pct != 42 || lib.Download.Seeders != 41 {
		t.Errorf("download = %+v, want E28's 42%%", lib.Download)
	}
	if *lib.WebDAVPath != "/TV Shows/Frieren- Beyond Journey's End (2023)/" {
		t.Errorf("webdav path = %s", *lib.WebDAVPath)
	}
	if title.Seasons[0].SeasonNumber != 1 || title.Seasons[1].SeasonNumber != 0 || *title.DefaultSeason != 1 {
		t.Errorf("seasons = %+v, default %v", title.Seasons, deref2(title.DefaultSeason))
	}
	if title.EpisodeOrdering != "absolute" || len(title.Similar) != 2 || title.Similar[0].Library.LibraryID != 4 {
		t.Errorf("title = %+v", title)
	}
}

func TestShowTitle_ReadyAndDownloading(t *testing.T) {
	f := newFixture()
	f.store.seasons[3] = frierenSeasons(28, 28)

	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if title.Library.AvailabilityStage != StageDownloading || title.Library.Missing != nil {
		t.Errorf("stage = %s, missing %+v; want downloading", title.Library.AvailabilityStage, title.Library.Missing)
	}

	f.torrents.loaded[frierenHash] = fakeTorrent{active: false}
	title, err = f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if title.Library.AvailabilityStage != StageReady || title.Library.Download != nil {
		t.Errorf("stage = %s, download %+v; want ready", title.Library.AvailabilityStage, title.Library.Download)
	}
}

func TestShowTitle_SpecialsOff(t *testing.T) {
	f := newFixture()
	f.store.shows[frierenTMDB].IncludeSpecials = false
	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(title.Seasons) != 1 || title.Seasons[0].SeasonNumber != 1 || title.IncludeSpecials {
		t.Errorf("seasons = %+v", title.Seasons)
	}
	if _, err := f.svc.Season(context.Background(), frierenTMDB, 0); !errors.Is(err, ErrSeasonNotFound) {
		t.Errorf("hidden specials season: err = %v, want ErrSeasonNotFound", err)
	}
}

func TestShowTitle_NotInLibrary(t *testing.T) {
	f := newFixture()
	delete(f.store.shows, frierenTMDB)
	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	wantSeasons := []SeasonSummary{
		{SeasonNumber: 1, Name: "Season 1", EpisodesTotal: 38},
		{SeasonNumber: 0, Name: "Specials", EpisodesTotal: 2},
	}
	if !reflect.DeepEqual(title.Seasons, wantSeasons) || *title.DefaultSeason != 1 {
		t.Errorf("seasons = %+v, default %v", title.Seasons, deref2(title.DefaultSeason))
	}
	if title.Library.AvailabilityStage != StageNone || title.Library.EpisodesTotal != nil || title.Library.InLibrary {
		t.Errorf("library = %+v", title.Library)
	}
	// Anime defaults, as the show would get when added.
	if !title.IsAnime || title.EpisodeOrdering != "absolute" || !title.IncludeSpecials || title.ClassifierMode != "auto" || title.EpisodeGroupID != nil {
		t.Errorf("settings = %+v", title)
	}
	if f.store.calls["show_episodes"] != 0 {
		t.Error("episodes loaded for a show that isn't in the library")
	}
}

func TestSeason_Golden(t *testing.T) {
	f := newFixture()
	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "season.json", season)

	e28, e29 := season.Episodes[27], season.Episodes[28]
	if e28.State != EpisodeDownloading || *e28.Pct != 42 ||
		*e28.WebDAVPath != "/TV Shows/Frieren- Beyond Journey's End (2023)/Season 01/Frieren- Beyond Journey's End - S01E28 - Episode 28.mkv" {
		t.Errorf("E28 = %+v", e28)
	}
	if e29.State != EpisodeMissing || e29.WebDAVPath != nil || e29.Pct != nil {
		t.Errorf("E29 = %+v", e29)
	}
	if len(season.Ranges) != 4 || season.Ranges[3].Label != "31–38" || season.Ranges[2].Missing != 2 {
		t.Errorf("ranges = %+v", season.Ranges)
	}
}

func TestSeason_Errors(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.Season(context.Background(), 1, 1); !errors.Is(err, ErrNotInLibrary) {
		t.Errorf("unknown show: %v", err)
	}
	if _, err := f.svc.Season(context.Background(), frierenTMDB, 5); !errors.Is(err, ErrSeasonNotFound) {
		t.Errorf("unknown season: %v", err)
	}
	specials, err := f.svc.Season(context.Background(), frierenTMDB, 0)
	if err != nil || len(specials.Episodes) != 2 || specials.Episodes[1].State != EpisodeUpcoming || len(specials.Ranges) != 0 {
		t.Errorf("specials = %+v, %v", specials, err)
	}
	f.store.err = errors.New("db down")
	if _, err := f.svc.Season(context.Background(), frierenTMDB, 1); err == nil || errors.Is(err, ErrNotInLibrary) {
		t.Errorf("db error = %v", err)
	}
}

func TestSeason_StoryArcs(t *testing.T) {
	f := newFixture()
	f.tmdb.groupLists[frierenTMDB] = []tmdb.EpisodeGroupSummary{
		{ID: "abs", Type: tmdb.GroupAbsolute, EpisodeCount: 38},
		{ID: "arcs-small", Type: tmdb.GroupStoryArc, EpisodeCount: 10},
		{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 38},
	}
	eps := func(from, to int) []tmdb.GroupEpisode {
		var out []tmdb.GroupEpisode
		for n := from; n <= to; n++ {
			out = append(out, tmdb.GroupEpisode{SeasonNumber: 1, EpisodeNumber: n})
		}
		return out
	}
	f.tmdb.groups["arcs"] = &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{
		{Name: "Journey's End", Order: 0, Episodes: eps(1, 17)},
		{Name: "First-Class Mage Exam", Order: 1, Episodes: eps(18, 28)},
	}}

	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, r := range season.Ranges {
		labels = append(labels, r.Kind+":"+r.Label)
	}
	want := []string{"arc:Journey's End", "arc:First-Class Mage Exam", "block:29–38"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("ranges = %v, want %v", labels, want)
	}
	if season.Ranges[2].Missing != 10 {
		t.Errorf("trailing block missing = %d, want 10", season.Ranges[2].Missing)
	}

	// TMDB down: numbered blocks instead.
	f.tmdb.err = &tmdb.StatusError{StatusCode: 500}
	season, err = f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil || len(season.Ranges) != 4 || season.Ranges[0].Kind != "block" {
		t.Errorf("ranges without TMDB = %+v, %v", season.Ranges, err)
	}
}

// The number of database and TMDB calls doesn't grow with the show.
func TestCallCounts_DontGrowWithEpisodes(t *testing.T) {
	type counts struct{ store, tmdb int }
	measure := func(episodes int, fn func(*fixture)) counts {
		f := newFixture()
		f.store.seasons[3] = frierenSeasons(episodes, episodes/2)
		fn(f)
		return counts{f.store.total(), f.tmdb.total()}
	}

	showTitle := func(f *fixture) {
		if _, err := f.svc.ShowTitle(context.Background(), frierenTMDB); err != nil {
			t.Fatal(err)
		}
	}
	small, large := measure(10, showTitle), measure(1000, showTitle)
	if small != large || small.store != 3 || small.tmdb != 1 {
		t.Errorf("ShowTitle calls: 10 episodes %+v, 1,000 episodes %+v; want {3 1} for both", small, large)
	}

	season := func(f *fixture) {
		f.tmdb.groupLists[frierenTMDB] = []tmdb.EpisodeGroupSummary{{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 5}}
		f.tmdb.groups["arcs"] = &tmdb.EpisodeGroup{}
		if _, err := f.svc.Season(context.Background(), frierenTMDB, 1); err != nil {
			t.Fatal(err)
		}
	}
	small, large = measure(10, season), measure(1000, season)
	if small.store != 1 || large.store != 1 || small.tmdb != 0 || large.tmdb != 2 {
		t.Errorf("Season calls: 10 episodes %+v, 1,000 episodes %+v; want one store call, and two TMDB calls for arcs only past 20 episodes", small, large)
	}
	if long := measure(25, season); long != large {
		t.Errorf("Season calls: 25 episodes %+v, 1,000 episodes %+v", long, large)
	}
}

func TestLibrary(t *testing.T) {
	f := newFixture()
	f.store.library = []library.LibraryItem{
		{MediaType: "tv", ID: 3, TMDBID: frierenTMDB, Title: "Frieren", Year: 2023, PosterPath: "/f.jpg", CreatedAt: addedAt,
			EpisodesTotal: intPtr(38), EpisodesReady: intPtr(28), SizeBytes: 28_000, Stage: "needs_torrent"},
		{MediaType: "movie", ID: 7, TMDBID: duneTMDB, Title: "Dune", Year: 2024, CreatedAt: addedAt, SizeBytes: 18_400, Stage: "ready"},
	}
	f.store.libTotal = 51

	page, err := f.svc.Library(context.Background(), LibraryParams{Status: StatusIncomplete, Sort: "size", Query: "fr", Page: 2, PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "library.json", page)

	q := f.store.lastQuery
	if q.Limit != MaxPageSize || q.Offset != MaxPageSize || q.Status != "incomplete" || q.Sort != "size" || q.Search != "fr" || q.Today != "2026-10-14" {
		t.Errorf("query = %+v", q)
	}
	got := map[library.FileRef]bool{}
	for _, ref := range q.Downloading {
		got[ref] = true
	}
	if len(got) != 1 || !got[library.FileRef{InfoHash: frierenHash, Path: "Frieren S01/Frieren.S01E28.mkv"}] {
		t.Errorf("downloading = %+v, want Frieren's E28 only (Dune's torrent is idle)", q.Downloading)
	}

	page, err = f.svc.Library(context.Background(), LibraryParams{})
	if err != nil || page.Page != 1 || page.PageSize != DefaultPageSize || f.store.lastQuery.Offset != 0 || f.store.lastQuery.Limit != 50 {
		t.Errorf("defaults = %+v (%+v), %v", page, f.store.lastQuery, err)
	}

	for _, status := range []string{StatusWatching, StatusNeedsReview} {
		before := f.store.total()
		page, err := f.svc.Library(context.Background(), LibraryParams{Status: status})
		if err != nil || page.Total != 0 || len(page.Items) != 0 || page.Items == nil || f.store.total() != before {
			t.Errorf("%s = %+v, %v (store calls %d)", status, page, err, f.store.total()-before)
		}
	}

	for _, p := range []LibraryParams{{MediaType: "music"}, {Status: "done"}, {Sort: "rating"}, {Page: -1}, {PageSize: -5}} {
		if _, err := f.svc.Library(context.Background(), p); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: err = %v, want ErrInvalidQuery", p, err)
		}
	}
}

func TestPatchShow(t *testing.T) {
	f := newFixture()
	off := false
	title, err := f.svc.PatchShow(context.Background(), 3, metadata.SettingsPatch{IncludeSpecials: &off})
	if err != nil {
		t.Fatal(err)
	}
	if f.settings.patch.IncludeSpecials == nil || title.IncludeSpecials || len(title.Seasons) != 1 {
		t.Errorf("patched title = %+v", title)
	}

	f.settings.err = library.ErrInvalidShowSettings
	if _, err := f.svc.PatchShow(context.Background(), 3, metadata.SettingsPatch{}); !errors.Is(err, library.ErrInvalidShowSettings) {
		t.Errorf("invalid: err = %v", err)
	}
	if _, err := New(Deps{Store: f.store, TMDB: f.tmdb}).PatchShow(context.Background(), 3, metadata.SettingsPatch{}); !errors.Is(err, ErrSettingsUnavailable) {
		t.Errorf("no settings service: err = %v", err)
	}
}

func TestEpisodeGroups(t *testing.T) {
	f := newFixture()
	f.tmdb.groupLists[frierenTMDB] = []tmdb.EpisodeGroupSummary{{ID: "g1", Name: "Absolute", Type: 2, GroupCount: 1, EpisodeCount: 38, Description: "x"}}
	groups, err := f.svc.EpisodeGroups(context.Background(), 3)
	if err != nil || !reflect.DeepEqual(groups, []EpisodeGroup{{ID: "g1", Name: "Absolute", Type: 2, GroupCount: 1, EpisodeCount: 38}}) {
		t.Errorf("groups = %+v, %v", groups, err)
	}
	if _, err := f.svc.EpisodeGroups(context.Background(), 99); !errors.Is(err, library.ErrShowNotFound) {
		t.Errorf("unknown show: %v", err)
	}
	f.tmdb.err = tmdb.ErrNoAPIKey
	if _, err := f.svc.EpisodeGroups(context.Background(), 3); !errors.Is(err, tmdb.ErrNoAPIKey) || !errors.Is(err, library.ErrTMDBFetch) {
		t.Errorf("no key: %v", err)
	}
	f.tmdb.err = nil
	delete(f.tmdb.groupLists, frierenTMDB)
	if groups, err := f.svc.EpisodeGroups(context.Background(), 3); err != nil || groups == nil || len(groups) != 0 {
		t.Errorf("no groups = %#v, %v", groups, err)
	}
}

func TestReleaseName(t *testing.T) {
	tests := []struct {
		magnet, path, want string
	}{
		{"magnet:?xt=urn:btih:abc&dn=Dune.Part.Two.2024.2160p-FLUX&tr=udp%3A%2F%2Ft", "Dune/dune.mkv", "Dune.Part.Two.2024.2160p-FLUX"},
		{"magnet:?xt=urn:btih:abc&dn=Space+Name%20Here", "x.mkv", "Space Name Here"},
		{"magnet:?xt=urn:btih:abc", "Frieren.S01.1080p.WEB-DL/Frieren.S01E01.mkv", "Frieren.S01.1080p.WEB-DL"},
		{"magnet:?xt=urn:btih:abc", "Past.Lives.2023.1080p.WEB-DL.mkv", "Past.Lives.2023.1080p.WEB-DL"},
		{"not a url %%", "a/b.mkv", "a"},
	}
	for _, tt := range tests {
		got := releaseName(&library.TorrentAssignment{MagnetURI: tt.magnet, FilePath: tt.path})
		if got != tt.want {
			t.Errorf("releaseName(%q, %q) = %q, want %q", tt.magnet, tt.path, got, tt.want)
		}
	}
}

// Every JSON shape carries exactly the fields of the TypeScript contract
// (web/src/lib/api/contract/titles.ts and library.ts).
func TestContractFields(t *testing.T) {
	f := newFixture()
	movie, err := f.svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	show, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	f.store.library = []library.LibraryItem{{MediaType: "tv", ID: 1, Stage: "ready"}}
	page, err := f.svc.Library(context.Background(), LibraryParams{})
	if err != nil {
		t.Fatal(err)
	}

	base := []string{"media_type", "tmdb_id", "title", "original_title", "year", "overview", "poster_path", "backdrop_path",
		"backdrop_dominant_color", "genres", "vote_average", "vote_count", "original_language", "cast", "trailer_youtube_key",
		"similar", "library", "watch_state", "recommendation_reason"}
	libraryState := []string{"in_library", "library_id", "added_at", "availability_stage", "webdav_path", "webdav_url", "download",
		"needs_review", "review_job_id", "episodes_total", "episodes_ready", "missing", "torrent"}

	tests := []struct {
		name string
		v    any
		want []string
	}{
		{"MovieTitle", movie, append(append([]string{}, base...), "runtime", "directors")},
		{"ShowTitle", show, append(append([]string{}, base...), "status", "is_anime", "episode_ordering", "episode_group_id",
			"include_specials", "classifier_mode", "creators", "networks", "end_year", "episode_run_time", "seasons", "default_season")},
		{"TitleLibraryState", movie.Library, libraryState},
		{"TitleTorrent", movie.Library.Torrent, []string{"info_hash", "release_name", "file_name", "size_bytes", "resolution", "source",
			"added_at", "state", "seeders"}},
		{"DownloadProgress", show.Library.Download, []string{"pct", "speed_bps", "eta_seconds", "seeders"}},
		{"MissingRun", show.Library.Missing, []string{"season_number", "from", "to", "absolute_from", "absolute_to", "count"}},
		{"CastMember", movie.Cast[0], []string{"name", "character", "profile_path"}},
		{"SimilarTitle", movie.Similar[0], []string{"media_type", "tmdb_id", "title", "year", "poster_path", "library"}},
		{"SimilarTitle.library", movie.Similar[0].Library, []string{"library_id", "availability_stage", "episodes_total", "episodes_ready"}},
		{"SeasonSummary", show.Seasons[0], []string{"season_number", "name", "episodes_total", "ready", "missing", "needs_review",
			"upcoming", "absolute_from", "absolute_to"}},
		{"SeasonDetail", season, []string{"season_number", "name", "episodes", "ranges"}},
		{"EpisodeSummary", season.Episodes[0], []string{"id", "season_number", "episode_number", "absolute_number", "name", "overview",
			"air_date", "still_path", "runtime", "episode_type", "state", "pct", "webdav_path"}},
		{"EpisodeRange", season.Ranges[0], []string{"label", "kind", "from", "to", "absolute_from", "absolute_to", "missing"}},
		{"Paginated", page, []string{"items", "total", "page", "page_size"}},
		{"LibraryItem", page.Items[0], []string{"id", "media_type", "tmdb_id", "title", "year", "poster_path", "availability_stage",
			"episodes_total", "episodes_ready", "episodes_watched", "needs_review", "size_bytes", "added_at", "last_watched_at"}},
		{"EpisodeGroup", EpisodeGroup{}, []string{"id", "name", "type", "group_count", "episode_count"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.v)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			var got []string
			for k := range fields {
				got = append(got, k)
			}
			sort.Strings(got)
			want := append([]string{}, tt.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("fields =\n%v\nwant\n%v", got, want)
			}
		})
	}
}
