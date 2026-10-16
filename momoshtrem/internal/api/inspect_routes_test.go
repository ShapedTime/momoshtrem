package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// fakeInspector answers Inspect with job 42 (created or not) or err, and
// records the request.
type fakeInspector struct {
	err     error
	created bool
	req     *service.InspectRequest
}

func (f *fakeInspector) Inspect(_ context.Context, req service.InspectRequest) (int64, bool, error) {
	f.req = &req
	if f.err != nil {
		return 0, false, f.err
	}
	return 42, f.created, nil
}

// fakeReleases resolves every release id to one magnet, or fails with err.
type fakeReleases struct {
	err error
	ids []string
}

func (f *fakeReleases) Resolve(_ context.Context, releaseID string) (string, string, error) {
	f.ids = append(f.ids, releaseID)
	if f.err != nil {
		return "", "", f.err
	}
	return "magnet:?xt=urn:btih:" + strings.Repeat("a", 40), "Nyaa", nil
}

func TestInspectRoute(t *testing.T) {
	const (
		magnetBody  = `{"media_type": "tv", "tmdb_id": 209867, "magnet_uri": "magnet:?xt=urn:btih:abc", "hint": {"season": 1, "episode": 29}}`
		releaseBody = `{"media_type": "movie", "tmdb_id": 27205, "release_id": "r1"}`
	)
	tests := []struct {
		name        string
		body        string
		releases    *fakeReleases // nil: no resolver
		inspectErr  error
		created     bool
		wantStatus  int
		wantCode    string
		wantInspect bool
	}{
		{"a magnet", magnetBody, nil, nil, true, 202, "", true},
		{"a magnet inspected within the hour", magnetBody, nil, nil, false, 200, "", true},
		{"a release", releaseBody, &fakeReleases{}, nil, true, 202, "", true},
		{"neither", `{"media_type": "tv", "tmdb_id": 1}`, &fakeReleases{}, nil, true, 400, CodeBadRequest, false},
		{"a broken body", `{"media_type":`, &fakeReleases{}, nil, true, 400, CodeBadRequest, false},
		{"an unknown media type", `{"media_type": "anime", "tmdb_id": 1, "release_id": "r1"}`, &fakeReleases{}, nil, true, 400, CodeBadRequest, false},
		{"no TMDB id", `{"media_type": "tv", "release_id": "r1"}`, &fakeReleases{}, nil, true, 400, CodeBadRequest, false},
		{"a negative season", `{"media_type": "tv", "tmdb_id": 1, "release_id": "r1", "hint": {"season": -1}}`, &fakeReleases{}, nil, true, 400, CodeBadRequest, false},
		{"episode zero", `{"media_type": "tv", "tmdb_id": 1, "release_id": "r1", "hint": {"episode": 0}}`, &fakeReleases{}, nil, true, 400, CodeBadRequest, false},
		{"a magnet without a hash", magnetBody, nil, library.ErrInvalidMagnet, true, 400, CodeBadRequest, true},
		{"no resolver yet", releaseBody, nil, nil, true, 404, CodeNotFound, false},
		{"an expired release", releaseBody, &fakeReleases{err: fmt.Errorf("lookup: %w", service.ErrReleaseNotFound)}, nil, true, 404, CodeNotFound, false},
		{"the indexer gave nothing", releaseBody, &fakeReleases{err: torznab.ErrRedirect}, nil, true, 502, CodeTorrentUnavailable, false},
		{"the indexer timed out", releaseBody, &fakeReleases{err: torznab.ErrTimeout}, nil, true, 504, CodeTimeout, false},
		{"the resolve ran out of time", releaseBody, &fakeReleases{err: context.DeadlineExceeded}, nil, true, 504, CodeTimeout, false},
		{"an unknown title", magnetBody, nil, fmt.Errorf("%w: %w", service.ErrTitleNotFound, tmdb.ErrNotFound), true, 404, CodeNotFound, true},
		{"TMDB down", magnetBody, nil, fmt.Errorf("%w: down", library.ErrTMDBFetch), true, 502, CodeTMDBUnavailable, true},
		{"a database error", magnetBody, nil, errors.New("conn reset by 10.0.0.3"), true, 500, CodeInternalError, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector := &fakeInspector{err: tt.inspectErr, created: tt.created}
			opts := Options{Inspect: inspector}
			if tt.releases != nil {
				opts.Releases = tt.releases
			}
			rec := doBody(t, newTestServer(opts), "POST", "/api/search/torrents/inspect", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec); got.Code != tt.wantCode || strings.Contains(got.Message, "10.0.0.3") {
					t.Errorf("error = %+v, want code %s", got, tt.wantCode)
				}
			} else if strings.TrimSpace(rec.Body.String()) != `{"job_id":42}` {
				t.Errorf("body = %s", rec.Body)
			}
			if (inspector.req != nil) != tt.wantInspect {
				t.Errorf("inspected = %v, want %v", inspector.req != nil, tt.wantInspect)
			}
		})
	}
}

// A magnet goes as it is with the hint; a release is resolved first, and
// its indexer goes along.
func TestInspectRoute_Requests(t *testing.T) {
	inspector, releases := &fakeInspector{created: true}, &fakeReleases{}
	s := newTestServer(Options{Inspect: inspector, Releases: releases})

	doBody(t, s, "POST", "/api/search/torrents/inspect",
		`{"media_type": "tv", "tmdb_id": 209867, "magnet_uri": "magnet:?xt=urn:btih:abc", "hint": {"season": 1, "episode": 29, "text": "ignored"}}`)
	r := inspector.req
	if r == nil || r.MediaType != library.JobTV || r.TMDBID != 209867 || r.MagnetURI != "magnet:?xt=urn:btih:abc" || r.Indexer != "" ||
		r.Hint == nil || *r.Hint.Season != 1 || *r.Hint.Episode != 29 || r.Hint.Text != "" || len(releases.ids) != 0 {
		t.Errorf("magnet request = %+v (resolved %q)", r, releases.ids)
	}

	doBody(t, s, "POST", "/api/search/torrents/inspect", `{"media_type": "movie", "tmdb_id": 27205, "release_id": "r1"}`)
	r = inspector.req
	if r.MediaType != library.JobMovie || r.MagnetURI != "magnet:?xt=urn:btih:"+strings.Repeat("a", 40) || r.Indexer != "Nyaa" ||
		r.Hint != nil || len(releases.ids) != 1 || releases.ids[0] != "r1" {
		t.Errorf("release request = %+v (resolved %q)", r, releases.ids)
	}
}

func TestInspectRoute_NotConfigured(t *testing.T) {
	rec := doBody(t, newTestServer(Options{}), "POST", "/api/search/torrents/inspect", `{"media_type": "tv", "tmdb_id": 1, "magnet_uri": "m"}`)
	if rec.Code != 503 || decodeError(t, rec).Code != CodeUnavailable {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}
