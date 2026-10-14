package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

// openTestDB connects to MOMOSHTREM_TEST_DATABASE_URL and runs migrations.
// Use a throwaway database: tests write to it.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrations_AppSettingsAppliedAndIdempotent(t *testing.T) {
	db := openTestDB(t)

	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version < 2 {
		t.Fatalf("schema version = %d, want >= 2", version)
	}

	// Re-running migrations on an up-to-date database is a no-op.
	if err := db.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestSettingsRepository(t *testing.T) {
	db := openTestDB(t)
	repo := NewSettingsRepository(db)
	ctx := context.Background()

	// Unique keys so reruns against the same database don't collide.
	prefix := fmt.Sprintf("test.%d.", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM app_settings WHERE key LIKE $1`, prefix+"%")
	})

	t.Run("get missing key", func(t *testing.T) {
		value, found, err := repo.Get(ctx, prefix+"missing")
		if err != nil || found || value != nil {
			t.Fatalf("Get missing = (%s, %v, %v), want (nil, false, nil)", value, found, err)
		}
	})

	t.Run("set then get round-trips JSON", func(t *testing.T) {
		key := prefix + "raw"
		if err := repo.Set(ctx, key, json.RawMessage(`{"b": 2, "a": [1, "x"]}`)); err != nil {
			t.Fatalf("Set: %v", err)
		}
		value, found, err := repo.Get(ctx, key)
		if err != nil || !found {
			t.Fatalf("Get = (found %v, err %v)", found, err)
		}
		assertJSONEqual(t, value, `{"a": [1, "x"], "b": 2}`)
	})

	t.Run("set upserts and bumps updated_at", func(t *testing.T) {
		key := prefix + "upsert"
		if err := repo.Set(ctx, key, json.RawMessage(`1`)); err != nil {
			t.Fatalf("first Set: %v", err)
		}
		first := updatedAt(t, db, key)

		time.Sleep(5 * time.Millisecond)
		if err := repo.Set(ctx, key, json.RawMessage(`"two"`)); err != nil {
			t.Fatalf("second Set: %v", err)
		}
		value, _, err := repo.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		assertJSONEqual(t, value, `"two"`)

		if second := updatedAt(t, db, key); !second.After(first) {
			t.Errorf("updated_at not bumped: %s then %s", first, second)
		}

		var rows int
		if err := db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key = $1`, key).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 1 {
			t.Errorf("rows for key = %d, want 1", rows)
		}
	})

	t.Run("typed helpers", func(t *testing.T) {
		type profile struct {
			PreferredResolution string   `json:"preferred_resolution"`
			AvoidTerms          []string `json:"avoid_terms"`
			MaxGBPerHour        float64  `json:"max_gb_per_hour"`
		}
		key := prefix + "typed"
		want := profile{PreferredResolution: "2160p", AvoidTerms: []string{"CAM", "TS"}, MaxGBPerHour: 7.5}

		if err := repo.SetJSON(ctx, key, want); err != nil {
			t.Fatalf("SetJSON: %v", err)
		}
		var got profile
		found, err := repo.GetJSON(ctx, key, &got)
		if err != nil || !found {
			t.Fatalf("GetJSON = (found %v, err %v)", found, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetJSON = %+v, want %+v", got, want)
		}

		untouched := profile{PreferredResolution: "default"}
		found, err = repo.GetJSON(ctx, prefix+"typed-missing", &untouched)
		if err != nil || found || untouched.PreferredResolution != "default" {
			t.Errorf("GetJSON missing = (found %v, err %v, dst %+v)", found, err, untouched)
		}
	})

	t.Run("delete", func(t *testing.T) {
		key := prefix + "delete"
		if err := repo.Set(ctx, key, json.RawMessage(`true`)); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := repo.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, found, err := repo.Get(ctx, key); err != nil || found {
			t.Errorf("after Delete: found %v, err %v", found, err)
		}
		if err := repo.Delete(ctx, key); err != nil {
			t.Errorf("Delete of a missing key should be a no-op, got %v", err)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		tests := []struct {
			name string
			call func() error
		}{
			{"set empty key", func() error { return repo.Set(ctx, "", json.RawMessage(`1`)) }},
			{"set invalid JSON", func() error { return repo.Set(ctx, prefix+"bad", json.RawMessage(`{nope`)) }},
			{"set empty value", func() error { return repo.Set(ctx, prefix+"bad", nil) }},
			{"get empty key", func() error { _, _, err := repo.Get(ctx, ""); return err }},
			{"delete empty key", func() error { return repo.Delete(ctx, "") }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := tt.call(); !errors.Is(err, ErrInvalidSetting) {
					t.Errorf("error = %v, want ErrInvalidSetting", err)
				}
			})
		}
	})
}

func TestDB_SizeBytes(t *testing.T) {
	db := openTestDB(t)
	size, err := db.SizeBytes(context.Background())
	if err != nil {
		t.Fatalf("SizeBytes: %v", err)
	}
	if size <= 0 {
		t.Errorf("SizeBytes = %d, want > 0", size)
	}
}

func updatedAt(t *testing.T, db *DB, key string) time.Time {
	t.Helper()
	var ts time.Time
	if err := db.QueryRow(`SELECT updated_at FROM app_settings WHERE key = $1`, key).Scan(&ts); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	return ts
}

func assertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("decode got %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want %s: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
