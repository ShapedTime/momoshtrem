package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// fakeDownloads answers what the test sets, and records what it was asked.
type fakeDownloads struct {
	list     downloads.List
	download downloads.Download
	created  bool
	estimate downloads.Estimate
	err      error
	target   library.DownloadTarget
	body     string
	id       int64
}

func (f *fakeDownloads) List(context.Context) (downloads.List, error) { return f.list, f.err }

func (f *fakeDownloads) Estimate(_ context.Context, t library.DownloadTarget) (downloads.Estimate, error) {
	f.target = t
	return f.estimate, f.err
}

func (f *fakeDownloads) Keep(_ context.Context, body []byte) (downloads.Download, bool, error) {
	f.body = string(body)
	return f.download, f.created, f.err
}

func (f *fakeDownloads) Update(_ context.Context, id int64, body []byte) (downloads.Download, error) {
	f.id, f.body = id, string(body)
	return f.download, f.err
}

func (f *fakeDownloads) Delete(_ context.Context, id int64) error {
	f.id = id
	return f.err
}

func TestDownloadRoutes_Unavailable(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/downloads"}, {"GET", "/api/downloads/estimate?scope=movie&id=1"}, {"POST", "/api/downloads"},
		{"PATCH", "/api/downloads/1"}, {"DELETE", "/api/downloads/1"},
	} {
		rec := doBody(t, s, r.method, r.path, `{}`)
		if e := errorOf(t, rec.Body.Bytes()); rec.Code != 503 || e.Code != CodeUnavailable {
			t.Errorf("%s %s = %d %s, want 503", r.method, r.path, rec.Code, rec.Body)
		}
	}
}

