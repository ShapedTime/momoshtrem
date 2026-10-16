package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
)

type fakeStorage struct {
	report    *storagecap.Report
	err       error // from both
	updateErr error
	patch     *library.StorageSettingsPatch
}

func (f *fakeStorage) Storage(context.Context) (*storagecap.Report, error) { return f.report, f.err }

func (f *fakeStorage) UpdateStorage(_ context.Context, p library.StorageSettingsPatch) (*storagecap.Report, error) {
	f.patch = &p
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.report, f.err
}

func TestStorageRoutes_Unavailable(t *testing.T) {
	s := newTestServer(Options{})
	for _, r := range []struct{ method, body string }{{"GET", ""}, {"PUT", `{"limit_bytes":null}`}} {
		rec := doBody(t, s, r.method, "/api/settings/storage", r.body)
		if e := errorOf(t, rec.Body.Bytes()); rec.Code != 503 || e.Code != CodeUnavailable {
			t.Errorf("%s = %d %s, want 503", r.method, rec.Code, rec.Body)
		}
	}
}

func TestStorageRoutes(t *testing.T) {
	report := &storagecap.Report{Source: storagecap.SourceAuto, EffectiveBytes: 7}

	t.Run("GET", func(t *testing.T) {
		s := newTestServer(Options{Storage: &fakeStorage{report: report}})
		rec := doBody(t, s, "GET", "/api/settings/storage", "")
		var got map[string]json.RawMessage
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || string(got["source"]) != `"auto"` || string(got["effective_bytes"]) != `7` {
			t.Errorf("GET = %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("PUT parses the patch", func(t *testing.T) {
		f := &fakeStorage{report: report}
		s := newTestServer(Options{Storage: f})
		rec := doBody(t, s, "PUT", "/api/settings/storage", `{"limit_bytes":null,"smart":{"episodes_ahead":3}}`)
		if rec.Code != 200 || f.patch == nil || !f.patch.SetLimit || f.patch.LimitBytes != nil || *f.patch.Smart.EpisodesAhead != 3 {
			t.Errorf("PUT = %d %s, patch %+v", rec.Code, rec.Body, f.patch)
		}
	})

	for _, tt := range []struct {
		name      string
		body      string
		updateErr error
		status    int
		code      string
		field     string
	}{
		{"not an object", `[1]`, nil, 400, CodeBadRequest, ""},
		{"a bad value is 422 naming the field", `{"smart":{"episodes_ahead":9}}`, nil, 422, CodeValidationError, "smart.episodes_ahead"},
		{"a nested bad value", `{"background":{"windows":[{"start":"01:00","end":"01:00"}]}}`, nil, 422, CodeValidationError, "background.windows"},
		{"the service's 422", `{"limit_bytes":1}`, &storagecap.FieldError{Field: "limit_bytes", Message: "too small"}, 422, CodeValidationError, "limit_bytes"},
		{"a failure", `{}`, errors.New("db down"), 500, CodeInternalError, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStorage{report: report, updateErr: tt.updateErr}
			s := newTestServer(Options{Storage: f})
			rec := doBody(t, s, "PUT", "/api/settings/storage", tt.body)
			e := errorOf(t, rec.Body.Bytes())
			if rec.Code != tt.status || e.Code != tt.code || e.Field != tt.field || e.Message == "" {
				t.Errorf("PUT = %d %s, want %d %s %q", rec.Code, rec.Body, tt.status, tt.code, tt.field)
			}
		})
	}

	t.Run("409 limit_below_committed carries what's committed", func(t *testing.T) {
		f := &fakeStorage{updateErr: &storagecap.CommittedError{CommittedKeptBytes: 9 << 30, KeptMaxBytes: 5 << 30}}
		s := newTestServer(Options{Storage: f})
		rec := doBody(t, s, "PUT", "/api/settings/storage", `{"limit_bytes":10737418240}`)
		var body struct {
			Error struct {
				Code               string `json:"code"`
				Message            string `json:"message"`
				CommittedKeptBytes int64  `json:"committed_kept_bytes"`
			} `json:"error"`
		}
		if rec.Code != 409 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error.Code != CodeLimitBelowCommitted ||
			body.Error.CommittedKeptBytes != 9<<30 || body.Error.Message == "" {
			t.Errorf("409 = %d %s", rec.Code, rec.Body)
		}
	})
}
