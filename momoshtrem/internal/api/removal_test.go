package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// fakeRemovals answers err and records the call.
type fakeRemovals struct {
	err        error
	called     string
	hash       string
	id         int64
	deleteData bool
}

func (f *fakeRemovals) DeleteTorrent(_ context.Context, hash string, deleteData bool) error {
	f.called, f.hash, f.deleteData = "delete-torrent", hash, deleteData
	return f.err
}

func (f *fakeRemovals) DeleteMovie(_ context.Context, id int64, deleteData bool) error {
	f.called, f.id, f.deleteData = "delete-movie", id, deleteData
	return f.err
}

func (f *fakeRemovals) DeleteShow(_ context.Context, id int64, deleteData bool) error {
	f.called, f.id, f.deleteData = "delete-show", id, deleteData
	return f.err
}

func (f *fakeRemovals) UnassignMovie(_ context.Context, id int64) error {
	f.called, f.id = "unassign-movie", id
	return f.err
}

func (f *fakeRemovals) UnassignEpisode(_ context.Context, id int64) error {
	f.called, f.id = "unassign-episode", id
	return f.err
}

func TestRemovalRoutes(t *testing.T) {
	const hex = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	tests := []struct {
		name           string
		path           string
		err            error
		wantStatus     int
		wantCode       string
		wantCall       string // "" when the handler mustn't call
		wantHash       string
		wantID         int64
		wantDeleteData bool
	}{
		{name: "torrent, uppercase hex", path: "/api/torrents/DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C",
			wantStatus: http.StatusNoContent, wantCall: "delete-torrent", wantHash: hex},
		{name: "torrent, base32, delete data", path: "/api/torrents/3WBFL3G4PSSV7MF37AJSHWDQMLNR63I4?delete_data=true",
			wantStatus: http.StatusNoContent, wantCall: "delete-torrent", wantHash: hex, wantDeleteData: true},
		{name: "torrent, keep data", path: "/api/torrents/" + hex + "?delete_data=false",
			wantStatus: http.StatusNoContent, wantCall: "delete-torrent", wantHash: hex},
		{name: "torrent, malformed hash", path: "/api/torrents/summary",
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "torrent, bad delete_data", path: "/api/torrents/" + hex + "?delete_data=yes",
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "torrent unknown", path: "/api/torrents/" + hex, err: torrent.ErrTorrentNotFound,
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantCall: "delete-torrent", wantHash: hex},
		{name: "torrent, database down", path: "/api/torrents/" + hex, err: errors.New("connection reset"),
			wantStatus: http.StatusInternalServerError, wantCode: CodeInternalError, wantCall: "delete-torrent", wantHash: hex},

		{name: "movie", path: "/api/movies/5", wantStatus: http.StatusNoContent, wantCall: "delete-movie", wantID: 5},
		{name: "movie, delete data", path: "/api/movies/5?delete_data=true",
			wantStatus: http.StatusNoContent, wantCall: "delete-movie", wantID: 5, wantDeleteData: true},
		{name: "movie, bad delete_data", path: "/api/movies/5?delete_data=1",
			wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "movie, bad id", path: "/api/movies/abc", wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest},
		{name: "movie unknown", path: "/api/movies/5", err: fmt.Errorf("delete: %w", library.ErrMovieNotFound),
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantCall: "delete-movie", wantID: 5},

		{name: "show, delete data", path: "/api/shows/7?delete_data=true",
			wantStatus: http.StatusNoContent, wantCall: "delete-show", wantID: 7, wantDeleteData: true},
		{name: "show unknown", path: "/api/shows/7", err: library.ErrShowNotFound,
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantCall: "delete-show", wantID: 7},

		{name: "unassign movie", path: "/api/movies/5/assign", wantStatus: http.StatusNoContent, wantCall: "unassign-movie", wantID: 5},
		{name: "unassign movie unknown", path: "/api/movies/5/assign", err: library.ErrMovieNotFound,
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantCall: "unassign-movie", wantID: 5},
		{name: "unassign episode", path: "/api/episodes/9/assign", wantStatus: http.StatusNoContent, wantCall: "unassign-episode", wantID: 9},
		{name: "unassign episode unknown", path: "/api/episodes/9/assign", err: library.ErrEpisodeNotFound,
			wantStatus: http.StatusNotFound, wantCode: CodeNotFound, wantCall: "unassign-episode", wantID: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRemovals{err: tt.err}
			rec := do(t, newTestServer(Options{Removals: fake}), http.MethodDelete, tt.path, nil)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec); got.Code != tt.wantCode {
					t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
				}
			}
			if fake.called != tt.wantCall {
				t.Fatalf("called %q, want %q", fake.called, tt.wantCall)
			}
			if fake.hash != tt.wantHash || fake.id != tt.wantID || fake.deleteData != tt.wantDeleteData {
				t.Errorf("args = (%q, %d, %v), want (%q, %d, %v)",
					fake.hash, fake.id, fake.deleteData, tt.wantHash, tt.wantID, tt.wantDeleteData)
			}
		})
	}
}

func TestRemovalRoutes_Unconfigured(t *testing.T) {
	s := newTestServer(Options{})
	for _, path := range []string{
		"/api/torrents/dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c", "/api/movies/1", "/api/shows/1",
		"/api/movies/1/assign", "/api/episodes/1/assign",
	} {
		rec := do(t, s, http.MethodDelete, path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503", path, rec.Code)
		}
	}
}
