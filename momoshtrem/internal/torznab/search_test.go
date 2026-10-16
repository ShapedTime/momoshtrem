package torznab

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// withEZTVCaps returns the indexer list with EZTV's tv-search taking the
// ids no recorded indexer supports yet.
func withEZTVCaps(t *testing.T) []byte {
	t.Helper()
	list := string(fixture(t, "indexers.xml"))
	i := strings.Index(list, `<indexer id="eztv"`)
	old := `<tv-search available="yes" supportedParams="q,season,ep" />`
	if i < 0 || !strings.Contains(list[i:], old) {
		t.Fatal("indexers.xml changed: no eztv tv-search to edit")
	}
	return []byte(list[:i] + strings.Replace(list[i:], old,
		`<tv-search available="yes" supportedParams="q,season,ep,imdbid,tvdbid,tmdbid" />`, 1))
}

func TestSearch_Modes(t *testing.T) {
	tests := []struct {
		name      string
		indexer   string
		fixture   string
		query     Query
		want      url.Values // the request, without apikey
		wantCount int
	}{
		{
			name:    "text search asks only for the categories the indexer declares",
			indexer: "thepiratebay", fixture: "search_thepiratebay.xml",
			query:     Query{Q: "Frieren"},
			want:      url.Values{"t": {"search"}, "q": {"Frieren"}, "cat": {"2000,5000"}},
			wantCount: 8,
		},
		{
			name:    "tv search drops the ids its caps lack",
			indexer: "torrentgalaxyclone", fixture: "tvsearch_torrentgalaxyclone.xml",
			query:     Query{Mode: ModeTV, Q: "Frieren", Season: "1", Episode: "1", IMDbID: "tt22248376", TVDBID: 424536},
			want:      url.Values{"t": {"tvsearch"}, "q": {"Frieren"}, "season": {"1"}, "ep": {"1"}, "cat": {"5000,5070"}},
			wantCount: 6,
		},
		{
			name:    "movie search sends imdbid with its tt prefix and drops tmdbid",
			indexer: "therarbg", fixture: "movie_therarbg.xml",
			query:     Query{Mode: ModeMovie, Q: "Interstellar", IMDbID: "0816692", TMDBID: 157336},
			want:      url.Values{"t": {"movie"}, "q": {"Interstellar"}, "imdbid": {"tt0816692"}, "cat": {"2000"}},
			wantCount: 6,
		},
		{
			name:    "an unavailable mode falls back to text search",
			indexer: "eztv", fixture: "tvsearch_torrentgalaxyclone.xml",
			query:     Query{Mode: ModeMovie, Q: "The Bear", IMDbID: "tt14452776", Categories: []int{CategoryTV}},
			want:      url.Values{"t": {"search"}, "q": {"The Bear"}, "cat": {"5000"}},
			wantCount: 6,
		},
		{
			name:    "ids the caps support are sent, an id-only query included",
			indexer: "eztv", fixture: "tvsearch_torrentgalaxyclone.xml",
			query:     Query{Mode: ModeTV, TVDBID: 424536, TMDBID: 209867, Season: "0", Episode: "3"},
			want:      url.Values{"t": {"tvsearch"}, "tvdbid": {"424536"}, "tmdbid": {"209867"}, "season": {"0"}, "ep": {"3"}, "cat": {"5000"}},
			wantCount: 6,
		},
		{
			name:    "adult categories are never requested; limit is capped",
			indexer: "thepiratebay", fixture: "search_thepiratebay.xml",
			query:     Query{Q: "Frieren", Categories: []int{6000, 6010, CategoryMovies}, Limit: 500, Offset: 100},
			want:      url.Values{"t": {"search"}, "q": {"Frieren"}, "cat": {"2000"}, "limit": {"100"}, "offset": {"100"}},
			wantCount: 8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := newFakeJackett(t)
			j.indexers = withEZTVCaps(t)
			j.handle(tt.indexer, serveFixture(t, tt.fixture))
			c := New(j.URL, testKey)

			results, err := c.Search(context.Background(), tt.indexer, tt.query)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("%d results, want %d", len(results), tt.wantCount)
			}
			reqs := j.searches(tt.indexer)
			if len(reqs) != 1 {
				t.Fatalf("%d requests, want 1", len(reqs))
			}
			got := reqs[0]
			if got.Get("apikey") != testKey {
				t.Errorf("apikey = %q", got.Get("apikey"))
			}
			got.Del("apikey")
			if got.Encode() != tt.want.Encode() {
				t.Errorf("request = %s\nwant      %s", got.Encode(), tt.want.Encode())
			}
		})
	}
}

