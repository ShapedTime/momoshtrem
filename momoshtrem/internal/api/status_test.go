package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/status"
)

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name        string
		report      status.Report
		query       string
		wantStatus  int
		wantRefresh int
	}{
		{"healthy", healthyReport(), "", http.StatusOK, 0},
		{"refresh flag", healthyReport(), "?refresh=1", http.StatusOK, 1},
		{"probe failure is still 200", func() status.Report {
			r := healthyReport()
			r.Jackett = status.ProbeStatus{Configured: true, Error: "timed out after 3s"}
			return r
		}(), "", http.StatusOK, 0},
		{"db down is 503", status.Report{DB: status.DBStatus{Error: "database unreachable"}}, "", http.StatusServiceUnavailable, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeStatus{report: tt.report}
			s := newTestServer(Options{Status: fake})

			rec := do(t, s, http.MethodGet, "/api/status"+tt.query, nil)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if fake.refreshs != tt.wantRefresh {
				t.Errorf("refresh calls = %d, want %d", fake.refreshs, tt.wantRefresh)
			}
			var got status.Report
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.DB.OK != tt.report.DB.OK {
				t.Errorf("db.ok = %v, want %v", got.DB.OK, tt.report.DB.OK)
			}
		})
	}
}

func TestGetStatus_NotConfigured(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil, Options{})
	rec := do(t, s, http.MethodGet, "/api/status", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := decodeError(t, rec).Code; got != CodeUnavailable {
		t.Errorf("code = %q, want %q", got, CodeUnavailable)
	}
}
