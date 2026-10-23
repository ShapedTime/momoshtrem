package runs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
)

func picksOf(n int) []library.RecHeroPickRow {
	out := make([]library.RecHeroPickRow, n)
	for i := range out {
		out[i] = library.RecHeroPickRow{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: i}, Kicker: "k", Reason: "r"}
	}
	return out
}

func ids(picks []library.RecHeroPickRow) []int {
	out := []int{}
	for _, p := range picks {
		out = append(out, p.TMDBID)
	}
	return out
}

// Three hero picks a server-local day: the n-th day after the run starts
// at 3n mod count and wraps, skipping negative feedback.
func TestDayPicks(t *testing.T) {
	finished := time.Date(2026, 10, 16, 3, 20, 0, 0, time.UTC)
	none := func(library.TitleRef) bool { return false }
	for _, tt := range []struct {
		name  string
		count int
		now   time.Time
		skip  func(library.TitleRef) bool
		want  []int
	}{
		{"its own day", 7, finished.Add(20 * time.Hour), none, []int{0, 1, 2}},
		{"the next day", 7, time.Date(2026, 10, 17, 0, 1, 0, 0, time.UTC), none, []int{3, 4, 5}},
		{"wrapping", 7, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), none, []int{6, 0, 1}},
		{"a week on", 7, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), none, []int{0, 1, 2}},
		{"skipping feedback", 7, time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC),
			func(r library.TitleRef) bool { return r.TMDBID == 4 }, []int{3, 5, 6}},
		{"fewer left", 2, finished, none, []int{0, 1}},
		{"all turned down", 3, finished, func(library.TitleRef) bool { return true }, []int{}},
		{"none", 0, finished, none, []int{}},
		{"a clock behind", 7, finished.Add(-48 * time.Hour), none, []int{0, 1, 2}},
	} {
		if got := ids(DayPicks(picksOf(tt.count), tt.skip, finished, tt.now)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// The day is the server's: across a DST change, still one day per date.
func TestDaysBetween_DST(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	// Clocks go back on 25 October 2026: that day has 25 hours.
	finished := time.Date(2026, 10, 25, 0, 30, 0, 0, berlin)
	if n := daysBetween(finished, time.Date(2026, 10, 25, 23, 50, 0, 0, berlin)); n != 0 {
		t.Errorf("same day = %d", n)
	}
	if n := daysBetween(finished.UTC(), time.Date(2026, 10, 26, 0, 10, 0, 0, berlin)); n != 1 {
		t.Errorf("next day = %d", n)
	}
}

// fakeReadStore holds member 1's runs; every other member has none.
type fakeReadStore struct {
	active, succeeded, finished *library.RecRun
	shelves                     []library.RecShelfRow
	picks                       []library.RecHeroPickRow
}

// of is run, if it's memberID's.
func of(memberID int64, run *library.RecRun) *library.RecRun {
	if run == nil || run.MemberID != memberID {
		return nil
	}
	return run
}

func (f *fakeReadStore) ActiveRecRun(_ context.Context, memberID int64) (*library.RecRun, error) {
	return of(memberID, f.active), nil
}
func (f *fakeReadStore) LatestSucceededRecRun(_ context.Context, memberID int64) (*library.RecRun, error) {
	return of(memberID, f.succeeded), nil
}
func (f *fakeReadStore) LatestFinishedRecRun(_ context.Context, memberID int64) (*library.RecRun, error) {
	return of(memberID, f.finished), nil
}
func (f *fakeReadStore) GetRecRun(_ context.Context, id int64) (*library.RecRun, error) {
	if f.succeeded != nil && f.succeeded.ID == id {
		return f.succeeded, nil
	}
	return nil, nil
}
func (f *fakeReadStore) ListRecRuns(_ context.Context, memberID int64, _ int) ([]library.RecRun, error) {
	if run := of(memberID, f.succeeded); run != nil {
		return []library.RecRun{*run}, nil
	}
	return nil, nil
}
func (f *fakeReadStore) RecRunShelves(context.Context, int64) ([]library.RecShelfRow, error) {
	return f.shelves, nil
}
func (f *fakeReadStore) RecRunHeroPicks(context.Context, int64) ([]library.RecHeroPickRow, error) {
	return f.picks, nil
}
func (f *fakeReadStore) RecommendationReason(_ context.Context, memberID int64, _ library.TitleRef) (*string, error) {
	if memberID != 1 {
		return nil, nil
	}
	s := "A reason"
	return &s, nil
}
func (f *fakeReadStore) RecRunCostMeans(context.Context, int) (map[string]float64, error) {
	return map[string]float64{"claude-opus-5-5": 0.41, "claude-haiku-5-5": 0.01}, nil
}

type fakeLibrary struct{ asked library.TMDBIDs }

func (f *fakeLibrary) LibraryStates(_ context.Context, ids library.TMDBIDs) (map[catalog.TitleRef]*catalog.SimilarLibrary, error) {
	f.asked = ids
	return map[catalog.TitleRef]*catalog.SimilarLibrary{
		{MediaType: "tv", TMDBID: 2}: {LibraryID: 9, AvailabilityStage: "ready"},
	}, nil
}

// /home: empty without a key or with picks off; else the member's newest
// succeeded run, their feedback applied (a turned-down title left out, a
// shelf left empty dropped, a like shown), the watchlist and library
// joined, and the day's hero picks. Another member sees none of it.
func TestHome(t *testing.T) {
	finished := now.Add(-time.Hour)
	started := now.Add(-2 * time.Hour)
	succeeded := &library.RecRun{ID: 5, MemberID: 1, Kind: "scheduled", Status: "succeeded", Model: "claude-opus-5-5", StartedAt: &started,
		FinishedAt: &finished, CostUSD: 0.4, Progress: json.RawMessage(`{"steps":[]}`)}
	active := &library.RecRun{ID: 6, MemberID: 1, Kind: "manual", Status: "running", Model: "claude-opus-5-5", StartedAt: &started,
		Progress: json.RawMessage(`{"steps":[{"step":"history","state":"active","detail":null}],"shelves_done":0,"shelves_target":5}`)}
	store := &fakeReadStore{
		succeeded: succeeded, active: active,
		shelves: []library.RecShelfRow{
			{Slug: "a", Kind: "because", Title: "A", Items: []library.RecItemRow{
				{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1}, Reason: "one", Meta: json.RawMessage(`{"title":"One","year":2020}`)},
				{TitleRef: library.TitleRef{MediaType: "tv", TMDBID: 2}, Reason: "two", Meta: json.RawMessage(`{"title":"Two"}`)},
				{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 3}, Reason: "three", Meta: json.RawMessage(`{"title":"Three"}`)},
			}},
			{Slug: "b", Kind: "gems", Title: "B", Items: []library.RecItemRow{
				{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 4}, Reason: "four"},
			}},
		},
		picks: []library.RecHeroPickRow{
			{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 3}, Kicker: "k3", Reason: "r3"},
			{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1}, Kicker: "k1", Reason: "r1"},
			{TitleRef: library.TitleRef{MediaType: "tv", TMDBID: 2}, Kicker: "k2", Reason: "r2"},
			{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 5}, Kicker: "k5", Reason: "r5"},
		},
	}
	titles := &fakeTitles{feedback: map[library.TitleRef]string{
		{MediaType: "movie", TMDBID: 3}: recommend.SignalSeen,
		{MediaType: "movie", TMDBID: 4}: recommend.SignalNotInterested,
		{MediaType: "movie", TMDBID: 1}: recommend.SignalLike,
	}}
	lib := &fakeLibrary{}
	enabled := library.DefaultAISettings()
	var asked []int64 // whose title states
	reader := func(configured bool, settings library.AISettings) *Reader {
		return NewReader(ReaderDeps{Store: store, Settings: fakeSettings{settings}, Library: lib,
			Titles: func(memberID int64) Titles {
				asked = append(asked, memberID)
				return titles
			},
			Configured: configured, Now: func() time.Time { return now }})
	}
	ctx := context.Background()

	home, err := reader(false, enabled).Home(ctx, 1)
	if err != nil || home.Configured || home.Enabled || home.Run != nil || home.ActiveRun != nil || len(home.Shelves) != 0 {
		t.Errorf("without a key = %+v, %v", home, err)
	}
	off := enabled
	off.RecEnabled = false
	home, err = reader(true, off).Home(ctx, 1)
	if err != nil || !home.Configured || home.Enabled || home.Run != nil || home.ActiveRun != nil || home.HeroPicks == nil {
		t.Errorf("picks off = %+v, %v", home, err)
	}
	if reason, _ := reader(true, off).RecommendationReason(ctx, 1, "movie", 1); reason != nil {
		t.Errorf("reason while off = %v", *reason)
	}

	// Another member: no picks of theirs yet.
	home, err = reader(true, enabled).Home(ctx, 2)
	if err != nil || !home.Enabled || home.Run != nil || home.ActiveRun != nil || len(home.Shelves) != 0 || len(home.HeroPicks) != 0 {
		t.Errorf("member 2's home = %+v, %v; want none of member 1's picks", home, err)
	}

	home, err = reader(true, enabled).Home(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, []int64{1}) {
		t.Errorf("title states asked for members %v, want member 1's", asked)
	}
	if !home.Enabled || home.Run.ID != 5 || home.Run.SkipReason != nil || home.ActiveRun.ID != 6 ||
		home.ActiveRun.Steps[0].Step != "history" || home.ActiveRun.ShelvesTarget != 5 {
		t.Errorf("home = %+v", home)
	}
	if len(home.Shelves) != 1 || len(home.Shelves[0].Items) != 2 {
		t.Fatalf("shelves = %+v", home.Shelves)
	}
	one, two := home.Shelves[0].Items[0], home.Shelves[0].Items[1]
	if one.Title != "One" || *one.Year != 2020 || one.Feedback == nil || *one.Feedback != "like" || one.Library != nil {
		t.Errorf("item one = %+v", one)
	}
	if two.Library == nil || two.Library.LibraryID != 9 || two.Feedback != nil {
		t.Errorf("item two = %+v", two)
	}
	var heroes []int
	for _, p := range home.HeroPicks {
		heroes = append(heroes, p.TMDBID)
	}
	if !reflect.DeepEqual(heroes, []int{1, 2, 5}) || *home.HeroPicks[0].Feedback != "like" {
		t.Errorf("hero picks = %+v", home.HeroPicks)
	}
	if !reflect.DeepEqual(lib.asked, library.TMDBIDs{Movies: []int{1, 3, 4, 5}, Shows: []int{2}}) {
		t.Errorf("library asked for %+v", lib.asked)
	}

	est, err := reader(true, enabled).RunEstimates(ctx)
	if err != nil || len(est) != 2 || *est["claude-opus-5-5"] != 0.41 || est["claude-sonnet-5-5"] != nil {
		t.Errorf("estimates = %v, %v", est, err)
	}
}

