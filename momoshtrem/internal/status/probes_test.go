package status

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testJackettKey = "jackett-secret-key"

func TestJackettProber(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantErr   string // "" means success
		wantQuery bool
	}{
		{
			name:   "caps ok",
			status: http.StatusOK,
			body:   `<?xml version="1.0" encoding="UTF-8"?><caps><server title="Jackett"/></caps>`,
		},
		{
			name:    "torznab error document",
			status:  http.StatusOK,
			body:    `<?xml version="1.0" encoding="UTF-8"?><error code="100" description="Invalid API Key" />`,
			wantErr: "Invalid API Key",
		},
		{
			name:    "unauthorized status",
			status:  http.StatusUnauthorized,
			body:    `nope`,
			wantErr: "HTTP 401",
		},
		{
			name:    "server error",
			status:  http.StatusInternalServerError,
			body:    `<html>oops</html>`,
			wantErr: "HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotT, gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotT = r.URL.Query().Get("t")
				gotKey = r.URL.Query().Get("apikey")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			// A trailing slash on the base URL must not produce "//api".
			probe := JackettProber(srv.Client(), srv.URL+"/", testJackettKey)
			err := probe(context.Background())

			if gotPath != "/api/v2.0/indexers/all/results/torznab/api" || gotT != "caps" || gotKey != testJackettKey {
				t.Errorf("request = %s t=%s apikey=%s", gotPath, gotT, gotKey)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), testJackettKey) {
				t.Errorf("error leaks the API key: %v", err)
			}
		})
	}
}

func TestJackettProber_TransportErrorHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused from here on

	err := JackettProber(http.DefaultClient, url, testJackettKey)(context.Background())
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), testJackettKey) || strings.Contains(err.Error(), "apikey") {
		t.Errorf("error leaks the request URL: %v", err)
	}
}

func TestJackettProber_HonoursContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := JackettProber(srv.Client(), srv.URL, testJackettKey)(ctx); err == nil {
		t.Fatal("want an error for a cancelled context")
	}
}
