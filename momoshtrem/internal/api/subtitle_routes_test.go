package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/subtitle/subtitletest"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

const (
	subHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	subFlux  = "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX"
)

// fakeItems resolves a fixed set of library items.
type fakeItems map[subtitle.Item]subtitle.ItemContext

func (f fakeItems) ResolveSubtitleItem(_ context.Context, item subtitle.Item) (*subtitle.ItemContext, error) {
	if ic, ok := f[item]; ok {
		return &ic, nil
	}
	return nil, nil
}

// countingTree records InvalidateTree calls.
type countingTree struct{ invalidated int }

func (c *countingTree) RemoveMovieFromTree(string, int)             {}
func (c *countingTree) RemoveEpisodeFromTree(string, int, int, int) {}
func (c *countingTree) RemoveShowFromTree(string, int)              {}
func (c *countingTree) InvalidateTree()                             { c.invalidated++ }
func (c *countingTree) InvalidateFileHandles(string)                {}

var _ vfs.TreeUpdater = (*countingTree)(nil)

type subtitleFixture struct {
	server  *Server
	repo    *subtitletest.Repo
	fetcher *subtitletest.Fetcher
	tree    *countingTree
	torrent *subtitle.Subtitle // Dune's English torrent subtitle
	ru      *subtitle.Subtitle // Dune's downloaded Russian subtitle
}

var (
	duneItem    = subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 1}
	pastLives   = subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 2} // In the library, nothing assigned
	severanceEp = subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: 501}
)

func newSubtitleFixture(t *testing.T, fetcher *subtitletest.Fetcher) *subtitleFixture {
	t.Helper()
	repo := subtitletest.NewRepo()
	f := &subtitleFixture{
		server:  newTestServer(Options{}),
		repo:    repo,
		fetcher: fetcher,
		tree:    &countingTree{},
		torrent: repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", LanguageName: "English",
			Format: "srt", Source: subtitle.SourceTorrent, InfoHash: subHashA, FilePath: "Dune/Subs/English.srt"}),
		ru: repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "ru", LanguageName: "Russian",
			Format: "srt", Source: subtitle.SourceOpenSubtitles, FilePath: t.TempDir() + "/ru.srt", ReleaseName: subFlux}),
	}
	var osFetcher opensubtitles.SubtitleFetcher
	if fetcher != nil {
		osFetcher = fetcher
	}
	f.server.subtitleService = subtitle.NewService(osFetcher, repo, t.TempDir())
	f.server.treeUpdater = f.tree
	f.server.subtitleItems = fakeItems{
		duneItem: {Item: duneItem, Search: opensubtitles.SearchParams{Type: "movie", TMDBID: 693134},
			InfoHash: subHashA, Release: subFlux},
		pastLives: {Item: pastLives, Search: opensubtitles.SearchParams{Type: "movie", TMDBID: 666277}},
		severanceEp: {Item: severanceEp, Search: opensubtitles.SearchParams{Type: "episode", ParentTMDBID: 95396, SeasonNumber: 2, EpisodeNumber: 5},
			InfoHash: subHashA, Release: "Severance.S02.2160p.ATVP.WEB-DL-FLUX"},
	}
	return f
}

func (f *subtitleFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(rec, req)
	return rec
}

// keys returns the sorted JSON keys of an object.
func keys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The field sets of web/src/lib/api/contract/subtitles.ts.
var (
	subtitleKeys  = []string{"created_at", "format", "id", "language_code", "language_name", "match", "release_name", "source"}
	candidateKeys = []string{"downloads", "file_id", "file_name", "hearing_impaired", "language_code", "language_name",
		"machine_translated", "match", "match_reason", "match_score", "release_name"}
)