// Every status code of PLAN §6 "Downloads (#77)".
func TestDownloadRoutes(t *testing.T) {
	ok := downloads.Download{ID: 4, Reason: library.DownloadKept, Scope: library.DownloadScopeMovie, State: downloads.StateQueued}
	tests := []struct {
		name, method, path, body string
		fake                     fakeDownloads
		status                   int
		code                     string
		field                    string
	}{
		{"GET lists", "GET", "/api/downloads", "", fakeDownloads{list: downloads.List{Items: []downloads.Download{ok}}}, 200, "", ""},
		{"estimate", "GET", "/api/downloads/estimate?scope=season&id=3&season_number=2", "", fakeDownloads{}, 200, "", ""},
		{"estimate: malformed target", "GET", "/api/downloads/estimate?scope=season&id=3", "", fakeDownloads{}, 400, CodeBadRequest, ""},
		{"estimate: bad scope", "GET", "/api/downloads/estimate?scope=film&id=3", "", fakeDownloads{}, 400, CodeBadRequest, ""},
		{"estimate: a season past int32", "GET", "/api/downloads/estimate?scope=season&id=3&season_number=2147483648", "", fakeDownloads{}, 400, CodeBadRequest, ""},
		{"estimate: a malformed id", "GET", "/api/downloads/estimate?scope=movie&id=x", "", fakeDownloads{}, 400, CodeBadRequest, ""},
		{"estimate: not in the library", "GET", "/api/downloads/estimate?scope=movie&id=3", "",
			fakeDownloads{err: fmt.Errorf("%w: %w", downloads.ErrTargetNotFound, library.ErrMovieNotFound)}, 404, CodeNotFound, ""},
		{"POST: a new Keep", "POST", "/api/downloads", `{"scope":"movie","id":1}`, fakeDownloads{download: ok, created: true}, 201, "", ""},
		{"POST: already kept, or a smart pick turned kept", "POST", "/api/downloads", `{"scope":"movie","id":1}`, fakeDownloads{download: ok}, 200, "", ""},
		{"POST: not an object", "POST", "/api/downloads", `[]`, fakeDownloads{err: downloads.ErrNotAnObject}, 400, CodeBadRequest, ""},
		{"POST: a bad value", "POST", "/api/downloads", `{}`,
			fakeDownloads{err: &downloads.FieldError{Field: "scope", Message: "scope must be movie, show, season or episode"}}, 422, CodeValidationError, "scope"},
		{"POST: not in the library", "POST", "/api/downloads", `{}`, fakeDownloads{err: downloads.ErrTargetNotFound}, 404, CodeNotFound, ""},
		{"POST: HDRezka", "POST", "/api/downloads", `{}`, fakeDownloads{err: downloads.ErrNotDownloadable}, 422, CodeNotDownloadable, ""},
		{"POST: no room", "POST", "/api/downloads", `{}`,
			fakeDownloads{err: &downloads.StorageFullError{NeededBytes: 9, AvailableBytes: 2}}, 409, CodeStorageFull, ""},
		{"PATCH", "PATCH", "/api/downloads/4", `{"until_watched":true}`, fakeDownloads{download: ok}, 200, "", ""},
		{"PATCH: unknown or released", "PATCH", "/api/downloads/4", `{}`, fakeDownloads{err: downloads.ErrNotFound}, 404, CodeNotFound, ""},
		{"PATCH: not an object", "PATCH", "/api/downloads/4", `1`, fakeDownloads{err: downloads.ErrNotAnObject}, 400, CodeBadRequest, ""},
		{"PATCH: a smart pick", "PATCH", "/api/downloads/4", `{}`, fakeDownloads{err: downloads.ErrSmartPick}, 422, CodeValidationError, ""},
		{"PATCH: a bad value", "PATCH", "/api/downloads/4", `{}`,
			fakeDownloads{err: &downloads.FieldError{Field: "expires_at", Message: "expires_at must be in the future"}}, 422, CodeValidationError, "expires_at"},
		{"PATCH: a malformed id", "PATCH", "/api/downloads/x", `{}`, fakeDownloads{}, 400, CodeBadRequest, ""},
		{"DELETE", "DELETE", "/api/downloads/4", "", fakeDownloads{}, 204, "", ""},
		{"DELETE: unknown or released", "DELETE", "/api/downloads/4", "", fakeDownloads{err: downloads.ErrNotFound}, 404, CodeNotFound, ""},
		{"a failure", "GET", "/api/downloads", "", fakeDownloads{err: errors.New("db down")}, 500, CodeInternalError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fake
			s := newTestServer(Options{Downloads: &f})
			rec := doBody(t, s, tt.method, tt.path, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.status)
			}
			if tt.code != "" {
				if e := errorOf(t, rec.Body.Bytes()); e.Code != tt.code || e.Field != tt.field {
					t.Errorf("error = %+v, want %s on %q", e, tt.code, tt.field)
				}
			}
		})
	}

	t.Run("the 409 carries what's needed and left", func(t *testing.T) {
		s := newTestServer(Options{Downloads: &fakeDownloads{err: &downloads.StorageFullError{NeededBytes: 9_000_000_000, AvailableBytes: 2_000_000_000}}})
		rec := doBody(t, s, "POST", "/api/downloads", `{"scope":"movie","id":1}`)
		var body struct {
			Error struct {
				Code           string `json:"code"`
				Message        string `json:"message"`
				NeededBytes    int64  `json:"needed_bytes"`
				AvailableBytes int64  `json:"available_bytes"`
			} `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error.NeededBytes != 9_000_000_000 || body.Error.AvailableBytes != 2_000_000_000 ||
			body.Error.Message != "Not enough space for kept downloads: this needs 9.0 GB and 2.0 GB is left." {
			t.Errorf("409 body = %s", rec.Body)
		}
	})

	t.Run("the routes pass the target, the body and the id", func(t *testing.T) {
		f := &fakeDownloads{download: ok}
		s := newTestServer(Options{Downloads: f})
		doBody(t, s, "GET", "/api/downloads/estimate?scope=season&id=3&season_number=2", "")
		if f.target != (library.DownloadTarget{Scope: library.DownloadScopeSeason, ID: 3, SeasonNumber: 2}) {
			t.Errorf("target = %+v", f.target)
		}
		doBody(t, s, "PATCH", "/api/downloads/12", `{"expires_at":null}`)
		if f.id != 12 || f.body != `{"expires_at":null}` {
			t.Errorf("PATCH passed %d %s", f.id, f.body)
		}
	})
}
