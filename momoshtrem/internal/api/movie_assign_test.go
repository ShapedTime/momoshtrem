package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// addOnlyTorrents is a torrent.Service that can only add one torrent.
type addOnlyTorrents struct {
	torrent.Service
	info *torrent.TorrentInfo
}

func (a addOnlyTorrents) AddTorrent(string) (*torrent.TorrentInfo, error) { return a.info, nil }

// The legacy movie assign takes the classifier's best candidate: the film
// by title and year, not just the largest video.
func TestAssignMovieTorrent_PicksTheFilm(t *testing.T) {
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tmdbID := 2_000_000_000 + int(time.Now().UnixNano()%100_000_000)
	var movieID int64
	if err := db.QueryRow(`INSERT INTO movies (tmdb_id, title, year) VALUES ($1, 'Interstellar', 2014) RETURNING id`, tmdbID).Scan(&movieID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM torrent_assignments WHERE item_type = 'movie' AND item_id = $1`, movieID)
		_, _ = db.Exec(`DELETE FROM movies WHERE id = $1`, movieID)
	})

	hash := strings.Repeat("e", 40)
	torrents := addOnlyTorrents{info: &torrent.TorrentInfo{InfoHash: hash, Name: "Nolan Pack", Files: []identify.TorrentFile{
		{Path: "Nolan Pack/Inception.2010.1080p.BluRay.x264.mkv", Size: 14 << 30},
		{Path: "Nolan Pack/Interstellar.2014.1080p.BluRay.x264.mkv", Size: 12 << 30},
		{Path: "Nolan Pack/Sample/interstellar.sample.mkv", Size: 50 << 20},
	}}}
	s := NewServer(library.NewMovieRepository(db), library.NewShowRepository(db), library.NewAssignmentRepository(db),
		torrents, nil, Options{})

	rec := doBody(t, s, http.MethodPost, "/api/movies/"+strconv.FormatInt(movieID, 10)+"/assign-torrent",
		`{"magnet_uri": "magnet:?xt=urn:btih:`+hash+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp MovieAssignmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Assignment == nil || resp.Assignment.FilePath != "Nolan Pack/Interstellar.2014.1080p.BluRay.x264.mkv" ||
		resp.Assignment.Resolution != "1080p" || resp.Assignment.Source != "BluRay" {
		t.Errorf("assignment = %+v", resp.Assignment)
	}
}
