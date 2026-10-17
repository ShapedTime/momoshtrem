package service

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// The removal tests run against Postgres, each in its own schema, with a
// real subtitle service over a temp download folder and a fake torrent
// client.

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hashC = "cccccccccccccccccccccccccccccccccccccccc"
)

// fakeClient is a torrent client that knows which torrents are loaded or
// stored and which ones peers would send, and records adds and removals.
type fakeClient struct {
	mu      sync.Mutex
	loaded  map[string]bool
	stored  map[string]bool // metainfo on disk, not loaded
	peers   map[string]bool // AddTorrent finds these
	adds    []string
	removed []removal
	resumed []string
}

type removal struct {
	hash       string
	deleteData bool
}

func newFakeClient() *fakeClient {
	return &fakeClient{loaded: map[string]bool{}, stored: map[string]bool{}, peers: map[string]bool{}}
}

func (f *fakeClient) GetTorrent(hash string) (*torrent.TorrentInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.loaded[hash] {
		return nil, torrent.ErrTorrentNotFound
	}
	return &torrent.TorrentInfo{InfoHash: hash}, nil
}

func (f *fakeClient) AddTorrent(magnetURI string) (*torrent.TorrentInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	hash := torrent.NormalizeInfoHash(torrent.ExtractInfoHash(magnetURI))
	f.adds = append(f.adds, hash)
	if !f.peers[hash] && !f.stored[hash] {
		return nil, torrent.ErrMetadataTimeout
	}
	f.loaded[hash] = true
	return &torrent.TorrentInfo{InfoHash: hash}, nil
}

func (f *fakeClient) RemoveTorrent(hash string, deleteData bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	hash = torrent.NormalizeInfoHash(hash)
	f.removed = append(f.removed, removal{hash, deleteData})
	if !f.loaded[hash] && !f.stored[hash] {
		return torrent.ErrTorrentNotFound
	}
	delete(f.loaded, hash)
	delete(f.stored, hash)
	return nil
}

func (f *fakeClient) Resume(hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumed = append(f.resumed, hash)
	return nil
}

func (f *fakeClient) removals() []removal {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.removed)
	slices.SortFunc(out, func(a, b removal) int { return strings.Compare(a.hash, b.hash) })
	return out
}

// recordingTree counts rebuilds and records file handle invalidations.
type recordingTree struct {
	mu       sync.Mutex
	rebuilds int
	handles  []string
}

func (r *recordingTree) InvalidateTree() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rebuilds++
}

func (r *recordingTree) InvalidateFileHandles(hash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handles = append(r.handles, hash)
}

func (r *recordingTree) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rebuilds
}

// countingWaker counts the download manager's wakes.
type countingWaker struct {
	mu    sync.Mutex
	wakes int
}

func (w *countingWaker) Wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wakes++
}

func (w *countingWaker) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wakes
}

type removalEnv struct {
	*jobEnv
	svc       *RemovalService
	client    *fakeClient
	tree      *recordingTree
	downloads *countingWaker
	dir       string // the subtitles' download path
}

func newRemovalEnv(t *testing.T) *removalEnv {
	t.Helper()
	je := newJobEnv(t)
	e := &removalEnv{jobEnv: je, client: newFakeClient(), tree: &recordingTree{}, downloads: &countingWaker{}, dir: t.TempDir()}
	e.svc = NewRemovalService(RemovalDeps{
		Jobs:        je.jobs,
		Movies:      library.NewMovieRepository(je.db),
		Shows:       library.NewShowRepository(je.db),
		Assignments: library.NewAssignmentRepository(je.db),
		Torrents:    e.client,
		Subtitles:   subtitle.NewService(nil, subtitle.NewRepository(je.db.DB), e.dir),
		Tree:        e.tree,
		Pauses:      library.NewSettingsRepository(je.db),
		Downloads:   e.downloads,
	})
	return e
}

// assign seeds a torrent assignment of the item.
func (e *removalEnv) assign(itemType library.ItemType, itemID int64, hash, path string, active bool) {
	e.t.Helper()
	if _, err := e.db.Exec(`
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active, match_source)
		VALUES ($1, $2, $3, $4, $5, 100, $6, 'rules')`,
		itemType, itemID, hash, magnet(hash), path, active); err != nil {
		e.t.Fatal(err)
	}
}

