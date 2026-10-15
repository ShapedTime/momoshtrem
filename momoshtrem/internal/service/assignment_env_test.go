package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Fakes and a harness for the job tests. Each test gets a private schema:
// the workers claim any queued job, so jobs mustn't be shared with tests
// of other packages running at the same time.

// fakeTorrents answers AddTorrent from a fixed set of torrents by info
// hash (an unknown one times out) and records removals.
type fakeTorrents struct {
	mu       sync.Mutex
	torrents map[string]*torrent.TorrentInfo
	adds     int
	removed  []string
}

func newFakeTorrents(infos ...*torrent.TorrentInfo) *fakeTorrents {
	f := &fakeTorrents{torrents: map[string]*torrent.TorrentInfo{}}
	for _, info := range infos {
		f.torrents[info.InfoHash] = info
	}
	return f
}

func (f *fakeTorrents) AddTorrent(magnetURI string) (*torrent.TorrentInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds++
	info := f.torrents[normalizeInfoHash(torrent.ExtractInfoHash(magnetURI))]
	if info == nil {
		return nil, torrent.ErrMetadataTimeout
	}
	return info, nil
}

func (f *fakeTorrents) RemoveTorrent(infoHash string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, infoHash)
	return nil
}

func (f *fakeTorrents) removals() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.removed...)
}

// countingTree counts full rebuilds.
type countingTree struct {
	mu sync.Mutex
	n  int
}

func (c *countingTree) InvalidateTree() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *countingTree) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// recordingSubtitles records the torrent subtitles created.
type recordingSubtitles struct {
	mu   sync.Mutex
	subs []*subtitle.Subtitle
}

func (r *recordingSubtitles) CreateTorrentSubtitle(_ context.Context, sub *subtitle.Subtitle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs = append(r.subs, sub)
	return nil
}

func (r *recordingSubtitles) all() []*subtitle.Subtitle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*subtitle.Subtitle(nil), r.subs...)
}

// fakeRefresher records metadata refreshes; it stamps the show as synced.
type fakeRefresher struct {
	db    *library.DB
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeRefresher) RefreshShow(ctx context.Context, showID int64) (*metadata.ShowResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	_, err := f.db.ExecContext(ctx, `UPDATE shows SET metadata_updated_at = NOW() WHERE id = $1`, showID)
	return &metadata.ShowResult{}, err
}

// fakeResolver answers every Resolve with out (or err), recording inputs.
type fakeResolver struct {
	mu    sync.Mutex
	out   identify.ResolveOutput
	err   error
	calls []identify.ResolveInput
}

func (f *fakeResolver) Resolve(_ context.Context, in identify.ResolveInput) (identify.ResolveOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	return f.out, f.err
}

// fakeCatalogSource serves fixed TMDB extras.
type fakeCatalogSource struct {
	titles    []tmdb.AlternativeTitle
	summaries []tmdb.EpisodeGroupSummary
	groups    map[string]*tmdb.EpisodeGroup
}

func (f *fakeCatalogSource) ShowAlternativeTitles(context.Context, int) ([]tmdb.AlternativeTitle, error) {
	if f.titles == nil {
		return nil, tmdb.ErrNotFound
	}
	return f.titles, nil
}

func (f *fakeCatalogSource) EpisodeGroups(context.Context, int) ([]tmdb.EpisodeGroupSummary, error) {
	return f.summaries, nil
}

func (f *fakeCatalogSource) EpisodeGroup(_ context.Context, id string) (*tmdb.EpisodeGroup, error) {
	if g := f.groups[id]; g != nil {
		return g, nil
	}
	return nil, tmdb.ErrNotFound
}

// openSchemaDB opens the test database in a private, freshly migrated
// schema; it skips without MOMOSHTREM_TEST_DATABASE_URL.
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
	schema := fmt.Sprintf("jobs_svc_%d", time.Now().UnixNano())
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

// jobEnv is a JobService over a private schema with fakes around it.
type jobEnv struct {
	t        *testing.T
	ctx      context.Context
	db       *library.DB
	jobs     *library.AssignmentJobRepository
	svc      *JobService
	torrents *fakeTorrents
	tree     *countingTree
	subs     *recordingSubtitles
}

func newJobEnv(t *testing.T, infos ...*torrent.TorrentInfo) *jobEnv {
	t.Helper()
	db := openSchemaDB(t)
	e := &jobEnv{
		t: t, ctx: context.Background(), db: db,
		jobs:     library.NewAssignmentJobRepository(db),
		torrents: newFakeTorrents(infos...),
		tree:     &countingTree{},
		subs:     &recordingSubtitles{},
	}
	e.svc = e.service(JobDeps{})
	return e
}

// service builds a JobService over the env's database and fakes; d may
// add a resolver, a refresher or a TMDB source.
func (e *jobEnv) service(d JobDeps) *JobService {
	d.Jobs = e.jobs
	d.Shows = library.NewShowRepository(e.db)
	d.Movies = library.NewMovieRepository(e.db)
	d.Assignments = library.NewAssignmentRepository(e.db)
	d.Settings = library.NewSettingsRepository(e.db)
	d.Torrents, d.Tree, d.Subtitles = e.torrents, e.tree, e.subs
	s := NewJobService(d)
	s.pollInterval = 5 * time.Millisecond
	return s
}

// start runs the workers until the test ends.
func (e *jobEnv) start() {
	e.svc.Start()
	e.t.Cleanup(e.svc.Close)
}

