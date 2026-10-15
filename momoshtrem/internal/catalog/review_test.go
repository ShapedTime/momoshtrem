package catalog

import (
	"context"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// An open mapping job on Frieren: its files under review target E28 (which
// has a file), E29, E30 and the unaired special 2.
func reviewFixture() *fixture {
	f := newFixture()
	job := int64(41)
	f.store.reviews["tv:3"] = &library.ReviewState{JobID: &job, NeedsReview: 3, EpisodeIDs: []int64{3002, 31028, 31029, 31030}}
	return f
}

func TestShowTitle_Review(t *testing.T) {
	f := reviewFixture()
	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	lib := title.Library
	if lib.NeedsReview != 3 || lib.ReviewJobID == nil || *lib.ReviewJobID != 41 {
		t.Errorf("review = %d, job %v", lib.NeedsReview, lib.ReviewJobID)
	}
	// The frontier is about files: E29 and E30 are still in it.
	if lib.AvailabilityStage != StageNeedsTorrent || lib.Missing == nil || lib.Missing.From != 29 || lib.Missing.Count != 10 ||
		*lib.EpisodesTotal != 38 || *lib.EpisodesReady != 28 {
		t.Errorf("availability = %s, missing %+v, %d/%d", lib.AvailabilityStage, lib.Missing, *lib.EpisodesReady, *lib.EpisodesTotal)
	}
	byNumber := map[int]SeasonSummary{}
	for _, s := range title.Seasons {
		byNumber[s.SeasonNumber] = s
	}
	// E28 has a file: it's ready, not under review.
	if s1 := byNumber[1]; s1.Ready != 28 || s1.NeedsReview != 2 || s1.Missing != 8 || s1.Upcoming != 0 {
		t.Errorf("season 1 = %+v", s1)
	}
	// Under review beats upcoming.
	if sp := byNumber[0]; sp.NeedsReview != 1 || sp.Upcoming != 0 || sp.Missing != 1 {
		t.Errorf("specials = %+v", sp)
	}
}

func TestSeason_Review(t *testing.T) {
	f := reviewFixture()
	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	states := map[int]EpisodeState{}
	for _, ep := range season.Episodes {
		states[ep.EpisodeNumber] = ep.State
	}
	if states[27] != EpisodeReady || states[28] != EpisodeDownloading || states[29] != EpisodeNeedsReview ||
		states[30] != EpisodeNeedsReview || states[31] != EpisodeMissing {
		t.Errorf("states = %v", states)
	}
	missing := 0
	for _, r := range season.Ranges {
		missing += r.Missing
	}
	if missing != 8 {
		t.Errorf("ranges missing = %d, want 8 (E29 and E30 are under review): %+v", missing, season.Ranges)
	}
	if f.store.calls["review"] != 1 {
		t.Errorf("review reads = %d, want 1", f.store.calls["review"])
	}
}

func TestMovieTitle_Review(t *testing.T) {
	f := newFixture()
	job := int64(9)
	f.store.reviews["movie:7"] = &library.ReviewState{JobID: &job, NeedsReview: 2}
	title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if title.Library.NeedsReview != 2 || title.Library.ReviewJobID == nil || *title.Library.ReviewJobID != 9 {
		t.Errorf("review = %d, job %v", title.Library.NeedsReview, title.Library.ReviewJobID)
	}

	// A ready job waiting for Apply keeps its id with nothing to review.
	f.store.reviews["movie:7"] = &library.ReviewState{JobID: &job}
	title, _ = f.svc.MovieTitle(context.Background(), duneTMDB)
	if title.Library.NeedsReview != 0 || title.Library.ReviewJobID == nil {
		t.Errorf("ready job: review = %d, job %v", title.Library.NeedsReview, title.Library.ReviewJobID)
	}

	// Not in the library: nothing to read.
	delete(f.store.movies, duneTMDB)
	before := f.store.calls["review"]
	title, _ = f.svc.MovieTitle(context.Background(), duneTMDB)
	if f.store.calls["review"] != before || title.Library.ReviewJobID != nil {
		t.Errorf("not in the library: review reads %d, job %v", f.store.calls["review"]-before, title.Library.ReviewJobID)
	}
}
