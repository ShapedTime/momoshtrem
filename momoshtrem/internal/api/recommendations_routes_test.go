package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// fakeRecs answers each call with err, or a value: zero values, nil lists
// included, which the handlers must still answer as [].
type fakeRecs struct {
	err       error
	home      *recommend.Home
	limits    []int
	runExists bool
}

func (f *fakeRecs) Home(context.Context) (*recommend.Home, error) {
	if f.home != nil {
		return f.home, f.err
	}
	return &recommend.Home{}, f.err
}

func (f *fakeRecs) Refresh(context.Context) (int64, error) { return 43, f.err }

func (f *fakeRecs) Runs(_ context.Context, limit int) ([]recommend.RunSummary, error) {
	f.limits = append(f.limits, limit)
	return nil, f.err
}

func (f *fakeRecs) Run(_ context.Context, id int64) (*recommend.RunDetail, error) {
	if !f.runExists {
		return nil, f.err
	}
	return &recommend.RunDetail{RunProgress: recommend.RunProgress{RunSummary: recommend.RunSummary{ID: id, Status: recommend.StatusQueued}}}, f.err
}

var _ Recommendations = (*fakeRecs)(nil)

// fakeFeedback records the feedback it's given.
type fakeFeedback struct {
	err      error
	feedback []recommend.Feedback
	cleared  []string
}

func (f *fakeFeedback) SetFeedback(_ context.Context, fb recommend.Feedback) error {
	f.feedback = append(f.feedback, fb)
	return f.err
}

func (f *fakeFeedback) ClearFeedback(_ context.Context, mediaType string, tmdbID int) error {
	f.cleared = append(f.cleared, fmt.Sprintf("%s:%d", mediaType, tmdbID))
	return f.err
}

var _ RecFeedback = (*fakeFeedback)(nil)

// errorOf reads an error body.
func errorOf(t *testing.T, body []byte) ErrorDetail {
	t.Helper()
	var e ErrorBody
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("not an error body: %s", body)
	}
	return e.Error
}

// Until the slices that implement them land (#153, #154, #157, #158),
// every new route answers 503 unavailable.
func TestAIRoutes_Unavailable(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/recommendations/home", ""},
		{"POST", "/api/recommendations/refresh", ""},
		{"GET", "/api/recommendations/runs", ""},
		{"GET", "/api/recommendations/runs/42", ""},
		{"POST", "/api/recommendations/feedback", `{"media_type":"tv","tmdb_id":95396,"signal":"like","source":"shelf"}`},
		{"DELETE", "/api/recommendations/feedback/tv/95396", ""},
		{"GET", "/api/recommendations/threads", ""},
		{"GET", "/api/recommendations/threads/12", ""},
		{"POST", "/api/recommendations/ask", `{"message":"something like Severance but lighter"}`},
		{"GET", "/api/settings/ai", ""},
		{"PUT", "/api/settings/ai", `{"rec_schedule":"weekly"}`},
		{"GET", "/api/settings/ai/usage", ""},
	} {
		rec := doBody(t, s, r.method, r.path, r.body)
		if rec.Code != 503 || errorOf(t, rec.Body.Bytes()).Code != CodeUnavailable {
			t.Errorf("%s %s = %d %s, want 503 unavailable", r.method, r.path, rec.Code, rec.Body)
		}
	}
}

func TestRecommendationRoutes(t *testing.T) {
	f := &fakeRecs{runExists: true}
	s := newTestServer(Options{Recommendations: f})

	if rec := doBody(t, s, "POST", "/api/recommendations/refresh", ""); rec.Code != 202 || rec.Body.String() != `{"run_id":43}` {
		t.Errorf("refresh = %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/runs", ""); rec.Code != 200 || rec.Body.String() != `{"items":[]}` {
		t.Errorf("runs = %d %s", rec.Code, rec.Body)
	}
	doBody(t, s, "GET", "/api/recommendations/runs?limit=50", "")
	if fmt.Sprint(f.limits) != "[10 50]" {
		t.Errorf("limits = %v, want [10 50]", f.limits)
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/runs/9", ""); rec.Code != 200 {
		t.Errorf("run 9 = %d", rec.Code)
	}
	f.runExists = false
	if rec := doBody(t, s, "GET", "/api/recommendations/runs/9", ""); rec.Code != 404 {
		t.Errorf("unknown run = %d, want 404", rec.Code)
	}
	// Feedback is its own dependency: without it, 503 while the picks answer
	if rec := doBody(t, s, "DELETE", "/api/recommendations/feedback/tv/1", ""); rec.Code != 503 {
		t.Errorf("feedback without RecFeedback = %d, want 503", rec.Code)
	}
}

// Lists are always arrays: a service's nil slices answer [], never null.
func TestRecommendationRoutes_EmptyLists(t *testing.T) {
	f := &fakeRecs{runExists: true}
	s := newTestServer(Options{Recommendations: f})

	rec := doBody(t, s, "GET", "/api/recommendations/home", "")
	if want := `{"configured":false,"enabled":false,"run":null,"active_run":null,"hero_picks":[],"shelves":[]}`; rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("zero home = %d %s, want %s", rec.Code, rec.Body, want)
	}

	f.home = &recommend.Home{
		Configured: true,
		Enabled:    true,
		ActiveRun:  &recommend.RunProgress{RunSummary: recommend.RunSummary{ID: 43, Status: recommend.StatusQueued}},
		Shelves:    []recommend.Shelf{{Slug: "anime", Kind: recommend.ShelfAnime, Title: "Anime you’ll like"}},
	}
	var home struct {
		ActiveRun struct{ Steps json.RawMessage }   `json:"active_run"`
		Shelves   []struct{ Items json.RawMessage } `json:"shelves"`
	}
	rec = doBody(t, s, "GET", "/api/recommendations/home", "")
	if json.Unmarshal(rec.Body.Bytes(), &home) != nil || string(home.ActiveRun.Steps) != "[]" || len(home.Shelves) != 1 || string(home.Shelves[0].Items) != "[]" {
		t.Errorf("home = %s", rec.Body)
	}

	rec = doBody(t, s, "GET", "/api/recommendations/runs/7", "")
	var run map[string]json.RawMessage
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &run) != nil || string(run["id"]) != "7" || string(run["steps"]) != "[]" || run["usage"] == nil {
		t.Errorf("zero run = %d %s", rec.Code, rec.Body)
	}
}

