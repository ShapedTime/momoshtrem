package search

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// elsewhere is a host that Jackett's links name but that must never be
// fetched (the SSRF guard): only the configured Jackett is.
type elsewhere struct {
	srv  *httptest.Server
	hits atomic.Int32
}

func newElsewhere(t *testing.T) *elsewhere {
	e := &elsewhere{}
	e.srv = httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { e.hits.Add(1) }))
	t.Cleanup(e.srv.Close)
	return e
}

// link is a Jackett download link on this host, carrying the API key.
func (e *elsewhere) link(file string) string {
	return e.srv.URL + "/dl/alpha/?jackett_apikey=" + testKey + "&path=x&file=" + file
}

func magnetFor(title string) string {
	return "magnet:?xt=urn:btih:" + hashOf(title) + "&dn=" + url.QueryEscape(title)
}

const (
	frierenOther  = "Totally.Other.Show.S01E29.1080p"
	frierenBatch  = "Sousou no Frieren (29-38) [1080p] Batch"
	frierenHDTV   = "Frieren Beyond Journeys End S02E01 720p HDTV"
	frierenE05    = "Frieren.Beyond.Journeys.End.S01E05.1080p.WEB-DL"
	frierenNoSeed = "Frieren.S01E29.480p"
)

// frierenItem is a feed item with a magnet.
func frierenItem(title string, seeders int, sizeBytes int64) feedItem {
	return feedItem{
		title: title, guid: "https://alpha.test/t/" + hashOf(title)[:8], hash: hashOf(title), magnet: magnetFor(title),
		size: sizeBytes, seeders: seeders, pub: testNow.Add(-48 * time.Hour),
	}
}

// frierenJackett has two indexers. Alpha answers the ID query with the
// scene-numbered episode and every text query with seven releases: five
// in scope for S01E29 (two without a magnet, linking to other), one
// episode outside it and one naming another show. Beta has a weaker copy
// of the scene-numbered episode.
func frierenJackett(t *testing.T, other *elsewhere) *fakeJackett {
	scene := frierenItem(frierenEpScene, 200, 1_400_000_000)
	scene.cover = "http://jackett.test/img/alpha?jackett_apikey=" + testKey // TheRARBG-style coverurl
	all := feed(
		scene,
		frierenItem(frierenEp, 500, 1_400_000_000),
		frierenItem(frierenPack, 50, 14_000_000_000),
		frierenItem(frierenE05, 90, 1_400_000_000),
		frierenItem(frierenOther, 70, 1_400_000_000),
		feedItem{title: frierenBatch, guid: "https://alpha.test/t/batch", link: other.link("f"), size: 14_000_000_000, seeders: 80},
		feedItem{title: frierenHDTV, guid: "https://alpha.test/t/hdtv", link: other.link("g"), size: 700_000_000, seeders: 30},
	)
	idOnly := feed(scene)
	weaker := frierenItem(frierenEpScene, 150, 1_400_000_000)
	weaker.guid = "https://beta.test/t/1"
	return newFakeJackett(t,
		fakeIndexer{id: "alpha", name: "Alpha", serve: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("q") == "" {
				_, _ = io.WriteString(w, idOnly)
				return
			}
			_, _ = io.WriteString(w, all)
		}},
		fakeIndexer{id: "beta", name: "Beta", serve: serveFeed(weaker)},
	)
}

func frierenRequest() SearchRequest {
	return SearchRequest{MediaType: "tv", TMDBID: 209867, Season: intp(1), Episode: intp(29)}
}

func queriesText(qs []SentQuery) []string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = q.Q
		if q.Label != nil {
			out[i] += " (" + *q.Label + ")"
		}
	}
	return out
}

