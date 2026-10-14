package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/status"
)

// NewServer switches gin to release mode, but tests that build a gin context
// directly (errors_test.go) can run first and would print debug warnings.
func init() {
	gin.SetMode(gin.TestMode)
}

// fakeStatus returns a fixed report and records how it was called.
type fakeStatus struct {
	report   status.Report
	calls    int
	refreshs int
}

func (f *fakeStatus) Report(_ context.Context, refresh bool) status.Report {
	f.calls++
	if refresh {
		f.refreshs++
	}
	return f.report
}

func healthyReport() status.Report {
	size := int64(1024)
	return status.Report{
		Version: "test",
		DB:      status.DBStatus{OK: true, SizeBytes: &size},
	}
}

// newTestServer builds a real Server (full middleware chain and routes) with
// no library dependencies; only /api/status is exercised.
func newTestServer(opts Options) *Server {
	if opts.Status == nil {
		opts.Status = &fakeStatus{report: healthyReport()}
	}
	return NewServer(nil, nil, nil, nil, nil, opts)
}

func do(t *testing.T, s *Server, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorDetail {
	t.Helper()
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error
}

func TestTokenMiddleware(t *testing.T) {
	const token = "s3cret-token-value"

	tests := []struct {
		name       string
		token      string // configured on the server
		header     string // sent by the client; "" means not sent
		path       string
		wantStatus int
		wantCode   string
	}{
		{"missing token", token, "", "/api/status", http.StatusUnauthorized, CodeUnauthorized},
		{"wrong token", token, "nope", "/api/status", http.StatusUnauthorized, CodeUnauthorized},
		{"token prefix", token, token[:5], "/api/status", http.StatusUnauthorized, CodeUnauthorized},
		{"right token", token, token, "/api/status", http.StatusOK, ""},
		{"unknown api route needs token", token, "", "/api/nope", http.StatusUnauthorized, CodeUnauthorized},
		{"unknown api route with token", token, token, "/api/nope", http.StatusNotFound, CodeNotFound},
		{"token unset passes through", "", "", "/api/status", http.StatusOK, ""},
		{"token unset ignores header", "", "anything", "/api/status", http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(Options{APIToken: tt.token})
			headers := map[string]string{}
			if tt.header != "" {
				headers[TokenHeader] = tt.header
			}

			rec := do(t, s, http.MethodGet, tt.path, headers)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				if got := decodeError(t, rec).Code; got != tt.wantCode {
					t.Errorf("error code = %q, want %q", got, tt.wantCode)
				}
			}
			if strings.Contains(rec.Body.String(), token) {
				t.Errorf("response leaks the token: %s", rec.Body)
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	const allowed = "https://momoshtrem.example"

	tests := []struct {
		name        string
		origins     []string
		token       string
		method      string
		origin      string
		headers     map[string]string
		path        string // "" means /api/status
		wantStatus  int
		wantACAO    string // expected Access-Control-Allow-Origin ("" = absent)
		wantNoCORS  bool   // expect no CORS headers at all, not even Vary
		wantMethods bool
	}{
		{
			name: "allowed origin echoed", origins: []string{allowed},
			method: http.MethodGet, origin: allowed,
			wantStatus: http.StatusOK, wantACAO: allowed, wantMethods: true,
		},
		{
			name: "disallowed origin gets no allow header", origins: []string{allowed},
			method: http.MethodGet, origin: "https://evil.example",
			wantStatus: http.StatusOK,
		},
		{
			name: "empty allowlist sends no CORS headers", origins: nil,
			method: http.MethodGet, origin: allowed,
			wantStatus: http.StatusOK, wantNoCORS: true,
		},
		{
			name: "preflight from allowed origin skips token check", origins: []string{allowed}, token: "tok",
			method: http.MethodOptions, origin: allowed,
			headers:    map[string]string{"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": TokenHeader},
			wantStatus: http.StatusNoContent, wantACAO: allowed, wantMethods: true,
		},
		{
			name: "preflight for a POST-only route skips token check", origins: []string{allowed}, token: "tok",
			method: http.MethodOptions, origin: allowed, path: "/api/movies",
			headers:    map[string]string{"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "Content-Type, " + TokenHeader},
			wantStatus: http.StatusNoContent, wantACAO: allowed, wantMethods: true,
		},
		{
			name: "preflight from disallowed origin falls through to auth", origins: []string{allowed}, token: "tok",
			method: http.MethodOptions, origin: "https://evil.example",
			headers:    map[string]string{"Access-Control-Request-Method": "POST"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "allowed origin still needs token for real requests", origins: []string{allowed}, token: "tok",
			method: http.MethodGet, origin: allowed,
			wantStatus: http.StatusUnauthorized, wantACAO: allowed, wantMethods: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(Options{APIToken: tt.token, CORSAllowedOrigins: tt.origins})
			headers := map[string]string{"Origin": tt.origin}
			for k, v := range tt.headers {
				headers[k] = v
			}

			path := tt.path
			if path == "" {
				path = "/api/status"
			}

			rec := do(t, s, tt.method, path, headers)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			h := rec.Header()
			if got := h.Get("Access-Control-Allow-Origin"); got != tt.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantACAO)
			}
			if tt.wantMethods {
				if got := h.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PATCH") {
					t.Errorf("Access-Control-Allow-Methods = %q, want PATCH included", got)
				}
				if got := h.Get("Access-Control-Allow-Headers"); !strings.Contains(got, TokenHeader) {
					t.Errorf("Access-Control-Allow-Headers = %q, want %s included", got, TokenHeader)
				}
				if got := h.Get("Vary"); got != "Origin" {
					t.Errorf("Vary = %q, want Origin", got)
				}
			}
			if tt.wantNoCORS {
				for name := range h {
					if strings.HasPrefix(name, "Access-Control-") || name == "Vary" {
						t.Errorf("unexpected header %s: %q", name, h.Get(name))
					}
				}
			}
			if h.Get("Access-Control-Allow-Origin") == "*" {
				t.Error("wildcard origin must never be sent")
			}
		})
	}
}

func TestRecovery(t *testing.T) {
	// Gin writes the panic and stack to DefaultErrorWriter when the
	// middleware is built; capture it to check the panic is still logged.
	var logged bytes.Buffer
	prev := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logged
	t.Cleanup(func() { gin.DefaultErrorWriter = prev })

	s := newTestServer(Options{})
	s.router.GET("/api/panic", func(*gin.Context) { panic("boom") })

	rec := do(t, s, http.MethodGet, "/api/panic", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", rec.Code, rec.Body)
	}
	if got := decodeError(t, rec).Code; got != CodeInternalError {
		t.Errorf("error code = %q, want %q", got, CodeInternalError)
	}
	if !strings.Contains(logged.String(), "boom") {
		t.Errorf("panic not logged, got %q", logged.String())
	}
}
