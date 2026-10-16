package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/activity"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// fakeActivity answers err (or a fixed result) and records the call.
type fakeActivity struct {
	err    error
	called string
	hash   string
}

func (f *fakeActivity) List(context.Context) ([]activity.Item, error) {
	f.called = "list"
	if f.err != nil {
		return nil, f.err
	}
	return []activity.Item{{InfoHash: "aa", State: activity.StateIdle, Titles: []activity.Title{}, Streaming: []activity.StreamingSession{}}}, nil
}

func (f *fakeActivity) Summary(context.Context) (*activity.Summary, error) {
	f.called = "summary"
	if f.err != nil {
		return nil, f.err
	}
	return &activity.Summary{Counts: activity.Counts{Idle: 1}, Streaming: []activity.StreamingSession{}}, nil
}

func (f *fakeActivity) Pause(_ context.Context, hash string) (*activity.Item, error) {
	return f.set("pause", hash)
}

func (f *fakeActivity) Resume(_ context.Context, hash string) (*activity.Item, error) {
	return f.set("resume", hash)
}

func (f *fakeActivity) set(call, hash string) (*activity.Item, error) {
	f.called, f.hash = call, hash
	if f.err != nil {
		return nil, f.err
	}
	return &activity.Item{InfoHash: hash, State: activity.StatePaused}, nil
}

func (f *fakeActivity) PauseAll(context.Context) (int, error) {
	f.called = "pause-all"
	return 2, f.err
}

func (f *fakeActivity) ResumeAll(context.Context) (int, error) {
	f.called = "resume-all"
	return 3, f.err
}

func TestTorrentRoutes(t *testing.T) {
	const hex = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	tests := []struct {
		name       string
		method     string
		path       string
		err        error
		wantStatus int
		wantCode   string
		wantCall   string // "" when the handler mustn't call
		wantHash   string
		wantBody   string // a JSON object the body must contain, field for field
	}{
		{name: "list", method: http.MethodGet, path: "/api/torrents",
			wantStatus: http.StatusOK, wantCall: "list", wantBody: `{"items":[{"info_hash":"aa"}]}`},
		{name: "list, database down", method: http.MethodGet, path: "/api/torrents", err: errors.New("connection reset"),
			wantStatus: http.StatusInternalServerError, wantCode: CodeInternalError, wantCall: "list"},
		{name: "summary is not a hash", method: http.MethodGet, path: "/api/torrents/summary",
			wantStatus: http.StatusOK, wantCall: "summary", wantBody: `{"counts":{"idle":1}}`},
		{name: "summary, database down", method: http.MethodGet, path: "/api/torrents/summary", err: errors.New("connection reset"),
			wantStatus: http.StatusInternalServerError, wantCode: CodeInternalError, wantCall: "summary"},
		{name: "pause all is not a hash", method: http.MethodPost, path: "/api/torrents/pause-all",
			wantStatus: http.StatusOK, wantCall: "pause-all", wantBody: `{"count":2}`},
		{name: "resume all", method: http.MethodPost, path: "/api/torrents/resume-all",
			wantStatus: http.StatusOK, wantCall: "resume-all", wantBody: `{"count":3}`},
		{name: "pause all fails", method: http.MethodPost, path: "/api/torrents/pause-all", err: errors.New("connection reset"),
			wantStatus: http.StatusInternalServerError, wantCode: CodeInternalError, wantCall: "pause-all"},

		{name: "pause, uppercase hex", method: http.MethodPost, path: "/api/torrents/DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C/pause",
			wantStatus: http.StatusOK, wantCall: "pause", wantHash: hex, wantBody: `{"info_hash":"` + hex + `","state":"paused"}`},
		{name: "resume, base32", method: http.MethodPost, path: "/api/torrents/3WBFL3G4PSSV7MF37AJSHWDQMLNR63I4/resume",
			wantStatus: http.StatusOK, wantCall: "resume", wantHash: hex},
		{name: "pause, malformed hash", method: http.MethodPost, path: "/api/torrents/not-a-hash/pause",
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "a static route's name is no hash", method: http.MethodPost, path: "/api/torrents/pause-all/pause",
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "pause unknown", method: http.MethodPost, path: "/api/torrents/" + hex + "/pause", err: torrent.ErrTorrentNotFound,
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantCall: "pause", wantHash: hex},
		{name: "resume, invalid hash from the service", method: http.MethodPost, path: "/api/torrents/" + hex + "/resume",
			err: fmt.Errorf("%w: %q", torrent.ErrInvalidInfoHash, "x"),
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest, wantCall: "resume", wantHash: hex},
		{name: "resume, database down", method: http.MethodPost, path: "/api/torrents/" + hex + "/resume", err: errors.New("connection reset"),
			wantStatus: http.StatusInternalServerError, wantCode: CodeInternalError, wantCall: "resume", wantHash: hex},

		// /torrents/:hash routes still take a hash beside the static ones.
		{name: "debug under summary is a hash", method: http.MethodGet, path: "/api/torrents/summary/debug",
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "legacy status needs the torrent service", method: http.MethodGet, path: "/api/torrents/" + hex,
			wantStatus: http.StatusServiceUnavailable, wantCode: CodeUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeActivity{err: tt.err}
			rec := do(t, newTestServer(Options{Activity: fake}), tt.method, tt.path, nil)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec); got.Code != tt.wantCode {
					t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
				}
			}
			if fake.called != tt.wantCall || fake.hash != tt.wantHash {
				t.Fatalf("called (%q, %q), want (%q, %q)", fake.called, fake.hash, tt.wantCall, tt.wantHash)
			}
			if tt.wantBody != "" {
				assertJSONContains(t, rec.Body.Bytes(), tt.wantBody)
			}
		})
	}
}

func TestTorrentRoutes_Unconfigured(t *testing.T) {
	const hex = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	s := newTestServer(Options{})
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/torrents"},
		{http.MethodGet, "/api/torrents/summary"},
		{http.MethodPost, "/api/torrents/pause-all"},
		{http.MethodPost, "/api/torrents/resume-all"},
		{http.MethodPost, "/api/torrents/" + hex + "/pause"},
		{http.MethodPost, "/api/torrents/" + hex + "/resume"},
	} {
		rec := do(t, s, r.method, r.path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status = %d, want 503", r.method, r.path, rec.Code)
		}
	}
}

// assertJSONContains checks that every field of want is in got, with the
// same value; objects are compared field by field, arrays element by
// element (got may have more fields, not more elements).
func assertJSONContains(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("decode body %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want %s: %v", want, err)
	}
	if !jsonContains(g, w) {
		t.Errorf("body %s doesn't contain %s", got, want)
	}
}

func jsonContains(got, want any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range w {
			if gv, ok := g[k]; !ok || !jsonContains(gv, v) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !jsonContains(g[i], w[i]) {
				return false
			}
		}
		return true
	default:
		return got == want
	}
}
