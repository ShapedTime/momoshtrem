package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// targetsText writes targets as spanText does, "" for none.
func targetsText(ts []identify.Target) string {
	if len(ts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s (%d)", spanText(ts), len(ts))
}

func TestIngest_EpisodeScope(t *testing.T) {
	tc := frierenContext(t)
	plan := mustPlan(t, tc, Request{Season: intp(1), Episode: intp(29)})
	in := newIngester(tc, plan, testNow)

	tests := []struct {
		name, title string
		id          bool // from the ID query
		keep        bool
		targets     string
		scene       string
	}{
		{"scene numbering: release S02E01 is TMDB S01E29", "Frieren.Beyond.Journeys.End.S02E01.1080p.WEB-DL.x265-GRP", false, true,
			"S01E29 (1)", `Scene "S02E01" = TMDB S01E29`},
		{"absolute number with the romaji title", "[SubsPlease] Sousou no Frieren - 29 (1080p) [ABCD1234].mkv", false, true, "S01E29 (1)", ""},
		{"the short title fansubs use", "[Erai-raws] Frieren - 29 [1080p]", false, true, "S01E29 (1)", ""},
		{"a scene season pack covers the episode", "Frieren Beyond Journeys End S02 1080p WEB-DL", false, true,
			"S01E29–E38 (10)", `Scene "Season 2" = TMDB S01E29–E38`},
		{"an absolute batch", "Sousou no Frieren (29-38) [1080p] Batch", false, true, "S01E29–E38 (10)", ""},
		{"a space-mangled batch of 1–28 doesn't hold 29", "[SubsPlease] Sousou no Frieren (01 28) (1080p) [V2] [Batch]", false, false, "", ""},
		{"a batch that doesn't say what it holds", "[SubsPlease] Sousou no Frieren (1080p) [Batch]", false, false, "", ""},
		{"TMDB's own numbering", "Frieren.S01E29.1080p.WEB", false, true, "S01E29 (1)", ""},
		{"another episode", "Frieren.Beyond.Journeys.End.S01E05.1080p.WEB-DL", false, false, "", ""},
		{"the first cour's season pack", "Frieren Beyond Journeys End S01 1080p", false, false, "", ""},
		{"another show from a text query", "Totally.Other.Show.S01E29.1080p", false, false, "", ""},
		{"another name from the ID query is trusted", "Totally.Other.Show.S01E29.1080p", true, true, "S01E29 (1)", ""},
		{"no episode marker", "Frieren Beyond Journeys End 1080p", false, false, "", ""},
		{"the complete series", "Frieren.Beyond.Journeys.End.Complete.Series.1080p", false, true, "S01E01–E38 (38)", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := textQuery(plan)
			if tt.id {
				q = idQuery(t, plan)
			}
			it, ok := in.ingest(result(tt.title), q)
			if ok != tt.keep {
				t.Fatalf("kept = %v, want %v", ok, tt.keep)
			}
			if !ok {
				return
			}
			if got := targetsText(it.targets); got != tt.targets {
				t.Errorf("targets = %s, want %s", got, tt.targets)
			}
			if it.sceneNote != tt.scene {
				t.Errorf("scene note = %q, want %q", it.sceneNote, tt.scene)
			}
			if !it.coversRequested || it.seasons != 1 {
				t.Errorf("coversRequested = %v, seasons = %d", it.coversRequested, it.seasons)
			}
		})
	}
	if in.showSeasons != 1 {
		t.Errorf("showSeasons = %d, want 1", in.showSeasons)
	}
}

func TestIngest_SeasonScope(t *testing.T) {
	tc := frierenContext(t)
	plan := mustPlan(t, tc, Request{Season: intp(1)})
	in := newIngester(tc, plan, testNow)

	tests := []struct {
		title     string
		keep      bool
		wholeSeas bool // CoversRequested: every aired episode of TMDB S01
	}{
		{"Frieren Beyond Journeys End S02 1080p WEB-DL", true, false}, // E29–E38
		{"Sousou no Frieren (01-38) [1080p] Batch", true, true},
		{"[SubsPlease] Sousou no Frieren (01 28) (1080p) [V2] [Batch]", true, false}, // "(01-28)" with the dash a space
		{"Frieren.S01E01-E03.1080p", true, false},
		{"Frieren.Beyond.Journeys.End.S02E01.1080p", false, false}, // a single episode
		{"Frieren.Beyond.Journeys.End.Specials.1080p", false, false},
		{"Frieren.Beyond.Journeys.End.Complete.Series.1080p", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			it, ok := in.ingest(result(tt.title), textQuery(plan))
			if ok != tt.keep {
				t.Fatalf("kept = %v, want %v", ok, tt.keep)
			}
			if ok && it.coversRequested != tt.wholeSeas {
				t.Errorf("coversRequested = %v, want %v (targets %s)", it.coversRequested, tt.wholeSeas, targetsText(it.targets))
			}
		})
	}
}

