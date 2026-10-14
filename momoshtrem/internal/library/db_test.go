package library

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPendingMigrations(t *testing.T) {
	m := func(v int) migration { return migration{Version: v, Name: "x"} }
	all := []migration{m(1), m(2), m(3), m(5)}

	tests := []struct {
		name        string
		all         []migration
		applied     []int
		wantPending []int
		wantUnknown []int
	}{
		{"fresh database", all, nil, []int{1, 2, 3, 5}, nil},
		{"up to date", all, []int{1, 2, 3, 5}, nil, nil},
		{"new migration on top", all, []int{1, 2, 3}, []int{5}, nil},
		{"reserved number landed late", all, []int{1, 2, 5}, []int{3}, nil},
		{"several gaps", all, []int{2, 5}, []int{1, 3}, nil},
		{"applied order doesn't matter", all, []int{5, 1, 3}, []int{2}, nil},
		{"database newer than binary", all, []int{1, 2, 3, 5, 7, 6}, nil, []int{6, 7}},
		{"unknown and pending at once", all, []int{1, 4}, []int{2, 3, 5}, []int{4}},
		{"nothing embedded", nil, []int{1}, nil, []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pending, unknown := pendingMigrations(tt.all, tt.applied)
			var got []int
			for _, p := range pending {
				got = append(got, p.Version)
			}
			if !reflect.DeepEqual(got, tt.wantPending) {
				t.Errorf("pending = %v, want %v", got, tt.wantPending)
			}
			if !reflect.DeepEqual(unknown, tt.wantUnknown) {
				t.Errorf("unknown = %v, want %v", unknown, tt.wantUnknown)
			}
		})
	}
}

func TestEmbeddedMigrations(t *testing.T) {
	t.Run("sorted and filtered", func(t *testing.T) {
		fsys := fstest.MapFS{
			"migrations/010_ten.sql":   {Data: []byte("SELECT 1")},
			"migrations/002_two.sql":   {Data: []byte("SELECT 1")},
			"migrations/001_one.sql":   {Data: []byte("SELECT 1")},
			"migrations/README.md":     {Data: []byte("docs")},
			"migrations/nounder.sql":   {Data: []byte("SELECT 1")},
			"migrations/abc_named.sql": {Data: []byte("SELECT 1")},
		}
		got, err := embeddedMigrations(fsys)
		if err != nil {
			t.Fatalf("embeddedMigrations: %v", err)
		}
		want := []migration{{1, "001_one.sql"}, {2, "002_two.sql"}, {10, "010_ten.sql"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("migrations = %v, want %v", got, want)
		}
	})

	t.Run("duplicate versions", func(t *testing.T) {
		fsys := fstest.MapFS{
			"migrations/004_jobs.sql":  {Data: []byte("SELECT 1")},
			"migrations/004_trakt.sql": {Data: []byte("SELECT 1")},
		}
		if _, err := embeddedMigrations(fsys); err == nil || !strings.Contains(err.Error(), "share version 4") {
			t.Fatalf("error = %v, want a duplicate version error", err)
		}
	})

	t.Run("the real migrations", func(t *testing.T) {
		got, err := embeddedMigrations(migrationsFS)
		if err != nil {
			t.Fatalf("embeddedMigrations: %v", err)
		}
		if len(got) < 3 || got[2].Name != "003_metadata.sql" {
			t.Errorf("migrations = %v, want 003_metadata.sql third", got)
		}
	})
}

func TestMigrate_ReappliesAMissingVersion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Simulate 003 landing after a later migration was applied: its row is
	// missing, but higher versions exist. The runner must still apply it,
	// and 003 must tolerate a schema that already has its columns.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 3`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (999) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM schema_migrations WHERE version = 999`) })

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var applied bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 3)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Error("version 3 was not re-applied below the highest recorded version")
	}
}

func TestMigrate_ConcurrentCallersWait(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { errs <- db.Migrate(ctx) }()
	}
	for i := 0; i < 4; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Migrate: %v", err)
		}
	}
}
