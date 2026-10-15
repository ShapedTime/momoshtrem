package library

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// testStream is a translation of HDRezka post 646 in one quality: episode
// 0 is the movie.
func testStream(translator int, quality string, season, episode int) *StreamRef {
	return &StreamRef{Provider: "hdrezka", Path: "/series/drama/646-breaking-bad-2008.html", PostID: 646,
		Title: "Breaking Bad", TranslatorID: translator, TranslatorName: fmt.Sprintf("Studio %d", translator),
		Season: season, Episode: episode, Quality: quality}
}

func TestStreamRef(t *testing.T) {
	interstellar := StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		Title: "Interstellar", TranslatorID: 56, TranslatorName: "Дубляж", Quality: "1080p"}
	tests := []struct {
		name string
		ref  StreamRef
		path string
	}{
		{"movie", interstellar, "hdrezka/2259/t56/movie/1080p.mp4"},
		{"episode", *testStream(1, "720p", 1, 5), "hdrezka/646/t1/s01e05/720p.mp4"},
		{"three-digit episode", *testStream(1, "720p", 1, 1100), "hdrezka/646/t1/s01e1100/720p.mp4"},
		{"special", *testStream(1, "720p", 0, 3), "hdrezka/646/t1/s00e03/720p.mp4"},
		{"flags", func() StreamRef { r := interstellar; r.Camrip, r.Ads, r.Director = true, true, true; return r }(),
			"hdrezka/2259/t56-c-a-d/movie/1080p.mp4"},
		{"director's cut", func() StreamRef { r := interstellar; r.Director = true; return r }(),
			"hdrezka/2259/t56-d/movie/1080p.mp4"},
		{"premium quality label", func() StreamRef { r := interstellar; r.Quality = "1080p Ultra"; return r }(),
			"hdrezka/2259/t56/movie/1080p-ultra.mp4"},
		{"no quality", func() StreamRef { r := interstellar; r.Quality = " "; return r }(),
			"hdrezka/2259/t56/movie/unknown.mp4"},
	}
	for _, tt := range tests {
		if got := tt.ref.FilePath(); got != tt.path {
			t.Errorf("%s: FilePath = %q, want %q", tt.name, got, tt.path)
		}
	}

	if got := testStream(56, "1080p", 1, 1).Label(); got != "HDRezka · Studio 56 · 1080p" {
		t.Errorf("Label = %q", got)
	}
	if got := (StreamRef{Quality: "720p"}).Label(); got != "HDRezka · 720p" {
		t.Errorf("Label without a translator name = %q", got)
	}

	for _, tt := range []struct {
		a                  TorrentAssignment
		torrent, groupable bool
	}{
		{TorrentAssignment{}, true, false},
		{TorrentAssignment{MatchSource: SourceRules}, true, true},
		{TorrentAssignment{Kind: AssignmentKindTorrent, MatchSource: SourceManual}, true, true},
		{TorrentAssignment{Kind: AssignmentKindHDRezka, Stream: &interstellar}, false, false},
		// The CHECK forbids it; the VFS must not group one anyway.
		{TorrentAssignment{Kind: AssignmentKindHDRezka, MatchSource: SourceManual}, false, false},
	} {
		if tt.a.IsTorrent() != tt.torrent || tt.a.Groupable() != tt.groupable {
			t.Errorf("%+v: IsTorrent %v Groupable %v", tt.a, tt.a.IsTorrent(), tt.a.Groupable())
		}
	}
}

