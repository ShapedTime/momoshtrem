package storagecap

import (
	"errors"
	"reflect"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

func TestParsePatch(t *testing.T) {
	t.Run("every field", func(t *testing.T) {
		got, err := ParsePatch([]byte(`{
			"limit_bytes": 107374182400,
			"kept": {"max_bytes": null, "default_until_watched": false, "default_days": 90},
			"smart": {"enabled": true, "episodes_ahead": 5, "max_bytes": 1073741824},
			"background": {"enabled": false, "pause_while_streaming": false,
				"windows": [{"start": "01:00", "end": "08:00"}, {"start": "23:00", "end": "06:00"}]}}`))
		if err != nil {
			t.Fatal(err)
		}
		want := library.StorageSettingsPatch{
			SetLimit: true, LimitBytes: ptr(gib(100)),
			Kept:  library.KeptSettingsPatch{SetMaxBytes: true, DefaultUntilWatched: ptr(false), SetDefaultDays: true, DefaultDays: ptr(90)},
			Smart: library.SmartSettingsPatch{Enabled: ptr(true), EpisodesAhead: ptr(5), SetMaxBytes: true, MaxBytes: ptr(GiB)},
			Background: library.BackgroundSettingsPatch{Enabled: ptr(false), PauseWhileStreaming: ptr(false), SetWindows: true,
				Windows: []library.DownloadWindow{{Start: "01:00", End: "08:00"}, {Start: "23:00", End: "06:00"}}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParsePatch = %+v\nwant %+v", got, want)
		}
	})

	t.Run("only what's sent; fields PUT doesn't take are ignored", func(t *testing.T) {
		got, err := ParsePatch([]byte(`{"source": "auto", "effective_bytes": 1, "disk": {"total_bytes": 1},
			"kept": {"effective_max_bytes": 5}, "smart": {}, "background": {"windows": []}, "unknown": true}`))
		if err != nil {
			t.Fatal(err)
		}
		want := library.StorageSettingsPatch{Background: library.BackgroundSettingsPatch{SetWindows: true, Windows: []library.DownloadWindow{}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParsePatch = %+v, want %+v", got, want)
		}
	})

	t.Run("nulls", func(t *testing.T) {
		got, err := ParsePatch([]byte(`{"limit_bytes": null, "kept": {"default_days": null}, "smart": {"max_bytes": null}}`))
		if err != nil {
			t.Fatal(err)
		}
		want := library.StorageSettingsPatch{SetLimit: true, Kept: library.KeptSettingsPatch{SetDefaultDays: true},
			Smart: library.SmartSettingsPatch{SetMaxBytes: true}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParsePatch = %+v, want %+v", got, want)
		}
	})

	for _, body := range []string{`[]`, `"storage"`, `null`, `3`, `{`, ``} {
		if _, err := ParsePatch([]byte(body)); !errors.Is(err, ErrNotAnObject) {
			t.Errorf("ParsePatch(%q) = %v, want ErrNotAnObject", body, err)
		}
	}
}

// Each bad value is a 422 naming its field.
func TestParsePatch_FieldErrors(t *testing.T) {
	tests := []struct {
		body, field string
	}{
		{`{"limit_bytes": "100"}`, "limit_bytes"},
		{`{"limit_bytes": -1}`, "limit_bytes"},
		{`{"limit_bytes": 1.5}`, "limit_bytes"},
		{`{"limit_bytes": true}`, "limit_bytes"},
		{`{"kept": 5}`, "kept"},
		{`{"kept": null}`, "kept"},
		{`{"kept": {"max_bytes": -5}}`, "kept.max_bytes"},
		{`{"kept": {"max_bytes": "lots"}}`, "kept.max_bytes"},
		{`{"kept": {"default_until_watched": null}}`, "kept.default_until_watched"},
		{`{"kept": {"default_until_watched": "yes"}}`, "kept.default_until_watched"},
		{`{"kept": {"default_days": 14}}`, "kept.default_days"},
		{`{"kept": {"default_days": "30"}}`, "kept.default_days"},
		{`{"smart": []}`, "smart"},
		{`{"smart": {"enabled": 1}}`, "smart.enabled"},
		{`{"smart": {"episodes_ahead": 0}}`, "smart.episodes_ahead"},
		{`{"smart": {"episodes_ahead": 6}}`, "smart.episodes_ahead"},
		{`{"smart": {"episodes_ahead": 2.5}}`, "smart.episodes_ahead"},
		{`{"smart": {"episodes_ahead": null}}`, "smart.episodes_ahead"},
		{`{"smart": {"max_bytes": -1}}`, "smart.max_bytes"},
		{`{"background": "on"}`, "background"},
		{`{"background": {"enabled": null}}`, "background.enabled"},
		{`{"background": {"pause_while_streaming": "no"}}`, "background.pause_while_streaming"},
		{`{"background": {"windows": null}}`, "background.windows"},
		{`{"background": {"windows": {}}}`, "background.windows"},
		{`{"background": {"windows": [{"start": "25:00", "end": "01:00"}]}}`, "background.windows"},
		{`{"background": {"windows": [{"start": "1:00", "end": "02:00"}]}}`, "background.windows"},
		{`{"background": {"windows": [{"start": "01:00"}]}}`, "background.windows"},
		{`{"background": {"windows": [{"start": "01:00", "end": "01:00"}]}}`, "background.windows"},
		{`{"background": {"windows": [{"start": "01:00", "end": "02:00"}, {"start": "03:00", "end": "04:00"},
			{"start": "05:00", "end": "06:00"}, {"start": "07:00", "end": "08:00"}, {"start": "09:00", "end": "10:00"}]}}`, "background.windows"},
		// The first bad field, in order: limit, kept, smart, background.
		{`{"background": {"enabled": 0}, "smart": {"enabled": 0}, "limit_bytes": "x"}`, "limit_bytes"},
		{`{"background": {"enabled": 0}, "smart": {"enabled": 0}}`, "smart.enabled"},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			_, err := ParsePatch([]byte(tt.body))
			var field *FieldError
			if !errors.As(err, &field) || field.Field != tt.field || field.Message == "" {
				t.Errorf("ParsePatch = %v, want a FieldError on %s", err, tt.field)
			}
		})
	}
}

// The checks that depend on the disk and the limit, on the VM's disk
// (limit 150 GiB auto, max 180, reserve 20).
func TestCheckLimits(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		field string // "" is valid
	}{
		{"a limit in range", `{"limit_bytes": 107374182400}`, ""},
		{"the minimum", `{"limit_bytes": 4294967296}`, ""},
		{"below 4 GiB", `{"limit_bytes": 4294967295}`, "limit_bytes"},
		{"the maximum", `{"limit_bytes": 193273528320}`, ""},
		{"above 90% of the disk", `{"limit_bytes": 193273528321}`, "limit_bytes"},
		{"null limit", `{"limit_bytes": null}`, ""},
		{"kept up to the limit less the reserve", `{"kept": {"max_bytes": 139586437120}}`, ""},
		{"kept over it", `{"kept": {"max_bytes": 139586437121}}`, "kept.max_bytes"},
		{"kept checked against the new limit", `{"limit_bytes": 53687091200, "kept": {"max_bytes": 53687091200}}`, "kept.max_bytes"},
		{"smart up to what kept leaves", `{"smart": {"max_bytes": 59055800320}}`, ""},
		{"smart over it", `{"smart": {"max_bytes": 59055800321}}`, "smart.max_bytes"},
		{"smart checked against the new kept cap", `{"kept": {"max_bytes": 128849018880}, "smart": {"max_bytes": 10737418241}}`, "smart.max_bytes"},
		{"a kept cap that leaves a default smart less shrinks smart: not an error", `{"kept": {"max_bytes": 128849018880}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkPatch(t, vm.Settings, tt.patch); !fieldIs(got, tt.field) {
				t.Errorf("checkLimits = %v, want %q", got, tt.field)
			}
		})
	}
}

// Caps stored by an earlier PUT count as much as the ones sent: the answer
// doesn't depend on the order of PUTs. On the VM's disk the room is 130 GiB.
func TestCheckLimits_StoredCaps(t *testing.T) {
	stored := func(kept, smart *int64) library.StorageSettings {
		s := library.DefaultStorageSettings()
		s.Kept.MaxBytes, s.Smart.MaxBytes = kept, smart
		return s
	}
	smart30, kept120 := stored(nil, ptr(gib(30))), stored(ptr(gib(120)), nil)
	tests := []struct {
		name   string
		stored library.StorageSettings
		patch  string
		field  string
	}{
		{"smart 30 stored, then kept 120: kept squeezes smart", smart30, `{"kept": {"max_bytes": 128849018880}}`, "kept.max_bytes"},
		{"kept 120 stored, then smart 30: the same answer", kept120, `{"smart": {"max_bytes": 32212254720}}`, "smart.max_bytes"},
		{"both at once", library.DefaultStorageSettings(), `{"kept": {"max_bytes": 128849018880}, "smart": {"max_bytes": 32212254720}}`, "smart.max_bytes"},
		{"kept 120 stored, then smart 10: fits", kept120, `{"smart": {"max_bytes": 10737418240}}`, ""},
		{"smart 30 stored, kept back to the default: fits", smart30, `{"kept": {"max_bytes": null}}`, ""},
		{"kept 120 stored, a lower limit squeezes it", kept120, `{"limit_bytes": 107374182400}`, "limit_bytes"},
		{"smart 30 stored, a lower limit squeezes it", smart30, `{"limit_bytes": 53687091200}`, "limit_bytes"},
		{"the limit and the cap together", kept120, `{"limit_bytes": 107374182400, "kept": {"max_bytes": 53687091200}}`, ""},
		{"a PUT that sends no limit or cap isn't checked", stored(ptr(gib(500)), nil), `{"smart": {"enabled": true}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkPatch(t, tt.stored, tt.patch)
			if !fieldIs(got, tt.field) {
				t.Errorf("checkLimits = %v, want %q", got, tt.field)
			}
			if got != nil && got.Message == "" {
				t.Error("no message")
			}
		})
	}
}

// checkPatch runs a PUT body's limit checks over stored, on the VM's disk.
func checkPatch(t *testing.T, stored library.StorageSettings, body string) *FieldError {
	t.Helper()
	p, err := ParsePatch([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	in := vm
	in.Settings = p.Apply(stored)
	return checkLimits(p, in.Settings, Resolve(in))
}

// fieldIs: no error for "", else an error on field.
func fieldIs(err *FieldError, field string) bool {
	if field == "" {
		return err == nil
	}
	return err != nil && err.Field == field
}

func TestFormatGB(t *testing.T) {
	for n, want := range map[int64]string{MinBytes: "4 GB", gib(2.5): "2.5 GB", gib(147.6): "148 GB", 0: "0 GB"} {
		if got := formatGB(n); got != want {
			t.Errorf("formatGB(%d) = %q, want %q", n, got, want)
		}
	}
}
