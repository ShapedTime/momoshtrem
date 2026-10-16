package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestQualityProfile_Validate(t *testing.T) {
	long := strings.Repeat("x", 31)
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("T%d", i)
		}
		return out
	}
	tests := []struct {
		name      string
		edit      func(p *QualityProfile)
		wantField string // "" when valid
		wantMsg   string
	}{
		{"defaults", func(p *QualityProfile) {}, "", ""},
		{"2160p with no limit", func(p *QualityProfile) { p.PreferredResolution = "2160p" }, "", ""},
		{"720p up to 1080p", func(p *QualityProfile) { p.PreferredResolution = "720p"; p.MaxResolution = strPtr("1080p") }, "", ""},
		{"limit equal to preferred", func(p *QualityProfile) { p.PreferredResolution = "2160p"; p.MaxResolution = strPtr("2160p") }, "", ""},
		{"half steps", func(p *QualityProfile) { p.MaxGBPerHour = 7.5 }, "", ""},
		{"bounds", func(p *QualityProfile) { p.MaxGBPerHour = 20 }, "", ""},
		{"30 terms of 30 characters", func(p *QualityProfile) { p.AvoidTerms = many(30); p.AvoidTerms[29] = long[:30] }, "", ""},
		{"no terms", func(p *QualityProfile) { p.AvoidTerms = nil }, "", ""},

		{"unknown preferred", func(p *QualityProfile) { p.PreferredResolution = "4k" }, "preferred_resolution", "preferred_resolution must be 720p, 1080p or 2160p"},
		{"480p preferred", func(p *QualityProfile) { p.PreferredResolution = "480p" }, "preferred_resolution", ""},
		{"720p limit", func(p *QualityProfile) { p.PreferredResolution = "720p"; p.MaxResolution = strPtr("720p") }, "max_resolution", "max_resolution must be 1080p, 2160p or null"},
		{"empty limit", func(p *QualityProfile) { p.MaxResolution = strPtr("") }, "max_resolution", ""},
		{"limit below preferred", func(p *QualityProfile) { p.PreferredResolution = "2160p"; p.MaxResolution = strPtr("1080p") }, "max_resolution", "max_resolution must be at least preferred_resolution"},
		{"under 1 GB/h", func(p *QualityProfile) { p.MaxGBPerHour = 0.5 }, "max_gb_per_hour", "max_gb_per_hour must be between 1 and 20"},
		{"zero GB/h", func(p *QualityProfile) { p.MaxGBPerHour = 0 }, "max_gb_per_hour", "max_gb_per_hour must be between 1 and 20"},
		{"over 20 GB/h", func(p *QualityProfile) { p.MaxGBPerHour = 20.5 }, "max_gb_per_hour", "max_gb_per_hour must be between 1 and 20"},
		{"off step", func(p *QualityProfile) { p.MaxGBPerHour = 7.3 }, "max_gb_per_hour", "max_gb_per_hour must be a multiple of 0.5"},
		{"blank term", func(p *QualityProfile) { p.AvoidTerms = []string{"CAM", "  "} }, "avoid_terms", "avoid_terms must be 1–30 characters each"},
		{"long term", func(p *QualityProfile) { p.AvoidTerms = []string{long} }, "avoid_terms", ""},
		{"31 terms", func(p *QualityProfile) { p.AvoidTerms = many(31) }, "avoid_terms", "avoid_terms can have at most 30 terms"},
		{"long group", func(p *QualityProfile) { p.PreferredGroups = []string{long} }, "preferred_groups", "preferred_groups must be 1–30 characters each"},
		{"31 groups", func(p *QualityProfile) { p.PreferredGroups = many(31) }, "preferred_groups", "preferred_groups can have at most 30 groups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultQualityProfile()
			tt.edit(&p)
			_, err := p.Validate()
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want valid", err)
				}
				return
			}
			var fe *QualityProfileError
			if !errors.As(err, &fe) || !errors.Is(err, ErrInvalidQualityProfile) {
				t.Fatalf("Validate = %v, want a *QualityProfileError", err)
			}
			if fe.Field != tt.wantField {
				t.Errorf("field = %q, want %q", fe.Field, tt.wantField)
			}
			if !strings.Contains(fe.Message, fe.Field) {
				t.Errorf("message %q doesn't name the field", fe.Message)
			}
			if tt.wantMsg != "" && fe.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", fe.Message, tt.wantMsg)
			}
		})
	}
}

func TestQualityProfile_ValidateNormalises(t *testing.T) {
	p := DefaultQualityProfile()
	p.AvoidTerms = []string{" CAM ", "cam", "HDTS", "hdts ", "KorSub"}
	p.PreferredGroups = nil
	got, err := p.Validate()
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if want := []string{"CAM", "HDTS", "KorSub"}; !reflect.DeepEqual(got.AvoidTerms, want) {
		t.Errorf("avoid_terms = %q, want %q", got.AvoidTerms, want)
	}
	if got.PreferredGroups == nil || len(got.PreferredGroups) != 0 {
		t.Errorf("preferred_groups = %#v, want []", got.PreferredGroups)
	}

	// 31 spellings of 30 terms fit once deduplicated.
	p = DefaultQualityProfile()
	p.AvoidTerms = nil
	for i := range 30 {
		p.AvoidTerms = append(p.AvoidTerms, strings.Repeat("a", i+1))
	}
	p.AvoidTerms = append(p.AvoidTerms, "A")
	if got, err := p.Validate(); err != nil || len(got.AvoidTerms) != 30 {
		t.Errorf("Validate = (%d terms, %v), want 30 terms", len(got.AvoidTerms), err)
	}
}

