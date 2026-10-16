package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/search"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// fakeSearch records the search request and answers with err, or an
// empty page; Resolve answers with magnet or resolveErr.
type fakeSearch struct {
	req        *search.SearchRequest
	err        error
	resolveErr error
	resolved   []string
}

func (f *fakeSearch) Search(_ context.Context, req search.SearchRequest) (*search.SearchResponse, error) {
	f.req = &req
	if f.err != nil {
		return nil, f.err
	}
	return &search.SearchResponse{Items: []search.Release{}, Page: 1, PageSize: 25, Complete: true}, nil
}

func (f *fakeSearch) Resolve(_ context.Context, id string) (string, string, error) {
	f.resolved = append(f.resolved, id)
	if f.resolveErr != nil {
		return "", "", f.resolveErr
	}
	return "magnet:?xt=urn:btih:" + strings.Repeat("AB", 20) + "&dn=x", "Alpha", nil
}

func TestSearchRoute_Params(t *testing.T) {
	f := &fakeSearch{}
	s := newTestServer(Options{Search: f})
	rec := do(t, s, "GET", "/api/search/torrents?media_type=tv&tmdb_id=209867&season=1&episode=29&scope=episode&q=Frieren+29"+
		"&resolution=1080p&resolution=2160p&resolution=&hdr=SDR&source=WEB-DL&codec=x265&audio=AAC&pack=season&indexer=Nyaa&group=SubsPlease"+
		"&min_seeders=5&min_size_gb=0.5&max_size_gb=4&sort=seeders&page=2&page_size=50&refresh=1", nil)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	r := f.req
	switch {
	case r.MediaType != "tv" || r.TMDBID != 209867 || *r.Season != 1 || *r.Episode != 29 || r.Scope != search.ScopeEpisode || r.Q != "Frieren 29":
		t.Errorf("context = %+v", r)
	case !slices.Equal(r.Filters.Resolution, []string{"1080p", "2160p"}) || !slices.Equal(r.Filters.HDR, []string{"SDR"}) ||
		!slices.Equal(r.Filters.Source, []string{"WEB-DL"}) || !slices.Equal(r.Filters.Codec, []string{"x265"}) ||
		!slices.Equal(r.Filters.Audio, []string{"AAC"}) || !slices.Equal(r.Filters.Pack, []string{"season"}) ||
		!slices.Equal(r.Filters.Indexer, []string{"Nyaa"}) || !slices.Equal(r.Filters.Group, []string{"SubsPlease"}):
		t.Errorf("filters = %+v", r.Filters)
	case *r.Filters.MinSeeders != 5 || *r.Filters.MinSizeGB != 0.5 || *r.Filters.MaxSizeGB != 4:
		t.Errorf("numeric filters = %v %v %v", *r.Filters.MinSeeders, *r.Filters.MinSizeGB, *r.Filters.MaxSizeGB)
	case r.Sort != search.SortSeeders || r.Page != 2 || r.PageSize != 50 || !r.Refresh:
		t.Errorf("sort and paging = %q %d %d %v", r.Sort, r.Page, r.PageSize, r.Refresh)
	}

	do(t, s, "GET", "/api/search/torrents?media_type=movie&tmdb_id=976893", nil)
	if r := f.req; r.Season != nil || r.Filters.MinSeeders != nil || r.Page != 0 || r.PageSize != 0 || r.Refresh || r.Filters.Resolution != nil {
		t.Errorf("defaults = %+v", r)
	}
}

func TestSearchRoute_MalformedParams(t *testing.T) {
	for _, query := range []string{
		"media_type=tv",
		"media_type=tv&tmdb_id=abc",
		"media_type=tv&tmdb_id=0",
		"media_type=tv&tmdb_id=1&season=one",
		"media_type=tv&tmdb_id=1&season=1&episode=1.5",
		"media_type=tv&tmdb_id=1&min_seeders=x",
		"media_type=tv&tmdb_id=1&min_size_gb=big",
		"media_type=tv&tmdb_id=1&max_size_gb=NaN",
		"media_type=tv&tmdb_id=1&page=0",
		"media_type=tv&tmdb_id=1&page_size=-5",
		"media_type=tv&tmdb_id=1&refresh=yes",
	} {
		t.Run(query, func(t *testing.T) {
			f := &fakeSearch{}
			rec := do(t, newTestServer(Options{Search: f}), "GET", "/api/search/torrents?"+query, nil)
			if rec.Code != 400 || decodeError(t, rec).Code != CodeBadRequest || f.req != nil {
				t.Errorf("status = %d, searched = %v: %s", rec.Code, f.req != nil, rec.Body)
			}
		})
	}
}

