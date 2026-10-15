package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// fakeReview is an AssignmentReview that answers err and records what it
// was asked.
type fakeReview struct {
	err     error
	called  string
	id      int64
	changes []service.ProposalChange
	hash    string
	path    string
}

func (f *fakeReview) PatchProposals(_ context.Context, id int64, changes []service.ProposalChange) (*service.JobDetail, error) {
	f.called, f.id, f.changes = "patch", id, changes
	if f.err != nil {
		return nil, f.err
	}
	return &service.JobDetail{Job: service.JobView{ID: id, Status: library.JobReady}, Proposals: []service.ProposalView{}}, nil
}

func (f *fakeReview) ShowEpisodes(_ context.Context, id int64) ([]service.ShowEpisodesSeason, error) {
	f.called, f.id = "episodes", id
	if f.err != nil {
		return nil, f.err
	}
	return []service.ShowEpisodesSeason{{SeasonNumber: 0, Name: "Specials", Episodes: []service.ShowEpisode{{ID: 5, EpisodeNumber: 1, Name: "OVA"}}}}, nil
}

func (f *fakeReview) OverrideEpisode(_ context.Context, id int64, hash, path string) error {
	f.called, f.id, f.hash, f.path = "override-episode", id, hash, path
	return f.err
}

func (f *fakeReview) OverrideMovie(_ context.Context, id int64, hash, path string) error {
	f.called, f.id, f.hash, f.path = "override-movie", id, hash, path
	return f.err
}

func (f *fakeReview) TorrentFiles(_ context.Context, hash string) ([]service.TorrentFileView, error) {
	f.called, f.hash = "files", hash
	if f.err != nil {
		return nil, f.err
	}
	movie := int64(7)
	return []service.TorrentFileView{
		{Path: "Pack/Film.mkv", Size: 10, MappedTo: &service.FileMapping{MovieID: &movie}},
		{Path: "Pack/Show.S01E01E02.mkv", Size: 20, MappedTo: &service.FileMapping{Episodes: []service.ProposalTarget{
			{EpisodeID: 1, Season: 1, Episode: 1, Name: "One"}, {EpisodeID: 2, Season: 1, Episode: 2, Name: "Two"}}}},
		{Path: "Pack/info.nfo", Size: 1},
	}, nil
}

func invalidErr(msg string) error { return &service.ValidationError{Message: msg} }

