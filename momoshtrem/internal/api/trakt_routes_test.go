package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// fakeTrakt answers each call with its error, or a value.
type fakeTrakt struct {
	err      error
	settings library.TraktSettings
	patches  []library.TraktSettingsPatch
	calls    []string
}

func (f *fakeTrakt) Integration(context.Context) (trakt.Integration, error) {
	f.calls = append(f.calls, "integration")
	return trakt.Integration{State: trakt.StateOff, Sync: trakt.SyncRun{Status: trakt.SyncIdle}, Settings: f.settings}, f.err
}

func (f *fakeTrakt) Connect(context.Context) (trakt.DeviceCode, error) {
	f.calls = append(f.calls, "connect")
	return trakt.DeviceCode{UserCode: "A1B2C3D4", VerificationURL: "https://trakt.tv/activate",
		ExpiresAt: time.Date(2026, 10, 16, 12, 10, 0, 0, time.UTC), Interval: 5}, f.err
}

func (f *fakeTrakt) Disconnect(context.Context) error {
	f.calls = append(f.calls, "disconnect")
	return f.err
}

func (f *fakeTrakt) SyncNow(context.Context) (trakt.Integration, error) {
	f.calls = append(f.calls, "sync")
	return trakt.Integration{State: trakt.StateConnected, Connected: true, Sync: trakt.SyncRun{Status: trakt.SyncSyncing}}, f.err
}

func (f *fakeTrakt) UpdateSettings(_ context.Context, p library.TraktSettingsPatch) (library.TraktSettings, error) {
	f.patches = append(f.patches, p)
	if p.UseRatings != nil {
		f.settings.UseRatings = *p.UseRatings
	}
	if p.ShowWatchlist != nil {
		f.settings.ShowWatchlist = *p.ShowWatchlist
	}
	if p.IncludePaused != nil {
		f.settings.IncludePaused = *p.IncludePaused
	}
	return f.settings, f.err
}

func doTrakt(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestTraktRoutes(t *testing.T) {
	f := &fakeTrakt{settings: library.DefaultTraktSettings()}
	s := newTestServer(Options{Trakt: f})

	rec := doTrakt(t, s, "GET", "/api/integrations/trakt", "")
	var in map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &in) != nil || in["state"] != "off" || in["username"] != nil {
		t.Errorf("GET = %d %s", rec.Code, rec.Body)
	}

	rec = doTrakt(t, s, "POST", "/api/integrations/trakt/connect", "")
	if want := `{"user_code":"A1B2C3D4","verification_url":"https://trakt.tv/activate","expires_at":"2026-10-16T12:10:00Z","interval":5}`; rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("connect = %d %s", rec.Code, rec.Body)
	}

	if rec := doTrakt(t, s, "DELETE", "/api/integrations/trakt", ""); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("DELETE = %d %s", rec.Code, rec.Body)
	}

	if rec := doTrakt(t, s, "POST", "/api/integrations/trakt/sync", ""); rec.Code != 202 || !strings.Contains(rec.Body.String(), `"status":"syncing"`) {
		t.Errorf("sync = %d %s", rec.Code, rec.Body)
	}

	rec = doTrakt(t, s, "PUT", "/api/integrations/trakt/settings", `{"include_paused": true, "theme": "dark"}`)
	if want := `{"use_ratings":true,"show_watchlist":true,"include_paused":true}`; rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("PUT settings = %d %s", rec.Code, rec.Body)
	}
	if p := f.patches[0]; p.IncludePaused == nil || !*p.IncludePaused || p.UseRatings != nil || p.ShowWatchlist != nil {
		t.Errorf("patch = %+v", p)
	}
}

func TestTraktRoutes_BadSettings(t *testing.T) {
	for _, body := range []string{``, `null`, `[]`, `"x"`, `{"use_ratings": "yes"}`, `{"show_watchlist": null}`, `{"include_paused": 1}`} {
		f := &fakeTrakt{}
		rec := doTrakt(t, newTestServer(Options{Trakt: f}), "PUT", "/api/integrations/trakt/settings", body)
		if rec.Code != 400 || decodeError(t, rec).Code != CodeBadRequest || len(f.patches) != 0 {
			t.Errorf("%q = %d %s", body, rec.Code, rec.Body)
		}
	}
	rec := doTrakt(t, newTestServer(Options{Trakt: &fakeTrakt{}}), "PUT", "/api/integrations/trakt/settings", `{}`)
	if rec.Code != 200 {
		t.Errorf("an empty patch = %d %s", rec.Code, rec.Body)
	}
}

func TestTraktRoutes_Errors(t *testing.T) {
	tests := []struct {
		method, path string
		err          error
		status       int
		code         string
	}{
		{"POST", "/api/integrations/trakt/connect", trakt.ErrNotConfigured, 503, CodeUnavailable},
		{"POST", "/api/integrations/trakt/connect", trakt.ErrConnected, 409, CodeConflict},
		{"POST", "/api/integrations/trakt/connect", fmt.Errorf("%w: trakt answered HTTP 503", trakt.ErrUnavailable), 502, CodeTraktUnavailable},
		{"POST", "/api/integrations/trakt/connect", fmt.Errorf("%w: %w", trakt.ErrUnavailable, &trakt.StatusError{StatusCode: 401, Code: "invalid_client"}), 502, CodeTraktUnavailable},
		{"POST", "/api/integrations/trakt/sync", trakt.ErrSyncUnavailable, 503, CodeUnavailable},
		{"POST", "/api/integrations/trakt/sync", trakt.ErrNotConnected, 409, CodeConflict},
		{"GET", "/api/integrations/trakt", errors.New("connection refused"), 500, CodeInternalError},
		{"DELETE", "/api/integrations/trakt", errors.New("connection refused"), 500, CodeInternalError},
		// The member was removed while the request ran.
		{"GET", "/api/integrations/trakt", member.ErrUnknown, 400, CodeUnknownMember},
		{"PUT", "/api/integrations/trakt/settings", fmt.Errorf("wrapped: %w", member.ErrNotFound), 400, CodeUnknownMember},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path+" "+tt.err.Error(), func(t *testing.T) {
			rec := doTrakt(t, newTestServer(Options{Trakt: &fakeTrakt{err: tt.err}}), tt.method, tt.path, "{}")
			if rec.Code != tt.status || decodeError(t, rec).Code != tt.code {
				t.Errorf("= %d %s, want %d %s", rec.Code, rec.Body, tt.status, tt.code)
			}
		})
	}
}

func TestTraktRoutes_Unavailable(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range [][2]string{
		{"GET", "/api/integrations/trakt"}, {"POST", "/api/integrations/trakt/connect"}, {"DELETE", "/api/integrations/trakt"},
		{"PUT", "/api/integrations/trakt/settings"}, {"POST", "/api/integrations/trakt/sync"},
	} {
		if rec := doTrakt(t, s, r[0], r[1], `{}`); rec.Code != 503 {
			t.Errorf("%s %s = %d", r[0], r[1], rec.Code)
		}
	}
}