// run claims the job and processes it as a worker would.
func (e *jobEnv) run(id int64) {
	e.t.Helper()
	res, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'fetching_metadata' WHERE id = $1 AND status = 'queued'`, id)
	if err != nil {
		e.t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		e.t.Fatalf("job %d isn't queued", id)
	}
	job, err := e.jobs.Get(e.ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	e.svc.process(e.ctx, job)
}

// assign queues a job for the torrent on a show and runs it.
func (e *jobEnv) assignShow(showID int64, hash string, hint *library.JobHint) int64 {
	e.t.Helper()
	id, created, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(hash), Hint: hint})
	if err != nil || !created {
		e.t.Fatalf("assign = %v, %v", created, err)
	}
	e.run(id)
	return id
}

func (e *jobEnv) detail(id int64) *JobDetail {
	e.t.Helper()
	d, err := e.svc.Detail(e.ctx, id)
	if err != nil {
		e.t.Fatalf("detail %d: %v", id, err)
	}
	return d
}

// decide changes proposals as PATCH will (slice 4): decisions by file
// path suffix, then the status is recomputed.
func (e *jobEnv) decide(id int64, decisions map[string]library.Decision) {
	e.t.Helper()
	err := e.jobs.Locked(e.ctx, id, func(tx *library.JobTx) error {
		var changed []*library.AssignmentProposal
		for _, p := range tx.Proposals {
			for suffix, d := range decisions {
				if strings.HasSuffix(p.FilePath, suffix) {
					next := *p
					next.Decision, next.UserDecided = d, d != p.Group.DefaultDecision()
					changed = append(changed, &next)
				}
			}
		}
		if err := tx.UpdateProposals(changed); err != nil {
			return err
		}
		tx.Job.Status = settledStatus(listed(tx.Proposals))
		return tx.SaveJob()
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// setAI stores the AI settings document.
func (e *jobEnv) setAI(doc string) {
	e.t.Helper()
	if err := library.NewSettingsRepository(e.db).Set(e.ctx, library.AISettingsKey, []byte(doc)); err != nil {
		e.t.Fatal(err)
	}
}

// show seeds a show with episodes ({season, episode, name}) and returns
// its id and the episode ids by "SxxEyy". Its metadata is fresh, so jobs
// don't refresh it.
func (e *jobEnv) show(title string, episodes [][3]any) (int64, map[string]int64) {
	e.t.Helper()
	var showID int64
	if err := e.db.QueryRow(`INSERT INTO shows (tmdb_id, title, year, metadata_updated_at) VALUES ($1, $2, 2020, NOW()) RETURNING id`,
		uniqueTMDB(), title).Scan(&showID); err != nil {
		e.t.Fatal(err)
	}
	seasons := map[int]int64{}
	ids := map[string]int64{}
	for _, ep := range episodes {
		season, number, name := ep[0].(int), ep[1].(int), ep[2].(string)
		seasonID, ok := seasons[season]
		if !ok {
			if err := e.db.QueryRow(`INSERT INTO seasons (show_id, season_number) VALUES ($1, $2) RETURNING id`, showID, season).Scan(&seasonID); err != nil {
				e.t.Fatal(err)
			}
			seasons[season] = seasonID
		}
		var id int64
		if err := e.db.QueryRow(`INSERT INTO episodes (season_id, episode_number, name, air_date) VALUES ($1, $2, $3, '2020-01-01') RETURNING id`,
			seasonID, number, name).Scan(&id); err != nil {
			e.t.Fatal(err)
		}
		ids[se(season, number)] = id
	}
	return showID, ids
}

func (e *jobEnv) movie(title string, year int) int64 {
	e.t.Helper()
	var id int64
	if err := e.db.QueryRow(`INSERT INTO movies (tmdb_id, title, year) VALUES ($1, $2, $3) RETURNING id`, uniqueTMDB(), title, year).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// active returns the episode's (or movie's) active assignment.
func (e *jobEnv) active(itemType library.ItemType, id int64) *library.TorrentAssignment {
	e.t.Helper()
	a, err := library.NewAssignmentRepository(e.db).GetActiveForItem(itemType, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

func (e *jobEnv) job(id int64) *library.AssignmentJob {
	e.t.Helper()
	j, err := e.jobs.Get(e.ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return j
}

var tmdbSeq struct {
	sync.Mutex
	n int
}

func uniqueTMDB() int {
	tmdbSeq.Lock()
	defer tmdbSeq.Unlock()
	tmdbSeq.n++
	return 1_500_000_000 + int(time.Now().UnixNano()%10_000_000)*10 + tmdbSeq.n%10
}

func se(season, episode int) string { return fmt.Sprintf("S%02dE%02d", season, episode) }

func magnet(hash string) string { return "magnet:?xt=urn:btih:" + hash }

// info is a torrent named after its root folder.
func info(hash, root string, files ...identify.TorrentFile) *torrent.TorrentInfo {
	for i := range files {
		files[i].Path = root + "/" + files[i].Path
	}
	return &torrent.TorrentInfo{InfoHash: hash, Name: root, Files: files}
}

func file(path string, size int64) identify.TorrentFile {
	return identify.TorrentFile{Path: path, Size: size}
}

func conflictCode(err error) string {
	var c *JobConflictError
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}

func proposalByPath(d *JobDetail, suffix string) *ProposalView {
	for i := range d.Proposals {
		if strings.HasSuffix(d.Proposals[i].FilePath, suffix) {
			return &d.Proposals[i]
		}
	}
	return nil
}