func TestSearchRoute_Errors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"a bad request", fmt.Errorf("%w: scope episode needs an episode", search.ErrBadRequest), 400, CodeBadRequest, "Scope episode needs an episode."},
		{"an unknown title", fmt.Errorf("%w: gone", service.ErrTitleNotFound), 404, CodeNotFound, ""},
		{"TMDB down", fmt.Errorf("%w: 503", library.ErrTMDBFetch), 502, CodeTMDBUnavailable, ""},
		{"Jackett down", fmt.Errorf("%w: jackett error: Invalid API Key", search.ErrJackettUnavailable), 502, CodeJackettUnavailable, "Jackett is unavailable: jackett error: Invalid API Key."},
		{"the client left", context.Canceled, StatusClientClosedRequest, CodeClientClosed, ""},
		{"anything else", errors.New("conn reset by 10.0.0.3"), 500, CodeInternalError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestServer(Options{Search: &fakeSearch{err: tt.err}}), "GET", "/api/search/torrents?media_type=tv&tmdb_id=1", nil)
			got := decodeError(t, rec)
			if rec.Code != tt.wantStatus || got.Code != tt.wantCode || strings.Contains(got.Message, "10.0.0.3") {
				t.Errorf("%d %+v, want %d %s", rec.Code, got, tt.wantStatus, tt.wantCode)
			}
			if tt.wantMsg != "" && got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}
		})
	}
}

