package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
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

// The torrent's state is the client's. Nothing keeps Dune, so it streams
// on demand: ready, never downloading, however much of it is cached.
func TestMovieTitle_TorrentStates(t *testing.T) {
	tests := []struct {
		name        string
		loaded      *fakeTorrent
		paused      bool
		wantState   TorrentState
		wantSeeders *int
	}{
		{"not loaded", nil, false, TorrentNotLoaded, nil},
		{"loaded and idle", &fakeTorrent{active: false, seeders: 3}, false, TorrentIdle, intPtr(3)},
		{"active, fetching around a reader", &fakeTorrent{active: true, seeders: 9, speedBps: 2_500_000}, false, TorrentActive, intPtr(9)},
		{"paused, not loaded", nil, true, TorrentPaused, nil},
		// A paused torrent's network is off, so the client reports it inactive.
		{"paused, loaded", &fakeTorrent{active: false, seeders: 4}, true, TorrentPaused, intPtr(4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			delete(f.torrents.loaded, duneHash)
			if tt.loaded != nil {
				f.torrents.loaded[duneHash] = *tt.loaded
			}
			f.torrents.paused = map[string]bool{duneHash: tt.paused}
			title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
			if err != nil {
				t.Fatal(err)
			}
			lib := title.Library
			if lib.AvailabilityStage != StageReady || lib.Torrent.State != tt.wantState || !reflect.DeepEqual(lib.Torrent.Seeders, tt.wantSeeders) {
				t.Errorf("stage %s state %s seeders %v", lib.AvailabilityStage, lib.Torrent.State, deref2(lib.Torrent.Seeders))
			}
			if lib.Download != nil || lib.Keep != nil {
				t.Errorf("download %+v, keep %+v; want nil: nothing keeps it", lib.Download, lib.Keep)
			}
		})
	}
}

