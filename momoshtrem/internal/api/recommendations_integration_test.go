package api

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
	"github.com/shapedtime/momoshtrem/internal/recommend/schedule"
)

// /recommendations/home, /runs and /runs/{id} end to end over Postgres
// (#157): the newest succeeded run's shelves and the day's hero picks,
// with feedback, the watchlist and library state joined at read time, and
// the active run.
func TestRecommendations_Integration(t *testing.T) {
	db := openRerunDB(t)
	ctx := context.Background()
	repo := library.NewRecRunRepository(db)
	settings := library.NewSettingsRepository(db)
	reader := runs.NewReader(runs.ReaderDeps{
		Store: repo, Settings: settings, Titles: library.NewTasteRepository(db),
		Library:    catalog.New(catalog.Deps{Store: library.NewCatalogRepository(db)}),
		Configured: true,
	})
	s := newTestServer(Options{Recommendations: schedule.New(schedule.Deps{Store: repo, Reader: reader, Settings: settings, Configured: true})})

	// Before the first run: no picks, and every list is [].
	rec := doBody(t, s, "GET", "/api/recommendations/home", "")
	if rec.Code != 200 || rec.Body.String() != `{"configured":true,"enabled":true,"run":null,"active_run":null,"hero_picks":[],"shelves":[]}` {
		t.Fatalf("home before a run = %d %s", rec.Code, rec.Body)
	}

	for _, q := range []string{
		`INSERT INTO movies (tmdb_id, title, year) VALUES (1003, 'Movie 1003', 2024)`,
		`INSERT INTO trakt_watchlist (media_type, trakt_id, tmdb_id, title, year, listed_at) VALUES ('tv', 77, 2001, 'Show 2001', 2022, NOW())`,
		`INSERT INTO rec_feedback (media_type, tmdb_id, signal, source) VALUES ('movie', 1002, 'dislike', 'shelf'), ('movie', 1004, 'like', 'hero'),
			('movie', 1006, 'seen', 'title')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%.50s…: %v", q, err)
		}
	}
	id, _, err := repo.QueueRecRun(ctx, recommend.KindFirstSync, "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StartRecRun(ctx, id, nil); err != nil {
		t.Fatal(err)
	}
	ref := id
	if err := library.NewAIStore(db).RecordUsage(ctx, library.AIUsage{Feature: library.AIFeatureRecs, Model: "claude-opus-5-5",
		InputTokens: 12000, OutputTokens: 3000, CacheReadTokens: 40000, WebSearches: 2, CostUSD: 0.3612, RefID: &ref}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncRecRunUsage(ctx, id); err != nil {
		t.Fatal(err)
	}
	meta := func(title string, year int) json.RawMessage {
		m, _ := json.Marshal(recommend.ItemMeta{Title: title, Year: &year})
		return m
	}
	shelves := []library.RecShelfRow{
		{Slug: "because-severance", Kind: "because", Title: "Because you loved Severance", Items: []library.RecItemRow{
			{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1002}, Reason: "Disliked since", Meta: meta("Movie 1002", 2024)},
			{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1003}, Reason: "In the library", Meta: meta("Movie 1003", 2024)},
			{TitleRef: library.TitleRef{MediaType: "tv", TMDBID: 2001}, Reason: "On the watchlist", Meta: meta("Show 2001", 2022)},
		}},
		{Slug: "gems", Kind: "gems", Title: "Hidden gems", Items: []library.RecItemRow{
			{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1006}, Reason: "Seen since", Meta: meta("Movie 1006", 2020)},
		}},
	}
	picks := []library.RecHeroPickRow{
		{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1004}, Kicker: "Tonight's pick", Reason: "Liked"},
		{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1002}, Kicker: "Turned down", Reason: "Disliked"},
		{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1005}, Kicker: "Second", Reason: "Five"},
		{TitleRef: library.TitleRef{MediaType: "tv", TMDBID: 2001}, Kicker: "Third", Reason: "Show"},
		{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 1007}, Kicker: "Tomorrow", Reason: "Seven"},
	}
	progress := json.RawMessage(`{"steps":[{"step":"history","state":"done","detail":"2,184 plays · 312 ratings"},` +
		`{"step":"write","state":"done","detail":"5 of 5 shelves"}],"shelves_done":5,"shelves_target":5}`)
	if err := repo.PublishRecRun(ctx, id, progress, shelves, picks); err != nil {
		t.Fatal(err)
	}
	active, _, err := repo.QueueRecRun(ctx, recommend.KindManual, "claude-sonnet-5-5")
	if err != nil {
		t.Fatal(err)
	}

	rec = doBody(t, s, "GET", "/api/recommendations/home", "")
	if rec.Code != 200 {
		t.Fatalf("home = %d %s", rec.Code, rec.Body)
	}
	var home recommend.Home
	if err := json.Unmarshal(rec.Body.Bytes(), &home); err != nil {
		t.Fatal(err)
	}
	if home.Run == nil || home.Run.ID != id || home.Run.Kind != "first_sync" || home.Run.CostUSD != 0.3612 ||
		home.Run.StartedAt == nil || home.Run.FinishedAt == nil {
		t.Errorf("run = %+v", home.Run)
	}
	if home.ActiveRun == nil || home.ActiveRun.ID != active || home.ActiveRun.Status != "queued" || home.ActiveRun.StartedAt != nil ||
		home.ActiveRun.ShelvesTarget != 0 {
		t.Errorf("active run = %+v", home.ActiveRun)
	}
	if len(home.Shelves) != 1 || home.Shelves[0].Slug != "because-severance" || len(home.Shelves[0].Items) != 2 {
		t.Fatalf("shelves = %+v (the disliked title and the emptied shelf left out)", home.Shelves)
	}
	inLibrary, watchlisted := home.Shelves[0].Items[0], home.Shelves[0].Items[1]
	if inLibrary.TMDBID != 1003 || inLibrary.Library == nil || inLibrary.Library.LibraryID == 0 || inLibrary.OnWatchlist {
		t.Errorf("library item = %+v", inLibrary)
	}
	if watchlisted.TMDBID != 2001 || !watchlisted.OnWatchlist || watchlisted.Library != nil || watchlisted.Title != "Show 2001" ||
		*watchlisted.Year != 2022 || watchlisted.Reason != "On the watchlist" {
		t.Errorf("watchlist item = %+v", watchlisted)
	}
	var heroes []int
	for _, p := range home.HeroPicks {
		heroes = append(heroes, p.TMDBID)
	}
	if !reflect.DeepEqual(heroes, []int{1004, 1005, 2001}) || home.HeroPicks[0].Feedback == nil || *home.HeroPicks[0].Feedback != "like" {
		t.Errorf("hero picks = %+v", home.HeroPicks)
	}
	if !strings.Contains(rec.Body.String(), `"feedback":null`) || strings.Contains(rec.Body.String(), `"signals"`) {
		t.Errorf("body = %s", rec.Body)
	}

	// The runs, newest first, and one with its usage.
	rec = doBody(t, s, "GET", "/api/recommendations/runs?limit=5", "")
	var list struct {
		Items []recommend.RunSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Items) != 2 || list.Items[0].ID != active || list.Items[1].ID != id {
		t.Errorf("runs = %s", rec.Body)
	}
	rec = doBody(t, s, "GET", "/api/recommendations/runs/"+itoa(id), "")
	var detail recommend.RunDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.Usage.APICalls != 1 || detail.Usage.InputTokens != 12000 ||
		detail.Usage.CacheReadTokens != 40000 || detail.Usage.WebSearches != 2 || detail.Usage.CostUSD != 0.3612 ||
		len(detail.Steps) != 2 || detail.ShelvesDone != 5 || *detail.Steps[0].Detail != "2,184 plays · 312 ratings" {
		t.Errorf("run = %s", rec.Body)
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/runs/99999", ""); rec.Code != 404 {
		t.Errorf("an unknown run = %d", rec.Code)
	}

	// Picks off: /home is empty, configured still true.
	if _, err := settings.UpdateAISettings(ctx, library.AISettingsPatch{RecEnabled: new(bool)}); err != nil {
		t.Fatal(err)
	}
	rec = doBody(t, s, "GET", "/api/recommendations/home", "")
	if rec.Body.String() != `{"configured":true,"enabled":false,"run":null,"active_run":null,"hero_picks":[],"shelves":[]}` {
		t.Errorf("home with picks off = %s", rec.Body)
	}
}