// A run, the runs and the reason are the member's own: another member's
// run is not found (#245).
func TestReader_Ownership(t *testing.T) {
	finished := now.Add(-time.Hour)
	store := &fakeReadStore{succeeded: &library.RecRun{ID: 5, MemberID: 1, Kind: "scheduled", Status: "succeeded",
		Model: "claude-opus-5-5", FinishedAt: &finished}}
	store.finished = store.succeeded
	r := NewReader(ReaderDeps{Store: store, Settings: fakeSettings{library.DefaultAISettings()}, Configured: true,
		Now: func() time.Time { return now }})
	ctx := context.Background()

	if run, err := r.Run(ctx, 1, 5); err != nil || run.ID != 5 {
		t.Errorf("member 1's run = %+v, %v", run, err)
	}
	if run, err := r.Run(ctx, 2, 5); !errors.Is(err, recommend.ErrNotFound) || run != nil {
		t.Errorf("member 2 reading member 1's run = %+v, %v; want recommend.ErrNotFound", run, err)
	}
	if run, err := r.Run(ctx, 1, 404); !errors.Is(err, recommend.ErrNotFound) || run != nil {
		t.Errorf("an unknown run = %+v, %v; want recommend.ErrNotFound", run, err)
	}
	for memberID, want := range map[int64]int{1: 1, 2: 0} {
		if list, err := r.Runs(ctx, memberID, 10); err != nil || len(list) != want {
			t.Errorf("member %d's runs = %+v, %v; want %d", memberID, list, err, want)
		}
		last, err := r.LastFinished(ctx, memberID)
		if err != nil || (last != nil) != (want == 1) {
			t.Errorf("member %d's last finished = %+v, %v", memberID, last, err)
		}
		reason, err := r.RecommendationReason(ctx, memberID, "movie", 1)
		if err != nil || (reason != nil) != (want == 1) {
			t.Errorf("member %d's reason = %v, %v", memberID, reason, err)
		}
	}
}

func TestSummaryOf(t *testing.T) {
	skip, msg := "no_history", "stale error"
	if s := SummaryOf(&library.RecRun{Status: "succeeded", SkipReason: &skip, Error: &msg}); s.SkipReason != nil || s.Error != nil {
		t.Errorf("a succeeded run's summary = %+v", s)
	}
	if s := SummaryOf(&library.RecRun{Status: "failed", Error: &msg}); s.Error == nil {
		t.Errorf("a failed run's summary = %+v", s)
	}
	p := ProgressOf(&library.RecRun{Status: "queued"})
	if p.Steps == nil || p.ShelvesTarget != 0 {
		t.Errorf("a queued run's progress = %+v", p)
	}
}