// 008 on a database shaped by 001–007: torrent rows keep everything and
// read as torrents, running it twice is harmless, and the CHECK keeps each
// kind's columns apart. It runs in a private schema on one connection.
func TestMigration008(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := fmt.Sprintf("mig008_%d", time.Now().UnixNano())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}
	exec(`CREATE SCHEMA ` + schema)
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `RESET search_path`)
		_, _ = conn.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
	}()
	exec(`SET search_path TO ` + schema)

	migration := func(name string) string {
		t.Helper()
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, name := range []string{"001_core_schema.sql", "002_app_settings.sql", "003_metadata.sql",
		"004_assignment_jobs.sql", "007_subtitles.sql"} {
		exec(migration(name))
	}

	const hash = "abcdef0123456789abcdef0123456789abcdef01"
	exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, match_source, confidence, is_active) VALUES
		('movie', 1, $1, 'magnet:?xt=urn:btih:abc', 'Film/film.mkv', 10, NULL, NULL, TRUE),
		('episode', 7, $1, 'magnet:?xt=urn:btih:abc', 'Show/E01.mkv', 20, 'rules', 'high', FALSE)`, hash)

	exec(migration("008_stream_sources.sql"))
	exec(migration("008_stream_sources.sql")) // Idempotent

	rows, err := conn.QueryContext(ctx, `SELECT `+assignmentSelect+` FROM torrent_assignments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s/%d/%s/%s/%s/%v/%v", a.ItemType, a.ItemID, a.Kind, a.InfoHash, a.FilePath, a.MatchSource, a.Stream))
	}
	rows.Close()
	want := []string{
		"movie/1/torrent/" + hash + "/Film/film.mkv//<nil>",
		"episode/7/torrent/" + hash + "/Show/E01.mkv/rules/<nil>",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows after 008 =\n%v\nwant\n%v", got, want)
	}

	insert := func(kind, hash, magnet, stream, matchSource, confidence string, item int64, path string) error {
		_, err := conn.ExecContext(ctx, `INSERT INTO torrent_assignments
			(item_type, item_id, kind, info_hash, magnet_uri, stream_ref, file_path, file_size, match_source, confidence)
			VALUES ('episode', $1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, '')::jsonb, $6, 1, NULLIF($7, ''), NULLIF($8, ''))`,
			item, kind, hash, magnet, stream, path, matchSource, confidence)
		return err
	}
	const ref = `{"provider":"hdrezka","post_id":646,"translator_id":1,"season":1,"episode":1,"quality":"720p"}`
	refused := []struct {
		name                                           string
		kind, hash, magnet, stream, source, confidence string
	}{
		{"an HDRezka row with a hash", "hdrezka", hash, "", ref, "", ""},
		{"an HDRezka row with a magnet", "hdrezka", "", "magnet:?", ref, "", ""},
		{"an HDRezka row without a stream", "hdrezka", "", "", "", "", ""},
		{"an HDRezka row with a match source", "hdrezka", "", "", ref, "manual", ""},
		{"an HDRezka row with a confidence", "hdrezka", "", "", ref, "", "high"},
		{"a torrent row without a hash", "torrent", "", "magnet:?", "", "", ""},
		{"a torrent row without a magnet", "torrent", hash, "", "", "", ""},
		{"a torrent row with a stream", "torrent", hash, "magnet:?", ref, "", ""},
		{"an unknown kind", "http", "", "", ref, "", ""},
	}
	for _, r := range refused {
		err := insert(r.kind, r.hash, r.magnet, r.stream, r.source, r.confidence, 8, "hdrezka/646/t1/s01e01/720p.mp4")
		if err == nil || !strings.Contains(err.Error(), "torrent_assignments_kind_check") {
			t.Errorf("%s: %v, want the kind check", r.name, err)
		}
	}

	// One row per item and stream: the same stream on another episode is fine.
	if err := insert("hdrezka", "", "", ref, "", "", 8, "hdrezka/646/t1/s01e01/720p.mp4"); err != nil {
		t.Fatal(err)
	}
	if err := insert("hdrezka", "", "", ref, "", "", 9, "hdrezka/646/t1/s01e01/720p.mp4"); err != nil {
		t.Errorf("the stream on another episode: %v", err)
	}
	err = insert("hdrezka", "", "", ref, "", "", 8, "hdrezka/646/t1/s01e01/720p.mp4")
	if err == nil || !strings.Contains(err.Error(), "torrent_assignments_hdrezka_key") {
		t.Errorf("the same stream twice on one episode: %v", err)
	}
}