// assignHDRezka seeds an active HDRezka stream for the item.
func (e *removalEnv) assignHDRezka(itemType library.ItemType, itemID int64) {
	e.t.Helper()
	if _, err := e.db.Exec(`
		INSERT INTO torrent_assignments (item_type, item_id, kind, stream_ref, file_path, file_size, is_active)
		VALUES ($1, $2, 'hdrezka', '{"provider":"hdrezka","path":"/films/x.html","post_id":7,"translator_id":56}',
		        'hdrezka/7/t56/movie/1080p.mp4', 100, TRUE)`,
		itemType, itemID); err != nil {
		e.t.Fatal(err)
	}
}

// subtitle seeds one of the item's subtitles and returns its file path.
// Downloaded and HDRezka ones get a real file in their folder; a torrent
// one is a path in the torrent.
func (e *removalEnv) subtitle(itemType subtitle.ItemType, itemID int64, src subtitle.Source, lang, hash string) string {
	e.t.Helper()
	id := strconv.FormatInt(itemID, 10)
	var path string
	switch src {
	case subtitle.SourceOpenSubtitles:
		path = filepath.Join(e.dir, string(itemType), id, lang+".srt")
	case subtitle.SourceHDRezka:
		path = filepath.Join(e.dir, "hdrezka", string(itemType), id, lang+".srt")
	case subtitle.SourceTorrent:
		path = "Pack/Subs/" + lang + ".srt"
	}
	if src != subtitle.SourceTorrent {
		e.writeFile(path)
	}
	if _, err := e.db.Exec(`
		INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash)
		VALUES ($1, $2, $3, $3, 'srt', $4, 10, $5, NULLIF($6, ''))`,
		itemType, itemID, lang, path, src, hash); err != nil {
		e.t.Fatal(err)
	}
	return path
}

