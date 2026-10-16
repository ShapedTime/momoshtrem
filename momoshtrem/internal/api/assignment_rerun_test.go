package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// fakeMagnets knows the magnets of some torrents, by lowercase hex hash.
type fakeMagnets struct {
	magnets map[string]string
	err     error
	asked   string
}

func (f *fakeMagnets) TorrentMagnet(_ context.Context, hash string) (string, error) {
	f.asked = hash
	return f.magnets[hash], f.err
}

// Re-run mapping: an assign by info_hash takes the magnet the library has
// for the torrent and creates a job like any assign.
func TestAssignmentRoutes_ByInfoHash(t *testing.T) {
	const hex = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	const stored = "magnet:?xt=urn:btih:" + hex + "&dn=Frieren"
	tests := []struct {
		name       string
		path       string
		body       string
		noMagnets  bool
		lookupErr  error
		created    bool
		wantStatus int
		wantCode   string
		wantCall   string // the job service's
		wantAsked  string // the hash looked up
		wantMagnet string
	}{
		{name: "show", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "` + hex + `", "hint": {"season": 2}}`, created: true,
			wantStatus: http.StatusAccepted, wantCall: "assign", wantAsked: hex, wantMagnet: stored},
		{name: "show: the open job for that torrent", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "` + hex + `"}`,
			wantStatus: http.StatusOK, wantCall: "assign", wantAsked: hex, wantMagnet: stored},
		{name: "movie, uppercase hex", path: "/api/movies/8/assign-torrent", body: `{"info_hash": "DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C"}`, created: true,
			wantStatus: http.StatusAccepted, wantCall: "assign", wantAsked: hex, wantMagnet: stored},
		{name: "base32", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "3WBFL3G4PSSV7MF37AJSHWDQMLNR63I4"}`, created: true,
			wantStatus: http.StatusAccepted, wantCall: "assign", wantAsked: hex, wantMagnet: stored},
		{name: "legacy wait", path: "/api/shows/7/assign-torrent?wait=true", body: `{"info_hash": "` + hex + `"}`,
			wantStatus: http.StatusCreated, wantCall: "show-wait", wantAsked: hex, wantMagnet: stored},
		{name: "a magnet wins over the hash", path: "/api/shows/7/assign-torrent", body: `{"magnet_uri": "magnet:?xt=urn:btih:abc", "info_hash": "` + hex + `"}`,
			created: true, wantStatus: http.StatusAccepted, wantCall: "assign", wantMagnet: "magnet:?xt=urn:btih:abc"},
		{name: "unknown torrent", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "` + "0123456789abcdef0123456789abcdef01234567" + `"}`,
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantAsked: "0123456789abcdef0123456789abcdef01234567"},
		{name: "malformed hash", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "summary"}`,
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "neither", path: "/api/shows/7/assign-torrent", body: `{"hint": {"season": 1}}`,
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "empty strings", path: "/api/movies/8/assign-torrent", body: `{"magnet_uri": "", "info_hash": ""}`,
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "not configured", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "` + hex + `"}`, noMagnets: true,
			wantStatus: http.StatusServiceUnavailable, wantCode: CodeUnavailable},
		{name: "the lookup fails", path: "/api/shows/7/assign-torrent", body: `{"info_hash": "` + hex + `"}`, lookupErr: errors.New("conn reset by 10.0.0.3"),
			wantStatus: http.StatusInternalServerError, wantCode: CodeInternalError, wantAsked: hex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := &fakeJobs{created: tt.created}
			magnets := &fakeMagnets{magnets: map[string]string{hex: stored}, err: tt.lookupErr}
			opts := Options{Assignments: jobs, Magnets: magnets}
			if tt.noMagnets {
				opts.Magnets = nil
			}
			rec := doBody(t, newTestServer(opts), http.MethodPost, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec); got.Code != tt.wantCode {
					t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
				}
			}
			if jobs.called != tt.wantCall || magnets.asked != tt.wantAsked {
				t.Fatalf("called %q, asked %q; want %q, %q", jobs.called, magnets.asked, tt.wantCall, tt.wantAsked)
			}
			if tt.wantMagnet != "" && jobs.req.MagnetURI != tt.wantMagnet {
				t.Errorf("magnet = %q, want %q", jobs.req.MagnetURI, tt.wantMagnet)
			}
		})
	}
}
