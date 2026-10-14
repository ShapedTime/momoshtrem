package library

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

var tmdbIDSeq atomic.Int64

// uniqueTMDBID returns a TMDB ID no other test run uses, so reruns against
// the same database don't collide.
func uniqueTMDBID() int {
	return 1_000_000_000 + int((time.Now().UnixNano()/1000+tmdbIDSeq.Add(1))%1_000_000_000)
}

// seedShow inserts a bare show (as the old code did) and deletes it after
// the test.
func seedShow(t *testing.T, db *DB, title string, year int) *Show {
	t.Helper()
	show := &Show{TMDBID: uniqueTMDBID(), Title: title, Year: year}
	if err := NewShowRepository(db).Create(show); err != nil {
		t.Fatalf("seed show: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM shows WHERE id = $1`, show.ID) })
	return show
}

func seedMovie(t *testing.T, db *DB, title string, year int) *Movie {
	t.Helper()
	movie := &Movie{TMDBID: uniqueTMDBID(), Title: title, Year: year}
	if err := NewMovieRepository(db).Create(movie); err != nil {
		t.Fatalf("seed movie: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM movies WHERE id = $1`, movie.ID) })
	return movie
}

// seedAssignment assigns a file to an episode.
func seedAssignment(t *testing.T, db *DB, episodeID int64, file string, active bool) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, is_active)
		VALUES ('episode', $1, 'abcdef0123456789abcdef0123456789abcdef01', 'magnet:?xt=urn:btih:abc', $2, 1000, '1080p', $3)
		RETURNING id
	`, episodeID, file, active).Scan(&id)
	if err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM torrent_assignments WHERE id = $1`, id) })
	return id
}

func episodeNames(t *testing.T, db *DB, seasonID int64) map[int]string {
	t.Helper()
	rows, err := db.Query(`SELECT episode_number, COALESCE(name, '') FROM episodes WHERE season_id = $1`, seasonID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	names := map[int]string{}
	for rows.Next() {
		var n int
		var name string
		if err := rows.Scan(&n, &name); err != nil {
			t.Fatal(err)
		}
		names[n] = name
	}
	return names
}

func TestMetadataRepository_UpdateMovieMetadataKeepsTitle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewMetadataRepository(db)
	movie := seedMovie(t, db, "Old Title", 1999)

	err := repo.UpdateMovieMetadata(ctx, movie.ID, MovieMetadata{
		IMDbID: "tt0001", PosterPath: "/p.jpg", BackdropPath: "/b.jpg", Runtime: 120, Genres: []string{"Drama", "Sci-Fi"},
	})
	if err != nil {
		t.Fatalf("UpdateMovieMetadata: %v", err)
	}

	got, err := NewMovieRepository(db).GetByID(movie.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Old Title" || got.Year != 1999 {
		t.Errorf("title/year changed to %q (%d)", got.Title, got.Year)
	}
	if got.IMDbID != "tt0001" || got.Runtime != 120 || !reflect.DeepEqual(got.Genres, []string{"Drama", "Sci-Fi"}) || got.MetadataUpdatedAt == nil {
		t.Errorf("metadata = %+v", got)
	}

	if err := repo.UpdateMovieMetadata(ctx, -1, MovieMetadata{}); !errors.Is(err, ErrMovieNotFound) {
		t.Errorf("missing movie: error = %v, want ErrMovieNotFound", err)
	}
}

