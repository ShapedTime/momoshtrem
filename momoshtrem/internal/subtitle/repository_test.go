package subtitle_test

import (
	"context"
	"os"
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