func TestSearch_EndToEnd(t *testing.T) {
	other := newElsewhere(t)
	j := frierenJackett(t, other)
	s := newTestService(t, j, frierenContext(t), newClock(), 5*time.Second)

	resp, err := s.Search(context.Background(), frierenRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Complete {
		t.Fatalf("the starting GET answered before the search completed: %+v", resp.Indexers)
	}
	wantQueries := []string{"Frieren 29", "Sousou no Frieren 29 (original title)", "Frieren S02E01 (scene numbering)"}
	if got := queriesText(resp.Queries); !slices.Equal(got, wantQueries) {
		t.Errorf("queries = %v, want %v", got, wantQueries)
	}
	if n := j.total(); n != 8 { // 4 queries (3 text and the ID query) on 2 indexers
		t.Errorf("%d requests, want 8", n)
	}
	wantTitles := []string{frierenEpScene, frierenEp, frierenPack, frierenBatch, frierenHDTV}
	if got := titles(resp.Items); resp.Total != 5 || !sameSet(got, wantTitles) {
		t.Fatalf("releases = %v, want %v", got, wantTitles)
	}
	if a, b := indexerNamed(t, resp, "Alpha"), indexerNamed(t, resp, "Beta"); a.Status != torznab.StatusOK || a.Count != 22 || b.Status != torznab.StatusOK || b.Count != 4 {
		t.Errorf("indexers = %+v", resp.Indexers)
	}
	if r := releaseNamed(t, resp, frierenEpScene); r.Seeders != 200 || r.Indexer != "Alpha" || !slices.Contains(r.Notes, `Scene "S02E01" = TMDB S01E29`) {
		t.Errorf("the merged copy = %+v", r)
	}
	if r := releaseNamed(t, resp, frierenBatch); r.MagnetURI != nil || !slices.Contains(r.Flags, FlagNeedsTorrentFetch) {
		t.Errorf("a release without a magnet = %+v", r)
	}
	if resp.BestID == nil || resp.Items[0].ID != *resp.BestID || resp.Missing != nil {
		t.Errorf("best_id = %v, missing = %v", resp.BestID, resp.Missing)
	}

	body, _ := json.Marshal(resp)
	for _, bad := range []string{testKey, "jackett_apikey", "/dl/", "/img/", strings.TrimPrefix(other.srv.URL, "http://")} {
		if strings.Contains(string(body), bad) {
			t.Errorf("the response contains %q", bad)
		}
	}

	t.Run("page 2 asks Jackett nothing", func(t *testing.T) {
		req := frierenRequest()
		req.Page, req.PageSize = 2, 2
		resp2, err := s.Search(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if n := j.total(); n != 8 {
			t.Errorf("%d requests after page 2, want 8", n)
		}
		if resp2.Total != 5 || len(resp2.Items) != 2 || resp2.Page != 2 || slices.Contains(titles(resp2.Items), resp.Items[0].RawTitle) {
			t.Errorf("page 2 = %d of %d: %v", len(resp2.Items), resp2.Total, titles(resp2.Items))
		}
	})
	t.Run("filters and sort ask Jackett nothing", func(t *testing.T) {
		req := frierenRequest()
		req.Filters.Pack, req.Sort = []string{"season"}, SortSize
		resp3, err := s.Search(context.Background(), req)
		if err != nil || resp3.Total != 1 || j.total() != 8 {
			t.Errorf("err %v, total %d, requests %d", err, resp3.Total, j.total())
		}
	})
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func TestSearch_ProgressWhileAnIndexerIsSlow(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)

	j := newFakeJackett(t,
		fakeIndexer{id: "alpha", name: "Alpha", serve: serveFeed(frierenItem(frierenEpScene, 200, 1_400_000_000))},
		fakeIndexer{id: "beta", name: "Beta", serve: func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
				_, _ = io.WriteString(w, feed(frierenItem(frierenEp, 500, 1_400_000_000)))
			case <-r.Context().Done():
			}
		}},
	)
	s := newTestService(t, j, frierenContext(t), newClock(), 100*time.Millisecond)

	start := time.Now()
	resp, err := s.Search(context.Background(), frierenRequest())
	if err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); resp.Complete || waited > 2*time.Second {
		t.Fatalf("complete = %v after %s; the starting GET waits at most Deps.Wait", resp.Complete, waited)
	}

	// Polls answer at once with what has arrived: Alpha's results, Beta
	// still pending.
	deadline := time.Now().Add(10 * time.Second)
	for {
		start := time.Now()
		resp, err = s.Search(context.Background(), frierenRequest())
		if err != nil {
			t.Fatal(err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("a GET that found the search running waited")
		}
		if indexerNamed(t, resp, "Alpha").Status == torznab.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Alpha never finished: %+v", resp.Indexers)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if b := indexerNamed(t, resp, "Beta"); resp.Complete || b.Status != torznab.StatusPending || b.MS < 0 {
		t.Fatalf("complete %v, beta %+v", resp.Complete, b)
	}
	if resp.Total != 1 || resp.Items[0].RawTitle != frierenEpScene {
		t.Errorf("results so far = %v", titles(resp.Items))
	}

	unblock()
	final := searchUntilComplete(t, s, frierenRequest())
	if b := indexerNamed(t, final, "Beta"); b.Status != torznab.StatusOK || b.Count != 4 || final.Total != 2 {
		t.Errorf("beta %+v, total %d", b, final.Total)
	}
}

