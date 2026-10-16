package main

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
)

// /settings/ai is served with or without a key; the loop's client only
// with one.
func TestWireAI(t *testing.T) {
	cfg := config.DefaultConfig()
	var opts api.Options
	core := wireAI(aiDeps{cfg: cfg}, &opts)
	if opts.AISettings == nil || core.settings == nil || core.ledger == nil || core.client != nil {
		t.Errorf("without a key: settings %v, client %v", opts.AISettings, core.client)
	}

	cfg.AI.AnthropicAPIKey = "sk-test"
	opts = api.Options{}
	core = wireAI(aiDeps{cfg: cfg}, &opts)
	if opts.AISettings == nil || core.client == nil {
		t.Errorf("with a key: settings %v, client %v", opts.AISettings, core.client)
	}
	core.Close()
}