// itemRows returns every row of an item, active or not, oldest first.
func itemRows(t *testing.T, db *DB, itemType ItemType, itemID int64) []*TorrentAssignment {
	t.Helper()
	rows, err := db.Query(`SELECT `+assignmentSelect+` FROM torrent_assignments
		WHERE item_type = $1 AND item_id = $2 ORDER BY id`, itemType, itemID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []*TorrentAssignment
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

// activeRow is an item's one active row: nil without one, a failure with
// several.
func activeRow(t *testing.T, db *DB, itemType ItemType, itemID int64) *TorrentAssignment {
	t.Helper()
	var active []*TorrentAssignment
	for _, a := range itemRows(t, db, itemType, itemID) {
		if a.IsActive {
			active = append(active, a)
		}
	}
	if len(active) > 1 {
		t.Fatalf("%s %d has %d active rows", itemType, itemID, len(active))
	}
	if len(active) == 0 {
		return nil
	}
	return active[0]
}

// seedSubtitle stores a subtitle of the given source for an item.
func seedSubtitle(t *testing.T, db *DB, itemType ItemType, itemID int64, source, lang string) {
	t.Helper()
	var hash any
	if source == "torrent" {
		hash = strings.Repeat("a", 40)
	}
	if _, err := db.Exec(`INSERT INTO subtitles (item_type, item_id, language_code, language_name, file_path, source, info_hash)
		VALUES ($1, $2, $3, $3, $4, $5, $6)`, itemType, itemID, lang, fmt.Sprintf("%s/%d/%s.srt", source, itemID, lang), source, hash); err != nil {
		t.Fatal(err)
	}
}

// subtitleSources lists an item's subtitles as source/language, sorted.
func subtitleSources(t *testing.T, db *DB, itemType ItemType, itemID int64) []string {
	t.Helper()
	rows, err := db.Query(`SELECT source || '/' || language_code FROM subtitles WHERE item_type = $1 AND item_id = $2`, itemType, itemID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// An HDRezka stream replaces an item's torrent file (its torrent subtitles
// go); the same stream again reactivates its row; another translator or
// quality is another row and drops the old stream's subtitles.
func TestAssignmentRepository_WriteStream(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentRepository(db)
	show, eps := jobShow(t, db, 2)
	e1 := eps[1]
	torrentRow := seedAssignment(t, db, e1, "Pack/E01.mkv", true)
	seedSubtitle(t, db, ItemTypeEpisode, e1, "torrent", "en")
	seedSubtitle(t, db, ItemTypeEpisode, e1, "opensubtitles", "de")

	write := func(ref *StreamRef, size int64) {
		t.Helper()
		err := repo.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID,
			Rows: []TorrentAssignment{{ItemID: e1, Stream: ref, FileSize: size, Resolution: ref.Quality}}})
		if err != nil {
			t.Fatal(err)
		}
	}

	studio56 := testStream(56, "1080p", 1, 1)
	write(studio56, 900)
	a := activeRow(t, db, ItemTypeEpisode, e1)
	if a == nil || a.Kind != AssignmentKindHDRezka || a.InfoHash != "" || a.MagnetURI != "" || a.FilePath != studio56.FilePath() ||
		a.FileSize != 900 || a.Resolution != "1080p" || a.Source != "" || a.MatchSource != "" || a.Confidence != "" ||
		!reflect.DeepEqual(a.Stream, studio56) {
		t.Fatalf("E01 = %+v", a)
	}
	if got := subtitleSources(t, db, ItemTypeEpisode, e1); !reflect.DeepEqual(got, []string{"opensubtitles/de"}) {
		t.Errorf("subtitles = %v, want the torrent's dropped", got)
	}
	if b := activeRow(t, db, ItemTypeEpisode, eps[2]); b != nil {
		t.Errorf("E02 = %+v, want untouched", b)
	}
	first := a.ID

	// The same stream again keeps its subtitles; a new size is taken.
	seedSubtitle(t, db, ItemTypeEpisode, e1, "hdrezka", "en")
	write(studio56, 950)
	if a := activeRow(t, db, ItemTypeEpisode, e1); a.ID != first || a.FileSize != 950 {
		t.Errorf("E01 = %+v, want row %d updated", a, first)
	}
	if got := subtitleSources(t, db, ItemTypeEpisode, e1); !reflect.DeepEqual(got, []string{"hdrezka/en", "opensubtitles/de"}) {
		t.Errorf("subtitles = %v", got)
	}

	// Back to the torrent and to the stream again: both rows come back.
	err := repo.WriteOverride(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID,
		InfoHash: "abcdef0123456789abcdef0123456789abcdef01", MagnetURI: "magnet:?xt=urn:btih:abc",
		Rows: []TorrentAssignment{{ItemID: e1, FilePath: "Pack/E01.mkv", FileSize: 1000, MatchSource: SourceManual}}})
	if err != nil {
		t.Fatal(err)
	}
	if a := activeRow(t, db, ItemTypeEpisode, e1); a.ID != torrentRow {
		t.Errorf("E01 = %+v, want the torrent row %d reactivated", a, torrentRow)
	}
	write(studio56, 950)
	if a := activeRow(t, db, ItemTypeEpisode, e1); a.ID != first {
		t.Errorf("E01 = %+v, want the stream's row %d reactivated", a, first)
	}

	// Another translator, then another quality: new rows each time.
	seedSubtitle(t, db, ItemTypeEpisode, e1, "hdrezka", "ru")
	studio1 := testStream(1, "1080p", 1, 1)
	write(studio1, 800)
	studio1Lower := testStream(1, "720p", 1, 1)
	write(studio1Lower, 400)
	a = activeRow(t, db, ItemTypeEpisode, e1)
	if a.FilePath != "hdrezka/646/t1/s01e01/720p.mp4" || a.Resolution != "720p" || !reflect.DeepEqual(a.Stream, studio1Lower) {
		t.Errorf("E01 = %+v", a)
	}
	if n := len(itemRows(t, db, ItemTypeEpisode, e1)); n != 4 {
		t.Errorf("E01 has %d rows, want 4: the torrent's and three streams'", n)
	}
	if got := subtitleSources(t, db, ItemTypeEpisode, e1); !reflect.DeepEqual(got, []string{"opensubtitles/de"}) {
		t.Errorf("subtitles = %v, want the old stream's dropped", got)
	}

	// A movie.
	movie := seedMovie(t, db, "Interstellar", 2014)
	ref := &StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		TranslatorID: 56, Quality: "1080p"}
	if err := repo.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeMovie, MovieID: movie.ID,
		Rows: []TorrentAssignment{{ItemID: movie.ID, Stream: ref, FileSize: 5}}}); err != nil {
		t.Fatal(err)
	}
	if a := activeRow(t, db, ItemTypeMovie, movie.ID); a == nil || a.FilePath != "hdrezka/2259/t56/movie/1080p.mp4" || a.Resolution != "" {
		t.Errorf("movie = %+v", a)
	}

	// What can't be written.
	err = repo.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID,
		Rows: []TorrentAssignment{{ItemID: eps[2], FilePath: "x.mp4", FileSize: 1}}})
	if err == nil || activeRow(t, db, ItemTypeEpisode, eps[2]) != nil {
		t.Errorf("a row without a stream: %v", err)
	}
	err = repo.WriteOverride(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID, Kind: "http",
		Rows: []TorrentAssignment{{ItemID: eps[2], FilePath: "x.mp4", FileSize: 1}}})
	if err == nil {
		t.Error("an unknown kind was written")
	}
	if _, err := db.Exec(`DELETE FROM movies WHERE id = $1`, movie.ID); err != nil {
		t.Fatal(err)
	}
	err = repo.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeMovie, MovieID: movie.ID,
		Rows: []TorrentAssignment{{ItemID: movie.ID, Stream: ref}}})
	if !errors.Is(err, ErrMovieNotFound) {
		t.Errorf("a deleted movie: %v", err)
	}
}

