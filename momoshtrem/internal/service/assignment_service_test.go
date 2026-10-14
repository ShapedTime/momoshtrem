package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// fakeAdder answers AddTorrent from a fixed set of torrents by info hash.
type fakeAdder map[string]*torrent.TorrentInfo

func (f fakeAdder) AddTorrent(magnetURI string) (*torrent.TorrentInfo, error) {
	return f[torrent.ExtractInfoHash(magnetURI)], nil
}

// countingTree records full rebuilds.
type countingTree struct{ invalidated int }

func (c *countingTree) AddMovieToTree(*library.Movie, *library.TorrentAssignment) {}
func (c *countingTree) RemoveMovieFromTree(string, int)                           {}
func (c *countingTree) AddEpisodesToTree([]vfs.EpisodeWithContext)                {}
func (c *countingTree) RemoveEpisodeFromTree(string, int, int, int)               {}
func (c *countingTree) RemoveShowFromTree(string, int)                            {}
func (c *countingTree) InvalidateTree()                                           { c.invalidated++ }
func (c *countingTree) InvalidateFileHandles(string)                              {}

// Moving an episode to another torrent rebuilds the tree, so the earlier
// torrent's subtitle files leave it (and its cache) even when the new
// torrent brings none.
func TestAssignTorrent_ReassignmentRebuildsTree(t *testing.T) {
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	tmdbID := 2_000_000_000 + int(time.Now().UnixNano()%100_000_000)
	var showID, episodeID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO shows (tmdb_id, title, year) VALUES ($1, 'Reassigned Show', 2020) RETURNING id`, tmdbID).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM torrent_assignments WHERE item_type = 'episode' AND item_id = $1`, episodeID)
		_, _ = db.Exec(`DELETE FROM shows WHERE id = $1`, showID)
	})
	var seasonID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO seasons (show_id, season_number) VALUES ($1, 1) RETURNING id`, showID).Scan(&seasonID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO episodes (season_id, episode_number, name) VALUES ($1, 1, 'Pilot') RETURNING id`, seasonID).Scan(&episodeID); err != nil {
		t.Fatal(err)
	}

	hashA, hashB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	adder := fakeAdder{
		hashA: {InfoHash: hashA, Name: "Reassigned.Show.S01.1080p.WEB-DL-GRP", Files: []identify.TorrentFile{
			{Path: "Reassigned.Show.S01.1080p.WEB-DL-GRP/Reassigned.Show.S01E01.1080p.WEB-DL-GRP.mkv", Size: 1 << 30},
		}},
		hashB: {InfoHash: hashB, Name: "Reassigned.Show.S01E01.720p.HDTV.x264-LOL.mkv", Files: []identify.TorrentFile{
			{Path: "Reassigned.Show.S01E01.720p.HDTV.x264-LOL.mkv", Size: 1 << 29},
		}},
	}
	tree := &countingTree{}
	svc := NewShowAssignmentService(library.NewShowRepository(db), library.NewAssignmentRepository(db),
		adder, identify.NewIdentifier(nil), WithTreeUpdater(tree))

	if res, err := svc.AssignTorrent(ctx, showID, "magnet:?xt=urn:btih:"+hashA); err != nil || res.Summary.Matched != 1 {
		t.Fatalf("first assignment = %+v, %v", res, err)
	}
	if tree.invalidated != 0 {
		t.Errorf("first assignment rebuilt the tree %d times, want 0 (targeted update)", tree.invalidated)
	}

	if res, err := svc.AssignTorrent(ctx, showID, "magnet:?xt=urn:btih:"+hashB); err != nil || res.Summary.Matched != 1 {
		t.Fatalf("reassignment = %+v, %v", res, err)
	}
	if tree.invalidated != 1 {
		t.Errorf("reassignment rebuilt the tree %d times, want 1", tree.invalidated)
	}
}