func TestSearch_Unsupported(t *testing.T) {
	tests := []struct {
		name    string
		indexer string
		query   Query
	}{
		{"no category in common", "eztv", Query{Mode: ModeMovie, Q: "Interstellar"}},
		{"id-only query without id support", "thepiratebay", Query{Mode: ModeMovie, IMDbID: "tt0816692"}},
		{"nothing to search for", "thepiratebay", Query{Mode: ModeTV, Season: "1", Episode: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := newFakeJackett(t)
			c := New(j.URL, testKey)
			if _, err := c.Search(context.Background(), tt.indexer, tt.query); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", err)
			}
			if n := len(j.searches(tt.indexer)); n != 0 {
				t.Errorf("%d requests, want none", n)
			}
			if h := c.Snapshot(); h[0].Status != StatusUnknown {
				t.Errorf("a skipped query changed health: %+v", h[0])
			}
		})
	}
}

func TestSearch_ParsesItems(t *testing.T) {
	j := newFakeJackett(t)
	j.handle("thepiratebay", serveFixture(t, "search_thepiratebay.xml"))
	j.handle("torrentgalaxyclone", serveFixture(t, "tvsearch_torrentgalaxyclone.xml"))
	c := New(j.URL, testKey)
	ctx := context.Background()

	tpb, err := c.Search(ctx, "thepiratebay", Query{Q: "Frieren"})
	if err != nil {
		t.Fatal(err)
	}
	r := tpb[0]
	if r.Title != "Frieren.Beyond.Journeys.End.S02E06.1080p.CR.WEBRip.10bits.x265-Rapta" {
		t.Errorf("title = %q", r.Title)
	}
	if r.InfoHash != "1f1ad4ac357b893b17c7a3554f292775b6f816f6" {
		t.Errorf("info hash = %q", r.InfoHash)
	}
	if !strings.HasPrefix(r.MagnetURI, "magnet:?xt=urn:btih:1F1AD4AC357B893B17C7A3554F292775B6F816F6&dn=") {
		t.Errorf("magnet = %q", r.MagnetURI)
	}
	if r.Size != 227876798 || r.Seeders != 180 || r.Leechers != 7 {
		t.Errorf("size, seeders, leechers = %d, %d, %d", r.Size, r.Seeders, r.Leechers)
	}
	if want := time.Date(2026, 2, 27, 23, 17, 59, 0, time.UTC); !r.PublishedAt.Equal(want) {
		t.Errorf("published = %v", r.PublishedAt)
	}
	if len(r.Categories) != 2 || r.Categories[0] != 5040 || r.Categories[1] != 100208 {
		t.Errorf("categories = %v", r.Categories)
	}
	if r.IndexerID != "thepiratebay" || r.IndexerName != "The Pirate Bay" {
		t.Errorf("indexer = %q %q", r.IndexerID, r.IndexerName)
	}

	// TorrentGalaxyClone links to Jackett's /dl/ proxy and gives no magnet.
	tgx, err := c.Search(ctx, "torrentgalaxyclone", Query{Mode: ModeTV, Q: "Frieren", Season: "1", Episode: "1"})
	if err != nil {
		t.Fatal(err)
	}
	r = tgx[0]
	if r.MagnetURI != "" || r.InfoHash != "" {
		t.Errorf("magnet, info hash = %q, %q; want none", r.MagnetURI, r.InfoHash)
	}
	if !strings.HasPrefix(r.Link, "http://jackett.test/dl/torrentgalaxyclone/?jackett_apikey=") {
		t.Errorf("link = %q", r.Link)
	}
	if r.Size != 1717986918 || r.Seeders != 35 || r.Leechers != 0 {
		t.Errorf("size, seeders, leechers = %d, %d, %d", r.Size, r.Seeders, r.Leechers)
	}
}

