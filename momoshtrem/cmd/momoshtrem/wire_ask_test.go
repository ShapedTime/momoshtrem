package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// Ask is served without a key (threads read; asking answers unavailable),
// and fills in the usage page's asks_today.
func TestWireAsk(t *testing.T) {
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := library.NewDB(url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	d := aiDeps{cfg: config.DefaultConfig(), db: db, settings: library.NewSettingsRepository(db), aiStore: library.NewAIStore(db), member: 1}
	var opts api.Options
	core := wireAI(d, &opts)
	ask := wireAsk(d, core, wireRecs(d, core, &opts), &opts)
	defer ask.Close()

	if opts.Ask == nil {
		t.Fatal("Options.Ask isn't set without a key")
	}
	if _, err := opts.Ask.Ask(context.Background(), recommend.AskRequest{Message: "anime"}); !errors.Is(err, recommend.ErrUnavailable) {
		t.Errorf("Ask without a key = %v, want ErrUnavailable", err)
	}
	if _, err := opts.Ask.Threads(context.Background()); err != nil {
		t.Errorf("Threads without a key = %v", err)
	}
	usage, err := opts.AISettings.AIUsage(context.Background())
	if err != nil || usage.AskDailyLimit != recommend.AskDailyLimit {
		t.Errorf("usage = %+v, %v", usage, err)
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
