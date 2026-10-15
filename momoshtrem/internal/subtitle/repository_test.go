package subtitle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// openRepo connects to MOMOSHTREM_TEST_DATABASE_URL (migrated) and returns
// a repository plus an item ID no other run uses: subtitles have no foreign
// key to movies or episodes.
func openRepo(t *testing.T) (*subtitle.Repository, *library.DB, int64) {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return subtitle.NewRepository(db.DB), db, time.Now().UnixNano() % 1_000_000_000_000
}

func cleanupItems(t *testing.T, repo *subtitle.Repository, items ...subtitle.Item) {
	t.Cleanup(func() {
		for _, it := range items {
			_ = repo.DeleteByItem(context.Background(), it.Type, it.ID)
		}
	})
}

func downloaded(item subtitle.Item, lang, path, release string) *subtitle.Subtitle {
	return &subtitle.Subtitle{
		ItemType: item.Type, ItemID: item.ID, LanguageCode: lang, LanguageName: subtitle.LanguageName(lang),
		Format: "srt", FilePath: path, FileSize: 10, ReleaseName: release,
	}
}

func fromTorrent(item subtitle.Item, lang, hash, path string) *subtitle.Subtitle {
	return &subtitle.Subtitle{
		ItemType: item.Type, ItemID: item.ID, LanguageCode: lang, LanguageName: subtitle.LanguageName(lang),
		Format: "srt", FilePath: path, FileSize: 20, InfoHash: hash, ReleaseName: "Show.S01.1080p.WEB-DL-GRP",
	}
}

func TestRepository_DownloadedAndTorrentCoexist(t *testing.T) {
	repo, _, id := openRepo(t)
	ctx := context.Background()
	ep := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id}
	cleanupItems(t, repo, ep)

	if err := repo.UpsertDownloaded(ctx, downloaded(ep, "en", "/subs/episode/1/en.srt", "Show.S01E01.1080p.WEB-DL-GRP")); err != nil {
		t.Fatal(err)
	}
	// Before 007 this overwrote the download (same item and language).
	if err := repo.UpsertTorrent(ctx, fromTorrent(ep, "en", hashA, "Show.S01/Subs/E01/English.srt")); err != nil {
		t.Fatal(err)
	}

	subs, err := repo.GetByItem(ctx, ep.Type, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("subtitles = %d, want both sources", len(subs))
	}
	if subs[0].Source != subtitle.SourceOpenSubtitles || subs[1].Source != subtitle.SourceTorrent {
		t.Errorf("order = %s, %s; want downloaded first", subs[0].Source, subs[1].Source)
	}
	if subs[0].ReleaseName != "Show.S01E01.1080p.WEB-DL-GRP" || subs[1].InfoHash != hashA {
		t.Errorf("rows = %+v / %+v", subs[0], subs[1])
	}
}

func TestRepository_UnknownLanguageTorrentFilesCoexist(t *testing.T) {
	repo, _, id := openRepo(t)
	ctx := context.Background()
	ep := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id}
	cleanupItems(t, repo, ep)

	for _, path := range []string{"Show.S01/Subs/E01/2_Track.srt", "Show.S01/Subs/E01/3_Track.srt"} {
		if err := repo.UpsertTorrent(ctx, fromTorrent(ep, "unknown", hashA, path)); err != nil {
			t.Fatal(err)
		}
	}
	// Re-recording the same file updates it instead of adding a row.
	again := fromTorrent(ep, "en", hashA, "Show.S01/Subs/E01/2_Track.srt")
	if err := repo.UpsertTorrent(ctx, again); err != nil {
		t.Fatal(err)
	}

	subs, err := repo.GetByItem(ctx, ep.Type, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("subtitles = %d, want 2", len(subs))
	}
	langs := map[string]string{subs[0].FilePath: subs[0].LanguageCode, subs[1].FilePath: subs[1].LanguageCode}
	if langs["Show.S01/Subs/E01/2_Track.srt"] != "en" || langs["Show.S01/Subs/E01/3_Track.srt"] != "unknown" {
		t.Errorf("languages = %v", langs)
	}
}