func TestSearch_Hygiene(t *testing.T) {
	j := newFakeJackett(t)
	j.handle("thepiratebay", serveFixture(t, "search_hygiene.xml"))
	c := New(j.URL, testKey)

	results, err := c.Search(context.Background(), "thepiratebay", Query{Q: "Frieren"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		for _, r := range results {
			t.Logf("kept %q %v", r.Title, r.Categories)
		}
		t.Fatalf("%d results, want 4 (both adult items dropped)", len(results))
	}
	for _, r := range results {
		if strings.Contains(r.Title, "dult") {
			t.Errorf("adult item kept: %q", r.Title)
		}
	}

	entity, dl, hashOnly, base32 := results[0], results[1], results[2], results[3]
	if want := "[Erai-raws] Sousou no Frieren ♥ Journey's End - 01 [1080p] & Extras"; entity.Title != want {
		t.Errorf("title = %q, want %q", entity.Title, want)
	}
	if entity.Seeders != 10 || entity.Leechers != 0 {
		t.Errorf("peers below seeders: seeders %d, leechers %d; want 10, 0", entity.Seeders, entity.Leechers)
	}
	if dl.Size != 1717986918 || dl.MagnetURI != "" || dl.Leechers != 5 {
		t.Errorf("enclosure-only item = %+v", dl)
	}
	if hashOnly.Size != 1500000000 || hashOnly.InfoHash != "abcdef0123456789abcdef0123456789abcdef01" ||
		hashOnly.MagnetURI != "" || !hashOnly.PublishedAt.IsZero() {
		t.Errorf("info-hash-only item = %+v", hashOnly)
	}
	if base32.InfoHash != "5f5b1e2f0c3a8e4d9b7a6c5d4e3f2a1b0c9d8e7f" || base32.Leechers != 0 {
		t.Errorf("base32 item = %+v", base32)
	}
}

func TestSearch_TorznabErrorDocument(t *testing.T) {
	j := newFakeJackett(t)
	body := fixture(t, "error_indexer.xml")
	j.handle("1337x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // as recorded
		_, _ = w.Write(body)
	})
	c := New(j.URL, testKey)

	_, err := c.Search(context.Background(), "1337x", Query{Q: "Frieren"})
	var e *Error
	if !errors.As(err, &e) || e.Code != 900 {
		t.Fatalf("err = %v (%T), want a Torznab error 900", err, err)
	}
	if e.Description != "Challenge detected but FlareSolverr is not configured" {
		t.Errorf("description = %q", e.Description)
	}
	h := snapshotOf(t, c, "1337x")
	if h.Status != StatusError || h.Error != "jackett error: Challenge detected but FlareSolverr is not configured" {
		t.Errorf("health = %+v", h)
	}
}

func TestSearch_Timeout(t *testing.T) {
	j := newFakeJackett(t)
	j.handle("thepiratebay", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	c := New(j.URL, testKey, WithTimeouts(50*time.Millisecond, 0))

	start := time.Now()
	_, err := c.Search(context.Background(), "thepiratebay", Query{Q: "Frieren"})
	if !errors.Is(err, ErrTimeout) || err.Error() != "timed out after 50ms" {
		t.Fatalf("err = %v, want ErrTimeout after 50ms", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %s; the timeout didn't apply", elapsed)
	}
	if h := snapshotOf(t, c, "thepiratebay"); h.Status != StatusTimeout || h.Since == nil {
		t.Errorf("health = %+v", h)
	}
}

func TestSearch_UnknownIndexer(t *testing.T) {
	j := newFakeJackett(t)
	c := New(j.URL, testKey)
	if _, err := c.Search(context.Background(), "nyaasi", Query{Q: "Frieren"}); !errors.Is(err, ErrUnknownIndexer) {
		t.Fatalf("err = %v, want ErrUnknownIndexer", err)
	}
	if n := len(j.searches("nyaasi")); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

func TestSearch_CallerCancelDoesNotTouchHealth(t *testing.T) {
	j := newFakeJackett(t)
	j.handle("thepiratebay", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c := New(j.URL, testKey)
	if _, err := c.Indexers(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	if _, err := c.Search(ctx, "thepiratebay", Query{Q: "Frieren"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if h := snapshotOf(t, c, "thepiratebay"); h.Status != StatusUnknown {
		t.Errorf("health = %+v, want unknown", h)
	}
}

func snapshotOf(t *testing.T, c *Client, id string) IndexerHealth {
	t.Helper()
	for _, h := range c.Snapshot() {
		if h.ID == id {
			return h
		}
	}
	t.Fatalf("no health for %s", id)
	return IndexerHealth{}
}
