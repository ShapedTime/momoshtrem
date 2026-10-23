package library

import (
	"context"
	"fmt"
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

// 012 on a database at 011, as production has it after #241, run twice:
// every Trakt row is the lowest member's (member 1 is gone here, so 2's),
// the keys lead with the member (the watched tables' with the title),
// trakt_auth is keyed by the member with user_slug unique, and the Trakt
// settings move to the member. member_id has no default, an unknown
// member is refused, and a member's rows go with them.
func TestMigration012_MemberTrakt(t *testing.T) {
	db := openBareSchemaDB(t, "mig012")
	ctx := context.Background()
	migrateUpTo(t, db, 12)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}
	exec(`INSERT INTO members (name, color) VALUES ('Sam', 'rose')`)
	exec(`DELETE FROM members WHERE id = 1`)
	for _, q := range []string{
		`INSERT INTO trakt_auth (username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at)
			VALUES ('Teymur', 'teymur', '\x01', '\x02', NOW(), NOW() + INTERVAL '7 days')`,
		`INSERT INTO trakt_sync_state (key, value) VALUES ('last_sync_at', '"2026-10-16T08:00:00Z"')`,
		`INSERT INTO trakt_history (id, media_type, tmdb_id, trakt_id, title, watched_at) VALUES (9001, 'movie', 603, 481, 'The Matrix', NOW())`,
		`INSERT INTO trakt_watched_movies (tmdb_id, plays, last_watched_at) VALUES (603, 1, NOW())`,
		`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, plays, last_watched_at) VALUES (1399, 1, 1, 1, NOW())`,
		`INSERT INTO trakt_ratings (media_type, trakt_id, tmdb_id, rating, rated_at) VALUES ('movie', 481, 603, 9, NOW())`,
		`INSERT INTO trakt_watchlist (media_type, trakt_id, tmdb_id, title, listed_at) VALUES ('movie', 481, 603, 'The Matrix', NOW())`,
		`INSERT INTO trakt_show_progress (show_trakt_id, show_tmdb_id, title, aired, completed) VALUES (1390, 1399, 'Game of Thrones', 73, 1)`,
		`INSERT INTO trakt_hidden (section, trakt_id, tmdb_id) VALUES ('dropped', 1390, 1399)`,
		`INSERT INTO trakt_calendar (show_trakt_id, season, episode, show_tmdb_id, show_title, first_aired) VALUES (1390, 9, 1, 1399, 'Game of Thrones', NOW())`,
		`INSERT INTO app_settings (key, value) VALUES ('trakt', '{"include_paused": true}'), ('ai', '{"rec_enabled": false}')`,
	} {
		exec(q)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	data, err := migrationsFS.ReadFile("migrations/012_member_trakt.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(data)) // idempotent

	// Every row is the lowest member's.
	for _, table := range traktTables {
		var all, owned int
		if err := db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE member_id = 2) FROM `+table).Scan(&all, &owned); err != nil {
			t.Fatal(err)
		}
		if all != 1 || owned != 1 {
			t.Errorf("%s: %d rows, %d of them member 2's; want its one row, member 2's", table, all, owned)
		}
	}

	// The keys.
	for table, want := range map[string]string{
		"trakt_auth":             "member_id",
		"trakt_sync_state":       "member_id,key",
		"trakt_history":          "member_id,id",
		"trakt_watched_movies":   "tmdb_id,member_id",
		"trakt_watched_episodes": "show_tmdb_id,season,episode,member_id",
		"trakt_ratings":          "member_id,media_type,trakt_id",
		"trakt_watchlist":        "member_id,media_type,trakt_id",
		"trakt_show_progress":    "member_id,show_trakt_id",
		"trakt_hidden":           "member_id,section,trakt_id",
		"trakt_calendar":         "member_id,show_trakt_id,season,episode",
	} {
		var got string
		if err := db.QueryRow(`
			SELECT string_agg(a.attname, ',' ORDER BY k.n)
			FROM pg_index i
			CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, n)
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
			WHERE i.indrelid = $1::regclass AND i.indisprimary`, table).Scan(&got); err != nil || got != want {
			t.Errorf("%s's primary key = (%s) %v, want (%s)", table, got, err, want)
		}
	}
	var hasID bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'trakt_auth' AND column_name = 'id')`).Scan(&hasID); err != nil || hasID {
		t.Errorf("trakt_auth.id is still there (%v)", err)
	}
	for name, want := range map[string]string{
		"trakt_auth_user_slug_key":          "UNIQUE INDEX trakt_auth_user_slug_key ON %s.trakt_auth USING btree (user_slug)",
		"idx_trakt_history_watched":         "INDEX idx_trakt_history_watched ON %s.trakt_history USING btree (member_id, watched_at DESC, id DESC)",
		"idx_trakt_history_title":           "INDEX idx_trakt_history_title ON %s.trakt_history USING btree (member_id, media_type, tmdb_id)",
		"idx_trakt_watched_movies_member":   "INDEX idx_trakt_watched_movies_member ON %s.trakt_watched_movies USING btree (member_id)",
		"idx_trakt_watched_episodes_member": "INDEX idx_trakt_watched_episodes_member ON %s.trakt_watched_episodes USING btree (member_id)",
		"idx_trakt_ratings_tmdb":            "INDEX idx_trakt_ratings_tmdb ON %s.trakt_ratings USING btree (member_id, media_type, tmdb_id)",
		"idx_trakt_watchlist_tmdb":          "INDEX idx_trakt_watchlist_tmdb ON %s.trakt_watchlist USING btree (member_id, media_type, tmdb_id)",
		"idx_trakt_watchlist_listed":        "INDEX idx_trakt_watchlist_listed ON %s.trakt_watchlist USING btree (member_id, listed_at DESC)",
		"idx_trakt_calendar_aired":          "INDEX idx_trakt_calendar_aired ON %s.trakt_calendar USING btree (member_id, first_aired)",
		"idx_trakt_show_progress_tmdb":      "INDEX idx_trakt_show_progress_tmdb ON %s.trakt_show_progress USING btree (show_tmdb_id)",
		"idx_trakt_hidden_tmdb":             "INDEX idx_trakt_hidden_tmdb ON %s.trakt_hidden USING btree (tmdb_id)",
		"idx_trakt_calendar_show":           "INDEX idx_trakt_calendar_show ON %s.trakt_calendar USING btree (show_tmdb_id)",
	} {
		var schema, def string
		err := db.QueryRow(`SELECT schemaname, indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, name).
			Scan(&schema, &def)
		if want = "CREATE " + fmt.Sprintf(want, schema); err != nil || def != want {
			t.Errorf("%s = %q %v\nwant %q", name, def, err, want)
		}
	}

	// The settings are the lowest member's; the other keys stay.
	if got, err := NewMemberRepository(db).TraktSettings(ctx, 2); err != nil || got != (TraktSettings{UseRatings: true, ShowWatchlist: true, IncludePaused: true}) {
		t.Errorf("member 2's Trakt settings = %+v, %v", got, err)
	}
	var keys string
	if err := db.QueryRow(`SELECT string_agg(key, ',' ORDER BY key) FROM app_settings`).Scan(&keys); err != nil || keys != "ai" {
		t.Errorf("app_settings keys = %q (%v), want only ai", keys, err)
	}

	// An account connects to one member; every row names a member that
	// exists.
	exec(`INSERT INTO members (name, color) VALUES ('Ari', 'lime')`) // 3
	for _, tt := range []struct{ name, insert, constraint string }{
		{"the account for another member", `INSERT INTO trakt_auth (member_id, username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at)
			VALUES (3, 'Teymur', 'teymur', '\x01', '\x02', NOW(), NOW())`, "trakt_auth_user_slug_key"},
		{"no member", `INSERT INTO trakt_hidden (section, trakt_id, tmdb_id) VALUES ('dropped', 1, 1)`, `"member_id"`},
		{"an unknown member", `INSERT INTO trakt_hidden (member_id, section, trakt_id, tmdb_id) VALUES (9, 'dropped', 1, 1)`, "trakt_hidden_member_id_fkey"},
	} {
		if _, err := db.Exec(tt.insert); err == nil || !strings.Contains(err.Error(), tt.constraint) {
			t.Errorf("%s: %v, want %s", tt.name, err, tt.constraint)
		}
	}
	exec(`INSERT INTO trakt_hidden (member_id, section, trakt_id, tmdb_id) VALUES (3, 'dropped', 1390, 1399)`)

	// A member's rows go with them.
	exec(`DELETE FROM members WHERE id = 2`)
	for _, table := range traktTables {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table + ` WHERE member_id <> 3`).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s keeps %d rows of the removed member (%v)", table, n, err)
		}
	}
	var kept int
	if err := db.QueryRow(`SELECT count(*) FROM trakt_hidden WHERE member_id = 3`).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("the other member's hidden show = %d rows (%v), want it kept", kept, err)
	}
}

// 013 on a database at 012, as production has it after #243, run twice:
// every profile, run and feedback is the lowest member's (member 1 is gone
// here, so 2's), one run is active and one signal kept per member and
// title, the reads' indexes lead with the member, member_id has no
// default, an unknown member is refused, and a member's rows (a run's
// shelves with it) go with them.
func TestMigration013_MemberPicks(t *testing.T) {
	db := openBareSchemaDB(t, "mig013")
	ctx := context.Background()
	migrateUpTo(t, db, 13)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}
	exec(`INSERT INTO members (name, color) VALUES ('Sam', 'rose')`)
	exec(`DELETE FROM members WHERE id = 1`)
	exec(`INSERT INTO taste_profiles (counts, profile) VALUES ('{"loved": 12}', '{}')`)
	exec(`INSERT INTO rec_runs (kind, status, model, profile_id, finished_at)
		VALUES ('scheduled', 'succeeded', 'x', (SELECT min(id) FROM taste_profiles), NOW())`)
	exec(`INSERT INTO rec_runs (kind, status, model) VALUES ('manual', 'queued', 'x')`)
	exec(`INSERT INTO rec_shelves (run_id, position, slug, kind, title) VALUES ((SELECT min(id) FROM rec_runs), 0, 'a', 'other', 'A')`)
	exec(`INSERT INTO rec_feedback (media_type, tmdb_id, signal, source) VALUES ('tv', 1399, 'dislike', 'shelf')`)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	data, err := migrationsFS.ReadFile("migrations/013_member_picks.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(data)) // idempotent

	picksTables := []string{"taste_profiles", "rec_runs", "rec_feedback"}
	for _, table := range picksTables {
		var all, owned int
		if err := db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE member_id = 2) FROM `+table).Scan(&all, &owned); err != nil {
			t.Fatal(err)
		}
		if all == 0 || all != owned {
			t.Errorf("%s: %d rows, %d of them member 2's; want them all member 2's", table, all, owned)
		}
	}
	var pkey string
	if err := db.QueryRow(`
		SELECT string_agg(a.attname, ',' ORDER BY k.n)
		FROM pg_index i
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, n)
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE i.indrelid = 'rec_feedback'::regclass AND i.indisprimary`).Scan(&pkey); err != nil || pkey != "member_id,media_type,tmdb_id" {
		t.Errorf("rec_feedback's primary key = (%s) %v, want (member_id,media_type,tmdb_id)", pkey, err)
	}
	for name, want := range map[string]string{
		"rec_runs_one_active": "UNIQUE INDEX rec_runs_one_active ON %s.rec_runs USING btree (member_id) " +
			"WHERE (status = ANY (ARRAY['queued'::text, 'running'::text]))",
		"idx_rec_runs_member_finished": "INDEX idx_rec_runs_member_finished ON %s.rec_runs USING btree (member_id, status, finished_at DESC)",
		"idx_rec_runs_member":          "INDEX idx_rec_runs_member ON %s.rec_runs USING btree (member_id, id DESC)",
		"idx_rec_runs_queued":          "INDEX idx_rec_runs_queued ON %s.rec_runs USING btree (created_at, id) WHERE (status = 'queued'::text)",
		"idx_taste_profiles_member":    "INDEX idx_taste_profiles_member ON %s.taste_profiles USING btree (member_id, id DESC)",
		"idx_rec_runs_profile":         "INDEX idx_rec_runs_profile ON %s.rec_runs USING btree (profile_id)",
	} {
		var schema, def string
		err := db.QueryRow(`SELECT schemaname, indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, name).
			Scan(&schema, &def)
		if want = "CREATE " + fmt.Sprintf(want, schema); err != nil || def != want {
			t.Errorf("%s = %q %v\nwant %q", name, def, err, want)
		}
	}
	var stale bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema()
		AND indexname = 'idx_rec_runs_status_finished')`).Scan(&stale); err != nil || stale {
		t.Errorf("idx_rec_runs_status_finished is still there (%v)", err)
	}

	// One active run and one signal per member and title; every row names
	// a member that exists.
	exec(`INSERT INTO members (name, color) VALUES ('Ari', 'lime')`) // 3
	exec(`INSERT INTO rec_runs (member_id, kind, status, model) VALUES (3, 'manual', 'running', 'x')`)
	exec(`INSERT INTO rec_feedback (member_id, media_type, tmdb_id, signal, source) VALUES (3, 'tv', 1399, 'like', 'hero')`)
	exec(`INSERT INTO taste_profiles (member_id, counts, profile) VALUES (3, '{}', '{}')`)
	for _, tt := range []struct{ name, insert, constraint string }{
		{"a second active run", `INSERT INTO rec_runs (member_id, kind, status, model) VALUES (2, 'scheduled', 'queued', 'x')`, "rec_runs_one_active"},
		{"feedback twice on a title", `INSERT INTO rec_feedback (member_id, media_type, tmdb_id, signal, source) VALUES (2, 'tv', 1399, 'like', 'title')`, "rec_feedback_pkey"},
		{"no member", `INSERT INTO rec_runs (kind, status, model) VALUES ('manual', 'failed', 'x')`, `"member_id"`},
		{"an unknown member", `INSERT INTO rec_feedback (member_id, media_type, tmdb_id, signal, source) VALUES (9, 'movie', 1, 'like', 'title')`, "rec_feedback_member_id_fkey"},
		{"a profile for an unknown member", `INSERT INTO taste_profiles (member_id, counts, profile) VALUES (9, '{}', '{}')`, "taste_profiles_member_id_fkey"},
	} {
		if _, err := db.Exec(tt.insert); err == nil || !strings.Contains(err.Error(), tt.constraint) {
			t.Errorf("%s: %v, want %s", tt.name, err, tt.constraint)
		}
	}

	// A member's rows go with them, their runs' shelves too.
	exec(`DELETE FROM members WHERE id = 2`)
	for _, table := range append(picksTables, "rec_shelves") {
		var gone int
		query := `SELECT count(*) FROM ` + table + ` WHERE member_id = 2`
		if table == "rec_shelves" {
			query = `SELECT count(*) FROM rec_shelves`
		}
		if err := db.QueryRow(query).Scan(&gone); err != nil || gone != 0 {
			t.Errorf("%s keeps %d rows of the removed member (%v)", table, gone, err)
		}
	}
	for _, table := range picksTables {
		var kept int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table + ` WHERE member_id = 3`).Scan(&kept); err != nil || kept != 1 {
			t.Errorf("%s: the other member's rows = %d (%v), want their one", table, kept, err)
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
