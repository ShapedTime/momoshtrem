package api

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// An HDRezka stream's file shows the stream's subtitles and has no
// release to rank subtitles against (its made-up path would name it
// "hdrezka"); a torrent's release is its magnet's name, and nothing
// assigned shows neither kind.
func TestLibraryItemResolver_Streams(t *testing.T) {
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := library.NewDB(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	movies := library.NewMovieRepository(db)
	assignments := library.NewAssignmentRepository(db)

	base := 1_000_000_000 + int(time.Now().UnixNano()%1_000_000_000)
	movie := func(n int, title string) *library.Movie {
		m := &library.Movie{TMDBID: base + n, Title: title, Year: 2014}
		if err := movies.Create(m); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM torrent_assignments WHERE item_type = 'movie' AND item_id = $1`, m.ID)
			_, _ = db.Exec(`DELETE FROM movies WHERE id = $1`, m.ID)
		})
		return m
	}
	const hash = "abcdef0123456789abcdef0123456789abcdef01"
	fromTorrent := movie(0, "Resolver Torrent")
	if _, err := db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size)
		VALUES ('movie', $1, $2, 'magnet:?xt=urn:btih:abc&dn=Film.2014.1080p.WEB-GRP', 'Film/film.mkv', 1)`, fromTorrent.ID, hash); err != nil {
		t.Fatal(err)
	}
	fromStream := movie(1, "Resolver Stream")
	ref := &library.StreamRef{Provider: "hdrezka", PostID: 2259, TranslatorID: 56, TranslatorName: "LostFilm", Quality: "1080p"}
	if err := assignments.WriteStream(ctx, library.AssignmentWrite{ItemType: library.ItemTypeMovie, MovieID: fromStream.ID,
		Rows: []library.TorrentAssignment{{ItemID: fromStream.ID, Stream: ref, FileSize: 1}}}); err != nil {
		t.Fatal(err)
	}

	r := &libraryItemResolver{movies: movies, shows: library.NewShowRepository(db), assignments: assignments}
	got, err := r.ResolveSubtitleItem(ctx, subtitle.Item{Type: subtitle.ItemTypeMovie, ID: fromTorrent.ID})
	if err != nil || got.File != subtitle.TorrentFile(hash) || got.Release != "Film.2014.1080p.WEB-GRP" {
		t.Errorf("torrent = %+v, %v", got, err)
	}
	got, err = r.ResolveSubtitleItem(ctx, subtitle.Item{Type: subtitle.ItemTypeMovie, ID: fromStream.ID})
	if err != nil || got == nil || got.File != subtitle.HDRezkaFile() || got.Release != "" {
		t.Errorf("stream = %+v, %v", got, err)
	}
	nothing := movie(2, "Resolver Nothing")
	got, err = r.ResolveSubtitleItem(ctx, subtitle.Item{Type: subtitle.ItemTypeMovie, ID: nothing.ID})
	if err != nil || got == nil || got.File != (subtitle.CurrentFile{}) || got.Release != "" {
		t.Errorf("nothing assigned = %+v, %v", got, err)
	}
}