// A movie is downloading only while a download wants its file and it
// isn't all on disk (#77), not while that download is paused. The ETA is
// what's left at the torrent's speed, while the manager downloads it.
func TestMovieTitle_Downloading(t *testing.T) {
	const duneFile = "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv"
	keep := downloads.Keep{DownloadID: 9, Reason: library.DownloadKept, Scope: library.DownloadScopeMovie,
		State: downloads.StateDownloading, Pct: 42, UntilWatched: true, ExpiresAt: &keepExpires}
	tests := []struct {
		name      string
		done      int64 // of 1,000 bytes
		state     downloads.State
		speedBps  int64
		wantStage Stage
		wantPct   int  // -1: no download
		wantETA   *int // seconds
	}{
		{"downloading", 429, downloads.StateDownloading, 100, StageDownloading, 42, intPtr(6)},
		{"downloading, no speed yet", 429, downloads.StateDownloading, 0, StageDownloading, 42, nil},
		{"queued, bytes arriving from a stream", 429, downloads.StateQueued, 100, StageDownloading, 42, nil},
		{"waiting for the window", 0, downloads.StateWaitingForWindow, 0, StageDownloading, 0, nil},
		{"stalled", 10, downloads.StateStalled, 0, StageDownloading, 1, nil},
		{"held by a full disk", 10, downloads.StateDiskFull, 0, StageDownloading, 1, nil},
		{"paused", 429, downloads.StatePaused, 0, StageReady, -1, nil},
		{"on disk", 1_000, downloads.StateDownloading, 100, StageReady, -1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			f.torrents.loaded[duneHash] = fakeTorrent{active: true, seeders: 9, speedBps: tt.speedBps}
			f.downloads.want(duneHash, duneFile, 1_000, tt.done, tt.state)
			f.downloads.movies[7] = keep
			title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
			if err != nil {
				t.Fatal(err)
			}
			lib := title.Library
			if lib.AvailabilityStage != tt.wantStage || !reflect.DeepEqual(lib.Keep, &keep) {
				t.Errorf("stage %s keep %+v", lib.AvailabilityStage, lib.Keep)
			}
			if tt.wantPct < 0 {
				if lib.Download != nil {
					t.Errorf("download = %+v, want nil", lib.Download)
				}
				return
			}
			d := lib.Download
			if d == nil {
				t.Fatalf("download = nil, want %d%%", tt.wantPct)
			}
			if d.Pct != tt.wantPct || d.SpeedBps != tt.speedBps || d.Seeders != 9 || !reflect.DeepEqual(d.ETASeconds, tt.wantETA) {
				t.Errorf("download = %+v (eta %v), want %d%%, eta %v", d, deref2(d.ETASeconds), tt.wantPct, deref2(tt.wantETA))
			}
		})
	}

	// Without a downloads service nothing is ever downloading, and keep is
	// null.
	f := newFixture()
	f.torrents.loaded[duneHash] = fakeTorrent{active: true, speedBps: 100}
	svc := New(Deps{Store: f.store, TMDB: f.tmdb, Torrents: f.torrents, Now: func() time.Time { return fixedNow }})
	title, err := svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil || title.Library.AvailabilityStage != StageReady || title.Library.Download != nil || title.Library.Keep != nil {
		t.Errorf("no downloads: %+v, %v", title.Library, err)
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
	if lib.Keep == nil || lib.Keep.DownloadID != 4 || lib.Keep.Scope != library.DownloadScopeShow {
		t.Errorf("keep = %+v, want the show-scope download", lib.Keep)
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

// A show's download is its downloading files together: what's on disk of
// them, their torrents' speeds added up (each torrent once, however many of
// its files download), and the slowest torrent's ETA while every one is in
// a download slot. A paused download's files aren't downloading.
func TestShowTitle_DownloadSpeed(t *testing.T) {
	const otherHash = "0123456789abcdef0123456789abcdef01234567"
	f := newFixture()
	e25 := &f.store.seasons[3][1].Episodes[24]
	e25.Assignment.InfoHash, e25.Assignment.FilePath = otherHash, "Other/Frieren.S01E25.mkv"
	f.torrents.loaded[frierenHash] = fakeTorrent{active: true, seeders: 41, speedBps: 100}
	f.torrents.loaded[otherHash] = fakeTorrent{active: true, seeders: 7, speedBps: 50}
	f.downloads.want(frierenHash, "Frieren S01/Frieren.S01E26.mkv", 1_000, 100, downloads.StateDownloading)
	f.downloads.want(otherHash, "Other/Frieren.S01E25.mkv", 1_000, 0, downloads.StateDownloading)

	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	// Frieren's torrent has 900 + 580 bytes left at 100 B/s (15 s), the
	// other 1,000 at 50 B/s (20 s).
	if d := title.Library.Download; d == nil || d.SpeedBps != 150 || d.Pct != 17 || d.Seeders != 41 || !reflect.DeepEqual(d.ETASeconds, intPtr(20)) {
		t.Fatalf("download = %+v, want 150 B/s over both torrents, 17%%, 20 s", d)
	}

	// The other torrent waits for a slot: its speed still counts, but
	// there's no ETA.
	f.downloads.want(otherHash, "Other/Frieren.S01E25.mkv", 1_000, 0, downloads.StateQueued)
	f.torrents.loaded[otherHash] = fakeTorrent{active: true, seeders: 7}
	title, err = f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if d := title.Library.Download; d == nil || d.SpeedBps != 100 || d.Pct != 17 || d.ETASeconds != nil {
		t.Fatalf("download = %+v, want 100 B/s, 17%%, no ETA", d)
	}

	// Its download is paused: only Frieren's torrent downloads.
	f.torrents.paused = map[string]bool{otherHash: true}
	f.downloads.want(otherHash, "Other/Frieren.S01E25.mkv", 1_000, 0, downloads.StatePaused)
	title, err = f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if d := title.Library.Download; d == nil || d.SpeedBps != 100 || d.Pct != 26 || !reflect.DeepEqual(d.ETASeconds, intPtr(15)) {
		t.Fatalf("download = %+v, want only Frieren's torrent at 100 B/s, 26%%, 15 s", d)
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

	// E28's download is paused (its torrent, or background downloads off).
	f.downloads.want(frierenHash, "Frieren S01/Frieren.S01E28.mkv", 1_000, 420, downloads.StatePaused)
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

	// The season's own keep; each episode's keep is the season's, with its
	// own file's state and pct (E27 is on disk, E28 downloading).
	if k := season.Keep; k == nil || k.DownloadID != 5 || k.Scope != library.DownloadScopeSeason || k.Pct != 71 {
		t.Errorf("season keep = %+v", k)
	}
	if k := season.Episodes[26].Keep; k == nil || k.DownloadID != 5 || k.State != downloads.StateDone || k.Pct != 100 ||
		season.Episodes[26].State != EpisodeReady || season.Episodes[26].Pct != nil {
		t.Errorf("E27 = %+v, keep %+v", season.Episodes[26], k)
	}
	if k := season.Episodes[27].Keep; k == nil || k.State != downloads.StateDownloading || k.Pct != 42 {
		t.Errorf("E28 keep = %+v", k)
	}
	if season.Episodes[0].Keep != nil || season.Episodes[28].Keep != nil {
		t.Errorf("E01 keep %+v, E29 keep %+v; want nil", season.Episodes[0].Keep, season.Episodes[28].Keep)
	}
	if specials, err := f.svc.Season(context.Background(), frierenTMDB, 0); err != nil || specials.Keep != nil {
		t.Errorf("specials keep = %+v, %v; want nil", specials, err)
	}

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

// hdrezkaAssignment is an HDRezka stream as the item's file.
func hdrezkaAssignment(itemType library.ItemType, itemID int64, ref library.StreamRef, size int64) *library.TorrentAssignment {
	return &library.TorrentAssignment{ID: 900, ItemType: itemType, ItemID: itemID, Kind: library.AssignmentKindHDRezka,
		FilePath: ref.FilePath(), FileSize: size, Resolution: "1080p", Stream: &ref, IsActive: true, CreatedAt: addedAt.Add(2 * time.Hour)}
}

func TestMovieTitle_HDRezkaStream(t *testing.T) {
	ref := library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/12345-dyuna-chast-vtoraya-2024.html", PostID: 12345,
		Title: "Dune: Part Two", TranslatorID: 238, TranslatorName: "Оригинал (+субтитры)", Quality: "1080p"}
	for _, tt := range []struct{ base, want string }{
		{"", "https://rezka.ag/films/fiction/12345-dyuna-chast-vtoraya-2024.html"},
		{"https://hdrezka.me/", "https://hdrezka.me/films/fiction/12345-dyuna-chast-vtoraya-2024.html"},
	} {
		f := newFixture()
		f.svc.hdrezkaURL = New(Deps{HDRezkaURL: tt.base}).hdrezkaURL
		f.store.movies[duneTMDB].Assignment = hdrezkaAssignment(library.ItemTypeMovie, 7, ref, 3_322_740_623)
		title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
		if err != nil {
			t.Fatal(err)
		}
		lib := title.Library
		want := &TitleStream{Provider: "hdrezka", Title: "Dune: Part Two", URL: tt.want, Translator: "Оригинал (+субтитры)",
			Quality: "1080p", SizeBytes: 3_322_740_623, AddedAt: addedAt.Add(2 * time.Hour)}
		if !reflect.DeepEqual(lib.Stream, want) {
			t.Errorf("stream = %+v, want %+v", lib.Stream, want)
		}
		if lib.Torrent != nil || lib.Download != nil || lib.AvailabilityStage != StageReady ||
			*lib.WebDAVPath != "/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mp4" {
			t.Errorf("library = %+v", lib)
		}
	}

	// A torrent movie has no stream.
	f := newFixture()
	title, _ := f.svc.MovieTitle(context.Background(), duneTMDB)
	if title.Library.Stream != nil || title.Library.Torrent == nil {
		t.Errorf("torrent movie: stream %+v, torrent %+v", title.Library.Stream, title.Library.Torrent)
	}
}

func TestSeason_Sources(t *testing.T) {
	f := newFixture()
	ref := library.StreamRef{Provider: "hdrezka", PostID: 77, TranslatorID: 238, TranslatorName: "Оригинал (+субтитры)",
		Season: 1, Episode: 2, Quality: "720p"}
	s1 := &f.store.seasons[3][1]
	s1.Episodes[1].Assignment = hdrezkaAssignment(library.ItemTypeEpisode, s1.Episodes[1].ID, ref, 500)

	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	e1, e2, e30 := season.Episodes[0], season.Episodes[1], season.Episodes[29]
	if e1.Source == nil || *e1.Source != SourceTorrent || e1.Stream != nil {
		t.Errorf("E1 (torrent) = %v %+v", e1.Source, e1.Stream)
	}
	if e2.Source == nil || *e2.Source != SourceHDRezka || !reflect.DeepEqual(e2.Stream, &EpisodeStream{"Оригинал (+субтитры)", "720p"}) ||
		e2.State != EpisodeReady || e2.Pct != nil || !strings.HasSuffix(*e2.WebDAVPath, " - S01E02 - Episode 2.mp4") {
		t.Errorf("E2 (HDRezka) = %+v, stream %+v", e2, e2.Stream)
	}
	if e30.Source != nil || e30.Stream != nil {
		t.Errorf("E30 (no file) = %v %+v", e30.Source, e30.Stream)
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
		{Name: "Golden Land", Order: 2, Episodes: eps(29, 31)},
	}}

	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, r := range season.Ranges {
		labels = append(labels, r.Kind+":"+r.Label)
	}
	want := []string{"arc:Journey's End", "arc:First-Class Mage Exam", "arc:Golden Land", "block:32–38"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("ranges = %v, want %v", labels, want)
	}
	if season.Ranges[3].Missing != 7 {
		t.Errorf("trailing block missing = %d, want 7", season.Ranges[3].Missing)
	}

	// Arcs covering less than 80% of the season (28 of 38): blocks.
	f.tmdb.groups["arcs"].Groups = f.tmdb.groups["arcs"].Groups[:2]
	season, err = f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil || len(season.Ranges) != 4 || season.Ranges[0].Kind != "block" {
		t.Errorf("ranges with 28 of 38 covered = %+v, %v", season.Ranges, err)
	}

	// TMDB down: numbered blocks instead.
	f.tmdb.err = &tmdb.StatusError{StatusCode: 500}
	season, err = f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil || len(season.Ranges) != 4 || season.Ranges[0].Kind != "block" {
		t.Errorf("ranges without TMDB = %+v, %v", season.Ranges, err)
	}
}

// Long specials get blocks without looking up story arcs, even when the
// arc group lists them.
func TestSeason_SpecialsNeverUseArcs(t *testing.T) {
	f := newFixture()
	specials := &f.store.seasons[3][0]
	group := tmdb.EpisodeGroupGroup{Name: "Specials"}
	for n := 1; n <= 25; n++ {
		if n > len(specials.Episodes) {
			specials.Episodes = append(specials.Episodes, library.Episode{ID: int64(3000 + n), EpisodeNumber: n, Name: "Special", AirDate: "2010-01-01"})
		}
		group.Episodes = append(group.Episodes, tmdb.GroupEpisode{SeasonNumber: 0, EpisodeNumber: n})
	}
	f.tmdb.groupLists[frierenTMDB] = []tmdb.EpisodeGroupSummary{{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 25}}
	f.tmdb.groups["arcs"] = &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupGroup{group}}

	season, err := f.svc.Season(context.Background(), frierenTMDB, 0)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, r := range season.Ranges {
		labels = append(labels, r.Kind+":"+r.Label)
	}
	if want := []string{"block:1–10", "block:11–20", "block:21–25"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("ranges = %v, want %v", labels, want)
	}
	if n := f.tmdb.total(); n != 0 {
		t.Errorf("TMDB calls = %d, want none for specials", n)
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
	if small != large || small.store != 5 || small.tmdb != 1 {
		t.Errorf("ShowTitle calls: 10 episodes %+v, 1,000 episodes %+v; want {5 1} for both", small, large)
	}

	season := func(f *fixture) {
		f.tmdb.groupLists[frierenTMDB] = []tmdb.EpisodeGroupSummary{{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 5}}
		f.tmdb.groups["arcs"] = &tmdb.EpisodeGroup{}
		if _, err := f.svc.Season(context.Background(), frierenTMDB, 1); err != nil {
			t.Fatal(err)
		}
	}
	small, large = measure(10, season), measure(1000, season)
	if small.store != 2 || large.store != 2 || small.tmdb != 0 || large.tmdb != 2 {
		t.Errorf("Season calls: 10 episodes %+v, 1,000 episodes %+v; want two store calls (the season, the review), and two TMDB calls for arcs only past 20 episodes", small, large)
	}
	if long := measure(25, season); long != large {
		t.Errorf("Season calls: 25 episodes %+v, 1,000 episodes %+v", long, large)
	}
}

func TestLibrary(t *testing.T) {
	f := newFixture()
	reviewJob := int64(101)
	watchedAt := time.Date(2026, 10, 12, 21, 30, 0, 0, time.UTC)
	f.store.library = []library.LibraryItem{
		{MediaType: "tv", ID: 3, TMDBID: frierenTMDB, Title: "Frieren", Year: 2023, PosterPath: "/f.jpg", CreatedAt: addedAt,
			EpisodesTotal: intPtr(38), EpisodesReady: intPtr(28), EpisodesWatched: intPtr(26), LastWatchedAt: &watchedAt,
			SizeBytes: 28_000, Stage: "needs_torrent", NeedsReview: 2, ReviewJobID: &reviewJob, HasFile: true,
			// E28 is 42% fetched (the fixture's torrent client)
			Downloading: []library.FileRef{{InfoHash: frierenHash, Path: "Frieren S01/Frieren.S01E28.mkv"}},
			Missing:     &library.FrontierRun{SeasonNumber: 1, From: 29, To: 38, AbsoluteFrom: intPtr(29), AbsoluteTo: intPtr(38), Count: 10}},
		{MediaType: "movie", ID: 7, TMDBID: duneTMDB, Title: "Dune", Year: 2024, CreatedAt: addedAt, SizeBytes: 18_400, Stage: "ready",
			Runtime: 166, Resolution: "2160p", HasFile: true, FilePath: "Dune.Part.Two.2024.2160p/Dune.mkv"},
		{MediaType: "movie", ID: 9, TMDBID: 1054867, Title: "One Battle After Another", Year: 2025, CreatedAt: addedAt, Stage: "needs_torrent"},
	}
	f.store.libTotal = 51
	f.store.libCounts = library.LibraryCounts{All: 51, Movies: 20, Shows: 31, Watching: 7, Ready: 40, Incomplete: 11, NeedsReview: 2}
	f.store.libSummary = library.LibrarySummary{Total: 60, SizeBytes: 2_000_000_000_000, Ready: 45}

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
		t.Errorf("downloading = %+v, want Frieren's E28 only (E27 is on disk, and nothing keeps Dune)", q.Downloading)
	}
	// A paused download's files aren't downloading.
	f.downloads.want(frierenHash, "Frieren S01/Frieren.S01E28.mkv", 1_000, 420, downloads.StatePaused)
	if _, err := f.svc.Library(context.Background(), LibraryParams{}); err != nil || f.store.lastQuery.Downloading != nil {
		t.Errorf("paused: downloading = %+v, %v; want none", f.store.lastQuery.Downloading, err)
	}
	f.downloads.want(frierenHash, "Frieren S01/Frieren.S01E28.mkv", 1_000, 420, downloads.StateDownloading)

	page, err = f.svc.Library(context.Background(), LibraryParams{})
	if err != nil || page.Page != 1 || page.PageSize != DefaultPageSize || f.store.lastQuery.Offset != 0 || f.store.lastQuery.Limit != 50 {
		t.Errorf("defaults = %+v (%+v), %v", page, f.store.lastQuery, err)
	}

	page, err = f.svc.Library(context.Background(), LibraryParams{Page: math.MaxInt, PageSize: MaxPageSize})
	if err != nil || page.Page != MaxPage || f.store.lastQuery.Offset != (MaxPage-1)*MaxPageSize {
		t.Errorf("huge page = %+v (offset %d), %v; want it capped at %d", page, f.store.lastQuery.Offset, err, MaxPage)
	}

	// Watching is the database's filter, in the same one query; the page
	// carries the counts and summary.
	before := f.store.total()
	page, err = f.svc.Library(context.Background(), LibraryParams{Status: StatusWatching})
	if err != nil || f.store.lastQuery.Status != StatusWatching || f.store.total() != before+1 || page.Counts.All != 51 ||
		page.Counts.Watching != 7 || page.Summary.Total != 60 {
		t.Errorf("watching = %+v, %v (store calls %d)", page, err, f.store.total()-before)
	}

	// needs_review is the database's filter; each item carries its count.
	f.store.library[0].NeedsReview = 3
	page, err = f.svc.Library(context.Background(), LibraryParams{Status: StatusNeedsReview})
	if err != nil || f.store.lastQuery.Status != StatusNeedsReview || page.Items[0].NeedsReview != 3 || page.Items[1].NeedsReview != 0 {
		t.Errorf("needs_review = %+v (query %+v), %v", page, f.store.lastQuery, err)
	}

	// ids: one page of every title named, whatever page and page_size say.
	page, err = f.svc.Library(context.Background(), LibraryParams{IDs: "movie:693134, tv:209867,movie:693134,movie:603", Page: 3, PageSize: 1, Query: "d"})
	q = f.store.lastQuery
	if err != nil || page.Page != 1 || page.PageSize != 3 || q.Offset != 0 || q.Limit != 3 || q.Search != "d" ||
		!reflect.DeepEqual(q.IDs, &library.TMDBIDs{Movies: []int{693134, 603}, Shows: []int{209867}}) || q.OnlyTMDB != nil {
		t.Errorf("ids = page %d size %d, query %+v, %v", page.Page, page.PageSize, q, err)
	}

	for _, p := range []LibraryParams{{MediaType: "music"}, {Status: "done"}, {Sort: "rating"}, {Page: -1}, {PageSize: -5},
		{IDs: "movie"}, {IDs: "show:1"}, {IDs: "movie:-1"}, {IDs: "movie:1,"}, {IDs: strings.Repeat("tv:1,", 200) + "tv:2"}} {
		if _, err := f.svc.Library(context.Background(), p); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: err = %v, want ErrInvalidQuery", p, err)
		}
	}
}