func TestMetadataRepository_UpdateShowMetadata(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewMetadataRepository(db)
	show := seedShow(t, db, "Frieren", 2023)

	m := ShowMetadata{
		IMDbID: "tt22248376", TVDBID: 424536, OriginalName: "葬送のフリーレン", OriginalLanguage: "ja",
		OriginCountry: []string{"JP"}, IsAnime: true, PosterPath: "/p.jpg", BackdropPath: "/b.jpg",
		Status: "Returning Series", Genres: []string{"Animation"},
	}
	if err := repo.UpdateShowMetadata(ctx, show.ID, m); err != nil {
		t.Fatalf("UpdateShowMetadata: %v", err)
	}

	got, err := NewShowRepository(db).GetByID(show.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotMeta := ShowMetadata{
		IMDbID: got.IMDbID, TVDBID: got.TVDBID, OriginalName: got.OriginalName, OriginalLanguage: got.OriginalLanguage,
		OriginCountry: got.OriginCountry, IsAnime: got.IsAnime, PosterPath: got.PosterPath, BackdropPath: got.BackdropPath,
		Status: got.Status, Genres: got.Genres,
	}
	if !reflect.DeepEqual(gotMeta, m) {
		t.Errorf("metadata = %+v, want %+v", gotMeta, m)
	}
	if got.Title != "Frieren" || got.MetadataUpdatedAt != nil {
		t.Errorf("title %q, metadata_updated_at %v: want title kept and no stamp yet", got.Title, got.MetadataUpdatedAt)
	}

	if err := repo.MarkShowMetadataUpdated(ctx, show.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := NewShowRepository(db).GetByID(show.ID); got.MetadataUpdatedAt == nil {
		t.Error("MarkShowMetadataUpdated didn't stamp")
	}
}

func TestMetadataRepository_SeasonsAndEpisodes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewMetadataRepository(db)
	show := seedShow(t, db, "Show", 2020)

	season := &Season{ShowID: show.ID, SeasonNumber: 0, Name: "Specials", PosterPath: "/s0.jpg"}
	created, err := repo.UpsertSeason(ctx, season)
	if err != nil || !created || season.ID == 0 {
		t.Fatalf("UpsertSeason = (%v, %v), id %d", created, err, season.ID)
	}
	firstID := season.ID
	season.Name = "Extras"
	created, err = repo.UpsertSeason(ctx, season)
	if err != nil || created || season.ID != firstID {
		t.Fatalf("second UpsertSeason = (%v, %v), id %d, want existing %d", created, err, season.ID, firstID)
	}
	if got, _ := NewShowRepository(db).GetSeason(show.ID, 0); got.Name != "Extras" {
		t.Errorf("season name = %q, want Extras", got.Name)
	}

	eps := []EpisodeMetadata{
		{EpisodeNumber: 1, Name: "One", AirDate: "2020-01-01", TMDBEpisodeID: 11, EpisodeType: "standard", Runtime: 24, StillPath: "/1.jpg", Overview: "o1"},
		{EpisodeNumber: 2, Name: "Two", AirDate: "", TMDBEpisodeID: 12},
		{EpisodeNumber: 3, Name: "Three", AirDate: "2020-01-15", TMDBEpisodeID: 13},
		{EpisodeNumber: 3, Name: "Three again"}, // duplicate: first wins
	}
	res, err := repo.UpsertEpisodes(ctx, season.ID, eps, false)
	if err != nil {
		t.Fatalf("UpsertEpisodes: %v", err)
	}
	if res != (UpsertEpisodesResult{Added: 3}) {
		t.Errorf("first upsert = %+v, want 3 added", res)
	}

	var nullAirDate bool
	if err := db.QueryRow(`SELECT air_date IS NULL FROM episodes WHERE season_id = $1 AND episode_number = 2`, season.ID).Scan(&nullAirDate); err != nil {
		t.Fatal(err)
	}
	if !nullAirDate {
		t.Error("empty air date stored as '', want NULL")
	}

	ep2, _ := NewShowRepository(db).GetEpisode(season.ID, 2)
	seedAssignment(t, db, ep2.ID, "Show.S00E02.mkv", true)

	t.Run("rename without keepNames", func(t *testing.T) {
		renamed := []EpisodeMetadata{
			{EpisodeNumber: 1, Name: "One", AirDate: "2020-01-01", TMDBEpisodeID: 11},
			{EpisodeNumber: 2, Name: "Two (renamed)", TMDBEpisodeID: 12},
			{EpisodeNumber: 3, Name: "Three (renamed)", TMDBEpisodeID: 13},
			{EpisodeNumber: 4, Name: "Four", TMDBEpisodeID: 14},
		}
		res, err := repo.UpsertEpisodes(ctx, season.ID, renamed, false)
		if err != nil {
			t.Fatal(err)
		}
		if res != (UpsertEpisodesResult{Added: 1, Renamed: 2, AssignedRenamed: true}) {
			t.Errorf("result = %+v, want 1 added, 2 renamed, assigned renamed", res)
		}
		want := map[int]string{1: "One", 2: "Two (renamed)", 3: "Three (renamed)", 4: "Four"}
		if got := episodeNames(t, db, season.ID); !reflect.DeepEqual(got, want) {
			t.Errorf("names = %v, want %v", got, want)
		}
	})

	t.Run("keepNames keeps stored names but updates the rest", func(t *testing.T) {
		renamed := []EpisodeMetadata{
			{EpisodeNumber: 2, Name: "Two (TMDB)", TMDBEpisodeID: 12, Overview: "new overview", AirDate: "2020-02-02"},
			{EpisodeNumber: 5, Name: "Five", TMDBEpisodeID: 15},
		}
		res, err := repo.UpsertEpisodes(ctx, season.ID, renamed, true)
		if err != nil {
			t.Fatal(err)
		}
		if res != (UpsertEpisodesResult{Added: 1}) {
			t.Errorf("result = %+v, want 1 added and nothing renamed", res)
		}
		got, _ := NewShowRepository(db).GetEpisode(season.ID, 2)
		if got.Name != "Two (renamed)" || got.Overview != "new overview" || got.AirDate != "2020-02-02" {
			t.Errorf("episode 2 = %+v", got)
		}
		if got, _ := NewShowRepository(db).GetEpisode(season.ID, 5); got.Name != "Five" {
			t.Errorf("new episode name = %q, want Five", got.Name)
		}
	})

	t.Run("renaming an unassigned episode doesn't flag the tree", func(t *testing.T) {
		res, err := repo.UpsertEpisodes(ctx, season.ID, []EpisodeMetadata{{EpisodeNumber: 4, Name: "Four (renamed)"}}, false)
		if err != nil {
			t.Fatal(err)
		}
		if res != (UpsertEpisodesResult{Renamed: 1}) {
			t.Errorf("result = %+v, want 1 renamed, not assigned", res)
		}
	})
}

func TestMetadataRepository_SetAbsoluteNumbers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewMetadataRepository(db)
	shows := NewShowRepository(db)
	show := seedShow(t, db, "Numbered", 2020)

	season := &Season{ShowID: show.ID, SeasonNumber: 1}
	if _, err := repo.UpsertSeason(ctx, season); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertEpisodes(ctx, season.ID, []EpisodeMetadata{{EpisodeNumber: 1}, {EpisodeNumber: 2}, {EpisodeNumber: 3}}, false); err != nil {
		t.Fatal(err)
	}
	eps, _ := shows.GetEpisodes(season.ID)

	changed, err := repo.SetAbsoluteNumbers(ctx, show.ID, []AbsoluteNumber{
		{EpisodeID: eps[0].ID, Number: 101}, {EpisodeID: eps[1].ID, Number: 102}, {EpisodeID: eps[2].ID, Number: 0},
	})
	if err != nil || changed != 2 {
		t.Fatalf("SetAbsoluteNumbers = (%d, %v), want 2 changed", changed, err)
	}

	// A second identical call changes nothing; leaving an episode out clears it.
	if changed, _ := repo.SetAbsoluteNumbers(ctx, show.ID, []AbsoluteNumber{{EpisodeID: eps[0].ID, Number: 101}, {EpisodeID: eps[1].ID, Number: 102}}); changed != 0 {
		t.Errorf("identical call changed %d rows", changed)
	}
	if changed, _ := repo.SetAbsoluteNumbers(ctx, show.ID, []AbsoluteNumber{{EpisodeID: eps[0].ID, Number: 101}}); changed != 1 {
		t.Errorf("dropping one changed %d rows, want 1", changed)
	}

	got, _ := shows.GetEpisodes(season.ID)
	if got[0].AbsoluteNumber == nil || *got[0].AbsoluteNumber != 101 || got[1].AbsoluteNumber != nil || got[2].AbsoluteNumber != nil {
		t.Errorf("numbers = %v, %v, %v", got[0].AbsoluteNumber, got[1].AbsoluteNumber, got[2].AbsoluteNumber)
	}

	// Episodes of other shows are never touched.
	other := seedShow(t, db, "Other", 2020)
	if changed, _ := repo.SetAbsoluteNumbers(ctx, other.ID, []AbsoluteNumber{{EpisodeID: eps[1].ID, Number: 5}}); changed != 0 {
		t.Errorf("numbered another show's episode (%d rows)", changed)
	}
}

