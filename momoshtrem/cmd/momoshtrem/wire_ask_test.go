package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
)

// openWireDB opens the test database in a new schema, migrated.
func openWireDB(t *testing.T) *library.DB {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	base, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { base.Close() })
	schema := fmt.Sprintf("wire_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	sqlDB, err := sql.Open("pgx", url+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db := library.NewDBFromSQL(sqlDB)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

// Ask is served without a key (threads read; asking answers unavailable),
// each household member's: their threads, their asks today and everyone's
// spend on the usage page, and their own stored taste in the tools
// (askMember). A removed member's threads go with them.
func TestWireAsk(t *testing.T) {
	db := openWireDB(t)
	ctx := context.Background()
	members := member.NewService(library.NewMemberRepository(db))
	sam, err := members.Create(ctx, "Sam", nil)
	if err != nil {
		t.Fatal(err)
	}
	me, other := member.WithID(ctx, 1), member.WithID(ctx, sam.ID)
	d := aiDeps{cfg: config.DefaultConfig(), db: db, settings: library.NewSettingsRepository(db), aiStore: library.NewAIStore(db),
		members: members}
	var opts api.Options
	core := wireAI(d, &opts)
	recs := wireRecs(d, core, &opts)
	ask := wireAsk(d, core, recs, &opts)
	defer ask.Close()

	if opts.Ask == nil {
		t.Fatal("Options.Ask isn't set without a key")
	}
	if _, err := opts.Ask.Ask(me, recommend.AskRequest{Message: "anime"}); !errors.Is(err, recommend.ErrUnavailable) {
		t.Errorf("Ask without a key = %v, want ErrUnavailable", err)
	}
	if _, err := db.Exec(`INSERT INTO ask_threads (member_id, title, model, prompt_version) VALUES ($1, 'Sam''s', 'x', 'ask-v2')`, sam.ID); err != nil {
		t.Fatal(err)
	}
	if threads, err := opts.Ask.Threads(me); err != nil || len(threads) != 0 {
		t.Errorf("Me's threads without a key = %v, %v; want none", threads, err)
	}
	if threads, err := opts.Ask.Threads(other); err != nil || len(threads) != 1 {
		t.Errorf("Sam's threads = %v, %v; want their one", threads, err)
	}
	usage, err := opts.AISettings.AIUsage(other)
	if err != nil || usage.AskDailyLimit != recommend.AskDailyLimit || len(usage.ByMember) != 2 ||
		usage.ByMember[0].Name != "Me" || usage.ByMember[1].Name != "Sam" {
		t.Errorf("usage = %+v, %v", usage, err)
	}

	// Each member's tools read their own stored profile.
	for id, title := range map[int64]string{1: "Severance", sam.ID: "Frieren"} {
		if _, err := db.Exec(`INSERT INTO taste_profiles (member_id, counts, profile) VALUES ($1, '{}', $2)`,
			id, `{"loved": [{"media_type": "tv", "tmdb_id": 1, "title": "`+title+`"}]}`); err != nil {
			t.Fatal(err)
		}
	}
	for id, title := range map[int64]string{1: "Severance", sam.ID: "Frieren"} {
		m := askMember(recs)(id)
		if len(m.Tools) == 0 || m.Tools[0].Name() != "get_taste_profile" {
			t.Fatalf("member %d's tools = %v", id, m.Tools)
		}
		out, err := m.Tools[0].Run(ctx, json.RawMessage(`{}`))
		var p profile.Profile
		if err != nil || json.Unmarshal(out, &p) != nil || len(p.Loved) != 1 || p.Loved[0].Title != title {
			t.Errorf("member %d's get_taste_profile = %s, %v; want their %s", id, out, err, title)
		}
	}

	// Removing Sam (their Ask let go first) takes their threads.
	if err := members.Delete(ctx, sam.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT count(*) FROM ask_threads`).Scan(&left); err != nil || left != 0 {
		t.Errorf("threads after Sam's removal = %d (%v), want none", left, err)
	}

	done := make(chan struct{})
	go func() {
		ask.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("Close didn't return")
	}
}