func TestIngest_AnyScopeKeepsEverythingNamingTheTitle(t *testing.T) {
	tc := frierenContext(t)
	plan := mustPlan(t, tc, Request{})
	in := newIngester(tc, plan, testNow)
	for _, title := range []string{"Frieren Beyond Journeys End 1080p", "Frieren.S01E05.1080p", "Sousou no Frieren - 28.5 [1080p]"} {
		if _, ok := in.ingest(result(title), textQuery(plan)); !ok {
			t.Errorf("%q was dropped", title)
		}
	}
	if _, ok := in.ingest(result("Other.Show.S01E05.1080p"), textQuery(plan)); ok {
		t.Error("another show was kept")
	}
}

func TestIngest_UserQuerySkipsTheTitleMatchButNotTheScope(t *testing.T) {
	tc := frierenContext(t)
	plan := mustPlan(t, tc, Request{Season: intp(1), Episode: intp(29), Q: "my own words"})
	in := newIngester(tc, plan, testNow)
	if _, ok := in.ingest(result("Totally.Other.Show.S01E29.1080p"), textQuery(plan)); !ok {
		t.Error("with q, a release naming another title was dropped")
	}
	if _, ok := in.ingest(result("Totally.Other.Show.S01E05.1080p"), textQuery(plan)); ok {
		t.Error("with q, a release outside the scope was kept")
	}
}

func TestIngest_TitleMatch(t *testing.T) {
	office := seasons("The Office", 2005, 6, 22)
	office.Countries = []string{"US"}
	officeTC := showContext(office, 2316, "tt0386676", 73244, "")
	rutracker := "Провожающая в последний путь Фрирен / Sousou no Frieren / Frieren: Beyond Journey's End [TV] [1-28 из 28] [2023, WEBRip 1080p]"
	spider := &service.TitleContext{MediaType: library.JobMovie, TMDBID: 569094, Title: "Spider-Man: Across the Spider-Verse", Year: 2023}

	tests := []struct {
		name  string
		tc    *service.TitleContext
		req   Request
		title string
		keep  bool
	}{
		{"a slash-separated name", frierenContext(t), Request{Season: intp(1)}, rutracker, true},
		{"a country tag", officeTC, Request{Season: intp(1), Episode: intp(1)}, "The.Office.US.S01E01.720p.WEB", true},
		{"without the leading The", officeTC, Request{Season: intp(1), Episode: intp(1)}, "Office.S01E01.720p", true},
		{"another country's show", officeTC, Request{Season: intp(1), Episode: intp(1)}, "The.Office.Mumbai.S01E01.720p", false},
		{"the movie", perfectDaysContext(), Request{}, "Perfect Days (2023) [1080p] [YTS.MX]", true},
		{"a year off by one", perfectDaysContext(), Request{}, "Perfect.Days.2024.1080p.WEB-DL", true},
		{"another film of the same name", perfectDaysContext(), Request{}, "Perfect.Days.1996.1080p", false},
		{"a movie result with an episode", perfectDaysContext(), Request{}, "Perfect.Days.S01E01.1080p", false},
		{"hyphens as dots", spider, Request{}, "Spider.Man.Across.The.Spider.Verse.2023.1080p", true},
		{"hyphens kept", spider, Request{}, "Spider-Man.Across.the.Spider-Verse.2023.2160p", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := mustPlan(t, tt.tc, tt.req)
			in := newIngester(tt.tc, plan, testNow)
			if _, ok := in.ingest(result(tt.title), textQuery(plan)); ok != tt.keep {
				t.Errorf("kept = %v, want %v", ok, tt.keep)
			}
		})
	}
}

func TestReleaseIDIsStable(t *testing.T) {
	a := result("Perfect.Days.2023.1080p")
	b := result("Perfect.Days.2023.1080p", seeders(5))
	c := result("Perfect.Days.2023.1080p", indexer("beta", "Beta"))
	if releaseID(a) != releaseID(b) {
		t.Error("the same result has two ids")
	}
	if releaseID(a) == releaseID(c) {
		t.Error("the same GUID on two indexers shares an id")
	}
	if id := releaseID(a); len(id) != 20 || strings.Contains(id, testKey) {
		t.Errorf("id = %q", id)
	}
}