func TestRepository_ReassignmentReplacesTorrentSubtitles(t *testing.T) {
	repo, _, id := openRepo(t)
	ctx := context.Background()
	ep := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id}
	neighbour := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id + 1}
	cleanupItems(t, repo, ep, neighbour)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.UpsertDownloaded(ctx, downloaded(ep, "en", "/subs/en.srt", "")))
	must(repo.UpsertTorrent(ctx, fromTorrent(ep, "en", hashA, "Old/en.srt")))
	must(repo.UpsertTorrent(ctx, fromTorrent(ep, "ru", hashA, "Old/ru.srt")))
	must(repo.UpsertTorrent(ctx, fromTorrent(neighbour, "en", hashA, "Old/E02.en.srt")))

	// The episode moves to torrent B.
	must(repo.UpsertTorrent(ctx, fromTorrent(ep, "en", hashB, "New/en.srt")))

	subs, err := repo.GetByItem(ctx, ep.Type, ep.ID)
	must(err)
	var got []string
	for _, s := range subs {
		got = append(got, string(s.Source)+":"+s.InfoHash+":"+s.FilePath)
	}
	want := []string{"opensubtitles::/subs/en.srt", "torrent:" + hashB + ":New/en.srt"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("after reassignment = %v, want %v", got, want)
	}

	// Another episode on torrent A keeps its subtitle.
	other, err := repo.GetByItem(ctx, neighbour.Type, neighbour.ID)
	must(err)
	if len(other) != 1 || other[0].InfoHash != hashA {
		t.Errorf("neighbour = %+v, want its torrent A subtitle", other)
	}
}

func TestRepository_DownloadingTwiceReplaces(t *testing.T) {
	repo, _, id := openRepo(t)
	ctx := context.Background()
	movie := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: id}
	cleanupItems(t, repo, movie)

	first := downloaded(movie, "en", "/subs/movie/1/en.srt", "Movie.2020.1080p.WEB-DL-GRP")
	if err := repo.UpsertDownloaded(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := downloaded(movie, "en", "/subs/movie/1/en.ass", "Movie.2020.1080p.BluRay-OTHER")
	second.Format = "ass"
	if err := repo.UpsertDownloaded(ctx, second); err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("second ID = %d, want %d (replaced in place)", second.ID, first.ID)
	}
	if second.CreatedAt.Before(first.CreatedAt) {
		t.Errorf("created_at went back: %v < %v", second.CreatedAt, first.CreatedAt)
	}

	got, err := repo.GetDownloaded(ctx, movie.Type, movie.ID, "en")
	if err != nil || got == nil {
		t.Fatalf("GetDownloaded = %v, %v", got, err)
	}
	if got.FilePath != "/subs/movie/1/en.ass" || got.Format != "ass" || got.ReleaseName != "Movie.2020.1080p.BluRay-OTHER" {
		t.Errorf("stored = %+v", got)
	}
	all, _ := repo.GetByItem(ctx, movie.Type, movie.ID)
	if len(all) != 1 {
		t.Errorf("rows = %d, want 1", len(all))
	}
}

