package service

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// The service writes through the real repositories: streams become the
// items' active files, a second assign replaces the first, and the picker
// annotation reads them back.
func TestIntegration_HDRezkaAssign(t *testing.T) {
	env := newJobEnv(t)
	showID, eps := env.show("Breaking Bad", [][3]any{{1, 1, "Pilot"}, {1, 2, "Cat's in the Bag..."}, {1, 3, "...And the Bag's in the River"}})
	movieID := env.movie("Interstellar", 2014)

	fakes := newHDRezkaEnv()
	shows := library.NewShowRepository(env.db)
	assignments := library.NewAssignmentRepository(env.db)
	svc := NewHDRezkaService(HDRezkaDeps{
		Site: fakes.site, Resolver: fakes.streams, TMDB: fakes.tmdb,
		Movies: library.NewMovieRepository(env.db), Shows: shows, Assignments: assignments, Tree: env.tree,
	})

	if _, err := svc.AssignMovie(env.ctx, movieID, HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"}); err != nil {
		t.Fatal(err)
	}
	a := env.active(library.ItemTypeMovie, movieID)
	if a == nil || a.Kind != library.AssignmentKindHDRezka || a.IsTorrent() || a.Stream == nil || a.Stream.TranslatorID != 238 ||
		a.Stream.Quality != "1080p" || a.FileSize != 300_000 || a.Resolution != "1080p" || a.FilePath != "hdrezka/2259/t238/movie/1080p.mp4" {
		t.Fatalf("movie file = %+v (stream %+v)", a, a.Stream)
	}

	fakes.streams.errs = map[[2]int]error{{1, 2}: hdrezka.ErrPremium}
	res, err := svc.AssignShow(env.ctx, showID, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "720p"},
		Items: []HDRezkaAssignItem{
			{EpisodeID: eps["S01E01"], Season: 1, Episode: 1},
			{EpisodeID: eps["S01E02"], Season: 1, Episode: 2},
			{EpisodeID: eps["S01E03"], Season: 1, Episode: 3},
		},
	})
	if err != nil || len(res.Assigned) != 2 || len(res.Failed) != 1 || res.Failed[0].Code != HDRezkaPremiumOnly {
		t.Fatalf("assign = %+v, %v", res, err)
	}
	if env.tree.count() != 2 {
		t.Errorf("rebuilds = %d, want one per assign", env.tree.count())
	}
	if a := env.active(library.ItemTypeEpisode, eps["S01E02"]); a != nil {
		t.Errorf("the failed episode has a file: %+v", a)
	}

	// Another quality replaces the stream: one active row.
	if _, err := svc.AssignShow(env.ctx, showID, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "1080p"},
		Items:         []HDRezkaAssignItem{{EpisodeID: eps["S01E01"], Season: 1, Episode: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM torrent_assignments WHERE item_type = 'episode' AND item_id = $1 AND is_active`,
		eps["S01E01"]).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active rows = %d, %v", active, err)
	}

	title, err := svc.Title(env.ctx, HDRezkaTitleRequest{URL: breakingBadPath, TranslatorID: 238, ShowID: showID})
	if err != nil {
		t.Fatal(err)
	}
	e1, e2, e3 := title.Seasons[0].Episodes[0].Library, title.Seasons[0].Episodes[1].Library, title.Seasons[0].Episodes[2].Library
	if e1 == nil || !e1.HasFile || *e1.Source != "hdrezka" || e1.Stream == nil || e1.Stream.Quality != "1080p" ||
		e1.Stream.Translator != "Оригинал (+субтитры)" || e1.Name != "Pilot" {
		t.Errorf("E1 = %+v", e1)
	}
	if e2 == nil || e2.HasFile || e2.Source != nil {
		t.Errorf("E2 = %+v", e2)
	}
	if e3 == nil || e3.Stream == nil || e3.Stream.Quality != "720p" {
		t.Errorf("E3 = %+v", e3)
	}
}
