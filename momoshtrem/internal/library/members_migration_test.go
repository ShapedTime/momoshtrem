package library

import (
	"context"
	"strings"
	"testing"
)

// migrateUpTo applies every embedded migration below version to a bare
// schema, recording them as the runner does.
func migrateUpTo(t *testing.T, db *DB, version int) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	all, err := embeddedMigrations(migrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Version >= version {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + m.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			t.Fatalf("%s: %v", m.Name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, m.Version); err != nil {
			t.Fatal(err)
		}
	}
}

// 011 on a database at 010: the first member is the Trakt connection's
// username (trimmed, 32 characters at most), else "Me", in teal, with id
// 1. Running it again adds nobody.
func TestMigration011_SeedsTheFirstMember(t *testing.T) {
	for _, tt := range []struct {
		name     string
		username *string // nil: no Trakt connection
		want     string
	}{
		{"no Trakt connection", nil, "Me"},
		{"the username", strPtr("  Teymur "), "Teymur"},
		{"a blank username", strPtr("   "), "Me"},
		{"a long username", strPtr(strings.Repeat("a", 31) + " b" + strings.Repeat("c", 10)), strings.Repeat("a", 31)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openBareSchemaDB(t, "mig011")
			migrateUpTo(t, db, 11)
			if tt.username != nil {
				if _, err := db.Exec(`INSERT INTO trakt_auth (username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at)
					VALUES ($1, 'teymur', '\x01', '\x02', NOW(), NOW() + interval '7 days')`, *tt.username); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Migrate(context.Background()); err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			data, err := migrationsFS.ReadFile("migrations/011_members.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(data)); err != nil {
				t.Fatalf("011 again: %v", err)
			}

			list, err := NewMemberRepository(db).List(context.Background())
			if err != nil || len(list) != 1 || list[0].ID != 1 || list[0].Name != tt.want || list[0].Color != "teal" {
				t.Fatalf("members = %+v, %v, want only 1 %q in teal", list, err, tt.want)
			}
			// The next member takes the sequence's next id.
			var id int64
			if err := db.QueryRow(`INSERT INTO members (name, color) VALUES ('Sam', 'rose') RETURNING id`).Scan(&id); err != nil || id != 2 {
				t.Errorf("next id = %d (%v), want 2", id, err)
			}
			if _, err := db.Exec(`INSERT INTO members (name, color) VALUES ('SAM', 'lime')`); err == nil || !strings.Contains(err.Error(), "members_name_key") {
				t.Errorf("a name in another case = %v, want members_name_key", err)
			}
			for _, name := range []string{"", strings.Repeat("x", 33)} {
				if _, err := db.Exec(`INSERT INTO members (name, color) VALUES ($1, 'lime')`, name); err == nil || !strings.Contains(err.Error(), "members_name_check") {
					t.Errorf("name %q = %v, want members_name_check", name, err)
				}
			}
		})
	}
}
