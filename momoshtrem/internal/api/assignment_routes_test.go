package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// fakeJobs is an AssignmentJobs that answers err (or fixed results) and
// records what it was asked.
type fakeJobs struct {
	err      error
	created  bool
	media    library.JobMediaType
	id       int64
	req      service.AssignRequest
	statuses []library.JobStatus
	called   string
}

func (f *fakeJobs) Assign(_ context.Context, media library.JobMediaType, id int64, req service.AssignRequest) (int64, bool, error) {
	f.called, f.media, f.id, f.req = "assign", media, id, req
	if f.err != nil {
		return 0, false, f.err
	}
	return 42, f.created, nil
}

func (f *fakeJobs) AssignShowAndWait(_ context.Context, id int64, req service.AssignRequest) (*service.ShowAssignmentResult, error) {
	f.called, f.id, f.req = "show-wait", id, req
	if f.err != nil {
		return nil, f.err
	}
	return &service.ShowAssignmentResult{JobID: 42, Summary: service.AssignmentSummary{TotalFiles: 1, Matched: 1},
		Matched: []service.MatchedAssignment{{EpisodeID: 9, Season: 1, Episode: 1}}}, nil
}

func (f *fakeJobs) AssignMovieAndWait(_ context.Context, id int64, req service.AssignRequest) (*service.MovieAssignmentResult, error) {
	f.called, f.id, f.req = "movie-wait", id, req
	if f.err != nil {
		return nil, f.err
	}
	return &service.MovieAssignmentResult{JobID: 42, Assignment: &library.TorrentAssignment{ID: 3, InfoHash: "h", FilePath: "Film.mkv"}}, nil
}

func (f *fakeJobs) Detail(_ context.Context, id int64) (*service.JobDetail, error) {
	f.called, f.id = "detail", id
	return f.detail(id)
}