func TestParseLibraryIDs(t *testing.T) {
	tests := []struct {
		raw    string
		movies []int
		shows  []int
		n      int
		ok     bool
	}{
		{"movie:603", []int{603}, nil, 1, true},
		{"movie:603,tv:1399", []int{603}, []int{1399}, 2, true},
		{" tv:1399 , tv:1399,movie:1", []int{1}, []int{1399}, 2, true}, // spaces trimmed, repeats once
		{strings.TrimSuffix(strings.Repeat("movie:7,", MaxLibraryIDs), ","), []int{7}, nil, 1, true},
		{strings.Repeat("movie:7,", MaxLibraryIDs) + "movie:8", nil, nil, 0, false},
		{"", nil, nil, 0, false},
		{"movie:", nil, nil, 0, false},
		{"tv:1x", nil, nil, 0, false},
		{"TV:1", nil, nil, 0, false},
		{"movie:0", nil, nil, 0, false},
		{"movie:1:2", nil, nil, 0, false},
	}
	for _, tt := range tests {
		ids, n, err := ParseLibraryIDs(tt.raw)
		if !tt.ok {
			if !errors.Is(err, ErrInvalidQuery) {
				t.Errorf("%q: err = %v, want ErrInvalidQuery", tt.raw, err)
			}
			continue
		}
		if err != nil || n != tt.n || !reflect.DeepEqual(ids.Movies, tt.movies) || !reflect.DeepEqual(ids.Shows, tt.shows) {
			t.Errorf("%q = %+v (%d), %v", tt.raw, ids, n, err)
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

// fakeReasons answers member 2's picks' reason for one title; err fails
// every lookup.
type fakeReasons struct {
	err   error
	asked []string
}

func (f *fakeReasons) RecommendationReason(_ context.Context, memberID int64, mediaType string, tmdbID int) (*string, error) {
	f.asked = append(f.asked, fmt.Sprintf("%d %s", memberID, mediaType))
	if f.err != nil || memberID != 2 || tmdbID != duneTMDB {
		return nil, f.err
	}
	reason := "Big, quiet science fiction like Arrival."
	return &reason, nil
}

// recommendation_reason comes from the viewer's picks once they set it
// (#157, #245); null before, without a viewer, for a title they didn't
// pick, and when the lookup fails.
func TestRecommendationReason(t *testing.T) {
	f := newFixture()
	ctx := member.WithID(context.Background(), 2)
	if movie, _ := f.svc.MovieTitle(ctx, duneTMDB); movie.RecommendationReason != nil {
		t.Errorf("before SetReasons = %v", *movie.RecommendationReason)
	}
	reasons := &fakeReasons{}
	f.svc.SetReasons(reasons)
	movie, err := f.svc.MovieTitle(ctx, duneTMDB)
	if err != nil || movie.RecommendationReason == nil || *movie.RecommendationReason != "Big, quiet science fiction like Arrival." {
		t.Errorf("reason = %v, %v", movie.RecommendationReason, err)
	}
	show, err := f.svc.ShowTitle(ctx, frierenTMDB)
	if err != nil || show.RecommendationReason != nil {
		t.Errorf("show reason = %v, %v", show.RecommendationReason, err)
	}
	if movie, err := f.svc.MovieTitle(member.WithID(context.Background(), 1), duneTMDB); err != nil || movie.RecommendationReason != nil {
		t.Errorf("another member's reason = %v, %v; want null", movie.RecommendationReason, err)
	}
	if movie, err := f.svc.MovieTitle(context.Background(), duneTMDB); err != nil || movie.RecommendationReason != nil {
		t.Errorf("no viewer's reason = %v, %v; want null", movie.RecommendationReason, err)
	}
	if !reflect.DeepEqual(reasons.asked, []string{"2 movie", "2 tv", "1 movie"}) {
		t.Errorf("asked = %v", reasons.asked)
	}
	reasons.err = errors.New("db down")
	if movie, err := f.svc.MovieTitle(ctx, duneTMDB); err != nil || movie.RecommendationReason != nil {
		t.Errorf("a failed lookup = %v, %v; want the page with a null reason", movie, err)
	}
}

// keep is the web's Keep or null (contract/downloads.ts, the titles mock):
// null, not left out, when nothing holds the title, season or episode;
// expires_at null without a maximum; the state, reason and scope the
// contract's.
func TestKeepJSON(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	movie, err := f.svc.MovieTitle(ctx, duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	show, err := f.svc.ShowTitle(ctx, frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	season, err := f.svc.Season(ctx, frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	object := func(v any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for name, v := range map[string]any{"movie": movie.Library, "E01": season.Episodes[0], "E29": season.Episodes[28]} {
		if k, ok := object(v)["keep"]; !ok || k != nil {
			t.Errorf("%s keep = %v (present %v), want null", name, k, ok)
		}
	}

	// The web's DownloadState, DownloadReason and DownloadScope.
	states := []string{"queued", "downloading", "waiting_for_stream", "waiting_for_window", "paused", "stalled", "disk_full",
		"storage_full", "done", "no_file", "unavailable"}
	for name, v := range map[string]any{"show": show.Library, "season": season, "E27": season.Episodes[26], "E28": season.Episodes[27]} {
		k, ok := object(v)["keep"].(map[string]any)
		if !ok {
			t.Fatalf("%s keep = %v, want an object", name, object(v)["keep"])
		}
		if !slices.Contains(states, k["state"].(string)) || !slices.Contains([]string{"kept", "smart"}, k["reason"].(string)) ||
			!slices.Contains([]string{"movie", "show", "season", "episode"}, k["scope"].(string)) {
			t.Errorf("%s keep = %v", name, k)
		}
		if _, ok := k["pct"].(float64); !ok {
			t.Errorf("%s keep pct = %v, want a number", name, k["pct"])
		}
		if _, ok := k["until_watched"].(bool); !ok {
			t.Errorf("%s keep until_watched = %v, want a boolean", name, k["until_watched"])
		}
	}
	if k := object(show.Library)["keep"].(map[string]any); k["expires_at"] != nil {
		t.Errorf("show keep expires_at = %v, want null (no maximum)", k["expires_at"])
	}
	if k := object(season)["keep"].(map[string]any); k["expires_at"] != "2026-11-13T12:00:00Z" {
		t.Errorf("season keep expires_at = %v", k["expires_at"])
	}
	// Every state the downloads have is one the web knows.
	for _, s := range []downloads.State{downloads.StateUnavailable, downloads.StateNoFile, downloads.StateDone, downloads.StatePaused,
		downloads.StateDiskFull, downloads.StateStorageFull, downloads.StateWaitingForWindow, downloads.StateWaitingForStream,
		downloads.StateStalled, downloads.StateDownloading, downloads.StateQueued} {
		if !slices.Contains(states, string(s)) {
			t.Errorf("state %q isn't the web's", s)
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
		"needs_review", "review_job_id", "episodes_total", "episodes_ready", "missing", "torrent", "stream", "keep"}

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
		{"SeasonDetail", season, []string{"season_number", "name", "episodes", "ranges", "keep"}},
		{"EpisodeSummary", season.Episodes[0], []string{"id", "season_number", "episode_number", "absolute_number", "name", "overview",
			"air_date", "still_path", "runtime", "episode_type", "state", "pct", "webdav_path", "source", "stream", "keep"}},
		// web/src/lib/api/contract/downloads.ts Keep
		{"Keep", season.Keep, []string{"download_id", "reason", "scope", "state", "pct", "until_watched", "expires_at"}},
		{"Keep (show)", show.Library.Keep, []string{"download_id", "reason", "scope", "state", "pct", "until_watched", "expires_at"}},
		{"TitleStream", TitleStream{}, []string{"provider", "title", "url", "translator", "quality", "size_bytes", "added_at"}},
		{"EpisodeStream", EpisodeStream{}, []string{"translator", "quality"}},
		{"EpisodeRange", season.Ranges[0], []string{"label", "kind", "from", "to", "absolute_from", "absolute_to", "missing"}},
		{"LibraryPage", page, []string{"items", "total", "page", "page_size", "counts", "summary"}},
		{"LibraryCounts", page.Counts, []string{"all", "movie", "tv", "watching", "ready", "incomplete", "needs_review"}},
		{"LibrarySummary", page.Summary, []string{"total", "size_bytes", "ready"}},
		{"LibraryItem", page.Items[0], []string{"id", "media_type", "tmdb_id", "title", "year", "poster_path", "availability_stage",
			"episodes_total", "episodes_ready", "episodes_watched", "needs_review", "size_bytes", "added_at", "last_watched_at",
			"runtime", "resolution", "webdav_path", "webdav_url", "review_job_id", "download", "missing", "streaming"}},
		{"LibraryDownload", LibraryDownload{}, []string{"pct", "info_hash"}},
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