func TestRepository_GetByItems(t *testing.T) {
	repo, _, id := openRepo(t)
	ctx := context.Background()
	movie := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: id}
	ep := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id} // Same ID, other type
	empty := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id + 1}
	cleanupItems(t, repo, movie, ep)

	for _, err := range []error{
		repo.UpsertTorrent(ctx, fromTorrent(movie, "en", hashA, "m/en.srt")),
		repo.UpsertDownloaded(ctx, downloaded(movie, "en", "/m/en.srt", "")),
		repo.UpsertDownloaded(ctx, downloaded(movie, "az", "/m/az.srt", "")),
		repo.UpsertDownloaded(ctx, downloaded(ep, "ru", "/e/ru.srt", "")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.GetByItems(ctx, []subtitle.Item{movie, ep, empty})
	if err != nil {
		t.Fatal(err)
	}
	var movieRows []string
	for _, s := range got[movie] {
		movieRows = append(movieRows, s.LanguageCode+":"+string(s.Source))
	}
	if want := []string{"az:opensubtitles", "en:opensubtitles", "en:torrent"}; len(movieRows) != 3 ||
		movieRows[0] != want[0] || movieRows[1] != want[1] || movieRows[2] != want[2] {
		t.Errorf("movie = %v, want %v", movieRows, want)
	}
	if len(got[ep]) != 1 || got[ep][0].LanguageCode != "ru" {
		t.Errorf("episode = %+v", got[ep])
	}
	if _, ok := got[empty]; ok {
		t.Error("an item without subtitles is in the map")
	}
	if none, err := repo.GetByItems(ctx, nil); err != nil || len(none) != 0 {
		t.Errorf("GetByItems(nil) = %v, %v", none, err)
	}
}

func TestRepository_TorrentSubtitleNeedsInfoHash(t *testing.T) {
	repo, db, id := openRepo(t)
	ctx := context.Background()
	movie := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: id}
	cleanupItems(t, repo, movie)

	if err := repo.UpsertTorrent(ctx, fromTorrent(movie, "en", "", "x.srt")); err == nil {
		t.Error("UpsertTorrent without an info hash succeeded")
	}
	// The CHECK holds even for writes that skip the repository.
	_, err := db.ExecContext(ctx, `INSERT INTO subtitles (item_type, item_id, language_code, language_name, file_path, source)
		VALUES ('movie', $1, 'en', 'English', 'x.srt', 'torrent')`, id)
	if err == nil {
		t.Error("a torrent subtitle without an info hash was inserted")
	}
}