func (f *fakeJobs) List(_ context.Context, statuses []library.JobStatus) ([]service.JobView, error) {
	f.called, f.statuses = "list", statuses
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

func (f *fakeJobs) Apply(_ context.Context, id int64) (*service.JobDetail, error) {
	f.called, f.id = "apply", id
	return f.detail(id)
}

func (f *fakeJobs) Discard(_ context.Context, id int64) (*service.JobDetail, error) {
	f.called, f.id = "discard", id
	return f.detail(id)
}

func (f *fakeJobs) detail(id int64) (*service.JobDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &service.JobDetail{Job: service.JobView{ID: id, Status: library.JobReady}, Proposals: []service.ProposalView{}}, nil
}

func conflictErr(code string) error {
	return &service.JobConflictError{Code: code, Message: "no: " + code}
}

func failedJob(code string) error {
	return &service.JobFailedError{JobID: 1, Code: code, Message: "failed: " + code}
}

func TestAssignmentRoutes(t *testing.T) {
	const magnetBody = `{"magnet_uri": "magnet:?xt=urn:btih:abc"}`
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		err        error
		created    bool
		wantStatus int
		wantCode   string
		wantCall   string
	}{
		{"assign show", "POST", "/api/shows/7/assign-torrent", magnetBody, nil, true, 202, "", "assign"},
		{"assign show: the open job", "POST", "/api/shows/7/assign-torrent", magnetBody, nil, false, 200, "", "assign"},
		{"assign movie", "POST", "/api/movies/7/assign-torrent", `{"magnet_uri": "m", "file_path": "Film.mkv"}`, nil, true, 202, "", "assign"},
		{"assign without a magnet", "POST", "/api/shows/7/assign-torrent", `{"indexer": "Nyaa"}`, nil, true, 400, CodeBadRequest, ""},
		{"assign with a broken body", "POST", "/api/shows/7/assign-torrent", `{"magnet_uri":`, nil, true, 400, CodeBadRequest, ""},
		{"assign with a fractional season", "POST", "/api/shows/7/assign-torrent", `{"magnet_uri": "m", "hint": {"season": 1.5}}`, nil, true, 400, CodeBadRequest, ""},
		{"assign a bad id", "POST", "/api/shows/x/assign-torrent", magnetBody, nil, true, 400, CodeBadRequest, ""},
		{"assign without a hash", "POST", "/api/shows/7/assign-torrent", magnetBody, library.ErrInvalidMagnet, true, 400, CodeBadRequest, "assign"},
		{"assign a bad hint", "POST", "/api/shows/7/assign-torrent", magnetBody, fmt.Errorf("%w: no", service.ErrInvalidHint), true, 400, CodeBadRequest, "assign"},
		{"assign an unknown show", "POST", "/api/shows/7/assign-torrent", magnetBody, library.ErrShowNotFound, true, 404, CodeNotFound, "assign"},
		{"assign an unknown movie", "POST", "/api/movies/7/assign-torrent", magnetBody, library.ErrMovieNotFound, true, 404, CodeNotFound, "assign"},
		{"assign and wait, show", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, nil, true, 201, "", "show-wait"},
		{"assign and wait, movie", "POST", "/api/movies/7/assign-torrent?wait=true", magnetBody, nil, true, 201, "", "movie-wait"},
		{"wait: no peers", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureMetadataTimeout), true, 504, CodeTimeout, "show-wait"},
		{"wait: no video", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureNoVideos), true, 400, CodeBadRequest, "show-wait"},
		{"wait: a bad magnet", "POST", "/api/movies/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureInvalidMagnet), true, 400, CodeBadRequest, "movie-wait"},
		{"wait: no torrent client", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureNoTorrents), true, 503, CodeUnavailable, "show-wait"},
		{"wait: the title left", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureTitleGone), true, 404, CodeNotFound, "show-wait"},
		{"wait: failed", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureInternal), true, 500, CodeInternalError, "show-wait"},
		{"wait: torrent error", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, failedJob(service.FailureTorrent), true, 500, CodeInternalError, "show-wait"},
		{"wait: no film", "POST", "/api/movies/7/assign-torrent?wait=true", magnetBody, library.ErrNoVideoFiles, true, 400, CodeBadRequest, "movie-wait"},
		{"wait: discarded", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, conflictErr(service.CodeJobClosed), true, 409, service.CodeJobClosed, "show-wait"},
		{"wait: still matching", "POST", "/api/shows/7/assign-torrent?wait=true", magnetBody, context.DeadlineExceeded, true, 504, CodeTimeout, "show-wait"},
		{"list", "GET", "/api/assignment-jobs", "", nil, true, 200, "", "list"},
		{"list by status", "GET", "/api/assignment-jobs?status=applied,failed", "", nil, true, 200, "", "list"},
		{"list an unknown status", "GET", "/api/assignment-jobs?status=applied,done", "", nil, true, 400, CodeBadRequest, ""},
		{"detail", "GET", "/api/assignment-jobs/5", "", nil, true, 200, "", "detail"},
		{"detail of an unknown job", "GET", "/api/assignment-jobs/5", "", library.ErrJobNotFound, true, 404, CodeNotFound, "detail"},
		{"detail with a bad id", "GET", "/api/assignment-jobs/0", "", nil, true, 400, CodeBadRequest, ""},
		{"apply", "POST", "/api/assignment-jobs/5/apply", "", nil, true, 200, "", "apply"},
		{"apply an unknown job", "POST", "/api/assignment-jobs/5/apply", "", library.ErrJobNotFound, true, 404, CodeNotFound, "apply"},
		{"apply a dry run", "POST", "/api/assignment-jobs/5/apply", "", conflictErr(service.CodeDryRun), true, 409, service.CodeDryRun, "apply"},
		{"apply a closed job", "POST", "/api/assignment-jobs/5/apply", "", conflictErr(service.CodeJobClosed), true, 409, service.CodeJobClosed, "apply"},
		{"apply a working job", "POST", "/api/assignment-jobs/5/apply", "", conflictErr(service.CodeJobWorking), true, 409, service.CodeJobWorking, "apply"},
		{"apply with conflicts", "POST", "/api/assignment-jobs/5/apply", "", conflictErr(service.CodeConflictingProposals), true, 409, service.CodeConflictingProposals, "apply"},
		{"apply with pending files", "POST", "/api/assignment-jobs/5/apply", "", conflictErr(service.CodePendingProposals), true, 409, service.CodePendingProposals, "apply"},
		{"discard", "POST", "/api/assignment-jobs/5/discard", "", nil, true, 200, "", "discard"},
		{"discard an applied job", "POST", "/api/assignment-jobs/5/discard", "", conflictErr(service.CodeJobClosed), true, 409, service.CodeJobClosed, "discard"},
		{"discard an unknown job", "POST", "/api/assignment-jobs/5/discard", "", library.ErrJobNotFound, true, 404, CodeNotFound, "discard"},
		{"the client went away", "GET", "/api/assignment-jobs/5", "", context.Canceled, true, StatusClientClosedRequest, CodeClientClosed, "detail"},
		{"a database error", "GET", "/api/assignment-jobs/5", "", errors.New("conn reset by 10.0.0.3"), true, 500, CodeInternalError, "detail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := &fakeJobs{err: tt.err, created: tt.created}
			s := newTestServer(Options{Assignments: jobs})
			rec := doBody(t, s, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec); got.Code != tt.wantCode || strings.Contains(got.Message, "10.0.0.3") {
					t.Errorf("error = %+v, want code %s", got, tt.wantCode)
				}
			}
			if jobs.called != tt.wantCall {
				t.Errorf("called %q, want %q", jobs.called, tt.wantCall)
			}
		})
	}
}