func TestEntryAdd_Dedupe(t *testing.T) {
	tc := perfectDaysContext()
	plan := mustPlan(t, tc, Request{})
	in := newIngester(tc, plan, testNow)
	q := textQuery(plan)

	t.Run("the same info hash keeps the most seeders", func(t *testing.T) {
		e := newEntry(plan, tc, testNow, defaultRunTimeout)
		defer e.cancel()
		e.add(mustIngest(t, in, result("Perfect.Days.2023.1080p.WEB-DL", seeders(10)), q))
		e.add(mustIngest(t, in, result("Perfect Days 2023 1080p WEB-DL", infoHash(hashOf("Perfect.Days.2023.1080p.WEB-DL")), seeders(90), indexer("beta", "Beta")), q))
		e.add(mustIngest(t, in, result("Perfect.Days.2023.1080p.WEB-DL", seeders(40), guid("other")), q))
		if len(e.items) != 1 || e.items[0].seeders != 90 || e.items[0].indexer != "Beta" {
			t.Fatalf("items = %+v", e.items)
		}
	})
	t.Run("the same title and a size within 5% without a hash", func(t *testing.T) {
		e := newEntry(plan, tc, testNow, defaultRunTimeout)
		defer e.cancel()
		e.add(mustIngest(t, in, result("Perfect.Days.2023.720p", noMagnet(), seeders(30)), q))
		// Indexers round sizes: "1.4 GiB" is 1,503,238,553 bytes.
		e.add(mustIngest(t, in, result("Perfect Days 2023 720p", noMagnet(), seeders(20), size(1_503_238_553), indexer("beta", "Beta")), q))
		e.add(mustIngest(t, in, result("Perfect Days 2023 720p", noMagnet(), size(2_000_000_000), guid("big")), q))
		if len(e.items) != 2 || e.items[0].seeders != 30 || e.items[1].size != 2_000_000_000 {
			t.Fatalf("items = %+v", e.items)
		}
	})
	t.Run("two info hashes are two releases, whatever the name", func(t *testing.T) {
		e := newEntry(plan, tc, testNow, defaultRunTimeout)
		defer e.cancel()
		e.add(mustIngest(t, in, result("Perfect.Days.2023.1080p.WEB", infoHash(hashOf("one"))), q))
		e.add(mustIngest(t, in, result("Perfect.Days.2023.1080p.WEB", infoHash(hashOf("two")), guid("two")), q))
		// A copy without a hash joins the closest in size.
		e.add(mustIngest(t, in, result("Perfect Days 2023 1080p WEB", noMagnet(), seeders(500), size(1_510_000_000), guid("three")), q))
		if len(e.items) != 2 || e.items[0].seeders != 500 || e.items[1].seeders != 100 {
			t.Fatalf("items = %+v", e.items)
		}
	})
	t.Run("a copy with a magnet wins a tie and lends its magnet", func(t *testing.T) {
		e := newEntry(plan, tc, testNow, defaultRunTimeout)
		defer e.cancel()
		e.add(mustIngest(t, in, result("Perfect.Days.2023.2160p", noMagnet(), seeders(50)), q))
		e.add(mustIngest(t, in, result("Perfect.Days.2023.2160p", seeders(50), guid("b")), q))
		if len(e.items) != 1 || e.items[0].magnet == "" {
			t.Fatalf("items = %+v", e.items)
		}

		e2 := newEntry(plan, tc, testNow, defaultRunTimeout)
		defer e2.cancel()
		first := mustIngest(t, in, result("Perfect.Days.2023.2160p", noMagnet(), seeders(80)), q)
		e2.add(first)
		id, magnet := e2.add(mustIngest(t, in, result("Perfect.Days.2023.2160p", seeders(5), guid("b")), q))
		it := e2.items[0]
		if len(e2.items) != 1 || it.seeders != 80 || it.magnet == "" || it.infoHash != hashOf("Perfect.Days.2023.2160p") {
			t.Fatalf("items = %+v", e2.items)
		}
		// The release index learns the magnet the release now has.
		if id != first.id || magnet != it.magnet {
			t.Errorf("add = %q, %q; want the kept release's id and magnet", id, magnet)
		}
	})
}
