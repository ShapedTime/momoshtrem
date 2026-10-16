package library

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func int64Ptr(n int64) *int64 { return &n }

// A patch sends only what it sets: explicit nulls included, nested objects
// only when they set something.
func TestStorageSettingsPatch_JSON(t *testing.T) {
	for _, tt := range []struct {
		name  string
		patch StorageSettingsPatch
		want  string
	}{
		{"empty", StorageSettingsPatch{}, `{}`},
		{"auto", StorageSettingsPatch{SetLimit: true}, `{"limit_bytes": null}`},
		{"a limit", StorageSettingsPatch{SetLimit: true, LimitBytes: int64Ptr(5 << 30)}, `{"limit_bytes": 5368709120}`},
		{"a value without its Set isn't sent", StorageSettingsPatch{LimitBytes: int64Ptr(1), Kept: KeptSettingsPatch{MaxBytes: int64Ptr(1)}}, `{}`},
		{"one nested field", StorageSettingsPatch{Kept: KeptSettingsPatch{SetDefaultDays: true}}, `{"kept": {"default_days": null}}`},
		{"every nested object", StorageSettingsPatch{
			Kept:       KeptSettingsPatch{SetMaxBytes: true, MaxBytes: int64Ptr(10), DefaultUntilWatched: boolPtr(false)},
			Smart:      SmartSettingsPatch{Enabled: boolPtr(true), EpisodesAhead: intPtr(3), SetMaxBytes: true},
			Background: BackgroundSettingsPatch{PauseWhileStreaming: boolPtr(false), SetWindows: true},
		}, `{"kept": {"max_bytes": 10, "default_until_watched": false},
			"smart": {"enabled": true, "episodes_ahead": 3, "max_bytes": null},
			"background": {"pause_while_streaming": false, "windows": []}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.patch)
			if err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, raw, tt.want)
		})
	}
}

func TestStorageSettings_Defaults(t *testing.T) {
	raw, err := json.Marshal(DefaultStorageSettings())
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, raw, `{"limit_bytes": null,
		"kept": {"max_bytes": null, "default_until_watched": true, "default_days": 30},
		"smart": {"enabled": false, "episodes_ahead": 2, "max_bytes": null},
		"background": {"enabled": true, "pause_while_streaming": true, "windows": []}}`)
}

// Apply changes what the patch sets, copying it: the patch's pointers
// never alias the settings'.
func TestStorageSettingsPatch_Apply(t *testing.T) {
	limit, days := int64(9<<30), 7
	p := StorageSettingsPatch{
		SetLimit: true, LimitBytes: &limit,
		Kept:       KeptSettingsPatch{SetDefaultDays: true, DefaultDays: &days},
		Smart:      SmartSettingsPatch{EpisodesAhead: intPtr(4)},
		Background: BackgroundSettingsPatch{SetWindows: true, Windows: []DownloadWindow{{"23:00", "06:00"}}},
	}
	got := p.Apply(DefaultStorageSettings())
	want := DefaultStorageSettings()
	want.LimitBytes, want.LimitSet = int64Ptr(9<<30), true
	want.Kept.DefaultDays = intPtr(7)
	want.Smart.EpisodesAhead = 4
	want.Background.Windows = []DownloadWindow{{"23:00", "06:00"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Apply = %+v, want %+v", got, want)
	}
	limit, days = 1, 90
	if *got.LimitBytes != 9<<30 || *got.Kept.DefaultDays != 7 {
		t.Error("Apply aliased the patch's values")
	}
	// Null limit: auto, still set.
	if got := (StorageSettingsPatch{SetLimit: true}).Apply(want); got.LimitBytes != nil || !got.LimitSet {
		t.Errorf("null limit = %+v", got)
	}
	// An empty patch changes nothing.
	if got := (StorageSettingsPatch{}).Apply(want); !reflect.DeepEqual(got, want) {
		t.Errorf("empty patch = %+v", got)
	}
}

func TestClockMinutes(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"00:00", 0, true}, {"01:30", 90, true}, {"23:59", 1439, true},
		{"24:00", 0, false}, {"12:60", 0, false}, {"1:30", 0, false}, {"01:3", 0, false},
		{"0a:00", 0, false}, {"01-30", 0, false}, {"", 0, false}, {" 1:30", 0, false},
	} {
		got, ok := ClockMinutes(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ClockMinutes(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	if (DownloadWindow{"01:00", "01:00"}).Valid() || !(DownloadWindow{"23:00", "06:00"}).Valid() {
		t.Error("Valid: start must differ from end; crossing midnight is fine")
	}
}

// Reading tolerates a document PUT never writes: bad values take their
// defaults.
func TestDecodeStorageSettings_Normalizes(t *testing.T) {
	got, err := decodeStorageSettings([]byte(`{"limit_bytes": -1, "kept": {"max_bytes": -5, "default_days": 12},
		"smart": {"episodes_ahead": 9, "max_bytes": 7},
		"background": {"windows": [{"start":"01:00","end":"08:00"}, {"start":"bad","end":"08:00"}, {"start":"02:00","end":"02:00"},
			{"start":"09:00","end":"10:00"}, {"start":"11:00","end":"12:00"}, {"start":"13:00","end":"14:00"}, {"start":"15:00","end":"16:00"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultStorageSettings()
	want.LimitSet = true
	want.Smart.MaxBytes = int64Ptr(7)
	want.Background.Windows = []DownloadWindow{{"01:00", "08:00"}, {"09:00", "10:00"}, {"11:00", "12:00"}, {"13:00", "14:00"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decode = %+v, want %+v", got, want)
	}
	// windows: null reads as none; a null default_days is no maximum.
	got, err = decodeStorageSettings([]byte(`{"kept": {"default_days": null}, "background": {"windows": null}}`))
	if err != nil || got.Kept.DefaultDays != nil || got.Background.Windows == nil || len(got.Background.Windows) != 0 || got.LimitSet {
		t.Errorf("nulls = %+v, %v", got, err)
	}
	if _, err := decodeStorageSettings([]byte(`"storage"`)); err == nil {
		t.Error("a document that isn't an object decoded")
	}
}

func TestSettingsRepository_StorageSettings(t *testing.T) {
	db := openTestDB(t)
	repo := NewSettingsRepository(db)
	ctx := context.Background()

	// The settings have one fixed key: leave the database as it was.
	before, had, err := repo.Get(ctx, StorageSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = repo.Set(ctx, StorageSettingsKey, before)
		} else {
			_ = repo.Delete(ctx, StorageSettingsKey)
		}
	})
	reset := func(t *testing.T) {
		t.Helper()
		if err := repo.Delete(ctx, StorageSettingsKey); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T) StorageSettings {
		t.Helper()
		got, err := repo.StorageSettings(ctx)
		if err != nil {
			t.Fatalf("StorageSettings: %v", err)
		}
		return got
	}
	update := func(t *testing.T, p StorageSettingsPatch) StorageSettings {
		t.Helper()
		got, err := repo.UpdateStorageSettings(ctx, p)
		if err != nil {
			t.Fatalf("UpdateStorageSettings: %v", err)
		}
		return got
	}

	t.Run("a fresh install has the defaults, the limit unset", func(t *testing.T) {
		reset(t)
		if got := read(t); !reflect.DeepEqual(got, DefaultStorageSettings()) || got.LimitSet {
			t.Errorf("StorageSettings = %+v, want the defaults", got)
		}
	})

	t.Run("the first update inserts; nested fields merge over the defaults", func(t *testing.T) {
		reset(t)
		got := update(t, StorageSettingsPatch{Kept: KeptSettingsPatch{SetDefaultDays: true, DefaultDays: intPtr(90)}})
		want := DefaultStorageSettings()
		want.Kept.DefaultDays = intPtr(90)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("first update = %+v, want %+v", got, want)
		}
		if got.LimitSet {
			t.Error("a kept-only update set the limit")
		}
	})

	t.Run("nested objects merge: a field of one keeps its siblings", func(t *testing.T) {
		reset(t)
		update(t, StorageSettingsPatch{
			Kept:       KeptSettingsPatch{SetMaxBytes: true, MaxBytes: int64Ptr(50 << 30), DefaultUntilWatched: boolPtr(false)},
			Smart:      SmartSettingsPatch{Enabled: boolPtr(true), EpisodesAhead: intPtr(4)},
			Background: BackgroundSettingsPatch{SetWindows: true, Windows: []DownloadWindow{{"01:00", "08:00"}, {"23:00", "00:30"}}},
		})
		got := update(t, StorageSettingsPatch{
			Kept:       KeptSettingsPatch{SetDefaultDays: true}, // null: no maximum
			Smart:      SmartSettingsPatch{SetMaxBytes: true, MaxBytes: int64Ptr(9 << 30)},
			Background: BackgroundSettingsPatch{Enabled: boolPtr(false)},
		})
		want := DefaultStorageSettings()
		want.Kept = KeptSettings{MaxBytes: int64Ptr(50 << 30), DefaultUntilWatched: false, DefaultDays: nil}
		want.Smart = SmartSettings{Enabled: true, EpisodesAhead: 4, MaxBytes: int64Ptr(9 << 30)}
		want.Background = BackgroundSettings{Enabled: false, PauseWhileStreaming: true, Windows: []DownloadWindow{{"01:00", "08:00"}, {"23:00", "00:30"}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("after the second update = %+v\nwant %+v", got, want)
		}
		if again := read(t); !reflect.DeepEqual(again, got) {
			t.Errorf("read back = %+v, want %+v", again, got)
		}
		// Windows are replaced as a whole.
		got = update(t, StorageSettingsPatch{Background: BackgroundSettingsPatch{SetWindows: true}})
		if len(got.Background.Windows) != 0 || got.Background.Enabled {
			t.Errorf("windows cleared = %+v", got.Background)
		}
	})

	t.Run("the limit: set, then null (auto, still set), kept by other updates", func(t *testing.T) {
		reset(t)
		got := update(t, StorageSettingsPatch{SetLimit: true, LimitBytes: int64Ptr(100 << 30)})
		if got.LimitBytes == nil || *got.LimitBytes != 100<<30 || !got.LimitSet {
			t.Fatalf("limit = %+v", got)
		}
		got = update(t, StorageSettingsPatch{Smart: SmartSettingsPatch{Enabled: boolPtr(true)}})
		if got.LimitBytes == nil || *got.LimitBytes != 100<<30 || !got.LimitSet || !got.Smart.Enabled {
			t.Fatalf("after a smart update = %+v", got)
		}
		got = update(t, StorageSettingsPatch{SetLimit: true})
		if got.LimitBytes != nil || !got.LimitSet {
			t.Errorf("null limit = %+v, want auto and set", got)
		}
	})

	t.Run("a stored document that isn't an object is replaced by the patch", func(t *testing.T) {
		reset(t)
		if err := repo.Set(ctx, StorageSettingsKey, json.RawMessage(`[1, 2]`)); err != nil {
			t.Fatal(err)
		}
		got := update(t, StorageSettingsPatch{Smart: SmartSettingsPatch{EpisodesAhead: intPtr(5)}})
		want := DefaultStorageSettings()
		want.Smart.EpisodesAhead = 5
		if !reflect.DeepEqual(got, want) {
			t.Errorf("over a non-object = %+v", got)
		}
		// A nested value that isn't an object is replaced too.
		if err := repo.Set(ctx, StorageSettingsKey, json.RawMessage(`{"kept": 3}`)); err != nil {
			t.Fatal(err)
		}
		got = update(t, StorageSettingsPatch{Kept: KeptSettingsPatch{DefaultUntilWatched: boolPtr(false)}})
		if got.Kept.DefaultUntilWatched || got.Kept.DefaultDays == nil || *got.Kept.DefaultDays != 30 {
			t.Errorf("over a non-object kept = %+v", got.Kept)
		}
	})

	t.Run("concurrent updates of different nested fields both land", func(t *testing.T) {
		reset(t)
		update(t, StorageSettingsPatch{Smart: SmartSettingsPatch{EpisodesAhead: intPtr(1)}})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = repo.UpdateStorageSettings(ctx, StorageSettingsPatch{Smart: SmartSettingsPatch{Enabled: boolPtr(true)}})
		}()
		go func() {
			defer wg.Done()
			_, _ = repo.UpdateStorageSettings(ctx, StorageSettingsPatch{Smart: SmartSettingsPatch{SetMaxBytes: true, MaxBytes: int64Ptr(3)}})
		}()
		wg.Wait()
		got := read(t)
		if !got.Smart.Enabled || got.Smart.MaxBytes == nil || *got.Smart.MaxBytes != 3 || got.Smart.EpisodesAhead != 1 {
			t.Errorf("after concurrent updates = %+v", got.Smart)
		}
	})
}
