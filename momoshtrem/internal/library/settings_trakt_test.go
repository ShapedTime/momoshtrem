package library

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

// The settings' JSON is the contract's TraktSettings, and a patch sends
// only the fields it sets.
func TestTraktSettings_JSON(t *testing.T) {
	raw, err := json.Marshal(DefaultTraktSettings())
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, raw, `{"use_ratings": true, "show_watchlist": true, "include_paused": false}`)

	raw, err = json.Marshal(TraktSettingsPatch{ShowWatchlist: boolPtr(false)})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, raw, `{"show_watchlist": false}`)
}

func TestSettingsRepository_TraktSettings(t *testing.T) {
	db := openTestDB(t)
	repo := NewSettingsRepository(db)
	ctx := context.Background()

	// The settings have one fixed key: leave the database as it was.
	before, had, err := repo.Get(ctx, TraktSettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = repo.Set(ctx, TraktSettingsKey, before)
		} else {
			_ = repo.Delete(ctx, TraktSettingsKey)
		}
	})
	reset := func(t *testing.T) {
		t.Helper()
		if err := repo.Delete(ctx, TraktSettingsKey); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T) TraktSettings {
		t.Helper()
		got, err := repo.TraktSettings(ctx)
		if err != nil {
			t.Fatalf("TraktSettings: %v", err)
		}
		return got
	}

	t.Run("defaults when unset", func(t *testing.T) {
		reset(t)
		if got := read(t); got != DefaultTraktSettings() {
			t.Errorf("TraktSettings = %+v, want the defaults", got)
		}
	})

	t.Run("a patch changes only its fields", func(t *testing.T) {
		reset(t)
		got, err := repo.UpdateTraktSettings(ctx, TraktSettingsPatch{IncludePaused: boolPtr(true)})
		if err != nil {
			t.Fatalf("UpdateTraktSettings: %v", err)
		}
		want := TraktSettings{UseRatings: true, ShowWatchlist: true, IncludePaused: true}
		if got != want {
			t.Errorf("answer = %+v, want %+v", got, want)
		}
		if _, err := repo.UpdateTraktSettings(ctx, TraktSettingsPatch{UseRatings: boolPtr(false), ShowWatchlist: boolPtr(false)}); err != nil {
			t.Fatal(err)
		}
		want = TraktSettings{IncludePaused: true}
		if got := read(t); got != want {
			t.Errorf("read back = %+v, want %+v", got, want)
		}
	})

	t.Run("an empty patch answers the settings", func(t *testing.T) {
		reset(t)
		got, err := repo.UpdateTraktSettings(ctx, TraktSettingsPatch{})
		if err != nil || got != DefaultTraktSettings() {
			t.Errorf("UpdateTraktSettings({}) = %+v, %v; want the defaults", got, err)
		}
	})

	t.Run("missing fields take their defaults", func(t *testing.T) {
		if err := repo.Set(ctx, TraktSettingsKey, json.RawMessage(`{"show_watchlist": false}`)); err != nil {
			t.Fatal(err)
		}
		if got, want := read(t), (TraktSettings{UseRatings: true}); got != want {
			t.Errorf("TraktSettings = %+v, want %+v", got, want)
		}
	})

	t.Run("a stored document that isn't an object is replaced by a patch", func(t *testing.T) {
		if err := repo.Set(ctx, TraktSettingsKey, json.RawMessage(`[1, 2]`)); err != nil {
			t.Fatal(err)
		}
		if got, err := repo.TraktSettings(ctx); err == nil || got != DefaultTraktSettings() {
			t.Errorf("TraktSettings = %+v, %v; want the defaults and a decode error", got, err)
		}
		got, err := repo.UpdateTraktSettings(ctx, TraktSettingsPatch{UseRatings: boolPtr(false)})
		if want := (TraktSettings{ShowWatchlist: true}); err != nil || got != want {
			t.Errorf("UpdateTraktSettings = %+v, %v; want %+v", got, err, want)
		}
	})

	t.Run("concurrent patches of different fields both land", func(t *testing.T) {
		reset(t)
		var wg sync.WaitGroup
		for _, patch := range []TraktSettingsPatch{
			{UseRatings: boolPtr(false)}, {ShowWatchlist: boolPtr(false)}, {IncludePaused: boolPtr(true)},
		} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := repo.UpdateTraktSettings(ctx, patch); err != nil {
					t.Errorf("UpdateTraktSettings: %v", err)
				}
			}()
		}
		wg.Wait()
		if got, want := read(t), (TraktSettings{IncludePaused: true}); got != want {
			t.Errorf("TraktSettings = %+v, want %+v", got, want)
		}
	})
}