// A torrent replaces HDRezka streams through Apply and through a manual
// override: the stream's row is deactivated and its subtitles go. Its NULL
// hash compared as unknown, so the old "NOT (hash = $4 AND path = …)" left
// it active only by luck of the paths differing.
func TestJobTx_WriteAssignmentsReplacesStreams(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	jobs := NewAssignmentJobRepository(db)
	repo := NewAssignmentRepository(db)
	show, eps := jobShow(t, db, 3)
	hash := strings.Repeat("f", 40)
	job := newShowJob(show, hash)
	if _, err := jobs.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 3; n++ {
		if err := repo.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID,
			Rows: []TorrentAssignment{{ItemID: eps[n], Stream: testStream(56, "1080p", 1, n), FileSize: 9}}}); err != nil {
			t.Fatal(err)
		}
		seedSubtitle(t, db, ItemTypeEpisode, eps[n], "hdrezka", "en")
	}

	// Apply maps E01 and E02; E03 keeps its stream.
	err := jobs.Locked(ctx, job.ID, func(tx *JobTx) error {
		return tx.WriteAssignments(AssignmentWrite{ItemType: ItemTypeEpisode, InfoHash: hash, MagnetURI: job.MagnetURI,
			ShowID: show.ID, Rows: []TorrentAssignment{
				{ItemID: eps[1], FilePath: "P/E01.mkv", FileSize: 7, Confidence: "high", MatchSource: SourceRules},
				{ItemID: eps[2], FilePath: "P/E02.mkv", FileSize: 7, Confidence: "high", MatchSource: SourceRules},
			}})
	})
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range map[int]string{1: "torrent P/E01.mkv", 2: "torrent P/E02.mkv", 3: "hdrezka hdrezka/646/t56/s01e03/1080p.mp4"} {
		if a := activeRow(t, db, ItemTypeEpisode, eps[n]); a == nil || string(a.Kind)+" "+a.FilePath != want {
			t.Errorf("E%02d = %+v, want %s", n, a, want)
		}
	}
	for n, want := range map[int][]string{1: {}, 2: {}, 3: {"hdrezka/en"}} {
		if got := subtitleSources(t, db, ItemTypeEpisode, eps[n]); !reflect.DeepEqual(got, want) {
			t.Errorf("E%02d subtitles = %v, want %v", n, got, want)
		}
	}

	// A manual override of E03.
	err = repo.WriteOverride(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID, InfoHash: hash, MagnetURI: job.MagnetURI,
		Rows: []TorrentAssignment{{ItemID: eps[3], FilePath: "P/E02.mkv", FileSize: 7, MatchSource: SourceManual}}})
	if err != nil {
		t.Fatal(err)
	}
	if a := activeRow(t, db, ItemTypeEpisode, eps[3]); a == nil || !a.IsTorrent() || a.FilePath != "P/E02.mkv" {
		t.Errorf("E03 = %+v", a)
	}
	if got := subtitleSources(t, db, ItemTypeEpisode, eps[3]); len(got) != 0 {
		t.Errorf("E03 subtitles = %v", got)
	}

	// And a movie, through Apply.
	movie := seedMovie(t, db, "Film", 2014)
	if err := repo.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeMovie, MovieID: movie.ID,
		Rows: []TorrentAssignment{{ItemID: movie.ID, Stream: testStream(56, "1080p", 0, 0), FileSize: 9}}}); err != nil {
		t.Fatal(err)
	}
	err = jobs.Locked(ctx, job.ID, func(tx *JobTx) error {
		return tx.WriteAssignments(AssignmentWrite{ItemType: ItemTypeMovie, InfoHash: hash, MagnetURI: job.MagnetURI,
			MovieID: movie.ID, Rows: []TorrentAssignment{{ItemID: movie.ID, FilePath: "P/Film.mkv", FileSize: 9}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if a := activeRow(t, db, ItemTypeMovie, movie.ID); a == nil || !a.IsTorrent() || a.FilePath != "P/Film.mkv" {
		t.Errorf("movie = %+v", a)
	}
}

// A torrent's Apply is authoritative for its own rows only: on a show
// whose episodes mix sources, the HDRezka episodes it doesn't map keep
// their streams while its own dropped rows go.
func TestJobTx_WriteAssignmentsMixedShow(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	jobs := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 3)
	hash := strings.Repeat("c", 40)
	job := newShowJob(show, hash)
	if _, err := jobs.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := NewAssignmentRepository(db).WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID,
		Rows: []TorrentAssignment{{ItemID: eps[1], Stream: testStream(56, "1080p", 1, 1), FileSize: 9}}}); err != nil {
		t.Fatal(err)
	}
	apply := func(episodes ...int) {
		t.Helper()
		var rows []TorrentAssignment
		for _, n := range episodes {
			rows = append(rows, TorrentAssignment{ItemID: eps[n], FilePath: fmt.Sprintf("P/E%02d.mkv", n), FileSize: 7,
				Confidence: "high", MatchSource: SourceRules})
		}
		err := jobs.Locked(ctx, job.ID, func(tx *JobTx) error {
			return tx.WriteAssignments(AssignmentWrite{ItemType: ItemTypeEpisode, InfoHash: hash, MagnetURI: job.MagnetURI,
				ShowID: show.ID, Rows: rows})
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	apply(2, 3)
	apply(2) // E03 dropped
	if a := activeRow(t, db, ItemTypeEpisode, eps[1]); a == nil || a.Kind != AssignmentKindHDRezka {
		t.Errorf("E01 = %+v, want its stream", a)
	}
	if a := activeRow(t, db, ItemTypeEpisode, eps[2]); a == nil || a.FilePath != "P/E02.mkv" {
		t.Errorf("E02 = %+v", a)
	}
	if a := activeRow(t, db, ItemTypeEpisode, eps[3]); a != nil {
		t.Errorf("E03 = %+v, want the torrent's dropped row gone", a)
	}
}

// Every reader takes an HDRezka row (no hash, no magnet) in its stride:
// one used to fail the VFS's scans and empty the Movies folder.
func TestReaders_StreamRows(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	assignments := NewAssignmentRepository(db)
	shows := NewShowRepository(db)
	catalog := NewCatalogRepository(db)
	const hash = "abcdef0123456789abcdef0123456789abcdef01"

	torrentMovie := seedMovie(t, db, "A Torrent Film", 2010)
	if _, err := db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size)
		VALUES ('movie', $1, $2, 'magnet:?xt=urn:btih:abc', 'Film/film.mkv', 10)`, torrentMovie.ID, hash); err != nil {
		t.Fatal(err)
	}
	streamMovie := seedMovie(t, db, "Interstellar", 2014)
	movieRef := testStream(56, "1080p", 0, 0)
	if err := assignments.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeMovie, MovieID: streamMovie.ID,
		Rows: []TorrentAssignment{{ItemID: streamMovie.ID, Stream: movieRef, FileSize: 20, Resolution: "1080p"}}}); err != nil {
		t.Fatal(err)
	}
	show, eps := jobShow(t, db, 2)
	seedAssignment(t, db, eps[1], "Pack/E01.mkv", true)
	episodeRef := testStream(56, "1080p", 1, 2)
	if err := assignments.WriteStream(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID,
		Rows: []TorrentAssignment{{ItemID: eps[2], Stream: episodeRef, FileSize: 30}}}); err != nil {
		t.Fatal(err)
	}

	isStream := func(name string, a *TorrentAssignment, ref *StreamRef) {
		t.Helper()
		if a == nil || a.Kind != AssignmentKindHDRezka || a.IsTorrent() || a.InfoHash != "" || a.MagnetURI != "" ||
			a.FilePath != ref.FilePath() || !reflect.DeepEqual(a.Stream, ref) || !a.IsActive {
			t.Errorf("%s: %+v", name, a)
		}
	}
	isTorrent := func(name string, a *TorrentAssignment) {
		t.Helper()
		if a == nil || a.Kind != AssignmentKindTorrent || a.InfoHash != hash || a.Stream != nil {
			t.Errorf("%s: %+v", name, a)
		}
	}

	// AssignmentRepository (scanAssignment).
	a, err := assignments.GetActiveForItem(ItemTypeMovie, streamMovie.ID)
	if err != nil {
		t.Fatal(err)
	}
	isStream("GetActiveForItem", a, movieRef)
	byID, err := assignments.GetByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	isStream("GetByID", byID, movieRef)
	if hashes, err := assignments.ListDistinctTorrents(); err != nil || !reflect.DeepEqual(hashes, []string{hash}) {
		t.Errorf("ListDistinctTorrents = %q, %v", hashes, err)
	}

	// The VFS tree's readers.
	movies, err := NewMovieRepository(db).ListWithAssignments()
	if err != nil || len(movies) != 2 {
		t.Fatalf("ListWithAssignments = %d movies, %v; want both", len(movies), err)
	}
	isStream("ListWithAssignments", movies[1].Assignment, movieRef)
	isTorrent("ListWithAssignments torrent", movies[0].Assignment)
	withShows, err := shows.GetShowsWithAssignedEpisodes()
	if err != nil || len(withShows) != 1 || len(withShows[0].Seasons) != 1 || len(withShows[0].Seasons[0].Episodes) != 2 {
		t.Fatalf("GetShowsWithAssignedEpisodes = %+v, %v", withShows, err)
	}
	isTorrent("GetEpisodesWithAssignments E01", withShows[0].Seasons[0].Episodes[0].Assignment)
	isStream("GetEpisodesWithAssignments E02", withShows[0].Seasons[0].Episodes[1].Assignment, episodeRef)

	// The title pages', the API's and the jobs' readers.
	listed, err := NewMovieRepository(db).ListWithActiveAssignment(ctx)
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListWithActiveAssignment = %d, %v", len(listed), err)
	}
	isStream("ListWithActiveAssignment", listed[1].Assignment, movieRef)
	full, err := shows.GetWithSeasonsAndEpisodesContext(ctx, show.ID)
	if err != nil || len(full.Seasons) != 1 {
		t.Fatalf("GetWithSeasonsAndEpisodes = %+v, %v", full, err)
	}
	isStream("GetWithSeasonsAndEpisodes", full.Seasons[0].Episodes[1].Assignment, episodeRef)
	m, err := catalog.MovieByTMDBID(ctx, streamMovie.TMDBID)
	if err != nil {
		t.Fatal(err)
	}
	isStream("MovieByTMDBID", m.Assignment, movieRef)
	seasons, err := catalog.ShowEpisodes(ctx, show.ID)
	if err != nil || len(seasons) != 1 {
		t.Fatalf("ShowEpisodes = %+v, %v", seasons, err)
	}
	isStream("ShowEpisodes", seasons[0].Episodes[1].Assignment, episodeRef)
	view, err := catalog.SeasonByTMDBID(ctx, show.TMDBID, 1)
	if err != nil || view.Season == nil {
		t.Fatalf("SeasonByTMDBID = %+v, %v", view, err)
	}
	isStream("SeasonByTMDBID", view.Season.Episodes[1].Assignment, episodeRef)
	refs, err := NewAssignmentJobRepository(db).EpisodeRefs(ctx, show.ID, []int64{eps[1], eps[2]})
	if err != nil {
		t.Fatal(err)
	}
	isTorrent("EpisodeRefs E01", refs[eps[1]].Active)
	isStream("EpisodeRefs E02", refs[eps[2]].Active, episodeRef)

	// The episode picker names each file's source.
	picker, err := shows.EpisodeList(ctx, show.ID)
	if err != nil || len(picker) != 1 {
		t.Fatalf("EpisodeList = %+v, %v", picker, err)
	}
	e1, e2 := picker[0].Episodes[0], picker[0].Episodes[1]
	if !e1.HasFile || e1.Source != AssignmentKindTorrent || e1.Stream != nil {
		t.Errorf("picker E01 = %+v", e1)
	}
	if !e2.HasFile || e2.Source != AssignmentKindHDRezka || !reflect.DeepEqual(e2.Stream, episodeRef) {
		t.Errorf("picker E02 = %+v", e2)
	}
}