func TestItemSubtitles_List(t *testing.T) {
	f := newSubtitleFixture(t, nil) // Listing doesn't need OpenSubtitles

	rec := f.do(t, http.MethodGet, "/api/movies/1/subtitles", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2: %s", len(body.Items), rec.Body)
	}
	if got := keys(t, body.Items[0]); !reflect.DeepEqual(got, subtitleKeys) {
		t.Errorf("Subtitle keys = %v, want %v", got, subtitleKeys)
	}

	var items []ItemSubtitle
	for _, raw := range body.Items {
		var s ItemSubtitle
		_ = json.Unmarshal(raw, &s)
		items = append(items, s)
	}
	// en (torrent) before ru (downloaded): by language.
	en, ru := items[0], items[1]
	if en.Source != "torrent" || en.Match != "exact_release" || en.ReleaseName == nil || *en.ReleaseName != subFlux {
		t.Errorf("torrent subtitle = %+v", en)
	}
	if ru.Source != "opensubtitles" || ru.Match != "exact_release" || ru.LanguageName != "Russian" {
		t.Errorf("downloaded subtitle = %+v", ru)
	}

	t.Run("nothing assigned: downloaded only, unknown", func(t *testing.T) {
		f.repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 2, LanguageCode: "en", Source: subtitle.SourceOpenSubtitles})
		rec := f.do(t, http.MethodGet, "/api/movies/2/subtitles", "")
		var got ItemSubtitleList
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusOK || len(got.Items) != 1 || got.Items[0].Match != "unknown" || got.Items[0].ReleaseName != nil {
			t.Errorf("status %d, body %s", rec.Code, rec.Body)
		}
	})

	t.Run("empty list is an array", func(t *testing.T) {
		rec := f.do(t, http.MethodGet, "/api/episodes/501/subtitles", "")
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"items":[]}` {
			t.Errorf("status %d, body %s", rec.Code, rec.Body)
		}
	})
}

func TestItemSubtitles_Search(t *testing.T) {
	fetcher := &subtitletest.Fetcher{Response: &opensubtitles.SearchResponse{Data: []opensubtitles.SubtitleResult{
		{Attributes: opensubtitles.SubtitleAttributes{Language: "en", DownloadCount: 5000, Release: "Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS",
			Files: []opensubtitles.SubtitleFile{{FileID: 11, FileName: "sparks.srt"}}}},
		{Attributes: opensubtitles.SubtitleAttributes{Language: "en", DownloadCount: 12, Release: subFlux, HearingImpaired: true,
			Files: []opensubtitles.SubtitleFile{{FileID: 12, FileName: "flux.srt"}}}},
	}}}
	f := newSubtitleFixture(t, fetcher)

	rec := f.do(t, http.MethodGet, "/api/movies/1/subtitles/search?languages=en,AZ", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var raw struct {
		Items       []json.RawMessage `json:"items"`
		ReleaseName *string           `json:"release_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if got := keys(t, rec.Body.Bytes()); !reflect.DeepEqual(got, []string{"items", "release_name"}) {
		t.Errorf("result keys = %v", got)
	}
	if got := keys(t, raw.Items[0]); !reflect.DeepEqual(got, candidateKeys) {
		t.Errorf("SubtitleCandidate keys = %v, want %v", got, candidateKeys)
	}
	if raw.ReleaseName == nil || *raw.ReleaseName != subFlux {
		t.Errorf("release_name = %v", raw.ReleaseName)
	}
	var first SubtitleCandidate
	_ = json.Unmarshal(raw.Items[0], &first)
	if first.FileID != 12 || first.Match != "exact_release" || first.MatchScore != 100 || !first.HearingImpaired {
		t.Errorf("first = %+v, want the exact release", first)
	}
	want := opensubtitles.SearchParams{Type: "movie", TMDBID: 693134, Languages: []string{"az", "en"}}
	if !reflect.DeepEqual(fetcher.Searches[0], want) {
		t.Errorf("search = %+v, want %+v", fetcher.Searches[0], want)
	}

	t.Run("episodes search by the show", func(t *testing.T) {
		rec := f.do(t, http.MethodGet, "/api/episodes/501/subtitles/search", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		got := fetcher.Searches[len(fetcher.Searches)-1]
		want := opensubtitles.SearchParams{Type: "episode", ParentTMDBID: 95396, SeasonNumber: 2, EpisodeNumber: 5, Languages: []string{"en"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("search = %+v, want %+v", got, want)
		}
	})

	t.Run("nothing assigned: unranked", func(t *testing.T) {
		rec := f.do(t, http.MethodGet, "/api/movies/2/subtitles/search?languages=en", "")
		var got ItemSubtitleSearch
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusOK || got.ReleaseName != nil {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		for _, c := range got.Items {
			if c.Match != "unknown" || c.MatchScore != 0 {
				t.Errorf("candidate %d = %s %d, want unknown 0", c.FileID, c.Match, c.MatchScore)
			}
		}
		if !strings.Contains(rec.Body.String(), `"release_name":null`) {
			t.Errorf("body %s, want release_name null", rec.Body)
		}
	})

	t.Run("bad language", func(t *testing.T) {
		rec := f.do(t, http.MethodGet, "/api/movies/1/subtitles/search?languages=en,../x", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("OpenSubtitles down", func(t *testing.T) {
		fetcher.SearchErr = errors.New("unexpected status 503: <html>secret upstream page</html>")
		defer func() { fetcher.SearchErr = nil }()
		rec := f.do(t, http.MethodGet, "/api/movies/1/subtitles/search", "")
		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rec.Code)
		}
		// The upstream detail goes to the log, not the client.
		if msg := decodeError(t, rec).Message; strings.Contains(msg, "secret") || strings.Contains(msg, "503") {
			t.Errorf("message = %q leaks the upstream error", msg)
		}
	})
}

func TestItemSubtitles_Download(t *testing.T) {
	fetcher := &subtitletest.Fetcher{Files: map[int]subtitletest.File{
		12: {Name: "Dune.Part.Two.FLUX.srt", Content: []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n")},
	}}
	f := newSubtitleFixture(t, fetcher)

	rec := f.do(t, http.MethodPost, "/api/movies/1/subtitles", `{"file_id":12,"language_code":"en","release_name":"`+subFlux+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := keys(t, rec.Body.Bytes()); !reflect.DeepEqual(got, subtitleKeys) {
		t.Errorf("Subtitle keys = %v", got)
	}
	var got ItemSubtitle
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Source != "opensubtitles" || got.LanguageCode != "en" || got.LanguageName != "English" || got.Match != "exact_release" ||
		got.ReleaseName == nil || *got.ReleaseName != subFlux || got.Format != "srt" {
		t.Errorf("subtitle = %+v", got)
	}
	if f.tree.invalidated != 1 {
		t.Errorf("tree invalidated %d times, want 1", f.tree.invalidated)
	}
	// The torrent's English subtitle stays beside it.
	if rows := f.repo.All(); len(rows) != 3 {
		t.Errorf("rows = %d, want 3", len(rows))
	}

	t.Run("nothing assigned still downloads", func(t *testing.T) {
		rec := f.do(t, http.MethodPost, "/api/movies/2/subtitles", `{"file_id":12,"language_code":"en"}`)
		var got ItemSubtitle
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusCreated || got.Match != "unknown" || got.ReleaseName != nil {
			t.Errorf("status %d, body %s", rec.Code, rec.Body)
		}
	})

	tests := []struct {
		name, body string
		want       int
	}{
		{"no file", `{"language_code":"en"}`, http.StatusBadRequest},
		{"not JSON", `nope`, http.StatusBadRequest},
		{"bad language", `{"file_id":12,"language_code":"../../x"}`, http.StatusBadRequest},
		{"OpenSubtitles fails", `{"file_id":99,"language_code":"en"}`, http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := f.do(t, http.MethodPost, "/api/movies/1/subtitles", tt.body); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestItemSubtitles_Delete(t *testing.T) {
	f := newSubtitleFixture(t, nil)
	path := func(id int64) string { return "/api/movies/1/subtitles/" + itoa(id) }

	rec := f.do(t, http.MethodDelete, path(f.torrent.ID), "")
	if rec.Code != http.StatusConflict || decodeError(t, rec).Code != CodeConflict {
		t.Errorf("torrent subtitle: status = %d %s, want 409 conflict", rec.Code, rec.Body)
	}
	// An earlier torrent's subtitle isn't listed, so it isn't found.
	stale := f.repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "de", Source: subtitle.SourceTorrent,
		InfoHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", FilePath: "Old/de.srt"})
	if rec := f.do(t, http.MethodDelete, path(stale.ID), ""); rec.Code != http.StatusNotFound {
		t.Errorf("an earlier torrent's subtitle: status = %d, want 404", rec.Code)
	}
	if rec := f.do(t, http.MethodDelete, "/api/episodes/501/subtitles/"+itoa(f.ru.ID), ""); rec.Code != http.StatusNotFound {
		t.Errorf("another item's subtitle: status = %d, want 404", rec.Code)
	}
	if rec := f.do(t, http.MethodDelete, "/api/movies/1/subtitles/abc", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: status = %d, want 400", rec.Code)
	}
	if f.tree.invalidated != 0 {
		t.Errorf("tree invalidated by failed deletes")
	}

	if rec := f.do(t, http.MethodDelete, path(f.ru.ID), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("downloaded subtitle: status = %d %s, want 204", rec.Code, rec.Body)
	}
	if f.tree.invalidated != 1 {
		t.Errorf("tree invalidated %d times, want 1", f.tree.invalidated)
	}
	if rec := f.do(t, http.MethodDelete, path(f.ru.ID), ""); rec.Code != http.StatusNotFound {
		t.Errorf("deleted twice: status = %d, want 404", rec.Code)
	}
}

func TestItemSubtitles_Errors(t *testing.T) {
	f := newSubtitleFixture(t, nil)
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"unknown movie", http.MethodGet, "/api/movies/99/subtitles", "", http.StatusNotFound},
		{"unknown episode", http.MethodGet, "/api/episodes/99/subtitles", "", http.StatusNotFound},
		{"unknown episode search", http.MethodGet, "/api/episodes/99/subtitles/search", "", http.StatusNotFound},
		{"unknown movie delete", http.MethodDelete, "/api/movies/99/subtitles/1", "", http.StatusNotFound},
		{"bad item id", http.MethodGet, "/api/movies/x/subtitles", "", http.StatusBadRequest},
		{"search without OpenSubtitles", http.MethodGet, "/api/movies/1/subtitles/search", "", http.StatusServiceUnavailable},
		{"download without OpenSubtitles", http.MethodPost, "/api/movies/1/subtitles", `{"file_id":1,"language_code":"en"}`, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := f.do(t, tt.method, tt.path, tt.body); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}

	t.Run("no subtitle service", func(t *testing.T) {
		s := newTestServer(Options{})
		if rec := do(t, s, http.MethodGet, "/api/movies/1/subtitles", nil); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
