package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
)

// /members and the member header end to end, over Postgres (#241): CRUD
// and its codes, the header rules, and the cap and "never zero members"
// under concurrent requests.
func TestMembers_Integration(t *testing.T) {
	db := openRerunDB(t)
	reporter := &memberStatus{}
	s := newTestServer(Options{Status: reporter, Members: member.NewService(library.NewMemberRepository(db))})
	withMember := func(method, path, body, header string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(MemberHeader, header)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	var list struct{ Items []member.View }
	rec := doBody(t, s, "GET", "/api/members", "")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Items) != 1 ||
		list.Items[0].ID != 1 || list.Items[0].Name != "Me" || list.Items[0].Color != "teal" || list.Items[0].Trakt.State != "off" {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}

	// Member 2 is unknown until added, then the header names them.
	if rec := withMember("GET", "/api/library", "", "2"); rec.Code != 400 || decodeError(t, rec).Code != CodeUnknownMember {
		t.Errorf("an unknown member = %d %s", rec.Code, rec.Body)
	}
	var sam member.View
	rec = doBody(t, s, "POST", "/api/members", `{"name":"  Sam  "}`)
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &sam) != nil || sam.ID != 2 || sam.Name != "Sam" || sam.Color != "rose" {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if rec := withMember("GET", "/api/library", "", "2"); rec.Code != 503 { // past the middleware: no catalog here
		t.Errorf("member 2 = %d %s, want the route's 503", rec.Code, rec.Body)
	}
	for _, header := range []string{"2", "", "7"} {
		if rec := withMember("GET", "/api/status", "", header); rec.Code != 200 {
			t.Errorf("status with %q = %d", header, rec.Code)
		}
	}
	if fmt.Sprint(reporter.seen) != "[2 1 0]" {
		t.Errorf("status saw %v, want [2 1 0]", reporter.seen)
	}

	for _, tt := range []struct {
		method, path, body string
		status             int
		code, field        string
	}{
		{"POST", "/api/members", `{"name":"SAM"}`, 409, CodeMemberNameTaken, ""},
		{"POST", "/api/members", `{"name":"` + strings.Repeat("é", 33) + `"}`, 422, CodeValidationError, "name"},
		{"POST", "/api/members", `{"name":"Ari","color":"blue"}`, 422, CodeValidationError, "color"},
		{"PATCH", "/api/members/2", `{"name":"me"}`, 409, CodeMemberNameTaken, ""},
		{"PATCH", "/api/members/9", `{"name":"Nine"}`, 404, CodeNotFound, ""},
		{"DELETE", "/api/members/9", ``, 404, CodeNotFound, ""},
	} {
		rec := doBody(t, s, tt.method, tt.path, tt.body)
		if e := errorOf(t, rec.Body.Bytes()); rec.Code != tt.status || e.Code != tt.code || e.Field != tt.field {
			t.Errorf("%s %s %s = %d %s", tt.method, tt.path, tt.body, rec.Code, rec.Body)
		}
	}
	rec = doBody(t, s, "PATCH", "/api/members/2", `{"name":"Samira","color":"sand"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"Samira","color":"sand"`) {
		t.Errorf("PATCH = %d %s", rec.Code, rec.Body)
	}

	// Ten adds at once: four fit beside Me and Samira.
	codes := parallel(10, func(i int) int {
		return doBody(t, s, "POST", "/api/members", fmt.Sprintf(`{"name":"Person %d"}`, i)).Code
	})
	if codes[201] != member.MaxMembers-2 || codes[409] != 10-codes[201] {
		t.Errorf("concurrent adds answered %v", codes)
	}
	// Removing everyone at once leaves one.
	if rec := doBody(t, s, "GET", "/api/members", ""); json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Items) != member.MaxMembers {
		t.Fatalf("GET = %s, want %d members", rec.Body, member.MaxMembers)
	}
	codes = parallel(len(list.Items), func(i int) int {
		return doBody(t, s, "DELETE", fmt.Sprintf("/api/members/%d", list.Items[i].ID), "").Code
	})
	if codes[204] != member.MaxMembers-1 || codes[409] != 1 {
		t.Errorf("concurrent removes answered %v", codes)
	}
	if rec := doBody(t, s, "GET", "/api/members", ""); json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Items) != 1 {
		t.Errorf("GET after = %s, want one member", rec.Body)
	}
}

// parallel runs fn n times at once and counts the status codes it answers.
func parallel(n int, fn func(i int) int) map[int]int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	codes := map[int]int{}
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code := fn(i)
			mu.Lock()
			codes[code]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	return codes
}