func (e *removalEnv) writeFile(path string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n"), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// seedJob seeds a job for the torrent on a show or movie (0: neither, a
// dry run on a title outside the library).
func (e *removalEnv) seedJob(showID, movieID int64, hash string, status library.JobStatus, dryRun bool) int64 {
	e.t.Helper()
	media, tmdb := library.JobTV, uniqueTMDB()
	var show, movie any
	if showID != 0 {
		show = showID
		e.mustScan(&tmdb, `SELECT tmdb_id FROM shows WHERE id = $1`, showID)
	}
	if movieID != 0 {
		media, movie = library.JobMovie, movieID
		e.mustScan(&tmdb, `SELECT tmdb_id FROM movies WHERE id = $1`, movieID)
	}
	var id int64
	e.mustScan(&id, `
		INSERT INTO assignment_jobs (media_type, tmdb_id, show_id, movie_id, info_hash, magnet_uri, torrent_name, status, dry_run)
		VALUES ($1, $2, $3, $4, $5, $6, 'Pack', $7, $8) RETURNING id`,
		media, tmdb, show, movie, hash, magnet(hash), status, dryRun)
	return id
}

func (e *removalEnv) mustScan(dest any, query string, args ...any) {
	e.t.Helper()
	if err := e.db.QueryRow(query, args...).Scan(dest); err != nil {
		e.t.Fatalf("%s: %v", strings.TrimSpace(query), err)
	}
}

func (e *removalEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	e.mustScan(&n, query, args...)
	return n
}

// sources lists the item's subtitles' sources, sorted.
func (e *removalEnv) sources(itemType subtitle.ItemType, itemID int64) []string {
	e.t.Helper()
	rows, err := e.db.Query(`SELECT source FROM subtitles WHERE item_type = $1 AND item_id = $2 ORDER BY source`, itemType, itemID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func (e *removalEnv) status(jobID int64) library.JobStatus {
	e.t.Helper()
	var s library.JobStatus
	e.mustScan(&s, `SELECT status FROM assignment_jobs WHERE id = $1`, jobID)
	return s
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestDeleteTorrent_ForgetsItAndKeepsHistory(t *testing.T) {
	e := newRemovalEnv(t)
	showID, eps := e.show("Frieren", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	e.assign(library.ItemTypeEpisode, eps["S01E01"], hashA, "Pack/E01.mkv", true)
	e.assign(library.ItemTypeEpisode, eps["S01E02"], hashA, "Pack/E02.mkv", true)
	e.assign(library.ItemTypeEpisode, eps["S01E03"], hashA, "Pack/E03.mkv", false) // history
	e.assign(library.ItemTypeEpisode, eps["S01E03"], hashB, "Other/E03.mkv", true)
	e.subtitle(subtitle.ItemTypeEpisode, eps["S01E01"], subtitle.SourceTorrent, "en", hashA)
	e.subtitle(subtitle.ItemTypeEpisode, eps["S01E03"], subtitle.SourceTorrent, "en", hashB)
	downloadedSub := e.subtitle(subtitle.ItemTypeEpisode, eps["S01E01"], subtitle.SourceOpenSubtitles, "az", "")
	open := e.seedJob(showID, 0, hashA, library.JobNeedsReview, false)
	dryRun := e.seedJob(0, 0, hashA, library.JobReady, true)
	applied := e.seedJob(showID, 0, hashA, library.JobApplied, false)
	other := e.seedJob(showID, 0, hashB, library.JobNeedsReview, false)
	e.client.loaded[hashA] = true

	if err := e.svc.DeleteTorrent(e.ctx, strings.ToUpper(hashA), true); err != nil {
		t.Fatalf("DeleteTorrent: %v", err)
	}

	if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE info_hash = $1 AND is_active`, hashA); n != 0 {
		t.Errorf("%d active rows left for the torrent", n)
	}
	if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE info_hash = $1`, hashA); n != 3 {
		t.Errorf("%d rows for the torrent, want all 3 kept as history", n)
	}
	if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE info_hash = $1 AND is_active`, hashB); n != 1 {
		t.Errorf("the other torrent's assignment changed: %d active", n)
	}
	if got := e.sources(subtitle.ItemTypeEpisode, eps["S01E01"]); !slices.Equal(got, []string{"opensubtitles"}) {
		t.Errorf("E01 subtitles = %v, want only the downloaded one", got)
	}
	if !exists(downloadedSub) {
		t.Error("the downloaded subtitle's file was removed")
	}
	if got := e.sources(subtitle.ItemTypeEpisode, eps["S01E03"]); !slices.Equal(got, []string{"torrent"}) {
		t.Errorf("E03 subtitles = %v, want the other torrent's kept", got)
	}
	for _, c := range []struct {
		id   int64
		want library.JobStatus
	}{{open, library.JobCancelled}, {dryRun, library.JobCancelled}, {applied, library.JobApplied}, {other, library.JobNeedsReview}} {
		if got := e.status(c.id); got != c.want {
			t.Errorf("job %d status = %s, want %s", c.id, got, c.want)
		}
	}
	if got := e.client.removals(); !slices.Equal(got, []removal{{hashA, true}}) {
		t.Errorf("removals = %v, want the torrent with its data", got)
	}
	if !slices.Equal(e.client.resumed, []string{hashA}) {
		t.Errorf("resumed = %v, want the deleted torrent's pause cleared", e.client.resumed)
	}
	if e.tree.count() != 1 || !slices.Equal(e.tree.handles, []string{hashA}) {
		t.Errorf("tree rebuilds = %d, handles = %v", e.tree.count(), e.tree.handles)
	}
}

// A torrent that isn't loaded is still deleted when the library knows it;
// its data is deleted in the background, from metainfo fetched again.
func TestDeleteTorrent_NotLoaded(t *testing.T) {
	tests := []struct {
		name        string
		deleteData  bool
		peers       bool // its metainfo can be fetched again
		reused      bool // assigned again before the fetch lands
		wantAdds    []string
		wantRemoved []removal
	}{
		{"keep data", false, true, false, nil, []removal{{hashA, false}}},
		{"delete data", true, true, false, []string{hashA}, []removal{{hashA, true}, {hashA, true}}},
		{"delete data without peers", true, false, false, []string{hashA}, []removal{{hashA, true}}},
		{"delete data, in use again", true, true, true, []string{hashA}, []removal{{hashA, true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newRemovalEnv(t)
			movieID := e.movie("Dune", 2021)
			e.assign(library.ItemTypeMovie, movieID, hashA, "Dune.mkv", true)
			e.client.peers[hashA] = tt.peers
			if tt.reused {
				// The fetch lands after the movie was assigned the torrent again.
				e.client.peers[hashA] = false
				e.client.stored[hashA] = false
				e.svc.torrents = &reassigningClient{fakeClient: e.client, reassign: func() {
					e.assign(library.ItemTypeMovie, e.movie("Dune Again", 2021), hashA, "Dune.mkv", true)
				}}
			}

			if err := e.svc.DeleteTorrent(e.ctx, hashA, tt.deleteData); err != nil {
				t.Fatalf("DeleteTorrent: %v, want the known torrent deleted", err)
			}
			e.svc.background.Wait()

			if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE item_id = $1 AND is_active`, movieID); n != 0 {
				t.Errorf("%d active rows left", n)
			}
			if !slices.Equal(e.client.adds, tt.wantAdds) {
				t.Errorf("adds = %v, want %v", e.client.adds, tt.wantAdds)
			}
			if got := e.client.removals(); !slices.Equal(got, tt.wantRemoved) {
				t.Errorf("removals = %v, want %v", got, tt.wantRemoved)
			}
		})
	}
}

