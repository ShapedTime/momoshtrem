package library

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// 005 after every other migration (it can land after 007–009: numbers are
// reserved per task), run twice in a private schema on one connection:
// every table takes a row, and the CHECKs hold.
func TestMigration005_Trakt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := fmt.Sprintf("mig005_%d", time.Now().UnixNano())
	exec := func(query string, args ...any) error {
		_, err := conn.ExecContext(ctx, query, args...)
		return err
	}
	must := func(query string, args ...any) {
		t.Helper()
		if err := exec(query, args...); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}
	must(`CREATE SCHEMA ` + schema)
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `RESET search_path`)
		_, _ = conn.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
	}()
	must(`SET search_path TO ` + schema)

	all, err := embeddedMigrations(migrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, m := range all {
		names = append(names, m.Name)
	}
	names = append(names, "005_trakt.sql") // Idempotent
	for _, name := range names {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		must(string(data))
	}

	rows := map[string]string{
		"trakt_auth": `INSERT INTO trakt_auth (member_id, username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at)
			VALUES (1, 'Teymur', 'teymur', '\x01aa', '\x01bb', NOW(), NOW() + INTERVAL '7 days')`,
		"trakt_sync_state": `INSERT INTO trakt_sync_state (member_id, key, value) VALUES (1, 'last_activities', '{"all": "2026-10-16T08:00:00Z"}')`,
		"trakt_history": `INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, year, season, episode, episode_tmdb_id, episode_title, watched_at) VALUES
			(1, 9001, 'movie', 693134, 367444, 'Dune: Part Two', 2024, NULL, NULL, NULL, NULL, NOW()),
			(1, 9002, 'episode', 209867, 184317, 'Frieren', 2023, 1, 26, 4720001, 'The Height of Magic', NOW())`,
		"trakt_watched_movies":   `INSERT INTO trakt_watched_movies (member_id, tmdb_id, plays, last_watched_at) VALUES (1, 693134, 1, NOW())`,
		"trakt_watched_episodes": `INSERT INTO trakt_watched_episodes (member_id, show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at) VALUES (1, 209867, 1, 26, 4720001, 1, NOW())`,
		"trakt_ratings": `INSERT INTO trakt_ratings (member_id, media_type, trakt_id, tmdb_id, show_tmdb_id, season, episode, rating, rated_at) VALUES
			(1, 'show', 184317, 209867, NULL, NULL, NULL, 10, NOW()), (1, 'episode', 9988776, NULL, 209867, 1, 26, 9, NOW())`,
		"trakt_watchlist": `INSERT INTO trakt_watchlist (member_id, media_type, trakt_id, tmdb_id, title, year, poster_path, backdrop_path, rank, listed_at)
			VALUES (1, 'movie', 761234, 666277, 'Past Lives', 2023, '/p.jpg', NULL, 1, NOW())`,
		"trakt_show_progress": `INSERT INTO trakt_show_progress (member_id, show_trakt_id, show_tmdb_id, title, year, aired, completed, last_watched_at,
			last_season, last_episode, next_season, next_episode, next_episode_tmdb_id, next_title)
			VALUES (1, 184317, 209867, 'Frieren', 2023, 38, 26, NOW(), 1, 26, 1, 27, 4720002, 'An Ordinary Mage')`,
		"trakt_hidden": `INSERT INTO trakt_hidden (member_id, section, trakt_id, tmdb_id) VALUES (1, 'dropped', 1390, 1399), (1, 'progress_watched', 1390, 1399)`,
		"trakt_calendar": `INSERT INTO trakt_calendar (member_id, show_trakt_id, season, episode, show_tmdb_id, episode_tmdb_id, show_title, year, episode_title, first_aired)
			VALUES (1, 154997, 2, 10, 95396, 5550010, 'Severance', 2022, 'Cold Harbor', NOW() + INTERVAL '7 days')`,
	}
	for table, insert := range rows {
		if err := exec(insert); err != nil {
			t.Errorf("%s: %v", table, err)
		}
	}

	rejected := []struct {
		name, insert, constraint string
	}{
		{"a second connection of the member", `INSERT INTO trakt_auth (member_id, username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at)
			VALUES (1, 'x', 'x', '\x01', '\x01', NOW(), NOW())`, "trakt_auth_pkey"},
		{"an episode play without its number", `INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, season, watched_at)
			VALUES (1, 9003, 'episode', 209867, 184317, 'Frieren', 1, NOW())`, "trakt_history_episode_check"},
		{"a movie play with an episode", `INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, season, episode, watched_at)
			VALUES (1, 9004, 'movie', 693134, 367444, 'Dune', 1, 1, NOW())`, "trakt_history_episode_check"},
		{"a rating of 11", `INSERT INTO trakt_ratings (member_id, media_type, trakt_id, tmdb_id, rating, rated_at) VALUES (1, 'movie', 1, 603, 11, NOW())`, "trakt_ratings_rating_check"},
		{"a watchlist show called show", `INSERT INTO trakt_watchlist (member_id, media_type, trakt_id, tmdb_id, title, listed_at) VALUES (1, 'show', 1, 1399, 'x', NOW())`, "trakt_watchlist_media_type_check"},
		{"an unknown hidden section", `INSERT INTO trakt_hidden (member_id, section, trakt_id, tmdb_id) VALUES (1, 'calendar', 1, 1)`, "trakt_hidden_section_check"},
	}
	for _, tt := range rejected {
		err := exec(tt.insert)
		if err == nil || !strings.Contains(err.Error(), tt.constraint) {
			t.Errorf("%s: %v, want %s", tt.name, err, tt.constraint)
		}
	}

	for table := range rows {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil || n == 0 {
			t.Errorf("%s has %d rows (%v), want its insert", table, n, err)
		}
	}
}
