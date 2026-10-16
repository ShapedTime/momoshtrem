package library

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// GET /library's counts, summary, ids lookup and the title page fields it
// carries, in a schema of its own so the summary covers exactly what's seeded.
func TestListLibrary(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	arrival := seed.movie("Arrival") // ready, with a runtime
	exec(`UPDATE movies SET runtime = 116 WHERE id = $1`, arrival.ID)
	seed.file(ItemTypeMovie, arrival.ID, "hash-arrival", "Arrival.2016/Arrival.mkv", 5000)
	blade := seed.movie("Blade Runner") // needs a torrent
	coco := seed.movie("Coco")          // downloading
	seed.file(ItemTypeMovie, coco.ID, "hash-coco", "Coco.mkv", 3000)

	// Dark: E1–E2 have files; E3–E4 aired, E5 has no date, E6 aired: the
	// first run is E3–E4. Absolute numbers 100 + episode. A mapping job
	// waits for review.
	dark := seed.show("Dark", true)
	d1 := seed.episodes(dark, 1, aired, aired, aired, aired, "", aired)
	exec(`UPDATE episodes SET absolute_number = 100 + episode_number WHERE season_id = (SELECT season_id FROM episodes WHERE id = $1)`, d1[1])
	seed.file(ItemTypeEpisode, d1[1], "hash-dark", "Dark/E01.mkv", 100)
	seed.file(ItemTypeEpisode, d1[2], "hash-dark", "Dark/E02.mkv", 100)
	darkJob := seedJob(t, db, newShowJob(dark, strings.Repeat("d", 40)), JobNeedsReview, []ProposalDraft{
		draft("Pack/E03.mkv", GroupNeedsReview, DecisionPending, d1[3]),
	})

	// Empire: its only file is a special, and specials are hidden: the VFS
	// still lists it, but it needs a torrent.
	empire := seed.show("Empire", false)
	seed.episodes(empire, 1, aired)
	seed.file(ItemTypeEpisode, seed.episodes(empire, 0, aired)[1], "hash-empire", "SP01.mkv", 50)

	// Fargo: season 1 has files; season 2 aired and the run stops at the
	// season's end (S2E1–E2), though season 3 aired too.
	fargo := seed.show("Fargo", true)
	f1 := seed.episodes(fargo, 1, aired, aired)
	seed.file(ItemTypeEpisode, f1[1], "hash-fargo", "S01E01.mkv", 10)
	seed.file(ItemTypeEpisode, f1[2], "hash-fargo", "S01E02.mkv", 10)
	seed.episodes(fargo, 2, aired, aired)
	seed.episodes(fargo, 3, aired)

	// Gotham: a two-episode file and another, both being fetched.
	gotham := seed.show("Gotham", true)
	g1 := seed.episodes(gotham, 1, aired, aired, aired)
	seed.file(ItemTypeEpisode, g1[1], "hash-gotham", "E01E02.mkv", 400)
	seed.file(ItemTypeEpisode, g1[2], "hash-gotham", "E01E02.mkv", 400)
	seed.file(ItemTypeEpisode, g1[3], "hash-gotham", "E03.mkv", 200)

	query := LibraryQuery{Today: today, Sort: LibrarySortTitle, Downloading: []FileRef{
		{InfoHash: "hash-coco", Path: "Coco.mkv"},
		{InfoHash: "hash-gotham", Path: "E01E02.mkv"},
		{InfoHash: "hash-gotham", Path: "E03.mkv"},
	}}
	listing, err := repo.ListLibrary(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]LibraryItem{}
	for _, item := range listing.Items {
		byTitle[item.Title] = item
	}
	if len(listing.Items) != 7 || listing.Total != 7 {
		t.Fatalf("items = %d, total %d, want 7", len(listing.Items), listing.Total)
	}

	wantCounts := LibraryCounts{All: 7, Movies: 3, Shows: 4, Ready: 1, Incomplete: 6, NeedsReview: 1}
	if listing.Counts != wantCounts {
		t.Errorf("counts = %+v, want %+v", listing.Counts, wantCounts)
	}
	wantSummary := LibrarySummary{Total: 7, SizeBytes: 5000 + 3000 + 200 + 20 + 600, Ready: 1} // hidden specials count for nothing
	if listing.Summary != wantSummary {
		t.Errorf("summary = %+v, want %+v", listing.Summary, wantSummary)
	}

	t.Run("fields", func(t *testing.T) {
		a := byTitle["Arrival"]
		if a.Runtime != 116 || a.Resolution != "1080p" || a.FilePath != "Arrival.2016/Arrival.mkv" || !a.HasFile ||
			a.Downloading != nil || a.Missing != nil || a.ReviewJobID != nil || a.Stage != "ready" {
			t.Errorf("Arrival = %+v", a)
		}
		if b := byTitle["Blade Runner"]; b.HasFile || b.FilePath != "" || b.Resolution != "" || b.Runtime != 0 || b.Stage != "needs_torrent" {
			t.Errorf("Blade Runner = %+v", b)
		}
		if c := byTitle["Coco"]; c.Stage != "downloading" || !reflect.DeepEqual(c.Downloading, []FileRef{{InfoHash: "hash-coco", Path: "Coco.mkv"}}) {
			t.Errorf("Coco = %+v", c)
		}

		d := byTitle["Dark"]
		wantRun := &FrontierRun{SeasonNumber: 1, From: 3, To: 4, AbsoluteFrom: intPtr(103), AbsoluteTo: intPtr(104), Count: 2}
		if !reflect.DeepEqual(d.Missing, wantRun) || d.ReviewJobID == nil || *d.ReviewJobID != darkJob.ID || d.NeedsReview != 1 ||
			!d.HasFile || d.Downloading != nil || d.Runtime != 0 || d.FilePath != "" {
			t.Errorf("Dark = %+v, missing %+v", d, d.Missing)
		}
		if e := byTitle["Empire"]; !e.HasFile || e.Stage != "needs_torrent" || e.SizeBytes != 0 ||
			!reflect.DeepEqual(e.Missing, &FrontierRun{SeasonNumber: 1, From: 1, To: 1, Count: 1}) {
			t.Errorf("Empire = %+v, missing %+v", e, e.Missing)
		}
		if f := byTitle["Fargo"]; !reflect.DeepEqual(f.Missing, &FrontierRun{SeasonNumber: 2, From: 1, To: 2, Count: 2}) {
			t.Errorf("Fargo missing = %+v", f.Missing)
		}
		g := byTitle["Gotham"]
		wantFiles := []FileRef{{InfoHash: "hash-gotham", Path: "E01E02.mkv"}, {InfoHash: "hash-gotham", Path: "E03.mkv"}}
		if g.Stage != "downloading" || g.Missing != nil || g.SizeBytes != 600 || !reflect.DeepEqual(g.Downloading, wantFiles) {
			t.Errorf("Gotham = %+v", g)
		}
	})

	t.Run("counts honour the search and ids, never the type or status", func(t *testing.T) {
		q := query
		q.MediaType, q.Status, q.Search = "tv", LibraryStatusIncomplete, "a" // Arrival, Blade Runner, Dark, Fargo, Gotham
		l, err := repo.ListLibrary(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		want := LibraryCounts{All: 5, Movies: 2, Shows: 3, Ready: 1, Incomplete: 4, NeedsReview: 1}
		if l.Counts != want || l.Summary != wantSummary || l.Total != 3 || len(l.Items) != 3 {
			t.Errorf("counts %+v (want %+v), summary %+v, total %d", l.Counts, want, l.Summary, l.Total)
		}

		q = query
		q.IDs = &TMDBIDs{Movies: []int{arrival.TMDBID, dark.TMDBID}, Shows: []int{dark.TMDBID, 1}}
		l, err = repo.ListLibrary(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if titles := titlesOf(l.Items); !reflect.DeepEqual(titles, []string{"Arrival", "Dark"}) || l.Total != 2 ||
			l.Counts != (LibraryCounts{All: 2, Movies: 1, Shows: 1, Ready: 1, Incomplete: 1, NeedsReview: 1}) || l.Summary != wantSummary {
			t.Errorf("ids = %v, total %d, counts %+v, summary %+v", titles, l.Total, l.Counts, l.Summary)
		}
		q.MediaType = "movie" // ANDed with the other filters
		if l, err := repo.ListLibrary(ctx, q); err != nil || !reflect.DeepEqual(titlesOf(l.Items), []string{"Arrival"}) {
			t.Errorf("ids and type = %v, %v", titlesOf(l.Items), err)
		}
	})

	t.Run("an empty page still has the counts and summary", func(t *testing.T) {
		for _, q := range []LibraryQuery{
			{Today: today, Downloading: query.Downloading, Status: LibraryStatusWatching},
			{Today: today, Downloading: query.Downloading, Search: "nothing like this"},
			{Today: today, Downloading: query.Downloading, Limit: 5, Offset: 50},
		} {
			l, err := repo.ListLibrary(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(l.Items) != 0 || l.Items == nil || l.Summary != wantSummary || (q.Search == "" && l.Counts != wantCounts) {
				t.Errorf("%+v: items %v, counts %+v, summary %+v", q, l.Items, l.Counts, l.Summary)
			}
		}
		if l, _ := repo.ListLibrary(ctx, LibraryQuery{Today: today, Limit: 5, Offset: 50}); l.Total != 7 {
			t.Errorf("past the end: total = %d, want 7", l.Total)
		}
	})

	_ = blade
}

func titlesOf(items []LibraryItem) []string {
	titles := []string{}
	for _, item := range items {
		titles = append(titles, item.Title)
	}
	return titles
}

func intPtr(n int) *int { return &n }
