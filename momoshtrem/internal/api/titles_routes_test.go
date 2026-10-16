package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// fakeCatalog returns err, or canned results, and records its arguments.
type fakeCatalog struct {
	err error

	tmdbID, season int
	showID         int64
	params         catalog.LibraryParams
	patch          metadata.SettingsPatch
}

var _ Catalog = (*fakeCatalog)(nil)

func (f *fakeCatalog) MovieTitle(_ context.Context, tmdbID int) (*catalog.MovieTitle, error) {
	f.tmdbID = tmdbID
	if f.err != nil {
		return nil, f.err
	}
	return &catalog.MovieTitle{TitleBase: catalog.TitleBase{MediaType: "movie", TMDBID: tmdbID, Title: "Dune"}}, nil
}

func (f *fakeCatalog) ShowTitle(_ context.Context, tmdbID int) (*catalog.ShowTitle, error) {
	f.tmdbID = tmdbID
	if f.err != nil {
		return nil, f.err
	}
	return &catalog.ShowTitle{TitleBase: catalog.TitleBase{MediaType: "tv", TMDBID: tmdbID, Title: "Frieren"}}, nil
}

func (f *fakeCatalog) Season(_ context.Context, tmdbID, season int) (*catalog.SeasonDetail, error) {
	f.tmdbID, f.season = tmdbID, season
	if f.err != nil {
		return nil, f.err
	}
	return &catalog.SeasonDetail{SeasonNumber: season, Name: "Season 1", Episodes: []catalog.EpisodeSummary{}, Ranges: []catalog.EpisodeRange{}}, nil
}

func (f *fakeCatalog) Library(_ context.Context, p catalog.LibraryParams) (*catalog.LibraryPage, error) {
	f.params = p
	if f.err != nil {
		return nil, f.err
	}
	// As the catalog does: malformed ids are an invalid query
	if p.IDs != "" {
		if _, _, err := catalog.ParseLibraryIDs(p.IDs); err != nil {
			return nil, err
		}
	}
	return &catalog.LibraryPage{Items: []catalog.LibraryItem{}, Total: 0, Page: 1, PageSize: 50}, nil
}

func (f *fakeCatalog) PatchShow(_ context.Context, id int64, patch metadata.SettingsPatch) (*catalog.ShowTitle, error) {
	f.showID, f.patch = id, patch
	if f.err != nil {
		return nil, f.err
	}
	return &catalog.ShowTitle{TitleBase: catalog.TitleBase{MediaType: "tv", Title: "Frieren"}, IncludeSpecials: false}, nil
}

func (f *fakeCatalog) EpisodeGroups(_ context.Context, id int64) ([]catalog.EpisodeGroup, error) {
	f.showID = id
	if f.err != nil {
		return nil, f.err
	}
	return []catalog.EpisodeGroup{{ID: "g1", Name: "Absolute", Type: 2, GroupCount: 1, EpisodeCount: 38}}, nil
}

func doBody(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// manyIDs is an ids parameter naming n movies.
func manyIDs(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("movie:%d", i+1)
	}
	return strings.Join(ids, ",")
}

// tmdbErr wraps a tmdb error the way the catalog does.
func tmdbErr(err error) error { return fmt.Errorf("%w: %w", library.ErrTMDBFetch, err) }

