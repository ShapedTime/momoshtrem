package service

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// TMDB titles that aren't in the library: a show with a special and two
// episodes (TMDB episode ids 9000 + n), and a film.
const (
	outsideShowTMDB  = 1_400_000_001
	outsideMovieTMDB = 1_400_000_002
)

func outsideTMDB() *fakeTitleTMDB {
	return &fakeTitleTMDB{
		shows: map[int]*tmdb.ShowDetails{outsideShowTMDB: {
			ID: outsideShowTMDB, Name: "Far Show", FirstAirDate: "2021-04-01", PosterPath: "/far.jpg", Status: "Ended",
			Seasons: []tmdb.SeasonSummary{{SeasonNumber: 0}, {SeasonNumber: 1}},
		}},
		seasons: map[int]*tmdb.Season{
			0: {Name: "Specials", Episodes: []tmdb.Episode{{ID: 9000, EpisodeNumber: 1, Name: "Holiday Special", AirDate: "2021-12-24"}}},
			1: {Name: "Season 1", Episodes: []tmdb.Episode{
				{ID: 9001, EpisodeNumber: 1, Name: "Arrival", AirDate: "2021-04-01"},
				{ID: 9002, EpisodeNumber: 2, Name: "Departure", AirDate: "2021-04-08"},
			}},
		},
		movies: map[int]*tmdb.MovieDetails{outsideMovieTMDB: {
			ID: outsideMovieTMDB, Title: "Inception", ReleaseDate: "2010-07-15", Runtime: 148, PosterPath: "/inception.jpg",
		}},
	}
}

// inspect creates a dry run and runs it as a worker would.
func (e *jobEnv) inspect(req InspectRequest) int64 {
	e.t.Helper()
	id, created, err := e.svc.Inspect(e.ctx, req)
	if err != nil || !created {
		e.t.Fatalf("inspect = %v, %v", created, err)
	}
	e.run(id)
	return id
}

// assignments counts the active assignment rows.
func (e *jobEnv) assignments() int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT count(*) FROM torrent_assignments WHERE is_active`).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// A library show's dry run takes the normal pipeline (the stale metadata
// is refreshed, the AI answers what the rules can't) but never applies,
// even when everything is high. It's read-only; Discard drops its
// torrent.
func TestInspect_LibraryShow(t *testing.T) {
	hash := strings.Repeat("1a", 20)
	root := "Near.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root, file("Near.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Something Strange.mkv", gb)))
	showID, eps := e.show("Near Show", [][3]any{{0, 1, "The Mystery Special"}, {1, 1, "One"}})
	if _, err := e.db.Exec(`UPDATE shows SET metadata_updated_at = NULL WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	season := 0
	resolver := &fakeResolver{out: identify.ResolveOutput{Files: []identify.AIFile{{
		Path: root + "/Something Strange.mkv", Kind: identify.KindSpecial, Season: &season, Episodes: []int{1},
		Confidence: identify.ConfidenceHigh, Reason: "The special by its title",
	}}}}
	refresher := &fakeRefresher{db: e.db}
	e.svc = e.service(JobDeps{Resolver: resolver, Metadata: refresher, TMDB: outsideTMDB()})

	id := e.inspect(InspectRequest{MediaType: library.JobTV, TMDBID: e.tmdbID("shows", showID), MagnetURI: magnet(hash), Indexer: "Nyaa",
		Hint: &library.JobHint{Season: &season}})
	d := e.detail(id)
	j := d.Job
	if j.Status != library.JobReady || !j.DryRun || j.AppliedAt != nil || j.Item.ID == nil || *j.Item.ID != showID ||
		j.Item.Title != "Near Show" || !j.AIAvailable || *j.Torrent.Indexer != "Nyaa" || j.Hint == nil || *j.Hint.Season != 0 {
		t.Fatalf("job = %+v", j)
	}
	if len(resolver.calls) != 1 || refresher.calls != 1 {
		t.Errorf("AI runs %d, refreshes %d; want 1 and 1", len(resolver.calls), refresher.calls)
	}
	if bonus := proposalByPath(d, "Something Strange.mkv"); bonus.Group != "specials" || bonus.Source != "ai" ||
		len(bonus.Targets) != 1 || bonus.Targets[0].EpisodeID != eps["S00E01"] {
		t.Errorf("bonus = %+v", bonus)
	}
	if n := e.assignments(); n != 0 || e.tree.count() != 0 {
		t.Errorf("a dry run wrote %d rows and rebuilt the tree %d times", n, e.tree.count())
	}

	// Read-only: PATCH, re-check and Apply say dry_run.
	accept := library.DecisionAccepted
	if _, err := e.svc.PatchProposals(e.ctx, id, []ProposalChange{{ID: d.Proposals[0].ID, Decision: &accept}}); conflictCode(err) != CodeDryRun {
		t.Errorf("patch: %v", err)
	}
	if err := e.svc.Reclassify(e.ctx, id, ReclassifyRequest{Scope: RecheckAll}); conflictCode(err) != CodeDryRun {
		t.Errorf("reclassify: %v", err)
	}
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodeDryRun {
		t.Errorf("apply: %v", err)
	}
	if jobs, err := e.svc.List(e.ctx, nil); err != nil || len(jobs) != 0 {
		t.Errorf("open jobs = %+v, %v (dry runs aren't listed)", jobs, err)
	}

	// Discard cancels it and drops the torrent nothing else uses.
	if d, err := e.svc.Discard(e.ctx, id); err != nil || d.Job.Status != library.JobCancelled {
		t.Fatalf("discard = %+v, %v", d, err)
	}
	if got := e.torrents.removals(); !slices.Equal(got, []string{hash}) {
		t.Errorf("removed %q, want the torrent", got)
	}
}

