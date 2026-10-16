package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/quality"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

func TestSearchRequestValidate(t *testing.T) {
	neg := -1
	negGB := -0.5
	tests := []struct {
		name    string
		req     SearchRequest
		wantErr string // "" for valid
	}{
		{"a movie", SearchRequest{MediaType: "movie", TMDBID: 1}, ""},
		{"a movie ignores TV fields", SearchRequest{MediaType: "movie", TMDBID: 1, Episode: intp(3), Scope: "nonsense"}, ""},
		{"no media type", SearchRequest{TMDBID: 1}, "media_type must be movie or tv"},
		{"no tmdb id", SearchRequest{MediaType: "tv"}, "tmdb_id must be a positive number"},
		{"an episode without a season", SearchRequest{MediaType: "tv", TMDBID: 1, Episode: intp(3)}, "an episode needs a season"},
		{"scope episode without an episode", SearchRequest{MediaType: "tv", TMDBID: 1, Season: intp(1), Scope: ScopeEpisode}, "scope episode needs an episode"},
		{"scope season without a season", SearchRequest{MediaType: "tv", TMDBID: 1, Scope: ScopeSeason}, "scope season needs a season"},
		{"an unknown scope", SearchRequest{MediaType: "tv", TMDBID: 1, Scope: "all"}, `unknown scope "all"`},
		{"an unknown sort", SearchRequest{MediaType: "movie", TMDBID: 1, Sort: "rating"}, `unknown sort "rating"`},
		{"negative seeders", SearchRequest{MediaType: "movie", TMDBID: 1, Filters: Filters{MinSeeders: &neg}}, "min_seeders must be 0 or more"},
		{"a negative size", SearchRequest{MediaType: "movie", TMDBID: 1, Filters: Filters{MaxSizeGB: &negGB}}, "max_size_gb must be 0 or more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.req.validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("err = %v", err)
			case tt.wantErr != "" && (!errors.Is(err, ErrBadRequest) || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("err = %v, want ErrBadRequest %q", err, tt.wantErr)
			}
		})
	}

	req := SearchRequest{MediaType: "tv", TMDBID: 1, PageSize: 500}
	if _, err := req.validate(); err != nil || req.Page != 1 || req.PageSize != MaxPageSize || req.Sort != SortBest {
		t.Errorf("defaults: %v, page %d, page_size %d, sort %q", err, req.Page, req.PageSize, req.Sort)
	}
}

// snapshotOf is a completed search of tc that found results (all from
// the primary text query).
func snapshotOf(t *testing.T, tc *service.TitleContext, req Request, results ...torznab.Result) snapshot {
	t.Helper()
	plan := mustPlan(t, tc, req)
	in := newIngester(tc, plan, testNow)
	e := newEntry(plan, tc, testNow, defaultRunTimeout)
	defer e.cancel()
	e.showSeasons = in.showSeasons
	for _, r := range results {
		if it, ok := in.ingest(r, textQuery(plan)); ok {
			e.add(it)
		}
	}
	e.complete = true
	return e.snapshot(testNow)
}

func readOf(t *testing.T, snap snapshot, tc *service.TitleContext, req SearchRequest, profile library.QualityProfile) *SearchResponse {
	t.Helper()
	req.MediaType, req.TMDBID = string(tc.MediaType), tc.TMDBID
	if _, err := req.validate(); err != nil {
		t.Fatal(err)
	}
	return read(snap, tc, req, profile, testNow, nil)
}

func bucketsText(bs []FacetBucket) string {
	parts := make([]string, len(bs))
	for i, b := range bs {
		parts[i] = fmt.Sprintf("%s:%d", b.Value, b.Count)
	}
	return strings.Join(parts, " ")
}

func titles(rs []Release) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.RawTitle
	}
	return out
}

const (
	pdUHD    = "Perfect.Days.2023.2160p.WEB-DL.DDP5.1.DV.HDR.H.265-FLUX"
	pdBluRay = "Perfect.Days.2023.1080p.BluRay.x264.DTS-HD.MA.5.1-GRP"
	pdWEB    = "Perfect.Days.2023.1080p.WEB-DL.DDP5.1.H.264-FLUX"
	pdSmall  = "Perfect.Days.2023.720p.WEBRip.x264.AAC-YTS"
	pdCam    = "Perfect.Days.2023.HDCAM.x264"
)