func TestSearch_PartialFailureIsPerIndexer(t *testing.T) {
	j := newFakeJackett(t,
		fakeIndexer{id: "alpha", name: "Alpha", serve: serveFeed(frierenItem(frierenEpScene, 200, 1_400_000_000))},
		fakeIndexer{id: "beta", name: "Beta", serve: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<error code="900" description="Cloudflare protection is enabled for https://beta.test/?apikey=`+testKey+`"/>`)
		}},
		fakeIndexer{id: "gamma", name: "Gamma", serve: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }},
	)
	s := newTestService(t, j, frierenContext(t), newClock(), 5*time.Second, torznab.WithTimeouts(200*time.Millisecond, 0))

	resp := searchUntilComplete(t, s, frierenRequest())
	alpha, beta, gamma := indexerNamed(t, resp, "Alpha"), indexerNamed(t, resp, "Beta"), indexerNamed(t, resp, "Gamma")
	if alpha.Status != torznab.StatusOK || beta.Status != torznab.StatusError || gamma.Status != torznab.StatusTimeout {
		t.Fatalf("indexers = %+v", resp.Indexers)
	}
	if !strings.Contains(beta.Error, "Cloudflare protection") || strings.Contains(beta.Error, testKey) || strings.Contains(beta.Error, "beta.test") {
		t.Errorf("beta's error = %q", beta.Error)
	}
	if gamma.Error == "" || resp.Total != 1 {
		t.Errorf("gamma %+v, total %d", gamma, resp.Total)
	}
}

func TestSearch_EveryIndexerFailingIsStillAnAnswer(t *testing.T) {
	failing := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `<error code="900" description="Indexer is down"/>`)
	}
	j := newFakeJackett(t, fakeIndexer{id: "alpha", name: "Alpha", serve: failing}, fakeIndexer{id: "beta", name: "Beta", serve: failing})
	s := newTestService(t, j, frierenContext(t), newClock(), 5*time.Second)
	resp := searchUntilComplete(t, s, frierenRequest())
	if resp.Total != 0 || indexerNamed(t, resp, "Alpha").Status != torznab.StatusError || indexerNamed(t, resp, "Beta").Status != torznab.StatusError {
		t.Errorf("total %d, indexers %+v", resp.Total, resp.Indexers)
	}
}

func TestSearch_FallbackWhenNothingIsInScope(t *testing.T) {
	j := newFakeJackett(t, fakeIndexer{id: "alpha", name: "Alpha", serve: func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "Frieren", "Sousou no Frieren":
			_, _ = io.WriteString(w, feed(frierenItem(frierenBatch, 80, 14_000_000_000), frierenItem(frierenE05, 90, 1_400_000_000)))
		default:
			_, _ = io.WriteString(w, feed(frierenItem(frierenE05, 90, 1_400_000_000)))
		}
	}})
	s := newTestService(t, j, frierenContext(t), newClock(), 5*time.Second)
	resp := searchUntilComplete(t, s, frierenRequest())

	want := []string{"Frieren 29", "Sousou no Frieren 29 (original title)", "Frieren S02E01 (scene numbering)",
		"Frieren (whole show)", "Sousou no Frieren (whole show)"}
	if got := queriesText(resp.Queries); !slices.Equal(got, want) {
		t.Errorf("queries = %v, want %v", got, want)
	}
	if got := titles(resp.Items); len(got) != 1 || got[0] != frierenBatch {
		t.Errorf("releases = %v", got)
	}
	if n := j.total(); n != 6 {
		t.Errorf("%d requests, want 4 exact and 2 fallback", n)
	}
	// 4 exact requests with one result each, 2 fallback ones with two.
	if a := indexerNamed(t, resp, "Alpha"); a.Status != torznab.StatusOK || a.Count != 8 {
		t.Errorf("alpha over both passes = %+v", a)
	}
}

func TestSearch_RefreshAndCacheExpiry(t *testing.T) {
	other := newElsewhere(t)
	j := frierenJackett(t, other)
	clock := newClock()
	s := newTestService(t, j, frierenContext(t), clock, 5*time.Second)

	first := searchUntilComplete(t, s, frierenRequest())
	n := j.total()
	refresh := frierenRequest()
	refresh.Refresh = true

	clock.Advance(5 * time.Second)
	again, err := s.Search(context.Background(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if j.total() != n || !again.CachedAt.Equal(first.CachedAt) {
		t.Fatalf("a refresh within 10 s searched again (%d requests, cached_at %s)", j.total(), again.CachedAt)
	}

	clock.Advance(6 * time.Second)
	rerun := searchUntilComplete(t, s, refresh)
	if j.total() != 2*n || !rerun.CachedAt.Equal(testNow.Add(11*time.Second)) {
		t.Fatalf("after 11 s: %d requests (want %d), cached_at %s", j.total(), 2*n, rerun.CachedAt)
	}

	clock.Advance(9 * time.Minute)
	if searchUntilComplete(t, s, frierenRequest()); j.total() != 2*n {
		t.Fatalf("a cached search ran again within 10 minutes")
	}
	clock.Advance(time.Minute)
	if searchUntilComplete(t, s, frierenRequest()); j.total() != 3*n {
		t.Fatalf("an expired search wasn't run again: %d requests", j.total())
	}
}

func TestSearch_JackettUnavailable(t *testing.T) {
	other := newElsewhere(t)
	j := frierenJackett(t, other)
	j.setList(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<error code="100" description="Invalid API Key"/>`)
	})
	s := newTestService(t, j, frierenContext(t), newClock(), 5*time.Second)

	_, err := s.Search(context.Background(), frierenRequest())
	if !errors.Is(err, ErrJackettUnavailable) || strings.Contains(err.Error(), testKey) || !strings.Contains(err.Error(), "Invalid API Key") {
		t.Fatalf("err = %v, want ErrJackettUnavailable", err)
	}
	// A failed search isn't cached: the next GET tries again.
	j.setList(nil)
	if resp := searchUntilComplete(t, s, frierenRequest()); resp.Total != 5 {
		t.Errorf("total = %d after Jackett came back", resp.Total)
	}
}

