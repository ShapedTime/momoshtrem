package service

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// The title pages and /library follow the review (PLAN §6 "Review
// rules"), over the jobs these tests drive.

// noTMDB has nothing: library titles fall back to their stored metadata.
type noTMDB struct{}

func (noTMDB) MovieDetails(context.Context, int) (*tmdb.MovieDetails, error) {
	return nil, tmdb.ErrNotFound
}
func (noTMDB) ShowDetails(context.Context, int) (*tmdb.ShowDetails, error) {
	return nil, tmdb.ErrNotFound
}
func (noTMDB) EpisodeGroups(context.Context, int) ([]tmdb.EpisodeGroupSummary, error) {
	return nil, tmdb.ErrNotFound
}
func (noTMDB) EpisodeGroup(context.Context, string) (*tmdb.EpisodeGroup, error) {
	return nil, tmdb.ErrNotFound
}

func (e *jobEnv) catalog() *catalog.Service {
	return catalog.New(catalog.Deps{Store: library.NewCatalogRepository(e.db), TMDB: noTMDB{}})
}

func (e *jobEnv) tmdbID(table string, id int64) int {
	e.t.Helper()
	var tmdbID int
	if err := e.db.QueryRow(`SELECT tmdb_id FROM `+table+` WHERE id = $1`, id).Scan(&tmdbID); err != nil {
		e.t.Fatal(err)
	}
	return tmdbID
}

// reviewView is what the catalog says about a show's review.
type reviewView struct {
	needsReview  int
	jobID        *int64
	season       catalog.SeasonSummary // season 1
	states       map[int]catalog.EpisodeState
	listed       bool // in /library?status=needs_review
	listedReview int
}

func (e *jobEnv) showReview(cat *catalog.Service, showID int64) reviewView {
	e.t.Helper()
	tmdbID := e.tmdbID("shows", showID)
	title, err := cat.ShowTitle(e.ctx, tmdbID)
	if err != nil {
		e.t.Fatal(err)
	}
	v := reviewView{needsReview: title.Library.NeedsReview, jobID: title.Library.ReviewJobID, states: map[int]catalog.EpisodeState{}}
	for _, s := range title.Seasons {
		if s.SeasonNumber == 1 {
			v.season = s
		}
	}
	season, err := cat.Season(e.ctx, tmdbID, 1)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, ep := range season.Episodes {
		v.states[ep.EpisodeNumber] = ep.State
	}
	page, err := cat.Library(e.ctx, catalog.LibraryParams{Status: catalog.StatusNeedsReview, MediaType: "tv"})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == showID {
			v.listed, v.listedReview = true, item.NeedsReview
		}
	}
	return v
}

