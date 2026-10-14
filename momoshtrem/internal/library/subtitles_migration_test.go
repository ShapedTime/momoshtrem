package library

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// 007 on a database shaped by 001–003: existing subtitles survive, the old
// unique constraint goes, and running it twice is harmless. It runs in a
// private schema on one connection so other tests sharing the database
// never see the old constraint.
func TestMigration007_KeepsExistingSubtitles(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := fmt.Sprintf("mig007_%d", time.Now().UnixNano())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}
	exec(`CREATE SCHEMA ` + schema)
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `RESET search_path`)
		_, _ = conn.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
	}()
	exec(`SET search_path TO ` + schema)

	migration := func(name string) string {
		t.Helper()
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, name := range []string{"001_core_schema.sql", "002_app_settings.sql", "003_metadata.sql"} {
		exec(migration(name))
	}

	const hash = "abcdef0123456789abcdef0123456789abcdef01"
	exec(`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash) VALUES
		('movie', 1, 'en', 'English', 'srt', '/data/subtitles/movie/1/en.srt', 1234, 'opensubtitles', NULL),
		('episode', 7, 'en', 'English', 'ass', 'Show.S01/Subs/E01/en.ass', 99, 'torrent', $1),
		('episode', 8, 'ru', 'Russian', 'srt', 'broken.ru.srt', 10, 'torrent', NULL)`, hash)

	exec(migration("007_subtitles.sql"))
	exec(migration("007_subtitles.sql")) // Idempotent

	rows, err := conn.QueryContext(ctx, `SELECT item_type, item_id, language_code, source, COALESCE(info_hash, ''), release_name IS NULL
		FROM subtitles ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var itemType, lang, source, infoHash string
		var itemID int64
		var noRelease bool
		if err := rows.Scan(&itemType, &itemID, &lang, &source, &infoHash, &noRelease); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s/%d/%s/%s/%s/%v", itemType, itemID, lang, source, infoHash, noRelease))
	}
	rows.Close()
	want := []string{
		"movie/1/en/opensubtitles//true",
		"episode/7/en/torrent/" + hash + "/true",
		// episode 8's torrent row had no info hash: it couldn't stream and is dropped.
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rows after 007 =\n%v\nwant\n%v", got, want)
	}

	var oldConstraints int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_constraint
		WHERE conrelid = 'subtitles'::regclass AND contype = 'u'`).Scan(&oldConstraints); err != nil {
		t.Fatal(err)
	}
	if oldConstraints != 0 {
		t.Errorf("unique constraints left = %d, want the old one dropped", oldConstraints)
	}

	// The new keys: a torrent subtitle in the downloaded one's language
	// coexists; a second download in that language conflicts.
	exec(`INSERT INTO subtitles (item_type, item_id, language_code, language_name, file_path, source, info_hash)
		VALUES ('movie', 1, 'en', 'English', 'Movie/en.srt', 'torrent', $1)`, hash)
	if _, err := conn.ExecContext(ctx, `INSERT INTO subtitles (item_type, item_id, language_code, language_name, file_path, source)
		VALUES ('movie', 1, 'en', 'English', '/other.srt', 'opensubtitles')`); err == nil {
		t.Error("a second downloaded English subtitle for movie 1 was accepted")
	}
}
