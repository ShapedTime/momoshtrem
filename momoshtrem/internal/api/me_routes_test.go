package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/me"
)

// fakeMe records what each /me route asked for and answers with err, or
// empty lists.
type fakeMe struct {
	days, page, pageSize int
	mediaType            string
	err                  error
}

func (f *fakeMe) UpNext(context.Context) ([]me.UpNextItem, error) {
	return []me.UpNextItem{}, f.err
}

func (f *fakeMe) NewEpisodes(_ context.Context, days int) ([]me.NewEpisodeItem, error) {
	f.days = days
	return []me.NewEpisodeItem{}, f.err
}

func (f *fakeMe) Watchlist(_ context.Context, mediaType string) ([]me.WatchlistItem, error) {
	f.mediaType = mediaType
	return []me.WatchlistItem{}, f.err
}

func (f *fakeMe) History(_ context.Context, page, pageSize int) (*me.HistoryPage, error) {
	f.page, f.pageSize = page, pageSize
	if f.err != nil {
		return nil, f.err
	}
	return &me.HistoryPage{Items: []me.HistoryItem{}, Page: page, PageSize: pageSize}, nil
}

type fakeNudger struct{ n int }

func (f *fakeNudger) Nudge() { f.n++ }

func TestMeRoutes(t *testing.T) {
	f, nudge := &fakeMe{}, &fakeNudger{}
	s := newTestServer(Options{Me: f, TraktNudge: nudge})

	rec := do(t, s, "GET", "/api/me/up-next", nil)
	if rec.Code != 200 || rec.Body.String() != `{"items":[]}` || nudge.n != 1 {
		t.Errorf("up next = %d %s, nudges %d", rec.Code, rec.Body, nudge.n)
	}

	for path, want := range map[string]int{"/api/me/new-episodes": 7, "/api/me/new-episodes?days=1": 1, "/api/me/new-episodes?days=30": 30} {
		if rec := do(t, s, "GET", path, nil); rec.Code != 200 || f.days != want {
			t.Errorf("%s = %d, days %d; want %d", path, rec.Code, f.days, want)
		}
	}
	for path, want := range map[string]string{"/api/me/watchlist": "", "/api/me/watchlist?type=movie": "movie", "/api/me/watchlist?type=tv": "tv"} {
		if rec := do(t, s, "GET", path, nil); rec.Code != 200 || f.mediaType != want {
			t.Errorf("%s = %d, type %q; want %q", path, rec.Code, f.mediaType, want)
		}
	}
	rec = do(t, s, "GET", "/api/me/history", nil)
	if rec.Code != 200 || f.page != 1 || f.pageSize != me.DefaultHistoryPageSize {
		t.Errorf("history = %d, page %d of %d", rec.Code, f.page, f.pageSize)
	}
	var page map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page["items"] == nil || page["page_size"] != float64(50) {
		t.Errorf("history body = %s", rec.Body)
	}
	if rec := do(t, s, "GET", "/api/me/history?page=3&page_size=200", nil); rec.Code != 200 || f.page != 3 || f.pageSize != 200 {
		t.Errorf("history page 3 = %d, page %d of %d", rec.Code, f.page, f.pageSize)
	}
	if nudge.n != 1 {
		t.Errorf("nudges = %d; only up next nudges", nudge.n)
	}
}

func TestMeRoutes_BadParams(t *testing.T) {
	s := newTestServer(Options{Me: &fakeMe{}})
	for _, path := range []string{
		"/api/me/new-episodes?days=0", "/api/me/new-episodes?days=31", "/api/me/new-episodes?days=abc", "/api/me/new-episodes?days=7.5",
		"/api/me/watchlist?type=show", "/api/me/watchlist?type=episode",
		"/api/me/history?page=0", "/api/me/history?page=x", "/api/me/history?page_size=201", "/api/me/history?page_size=0",
	} {
		rec := do(t, s, "GET", path, nil)
		var body ErrorBody
		if rec.Code != 400 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error.Code != CodeBadRequest {
			t.Errorf("%s = %d %s, want 400 bad_request", path, rec.Code, rec.Body)
		}
	}
}

func TestMeRoutes_Unavailable(t *testing.T) {
	s := newTestServer(Options{}) // no /me, no nudger: nothing panics
	for _, path := range []string{"/api/me/up-next", "/api/me/new-episodes", "/api/me/watchlist", "/api/me/history"} {
		if rec := do(t, s, "GET", path, nil); rec.Code != 503 {
			t.Errorf("%s = %d, want 503", path, rec.Code)
		}
	}

	s = newTestServer(Options{Me: &fakeMe{err: errors.New("db down")}})
	if rec := do(t, s, "GET", "/api/me/up-next", nil); rec.Code != 500 {
		t.Errorf("failure = %d, want 500", rec.Code)
	}
	s = newTestServer(Options{Me: &fakeMe{err: context.Canceled}})
	if rec := do(t, s, "GET", "/api/me/history", nil); rec.Code != StatusClientClosedRequest {
		t.Errorf("canceled = %d, want %d", rec.Code, StatusClientClosedRequest)
	}
}