func TestTitleRoutes_Status(t *testing.T) {
	down := tmdbErr(&tmdb.StatusError{StatusCode: 503})
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		err        error
		nilCatalog bool
		wantStatus int
		wantCode   string
	}{
		{"movie", "GET", "/api/titles/movie/693134", "", nil, false, 200, ""},
		{"movie bad id", "GET", "/api/titles/movie/dune", "", nil, false, 400, CodeBadRequest},
		{"movie zero id", "GET", "/api/titles/movie/0", "", nil, false, 400, CodeBadRequest},
		{"movie unknown on TMDB", "GET", "/api/titles/movie/1", "", tmdbErr(tmdb.ErrNotFound), false, 404, CodeNotFound},
		{"movie TMDB down", "GET", "/api/titles/movie/1", "", down, false, 502, CodeTMDBUnavailable},
		{"movie no TMDB key", "GET", "/api/titles/movie/1", "", tmdbErr(tmdb.ErrNoAPIKey), false, 503, CodeUnavailable},
		{"movie database error", "GET", "/api/titles/movie/1", "", errors.New("conn reset"), false, 500, CodeInternalError},
		{"movie request canceled", "GET", "/api/titles/movie/1", "", fmt.Errorf("load movie: %w", context.Canceled), false, StatusClientClosedRequest, CodeClientClosed},
		{"movie without catalog", "GET", "/api/titles/movie/1", "", nil, true, 503, CodeUnavailable},
		{"unknown media type", "GET", "/api/titles/person/1", "", nil, false, 404, CodeNotFound},

		{"show", "GET", "/api/titles/tv/209867", "", nil, false, 200, ""},
		{"show TMDB down", "GET", "/api/titles/tv/209867", "", down, false, 502, CodeTMDBUnavailable},
		{"show no key", "GET", "/api/titles/tv/209867", "", tmdbErr(tmdb.ErrNoAPIKey), false, 503, CodeUnavailable},
		{"show unknown", "GET", "/api/titles/tv/209867", "", tmdbErr(tmdb.ErrNotFound), false, 404, CodeNotFound},

		{"season", "GET", "/api/titles/tv/209867/seasons/1", "", nil, false, 200, ""},
		{"specials season", "GET", "/api/titles/tv/209867/seasons/0", "", nil, false, 200, ""},
		{"season not in library", "GET", "/api/titles/tv/209867/seasons/1", "", catalog.ErrNotInLibrary, false, 404, CodeNotInLibrary},
		{"season unknown", "GET", "/api/titles/tv/209867/seasons/9", "", catalog.ErrSeasonNotFound, false, 404, CodeNotFound},
		{"season request canceled", "GET", "/api/titles/tv/209867/seasons/1", "", context.Canceled, false, StatusClientClosedRequest, CodeClientClosed},
		{"season negative", "GET", "/api/titles/tv/209867/seasons/-1", "", nil, false, 400, CodeBadRequest},
		{"season not a number", "GET", "/api/titles/tv/209867/seasons/one", "", nil, false, 400, CodeBadRequest},

		{"patch", "PATCH", "/api/shows/3", `{"include_specials": false}`, nil, false, 200, ""},
		{"patch invalid settings", "PATCH", "/api/shows/3", `{"episode_ordering": "episode_group"}`,
			fmt.Errorf("%w: needs a group", library.ErrInvalidShowSettings), false, 422, CodeValidationError},
		{"patch wrong type", "PATCH", "/api/shows/3", `{"include_specials": "yes"}`, nil, false, 422, CodeValidationError},
		{"patch not JSON", "PATCH", "/api/shows/3", `include_specials=false`, nil, false, 400, CodeBadRequest},
		{"patch empty body", "PATCH", "/api/shows/3", ``, nil, false, 400, CodeBadRequest},
		{"patch unknown show", "PATCH", "/api/shows/99", `{}`, library.ErrShowNotFound, false, 404, CodeNotFound},
		{"patch TMDB down", "PATCH", "/api/shows/3", `{"episode_group_id": "x"}`, down, false, 502, CodeTMDBUnavailable},
		{"patch without settings", "PATCH", "/api/shows/3", `{}`, catalog.ErrSettingsUnavailable, false, 503, CodeUnavailable},

		{"episode groups", "GET", "/api/shows/3/episode-groups", "", nil, false, 200, ""},
		{"episode groups unknown show", "GET", "/api/shows/3/episode-groups", "", library.ErrShowNotFound, false, 404, CodeNotFound},
		{"episode groups no key", "GET", "/api/shows/3/episode-groups", "", tmdbErr(tmdb.ErrNoAPIKey), false, 503, CodeUnavailable},
		{"episode groups TMDB down", "GET", "/api/shows/3/episode-groups", "", down, false, 502, CodeTMDBUnavailable},

		{"library", "GET", "/api/library", "", nil, false, 200, ""},
		{"library bad page", "GET", "/api/library?page=0", "", nil, false, 400, CodeBadRequest},
		{"library bad page size", "GET", "/api/library?page_size=lots", "", nil, false, 400, CodeBadRequest},
		{"library invalid filter", "GET", "/api/library?sort=rating", "", fmt.Errorf("%w: sort", catalog.ErrInvalidQuery), false, 400, CodeBadRequest},
		{"library database error", "GET", "/api/library", "", errors.New("conn reset"), false, 500, CodeInternalError},
		{"library request canceled", "GET", "/api/library", "", fmt.Errorf("list: %w", context.Canceled), false, StatusClientClosedRequest, CodeClientClosed},
		{"library without catalog", "GET", "/api/library", "", nil, true, 503, CodeUnavailable},
		{"library ids", "GET", "/api/library?ids=movie:603,tv:1399", "", nil, false, 200, ""},
		{"library ids unknown type", "GET", "/api/library?ids=person:1", "", nil, false, 400, CodeBadRequest},
		{"library ids not a number", "GET", "/api/library?ids=movie:matrix", "", nil, false, 400, CodeBadRequest},
		{"library ids without a type", "GET", "/api/library?ids=603", "", nil, false, 400, CodeBadRequest},
		{"library ids empty entry", "GET", "/api/library?ids=movie:603,,tv:1399", "", nil, false, 400, CodeBadRequest},
		{"library ids zero", "GET", "/api/library?ids=tv:0", "", nil, false, 400, CodeBadRequest},
		{"library ids over 200", "GET", "/api/library?ids=" + manyIDs(201), "", nil, false, 400, CodeBadRequest},
		{"library ids at 200", "GET", "/api/library?ids=" + manyIDs(200), "", nil, false, 200, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{}
			if !tt.nilCatalog {
				opts.Catalog = &fakeCatalog{err: tt.err}
			}
			rec := doBody(t, newTestServer(opts), tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec).Code; got != tt.wantCode {
					t.Errorf("code = %q, want %q", got, tt.wantCode)
				}
			}
		})
	}
}