// A show outside the library is classified against TMDB by the rules
// alone: the targets carry TMDB episode ids, the item and the show's size
// come from TMDB, and nothing is refreshed, asked or applied.
func TestInspect_ShowOutsideLibrary(t *testing.T) {
	hash, single := strings.Repeat("2b", 20), strings.Repeat("2c", 20)
	root := "Far.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(single, "Far.Show.S01E01.720p", file("Far.Show.S01E01.720p.mkv", gb)), info(hash, root,
		file("Far.Show.S01E01.1080p.WEB-GRP.mkv", gb),
		file("Far.Show.S01E02.1080p.WEB-GRP.mkv", gb),
		file("Far.Show.S00E01.Holiday.Special.1080p.WEB-GRP.mkv", gb),
		file("Something Strange.mkv", gb),
	))
	resolver := &fakeResolver{}
	refresher := &fakeRefresher{db: e.db}
	e.svc = e.service(JobDeps{Resolver: resolver, Metadata: refresher, TMDB: outsideTMDB()})

	id, created, err := e.svc.Inspect(e.ctx, InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB, MagnetURI: magnet(hash)})
	if err != nil || !created {
		t.Fatal(created, err)
	}
	queued := e.detail(id).Job
	if queued.Item.ID != nil || queued.Item.Title != "Far Show" || queued.Progress.Seasons != nil {
		t.Errorf("queued = %+v", queued)
	}
	e.run(id)

	d := e.detail(id)
	j := d.Job
	if j.Status != library.JobReady || !j.DryRun || j.Item.ID != nil || j.Item.MediaType != "tv" || j.Item.TMDBID != outsideShowTMDB ||
		j.Item.Title != "Far Show" || *j.Item.Year != 2021 || *j.Item.PosterPath != "/far.jpg" || j.AIAvailable ||
		*j.Progress.Seasons != 2 || *j.Progress.Episodes != 3 {
		t.Fatalf("job = %+v\nitem %+v progress %+v", j, j.Item, j.Progress)
	}
	if want := (JobCounts{Files: 4, ToApply: 3, Pending: 0, Matched: 2, Specials: 1, Unmatched: 1}); j.Counts != want {
		t.Errorf("counts = %+v, want %+v", j.Counts, want)
	}
	if len(resolver.calls) != 0 || refresher.calls != 0 {
		t.Errorf("AI runs %d, refreshes %d; want none", len(resolver.calls), refresher.calls)
	}
	wantTargets := map[string]ProposalTarget{
		"S01E01.1080p.WEB-GRP.mkv":                 {EpisodeID: 9001, Season: 1, Episode: 1, Name: "Arrival"},
		"S01E02.1080p.WEB-GRP.mkv":                 {EpisodeID: 9002, Season: 1, Episode: 2, Name: "Departure"},
		"S00E01.Holiday.Special.1080p.WEB-GRP.mkv": {EpisodeID: 9000, Season: 0, Episode: 1, Name: "Holiday Special"},
	}
	for suffix, want := range wantTargets {
		p := proposalByPath(d, suffix)
		if p == nil || len(p.Targets) != 1 || p.Targets[0].EpisodeID != want.EpisodeID || p.Targets[0].Season != want.Season ||
			p.Targets[0].Episode != want.Episode || p.Targets[0].Name != want.Name || p.Replaces != nil {
			t.Errorf("%s = %+v", suffix, p)
		}
	}
	if p := proposalByPath(d, "S01E02.1080p.WEB-GRP.mkv"); p.Targets[0].AbsoluteNumber == nil || *p.Targets[0].AbsoluteNumber != 2 {
		t.Errorf("E02's absolute number = %v", p.Targets[0].AbsoluteNumber)
	}
	if d.Proposals[0].Group != "unmatched" || d.Proposals[1].Group != "specials" {
		t.Errorf("order = %s, %s", d.Proposals[0].Group, d.Proposals[1].Group)
	}
	if n := e.assignments(); n != 0 {
		t.Errorf("a dry run wrote %d rows", n)
	}
	accept := library.DecisionAccepted
	if _, err := e.svc.PatchProposals(e.ctx, id, []ProposalChange{{ID: d.Proposals[0].ID, Decision: &accept}}); conflictCode(err) != CodeDryRun {
		t.Errorf("patch: %v", err)
	}
	down := e.service(JobDeps{TMDB: &fakeTitleTMDB{err: errors.New("TMDB is down")}})
	if _, err := down.Detail(e.ctx, id); !errors.Is(err, library.ErrTMDBFetch) {
		t.Errorf("detail without TMDB: %v", err)
	}

	// TMDB fails after the details were read: the dry run fails with
	// tmdb_unavailable, and its torrent is dropped.
	broken := outsideTMDB()
	broken.seasonErr = map[int]error{1: errors.New("TMDB is down")}
	e.svc = e.service(JobDeps{TMDB: broken})
	failed := e.inspect(InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB, MagnetURI: magnet(single)})
	if got := e.job(failed); got.Status != library.JobFailed || got.ErrorCode != FailureTMDB {
		t.Errorf("failed = %s %q", got.Status, got.ErrorCode)
	}
	if got := e.torrents.removals(); !slices.Equal(got, []string{single}) {
		t.Errorf("removed %q, want the failed run's torrent", got)
	}
}