func TestSearchRoutes_NotConfigured(t *testing.T) {
	s := newTestServer(Options{})
	if rec := do(t, s, "GET", "/api/search/torrents?media_type=tv&tmdb_id=1", nil); rec.Code != 503 || decodeError(t, rec).Code != CodeUnavailable {
		t.Errorf("search: %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(t, s, "POST", "/api/search/torrents/resolve", `{"release_id": "r1"}`); rec.Code != 503 || decodeError(t, rec).Code != CodeUnavailable {
		t.Errorf("resolve: %d %s", rec.Code, rec.Body)
	}
}

func TestResolveRoute(t *testing.T) {
	f := &fakeSearch{}
	rec := doBody(t, newTestServer(Options{Search: f}), "POST", "/api/search/torrents/resolve", `{"release_id": " r1 "}`)
	var body resolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if body.InfoHash != strings.Repeat("ab", 20) || !strings.HasPrefix(body.MagnetURI, "magnet:?xt=urn:btih:") || f.resolved[0] != "r1" {
		t.Errorf("body = %+v, resolved %q", body, f.resolved)
	}

	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"no id", `{}`, nil, 400, CodeBadRequest},
		{"a broken body", `{"release_id":`, nil, 400, CodeBadRequest},
		{"expired", `{"release_id": "r1"}`, search.ErrReleaseNotFound, 404, CodeNotFound},
		{"too slow", `{"release_id": "r1"}`, search.ErrResolveTimeout, 504, CodeTimeout},
		{"no torrent", `{"release_id": "r1"}`, fmt.Errorf("%w: jackett answered HTTP 500", search.ErrTorrentUnavailable), 502, CodeTorrentUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doBody(t, newTestServer(Options{Search: &fakeSearch{resolveErr: tt.err}}), "POST", "/api/search/torrents/resolve", tt.body)
			if got := decodeError(t, rec); rec.Code != tt.wantStatus || got.Code != tt.wantCode {
				t.Errorf("%d %+v, want %d %s", rec.Code, got, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

// stubJackett is one indexer that answers every query with results, and a
// .torrent for every download.
type stubJackett struct {
	results []torznab.Result
	torrent []byte
	links   []string
}

func (j *stubJackett) Indexers(context.Context) ([]torznab.Indexer, error) {
	return []torznab.Indexer{{ID: "alpha", Title: "Alpha"}}, nil
}

func (j *stubJackett) Fanout(_ context.Context, reqs []torznab.Request, o torznab.FanoutOptions) []torznab.Response {
	o.OnProgress(torznab.Progress{IndexerID: "alpha", IndexerName: "Alpha", Status: torznab.StatusPending, RequestIndex: -1})
	for i := range reqs {
		status := torznab.StatusPending
		if i == len(reqs)-1 {
			status = torznab.StatusOK
		}
		o.OnProgress(torznab.Progress{IndexerID: "alpha", IndexerName: "Alpha", Status: status,
			Count: (i + 1) * len(j.results), RequestIndex: i, Results: j.results})
	}
	return nil
}

func (j *stubJackett) Download(_ context.Context, link string) (string, []byte, error) {
	j.links = append(j.links, link)
	return "", j.torrent, nil
}

func (j *stubJackett) SafeError(err error) string { return err.Error() }

type movieTitles struct{}

func (movieTitles) Get(_ context.Context, mt library.JobMediaType, id int) (*service.TitleContext, error) {
	return &service.TitleContext{MediaType: mt, TMDBID: id, Title: "Perfect Days", Year: 2023, Runtime: 124}, nil
}

type defaultProfile struct{}

func (defaultProfile) QualityProfile(context.Context) (library.QualityProfile, error) {
	return library.DefaultQualityProfile(), nil
}

// Inspect with a release_id goes through the search's resolver: the
// release a search returned, without a magnet, has its .torrent fetched,
// and Inspect gets the magnet and the indexer's name.
func TestInspectResolvesThroughTheSearch(t *testing.T) {
	infoBytes, err := bencode.Marshal(metainfo.Info{Name: "Perfect Days", PieceLength: 16384, Pieces: make([]byte, 20), Length: 1000})
	if err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: infoBytes, Announce: "udp://tracker.test:1337/announce"}
	var torrentFile bytes.Buffer
	if err := mi.Write(&torrentFile); err != nil {
		t.Fatal(err)
	}
	hash := mi.HashInfoBytes().HexString()
	const link = "http://jackett.test/dl/alpha/?jackett_apikey=secret&file=pd"

	jackett := &stubJackett{
		results: []torznab.Result{{Title: "Perfect.Days.2023.1080p.WEB-DL", GUID: "g1", Link: link, Size: 5e9, Seeders: 40, IndexerID: "alpha", IndexerName: "Alpha"}},
		torrent: torrentFile.Bytes(),
	}
	svc := search.New(search.Deps{Titles: movieTitles{}, Jackett: jackett, Profiles: defaultProfile{}})
	inspector := &fakeInspector{created: true}
	s := newTestServer(Options{Search: svc, Releases: svc, Inspect: inspector})

	rec := do(t, s, "GET", "/api/search/torrents?media_type=movie&tmdb_id=976893", nil)
	var resp search.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != 200 || len(resp.Items) != 1 {
		t.Fatalf("search: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "secret") || !slices.Contains(resp.Items[0].Flags, search.FlagNeedsTorrentFetch) {
		t.Fatalf("release = %+v", resp.Items[0])
	}

	rec = doBody(t, s, "POST", "/api/search/torrents/inspect", `{"media_type": "movie", "tmdb_id": 976893, "release_id": "`+resp.Items[0].ID+`"}`)
	if rec.Code != 202 {
		t.Fatalf("inspect: %d %s", rec.Code, rec.Body)
	}
	r := inspector.req
	if r == nil || !strings.HasPrefix(r.MagnetURI, "magnet:?xt=urn:btih:"+hash) || r.Indexer != "Alpha" || len(jackett.links) != 1 || jackett.links[0] != link {
		t.Fatalf("inspect request = %+v, downloads %d", r, len(jackett.links))
	}

	// Resolve answers the same magnet without fetching again.
	rec = doBody(t, s, "POST", "/api/search/torrents/resolve", `{"release_id": "`+resp.Items[0].ID+`"}`)
	var body resolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.MagnetURI != r.MagnetURI || body.InfoHash != hash || len(jackett.links) != 1 {
		t.Errorf("resolve: %d %s", rec.Code, rec.Body)
	}

	rec = doBody(t, s, "POST", "/api/search/torrents/inspect", `{"media_type": "movie", "tmdb_id": 976893, "release_id": "expired"}`)
	if rec.Code != 404 || decodeError(t, rec).Code != CodeNotFound {
		t.Errorf("an unknown release: %d %s", rec.Code, rec.Body)
	}
}
