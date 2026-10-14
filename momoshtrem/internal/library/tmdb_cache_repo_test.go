package library

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTMDBCacheRepository(t *testing.T) {
	db := openTestDB(t)
	repo := NewTMDBCacheRepository(db)
	ctx := context.Background()

	prefix := fmt.Sprintf("test:%d:", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM tmdb_cache WHERE key LIKE $1`, prefix+"%") })

	if _, _, found, err := repo.Get(ctx, prefix+"missing"); err != nil || found {
		t.Fatalf("Get missing = (found %v, err %v), want not found", found, err)
	}

	key := prefix + "tv/1"
	expires := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	if err := repo.Set(ctx, key, []byte(`{"id": 1, "name": "One"}`), expires); err != nil {
		t.Fatalf("Set: %v", err)
	}
	payload, gotExpires, found, err := repo.Get(ctx, key)
	if err != nil || !found {
		t.Fatalf("Get = (found %v, err %v)", found, err)
	}
	assertJSONEqual(t, payload, `{"name": "One", "id": 1}`)
	if !gotExpires.Equal(expires) {
		t.Errorf("expires = %v, want %v", gotExpires, expires)
	}

	// Overwrite with a payload that never expires.
	if err := repo.Set(ctx, key, []byte(`{"id": 2}`), time.Time{}); err != nil {
		t.Fatalf("Set again: %v", err)
	}
	payload, gotExpires, _, err = repo.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, payload, `{"id": 2}`)
	if !gotExpires.IsZero() {
		t.Errorf("expires = %v, want zero (never)", gotExpires)
	}

	if err := repo.Set(ctx, prefix+"bad", []byte(`{nope`), time.Time{}); err == nil {
		t.Error("Set with invalid JSON: want an error")
	}
}

func TestTMDBCacheRepository_PruneExpired(t *testing.T) {
	db := openTestDB(t)
	repo := NewTMDBCacheRepository(db)
	ctx := context.Background()

	prefix := fmt.Sprintf("test:prune:%d:", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM tmdb_cache WHERE key LIKE $1`, prefix+"%") })

	now := time.Now()
	entries := map[string]time.Time{
		"long-expired":     now.Add(-40 * 24 * time.Hour),
		"recently-expired": now.Add(-2 * 24 * time.Hour), // kept for stale-on-error
		"fresh":            now.Add(time.Hour),
		"never":            {},
	}
	for key, expires := range entries {
		if err := repo.Set(ctx, prefix+key, []byte(`{}`), expires); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repo.PruneExpired(ctx, 30*24*time.Hour); err != nil {
		t.Fatalf("PruneExpired: %v", err)
	}
	for key := range entries {
		_, _, found, err := repo.Get(ctx, prefix+key)
		if err != nil {
			t.Fatal(err)
		}
		if want := key != "long-expired"; found != want {
			t.Errorf("%s: found %v, want %v", key, found, want)
		}
	}
}
