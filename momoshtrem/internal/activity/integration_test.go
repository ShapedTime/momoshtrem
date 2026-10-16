package activity_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/activity"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// The integration tests run over a private, freshly migrated schema and a
// real torrent service with nothing loaded (no client: torrents load on
// their first WebDAV read, and these never read).

func openSchemaDB(t *testing.T) *library.DB {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	base, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { base.Close() })
	schema := fmt.Sprintf("activity_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := library.NewDB(url + sep + "search_path=" + schema)
	if err != nil {
		t.Fatalf("NewDB in %s: %v", schema, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type fixture struct {
	t        *testing.T
	ctx      context.Context
	db       *library.DB
	jobs     *library.AssignmentJobRepository
	settings *library.SettingsRepository
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
}

func (f *fixture) id(query string, args ...any) int64 {
	f.t.Helper()
	var id int64
	if err := f.db.QueryRow(query, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
	return id
}

func (f *fixture) assign(itemType string, itemID int64, hash, dn, path string) {
	f.t.Helper()
	f.exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
		VALUES ($1, $2, $3, $4, $5, 100, TRUE)`, itemType, itemID, hash, "magnet:?xt=urn:btih:"+hash+"&dn="+dn, path)
}

// activityOver is an activity service over a fresh torrent service: what a
// restart starts with.
func (f *fixture) activityOver() (*activity.Service, torrent.Service) {
	client := torrent.NewService(nil, nil, time.Second, time.Second)
	return activity.New(activity.Deps{Torrents: client, Library: f.jobs, Pauses: f.settings}), client
}

// The B6 acceptance: with assignments in the library and nothing loaded,
// Activity lists every torrent, idle, with its titles. Then pauses persist
// across a restart, pause all and resume all count, and deleting a torrent
// clears its pause.
func TestActivity_AssignedTorrentsWithNothingLoaded(t *testing.T) {
	db := openSchemaDB(t)
	f := &fixture{t: t, ctx: context.Background(), db: db,
		jobs: library.NewAssignmentJobRepository(db), settings: library.NewSettingsRepository(db)}
	showHash, movieHash := strings.Repeat("a", 40), strings.Repeat("b", 40)

	show := f.id(`INSERT INTO shows (tmdb_id, title, year, poster_path) VALUES (209867, 'Frieren', 2023, '/frieren.jpg') RETURNING id`)
	season := f.id(`INSERT INTO seasons (show_id, season_number) VALUES ($1, 1) RETURNING id`, show)
	for n := 1; n <= 3; n++ {
		ep := f.id(`INSERT INTO episodes (season_id, episode_number) VALUES ($1, $2) RETURNING id`, season, n)
		f.assign("episode", ep, showHash, "Frieren.S01.1080p", fmt.Sprintf("Frieren/E%02d.mkv", n))
	}
	movie := f.id(`INSERT INTO movies (tmdb_id, title, year) VALUES (666277, 'Past Lives', 2023) RETURNING id`)
	f.assign("movie", movie, movieHash, "Past.Lives.2023.1080p", "Past.Lives.mkv")

	svc, _ := f.activityOver()
	items, err := svc.List(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want both torrents", items)
	}
	byHash := map[string]activity.Item{}
	for _, it := range items {
		byHash[it.InfoHash] = it
		if it.State != activity.StateIdle || it.Loaded || it.Progress != nil || it.Complete != nil || it.Peers != nil ||
			it.LastActiveAt != nil || it.DownloadBps != 0 || it.AddedAt.IsZero() || len(it.Titles) != 1 {
			t.Errorf("%s: %+v, want idle and not loaded, with its title", it.InfoHash[:4], it)
		}
	}
	s := byHash[showHash]
	if s.Name != "Frieren.S01.1080p" || s.Titles[0].Title != "Frieren" || s.Titles[0].LibraryID != show ||
		s.Titles[0].TMDBID != 209867 || s.Titles[0].Scope == nil || *s.Titles[0].Scope != "Season 1" ||
		s.Titles[0].PosterPath == nil || *s.Titles[0].PosterPath != "/frieren.jpg" {
		t.Errorf("the show's torrent = %+v (title %+v)", s, s.Titles[0])
	}
	m := byHash[movieHash]
	if m.Name != "Past.Lives.2023.1080p" || m.Titles[0].MediaType != "movie" || m.Titles[0].Scope != nil || m.Titles[0].PosterPath != nil {
		t.Errorf("the movie's torrent = %+v (title %+v)", m, m.Titles[0])
	}
	sum, err := svc.Summary(f.ctx)
	if err != nil || sum.Counts != (activity.Counts{Idle: 2}) || sum.Disk != nil {
		t.Fatalf("summary = %+v, %v", sum, err)
	}

	// A pause is stored, and a new torrent service (a restart) gets it back.
	if item, err := svc.Pause(f.ctx, strings.ToUpper(showHash)); err != nil || item.State != activity.StatePaused {
		t.Fatalf("Pause = %+v, %v", item, err)
	}
	// Meanwhile a job on another torrent was paused, then discarded: the
	// library doesn't know that torrent any more, so the restart prunes its
	// pause instead of applying it.
	discarded := strings.Repeat("c", 40)
	f.exec(`INSERT INTO assignment_jobs (media_type, tmdb_id, show_id, info_hash, magnet_uri, torrent_name, status)
		VALUES ('tv', 209867, $1, $2, $3, 'Discarded', 'cancelled')`, show, discarded, "magnet:?xt=urn:btih:"+discarded)
	if err := f.settings.PauseTorrents(f.ctx, map[string]time.Time{discarded: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc, client := f.activityOver()
	if err := svc.RestorePauses(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := client.PausedAt(showHash); !ok {
		t.Fatal("the pause didn't survive the restart")
	}
	if _, ok := client.PausedAt(discarded); ok {
		t.Error("the discarded torrent came back paused")
	}
	if stored, err := f.settings.TorrentPauses(f.ctx); err != nil || len(stored) != 1 || stored[showHash].IsZero() {
		t.Errorf("stored pauses after the restart = %v, %v; want only the known torrent's", stored, err)
	}
	if items, _ := svc.List(f.ctx); items[0].InfoHash != showHash || items[0].State != activity.StatePaused {
		t.Errorf("after the restart: %+v, want the show's torrent paused first", items)
	}

	// Pause all takes streaming and downloading torrents only: none here.
	if n, err := svc.PauseAll(f.ctx); err != nil || n != 0 {
		t.Errorf("PauseAll = %d, %v; want 0", n, err)
	}
	if _, err := svc.Pause(f.ctx, movieHash); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.ResumeAll(f.ctx); err != nil || n != 2 {
		t.Errorf("ResumeAll = %d, %v; want 2", n, err)
	}
	if stored, err := f.settings.TorrentPauses(f.ctx); err != nil || len(stored) != 0 {
		t.Errorf("stored pauses after resume all = %v, %v", stored, err)
	}

	// Deleting a paused torrent clears its pause, stored and in the client.
	if _, err := svc.Pause(f.ctx, movieHash); err != nil {
		t.Fatal(err)
	}
	removals := service.NewRemovalService(service.RemovalDeps{
		Jobs: f.jobs, Movies: library.NewMovieRepository(db), Shows: library.NewShowRepository(db),
		Assignments: library.NewAssignmentRepository(db), Torrents: client,
	})
	if err := removals.DeleteTorrent(f.ctx, movieHash, false); err != nil {
		t.Fatal(err)
	}
	if stored, _ := f.settings.TorrentPauses(f.ctx); len(stored) != 0 {
		t.Errorf("stored pauses after the delete = %v", stored)
	}
	if _, ok := client.PausedAt(movieHash); ok {
		t.Error("the client still has the deleted torrent paused")
	}
	if items, _ := svc.List(f.ctx); len(items) != 1 || items[0].InfoHash != showHash {
		t.Errorf("after the delete: %+v, want only the show's torrent", items)
	}
	if _, err := svc.Pause(f.ctx, movieHash); err == nil {
		t.Error("pausing the deleted torrent: want ErrTorrentNotFound")
	}
}
