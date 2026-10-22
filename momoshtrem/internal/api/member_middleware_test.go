package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/status"
)

// fakeMembers resolves the ids it knows (the first is the default) and
// answers the CRUD calls with err, recording them.
type fakeMembers struct {
	mu         sync.Mutex
	ids        []int64
	resolveErr error
	resolves   int
	err        error
	created    []string
	colors     []*string
	patches    []member.Patch
	deleted    []int64
}

func (f *fakeMembers) Resolve(_ context.Context, header string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolves++
	if f.resolveErr != nil {
		return 0, f.resolveErr
	}
	if header == "" {
		return f.ids[0], nil
	}
	id, err := member.ParseID(header)
	if err != nil {
		return 0, err
	}
	if !slices.Contains(f.ids, id) {
		return 0, member.ErrUnknown
	}
	return id, nil
}

var memberCreated = time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)

func (f *fakeMembers) view(id int64, name, color string) member.View {
	return member.View{Member: member.Member{ID: id, Name: name, Color: color, CreatedAt: memberCreated},
		Trakt: member.TraktSummary{State: "off"}}
}

func (f *fakeMembers) List(context.Context) ([]member.View, error) {
	username := "teymur"
	teymur := f.view(1, "Teymur", "teal")
	teymur.Trakt = member.TraktSummary{State: "connected", Username: &username}
	return []member.View{teymur, f.view(2, "Sam", "rose")}, f.err
}

func (f *fakeMembers) Create(_ context.Context, name string, color *string) (member.View, error) {
	f.created = append(f.created, name)
	f.colors = append(f.colors, color)
	c := "rose"
	if color != nil {
		c = *color
	}
	return f.view(3, name, c), f.err
}

func (f *fakeMembers) Update(_ context.Context, id int64, p member.Patch) (member.View, error) {
	f.patches = append(f.patches, p)
	v := f.view(id, "Sam", "rose")
	if p.Name != nil {
		v.Name = *p.Name
	}
	if p.Color != nil {
		v.Color = *p.Color
	}
	return v, f.err
}

func (f *fakeMembers) Delete(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return f.err
}

func strPtr(s string) *string { return &s }

func (f *fakeMembers) resolveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolves
}

// memberEngine serves the middleware alone, each route answering the
// member its context names (0: none).
func memberEngine(m Members) *gin.Engine {
	r := gin.New()
	r.Use(memberMiddleware(m))
	seen := func(c *gin.Context) {
		id, _ := member.IDFrom(c.Request.Context())
		c.JSON(200, gin.H{"member": id})
	}
	for _, path := range []string{"/api/library", "/api/status", "/api/members", "/api/members/:id", "/healthz"} {
		r.GET(path, seen)
	}
	return r
}

func TestMemberMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		header   *string // nil: not sent
		status   int
		code     string
		member   int64 // the context's, when it answers 200
		resolves bool
	}{
		{"no header is the lowest", "/api/library", nil, 200, "", 1, true},
		{"an empty header is the lowest", "/api/library", strPtr(""), 200, "", 1, true},
		{"a member's id", "/api/library", strPtr("3"), 200, "", 3, true},
		{"not an id", "/api/library", strPtr("sam"), 400, CodeBadRequest, 0, true},
		{"not positive", "/api/library", strPtr("0"), 400, CodeBadRequest, 0, true},
		{"an id nobody has", "/api/library", strPtr("2"), 400, CodeUnknownMember, 0, true},
		{"status with a member's id", "/api/status", strPtr("3"), 200, "", 3, true},
		{"status with an id nobody has goes on without a member", "/api/status", strPtr("2"), 200, "", 0, true},
		{"status with a malformed header", "/api/status", strPtr("x"), 400, CodeBadRequest, 0, true},
		{"/members never reads it", "/api/members", strPtr("x"), 200, "", 0, false},
		{"/members/{id} never reads it", "/api/members/2", strPtr("2"), 200, "", 0, false},
		{"outside /api", "/healthz", strPtr("x"), 200, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeMembers{ids: []int64{1, 3}}
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.header != nil {
				req.Header.Set(MemberHeader, *tt.header)
			}
			rec := httptest.NewRecorder()
			memberEngine(f).ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, tt.status)
			}
			if tt.code != "" {
				if got := decodeError(t, rec).Code; got != tt.code {
					t.Errorf("code = %s, want %s", got, tt.code)
				}
			} else {
				var body struct{ Member int64 }
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Member != tt.member {
					t.Errorf("context member = %d (%v), want %d", body.Member, err, tt.member)
				}
			}
			if got := f.resolveCount() > 0; got != tt.resolves {
				t.Errorf("resolved = %v, want %v", got, tt.resolves)
			}
		})
	}

	t.Run("the store fails", func(t *testing.T) {
		f := &fakeMembers{ids: []int64{1}, resolveErr: errors.New("db down")}
		rec := httptest.NewRecorder()
		memberEngine(f).ServeHTTP(rec, httptest.NewRequest("GET", "/api/library", nil))
		if rec.Code != 500 || decodeError(t, rec).Code != CodeInternalError {
			t.Errorf("= %d %s, want 500", rec.Code, rec.Body)
		}
		// Status gives its own report (a 503 for the database), without a
		// member.
		rec = httptest.NewRecorder()
		memberEngine(f).ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))
		if rec.Code != 200 || rec.Body.String() != `{"member":0}` {
			t.Errorf("status = %d %s, want it to go on without a member", rec.Code, rec.Body)
		}
	})
	if memberMiddleware(nil) != nil {
		t.Error("memberMiddleware(nil) isn't nil")
	}
}

// memberStatus answers /status, recording the member each call's context
// names (0: none).
type memberStatus struct{ seen []int64 }

func (s *memberStatus) Report(ctx context.Context, _ bool) status.Report {
	id, _ := member.IDFrom(ctx)
	s.seen = append(s.seen, id)
	return healthyReport()
}

// On the server: after the token check, before every /api route; CORS
// lets browsers send the header.
func TestMemberMiddleware_Server(t *testing.T) {
	const token = "s3cret-token-value"
	reporter := &memberStatus{}
	f := &fakeMembers{ids: []int64{1, 2}}
	s := newTestServer(Options{APIToken: token, CORSAllowedOrigins: []string{"https://app.example"}, Status: reporter, Members: f})

	rec := do(t, s, "GET", "/api/status", map[string]string{MemberHeader: "x"})
	if rec.Code != 401 || f.resolveCount() != 0 {
		t.Errorf("without the token = %d after %d resolves, want 401 first", rec.Code, f.resolveCount())
	}
	for _, header := range []string{"2", "9", ""} {
		if rec := do(t, s, "GET", "/api/status", map[string]string{TokenHeader: token, MemberHeader: header}); rec.Code != 200 {
			t.Errorf("status with %q = %d %s", header, rec.Code, rec.Body)
		}
	}
	if want := []int64{2, 0, 1}; !slices.Equal(reporter.seen, want) {
		t.Errorf("status saw members %v, want %v", reporter.seen, want)
	}
	rec = do(t, s, "GET", "/api/integrations/trakt", map[string]string{TokenHeader: token, MemberHeader: "9"})
	if rec.Code != 400 || decodeError(t, rec).Code != CodeUnknownMember {
		t.Errorf("an unknown member = %d %s", rec.Code, rec.Body)
	}
	// Unknown routes go through it too, like the token check.
	rec = do(t, s, "GET", "/api/nowhere", map[string]string{TokenHeader: token, MemberHeader: "x"})
	if rec.Code != 400 {
		t.Errorf("an unknown route with a malformed member = %d, want 400", rec.Code)
	}

	rec = do(t, s, "OPTIONS", "/api/status", map[string]string{"Origin": "https://app.example", "Access-Control-Request-Headers": MemberHeader})
	if got := rec.Header().Get("Access-Control-Allow-Headers"); rec.Code != 204 || !strings.Contains(got, MemberHeader) {
		t.Errorf("preflight = %d, Allow-Headers %q, want %s included", rec.Code, got, MemberHeader)
	}
}