// hdrezkaMovie is a library movie playing translator 238's stream; it
// returns the movie as a subtitle item, the stream's file and a writer
// that assigns another translator.
func hdrezkaMovie(t *testing.T, db *library.DB) (subtitle.Item, string, func(translator int) error) {
	t.Helper()
	movies := library.NewMovieRepository(db)
	movie := &library.Movie{TMDBID: 1_100_000_000 + int(time.Now().UnixNano()%1_000_000_000), Title: "Interstellar", Year: 2014}
	if err := movies.Create(movie); err != nil {
		t.Fatal(err)
	}
	item := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: movie.ID}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM subtitles WHERE item_type = 'movie' AND item_id = $1`, movie.ID)
		_, _ = db.Exec(`DELETE FROM torrent_assignments WHERE item_type = 'movie' AND item_id = $1`, movie.ID)
		_, _ = db.Exec(`DELETE FROM movies WHERE id = $1`, movie.ID)
	})
	assignments := library.NewAssignmentRepository(db)
	assign := func(translator int) error {
		ref := &library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
			TranslatorID: translator, TranslatorName: "Оригинал (+субтитры)", Quality: "1080p"}
		return assignments.WriteStream(context.Background(), library.AssignmentWrite{ItemType: library.ItemTypeMovie, MovieID: movie.ID,
			Rows: []library.TorrentAssignment{{ItemID: movie.ID, Stream: ref, FileSize: 1}}})
	}
	if err := assign(238); err != nil {
		t.Fatal(err)
	}
	return item, "hdrezka/2259/t238/movie/1080p.mp4", assign
}

func fromStream(item subtitle.Item, lang, release string) *subtitle.Subtitle {
	return &subtitle.Subtitle{
		ItemType: item.Type, ItemID: item.ID, LanguageCode: lang, LanguageName: subtitle.LanguageName(lang),
		Format: "srt", FilePath: fmt.Sprintf("/subs/hdrezka/movie/%d/%s.srt", item.ID, lang), FileSize: 30, ReleaseName: release,
	}
}

// One HDRezka subtitle per language, beside a downloaded one in the same
// language; storing the stream's again refreshes them; another stream's
// aren't stored.
func TestRepository_UpsertHDRezka(t *testing.T) {
	repo, db, _ := openRepo(t)
	ctx := context.Background()
	movie, file, _ := hdrezkaMovie(t, db)

	placed := 0
	place := func() error { placed++; return nil }
	en, ru := fromStream(movie, "en", "HDRezka · 238 · 720p"), fromStream(movie, "ru", "HDRezka · 238 · 720p")
	if err := repo.UpsertHDRezka(ctx, movie, file, []*subtitle.Subtitle{en, ru}, place); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertDownloaded(ctx, downloaded(movie, "en", "/subs/movie/en.srt", "")); err != nil {
		t.Fatal(err)
	}
	again := fromStream(movie, "en", "HDRezka · 238 · 1080p")
	if err := repo.UpsertHDRezka(ctx, movie, file, []*subtitle.Subtitle{again}, place); err != nil {
		t.Fatal(err)
	}
	if again.ID != en.ID || again.Source != subtitle.SourceHDRezka || placed != 2 {
		t.Errorf("again = %+v (placed %d), want en's row %d refreshed", again, placed, en.ID)
	}

	subs, err := repo.GetByItem(ctx, movie.Type, movie.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range subs {
		got = append(got, s.LanguageCode+":"+string(s.Source)+":"+s.ReleaseName)
	}
	want := []string{"en:opensubtitles:", "en:hdrezka:HDRezka · 238 · 1080p", "ru:hdrezka:HDRezka · 238 · 720p"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subtitles = %v, want %v", got, want)
	}

	// Another file isn't the item's: nothing is stored or placed.
	err = repo.UpsertHDRezka(ctx, movie, "hdrezka/2259/t56/movie/1080p.mp4", []*subtitle.Subtitle{fromStream(movie, "de", "")}, place)
	if !errors.Is(err, subtitle.ErrStaleStream) {
		t.Errorf("another stream: err = %v, want ErrStaleStream", err)
	}
	// A place that fails stores nothing.
	failing := func() error { return errors.New("disk full") }
	if err := repo.UpsertHDRezka(ctx, movie, file, []*subtitle.Subtitle{fromStream(movie, "uk", "")}, failing); err == nil {
		t.Error("a failed place was committed")
	}
	// Another item's subtitle isn't stored as this one's.
	other := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: movie.ID + 1}
	if err := repo.UpsertHDRezka(ctx, movie, file, []*subtitle.Subtitle{fromStream(other, "en", "")}, nil); err == nil {
		t.Error("another item's subtitle was stored")
	}
	if subs, _ := repo.GetByItem(ctx, movie.Type, movie.ID); len(subs) != 3 || placed != 2 {
		t.Errorf("subtitles = %d (placed %d), want the same 3", len(subs), placed)
	}
}

// Storing a stream's subtitles holds the stream: an assign of another
// translator waits for the commit, then drops them, and the stream's
// subtitles can't be stored after it.
func TestRepository_UpsertHDRezkaHoldsTheStream(t *testing.T) {
	repo, db, _ := openRepo(t)
	ctx := context.Background()
	movie, file, assign := hdrezkaMovie(t, db)

	held, release := make(chan struct{}), make(chan struct{})
	stored := make(chan error, 1)
	go func() {
		stored <- repo.UpsertHDRezka(ctx, movie, file, []*subtitle.Subtitle{fromStream(movie, "en", "")}, func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	assigned := make(chan error, 1)
	go func() { assigned <- assign(56) }()

	select {
	case err := <-assigned:
		t.Fatalf("the assign didn't wait for the subtitles (err %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-stored; err != nil {
		t.Fatal(err)
	}
	if err := <-assigned; err != nil {
		t.Fatal(err)
	}
	if subs, err := repo.GetByItem(ctx, movie.Type, movie.ID); err != nil || len(subs) != 0 {
		t.Errorf("subtitles after the new translator = %+v, %v; want the old stream's dropped", subs, err)
	}
	err := repo.UpsertHDRezka(ctx, movie, file, []*subtitle.Subtitle{fromStream(movie, "en", "")}, nil)
	if !errors.Is(err, subtitle.ErrStaleStream) {
		t.Errorf("the old stream's subtitles after the assign: err = %v, want ErrStaleStream", err)
	}
}