func TestAssignmentRoutes_Bodies(t *testing.T) {
	jobs := &fakeJobs{created: true}
	s := newTestServer(Options{Assignments: jobs})

	rec := doBody(t, s, "POST", "/api/shows/7/assign-torrent",
		`{"magnet_uri": "magnet:?xt=urn:btih:abc", "hint": {"season": 2, "text": "absolute"}, "indexer": "Nyaa"}`)
	if rec.Code != http.StatusAccepted || strings.TrimSpace(rec.Body.String()) != `{"job_id":42}` {
		t.Errorf("assign = %d %s", rec.Code, rec.Body)
	}
	if jobs.media != library.JobTV || jobs.id != 7 || jobs.req.Indexer != "Nyaa" || jobs.req.Hint == nil ||
		*jobs.req.Hint.Season != 2 || jobs.req.Hint.Text != "absolute" || jobs.req.Hint.Episode != nil {
		t.Errorf("request = %v %d %+v", jobs.media, jobs.id, jobs.req)
	}

	rec = doBody(t, s, "POST", "/api/movies/8/assign-torrent", `{"magnet_uri": "m", "file_path": "Pack/Film.mkv"}`)
	if rec.Code != http.StatusAccepted || jobs.media != library.JobMovie || jobs.req.FilePath != "Pack/Film.mkv" {
		t.Errorf("movie assign = %d %+v", rec.Code, jobs.req)
	}

	rec = doBody(t, s, "POST", "/api/shows/7/assign-torrent?wait=true", `{"magnet_uri": "m"}`)
	var show ShowAssignmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &show); err != nil || !show.Success || show.JobID != 42 || show.Summary.Matched != 1 {
		t.Errorf("legacy show = %s", rec.Body)
	}
	rec = doBody(t, s, "POST", "/api/movies/8/assign-torrent?wait=true", `{"magnet_uri": "m"}`)
	var movie MovieAssignmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &movie); err != nil || !movie.Success || movie.JobID != 42 ||
		movie.Assignment == nil || movie.Assignment.FilePath != "Film.mkv" {
		t.Errorf("legacy movie = %s", rec.Body)
	}

	rec = doBody(t, s, "GET", "/api/assignment-jobs", "")
	if strings.TrimSpace(rec.Body.String()) != `{"items":[]}` || !reflect.DeepEqual(jobs.statuses, library.OpenJobStatuses) {
		t.Errorf("list = %s, statuses %v", rec.Body, jobs.statuses)
	}
	doBody(t, s, "GET", "/api/assignment-jobs?status=applied,failed", "")
	if !reflect.DeepEqual(jobs.statuses, []library.JobStatus{library.JobApplied, library.JobFailed}) {
		t.Errorf("statuses = %v", jobs.statuses)
	}

	rec = doBody(t, s, "POST", "/api/assignment-jobs/5/apply", "")
	var detail service.JobDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.Job.ID != 5 || detail.Proposals == nil {
		t.Errorf("apply = %s", rec.Body)
	}
}

// Without a job service every assignment route answers 503.
func TestAssignmentRoutes_NotConfigured(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range [][2]string{
		{"POST", "/api/shows/7/assign-torrent"}, {"POST", "/api/movies/7/assign-torrent"},
		{"GET", "/api/assignment-jobs"}, {"GET", "/api/assignment-jobs/1"},
		{"POST", "/api/assignment-jobs/1/apply"}, {"POST", "/api/assignment-jobs/1/discard"},
	} {
		rec := doBody(t, s, r[0], r[1], `{"magnet_uri": "m"}`)
		if rec.Code != http.StatusServiceUnavailable || decodeError(t, rec).Code != CodeUnavailable {
			t.Errorf("%s %s = %d %s", r[0], r[1], rec.Code, rec.Body)
		}
	}
}