// The profile's JSON is the contract's QualityProfile.
func TestQualityProfile_JSON(t *testing.T) {
	raw, err := json.Marshal(DefaultQualityProfile())
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, raw, `{
		"preferred_resolution": "1080p", "max_resolution": null, "max_gb_per_hour": 8,
		"prefer_x265": false, "prefer_hdr": false, "prefer_dolby_vision": false,
		"avoid_terms": ["CAM", "HDCAM", "TS", "HDTS", "TELESYNC"], "preferred_groups": []
	}`)
}

func TestSettingsRepository_QualityProfile(t *testing.T) {
	db := openTestDB(t)
	repo := NewSettingsRepository(db)
	ctx := context.Background()

	// The profile has one fixed key: leave the database as it was.
	before, hadProfile, err := repo.Get(ctx, QualityProfileKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadProfile {
			_ = repo.Set(ctx, QualityProfileKey, before)
		} else {
			_ = repo.Delete(ctx, QualityProfileKey)
		}
	})

	t.Run("defaults when unset", func(t *testing.T) {
		if err := repo.Delete(ctx, QualityProfileKey); err != nil {
			t.Fatal(err)
		}
		got, err := repo.QualityProfile(ctx)
		if err != nil {
			t.Fatalf("QualityProfile: %v", err)
		}
		if !reflect.DeepEqual(got, DefaultQualityProfile()) {
			t.Errorf("QualityProfile = %+v, want the defaults", got)
		}
	})

	t.Run("round trip, normalised", func(t *testing.T) {
		in := QualityProfile{
			PreferredResolution: "2160p", MaxResolution: strPtr("2160p"), MaxGBPerHour: 12.5,
			PreferX265: true, PreferHDR: true, PreferDolbyVision: true,
			AvoidTerms:      []string{"CAM", " HC ", "hc", "KORSUB"},
			PreferredGroups: []string{"FLUX", "NTb", "ntb"},
		}
		saved, err := repo.SetQualityProfile(ctx, in)
		if err != nil {
			t.Fatalf("SetQualityProfile: %v", err)
		}
		want := in
		want.AvoidTerms = []string{"CAM", "HC", "KORSUB"}
		want.PreferredGroups = []string{"FLUX", "NTb"}
		if !reflect.DeepEqual(saved, want) {
			t.Errorf("saved = %+v, want %+v", saved, want)
		}
		got, err := repo.QualityProfile(ctx)
		if err != nil {
			t.Fatalf("QualityProfile: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("read back = %+v, want %+v", got, want)
		}
	})

	t.Run("null limit and empty lists", func(t *testing.T) {
		in := DefaultQualityProfile()
		in.AvoidTerms = []string{}
		if _, err := repo.SetQualityProfile(ctx, in); err != nil {
			t.Fatalf("SetQualityProfile: %v", err)
		}
		got, err := repo.QualityProfile(ctx)
		if err != nil {
			t.Fatalf("QualityProfile: %v", err)
		}
		if got.MaxResolution != nil || got.AvoidTerms == nil || len(got.AvoidTerms) != 0 {
			t.Errorf("read back = %+v, want no limit and no avoid terms", got)
		}
	})

	t.Run("missing fields take their defaults", func(t *testing.T) {
		if err := repo.Set(ctx, QualityProfileKey, json.RawMessage(`{"preferred_resolution": "720p", "preferred_groups": null}`)); err != nil {
			t.Fatal(err)
		}
		got, err := repo.QualityProfile(ctx)
		if err != nil {
			t.Fatalf("QualityProfile: %v", err)
		}
		want := DefaultQualityProfile()
		want.PreferredResolution = "720p"
		if !reflect.DeepEqual(got, want) {
			t.Errorf("QualityProfile = %+v, want %+v", got, want)
		}
	})

	t.Run("invalid profile isn't stored", func(t *testing.T) {
		if _, err := repo.SetQualityProfile(ctx, DefaultQualityProfile()); err != nil {
			t.Fatal(err)
		}
		bad := DefaultQualityProfile()
		bad.MaxGBPerHour = 25
		if _, err := repo.SetQualityProfile(ctx, bad); !errors.Is(err, ErrInvalidQualityProfile) {
			t.Fatalf("SetQualityProfile = %v, want ErrInvalidQualityProfile", err)
		}
		if got, _ := repo.QualityProfile(ctx); got.MaxGBPerHour != 8 {
			t.Errorf("max_gb_per_hour = %v after a rejected save, want 8", got.MaxGBPerHour)
		}
	})

	t.Run("a document that isn't a profile", func(t *testing.T) {
		if err := repo.Set(ctx, QualityProfileKey, json.RawMessage(`{"max_gb_per_hour": "lots"}`)); err != nil {
			t.Fatal(err)
		}
		got, err := repo.QualityProfile(ctx)
		if err == nil {
			t.Error("QualityProfile: want a decode error")
		}
		if !reflect.DeepEqual(got, DefaultQualityProfile()) {
			t.Errorf("QualityProfile = %+v, want the defaults", got)
		}
	})
}