// reassigningClient assigns the torrent again while its metainfo is being
// fetched.
type reassigningClient struct {
	*fakeClient
	reassign func()
}

func (r *reassigningClient) AddTorrent(magnetURI string) (*torrent.TorrentInfo, error) {
	r.reassign()
	r.fakeClient.mu.Lock()
	r.fakeClient.peers[hashA] = true
	r.fakeClient.mu.Unlock()
	return r.fakeClient.AddTorrent(magnetURI)
}

func TestDeleteTorrent_Unknown(t *testing.T) {
	e := newRemovalEnv(t)
	movieID := e.movie("Arrival", 2016)
	e.assign(library.ItemTypeMovie, movieID, hashA, "Arrival.mkv", false) // history only
	dryRun := e.seedJob(0, 0, hashA, library.JobReady, true)              // dry runs don't count
	known := e.seedJob(0, movieID, hashB, library.JobQueued, false)       // an open job does

	if err := e.svc.DeleteTorrent(e.ctx, "not-a-hash", false); !errors.Is(err, torrent.ErrInvalidInfoHash) {
		t.Errorf("malformed hash: err = %v, want ErrInvalidInfoHash", err)
	}
	if err := e.svc.DeleteTorrent(e.ctx, hashA, true); !errors.Is(err, torrent.ErrTorrentNotFound) {
		t.Errorf("unknown torrent: err = %v, want ErrTorrentNotFound", err)
	}
	if got := e.status(dryRun); got != library.JobReady {
		t.Errorf("a 404 changed the dry run: %s", got)
	}
	if err := e.svc.DeleteTorrent(e.ctx, hashB, false); err != nil {
		t.Errorf("torrent of an open job: %v", err)
	}
	if got := e.status(known); got != library.JobCancelled {
		t.Errorf("open job = %s, want cancelled", got)
	}
	if len(e.client.removals()) != 1 || e.tree.count() != 1 {
		t.Errorf("removals = %v, rebuilds = %d; want only the known torrent's", e.client.removals(), e.tree.count())
	}
}