// perfectDaysSnapshot: four visible releases on two indexers, and a CAM
// the default profile hides.
func perfectDaysSnapshot(t *testing.T) (snapshot, *service.TitleContext) {
	tc := perfectDaysContext()
	day := 24 * time.Hour
	return snapshotOf(t, tc, Request{},
		result(pdUHD, seeders(120), size(15_000_000_000), published(testNow.Add(-2*day))),
		result(pdBluRay, seeders(300), size(12_000_000_000), indexer("beta", "Beta"), published(testNow.Add(-200*day))),
		result(pdWEB, seeders(80), size(5_000_000_000)),
		result(pdSmall, seeders(40), size(1_200_000_000), indexer("beta", "Beta"), published(testNow.Add(-30*day))),
		result(pdCam, seeders(900), size(1_000_000_000)),
	), tc
}

func TestRead_FiltersAndDisjunctiveFacets(t *testing.T) {
	snap, tc := perfectDaysSnapshot(t)
	profile := library.DefaultQualityProfile()

	all := readOf(t, snap, tc, SearchRequest{}, profile)
	if all.Total != 4 || all.TotalUnfiltered != 4 || all.Hidden != 1 {
		t.Fatalf("total %d, unfiltered %d, hidden %d", all.Total, all.TotalUnfiltered, all.Hidden)
	}
	if got := bucketsText(all.Facets.HDR); got != "DV:1 HDR:1 SDR:3" {
		t.Errorf("hdr = %s", got)
	}
	if len(all.Facets.Pack) != 0 || all.Facets.Pack == nil {
		t.Errorf("pack = %#v, want empty for movies", all.Facets.Pack)
	}

	resp := readOf(t, snap, tc, SearchRequest{Filters: Filters{Resolution: []string{"1080p", "2160p"}, HDR: []string{"SDR"}}}, profile)
	if got := titles(resp.Items); resp.Total != 2 || !slices.Contains(got, pdBluRay) || !slices.Contains(got, pdWEB) {
		t.Fatalf("filtered = %d %v", resp.Total, got)
	}
	if resp.TotalUnfiltered != 4 {
		t.Errorf("total_unfiltered = %d", resp.TotalUnfiltered)
	}
	want := map[string]string{
		// Each facet ignores its own selection: resolution counts the SDR
		// releases, hdr the 1080p and 2160p ones.
		"resolution": "2160p:0 1080p:2 720p:1",
		"hdr":        "DV:1 HDR:1 SDR:2",
		"source":     "BluRay:1 WEB-DL:1 WEBRip:0",
		"codec":      "x265:0 x264:2",
		"audio":      "DTS-HD:1 DD+:1 AAC:0",
		"indexer":    "Alpha:1 Beta:1",
		"group":      "FLUX:1 GRP:1 YTS:0",
	}
	got := map[string]string{
		"resolution": bucketsText(resp.Facets.Resolution), "hdr": bucketsText(resp.Facets.HDR),
		"source": bucketsText(resp.Facets.Source), "codec": bucketsText(resp.Facets.Codec),
		"audio": bucketsText(resp.Facets.Audio), "indexer": bucketsText(resp.Facets.Indexer),
		"group": bucketsText(resp.Facets.Group),
	}
	for facet, w := range want {
		if got[facet] != w {
			t.Errorf("%s = %s, want %s", facet, got[facet], w)
		}
	}

	t.Run("an unknown value matches nothing and is listed", func(t *testing.T) {
		resp := readOf(t, snap, tc, SearchRequest{Filters: Filters{Resolution: []string{"999p"}}}, profile)
		if resp.Total != 0 || len(resp.Items) != 0 {
			t.Errorf("total = %d", resp.Total)
		}
		if got := bucketsText(resp.Facets.Resolution); got != "2160p:1 1080p:2 720p:1 999p:0" {
			t.Errorf("resolution = %s", got)
		}
	})
	t.Run("numeric filters", func(t *testing.T) {
		n, gb := 100, 6.0
		if resp := readOf(t, snap, tc, SearchRequest{Filters: Filters{MinSeeders: &n}}, profile); resp.Total != 2 {
			t.Errorf("min_seeders: %v", titles(resp.Items))
		}
		resp := readOf(t, snap, tc, SearchRequest{Filters: Filters{MaxSizeGB: &gb}}, profile)
		if got := titles(resp.Items); len(got) != 2 || !slices.Contains(got, pdWEB) || !slices.Contains(got, pdSmall) {
			t.Errorf("max_size_gb: %v", got)
		}
		// Numeric filters apply to every facet.
		if got := bucketsText(resp.Facets.Resolution); got != "2160p:0 1080p:1 720p:1" {
			t.Errorf("resolution under max_size_gb = %s", got)
		}
	})
}