func TestReview_CatalogFollowsTheJob(t *testing.T) {
	hash, again := strings.Repeat("c9", 20), strings.Repeat("d0", 20)
	e := newJobEnv(t,
		info(hash, "Follow.Show.S01",
			file("Follow.Show.S01E01.Pilot.mkv", gb),
			file("Follow.Show.S01E02.Two.mkv", gb),
			file("Follow.Show.S01E02.Extended.Cut.mkv", gb)),
		info(again, "Follow.Show.S01.REPACK",
			file("Follow.Show.S01E03.Pilot.mkv", gb),
			file("Follow.Show.S01E03.Extended.Cut.mkv", gb)))
	showID, _ := e.show("Follow Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}, {1, 4, "Four"}})
	cat := e.catalog()

	if v := e.showReview(cat, showID); v.needsReview != 0 || v.jobID != nil || v.listed || v.season.Missing != 4 {
		t.Fatalf("before = %+v", v)
	}

	// Two files on E02: both need a look; E02 is under review, E01 isn't
	// (its file is accepted, not pending).
	id := e.assignShow(showID, hash, nil)
	v := e.showReview(cat, showID)
	if v.needsReview != 2 || v.jobID == nil || *v.jobID != id || !v.listed || v.listedReview != 2 {
		t.Errorf("open job = %+v", v)
	}
	if v.season.NeedsReview != 1 || v.season.Missing != 3 || v.states[2] != catalog.EpisodeNeedsReview || v.states[1] != catalog.EpisodeMissing {
		t.Errorf("season = %+v, states %v", v.season, v.states)
	}

	// One left out: E02's file is still pending.
	d := e.detail(id)
	extended := proposalByPath(d, "E02.Extended.Cut.mkv")
	two := proposalByPath(d, "E02.Two.mkv")
	e.patch(id, change(extended.ID, library.DecisionIgnored))
	if v := e.showReview(cat, showID); v.needsReview != 1 || v.states[2] != catalog.EpisodeNeedsReview || v.listedReview != 1 {
		t.Errorf("one pending = %+v", v)
	}

	// Accepted: the job is ready and waits for Apply; nothing to review.
	e.patch(id, change(two.ID, library.DecisionAccepted))
	v = e.showReview(cat, showID)
	if v.needsReview != 0 || v.jobID == nil || *v.jobID != id || v.listed || v.states[2] != catalog.EpisodeMissing {
		t.Errorf("ready = %+v", v)
	}

	// A re-check doesn't make the banner flicker: resolving still counts.
	e.patch(id, change(two.ID, library.DecisionPending))
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'resolving' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if v := e.showReview(cat, showID); v.needsReview != 1 || v.jobID == nil {
		t.Errorf("resolving = %+v", v)
	}
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'needs_review' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	e.patch(id, change(two.ID, library.DecisionAccepted))

	// Applied: cleared, and the files are there.
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	v = e.showReview(cat, showID)
	if v.needsReview != 0 || v.jobID != nil || v.states[1] != catalog.EpisodeReady || v.states[2] != catalog.EpisodeReady || v.season.NeedsReview != 0 {
		t.Errorf("applied = %+v", v)
	}

	// Another torrent's conflict on E03, then discarded: cleared.
	next := e.assignShow(showID, again, nil)
	if v := e.showReview(cat, showID); v.needsReview != 2 || v.jobID == nil || *v.jobID != next || v.states[3] != catalog.EpisodeNeedsReview {
		t.Errorf("second job = %+v", v)
	}
	if _, err := e.svc.Discard(e.ctx, next); err != nil {
		t.Fatal(err)
	}
	if v := e.showReview(cat, showID); v.needsReview != 0 || v.jobID != nil || v.listed || v.states[3] != catalog.EpisodeMissing {
		t.Errorf("discarded = %+v", v)
	}

	// A dry run never counts.
	dry := e.assignShow(showID, again, nil)
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET dry_run = TRUE WHERE id = $1`, dry); err != nil {
		t.Fatal(err)
	}
	if v := e.showReview(cat, showID); v.needsReview != 0 || v.jobID != nil {
		t.Errorf("dry run = %+v", v)
	}
}

func TestReview_CatalogMovie(t *testing.T) {
	pack := strings.Repeat("e1", 20)
	e := newJobEnv(t, info(pack, "Nolan Pack",
		file("Inception.2010.1080p.BluRay.x264.mkv", 14*gb),
		file("Interstellar.2014.1080p.BluRay.x264.mkv", 12*gb)))
	movieID := e.movie("Interstellar", 2014)
	cat := e.catalog()
	tmdbID := e.tmdbID("movies", movieID)

	id, _, err := e.svc.Assign(e.ctx, library.JobMovie, movieID, AssignRequest{MagnetURI: magnet(pack)})
	if err != nil {
		t.Fatal(err)
	}
	e.run(id)
	title, err := cat.MovieTitle(e.ctx, tmdbID)
	if err != nil {
		t.Fatal(err)
	}
	if title.Library.NeedsReview != 2 || title.Library.ReviewJobID == nil || *title.Library.ReviewJobID != id {
		t.Errorf("candidates = %d, job %v", title.Library.NeedsReview, title.Library.ReviewJobID)
	}
	page, err := cat.Library(e.ctx, catalog.LibraryParams{Status: catalog.StatusNeedsReview})
	if err != nil || page.Total != 1 || page.Items[0].MediaType != "movie" || page.Items[0].NeedsReview != 2 {
		t.Errorf("library = %+v, %v", page, err)
	}

	d := e.detail(id)
	e.patch(id, change(proposalByPath(d, "Interstellar.2014.1080p.BluRay.x264.mkv").ID, library.DecisionAccepted))
	title, _ = cat.MovieTitle(e.ctx, tmdbID)
	if title.Library.NeedsReview != 0 || title.Library.ReviewJobID == nil {
		t.Errorf("picked = %d, job %v", title.Library.NeedsReview, title.Library.ReviewJobID)
	}
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	title, _ = cat.MovieTitle(e.ctx, tmdbID)
	if title.Library.ReviewJobID != nil || title.Library.AvailabilityStage != catalog.StageReady {
		t.Errorf("applied = %+v", title.Library)
	}
}

// A file a job maps to episodes with a gap (E07 and E09) is one entry per
// run in the WebDAV tree, and the catalog's webdav_path agrees.
func TestReview_GroupingSplitsAtGaps(t *testing.T) {
	hash := strings.Repeat("f2", 20)
	root := "Gap.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Gap.Show.S01E01E02.1080p.WEB-GRP.mkv", 2*gb),
		file("Gap.Show.S01E07.1080p.WEB-GRP.mkv", gb),
		file("Gap.Show.S01E07.1080p.WEB-GRP.en.srt", 1<<10),
		file("Gap.Show.S01E08.1080p.WEB-GRP.mkv", gb)))
	var episodes [][3]any
	for n, name := range []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"} {
		episodes = append(episodes, [3]any{1, n + 1, name})
	}
	showID, eps := e.show("Gap Show", episodes)
	fs := vfs.NewLibraryFS(library.NewMovieRepository(e.db), library.NewShowRepository(e.db), library.NewAssignmentRepository(e.db), 0)
	fs.SetSubtitleRepository(e.realSubtitles())
	e.svc.tree = fs
	e.setAI(`{"auto_apply_high_confidence": false}`)
	id := e.assignShow(showID, hash, nil)

	seven := proposalByPath(e.detail(id), "E07.1080p.WEB-GRP.mkv")
	e.patch(id, change(seven.ID, library.DecisionAccepted, eps["S01E07"], eps["S01E09"]))
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}

	const dir = "/TV Shows/Gap Show (2020)/Season 01"
	entries, err := fs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	// The file's subtitle (recorded on E07) sits next to both of its entries.
	want := []string{
		"Gap Show - S01E01-E02 - One.mkv",
		"Gap Show - S01E07 - Seven.en.srt",
		"Gap Show - S01E07 - Seven.mkv",
		"Gap Show - S01E08 - Eight.mkv",
		"Gap Show - S01E09 - Nine.en.srt",
		"Gap Show - S01E09 - Nine.mkv",
	}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Errorf("tree = %q, want %q", names, want)
	}

	season, err := e.catalog().Season(e.ctx, e.tmdbID("shows", showID), 1)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[int]string{}
	for _, ep := range season.Episodes {
		if ep.WebDAVPath != nil {
			paths[ep.EpisodeNumber] = *ep.WebDAVPath
		}
	}
	for n, name := range map[int]string{1: want[0], 2: want[0], 7: want[2], 8: want[3], 9: want[5]} {
		if paths[n] != dir+"/"+name {
			t.Errorf("E%02d webdav_path = %q, want %q", n, paths[n], dir+"/"+name)
		}
	}
}