func TestDeleteMovie(t *testing.T) {
	e := newRemovalEnv(t)
	dune := e.movie("Dune", 2021)
	other := e.movie("Blade Runner", 1982)
	e.assign(library.ItemTypeMovie, dune, hashA, "Dune.mkv", true)
	e.assign(library.ItemTypeMovie, dune, hashB, "Old/Dune.mkv", false) // an earlier torrent
	e.assign(library.ItemTypeMovie, other, hashB, "Old/BR.mkv", true)   // still used
	duneSubs := []string{
		e.subtitle(subtitle.ItemTypeMovie, dune, subtitle.SourceOpenSubtitles, "en", ""),
		e.subtitle(subtitle.ItemTypeMovie, dune, subtitle.SourceHDRezka, "ru", ""),
	}
	e.subtitle(subtitle.ItemTypeMovie, dune, subtitle.SourceTorrent, "fr", hashA)
	stray := filepath.Join(e.dir, "movie", strconv.FormatInt(dune, 10), "old.srt") // no row names it
	e.writeFile(stray)
	otherSub := e.subtitle(subtitle.ItemTypeMovie, other, subtitle.SourceOpenSubtitles, "en", "")
	job := e.seedJob(0, dune, hashC, library.JobNeedsReview, false)
	e.client.loaded[hashA], e.client.loaded[hashB], e.client.loaded[hashC] = true, true, true

	if err := e.svc.DeleteMovie(e.ctx, dune, true); err != nil {
		t.Fatalf("DeleteMovie: %v", err)
	}

	if n := e.count(`SELECT count(*) FROM movies WHERE id = $1`, dune); n != 0 {
		t.Error("the movie is still there")
	}
	if n := e.count(`SELECT count(*) FROM assignment_jobs WHERE id = $1`, job); n != 0 {
		t.Error("the movie's job is still there")
	}
	if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE item_type = 'movie' AND item_id = $1 AND is_active`, dune); n != 0 {
		t.Errorf("%d active rows left", n)
	}
	if got := e.sources(subtitle.ItemTypeMovie, dune); len(got) != 0 {
		t.Errorf("subtitles left: %v", got)
	}
	for _, path := range append(duneSubs, stray, filepath.Dir(duneSubs[0]), filepath.Dir(duneSubs[1])) {
		if exists(path) {
			t.Errorf("%s is still on disk", path)
		}
	}
	if got := e.sources(subtitle.ItemTypeMovie, other); !slices.Equal(got, []string{"opensubtitles"}) || !exists(otherSub) {
		t.Errorf("the other movie's subtitles changed: %v", got)
	}
	// B is still used by the other movie.
	if got := e.client.removals(); !slices.Equal(got, []removal{{hashA, true}, {hashC, true}}) {
		t.Errorf("removals = %v, want the unused torrents with their data", got)
	}
	if e.tree.count() != 1 {
		t.Errorf("tree rebuilds = %d, want 1", e.tree.count())
	}
	// Its downloads cascaded away: the manager drops their pins at once.
	if e.downloads.count() != 1 {
		t.Errorf("download manager wakes = %d, want 1", e.downloads.count())
	}

	if err := e.svc.DeleteMovie(e.ctx, dune, false); !errors.Is(err, library.ErrMovieNotFound) {
		t.Errorf("second delete: err = %v, want ErrMovieNotFound", err)
	}
}

func TestDeleteShow(t *testing.T) {
	e := newRemovalEnv(t)
	showID, eps := e.show("Frieren", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {0, 1, "Special"}})
	otherShow, otherEps := e.show("Dungeon Meshi", [][3]any{{1, 1, "One"}})
	e.assign(library.ItemTypeEpisode, eps["S01E01"], hashA, "Pack/E01-E02.mkv", true)
	e.assign(library.ItemTypeEpisode, eps["S01E02"], hashA, "Pack/E01-E02.mkv", true)
	e.assignHDRezka(library.ItemTypeEpisode, eps["S00E01"])
	e.assign(library.ItemTypeEpisode, eps["S00E01"], hashB, "Old/SP.mkv", false)
	e.assign(library.ItemTypeEpisode, otherEps["S01E01"], hashB, "Other/E01.mkv", true)
	e.subtitle(subtitle.ItemTypeEpisode, eps["S01E01"], subtitle.SourceTorrent, "en", hashA)
	files := []string{
		e.subtitle(subtitle.ItemTypeEpisode, eps["S01E02"], subtitle.SourceOpenSubtitles, "en", ""),
		e.subtitle(subtitle.ItemTypeEpisode, eps["S00E01"], subtitle.SourceHDRezka, "ru", ""),
	}
	otherSub := e.subtitle(subtitle.ItemTypeEpisode, otherEps["S01E01"], subtitle.SourceOpenSubtitles, "en", "")
	e.seedJob(showID, 0, hashA, library.JobApplied, false)
	e.client.loaded[hashA], e.client.loaded[hashB] = true, true

	if err := e.svc.DeleteShow(e.ctx, showID, false); err != nil {
		t.Fatalf("DeleteShow: %v", err)
	}

	if n := e.count(`SELECT count(*) FROM shows WHERE id = $1`, showID); n != 0 {
		t.Error("the show is still there")
	}
	if n := e.count(`SELECT count(*) FROM assignment_jobs WHERE show_id = $1`, showID); n != 0 {
		t.Error("the show's jobs are still there")
	}
	for name, id := range eps {
		if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE item_type = 'episode' AND item_id = $1 AND is_active`, id); n != 0 {
			t.Errorf("%s: %d active rows left", name, n)
		}
		if got := e.sources(subtitle.ItemTypeEpisode, id); len(got) != 0 {
			t.Errorf("%s: subtitles left: %v", name, got)
		}
	}
	for _, path := range files {
		if exists(path) || exists(filepath.Dir(path)) {
			t.Errorf("%s (or its folder) is still on disk", path)
		}
	}
	if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE item_id = $1 AND is_active`, otherEps["S01E01"]); n != 1 || !exists(otherSub) {
		t.Error("the other show changed")
	}
	if n := e.count(`SELECT count(*) FROM shows WHERE id = $1`, otherShow); n != 1 {
		t.Error("the other show is gone")
	}
	// B is still used by the other show; A goes, its data kept.
	if got := e.client.removals(); !slices.Equal(got, []removal{{hashA, false}}) {
		t.Errorf("removals = %v, want only the unused torrent, data kept", got)
	}

	if err := e.svc.DeleteShow(e.ctx, showID, false); !errors.Is(err, library.ErrShowNotFound) {
		t.Errorf("second delete: err = %v, want ErrShowNotFound", err)
	}
}

// A release that drops a torrent clears its pause, stored and in the
// client, so it isn't paused when it's assigned again; a torrent still in
// use keeps its pause.
func TestRelease_ClearsPauses(t *testing.T) {
	e := newRemovalEnv(t)
	settings := library.NewSettingsRepository(e.db)
	dune := e.movie("Dune", 2021)
	other := e.movie("Blade Runner", 1982)
	_, eps := e.show("Frieren", [][3]any{{1, 1, "One"}})
	e.assign(library.ItemTypeMovie, dune, hashA, "Dune.mkv", true)
	e.assign(library.ItemTypeMovie, dune, hashB, "Shared/Dune.mkv", false)
	e.assign(library.ItemTypeMovie, other, hashB, "Shared/BR.mkv", true) // B stays in use
	e.assign(library.ItemTypeEpisode, eps["S01E01"], hashC, "Frieren/E01.mkv", true)
	e.client.loaded[hashA], e.client.loaded[hashB], e.client.loaded[hashC] = true, true, true
	at := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	if err := settings.PauseTorrents(e.ctx, map[string]time.Time{hashA: at, hashB: at, hashC: at}); err != nil {
		t.Fatal(err)
	}
	stored := func() []string {
		t.Helper()
		pauses, err := settings.TorrentPauses(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Sorted(maps.Keys(pauses))
	}

	// A title delete releases A (dropped: pause cleared) but not B (in use).
	if err := e.svc.DeleteMovie(e.ctx, dune, false); err != nil {
		t.Fatal(err)
	}
	if got := stored(); !slices.Equal(got, []string{hashB, hashC}) {
		t.Errorf("stored pauses after the movie delete = %v, want B's and C's", got)
	}
	if !slices.Equal(e.client.resumed, []string{hashA}) {
		t.Errorf("client resumes = %v, want A's pause cleared", e.client.resumed)
	}

	// Unassigning the last episode on C releases it too.
	if err := e.svc.UnassignEpisode(e.ctx, eps["S01E01"]); err != nil {
		t.Fatal(err)
	}
	if got := stored(); !slices.Equal(got, []string{hashB}) {
		t.Errorf("stored pauses after the unassign = %v, want only B's", got)
	}
	if !slices.Equal(e.client.resumed, []string{hashA, hashC}) {
		t.Errorf("client resumes = %v, want A's and C's", e.client.resumed)
	}
	if got := e.client.removals(); !slices.Equal(got, []removal{{hashA, false}, {hashC, false}}) {
		t.Errorf("removals = %v", got)
	}
}

func TestUnassignEpisode(t *testing.T) {
	e := newRemovalEnv(t)
	_, eps := e.show("Frieren", [][3]any{{1, 1, "One"}, {1, 2, "Two"}})
	e.assign(library.ItemTypeEpisode, eps["S01E01"], hashA, "Pack/E01-E02.mkv", true)
	e.assign(library.ItemTypeEpisode, eps["S01E02"], hashA, "Pack/E01-E02.mkv", true)
	e.subtitle(subtitle.ItemTypeEpisode, eps["S01E01"], subtitle.SourceTorrent, "en", hashA)
	downloadedSub := e.subtitle(subtitle.ItemTypeEpisode, eps["S01E01"], subtitle.SourceOpenSubtitles, "az", "")
	e.client.loaded[hashA] = true

	// E02 still plays from the torrent: it stays.
	if err := e.svc.UnassignEpisode(e.ctx, eps["S01E01"]); err != nil {
		t.Fatalf("UnassignEpisode: %v", err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a != nil {
		t.Errorf("E01 still has %+v", a)
	}
	if got := e.sources(subtitle.ItemTypeEpisode, eps["S01E01"]); !slices.Equal(got, []string{"opensubtitles"}) || !exists(downloadedSub) {
		t.Errorf("E01 subtitles = %v, want the downloaded one only", got)
	}
	if got := e.client.removals(); len(got) != 0 {
		t.Errorf("removals = %v; the torrent is still used", got)
	}

	// The last episode goes: so does the torrent, its data kept.
	if err := e.svc.UnassignEpisode(e.ctx, eps["S01E02"]); err != nil {
		t.Fatalf("UnassignEpisode: %v", err)
	}
	if got := e.client.removals(); !slices.Equal(got, []removal{{hashA, false}}) {
		t.Errorf("removals = %v, want the torrent, data kept", got)
	}
	if e.tree.count() != 2 {
		t.Errorf("tree rebuilds = %d, want 2", e.tree.count())
	}
	if n := e.count(`SELECT count(*) FROM torrent_assignments WHERE info_hash = $1`, hashA); n != 2 {
		t.Errorf("%d rows, want both kept inactive", n)
	}

	if err := e.svc.UnassignEpisode(e.ctx, 1<<40); !errors.Is(err, library.ErrEpisodeNotFound) {
		t.Errorf("unknown episode: err = %v, want ErrEpisodeNotFound", err)
	}
}

func TestUnassignMovie_HDRezka(t *testing.T) {
	e := newRemovalEnv(t)
	movieID := e.movie("Interstellar", 2014)
	e.assignHDRezka(library.ItemTypeMovie, movieID)
	hdrezka := []string{
		e.subtitle(subtitle.ItemTypeMovie, movieID, subtitle.SourceHDRezka, "en", ""),
		e.subtitle(subtitle.ItemTypeMovie, movieID, subtitle.SourceHDRezka, "ru", ""),
	}
	downloadedSub := e.subtitle(subtitle.ItemTypeMovie, movieID, subtitle.SourceOpenSubtitles, "az", "")

	if err := e.svc.UnassignMovie(e.ctx, movieID); err != nil {
		t.Fatalf("UnassignMovie: %v", err)
	}
	if a := e.active(library.ItemTypeMovie, movieID); a != nil {
		t.Errorf("the movie still has %+v", a)
	}
	if got := e.sources(subtitle.ItemTypeMovie, movieID); !slices.Equal(got, []string{"opensubtitles"}) || !exists(downloadedSub) {
		t.Errorf("subtitles = %v, want the downloaded one only", got)
	}
	for _, path := range hdrezka {
		if exists(path) {
			t.Errorf("%s is still on disk", path)
		}
	}
	if exists(filepath.Dir(hdrezka[0])) {
		t.Error("the empty HDRezka folder is still there")
	}
	if got := e.client.removals(); len(got) != 0 {
		t.Errorf("removals = %v; an HDRezka stream has no torrent", got)
	}
	if e.tree.count() != 1 {
		t.Errorf("tree rebuilds = %d, want 1", e.tree.count())
	}

	if err := e.svc.UnassignMovie(e.ctx, 1<<40); !errors.Is(err, library.ErrMovieNotFound) {
		t.Errorf("unknown movie: err = %v, want ErrMovieNotFound", err)
	}
}
