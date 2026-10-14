package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "tmdb-secret-key"

func TestGetContext_Status(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr string // "" means success
	}{
		{"ok", http.StatusOK, ""},
		{"unauthorized", http.StatusUnauthorized, "rejected the API key"},
		{"not found", http.StatusNotFound, "not found"},
		{"server error", http.StatusInternalServerError, "unexpected status: 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotKey = r.URL.Query().Get("api_key")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"images":{}}`))
			}))
			defer srv.Close()

			c := NewClient(testKey)
			err := c.getContext(context.Background(), srv.URL+"/configuration", nil)

			if gotKey != testKey {
				t.Errorf("api_key = %q, want %q", gotKey, testKey)
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
			if tt.status == http.StatusNotFound && !errors.Is(err, ErrNotFound) {
				t.Errorf("error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestGetContext_TransportErrorHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused from here on

	err := NewClient(testKey).getContext(context.Background(), url+"/configuration", nil)
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error leaks the API key: %v", err)
	}
}
