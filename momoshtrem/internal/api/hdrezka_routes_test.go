package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/service"
)

// fakeHDRezka records what each route asked and answers err.
type fakeHDRezka struct {
	err      error
	called   string
	search   [3]any // media type, tmdb id, q
	title    service.HDRezkaTitleRequest
	streams  service.HDRezkaStreamsRequest
	id       int64
	movie    service.HDRezkaAssign
	show     service.HDRezkaShowAssign
	assigned *service.HDRezkaShowResult
}

func (f *fakeHDRezka) Search(_ context.Context, mediaType string, tmdbID int, q string) (*service.HDRezkaSearch, error) {
	f.called, f.search = "search", [3]any{mediaType, tmdbID, q}
	if f.err != nil {
		return nil, f.err
	}
	return &service.HDRezkaSearch{Items: []service.HDRezkaSearchItem{}, Query: q}, nil
}

func (f *fakeHDRezka) Title(_ context.Context, req service.HDRezkaTitleRequest) (*service.HDRezkaTitle, error) {
	f.called, f.title = "title", req
	if f.err != nil {
		return nil, f.err
	}
	return &service.HDRezkaTitle{Kind: "movie", Translators: []service.HDRezkaTranslator{}, Seasons: []service.HDRezkaSeason{}}, nil
}

func (f *fakeHDRezka) Streams(_ context.Context, req service.HDRezkaStreamsRequest) (*service.HDRezkaStreams, error) {
	f.called, f.streams = "streams", req
	if f.err != nil {
		return nil, f.err
	}
	return &service.HDRezkaStreams{Qualities: []service.HDRezkaQuality{}, Subtitles: []service.HDRezkaSubtitle{}}, nil
}

func (f *fakeHDRezka) AssignMovie(_ context.Context, id int64, req service.HDRezkaAssign) (*service.HDRezkaAssigned, error) {
	f.called, f.id, f.movie = "assign-movie", id, req
	if f.err != nil {
		return nil, f.err
	}
	return &service.HDRezkaAssigned{Quality: req.Quality}, nil
}

func (f *fakeHDRezka) AssignShow(_ context.Context, id int64, req service.HDRezkaShowAssign) (*service.HDRezkaShowResult, error) {
	f.called, f.id, f.show = "assign-show", id, req
	if f.err != nil {
		return nil, f.err
	}
	if f.assigned != nil {
		return f.assigned, nil
	}
	return &service.HDRezkaShowResult{Assigned: []service.HDRezkaAssigned{}, Failed: []service.HDRezkaFailed{}}, nil
}

func hdrezkaFailure(code string) error {
	return &service.HDRezkaError{Code: code, Message: "no: " + code, Err: errors.New("cause at 10.0.0.3")}
}

const assignBody = `{"url": "/films/fiction/2259-interstellar-2014.html", "translator_id": 238, "quality": "1080p"}`

var hdrezkaRoutes = []struct {
	method, path, body string
}{
	{"GET", "/api/hdrezka/search?media_type=movie&tmdb_id=157336", ""},
	{"GET", "/api/hdrezka/titles?url=x", ""},
	{"GET", "/api/hdrezka/streams?url=x&translator_id=238", ""},
	{"POST", "/api/movies/7/assign-hdrezka", assignBody},
	{"POST", "/api/shows/5/assign-hdrezka", assignBody},
}

func TestHDRezkaRoutes_Disabled(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range hdrezkaRoutes {
		rec := doBody(t, s, r.method, r.path, r.body)
		if rec.Code != http.StatusServiceUnavailable || decodeError(t, rec).Code != CodeUnavailable {
			t.Errorf("%s %s = %d %s, want 503 unavailable", r.method, r.path, rec.Code, rec.Body)
		}
	}
	// Even a bad id: the routes are off.
	if rec := doBody(t, s, "POST", "/api/movies/x/assign-hdrezka", assignBody); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("bad id while off = %d", rec.Code)
	}
}

func TestHDRezkaRoutes_ErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{hdrezkaFailure(service.HDRezkaBadRequest), 400, CodeBadRequest},
		{hdrezkaFailure(service.HDRezkaNotFound), 404, CodeNotFound},
		{hdrezkaFailure(service.HDRezkaPremiumOnly), 409, CodePremiumOnly},
		{hdrezkaFailure(service.HDRezkaInvalid), 422, CodeValidationError},
		{hdrezkaFailure(service.HDRezkaUnavailable), 502, CodeHDRezkaUnavailable},
		{hdrezkaFailure(service.HDRezkaTMDBDown), 502, CodeTMDBUnavailable},
		{hdrezkaFailure(service.HDRezkaBlocked), 503, CodeHDRezkaBlocked},
		{hdrezkaFailure(service.HDRezkaOff), 503, CodeUnavailable},
		{hdrezkaFailure(service.HDRezkaTimeout), 504, CodeTimeout},
		{hdrezkaFailure("forbidden"), 500, CodeInternalError},
		{context.Canceled, StatusClientClosedRequest, CodeClientClosed},
		{errors.New("pq: connection to 10.0.0.3 refused"), 500, CodeInternalError},
	}
	for _, tt := range tests {
		for _, r := range hdrezkaRoutes {
			f := &fakeHDRezka{err: tt.err}
			s := newTestServer(Options{HDRezka: f})
			rec := doBody(t, s, r.method, r.path, r.body)
			if rec.Code != tt.status {
				t.Errorf("%v on %s = %d, want %d", tt.err, r.path, rec.Code, tt.status)
				continue
			}
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("%s answered %d: the web proxy would read it as a bad token", r.path, rec.Code)
			}
			got := decodeError(t, rec)
			if got.Code != tt.code || strings.Contains(got.Message, "10.0.0.3") {
				t.Errorf("%v on %s = %+v, want code %s", tt.err, r.path, got, tt.code)
			}
		}
	}
}