// A film's dry run ranks its files as a movie job does, in the library and
// outside it, and never applies the one plausible file.
func TestInspect_Movies(t *testing.T) {
	interstellar, inception := strings.Repeat("3c", 20), strings.Repeat("3d", 20)
	e := newJobEnv(t,
		info(interstellar, "Interstellar.2014.2160p.WEB-DL-GRP", file("Interstellar.2014.2160p.WEB-DL-GRP.mkv", 20*gb)),
		info(inception, "Inception.2010.1080p.BluRay.x264",
			file("Inception.2010.1080p.BluRay.x264.mkv", 14*gb), file("Sample/inception.sample.mkv", 50<<20)))
	movieID := e.movie("Interstellar", 2014)
	e.svc = e.service(JobDeps{Resolver: &fakeResolver{}, TMDB: outsideTMDB()})

	tests := []struct {
		name   string
		tmdbID int
		hash   string
		itemID *int64
		title  string
		year   int
		poster string
		film   string
	}{
		{"in the library", e.tmdbID("movies", movieID), interstellar, &movieID, "Interstellar", 2014, "", "Interstellar.2014.2160p.WEB-DL-GRP.mkv"},
		{"outside the library", outsideMovieTMDB, inception, nil, "Inception", 2010, "/inception.jpg", "Inception.2010.1080p.BluRay.x264.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := e.inspect(InspectRequest{MediaType: library.JobMovie, TMDBID: tt.tmdbID, MagnetURI: magnet(tt.hash)})
			d := e.detail(id)
			j := d.Job
			if j.Status != library.JobReady || !j.DryRun || !equalPtr(j.Item.ID, tt.itemID) || j.Item.Title != tt.title ||
				*j.Item.Year != tt.year || (tt.poster != "" && *j.Item.PosterPath != tt.poster) || j.AIAvailable || j.Progress.Seasons != nil {
				t.Fatalf("job = %+v (item %+v)", j, j.Item)
			}
			if p := proposalByPath(d, tt.film); p == nil || p.Kind != "movie" || p.Group != "matched" || p.Decision != "accepted" {
				t.Errorf("film = %+v", p)
			}
			if n := e.assignments(); n != 0 {
				t.Errorf("a dry run wrote %d rows", n)
			}
		})
	}
}

