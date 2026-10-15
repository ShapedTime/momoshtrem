package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// fakeAdder answers AddTorrent from a fixed set of torrents by info hash.
type fakeAdder map[string]*torrent.TorrentInfo

func (f fakeAdder) AddTorrent(magnetURI string) (*torrent.TorrentInfo, error) {
	return f[torrent.ExtractInfoHash(magnetURI)], nil
}

// countingTree records full rebuilds and the episodes added to the tree.
type countingTree struct {
	invalidated int
	added       []vfs.EpisodeWithContext
}

func (c *countingTree) AddMovieToTree(*library.Movie, *library.TorrentAssignment) {}
func (c *countingTree) RemoveMovieFromTree(string, int)                           {}
func (c *countingTree) AddEpisodesToTree(eps []vfs.EpisodeWithContext) {
	c.added = append(c.added, eps...)
}
func (c *countingTree) RemoveEpisodeFromTree(string, int, int, int) {}
func (c *countingTree) RemoveShowFromTree(string, int)              {}
func (c *countingTree) InvalidateTree()                             { c.invalidated++ }
func (c *countingTree) InvalidateFileHandles(string)                {}

// recordingSubtitles records the torrent subtitles created.
type recordingSubtitles struct{ subs []*subtitle.Subtitle }

func (r *recordingSubtitles) CreateTorrentSubtitle(_ context.Context, sub *subtitle.Subtitle) error {
	r.subs = append(r.subs, sub)
	return nil
}

func testDB(t *testing.T) *library.DB {
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
	return db
}

