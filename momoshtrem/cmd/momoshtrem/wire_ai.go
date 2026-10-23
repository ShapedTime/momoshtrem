package main

import (
	"context"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aisettings"
	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// aiDeps is what the AI features (#9) are built from. main fills it in
// once; wire_ai.go, wire_recs.go and wire_ask.go each take what they need,
// so the slices that implement them (#153, #154, #157, #158) only touch
// their own wire file.
type aiDeps struct {
	cfg      *config.Config // AI.AnthropicAPIKey; Anthropic is off without it
	db       *library.DB
	settings *library.SettingsRepository // app_settings: "ai"
	aiStore  *library.AIStore            // ai_usage and ai_cache, shared with the classifier
	tmdb     *tmdb.Cached
	// members are the household's: the picks and Ask are each member's
	// (#245, #246), the schedule queues a run for each with Trakt
	// connected, and a removed member's run and Ask turns are stopped
	// (OnRemoving listeners). Nil in recommend-run, which runs one member.
	members *member.Service
	// catalog joins the library state onto rec cards (LibraryStates) and
	// takes the title pages' reasons (a SetReasons setter, #157).
	catalog *catalog.Service
	// trakt says whether a member's Trakt is connected (Connection,
	// Connected); the first_sync run hooks onto the end of each member's
	// initial import (a SetOnImported setter, #157).
	trakt *trakt.Registry
}

// aiCore is the AI core (#154) the picks and Ask run on.
type aiCore struct {
	// client runs tool loops (Client.Run), recording each API call's usage
	// in the ledger; nil without ANTHROPIC_API_KEY.
	client *aicore.Client
	// ledger records usage and checks the monthly budget every AI feature
	// shares (Check before a run or an Ask turn; a *aicore.BudgetError
	// carries resets_at).
	ledger *aicore.Ledger
	// settings serves /settings/ai and its usage; the picks and Ask install
	// their parts of the usage page with SetRecs (#157) and SetAsk (#158).
	settings *aisettings.Service
}

// wireAI sets up the AI core and serves GET/PUT /settings/ai and
// /settings/ai/usage (Options.AISettings), with or without a key.
func wireAI(d aiDeps, opts *api.Options) *aiCore {
	key := d.cfg.AI.AnthropicAPIKey
	ledger := aicore.NewLedger(d.aiStore, d.settings)
	core := &aiCore{
		client:   aicore.New(aicore.Config{APIKey: key, Recorder: ledger}),
		ledger:   ledger,
		settings: aisettings.New(d.settings, d.aiStore, key != ""),
	}
	opts.AISettings = core.settings
	if core.client == nil {
		slog.Info("ANTHROPIC_API_KEY not set: picks and Ask are off; the AI settings still work", "component", "aicore")
	}
	return core
}

// Close stops what wireAI started: nothing runs in the background.
func (c *aiCore) Close() {}

// aiMonthToDate is /status's ai.mtd_cost_usd: every AI feature's spend
// this server-local month, as the budget counts it.
func aiMonthToDate(store *library.AIStore, settings *library.SettingsRepository) func(context.Context) (float64, error) {
	return aicore.NewLedger(store, settings).MonthToDate
}
