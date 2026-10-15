package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// fakePinger returns a fixed error and records whether the ping had a deadline.
type fakePinger struct {
	err         error
	hadDeadline bool
}

func (f *fakePinger) PingContext(ctx context.Context) error {
	_, f.hadDeadline = ctx.Deadline()
	return f.err
}

func TestGetHealthz(t *testing.T) {
	const dbErr = "dial tcp postgres:5432: password authentication failed for user momoshtrem"

	tests := []struct {
		name       string
		db         Pinger
		wantStatus int
	}{
		{"db up", &fakePinger{}, http.StatusOK},
		{"db down is 503", &fakePinger{err: errors.New(dbErr)}, http.StatusServiceUnavailable},
		{"no db is 503", nil, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(Options{DB: tt.db})

			rec := do(t, s, http.MethodGet, "/healthz", nil)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusOK {
				var body struct{ Status string }
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Status != "ok" {
					t.Errorf("body = %s, want {\"status\":\"ok\"}", rec.Body)
				}
			} else if got := decodeError(t, rec).Code; got != CodeUnavailable {
				t.Errorf("code = %q, want %q", got, CodeUnavailable)
			}
			if strings.Contains(rec.Body.String(), "password") {
				t.Errorf("response leaks the driver error: %s", rec.Body)
			}
			if p, ok := tt.db.(*fakePinger); ok && !p.hadDeadline {
				t.Error("ping had no deadline")
			}
		})
	}
}

func TestGetHealthz_NoTokenNeeded(t *testing.T) {
	s := newTestServer(Options{APIToken: "s3cret-token-value", DB: &fakePinger{}})

	if rec := do(t, s, http.MethodGet, "/healthz", nil); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without a token (body %s)", rec.Code, rec.Body)
	}
	// The rest of the API still needs it
	if rec := do(t, s, http.MethodGet, "/api/status", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/status = %d, want 401", rec.Code)
	}
}
