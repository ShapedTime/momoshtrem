package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidStorageSettings is a stored "storage" document that doesn't
// decode (not an object, or a field of the wrong type): a hand edit. Its
// readers go on with the defaults, and ReplaceStorageSettings repairs it.
var ErrInvalidStorageSettings = errors.New("the stored storage settings don't decode")

// StorageSettingsKey is the app_settings key of the storage settings (#77,
// PLAN §6 "Storage (#77)"): the piece cache's limit, and the caps and
// defaults of kept and smart downloads and background downloading. The
// document is shaped like PUT /settings/storage's body; fields it doesn't
// hold take their defaults.
const StorageSettingsKey = "storage"

// The storage settings' defaults and bounds.
const (
	// DefaultKeptDays is a Keep's default maximum: "until watched, at
	// most 30 days".
	DefaultKeptDays = 30
	// DefaultEpisodesAhead is how many next episodes smart downloads
	// fetch, between MinEpisodesAhead and MaxEpisodesAhead.
	DefaultEpisodesAhead = 2
	MinEpisodesAhead     = 1
	MaxEpisodesAhead     = 5
	// MaxDownloadWindows bounds background.windows.
	MaxDownloadWindows = 4
)

// KeptDayChoices are the values kept.default_days takes besides null (no
// maximum).
var KeptDayChoices = []int{7, 30, 90}

// StorageSettings is the "storage" document.
type StorageSettings struct {
	// LimitBytes caps the piece cache; nil is auto (75% of the disk).
	LimitBytes *int64 `json:"limit_bytes"`
	// LimitSet is true once the document holds limit_bytes (a PUT sent
	// it): it then wins over config.yaml's global_cache_size, a null
	// meaning auto. Without it the limit falls through to the config.
	LimitSet   bool               `json:"-"`
	Kept       KeptSettings       `json:"kept"`
	Smart      SmartSettings      `json:"smart"`
	Background BackgroundSettings `json:"background"`
}

// KeptSettings are Download & keep's cap and the defaults a new Keep takes.
type KeptSettings struct {
	// MaxBytes caps kept downloads; nil is 50% of the limit.
	MaxBytes *int64 `json:"max_bytes"`
	// DefaultUntilWatched releases a Keep once it's watched.
	DefaultUntilWatched bool `json:"default_until_watched"`
	// DefaultDays is a Keep's maximum, one of KeptDayChoices; nil is no
	// maximum.
	DefaultDays *int `json:"default_days"`
}

// SmartSettings are smart downloads': the next episodes of shows being
// watched.
type SmartSettings struct {
	// Enabled is off by default until the device check (#202).
	Enabled       bool `json:"enabled"`
	EpisodesAhead int  `json:"episodes_ahead"`
	// MaxBytes caps smart downloads; nil is 15% of the limit.
	MaxBytes *int64 `json:"max_bytes"`
}

// BackgroundSettings are when background downloads run.
type BackgroundSettings struct {
	// Enabled is the kill switch for every background download.
	Enabled bool `json:"enabled"`
	// PauseWhileStreaming pauses background downloads of other torrents
	// while anything streams.
	PauseWhileStreaming bool `json:"pause_while_streaming"`
	// Windows are the times downloads may run, in the server's TZ; empty
	// is any time. Never nil.
	Windows []DownloadWindow `json:"windows"`
}