func TestRead_Sorts(t *testing.T) {
	snap, tc := perfectDaysSnapshot(t)
	profile := library.DefaultQualityProfile()
	tests := []struct {
		sort Sort
		want []string
	}{
		{SortSeeders, []string{pdBluRay, pdUHD, pdWEB, pdSmall}},
		{SortSize, []string{pdSmall, pdWEB, pdBluRay, pdUHD}},
		{SortNewest, []string{pdUHD, pdSmall, pdBluRay, pdWEB}}, // pdWEB has no date: last
	}
	for _, tt := range tests {
		t.Run(string(tt.sort), func(t *testing.T) {
			if got := titles(readOf(t, snap, tc, SearchRequest{Sort: tt.sort}, profile).Items); !slices.Equal(got, tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("best: the best match first, then score", func(t *testing.T) {
		resp := readOf(t, snap, tc, SearchRequest{}, profile)
		if resp.BestID == nil || resp.Items[0].ID != *resp.BestID {
			t.Fatalf("best_id = %v, first = %s", resp.BestID, resp.Items[0].RawTitle)
		}
		for i := 2; i < len(resp.Items); i++ {
			if resp.Items[i].Score > resp.Items[i-1].Score {
				t.Errorf("not by score: %v", titles(resp.Items))
			}
		}
		// The best match leads page 1 under sort=best only.
		bySize := readOf(t, snap, tc, SearchRequest{Sort: SortSize}, profile)
		if *bySize.BestID != *resp.BestID {
			t.Errorf("best_id depends on the sort")
		}
	})
}

func TestRead_BestSkipsReleasesWithoutSeeders(t *testing.T) {
	tc := perfectDaysContext()
	snap := snapshotOf(t, tc, Request{},
		result(pdBluRay, seeders(0), size(12_000_000_000)),
		result(pdSmall, seeders(40), size(1_200_000_000)),
	)
	resp := readOf(t, snap, tc, SearchRequest{}, library.DefaultQualityProfile())
	if resp.BestID == nil || *resp.BestID != releaseNamed(t, resp, pdSmall).ID {
		t.Fatalf("best_id = %v, want the 720p with seeders", resp.BestID)
	}
	if !slices.Contains(releaseNamed(t, resp, pdBluRay).Flags, quality.FlagNoSeeders) {
		t.Error("no no_seeders flag")
	}

	none := readOf(t, snapshotOf(t, tc, Request{}, result(pdBluRay, seeders(0))), tc, SearchRequest{}, library.DefaultQualityProfile())
	if none.BestID != nil {
		t.Errorf("best_id = %v, want null", *none.BestID)
	}
}

func TestRead_ProfileAppliesAtReadTime(t *testing.T) {
	snap, tc := perfectDaysSnapshot(t)
	profile := library.DefaultQualityProfile()
	limit := "1080p"
	profile.MaxResolution = &limit
	profile.AvoidTerms = []string{"YTS"}
	resp := readOf(t, snap, tc, SearchRequest{}, profile)
	// The 2160p is over the limit and YTS is avoided; the CAM shows now.
	if resp.Hidden != 2 || resp.TotalUnfiltered != 3 {
		t.Fatalf("hidden %d, unfiltered %d: %v", resp.Hidden, resp.TotalUnfiltered, titles(resp.Items))
	}
	cam := releaseNamed(t, resp, pdCam)
	if !slices.Contains(cam.Flags, quality.FlagCAM) || resp.BestID != nil && *resp.BestID == cam.ID {
		t.Errorf("cam = %+v, best %v", cam.Flags, resp.BestID)
	}
}

// libraryFrieren is Frieren in the library with E01–E28 on disk: the
// missing run is S01E29–E38.
func libraryFrieren(t *testing.T) *service.TitleContext {
	tc := frierenContext(t)
	id := int64(7)
	tc.LibraryID, tc.EpisodeIDs, tc.Files = &id, service.EpisodeIDsLibrary, map[int64]string{}
	for _, ep := range tc.Catalog.Episodes {
		if ep.Season == 1 && ep.Number <= 28 {
			tc.Files[ep.ID] = catalog.SourceTorrent
		}
	}
	from, to := 29, 38
	tc.Missing = &catalog.MissingRun{SeasonNumber: 1, From: 29, To: 38, AbsoluteFrom: &from, AbsoluteTo: &to, Count: 10}
	tc.Runtime = 24
	return tc
}

const (
	frierenPack    = "Frieren Beyond Journeys End S02 1080p WEB-DL x264-PACK"
	frierenEp      = "[SubsPlease] Sousou no Frieren - 29 (1080p) [ABCD1234].mkv"
	frierenAll     = "Sousou no Frieren (01-38) [1080p] Batch"
	frierenEpScene = "Frieren.Beyond.Journeys.End.S02E01.1080p.WEB-DL.x265-GRP"
)

func TestRead_MissingAndBestCovers(t *testing.T) {
	tc := libraryFrieren(t)
	snap := snapshotOf(t, tc, Request{Season: intp(1), Episode: intp(29)},
		result(frierenPack, seeders(300), size(14_000_000_000)),
		result(frierenEp, seeders(50), size(1_400_000_000)),
		result(frierenAll, seeders(20), size(50_000_000_000)),
	)
	profile := library.DefaultQualityProfile()

	resp := readOf(t, snap, tc, SearchRequest{Season: intp(1), Episode: intp(29)}, profile)
	pack := releaseNamed(t, resp, frierenPack)
	if resp.Missing == nil || resp.Missing.Count != 10 || resp.Missing.From != 29 || resp.Missing.To != 38 {
		t.Fatalf("missing = %+v", resp.Missing)
	}
	if resp.BestID == nil || *resp.BestID != pack.ID || !resp.Missing.BestCovers {
		t.Fatalf("best = %v (pack %s), best_covers %v; scores %v", resp.BestID, pack.ID, resp.Missing.BestCovers, scores(resp))
	}
	if !slices.Contains(pack.Notes, `Scene "Season 2" = TMDB S01E29–E38`) || !slices.Contains(pack.ScoreReasons, "Covers all 10 missing episodes") {
		t.Errorf("pack notes %v, reasons %v", pack.Notes, pack.ScoreReasons)
	}
	all := releaseNamed(t, resp, frierenAll)
	if !slices.Contains(all.Notes, "Includes 28 episodes you already have; the mapping review asks before replacing them") {
		t.Errorf("batch notes = %v", all.Notes)
	}

	t.Run("the best match without the pack doesn't cover the run", func(t *testing.T) {
		resp := readOf(t, snap, tc, SearchRequest{Season: intp(1), Episode: intp(29), Filters: Filters{Pack: []string{"episode"}}}, profile)
		if resp.Missing == nil || resp.Missing.BestCovers || resp.BestID == nil {
			t.Errorf("missing = %+v, best %v", resp.Missing, resp.BestID)
		}
		if got := bucketsText(resp.Facets.Pack); got != "episode:1 multi_episode:1 season:1" {
			t.Errorf("pack facet = %s", got)
		}
	})
	t.Run("a request outside the run", func(t *testing.T) {
		snap := snapshotOf(t, tc, Request{Season: intp(1), Episode: intp(5)}, result("Frieren.S01E05.1080p"))
		if resp := readOf(t, snap, tc, SearchRequest{Season: intp(1), Episode: intp(5)}, profile); resp.Missing != nil {
			t.Errorf("missing = %+v", resp.Missing)
		}
	})
	t.Run("the season", func(t *testing.T) {
		snap := snapshotOf(t, tc, Request{Season: intp(1)}, result(frierenPack))
		if resp := readOf(t, snap, tc, SearchRequest{Season: intp(1)}, profile); resp.Missing == nil || !resp.Missing.BestCovers {
			t.Errorf("missing = %+v", resp.Missing)
		}
	})
	t.Run("a title outside the library", func(t *testing.T) {
		out := frierenContext(t)
		snap := snapshotOf(t, out, Request{Season: intp(1), Episode: intp(29)}, result(frierenPack))
		if resp := readOf(t, snap, out, SearchRequest{Season: intp(1), Episode: intp(29)}, profile); resp.Missing != nil {
			t.Errorf("missing = %+v", resp.Missing)
		}
	})
}

func scores(resp *SearchResponse) map[string]int {
	out := map[string]int{}
	for _, r := range resp.Items {
		out[r.RawTitle] = r.Score
	}
	return out
}

func TestRead_Magnets(t *testing.T) {
	tc := perfectDaysContext()
	snap := snapshotOf(t, tc, Request{},
		result(pdWEB, noMagnet()),
		result(pdSmall, hashOnly()),
		result(pdBluRay),
	)
	resp := readOf(t, snap, tc, SearchRequest{}, library.DefaultQualityProfile())

	fetch := releaseNamed(t, resp, pdWEB)
	if fetch.MagnetURI != nil || fetch.InfoHash != nil || !slices.Contains(fetch.Flags, FlagNeedsTorrentFetch) {
		t.Errorf("no magnet: %+v", fetch)
	}
	built := releaseNamed(t, resp, pdSmall)
	if built.MagnetURI == nil || !strings.HasPrefix(*built.MagnetURI, "magnet:?xt=urn:btih:"+hashOf(pdSmall)+"&dn=Perfect.Days") ||
		slices.Contains(built.Flags, FlagNeedsTorrentFetch) {
		t.Errorf("hash only: %+v", built)
	}
	if own := releaseNamed(t, resp, pdBluRay); own.MagnetURI == nil || slices.Contains(own.Flags, FlagNeedsTorrentFetch) {
		t.Errorf("magnet: %+v", own)
	}
}

func TestRead_Paging(t *testing.T) {
	snap, tc := perfectDaysSnapshot(t)
	profile := library.DefaultQualityProfile()
	p1 := readOf(t, snap, tc, SearchRequest{PageSize: 3}, profile)
	p2 := readOf(t, snap, tc, SearchRequest{Page: 2, PageSize: 3}, profile)
	p3 := readOf(t, snap, tc, SearchRequest{Page: 3, PageSize: 3}, profile)
	if len(p1.Items) != 3 || len(p2.Items) != 1 || len(p3.Items) != 0 || p3.Items == nil || p2.Total != 4 || p2.Page != 2 {
		t.Fatalf("pages = %d, %d, %d (total %d)", len(p1.Items), len(p2.Items), len(p3.Items), p2.Total)
	}
	if slices.Contains(titles(p1.Items), p2.Items[0].RawTitle) {
		t.Error("page 2 repeats page 1")
	}
}

func TestResponseNeverCarriesLinksOrTheKey(t *testing.T) {
	snap, tc := perfectDaysSnapshot(t)
	resp := readOf(t, snap, tc, SearchRequest{}, library.DefaultQualityProfile())
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{testKey, "jackett_apikey", "/dl/", "jackett.test", "alpha.test"} {
		if strings.Contains(string(body), bad) {
			t.Errorf("the response contains %q", bad)
		}
	}
	// The contract's shape: arrays, never null.
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	for _, key := range []string{"items", "queries", "indexers", "warnings"} {
		if _, ok := raw[key].([]any); !ok {
			t.Errorf("%s = %#v, want an array", key, raw[key])
		}
	}
	item := raw["items"].([]any)[0].(map[string]any)
	for _, key := range []string{"score_reasons", "notes", "warnings", "flags"} {
		if _, ok := item[key].([]any); !ok {
			t.Errorf("item %s = %#v, want an array", key, item[key])
		}
	}
	if attrs := item["attrs"].(map[string]any); attrs["pack"] != nil {
		t.Errorf("a movie's pack = %v", attrs["pack"])
	}
}