func TestMetadataRepository_UpdateShowSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewMetadataRepository(db)
	show := seedShow(t, db, "Settings", 2020)

	if show.EpisodeOrdering != OrderingAired || !show.IncludeSpecials || show.ClassifierMode != ClassifierAuto {
		t.Errorf("defaults = %+v", show.Settings())
	}

	want := ShowSettings{EpisodeOrdering: OrderingEpisodeGroup, EpisodeGroupID: "abc", IncludeSpecials: false, ClassifierMode: ClassifierRulesOnly}
	if err := repo.UpdateShowSettings(ctx, show.ID, want); err != nil {
		t.Fatalf("UpdateShowSettings: %v", err)
	}
	got, _ := NewShowRepository(db).GetByID(show.ID)
	if got.Settings() != want {
		t.Errorf("settings = %+v, want %+v", got.Settings(), want)
	}

	tests := []struct {
		name     string
		settings ShowSettings
		id       int64
		want     error
	}{
		{"unknown ordering", ShowSettings{EpisodeOrdering: "dvd", ClassifierMode: ClassifierAuto}, show.ID, ErrInvalidShowSettings},
		{"unknown classifier", ShowSettings{EpisodeOrdering: OrderingAired, ClassifierMode: "always"}, show.ID, ErrInvalidShowSettings},
		{"missing show", ShowSettings{EpisodeOrdering: OrderingAired, ClassifierMode: ClassifierAuto}, -1, ErrShowNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := repo.UpdateShowSettings(ctx, tt.id, tt.settings); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}

	// The CHECK constraints back the Go validation.
	if _, err := db.Exec(`UPDATE shows SET episode_ordering = 'dvd' WHERE id = $1`, show.ID); err == nil {
		t.Error("episode_ordering CHECK constraint missing")
	}
	if _, err := db.Exec(`UPDATE shows SET classifier_mode = 'sometimes' WHERE id = $1`, show.ID); err == nil {
		t.Error("classifier_mode CHECK constraint missing")
	}
}

