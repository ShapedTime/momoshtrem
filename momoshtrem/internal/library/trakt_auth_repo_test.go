package library

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func traktAuthRow(slug string) TraktAuth {
	created := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	return TraktAuth{
		Username: "Teymur", UserSlug: slug,
		AccessTokenEnc: []byte{1, 0xaa}, RefreshTokenEnc: []byte{1, 0xbb},
		TokenCreatedAt: created, ExpiresAt: created.Add(7 * 24 * time.Hour),
	}
}

// seedTraktData puts a row of member's in every Trakt table but
// trakt_auth (two in the watchlist).
func seedTraktData(t *testing.T, db *DB, member int64) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO trakt_sync_state (member_id, key, value) VALUES ($1, 'last_sync_at', '"2026-10-16T08:00:00Z"')`,
		`INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, watched_at) VALUES ($1, 9001, 'movie', 603, 481, 'The Matrix', NOW())`,
		`INSERT INTO trakt_watched_movies (member_id, tmdb_id, plays, last_watched_at) VALUES ($1, 603, 1, NOW())`,
		`INSERT INTO trakt_watched_episodes (member_id, show_tmdb_id, season, episode, plays, last_watched_at) VALUES ($1, 1399, 1, 1, 1, NOW())`,
		`INSERT INTO trakt_ratings (member_id, media_type, trakt_id, tmdb_id, rating, rated_at) VALUES ($1, 'movie', 481, 603, 9, NOW())`,
		`INSERT INTO trakt_watchlist (member_id, media_type, trakt_id, tmdb_id, title, listed_at) VALUES ($1, 'movie', 481, 603, 'The Matrix', NOW()), ($1, 'tv', 1390, 1399, 'Game of Thrones', NOW())`,
		`INSERT INTO trakt_show_progress (member_id, show_trakt_id, show_tmdb_id, title, aired, completed) VALUES ($1, 1390, 1399, 'Game of Thrones', 73, 1)`,
		`INSERT INTO trakt_hidden (member_id, section, trakt_id, tmdb_id) VALUES ($1, 'dropped', 1390, 1399)`,
		`INSERT INTO trakt_calendar (member_id, show_trakt_id, season, episode, show_tmdb_id, show_title, first_aired) VALUES ($1, 1390, 9, 1, 1399, 'Game of Thrones', NOW())`,
	} {
		if _, err := db.Exec(q, member); err != nil {
			t.Fatalf("%.50s…: %v", q, err)
		}
	}
}

// rowCounts counts member's rows of every table the schema has whose name
// starts with trakt_, so a table added later without Purge knowing fails.
func rowCounts(t *testing.T, db *DB, member int64) map[string]int {
	t.Helper()
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name LIKE 'trakt\_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if len(tables) < len(traktTables) {
		t.Fatalf("tables = %v, want all of %v", tables, traktTables)
	}
	counts := map[string]int{}
	for _, table := range tables {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE member_id = $1`, member).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
	}
	return counts
}

func TestTraktAuthRepository_ConnectAndTokens(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewTraktAuthRepository(db).ForMember(1)
	ctx := context.Background()

	if got, err := repo.Get(ctx); got != nil || err != nil {
		t.Fatalf("Get before a connect = %+v, %v", got, err)
	}
	gen, err := repo.Connect(ctx, traktAuthRow("teymur"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx)
	if err != nil || got == nil {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if got.UserSlug != "teymur" || !bytes.Equal(got.AccessTokenEnc, []byte{1, 0xaa}) || !got.ConnectedAt.Equal(gen) || got.NeedsReauth {
		t.Errorf("stored = %+v", got)
	}

	// A refresh names the generation; another generation is refused.
	tokens := TraktTokens{AccessTokenEnc: []byte{1, 0xcc}, RefreshTokenEnc: []byte{1, 0xdd}, CreatedAt: got.TokenCreatedAt.Add(time.Hour), ExpiresAt: got.ExpiresAt.Add(time.Hour)}
	if err := repo.UpdateTokens(ctx, gen.Add(-time.Second), tokens); !errors.Is(err, ErrTraktConnectionReplaced) {
		t.Errorf("UpdateTokens with an old generation = %v", err)
	}
	if err := repo.UpdateTokens(ctx, gen, tokens); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetReauth(ctx, gen, "Trakt stopped accepting momoshtrem’s sign-in."); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx)
	if !bytes.Equal(got.RefreshTokenEnc, []byte{1, 0xdd}) || !got.ExpiresAt.Equal(tokens.ExpiresAt) || !got.NeedsReauth || got.ReauthReason == "" {
		t.Errorf("after the refresh and reauth = %+v", got)
	}

	// Reconnecting the same account keeps its data, clears reauth and
	// starts a new generation.
	seedTraktData(t, db, 1)
	gen2, err := repo.Connect(ctx, traktAuthRow("teymur"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx)
	if !gen2.After(gen) || got.NeedsReauth || got.ReauthReason != "" {
		t.Errorf("reconnect = %+v (generation %v after %v)", got, gen2, gen)
	}
	if n := rowCounts(t, db, 1)["trakt_history"]; n != 1 {
		t.Errorf("the same account's history = %d rows, want it kept", n)
	}
	if err := repo.UpdateTokens(ctx, gen, tokens); !errors.Is(err, ErrTraktConnectionReplaced) {
		t.Errorf("a refresh of the replaced connection = %v", err)
	}

	// Another account purges first.
	if _, err := repo.Connect(ctx, traktAuthRow("someone-else")); err != nil {
		t.Fatal(err)
	}
	for table, n := range rowCounts(t, db, 1) {
		if want := map[bool]int{true: 1, false: 0}[table == "trakt_auth"]; n != want {
			t.Errorf("after another account connected, %s has %d rows, want %d", table, n, want)
		}
	}
}

func TestTraktAuthRepository_PurgeAndCounts(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewTraktAuthRepository(db).ForMember(1)
	settings := NewMemberRepository(db).TraktSettingsFor(1)
	ctx := context.Background()

	if _, err := repo.Connect(ctx, traktAuthRow("teymur")); err != nil {
		t.Fatal(err)
	}
	seedTraktData(t, db, 1)
	if _, err := db.Exec(`INSERT INTO movies (tmdb_id, title, year) VALUES (603, 'The Matrix', 1999)`); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.UpdateTraktSettings(ctx, TraktSettingsPatch{IncludePaused: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}

	counts, err := repo.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (TraktCounts{History: 1, Ratings: 1, Watchlist: 2, WatchlistInLibrary: 1}) {
		t.Errorf("counts = %+v", counts)
	}

	if err := repo.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	for table, n := range rowCounts(t, db, 1) {
		if n != 0 {
			t.Errorf("after the purge %s has %d rows", table, n)
		}
	}
	if got, err := settings.TraktSettings(ctx); err != nil || !got.IncludePaused {
		t.Errorf("settings after the purge = %+v, %v; want them kept", got, err)
	}
	if err := repo.Purge(ctx); err != nil {
		t.Errorf("a second purge = %v", err)
	}
}
