package library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// openBareSchemaDB opens the test database in a new private schema, not
// migrated: the caller decides what's applied.
func openBareSchemaDB(t *testing.T, prefix string) *DB {
	t.Helper()
	base := openTestDB(t)
	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	sqlDB, err := sql.Open("pgx", url+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return NewDBFromSQL(sqlDB)
}

// Production is at 009 when 006 lands (numbers are reserved per task): the
// runner applies the missing version on top, and 006 is idempotent. Then
// every table takes a row and the CHECKs and partial unique indexes hold.
func TestMigration006_AppliesOnTopOf009(t *testing.T) {
	db := openBareSchemaDB(t, "mig006")
	ctx := context.Background()

	// A database at 009 without 006, as production has it.
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	all, err := embeddedMigrations(migrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	highest := 0
	for _, m := range all {
		if m.Version == 6 {
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
		highest = max(highest, m.Version)
	}
	if highest < 9 {
		t.Fatalf("highest migration = %d, want at least 009", highest)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var applied bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 6)`).Scan(&applied); err != nil || !applied {
		t.Fatalf("006 applied = %v (%v), want it applied on top of 009", applied, err)
	}
	// Idempotent
	data, err := migrationsFS.ReadFile("migrations/006_recommendations.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(data)); err != nil {
		t.Fatalf("006 again: %v", err)
	}

	must := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%.70s…: %v", query, err)
		}
	}
	must(`INSERT INTO taste_profiles (id, counts, profile) VALUES (1, '{"loved": 12}', '{"genres": []}')`)
	must(`INSERT INTO rec_runs (id, kind, status, model, profile_id, progress) VALUES (1, 'scheduled', 'succeeded', 'claude-opus-5-5', 1, '{"steps": []}')`)
	must(`INSERT INTO rec_runs (id, kind, status, model, skip_reason) VALUES (2, 'first_sync', 'skipped', 'claude-opus-5-5', 'no_history')`)
	must(`INSERT INTO rec_runs (id, kind, status, model) VALUES (3, 'manual', 'queued', 'claude-sonnet-5-5')`)
	must(`INSERT INTO rec_shelves (id, run_id, position, slug, kind, title) VALUES (1, 1, 0, 'because-severance', 'because', 'Because you loved Severance')`)
	must(`INSERT INTO rec_items (shelf_id, position, media_type, tmdb_id, reason, meta) VALUES (1, 0, 'tv', 95396, 'Workplace mystery', '{"title": "Severance"}')`)
	must(`INSERT INTO rec_hero_picks (run_id, position, media_type, tmdb_id, kicker, reason) VALUES (1, 0, 'movie', 693134, 'Tonight', 'Big and quiet')`)
	must(`INSERT INTO rec_feedback (media_type, tmdb_id, signal, source) VALUES ('tv', 1399, 'dislike', 'shelf')`)
	must(`INSERT INTO ask_threads (id, title, model, prompt_version) VALUES (1, 'Something like Severance', 'claude-opus-5-5', 'ask-v1')`)
	must(`INSERT INTO ask_turns (id, thread_id, position, message, status) VALUES (1, 1, 0, 'something like Severance', 'done')`)
	must(`INSERT INTO ask_turns (id, thread_id, position, message, status) VALUES (2, 1, 1, 'lighter', 'streaming')`)
	must(`INSERT INTO ask_messages (thread_id, turn_id, position, role, content) VALUES (1, 1, 0, 'user', '[{"type": "text", "text": "hi"}]')`)

	// Ask's history replays byte for byte: json keeps key order and
	// whitespace, where jsonb would rewrite them
	blocks := `[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}},  {"type":"thinking","thinking":"","signature":"c2ln"}]`
	var replayed string
	if err := db.QueryRow(`INSERT INTO ask_messages (thread_id, turn_id, position, role, content) VALUES (1, 1, 1, 'assistant', $1)
		RETURNING content::text`, blocks).Scan(&replayed); err != nil || replayed != blocks {
		t.Errorf("content = %s (%v), want it byte for byte", replayed, err)
	}

	rejected := []struct{ name, insert, constraint string }{
		{"a second active run", `INSERT INTO rec_runs (id, kind, status, model) VALUES (10, 'manual', 'running', 'x')`, "rec_runs_one_active"},
		{"an unknown run kind", `INSERT INTO rec_runs (kind, status, model) VALUES ('nightly', 'failed', 'x')`, "rec_runs_kind_check"},
		{"an unknown skip reason", `INSERT INTO rec_runs (kind, status, model, skip_reason) VALUES ('manual', 'skipped', 'x', 'tired')`, "rec_runs_skip_reason_check"},
		{"an unknown shelf kind", `INSERT INTO rec_shelves (run_id, position, slug, kind, title) VALUES (1, 1, 'x', 'mixed', 'x')`, "rec_shelves_kind_check"},
		{"a shelf slug twice", `INSERT INTO rec_shelves (run_id, position, slug, kind, title) VALUES (1, 1, 'because-severance', 'other', 'x')`, "rec_shelves_run_id_slug_key"},
		{"an item twice on a shelf", `INSERT INTO rec_items (shelf_id, position, media_type, tmdb_id, reason) VALUES (1, 1, 'tv', 95396, 'x')`, "rec_items_shelf_id_media_type_tmdb_id_key"},
		{"a hero pick twice", `INSERT INTO rec_hero_picks (run_id, position, media_type, tmdb_id, kicker, reason) VALUES (1, 1, 'movie', 693134, 'x', 'x')`, "rec_hero_picks_run_id_media_type_tmdb_id_key"},
		{"an unknown signal", `INSERT INTO rec_feedback (media_type, tmdb_id, signal, source) VALUES ('movie', 1, 'love', 'shelf')`, "rec_feedback_signal_check"},
		{"an unknown source", `INSERT INTO rec_feedback (media_type, tmdb_id, signal, source) VALUES ('movie', 1, 'like', 'email')`, "rec_feedback_source_check"},
		{"feedback twice on a title", `INSERT INTO rec_feedback (media_type, tmdb_id, signal, source) VALUES ('tv', 1399, 'like', 'title')`, "rec_feedback_pkey"},
		{"a second streaming turn", `INSERT INTO ask_turns (id, thread_id, position, message, status) VALUES (10, 1, 2, 'x', 'streaming')`, "ask_turns_one_streaming"},
		{"a system message", `INSERT INTO ask_messages (thread_id, turn_id, position, role, content) VALUES (1, 1, 1, 'system', '[]')`, "ask_messages_role_check"},
	}
	for _, tt := range rejected {
		_, err := db.Exec(tt.insert)
		if err == nil || !strings.Contains(err.Error(), tt.constraint) {
			t.Errorf("%s: %v, want %s", tt.name, err, tt.constraint)
		}
	}

	// A profile a run references loses the reference when it's pruned;
	// deleting a run or a thread takes its rows with it.
	must(`DELETE FROM taste_profiles WHERE id = 1`)
	var profileID sql.NullInt64
	if err := db.QueryRow(`SELECT profile_id FROM rec_runs WHERE id = 1`).Scan(&profileID); err != nil || profileID.Valid {
		t.Errorf("profile_id after its profile went = %v (%v), want NULL", profileID, err)
	}
	must(`DELETE FROM rec_runs WHERE id = 1`)
	must(`DELETE FROM ask_threads WHERE id = 1`)
	for _, table := range []string{"rec_shelves", "rec_items", "rec_hero_picks", "ask_turns", "ask_messages"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s has %d rows (%v) after its parent went, want 0", table, n, err)
		}
	}
}