func TestTitleRoutes_Bodies(t *testing.T) {
	cat := &fakeCatalog{}
	s := newTestServer(Options{Catalog: cat})

	rec := doBody(t, s, "GET", "/api/titles/tv/209867/seasons/2", "")
	var season catalog.SeasonDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &season); err != nil || cat.tmdbID != 209867 || cat.season != 2 || season.SeasonNumber != 2 {
		t.Errorf("season = %s (args %d %d)", rec.Body, cat.tmdbID, cat.season)
	}
	if !strings.Contains(rec.Body.String(), `"episodes":[]`) || !strings.Contains(rec.Body.String(), `"ranges":[]`) {
		t.Errorf("empty lists must be [] not null: %s", rec.Body)
	}

	rec = doBody(t, s, "GET", "/api/shows/3/episode-groups", "")
	if got := rec.Body.String(); got != `{"items":[{"id":"g1","name":"Absolute","type":2,"group_count":1,"episode_count":38}]}` || cat.showID != 3 {
		t.Errorf("episode groups = %s", got)
	}

	rec = doBody(t, s, "GET", "/api/library?type=tv&status=ready&sort=title&q=fr%25&page=3&page_size=500", "")
	want := catalog.LibraryParams{MediaType: "tv", Status: "ready", Sort: "title", Query: "fr%", Page: 3, PageSize: 500}
	if rec.Code != 200 || cat.params != want {
		t.Errorf("library params = %+v, want %+v", cat.params, want)
	}
	if got := rec.Body.String(); got != `{"items":[],"total":0,"page":1,"page_size":50,`+
		`"counts":{"all":0,"movie":0,"tv":0,"watching":0,"ready":0,"incomplete":0,"needs_review":0},`+
		`"summary":{"total":0,"size_bytes":0,"ready":0}}` {
		t.Errorf("library body = %s", got)
	}

	doBody(t, s, "GET", "/api/library?ids=movie:603,tv:1399&q=matrix", "")
	if want := (catalog.LibraryParams{Query: "matrix", IDs: "movie:603,tv:1399"}); cat.params != want {
		t.Errorf("library lookup params = %+v, want %+v", cat.params, want)
	}
}

func TestPatchShow_Body(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		body string
		want metadata.SettingsPatch
	}{
		{"empty object keeps everything", `{}`, metadata.SettingsPatch{}},
		{"null group clears it", `{"episode_group_id": null}`, metadata.SettingsPatch{EpisodeGroupID: str("")}},
		{"group set", `{"episode_ordering": "episode_group", "episode_group_id": "5eb7"}`, metadata.SettingsPatch{
			EpisodeOrdering: func() *library.EpisodeOrdering { o := library.OrderingEpisodeGroup; return &o }(),
			EpisodeGroupID:  str("5eb7"),
		}},
		{"specials and classifier", `{"include_specials": true, "classifier_mode": "rules_only", "unknown": 1}`, metadata.SettingsPatch{
			IncludeSpecials: func() *bool { b := true; return &b }(),
			ClassifierMode:  func() *library.ClassifierMode { m := library.ClassifierRulesOnly; return &m }(),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := &fakeCatalog{}
			rec := doBody(t, newTestServer(Options{Catalog: cat}), "PATCH", "/api/shows/3", tt.body)
			if rec.Code != http.StatusOK || cat.showID != 3 {
				t.Fatalf("status %d (show %d): %s", rec.Code, cat.showID, rec.Body)
			}
			if !reflect.DeepEqual(cat.patch, tt.want) {
				t.Errorf("patch = %+v, want %+v", cat.patch, tt.want)
			}
		})
	}
}
