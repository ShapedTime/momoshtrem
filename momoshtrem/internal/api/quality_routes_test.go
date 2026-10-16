package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// fakeQualityProfiles validates like the repository and keeps the profile
// in memory.
type fakeQualityProfiles struct {
	profile library.QualityProfile
	err     error // returned by both calls when set
	saves   int
}

func (f *fakeQualityProfiles) QualityProfile(context.Context) (library.QualityProfile, error) {
	return f.profile, f.err
}

func (f *fakeQualityProfiles) SetQualityProfile(_ context.Context, p library.QualityProfile) (library.QualityProfile, error) {
	if f.err != nil {
		return p, f.err
	}
	p, err := p.Validate()
	if err != nil {
		return p, err
	}
	f.profile = p
	f.saves++
	return p, nil
}

func newQualityRouter(profiles QualityProfiles) *gin.Engine {
	r := gin.New()
	registerQualityProfileRoutes(r.Group("/api"), profiles)
	return r
}

func serveQuality(t *testing.T, r http.Handler, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/settings/quality-profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeProfile(t *testing.T, rec *httptest.ResponseRecorder) library.QualityProfile {
	t.Helper()
	var p library.QualityProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return p
}

func TestQualityRoutes_Get(t *testing.T) {
	fake := &fakeQualityProfiles{profile: library.DefaultQualityProfile()}
	rec := serveQuality(t, newQualityRouter(fake), http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	// The contract's shape, nulls and empty arrays included.
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"preferred_resolution": "1080p", "max_resolution": nil, "max_gb_per_hour": 8.0,
		"prefer_x265": false, "prefer_hdr": false, "prefer_dolby_vision": false,
		"avoid_terms": []any{"CAM", "HDCAM", "TS", "HDTS", "TELESYNC"}, "preferred_groups": []any{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestQualityRoutes_Put(t *testing.T) {
	fake := &fakeQualityProfiles{profile: library.DefaultQualityProfile()}
	r := newQualityRouter(fake)

	body := `{"preferred_resolution": "2160p", "max_resolution": "2160p", "max_gb_per_hour": 12.5,
		"prefer_x265": true, "prefer_hdr": true, "prefer_dolby_vision": true,
		"avoid_terms": ["CAM", " ts ", "TS"], "preferred_groups": ["FLUX", "NTb"]}`
	rec := serveQuality(t, r, http.MethodPut, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	got := decodeProfile(t, rec)
	if got.PreferredResolution != "2160p" || got.MaxResolution == nil || *got.MaxResolution != "2160p" ||
		got.MaxGBPerHour != 12.5 || !got.PreferX265 || !got.PreferHDR || !got.PreferDolbyVision {
		t.Errorf("saved = %+v", got)
	}
	if want := []string{"CAM", "ts"}; !reflect.DeepEqual(got.AvoidTerms, want) {
		t.Errorf("avoid_terms = %q, want the normalised %q", got.AvoidTerms, want)
	}
	if !reflect.DeepEqual(fake.profile, got) {
		t.Errorf("stored %+v, answered %+v", fake.profile, got)
	}

	// GET reads what PUT saved.
	if again := decodeProfile(t, serveQuality(t, r, http.MethodGet, "")); !reflect.DeepEqual(again, got) {
		t.Errorf("GET after PUT = %+v, want %+v", again, got)
	}
}

func TestQualityRoutes_PutMissingFieldsTakeDefaults(t *testing.T) {
	fake := &fakeQualityProfiles{}
	rec := serveQuality(t, newQualityRouter(fake), http.MethodPut, `{"prefer_hdr": true, "max_resolution": null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	want := library.DefaultQualityProfile()
	want.PreferHDR = true
	if got := decodeProfile(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("saved = %+v, want %+v", got, want)
	}
}

func TestQualityRoutes_PutErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"not JSON", `{nope`, http.StatusBadRequest, CodeBadRequest, ""},
		{"empty body", ``, http.StatusBadRequest, CodeBadRequest, ""},
		{"wrong type", `{"max_gb_per_hour": "lots"}`, http.StatusBadRequest, CodeBadRequest, "max_gb_per_hour has the wrong type"},
		{"size limit", `{"max_gb_per_hour": 25}`, http.StatusUnprocessableEntity, CodeValidationError, "max_gb_per_hour must be between 1 and 20"},
		{"size step", `{"max_gb_per_hour": 7.25}`, http.StatusUnprocessableEntity, CodeValidationError, "max_gb_per_hour must be a multiple of 0.5"},
		{"resolution", `{"preferred_resolution": "4K"}`, http.StatusUnprocessableEntity, CodeValidationError, "preferred_resolution must be 720p, 1080p or 2160p"},
		{"limit below preferred", `{"preferred_resolution": "2160p", "max_resolution": "1080p"}`, http.StatusUnprocessableEntity, CodeValidationError, "max_resolution must be at least preferred_resolution"},
		{"blank term", `{"avoid_terms": ["CAM", ""]}`, http.StatusUnprocessableEntity, CodeValidationError, "avoid_terms must be 1–30 characters each"},
		{"long group", `{"preferred_groups": ["` + strings.Repeat("g", 31) + `"]}`, http.StatusUnprocessableEntity, CodeValidationError, "preferred_groups must be 1–30 characters each"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeQualityProfiles{profile: library.DefaultQualityProfile()}
			rec := serveQuality(t, newQualityRouter(fake), http.MethodPut, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			got := decodeError(t, rec)
			if got.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
			}
			if tt.wantMsg != "" && got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}
			if fake.saves != 0 {
				t.Errorf("saved %d times, want none", fake.saves)
			}
		})
	}
}

func TestQualityRoutes_StoreFails(t *testing.T) {
	fake := &fakeQualityProfiles{err: errors.New("connection reset")}
	r := newQualityRouter(fake)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rec := serveQuality(t, r, method, `{}`)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s status = %d, want 500", method, rec.Code)
		}
		if msg := decodeError(t, rec).Message; strings.Contains(msg, "connection") {
			t.Errorf("%s message %q leaks the store's error", method, msg)
		}
	}
}

func TestQualityRoutes_WithoutSettings(t *testing.T) {
	s := newTestServer(Options{})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		req := httptest.NewRequest(method, "/api/settings/quality-profile", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, want 503 (body %s)", method, rec.Code, rec.Body.String())
		}
		if code := decodeError(t, rec).Code; code != CodeUnavailable {
			t.Errorf("%s code = %q, want %q", method, code, CodeUnavailable)
		}
	}
}