func equalPtr(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// Inspecting the same title and torrent within the hour answers the same
// dry run, unless it was cancelled or failed; an hour later, or once the
// title is in the library, it's a new one.
func TestInspect_Reuse(t *testing.T) {
	hash, unknown := strings.Repeat("4d", 20), strings.Repeat("4e", 20)
	e := newJobEnv(t, info(hash, "Far.Show.S01", file("Far.Show.S01E01.mkv", gb)))
	e.svc = e.service(JobDeps{TMDB: outsideTMDB()})
	req := InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB, MagnetURI: magnet(hash)}

	first := e.inspect(req)
	again, created, err := e.svc.Inspect(e.ctx, InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB,
		MagnetURI: magnet(strings.ToUpper(hash)) + "&dn=Other.Name"})
	if err != nil || created || again != first {
		t.Fatalf("again = %d %v %v, want %d", again, created, err, first)
	}

	// Another title: another dry run.
	if movie, created, err := e.svc.Inspect(e.ctx, InspectRequest{MediaType: library.JobMovie, TMDBID: outsideMovieTMDB, MagnetURI: magnet(hash)}); err != nil ||
		!created || movie == first {
		t.Errorf("another title = %d %v %v", movie, created, err)
	}

	// Cancelled: a new one.
	if _, err := e.svc.Discard(e.ctx, first); err != nil {
		t.Fatal(err)
	}
	second, created, err := e.svc.Inspect(e.ctx, req)
	if err != nil || !created || second == first {
		t.Fatalf("after a cancel = %d %v %v", second, created, err)
	}

	// Over an hour old: a new one.
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET created_at = NOW() - interval '61 minutes' WHERE id = $1`, second); err != nil {
		t.Fatal(err)
	}
	third, created, err := e.svc.Inspect(e.ctx, req)
	if err != nil || !created || third == second {
		t.Fatalf("an hour later = %d %v %v", third, created, err)
	}

	// The show joins the library: its dry runs have TMDB ids, so a new one.
	var showID int64
	if err := e.db.QueryRow(`INSERT INTO shows (tmdb_id, title, year, metadata_updated_at) VALUES ($1, 'Far Show', 2021, NOW()) RETURNING id`,
		outsideShowTMDB).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	inLibrary, created, err := e.svc.Inspect(e.ctx, req)
	if err != nil || !created || inLibrary == third || e.job(inLibrary).ShowID == nil || *e.job(inLibrary).ShowID != showID {
		t.Fatalf("in the library = %d %v %v", inLibrary, created, err)
	}

	// Failed: a new one.
	failReq := InspectRequest{MediaType: library.JobMovie, TMDBID: outsideMovieTMDB, MagnetURI: magnet(unknown)}
	failed := e.inspect(failReq)
	if e.job(failed).Status != library.JobFailed {
		t.Fatalf("unknown torrent = %s", e.job(failed).Status)
	}
	if next, created, err := e.svc.Inspect(e.ctx, failReq); err != nil || !created || next == failed {
		t.Errorf("after a failure = %d %v %v", next, created, err)
	}
}

// Bad requests and unknown titles fail before a job exists.
func TestInspect_Errors(t *testing.T) {
	e := newJobEnv(t)
	e.svc = e.service(JobDeps{TMDB: outsideTMDB()})
	minus := -1
	tests := []struct {
		name string
		req  InspectRequest
		want error
	}{
		{"no media type", InspectRequest{TMDBID: outsideShowTMDB, MagnetURI: magnet(strings.Repeat("a", 40))}, ErrInvalidInspect},
		{"no TMDB id", InspectRequest{MediaType: library.JobTV, MagnetURI: magnet(strings.Repeat("a", 40))}, ErrInvalidInspect},
		{"a negative season", InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB, MagnetURI: magnet(strings.Repeat("a", 40)),
			Hint: &library.JobHint{Season: &minus}}, ErrInvalidHint},
		{"no info hash", InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB, MagnetURI: "magnet:?dn=x"}, library.ErrInvalidMagnet},
		{"an unknown show", InspectRequest{MediaType: library.JobTV, TMDBID: 7, MagnetURI: magnet(strings.Repeat("a", 40))}, ErrTitleNotFound},
		{"an unknown film", InspectRequest{MediaType: library.JobMovie, TMDBID: 7, MagnetURI: magnet(strings.Repeat("a", 40))}, ErrTitleNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := e.svc.Inspect(e.ctx, tt.req); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	down := e.service(JobDeps{TMDB: &fakeTitleTMDB{err: errors.New("TMDB is down")}})
	if _, _, err := down.Inspect(e.ctx, InspectRequest{MediaType: library.JobMovie, TMDBID: 7, MagnetURI: magnet(strings.Repeat("a", 40))}); !errors.Is(err, library.ErrTMDBFetch) {
		t.Errorf("TMDB down: %v", err)
	}
	noTMDB := e.service(JobDeps{})
	if _, _, err := noTMDB.Inspect(e.ctx, InspectRequest{MediaType: library.JobTV, TMDBID: 7, MagnetURI: magnet(strings.Repeat("a", 40))}); !errors.Is(err, library.ErrTMDBFetch) ||
		!errors.Is(err, tmdb.ErrNoAPIKey) {
		t.Errorf("no TMDB: %v", err)
	}
	var jobs int
	if err := e.db.QueryRow(`SELECT count(*) FROM assignment_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Errorf("jobs = %d, %v", jobs, err)
	}
}