func TestRecommendationFeedbackRoutes(t *testing.T) {
	f := &fakeFeedback{}
	s := newTestServer(Options{RecFeedback: f}) // feedback lands before the picks (#153)

	body := `{"media_type":"movie","tmdb_id":666277,"signal":"not_interested","source":"hero"}`
	if rec := doBody(t, s, "POST", "/api/recommendations/feedback", body); rec.Code != 204 {
		t.Errorf("feedback = %d %s", rec.Code, rec.Body)
	}
	if want := (recommend.Feedback{MediaType: "movie", TMDBID: 666277, Signal: "not_interested", Source: "hero"}); len(f.feedback) != 1 || f.feedback[0] != want {
		t.Errorf("feedback = %+v", f.feedback)
	}
	if rec := doBody(t, s, "DELETE", "/api/recommendations/feedback/movie/666277", ""); rec.Code != 204 || fmt.Sprint(f.cleared) != "[movie:666277]" {
		t.Errorf("delete feedback = %d, cleared %v", rec.Code, f.cleared)
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/home", ""); rec.Code != 503 {
		t.Errorf("home without Recommendations = %d, want 503", rec.Code)
	}

	s = newTestServer(Options{RecFeedback: &fakeFeedback{err: errors.New("db down")}})
	if rec := doBody(t, s, "POST", "/api/recommendations/feedback", body); rec.Code != 500 {
		t.Errorf("failure = %d, want 500", rec.Code)
	}
}

func TestRecommendationRoutes_BadRequests(t *testing.T) {
	f := &fakeRecs{}
	fb := &fakeFeedback{}
	s := newTestServer(Options{Recommendations: f, RecFeedback: fb})
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/recommendations/runs?limit=0", ""},
		{"GET", "/api/recommendations/runs?limit=51", ""},
		{"GET", "/api/recommendations/runs?limit=ten", ""},
		{"GET", "/api/recommendations/runs/abc", ""},
		{"POST", "/api/recommendations/feedback", `not json`},
		{"POST", "/api/recommendations/feedback", `{"media_type":"book","tmdb_id":1,"signal":"like","source":"shelf"}`},
		{"POST", "/api/recommendations/feedback", `{"media_type":"tv","tmdb_id":0,"signal":"like","source":"shelf"}`},
		{"POST", "/api/recommendations/feedback", `{"media_type":"tv","tmdb_id":1.5,"signal":"like","source":"shelf"}`},
		{"POST", "/api/recommendations/feedback", `{"media_type":"tv","tmdb_id":1,"signal":"love","source":"shelf"}`},
		{"POST", "/api/recommendations/feedback", `{"media_type":"tv","tmdb_id":1,"signal":"like"}`},
		{"DELETE", "/api/recommendations/feedback/book/1", ""},
		{"DELETE", "/api/recommendations/feedback/tv/-3", ""},
	} {
		if rec := doBody(t, s, r.method, r.path, r.body); rec.Code != 400 {
			t.Errorf("%s %s %s = %d, want 400", r.method, r.path, r.body, rec.Code)
		}
	}
	if len(fb.feedback)+len(fb.cleared)+len(f.limits) != 0 {
		t.Error("a bad request reached the service")
	}
}

func TestRecommendationRoutes_Errors(t *testing.T) {
	resets := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("CET", 3600))
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"no key", recommend.ErrUnavailable, 503, CodeUnavailable},
		{"picks off", recommend.ErrDisabled, 409, CodeRecsDisabled},
		{"no history", recommend.ErrNoHistory, 409, CodeNoHistory},
		{"budget", fmt.Errorf("refresh: %w", &recommend.LimitError{Code: recommend.CodeBudgetExceeded, ResetsAt: resets}), 429, recommend.CodeBudgetExceeded},
		{"canceled", context.Canceled, StatusClientClosedRequest, CodeClientClosed},
		{"other", errors.New("db down"), 500, CodeInternalError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(Options{Recommendations: &fakeRecs{err: tt.err}})
			rec := doBody(t, s, "POST", "/api/recommendations/refresh", "")
			e := errorOf(t, rec.Body.Bytes())
			if rec.Code != tt.status || e.Code != tt.code {
				t.Errorf("= %d %s, want %d %s", rec.Code, rec.Body, tt.status, tt.code)
			}
			if wantResets := tt.status == 429; wantResets != (e.ResetsAt != nil) || (wantResets && !e.ResetsAt.Equal(resets)) {
				t.Errorf("resets_at = %v", e.ResetsAt)
			}
		})
	}
	s := newTestServer(Options{Recommendations: &fakeRecs{err: recommend.ErrNotFound}})
	if rec := doBody(t, s, "GET", "/api/recommendations/runs/9", ""); rec.Code != 404 {
		t.Errorf("not found = %d", rec.Code)
	}
}