func TestSearch_TitleErrorsPassThrough(t *testing.T) {
	j := newFakeJackett(t)
	s := newTestService(t, j, frierenContext(t), newClock(), time.Second)
	if _, err := s.Search(context.Background(), SearchRequest{MediaType: "tv", TMDBID: 1}); !errors.Is(err, service.ErrTitleNotFound) {
		t.Errorf("err = %v, want ErrTitleNotFound", err)
	}
	if _, err := s.Search(context.Background(), SearchRequest{MediaType: "tv", TMDBID: 209867, Episode: intp(3)}); !errors.Is(err, ErrBadRequest) {
		t.Errorf("err = %v, want ErrBadRequest", err)
	}
	if n := j.total(); n != 0 {
		t.Errorf("%d requests", n)
	}
}

func TestResolve(t *testing.T) {
	other := newElsewhere(t)
	j := frierenJackett(t, other)
	clock := newClock()
	s := newTestService(t, j, frierenContext(t), clock, 5*time.Second)
	resp := searchUntilComplete(t, s, frierenRequest())

	torrent, hash := torrentFile(t, "Frieren S02 Batch", "udp://tracker.test:1337/announce")
	redirect := "magnet:?xt=urn:btih:" + hashOf("hdtv") + "&dn=Frieren+S02E01+720p"
	var gotPath, gotKey string
	j.setDL(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.URL.Query().Get("jackett_apikey")
		switch r.URL.Query().Get("file") {
		case "f":
			_, _ = w.Write(torrent)
		case "g":
			http.Redirect(w, r, redirect, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()

	batch := releaseNamed(t, resp, frierenBatch)
	magnet, ix, err := s.Resolve(ctx, batch.ID)
	if err != nil {
		t.Fatalf("Resolve(.torrent): %v", err)
	}
	for _, want := range []string{"xt=urn:btih:" + hash, "dn=Frieren+S02+Batch", "tr=udp%3A%2F%2Ftracker.test%3A1337%2Fannounce"} {
		if !strings.Contains(magnet, want) {
			t.Errorf("magnet %q lacks %q", magnet, want)
		}
	}
	if ix != "Alpha" || gotPath != "/dl/alpha/" || gotKey != testKey {
		t.Errorf("indexer %q, fetched %s with key %v", ix, gotPath, gotKey == testKey)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("the link's own host got %d requests", n)
	}
	if again, _, err := s.Resolve(ctx, batch.ID); err != nil || again != magnet || j.downloads() != 1 {
		t.Errorf("a second resolve: %v, %d downloads", err, j.downloads())
	}

	if got, _, err := s.Resolve(ctx, releaseNamed(t, resp, frierenHDTV).ID); err != nil || got != redirect {
		t.Errorf("Resolve(magnet redirect) = %q, %v", got, err)
	}
	scene := releaseNamed(t, resp, frierenEpScene)
	if got, _, err := s.Resolve(ctx, scene.ID); err != nil || got != *scene.MagnetURI || j.downloads() != 2 {
		t.Errorf("Resolve(magnet) = %q, %v (%d downloads)", got, err, j.downloads())
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("the link's own host got %d requests", n)
	}

	if _, _, err := s.Resolve(ctx, "no-such-id"); !errors.Is(err, ErrReleaseNotFound) || !errors.Is(err, service.ErrReleaseNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	clock.Advance(61 * time.Minute)
	if _, _, err := s.Resolve(ctx, scene.ID); !errors.Is(err, ErrReleaseNotFound) {
		t.Errorf("after an hour: %v", err)
	}
}

func TestResolve_Failures(t *testing.T) {
	other := newElsewhere(t)
	j := newFakeJackett(t, fakeIndexer{id: "alpha", name: "Alpha", serve: serveFeed(
		feedItem{title: "Perfect.Days.2023.1080p.WEB-DL-A", guid: "a", link: other.link("broken"), seeders: 10, size: 5e9, category: 2000},
		feedItem{title: "Perfect.Days.2023.1080p.WEB-DL-B", guid: "b", link: other.link("slow"), seeders: 10, size: 5e9, category: 2000},
		feedItem{title: "Perfect.Days.2023.1080p.WEB-DL-C", guid: "c", link: other.link("junk"), seeders: 10, size: 5e9, category: 2000},
		feedItem{title: "Perfect.Days.2023.1080p.WEB-DL-D", guid: "d", link: "http://jackett.test/details/1", seeders: 10, size: 5e9, category: 2000},
	)})
	j.setDL(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("file") {
		case "broken":
			w.WriteHeader(http.StatusInternalServerError)
		case "slow":
			<-r.Context().Done()
		default:
			_, _ = io.WriteString(w, "<html>not a torrent</html>")
		}
	})
	s := newTestService(t, j, perfectDaysContext(), newClock(), 5*time.Second, torznab.WithTimeouts(0, 200*time.Millisecond))
	resp := searchUntilComplete(t, s, SearchRequest{MediaType: "movie", TMDBID: 976893})
	if resp.Total != 4 {
		t.Fatalf("releases = %v", titles(resp.Items))
	}

	tests := []struct {
		title string
		want  error
	}{
		{"Perfect.Days.2023.1080p.WEB-DL-A", ErrTorrentUnavailable},
		{"Perfect.Days.2023.1080p.WEB-DL-B", ErrResolveTimeout},
		{"Perfect.Days.2023.1080p.WEB-DL-C", ErrTorrentUnavailable},
		{"Perfect.Days.2023.1080p.WEB-DL-D", ErrTorrentUnavailable}, // not a /dl/ link: never fetched
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			_, _, err := s.Resolve(context.Background(), releaseNamed(t, resp, tt.title).ID)
			if !errors.Is(err, tt.want) || strings.Contains(err.Error(), testKey) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if !errors.Is(ErrResolveTimeout, torznab.ErrTimeout) {
		t.Error("ErrResolveTimeout doesn't wrap torznab.ErrTimeout")
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("the links' own host got %d requests", n)
	}
}
