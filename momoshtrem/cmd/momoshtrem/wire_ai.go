package main

import (
	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
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
	settings *library.SettingsRepository // app_settings: "ai" and "trakt" (use_ratings)
	aiStore  *library.AIStore            // ai_usage and ai_cache, shared with the classifier
	tmdb     *tmdb.Cached
	// catalog joins the library state onto rec cards (LibraryStates).
	catalog *catalog.Service
	// traktAuth says whether Trakt is connected; traktSync's hooks queue
	// the first_sync run when the initial import finishes.
	traktAuth *trakt.Auth
	traktSync *trakt.Syncer
}

// aiCore is the AI core (#154): the Claude client, our streaming tool loop,
// the pricing table (web searches and fallback hops included) and the
// monthly budget every AI feature shares. Picks and Ask run on it.
type aiCore struct{}

// wireAI sets up the AI core and serves GET/PUT /settings/ai and
// /settings/ai/usage (Options.AISettings), with or without a key. #154
// fills it in; until then those routes answer 503.
func wireAI(d aiDeps, opts *api.Options) *aiCore {
	_, _ = d, opts
	return &aiCore{}
}

// Close stops what wireAI started.
func (c *aiCore) Close() {}