func TestReviewRoutes(t *testing.T) {
	const ok = `{"items": [{"id": 3, "decision": "accepted"}]}`
	const assignment = `{"info_hash": "abc", "file_path": "Pack/E01.mkv"}`
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		err        error
		wantStatus int
		wantCode   string
		wantCall   string
	}{
		{"patch", "PATCH", "/api/assignment-jobs/5/proposals", ok, nil, 200, "", "patch"},
		{"patch: episodes only", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3, "episode_ids": [9, 10]}]}`, nil, 200, "", "patch"},
		{"patch: no body", "PATCH", "/api/assignment-jobs/5/proposals", ``, nil, 400, CodeBadRequest, ""},
		{"patch: broken JSON", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [`, nil, 400, CodeBadRequest, ""},
		{"patch: no items", "PATCH", "/api/assignment-jobs/5/proposals", `{}`, nil, 400, CodeBadRequest, ""},
		{"patch: empty items", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": []}`, nil, 400, CodeBadRequest, ""},
		{"patch: an item without id", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"decision": "accepted"}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: an item with neither field", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: null fields", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3, "decision": null, "episode_ids": null}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: unknown decision", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3, "decision": "maybe"}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: empty episode_ids", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3, "episode_ids": []}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: fractional id", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3.5, "decision": "ignored"}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: string episode id", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3, "episode_ids": ["9"]}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: one bad item of two", "PATCH", "/api/assignment-jobs/5/proposals", `{"items": [{"id": 3, "decision": "accepted"}, {"id": 4}]}`, nil, 400, CodeBadRequest, ""},
		{"patch: bad job id", "PATCH", "/api/assignment-jobs/x/proposals", ok, nil, 400, CodeBadRequest, ""},
		{"patch: unknown job", "PATCH", "/api/assignment-jobs/5/proposals", ok, library.ErrJobNotFound, 404, CodeNotFound, "patch"},
		{"patch: still working", "PATCH", "/api/assignment-jobs/5/proposals", ok, conflictErr(service.CodeJobWorking), 409, service.CodeJobWorking, "patch"},
		{"patch: closed", "PATCH", "/api/assignment-jobs/5/proposals", ok, conflictErr(service.CodeJobClosed), 409, service.CodeJobClosed, "patch"},
		{"patch: can't apply", "PATCH", "/api/assignment-jobs/5/proposals", ok, invalidErr("This job has no proposal 3."), 422, CodeValidationError, "patch"},
		{"patch: a database error", "PATCH", "/api/assignment-jobs/5/proposals", ok, errors.New("conn reset by 10.0.0.3"), 500, CodeInternalError, "patch"},

		{"episodes", "GET", "/api/shows/7/episodes", "", nil, 200, "", "episodes"},
		{"episodes: unknown show", "GET", "/api/shows/7/episodes", "", library.ErrShowNotFound, 404, CodeNotFound, "episodes"},
		{"episodes: bad id", "GET", "/api/shows/0/episodes", "", nil, 400, CodeBadRequest, ""},

		{"override episode", "PUT", "/api/episodes/9/assignment", assignment, nil, 204, "", "override-episode"},
		{"override movie", "PUT", "/api/movies/9/assignment", assignment, nil, 204, "", "override-movie"},
		{"override: no hash", "PUT", "/api/episodes/9/assignment", `{"file_path": "E01.mkv"}`, nil, 400, CodeBadRequest, ""},
		{"override: no path", "PUT", "/api/movies/9/assignment", `{"info_hash": "abc", "file_path": " "}`, nil, 400, CodeBadRequest, ""},
		{"override: broken JSON", "PUT", "/api/episodes/9/assignment", `{`, nil, 400, CodeBadRequest, ""},
		{"override: bad id", "PUT", "/api/episodes/-1/assignment", assignment, nil, 400, CodeBadRequest, ""},
		{"override: unknown episode", "PUT", "/api/episodes/9/assignment", assignment, library.ErrEpisodeNotFound, 404, CodeNotFound, "override-episode"},
		{"override: unknown movie", "PUT", "/api/movies/9/assignment", assignment, library.ErrMovieNotFound, 404, CodeNotFound, "override-movie"},
		{"override: unknown torrent", "PUT", "/api/episodes/9/assignment", assignment, service.ErrUnknownTorrent, 404, CodeNotFound, "override-episode"},
		{"override: no such file", "PUT", "/api/episodes/9/assignment", assignment, service.ErrFileNotInTorrent, 404, CodeNotFound, "override-episode"},
		{"override: not a video", "PUT", "/api/episodes/9/assignment", assignment, invalidErr("Pick a video file"), 422, CodeValidationError, "override-episode"},
		{"override: no peers", "PUT", "/api/episodes/9/assignment", assignment, service.ErrFileListTimeout, 504, CodeTimeout, "override-episode"},
		{"override: no torrent client", "PUT", "/api/movies/9/assignment", assignment, library.ErrTorrentServiceUnavailable, 503, CodeUnavailable, "override-movie"},
		{"override: the show went", "PUT", "/api/episodes/9/assignment", assignment, library.ErrShowNotFound, 404, CodeNotFound, "override-episode"},

		{"files", "GET", "/api/torrents/abc/files", "", nil, 200, "", "files"},
		{"files: unknown torrent", "GET", "/api/torrents/abc/files", "", service.ErrUnknownTorrent, 404, CodeNotFound, "files"},
		{"files: no peers", "GET", "/api/torrents/abc/files", "", service.ErrFileListTimeout, 504, CodeTimeout, "files"},
		{"files: no torrent client", "GET", "/api/torrents/abc/files", "", library.ErrTorrentServiceUnavailable, 503, CodeUnavailable, "files"},
		{"files: the client went away", "GET", "/api/torrents/abc/files", "", context.Canceled, StatusClientClosedRequest, CodeClientClosed, "files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review := &fakeReview{err: tt.err}
			s := newTestServer(Options{Review: review})
			rec := doBody(t, s, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec); got.Code != tt.wantCode || strings.Contains(got.Message, "10.0.0.3") {
					t.Errorf("error = %+v, want code %s", got, tt.wantCode)
				}
			}
			if review.called != tt.wantCall {
				t.Errorf("called %q, want %q", review.called, tt.wantCall)
			}
		})
	}
}

