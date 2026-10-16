package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// QualityProfileKey is the app_settings key of the quality profile: one
// JSON document shaped like GET/PUT /settings/quality-profile (PLAN §6
// "Search"). Fields it doesn't hold take their defaults.
const QualityProfileKey = "quality_profile"

// Limits of a quality profile (PLAN §6 QualityProfile).
const (
	MinGBPerHour      = 1.0
	MaxGBPerHour      = 20.0
	MaxQualityTerms   = 30 // avoid terms, and preferred groups
	MaxQualityTermLen = 30 // characters, after trimming
)

// ErrInvalidQualityProfile is what every *QualityProfileError matches. The
// API maps it to 422.
var ErrInvalidQualityProfile = errors.New("invalid quality profile")

// QualityProfileError names the field a profile can't take.
type QualityProfileError struct {
	Field   string // the JSON name: "max_gb_per_hour"
	Message string // for the user, naming the field: "max_gb_per_hour must be between 1 and 20"
}

func (e *QualityProfileError) Error() string { return e.Message }

func (e *QualityProfileError) Unwrap() error { return ErrInvalidQualityProfile }

// QualityProfile is what "Best match" means: torrent search hides and
// scores releases with it (internal/quality).
type QualityProfile struct {
	PreferredResolution string `json:"preferred_resolution"` // "720p", "1080p" or "2160p"
	// MaxResolution hides releases above it; nil is no limit. "1080p" or
	// "2160p", at least PreferredResolution.
	MaxResolution *string `json:"max_resolution"`
	// MaxGBPerHour is soft: a release over it gets a warning and a
	// penalty, never hidden. 1–20 in steps of 0.5.
	MaxGBPerHour      float64 `json:"max_gb_per_hour"`
	PreferX265        bool    `json:"prefer_x265"`
	PreferHDR         bool    `json:"prefer_hdr"`
	PreferDolbyVision bool    `json:"prefer_dolby_vision"`
	// AvoidTerms hide a release that has one as a whole word.
	AvoidTerms []string `json:"avoid_terms"`
	// PreferredGroups get a small boost that never beats seeders or the
	// size limit.
	PreferredGroups []string `json:"preferred_groups"`
}

// DefaultQualityProfile is the profile before anyone changes it.
func DefaultQualityProfile() QualityProfile {
	return QualityProfile{
		PreferredResolution: "1080p",
		MaxGBPerHour:        8,
		AvoidTerms:          []string{"CAM", "HDCAM", "TS", "HDTS", "TELESYNC"},
		PreferredGroups:     []string{},
	}
}

// profileResolutions are the resolutions a profile may name, lowest first.
var profileResolutions = []string{"720p", "1080p", "2160p"}

// Validate checks p and returns it normalised: terms and groups trimmed,
// and deduplicated without regard to case (the first spelling stays). An
// invalid field is a *QualityProfileError, the first in field order.
func (p QualityProfile) Validate() (QualityProfile, error) {
	pref := slices.Index(profileResolutions, p.PreferredResolution)
	if pref < 0 {
		return p, fieldError("preferred_resolution", "preferred_resolution must be 720p, 1080p or 2160p")
	}
	if p.MaxResolution != nil {
		limit := slices.Index(profileResolutions, *p.MaxResolution)
		switch {
		case limit < 1: // 720p isn't a limit a profile can set
			return p, fieldError("max_resolution", "max_resolution must be 1080p, 2160p or null")
		case limit < pref:
			return p, fieldError("max_resolution", "max_resolution must be at least preferred_resolution")
		}
	}
	if p.MaxGBPerHour < MinGBPerHour || p.MaxGBPerHour > MaxGBPerHour {
		return p, fieldError("max_gb_per_hour", "max_gb_per_hour must be between 1 and 20")
	}
	if half := p.MaxGBPerHour * 2; half != math.Trunc(half) {
		return p, fieldError("max_gb_per_hour", "max_gb_per_hour must be a multiple of 0.5")
	}

	var err error
	if p.AvoidTerms, err = normaliseTerms("avoid_terms", "terms", p.AvoidTerms); err != nil {
		return p, err
	}
	if p.PreferredGroups, err = normaliseTerms("preferred_groups", "groups", p.PreferredGroups); err != nil {
		return p, err
	}
	return p, nil
}

// normaliseTerms trims and deduplicates a profile's list; never nil.
func normaliseTerms(field, noun string, terms []string) ([]string, error) {
	out := make([]string, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if n := utf8.RuneCountInString(t); n < 1 || n > MaxQualityTermLen {
			return nil, fieldError(field, fmt.Sprintf("%s must be 1–%d characters each", field, MaxQualityTermLen))
		}
		if key := strings.ToLower(t); !seen[key] {
			seen[key] = true
			out = append(out, t)
		}
	}
	if len(out) > MaxQualityTerms {
		return nil, fieldError(field, fmt.Sprintf("%s can have at most %d %s", field, MaxQualityTerms, noun))
	}
	return out, nil
}

func fieldError(field, message string) error {
	return &QualityProfileError{Field: field, Message: message}
}

// QualityProfile reads the quality profile over its defaults: a field the
// stored document doesn't have keeps its default. A document that isn't a
// profile gives the defaults and the error.
func (r *SettingsRepository) QualityProfile(ctx context.Context) (QualityProfile, error) {
	profile := DefaultQualityProfile()
	raw, found, err := r.Get(ctx, QualityProfileKey)
	if err != nil || !found {
		return profile, err
	}
	// Unmarshal only overwrites the fields present in raw.
	if err := json.Unmarshal(raw, &profile); err != nil {
		return DefaultQualityProfile(), fmt.Errorf("failed to decode setting %q: %w", QualityProfileKey, err)
	}
	if profile.AvoidTerms == nil {
		profile.AvoidTerms = []string{}
	}
	if profile.PreferredGroups == nil {
		profile.PreferredGroups = []string{}
	}
	return profile, nil
}

// SetQualityProfile validates p and stores it, answering the saved
// (normalised) profile. An invalid field is a *QualityProfileError.
func (r *SettingsRepository) SetQualityProfile(ctx context.Context, p QualityProfile) (QualityProfile, error) {
	p, err := p.Validate()
	if err != nil {
		return p, err
	}
	if err := r.SetJSON(ctx, QualityProfileKey, p); err != nil {
		return p, err
	}
	return p, nil
}