// seedShow inserts a show with the given episodes ({season, episode, name})
// and returns the show ID and the episode IDs by "SxxEyy".
func seedShow(t *testing.T, db *library.DB, title string, episodes [][3]any) (int64, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	tmdbID := 2_000_000_000 + int(time.Now().UnixNano()%100_000_000)
	var showID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO shows (tmdb_id, title, year) VALUES ($1, $2, 2020) RETURNING id`, tmdbID, title).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = db.Exec(`DELETE FROM torrent_assignments WHERE item_type = 'episode' AND item_id = $1`, id)
		}
		_, _ = db.Exec(`DELETE FROM shows WHERE id = $1`, showID)
	})
	seasons := map[int]int64{}
	for _, e := range episodes {
		season, number, name := e[0].(int), e[1].(int), e[2].(string)
		seasonID, ok := seasons[season]
		if !ok {
			if err := db.QueryRowContext(ctx, `INSERT INTO seasons (show_id, season_number) VALUES ($1, $2) RETURNING id`, showID, season).Scan(&seasonID); err != nil {
				t.Fatal(err)
			}
			seasons[season] = seasonID
		}
		var id int64
		if err := db.QueryRowContext(ctx, `INSERT INTO episodes (season_id, episode_number, name) VALUES ($1, $2, $3) RETURNING id`, seasonID, number, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[seLabelOf(season, number)] = id
	}
	return showID, ids
}

func seLabelOf(season, episode int) string {
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

// Moving an episode to another torrent rebuilds the tree, so the earlier
// torrent's subtitle files leave it (and its cache) even when the new
// torrent brings none.
func TestAssignTorrent_ReassignmentRebuildsTree(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	showID, _ := seedShow(t, db, "Reassigned Show", [][3]any{{1, 1, "Pilot"}})

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
		adder, WithTreeUpdater(tree))

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

// The legacy path assigns everything the classifier places: a multi-episode
// file gets a row per episode, a special goes to season 0, extras are
// skipped, subtitles are created, and two files on one episode are both
// left out.
func TestAssignTorrent_LegacySemantics(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	showID, ids := seedShow(t, db, "Legacy Show", [][3]any{
		{0, 1, "Christmas Special"},
		{1, 1, "Pilot"}, {1, 2, "Second"}, {1, 3, "Third"}, {1, 4, "Fourth"},
	})

	hash := strings.Repeat("c", 40)
	root := "Legacy.Show.S01.1080p.WEB-GRP/"
	adder := fakeAdder{hash: {InfoHash: hash, Name: "Legacy.Show.S01.1080p.WEB-GRP", Files: []identify.TorrentFile{
		{Path: root + "Legacy.Show.S01E01E02.1080p.WEB-GRP.mkv", Size: 2 << 30},
		{Path: root + "1080p/Legacy.Show.S01E03.1080p.WEB-GRP.mkv", Size: 1 << 30},
		{Path: root + "720p/Legacy.Show.S01E03.720p.WEB-GRP.mkv", Size: 1 << 29},
		{Path: root + "Legacy.Show.S00E01.Christmas.Special.1080p.WEB-GRP.mkv", Size: 1 << 30},
		{Path: root + "Legacy.Show.S01E04.1080p.WEB-GRP.en.srt", Size: 40 << 10},
		{Path: root + "Sample/legacy.show.s01e01.1080p-sample.mkv", Size: 30 << 20},
		{Path: root + "Legacy.Show.S01.1080p.WEB-GRP.nfo", Size: 2 << 10},
	}}}
	tree := &countingTree{}
	subs := &recordingSubtitles{}
	assignments := library.NewAssignmentRepository(db)
	svc := NewShowAssignmentService(library.NewShowRepository(db), assignments, adder,
		WithTreeUpdater(tree), WithSubtitleCreator(subs))

	res, err := svc.AssignTorrent(ctx, showID, "magnet:?xt=urn:btih:"+hash+"&dn=Legacy.Show.S01.1080p.WEB-GRP")
	if err != nil {
		t.Fatal(err)
	}
	want := AssignmentSummary{TotalFiles: 6, Matched: 3, Unmatched: 2, Skipped: 1, SubtitlesFound: 1}
	if res.Summary != want {
		t.Errorf("summary = %+v, want %+v", res.Summary, want)
	}

	for _, label := range []string{"S01E01", "S01E02"} {
		a, err := assignments.GetActiveForItem(library.ItemTypeEpisode, ids[label])
		if err != nil || a == nil || a.FilePath != root+"Legacy.Show.S01E01E02.1080p.WEB-GRP.mkv" || a.Resolution != "1080p" {
			t.Errorf("%s assignment = %+v, %v", label, a, err)
		}
	}
	if a, _ := assignments.GetActiveForItem(library.ItemTypeEpisode, ids["S00E01"]); a == nil {
		t.Error("the special wasn't assigned")
	}
	if a, _ := assignments.GetActiveForItem(library.ItemTypeEpisode, ids["S01E03"]); a != nil {
		t.Errorf("a conflicting file was assigned: %+v", a)
	}
	for _, u := range res.Unmatched {
		if !strings.HasPrefix(u.Reason, "Conflicts with ") || u.Season != -1 {
			t.Errorf("unmatched %+v", u)
		}
	}

	special := false
	for _, ep := range tree.added {
		if ep.SeasonNumber == 0 && ep.Episode.ID == ids["S00E01"] {
			special = true
		}
	}
	if !special {
		t.Error("the special wasn't added to the tree in season 0")
	}
	if len(subs.subs) != 1 || subs.subs[0].ItemID != ids["S01E04"] || subs.subs[0].LanguageCode != "en" {
		t.Errorf("subtitles = %+v", subs.subs)
	}
	if tree.invalidated != 1 {
		t.Errorf("tree rebuilt %d times, want 1 (new subtitles)", tree.invalidated)
	}
}

// With specials off, a special is reported unmatched instead of assigned.
func TestAssignTorrent_SpecialsOff(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	showID, ids := seedShow(t, db, "No Specials Show", [][3]any{{0, 1, "Christmas Special"}, {1, 1, "Pilot"}})
	if _, err := db.ExecContext(ctx, `UPDATE shows SET include_specials = FALSE WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}

	hash := strings.Repeat("d", 40)
	adder := fakeAdder{hash: {InfoHash: hash, Name: "No.Specials.Show.S00E01.mkv", Files: []identify.TorrentFile{
		{Path: "No.Specials.Show.S00E01.Christmas.Special.mkv", Size: 1 << 30},
	}}}
	svc := NewShowAssignmentService(library.NewShowRepository(db), library.NewAssignmentRepository(db), adder)
	res, err := svc.AssignTorrent(ctx, showID, "magnet:?xt=urn:btih:"+hash)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Matched != 0 || len(res.Unmatched) != 1 || res.Unmatched[0].Reason != "Specials are off for this show" {
		t.Errorf("result = %+v", res)
	}
	if a, _ := library.NewAssignmentRepository(db).GetActiveForItem(library.ItemTypeEpisode, ids["S00E01"]); a != nil {
		t.Errorf("the special was assigned: %+v", a)
	}
}