func TestHDRezkaRoutes_Requests(t *testing.T) {
	f := &fakeHDRezka{}
	s := newTestServer(Options{HDRezka: f})

	rec := doBody(t, s, "GET", "/api/hdrezka/search?media_type=tv&tmdb_id=1396&q=breaking+bad", "")
	if rec.Code != 200 || f.search != [3]any{"tv", 1396, "breaking bad"} {
		t.Errorf("search = %d %v", rec.Code, f.search)
	}

	rec = doBody(t, s, "GET", "/api/hdrezka/titles?url=https%3A%2F%2Frezka.ag%2Fseries%2Fx%2F646-a.html&translator_id=238&show_id=5", "")
	want := service.HDRezkaTitleRequest{URL: "https://rezka.ag/series/x/646-a.html", TranslatorID: 238, ShowID: 5}
	if rec.Code != 200 || f.title != want {
		t.Errorf("title = %d %+v", rec.Code, f.title)
	}

	rec = doBody(t, s, "GET", "/api/hdrezka/streams?url=x&translator_id=56&director=true&camrip=0&season=0&episode=3", "")
	r := f.streams
	if rec.Code != 200 || r.TranslatorID != 56 || r.Cut.Director == nil || !*r.Cut.Director || r.Cut.Camrip == nil || *r.Cut.Camrip ||
		r.Cut.Ads != nil || r.Season == nil || *r.Season != 0 || *r.Episode != 3 {
		t.Errorf("streams = %d %+v", rec.Code, r)
	}
	doBody(t, s, "GET", "/api/hdrezka/streams?url=x&translator_id=56", "")
	if f.streams.Season != nil || f.streams.Episode != nil || f.streams.Cut != (service.TranslatorCut{}) {
		t.Errorf("streams without options = %+v", f.streams)
	}

	rec = doBody(t, s, "POST", "/api/movies/7/assign-hdrezka",
		`{"url": "u", "translator_id": 56, "director": true, "quality": "1080p"}`)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || f.id != 7 || f.movie.URL != "u" || f.movie.TranslatorID != 56 ||
		f.movie.Director == nil || !*f.movie.Director || f.movie.Camrip != nil || f.movie.Quality != "1080p" {
		t.Errorf("assign movie = %d %s %+v", rec.Code, rec.Body, f.movie)
	}

	f.assigned = &service.HDRezkaShowResult{
		Assigned: []service.HDRezkaAssigned{{EpisodeID: 501, Quality: "1080p", SizeBytes: 1145043718}},
		Failed:   []service.HDRezkaFailed{{EpisodeID: 502, Code: "premium_only", Message: "Premium"}},
	}
	rec = doBody(t, s, "POST", "/api/shows/5/assign-hdrezka",
		`{"url": "u", "translator_id": 238, "quality": "1080p", "items": [{"episode_id": 501, "season": 1, "episode": 1}, {"episode_id": 502, "season": 1, "episode": 2}]}`)
	wantItems := []service.HDRezkaAssignItem{{EpisodeID: 501, Season: 1, Episode: 1}, {EpisodeID: 502, Season: 1, Episode: 2}}
	if rec.Code != 200 || f.id != 5 || f.show.TranslatorID != 238 || !reflect.DeepEqual(f.show.Items, wantItems) {
		t.Errorf("assign show = %d %+v", rec.Code, f.show)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want2 := `{"assigned":[{"episode_id":501,"quality":"1080p","size_bytes":1145043718}],"failed":[{"episode_id":502,"code":"premium_only","message":"Premium"}]}`
	if strings.TrimSpace(rec.Body.String()) != want2 {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestHDRezkaRoutes_BadRequests(t *testing.T) {
	tests := []struct{ method, path, body string }{
		{"GET", "/api/hdrezka/search?media_type=tv&tmdb_id=abc", ""},
		{"GET", "/api/hdrezka/titles?url=x&translator_id=-1", ""},
		{"GET", "/api/hdrezka/titles?url=x&show_id=five", ""},
		{"GET", "/api/hdrezka/streams?url=x&translator_id=56&season=one&episode=1", ""},
		{"GET", "/api/hdrezka/streams?url=x&translator_id=56&ads=maybe", ""},
		{"POST", "/api/movies/x/assign-hdrezka", assignBody},
		{"POST", "/api/movies/7/assign-hdrezka", `{"url":`},
		{"POST", "/api/movies/7/assign-hdrezka", `{"url": "u", "translator_id": "238", "quality": "1080p"}`},
		{"POST", "/api/shows/5/assign-hdrezka", `{"url": "u", "translator_id": 238, "quality": "1080p", "items": {}}`},
		{"POST", "/api/shows/5/assign-hdrezka", ""},
	}
	for _, tt := range tests {
		f := &fakeHDRezka{}
		s := newTestServer(Options{HDRezka: f})
		rec := doBody(t, s, tt.method, tt.path, tt.body)
		if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != CodeBadRequest || f.called != "" {
			t.Errorf("%s %s %s = %d (called %q), want 400 before the service", tt.method, tt.path, tt.body, rec.Code, f.called)
		}
	}
}