// The janitor deletes dry runs past a day and drops each one's torrent,
// unless an active assignment or another open job (a newer dry run too)
// uses it.
func TestInspect_JanitorDropsUnusedTorrents(t *testing.T) {
	unused, assigned, queued, inspected := strings.Repeat("5a", 20), strings.Repeat("5b", 20), strings.Repeat("5c", 20), strings.Repeat("5d", 20)
	e := newJobEnv(t)
	e.svc = e.service(JobDeps{TMDB: outsideTMDB()})
	showID, eps := e.show("Janitor Show", [][3]any{{1, 1, "One"}})

	old := func(hash string) {
		t.Helper()
		if _, _, err := e.svc.Inspect(e.ctx, InspectRequest{MediaType: library.JobTV, TMDBID: outsideShowTMDB, MagnetURI: magnet(hash)}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'ready', created_at = NOW() - interval '25 hours' WHERE info_hash = $1 AND dry_run`, hash); err != nil {
			t.Fatal(err)
		}
	}
	for _, hash := range []string{unused, assigned, queued, inspected} {
		old(hash)
	}
	if _, err := e.db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
		VALUES ('episode', $1, $2, $3, 'E01.mkv', 1, TRUE)`, eps["S01E01"], assigned, magnet(assigned)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(queued)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Inspect(e.ctx, InspectRequest{MediaType: library.JobMovie, TMDBID: outsideMovieTMDB, MagnetURI: magnet(inspected)}); err != nil {
		t.Fatal(err)
	}

	e.svc.sweepDryRuns(e.ctx)
	if got := e.torrents.removals(); !slices.Equal(got, []string{unused}) {
		t.Errorf("removed %q, want only the unused torrent", got)
	}
	var left int
	if err := e.db.QueryRow(`SELECT count(*) FROM assignment_jobs WHERE dry_run`).Scan(&left); err != nil || left != 1 {
		t.Errorf("dry runs left = %d, %v (the young one)", left, err)
	}
}

// Auto-apply waits for review when a file would replace one an episode
// already has, from another torrent or HDRezka; a job that replaces
// nothing still applies on its own. The legacy ?wait=true path still
// applies, replacing the file.
func TestAutoApply_WaitsOnReplaces(t *testing.T) {
	first, pack, fresh, legacy := strings.Repeat("6a", 20), strings.Repeat("6b", 20), strings.Repeat("6c", 20), strings.Repeat("6d", 20)
	e := newJobEnv(t,
		info(first, "Swap.Show.S01E01.720p.HDTV-LOL", file("Swap.Show.S01E01.720p.HDTV-LOL.mkv", gb/2)),
		info(pack, "Swap.Show.S01.1080p.WEB-GRP", file("Swap.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Swap.Show.S01E02.1080p.WEB-GRP.mkv", gb)),
		info(fresh, "Swap.Show.S01E03.1080p.WEB-GRP", file("Swap.Show.S01E03.1080p.WEB-GRP.mkv", gb)),
		info(legacy, "Swap.Show.S01.2160p.WEB-GRP", file("Swap.Show.S01E01.2160p.WEB-GRP.mkv", gb), file("Swap.Show.S01E02.2160p.WEB-GRP.mkv", gb)),
	)
	showID, eps := e.show("Swap Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}, {1, 4, "Four"}})

	// Nothing to replace: E01 applies on its own.
	if id := e.assignShow(showID, first, nil); e.job(id).Status != library.JobApplied {
		t.Fatalf("first = %s", e.job(id).Status)
	}

	// The pack would replace E01's torrent file: it waits, ready.
	id := e.assignShow(showID, pack, nil)
	d := e.detail(id)
	if d.Job.Status != library.JobReady {
		t.Fatalf("pack = %s", d.Job.Status)
	}
	if p := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv"); p.Replaces == nil || p.Replaces.ReleaseName != "Swap.Show.S01E01.720p.HDTV-LOL" {
		t.Errorf("E01 replaces = %+v", p.Replaces)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a == nil || a.InfoHash != first {
		t.Errorf("E01 = %+v, want the first torrent still", a)
	}
	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}

	// E03 plays from HDRezka: a torrent for it waits too.
	stream := &library.StreamRef{Provider: "hdrezka", Path: "/series/1-swap.html", PostID: 1, TranslatorID: 56,
		TranslatorName: "LostFilm", Season: 1, Episode: 3, Quality: "1080p"}
	if err := library.NewAssignmentRepository(e.db).WriteStream(e.ctx, library.AssignmentWrite{
		ItemType: library.ItemTypeEpisode, ShowID: showID,
		Rows: []library.TorrentAssignment{{ItemID: eps["S01E03"], Stream: stream, FileSize: gb}},
	}); err != nil {
		t.Fatal(err)
	}
	id = e.assignShow(showID, fresh, nil)
	d = e.detail(id)
	if d.Job.Status != library.JobReady || d.Proposals[0].Replaces == nil || d.Proposals[0].Replaces.ReleaseName != "HDRezka · LostFilm · 1080p" {
		t.Fatalf("over HDRezka = %s, replaces %+v", d.Job.Status, d.Proposals[0].Replaces)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E03"]); a == nil || a.IsTorrent() {
		t.Errorf("E03 = %+v, want the stream still", a)
	}
	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}

	// The legacy path applies explicitly, replacing E01's file.
	e.start()
	res, err := e.svc.AssignShowAndWait(e.ctx, showID, AssignRequest{MagnetURI: magnet(legacy)})
	if err != nil {
		t.Fatal(err)
	}
	if e.job(res.JobID).Status != library.JobApplied || res.Summary.Matched != 2 {
		t.Errorf("legacy = %s %+v", e.job(res.JobID).Status, res.Summary)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a == nil || a.InfoHash != legacy {
		t.Errorf("E01 = %+v, want the legacy torrent", a)
	}
}
