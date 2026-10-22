package api

import (
	"errors"
	"slices"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/member"
)

func TestMemberRoutes(t *testing.T) {
	f := &fakeMembers{ids: []int64{1, 2}}
	s := newTestServer(Options{Members: f})

	rec := doBody(t, s, "GET", "/api/members", "")
	want := `{"items":[` +
		`{"id":1,"name":"Teymur","color":"teal","created_at":"2026-10-22T08:00:00Z","trakt":{"state":"connected","username":"teymur"}},` +
		`{"id":2,"name":"Sam","color":"rose","created_at":"2026-10-22T08:00:00Z","trakt":{"state":"off","username":null}}]}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("GET = %d %s, want %s", rec.Code, rec.Body, want)
	}

	// POST: color is optional (null too); unknown fields are ignored.
	rec = doBody(t, s, "POST", "/api/members", `{"name":" Ari ","color":"lime","theme":"dark"}`)
	if want := `{"id":3,"name":" Ari ","color":"lime","created_at":"2026-10-22T08:00:00Z","trakt":{"state":"off","username":null}}`; rec.Code != 201 || rec.Body.String() != want {
		t.Errorf("POST = %d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{`{"name":"Kim"}`, `{"name":"Kim","color":null}`} {
		if rec := doBody(t, s, "POST", "/api/members", body); rec.Code != 201 {
			t.Errorf("POST %s = %d %s", body, rec.Code, rec.Body)
		}
	}
	// No name is the service's to refuse (as an empty one).
	if rec := doBody(t, s, "POST", "/api/members", `{"color":"sand"}`); rec.Code != 201 {
		t.Errorf("POST without a name = %d %s", rec.Code, rec.Body)
	}
	if want := []string{" Ari ", "Kim", "Kim", ""}; !slices.Equal(f.created, want) {
		t.Errorf("created %q, want %q", f.created, want)
	}
	if f.colors[0] == nil || *f.colors[0] != "lime" || f.colors[1] != nil || f.colors[2] != nil {
		t.Errorf("colors = %v", f.colors)
	}

	// PATCH: only what's sent.
	rec = doBody(t, s, "PATCH", "/api/members/2", `{"color":"sand"}`)
	if want := `{"id":2,"name":"Sam","color":"sand","created_at":"2026-10-22T08:00:00Z","trakt":{"state":"off","username":null}}`; rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("PATCH = %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(t, s, "PATCH", "/api/members/2", `{}`); rec.Code != 200 {
		t.Errorf("an empty PATCH = %d %s", rec.Code, rec.Body)
	}
	if p := f.patches[0]; p.Name != nil || p.Color == nil || *p.Color != "sand" {
		t.Errorf("patch = %+v", p)
	}
	if p := f.patches[1]; p.Name != nil || p.Color != nil {
		t.Errorf("empty patch = %+v", p)
	}

	if rec := doBody(t, s, "DELETE", "/api/members/2", ""); rec.Code != 204 || rec.Body.Len() != 0 || len(f.deleted) != 1 || f.deleted[0] != 2 {
		t.Errorf("DELETE = %d %s, deleted %v", rec.Code, rec.Body, f.deleted)
	}
}

func TestMemberRoutes_BadRequests(t *testing.T) {
	tests := []struct {
		method, path, body string
		status             int
		code, field        string
	}{
		{"POST", "/api/members", ``, 400, CodeBadRequest, ""},
		{"POST", "/api/members", `null`, 400, CodeBadRequest, ""},
		{"POST", "/api/members", `[]`, 400, CodeBadRequest, ""},
		{"POST", "/api/members", `"Sam"`, 400, CodeBadRequest, ""},
		{"POST", "/api/members", `{"name":`, 400, CodeBadRequest, ""},
		{"POST", "/api/members", `{"name":5}`, 422, CodeValidationError, "name"},
		{"POST", "/api/members", `{"name":null}`, 422, CodeValidationError, "name"},
		{"POST", "/api/members", `{"name":"Sam","color":3}`, 422, CodeValidationError, "color"},
		{"PATCH", "/api/members/2", `[]`, 400, CodeBadRequest, ""},
		{"PATCH", "/api/members/2", `{"name":null}`, 422, CodeValidationError, "name"},
		{"PATCH", "/api/members/2", `{"color":null}`, 422, CodeValidationError, "color"},
		{"PATCH", "/api/members/two", `{}`, 400, CodeBadRequest, ""},
		{"PATCH", "/api/members/0", `{}`, 400, CodeBadRequest, ""},
		{"DELETE", "/api/members/two", ``, 400, CodeBadRequest, ""},
	}
	for _, tt := range tests {
		f := &fakeMembers{ids: []int64{1}}
		rec := doBody(t, newTestServer(Options{Members: f}), tt.method, tt.path, tt.body)
		e := errorOf(t, rec.Body.Bytes())
		if rec.Code != tt.status || e.Code != tt.code || e.Field != tt.field {
			t.Errorf("%s %s %s = %d %s, want %d %s %q", tt.method, tt.path, tt.body, rec.Code, rec.Body, tt.status, tt.code, tt.field)
		}
		if len(f.created)+len(f.patches)+len(f.deleted) != 0 {
			t.Errorf("%s %s %s reached the service", tt.method, tt.path, tt.body)
		}
	}
}

func TestMemberRoutes_Errors(t *testing.T) {
	tests := []struct {
		err         error
		status      int
		code, field string
	}{
		{&member.ValidationError{Field: "name", Message: "name must be 1 to 32 characters"}, 422, CodeValidationError, "name"},
		{&member.ValidationError{Field: "color", Message: "color must be …"}, 422, CodeValidationError, "color"},
		{member.ErrNotFound, 404, CodeNotFound, ""},
		{member.ErrLimit, 409, CodeMemberLimit, ""},
		{member.ErrNameTaken, 409, CodeMemberNameTaken, ""},
		{member.ErrLastMember, 409, CodeLastMember, ""},
		{errors.New("db down"), 500, CodeInternalError, ""},
	}
	routes := []struct{ method, path, body string }{
		{"GET", "/api/members", ""},
		{"POST", "/api/members", `{"name":"Sam"}`},
		{"PATCH", "/api/members/2", `{"name":"Sam"}`},
		{"DELETE", "/api/members/2", ""},
	}
	for _, tt := range tests {
		for _, r := range routes {
			rec := doBody(t, newTestServer(Options{Members: &fakeMembers{ids: []int64{1}, err: tt.err}}), r.method, r.path, r.body)
			e := errorOf(t, rec.Body.Bytes())
			if rec.Code != tt.status || e.Code != tt.code || e.Field != tt.field || e.Message == "" {
				t.Errorf("%s %s with %v = %d %s", r.method, r.path, tt.err, rec.Code, rec.Body)
			}
		}
	}

	// Without the service every route is 503, and nothing reads the header.
	s := newTestServer(Options{})
	for _, r := range routes {
		if rec := doBody(t, s, r.method, r.path, r.body); rec.Code != 503 {
			t.Errorf("%s %s without members = %d", r.method, r.path, rec.Code)
		}
	}
	if rec := do(t, s, "GET", "/api/status", map[string]string{MemberHeader: "x"}); rec.Code != 200 {
		t.Errorf("status without members, with a malformed header = %d, want 200 as before", rec.Code)
	}
}