// DownloadWindow is a daily time range, "HH:MM" to "HH:MM" in the server's
// TZ. End before Start crosses midnight; Start never equals End.
type DownloadWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ClockMinutes reads an "HH:MM" time of day (00:00 to 23:59) as minutes
// after midnight.
func ClockMinutes(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	digit := func(b byte) (int, bool) { return int(b - '0'), b >= '0' && b <= '9' }
	h1, ok1 := digit(s[0])
	h2, ok2 := digit(s[1])
	m1, ok3 := digit(s[3])
	m2, ok4 := digit(s[4])
	h, m := h1*10+h2, m1*10+m2
	if !ok1 || !ok2 || !ok3 || !ok4 || h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// Valid reports whether the window is two "HH:MM" times that differ.
func (w DownloadWindow) Valid() bool {
	start, ok1 := ClockMinutes(w.Start)
	end, ok2 := ClockMinutes(w.End)
	return ok1 && ok2 && start != end
}

// DefaultStorageSettings are the settings before anyone changes them.
func DefaultStorageSettings() StorageSettings {
	days := DefaultKeptDays
	return StorageSettings{
		Kept:       KeptSettings{DefaultUntilWatched: true, DefaultDays: &days},
		Smart:      SmartSettings{EpisodesAhead: DefaultEpisodesAhead},
		Background: BackgroundSettings{Enabled: true, PauseWhileStreaming: true, Windows: []DownloadWindow{}},
	}
}

// StorageSettingsPatch is a partial update: a nil field keeps its value.
// The nullable fields need two levels: Set… says it's sent, and a nil value
// with it is null (auto, the default fraction, or no maximum).
type StorageSettingsPatch struct {
	SetLimit   bool
	LimitBytes *int64
	Kept       KeptSettingsPatch
	Smart      SmartSettingsPatch
	Background BackgroundSettingsPatch
}

// KeptSettingsPatch changes the kept fields it sets.
type KeptSettingsPatch struct {
	SetMaxBytes         bool
	MaxBytes            *int64
	DefaultUntilWatched *bool
	SetDefaultDays      bool
	DefaultDays         *int
}

// SmartSettingsPatch changes the smart fields it sets.
type SmartSettingsPatch struct {
	Enabled       *bool
	EpisodesAhead *int
	SetMaxBytes   bool
	MaxBytes      *int64
}

// BackgroundSettingsPatch changes the background fields it sets; windows
// are replaced as a whole.
type BackgroundSettingsPatch struct {
	Enabled             *bool
	PauseWhileStreaming *bool
	SetWindows          bool
	Windows             []DownloadWindow
}

// fields are the JSON fields a patch sets, the object merged into the
// document.
func (p KeptSettingsPatch) fields() map[string]any {
	set := map[string]any{}
	patchField(set, "max_bytes", p.SetMaxBytes, p.MaxBytes)
	patchField(set, "default_until_watched", p.DefaultUntilWatched != nil, p.DefaultUntilWatched)
	patchField(set, "default_days", p.SetDefaultDays, p.DefaultDays)
	return set
}

func (p SmartSettingsPatch) fields() map[string]any {
	set := map[string]any{}
	patchField(set, "enabled", p.Enabled != nil, p.Enabled)
	patchField(set, "episodes_ahead", p.EpisodesAhead != nil, p.EpisodesAhead)
	patchField(set, "max_bytes", p.SetMaxBytes, p.MaxBytes)
	return set
}

func (p BackgroundSettingsPatch) fields() map[string]any {
	set := map[string]any{}
	patchField(set, "enabled", p.Enabled != nil, p.Enabled)
	patchField(set, "pause_while_streaming", p.PauseWhileStreaming != nil, p.PauseWhileStreaming)
	windows := p.Windows
	if windows == nil {
		windows = []DownloadWindow{}
	}
	patchField(set, "windows", p.SetWindows, windows)
	return set
}

func patchField(set map[string]any, name string, ok bool, v any) {
	if ok {
		set[name] = v
	}
}

// MarshalJSON writes the fields the patch sets, and only those: the object
// UpdateStorageSettings merges into the document, nested objects included
// only when they set something.
func (p StorageSettingsPatch) MarshalJSON() ([]byte, error) {
	set := map[string]any{}
	patchField(set, "limit_bytes", p.SetLimit, p.LimitBytes)
	for name, nested := range map[string]map[string]any{
		"kept": p.Kept.fields(), "smart": p.Smart.fields(), "background": p.Background.fields(),
	} {
		patchField(set, name, len(nested) > 0, nested)
	}
	return json.Marshal(set)
}

// Apply is the settings with the patch's fields changed: what
// UpdateStorageSettings stores, for checking a change before it's made.
func (p StorageSettingsPatch) Apply(s StorageSettings) StorageSettings {
	if p.SetLimit {
		s.LimitBytes, s.LimitSet = cloneInt64(p.LimitBytes), true
	}
	if p.Kept.SetMaxBytes {
		s.Kept.MaxBytes = cloneInt64(p.Kept.MaxBytes)
	}
	if p.Kept.DefaultUntilWatched != nil {
		s.Kept.DefaultUntilWatched = *p.Kept.DefaultUntilWatched
	}
	if p.Kept.SetDefaultDays {
		s.Kept.DefaultDays = nil
		if p.Kept.DefaultDays != nil {
			days := *p.Kept.DefaultDays
			s.Kept.DefaultDays = &days
		}
	}
	if p.Smart.Enabled != nil {
		s.Smart.Enabled = *p.Smart.Enabled
	}
	if p.Smart.EpisodesAhead != nil {
		s.Smart.EpisodesAhead = *p.Smart.EpisodesAhead
	}
	if p.Smart.SetMaxBytes {
		s.Smart.MaxBytes = cloneInt64(p.Smart.MaxBytes)
	}
	if p.Background.Enabled != nil {
		s.Background.Enabled = *p.Background.Enabled
	}
	if p.Background.PauseWhileStreaming != nil {
		s.Background.PauseWhileStreaming = *p.Background.PauseWhileStreaming
	}
	if p.Background.SetWindows {
		s.Background.Windows = append([]DownloadWindow{}, p.Background.Windows...)
	}
	return s
}

func cloneInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// StorageSettings reads the storage settings over their defaults: a field
// the stored document doesn't have keeps its default. A document that isn't
// the settings gives the defaults and an error matching
// ErrInvalidStorageSettings.
func (r *SettingsRepository) StorageSettings(ctx context.Context) (StorageSettings, error) {
	raw, found, err := r.Get(ctx, StorageSettingsKey)
	if err != nil || !found {
		return DefaultStorageSettings(), err
	}
	return decodeStorageSettings(raw)
}

// storageMerge stores a patch: top-level fields replace, and the nested
// objects (kept, smart, background) merge, so a patch of one nested field
// keeps the others. It's one statement, so two updates of different fields
// never lose each other's change.
var storageMerge = `
	INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2::jsonb, NOW())
	ON CONFLICT (key) DO UPDATE SET
		value = (
			SELECT cur || patch` + nestedMerge("kept") + nestedMerge("smart") + nestedMerge("background") + `
			FROM (SELECT
				CASE WHEN jsonb_typeof(app_settings.value) = 'object' THEN app_settings.value ELSE '{}'::jsonb END AS cur,
				EXCLUDED.value AS patch) AS m
		),
		updated_at = NOW()
	RETURNING value`

// nestedMerge is the SQL that merges the patch's object at key into the
// stored one, instead of replacing it.
func nestedMerge(key string) string {
	return fmt.Sprintf(`
				|| CASE WHEN jsonb_typeof(patch->'%[1]s') = 'object' THEN jsonb_build_object('%[1]s',
					CASE WHEN jsonb_typeof(cur->'%[1]s') = 'object' THEN cur->'%[1]s' ELSE '{}'::jsonb END || (patch->'%[1]s'))
					ELSE '{}'::jsonb END`, key)
}

// UpdateStorageSettings stores the fields patch sets and answers every
// setting. Values are the caller's to check (internal/storagecap).
func (r *SettingsRepository) UpdateStorageSettings(ctx context.Context, patch StorageSettingsPatch) (StorageSettings, error) {
	set, err := json.Marshal(patch)
	if err != nil {
		return DefaultStorageSettings(), fmt.Errorf("failed to encode setting %q: %w", StorageSettingsKey, err)
	}
	var raw []byte
	if err := r.db.QueryRowContext(ctx, storageMerge, StorageSettingsKey, string(set)).Scan(&raw); err != nil {
		return DefaultStorageSettings(), fmt.Errorf("failed to update setting %q: %w", StorageSettingsKey, err)
	}
	return decodeStorageSettings(raw)
}

// ReplaceStorageSettings replaces the document with the defaults and the
// fields patch sets, and answers them: how a PUT repairs a document that
// doesn't decode, where merging would keep its bad fields. Values are the
// caller's to check.
func (r *SettingsRepository) ReplaceStorageSettings(ctx context.Context, patch StorageSettingsPatch) (StorageSettings, error) {
	settings := patch.Apply(DefaultStorageSettings())
	doc := map[string]any{"kept": settings.Kept, "smart": settings.Smart, "background": settings.Background}
	if settings.LimitSet {
		doc["limit_bytes"] = settings.LimitBytes
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return DefaultStorageSettings(), fmt.Errorf("failed to encode setting %q: %w", StorageSettingsKey, err)
	}
	if err := r.Set(ctx, StorageSettingsKey, raw); err != nil {
		return DefaultStorageSettings(), err
	}
	return decodeStorageSettings(raw)
}

func decodeStorageSettings(raw []byte) (StorageSettings, error) {
	settings := DefaultStorageSettings()
	// Unmarshal only overwrites the fields present in raw, nested ones
	// included; an explicit null sets a pointer to nil.
	var keys map[string]json.RawMessage
	err := json.Unmarshal(raw, &keys)
	if err == nil {
		err = json.Unmarshal(raw, &settings)
	}
	if err == nil && keys == nil {
		err = errors.New("null")
	}
	if err != nil {
		return DefaultStorageSettings(), fmt.Errorf("%w (setting %q): %w", ErrInvalidStorageSettings, StorageSettingsKey, err)
	}
	_, settings.LimitSet = keys["limit_bytes"]
	settings.normalize()
	return settings, nil
}

// normalize replaces values PUT never stores (a hand-edited document)
// with their defaults, so readers can trust the settings.
func (s *StorageSettings) normalize() {
	def := DefaultStorageSettings()
	for _, v := range []**int64{&s.LimitBytes, &s.Kept.MaxBytes, &s.Smart.MaxBytes} {
		if *v != nil && **v < 0 {
			*v = nil
		}
	}
	if d := s.Kept.DefaultDays; d != nil && !slices.Contains(KeptDayChoices, *d) {
		s.Kept.DefaultDays = def.Kept.DefaultDays
	}
	if n := s.Smart.EpisodesAhead; n < MinEpisodesAhead || n > MaxEpisodesAhead {
		s.Smart.EpisodesAhead = def.Smart.EpisodesAhead
	}
	windows := []DownloadWindow{}
	for _, w := range s.Background.Windows {
		if w.Valid() && len(windows) < MaxDownloadWindows {
			windows = append(windows, w)
		}
	}
	s.Background.Windows = windows
}