func TestMetadataRepository_ListForBackfill(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewMetadataRepository(db)

	pending := seedMovie(t, db, "Pending", 2001)
	done := seedMovie(t, db, "Done", 2002)
	if err := repo.UpdateMovieMetadata(ctx, done.ID, MovieMetadata{}); err != nil {
		t.Fatal(err)
	}
	show := seedShow(t, db, "Pending show", 2003)

	ids := func(items []BackfillItem) map[int64]bool {
		m := map[int64]bool{}
		for _, it := range items {
			m[it.ID] = true
		}
		return m
	}

	movies, err := repo.ListForBackfill(ctx, MediaMovie, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(movies); !got[pending.ID] || got[done.ID] {
		t.Errorf("unforced list = %v; want pending %d without done %d", got, pending.ID, done.ID)
	}

	forced, _ := repo.ListForBackfill(ctx, MediaMovie, true, 0)
	if got := ids(forced); !got[pending.ID] || !got[done.ID] {
		t.Errorf("forced list misses %d or %d", pending.ID, done.ID)
	}

	if limited, _ := repo.ListForBackfill(ctx, MediaMovie, true, 1); len(limited) != 1 {
		t.Errorf("limit 1 returned %d items", len(limited))
	}

	shows, _ := repo.ListForBackfill(ctx, MediaShow, false, 0)
	if got := ids(shows); !got[show.ID] {
		t.Errorf("show %d not listed", show.ID)
	}
	for _, item := range shows {
		if item.ID == show.ID && (item.Title != "Pending show" || item.Year != 2003 || item.TMDBID != show.TMDBID) {
			t.Errorf("item = %+v", item)
		}
	}

	if _, err := repo.ListForBackfill(ctx, "episode", false, 0); err == nil {
		t.Error("unknown kind: want an error")
	}
}

func TestRepositories_CreateIfAbsent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	t.Run("show", func(t *testing.T) {
		shows := NewShowRepository(db)
		show := &Show{
			TMDBID: uniqueTMDBID(), Title: "Anime", Year: 2023, OriginCountry: []string{"JP"}, IsAnime: true,
			Genres: []string{"Animation"}, Status: "Ended", EpisodeOrdering: OrderingAbsolute,
		}
		created, err := shows.CreateIfAbsent(ctx, show)
		if err != nil || !created {
			t.Fatalf("CreateIfAbsent = (%v, %v)", created, err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM shows WHERE id = $1`, show.ID) })
		if show.EpisodeOrdering != OrderingAbsolute || !show.IncludeSpecials || show.ClassifierMode != ClassifierAuto {
			t.Errorf("settings = %+v", show.Settings())
		}

		again := &Show{TMDBID: show.TMDBID, Title: "Different", Year: 1}
		created, err = shows.CreateIfAbsent(ctx, again)
		if err != nil || created {
			t.Fatalf("second CreateIfAbsent = (%v, %v), want existing", created, err)
		}
		if again.ID != show.ID || again.Title != "Anime" || !again.IsAnime || !reflect.DeepEqual(again.OriginCountry, []string{"JP"}) {
			t.Errorf("existing = %+v", again)
		}

		if _, err := shows.CreateIfAbsent(ctx, &Show{TMDBID: uniqueTMDBID(), EpisodeOrdering: "dvd"}); !errors.Is(err, ErrInvalidShowSettings) {
			t.Errorf("invalid ordering: error = %v", err)
		}
	})

	t.Run("movie", func(t *testing.T) {
		movies := NewMovieRepository(db)
		movie := &Movie{TMDBID: uniqueTMDBID(), Title: "Dune", Year: 2024, Runtime: 166, Genres: []string{"Sci-Fi"}}
		created, err := movies.CreateIfAbsent(ctx, movie)
		if err != nil || !created || movie.MetadataUpdatedAt == nil {
			t.Fatalf("CreateIfAbsent = (%v, %v), stamped %v", created, err, movie.MetadataUpdatedAt)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM movies WHERE id = $1`, movie.ID) })

		again := &Movie{TMDBID: movie.TMDBID, Title: "Other"}
		if created, err := movies.CreateIfAbsent(ctx, again); err != nil || created || again.ID != movie.ID || again.Runtime != 166 {
			t.Errorf("second CreateIfAbsent = (%v, %v), %+v", created, err, again)
		}
	})
}

func TestShowRepository_EpisodeAirDateEmptyIsNull(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	show := seedShow(t, db, "Air", 2020)
	season := &Season{ShowID: show.ID, SeasonNumber: 1}
	if _, err := NewMetadataRepository(db).UpsertSeason(ctx, season); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetadataRepository(db).UpsertEpisodes(ctx, season.ID, []EpisodeMetadata{{EpisodeNumber: 1, AirDate: "2020-05-05"}}, false); err != nil {
		t.Fatal(err)
	}

	shows := NewShowRepository(db)
	if err := shows.UpdateEpisodeAirDate(ctx, season.ID, 1, ""); err != nil {
		t.Fatal(err)
	}
	var isNull bool
	_ = db.QueryRow(`SELECT air_date IS NULL FROM episodes WHERE season_id = $1`, season.ID).Scan(&isNull)
	if !isNull {
		t.Error("an empty air date was stored as '', want NULL")
	}
}