func TestReviewRoutes_Bodies(t *testing.T) {
	review := &fakeReview{}
	s := newTestServer(Options{Review: review})

	rec := doBody(t, s, "PATCH", "/api/assignment-jobs/5/proposals",
		`{"items": [{"id": 3, "decision": "ignored"}, {"id": 4, "episode_ids": [9, 10], "decision": "accepted"}, {"id": 6, "episode_ids": [11]}]}`)
	var detail service.JobDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.Job.ID != 5 || detail.Proposals == nil {
		t.Errorf("patch = %s", rec.Body)
	}
	ignored, accepted := library.DecisionIgnored, library.DecisionAccepted
	want := []service.ProposalChange{
		{ID: 3, Decision: &ignored},
		{ID: 4, Decision: &accepted, EpisodeIDs: []int64{9, 10}},
		{ID: 6, EpisodeIDs: []int64{11}},
	}
	if review.id != 5 || !reflect.DeepEqual(review.changes, want) {
		t.Errorf("changes = %+v", review.changes)
	}

	rec = doBody(t, s, "GET", "/api/shows/7/episodes", "")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[{"season_number":0,"name":"Specials","episodes":[{"id":5,"episode_number":1,"absolute_number":null,"name":"OVA","air_date":null,"has_file":false}]}]}` {
		t.Errorf("episodes = %s", got)
	}

	rec = doBody(t, s, "PUT", "/api/episodes/9/assignment", `{"info_hash": " ABC ", "file_path": "Pack/E01.mkv"}`)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || review.id != 9 || review.hash != "ABC" || review.path != "Pack/E01.mkv" {
		t.Errorf("override = %d %q, asked %d %q %q", rec.Code, rec.Body, review.id, review.hash, review.path)
	}

	rec = doBody(t, s, "GET", "/api/torrents/ABC/files", "")
	want2 := `{"items":[{"path":"Pack/Film.mkv","size":10,"mapped_to":{"movie_id":7}},` +
		`{"path":"Pack/Show.S01E01E02.mkv","size":20,"mapped_to":{"episodes":[{"episode_id":1,"season":1,"episode":1,"absolute_number":null,"name":"One"},{"episode_id":2,"season":1,"episode":2,"absolute_number":null,"name":"Two"}]}},` +
		`{"path":"Pack/info.nfo","size":1,"mapped_to":null}]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want2 || review.hash != "ABC" {
		t.Errorf("files = %s", got)
	}
}

// Without a review service every review route answers 503.
func TestReviewRoutes_NotConfigured(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range [][3]string{
		{"PATCH", "/api/assignment-jobs/1/proposals", `{"items": [{"id": 1, "decision": "accepted"}]}`},
		{"GET", "/api/shows/1/episodes", ""},
		{"PUT", "/api/episodes/1/assignment", `{"info_hash": "a", "file_path": "b"}`},
		{"PUT", "/api/movies/1/assignment", `{"info_hash": "a", "file_path": "b"}`},
		{"GET", "/api/torrents/abc/files", ""},
	} {
		rec := doBody(t, s, r[0], r[1], r[2])
		if rec.Code != http.StatusServiceUnavailable || decodeError(t, rec).Code != CodeUnavailable {
			t.Errorf("%s %s = %d %s", r[0], r[1], rec.Code, rec.Body)
		}
	}
}
