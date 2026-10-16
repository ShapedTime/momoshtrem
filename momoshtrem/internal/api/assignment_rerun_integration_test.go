package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// openRerunDB opens the test database in a private, freshly migrated
// schema.
func openRerunDB(t *testing.T) *library.DB {
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
	schema := fmt.Sprintf("api_rerun_%d", time.Now().UnixNano())
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

// Re-run mapping end to end: an assign by info_hash creates a job from the
// magnet the library stored (an assignment's, else a job's), and the open
// job dedupe still holds.
func TestAssign_ByInfoHash_Integration(t *testing.T) {
	db := openRerunDB(t)
	jobs := library.NewAssignmentJobRepository(db)
	shows, movies := library.NewShowRepository(db), library.NewMovieRepository(db)
	s := newTestServer(Options{
		Assignments: service.NewJobService(service.JobDeps{Jobs: jobs, Shows: shows, Movies: movies}), // no workers started
		Magnets:     jobs,
	})
	id := func(q string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(q, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return id
	}
	show := id(`INSERT INTO shows (tmdb_id, title, year) VALUES (209867, 'Frieren', 2023) RETURNING id`)
	season := id(`INSERT INTO seasons (show_id, season_number) VALUES ($1, 1) RETURNING id`, show)
	ep := id(`INSERT INTO episodes (season_id, episode_number) VALUES ($1, 1) RETURNING id`, season)
	hash := strings.Repeat("a", 40)
	stored := "magnet:?xt=urn:btih:" + hash + "&dn=Frieren.S01&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337"
	if _, err := db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
		VALUES ('episode', $1, $2, $3, 'Frieren.S01/E01.mkv', 100, TRUE)`, ep, hash, stored); err != nil {
		t.Fatal(err)
	}

	assign := func(path, body string) (int, int64) {
		t.Helper()
		rec := doBody(t, s, http.MethodPost, path, body)
		var got struct {
			JobID int64 `json:"job_id"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return rec.Code, got.JobID
	}

	code, jobID := assign(fmt.Sprintf("/api/shows/%d/assign-torrent", show), `{"info_hash": "`+strings.ToUpper(hash)+`", "hint": {"season": 1}}`)
	if code != http.StatusAccepted || jobID == 0 {
		t.Fatalf("re-run = %d, job %d; want 202", code, jobID)
	}
	var magnet, status, gotHash string
	var showID int64
	if err := db.QueryRow(`SELECT magnet_uri, status, info_hash, show_id FROM assignment_jobs WHERE id = $1`, jobID).
		Scan(&magnet, &status, &gotHash, &showID); err != nil {
		t.Fatal(err)
	}
	if magnet != stored || status != "queued" || gotHash != hash || showID != show {
		t.Errorf("job = %q %s %s show %d; want the stored magnet, queued", magnet, status, gotHash, showID)
	}

	// The open job for that show and torrent is the answer again.
	if code, again := assign(fmt.Sprintf("/api/shows/%d/assign-torrent", show), `{"info_hash": "`+hash+`"}`); code != http.StatusOK || again != jobID {
		t.Errorf("again = %d, job %d; want 200 and job %d", code, again, jobID)
	}

	// A movie whose torrent only a cancelled job knows: that job's magnet.
	movie := id(`INSERT INTO movies (tmdb_id, title, year) VALUES (666277, 'Past Lives', 2023) RETURNING id`)
	other := strings.Repeat("b", 40)
	jobMagnet := "magnet:?xt=urn:btih:" + other + "&dn=Past.Lives"
	if _, err := db.Exec(`INSERT INTO assignment_jobs (media_type, tmdb_id, movie_id, info_hash, magnet_uri, torrent_name, status)
		VALUES ('movie', 666277, $1, $2, $3, 'Past.Lives', 'cancelled')`, movie, other, jobMagnet); err != nil {
		t.Fatal(err)
	}
	code, movieJob := assign(fmt.Sprintf("/api/movies/%d/assign-torrent", movie), `{"info_hash": "`+other+`"}`)
	if code != http.StatusAccepted || movieJob == jobID {
		t.Fatalf("movie re-run = %d, job %d", code, movieJob)
	}
	if err := db.QueryRow(`SELECT magnet_uri FROM assignment_jobs WHERE id = $1`, movieJob).Scan(&magnet); err != nil || magnet != jobMagnet {
		t.Errorf("movie job magnet = %q, %v; want the cancelled job's", magnet, err)
	}

	// A torrent the library never knew.
	if code, _ := assign(fmt.Sprintf("/api/shows/%d/assign-torrent", show), `{"info_hash": "`+strings.Repeat("c", 40)+`"}`); code != http.StatusNotFound {
		t.Errorf("unknown torrent = %d, want 404", code)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM assignment_jobs`).Scan(&n); err != nil || n != 3 {
		t.Errorf("jobs = %d, %v; want 3 (no job for the unknown torrent)", n, err)
	}
}
