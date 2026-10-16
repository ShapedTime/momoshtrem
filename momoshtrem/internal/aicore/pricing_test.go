package aicore

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestCost(t *testing.T) {
	tests := []struct {
		name  string
		model string
		t     Tokens
		want  float64
	}{
		{"haiku, input and output", "claude-haiku-5-5", Tokens{Input: 10_000, Output: 10_000}, (10_000*0.10 + 10_000*0.50) / 1e6},
		{"haiku, cache reads and writes", "claude-haiku-5-5", Tokens{CacheRead: 50_000, CacheWrite: 50_000}, (50_000*0.01 + 50_000*0.125) / 1e6},
		{"haiku, a prompt of exactly 100K tokens", "claude-haiku-5-5", Tokens{Input: 60_000, CacheRead: 40_000, Output: 1000},
			(60_000*0.10 + 40_000*0.01 + 1000*0.50) / 1e6},
		{"haiku, a prompt over 100K tokens costs more, output too", "claude-haiku-5-5", Tokens{Input: 60_000, CacheRead: 40_001, Output: 1000},
			(60_000*0.50 + 40_001*0.05 + 1000*2.50) / 1e6},
		{"sonnet 5.5", "claude-sonnet-5-5", Tokens{Input: 1_000_000, Output: 100_000}, 2.00 + 1.00},
		{"sonnet 5.5 reads the cache at 0.05x", "claude-sonnet-5-5", Tokens{CacheRead: 1_000_000}, 0.10},
		{"sonnet 5 reads it at 0.1x", "claude-sonnet-5", Tokens{CacheRead: 1_000_000, CacheWrite: 1_000_000}, 0.20 + 2.50},
		{"opus 5.5", "claude-opus-5-5", Tokens{Input: 100_000, CacheRead: 1_000_000}, 0.40 + 0.20},
		{"opus 5.5, 1-hour cache writes at 2x", "claude-opus-5-5", Tokens{CacheWrite: 1_000_000, CacheWrite1h: 400_000}, 0.6*5.00 + 0.4*8.00},
		{"opus 5", "claude-opus-5", Tokens{Input: 1_000_000, Output: 1_000_000, CacheRead: 1_000_000}, 5.00 + 25.00 + 0.50},
		{"opus 4.8", "claude-opus-4-8", Tokens{Input: 1_000_000, CacheWrite: 1_000_000}, 5.00 + 6.25},
		{"a dated id prices as its model", "claude-opus-4-8-20260501", Tokens{Output: 1_000_000}, 25.00},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Cost(tt.model, tt.t)
			if !ok || math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("Cost = %.9f, %v; want %.9f", got, ok, tt.want)
			}
		})
	}
	if got, ok := Cost("claude-unknown-1", Tokens{Input: 1_000_000}); ok || got != 0 {
		t.Errorf("an unknown model = %v, %v", got, ok)
	}
}

func TestEveryPricedModelHasAFullCard(t *testing.T) {
	for model, cs := range prices {
		if len(cs) == 0 || cs[len(cs)-1].upTo != 0 {
			t.Errorf("%s: the last card must apply to any length", model)
		}
		for _, c := range cs {
			if c.input <= 0 || c.output <= 0 || c.cacheRead <= 0 || c.cacheWrite5m <= 0 || c.cacheWrite1h <= 0 {
				t.Errorf("%s: a rate is missing: %+v", model, c)
			}
			if math.Abs(c.cacheWrite5m-1.25*c.input) > 1e-9 || math.Abs(c.cacheWrite1h-2*c.input) > 1e-9 {
				t.Errorf("%s: cache writes aren't 1.25x and 2x input: %+v", model, c)
			}
		}
	}
	// The models features run on and their server-side fallbacks.
	for _, m := range []string{"claude-haiku-5-5", "claude-sonnet-5-5", "claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-sonnet-5"} {
		if !Priced(m) || !slices.Contains(Models(), m) {
			t.Errorf("%s has no price", m)
		}
	}
}

// usage decodes a usage object the way the SDK reads it from the API.
func usage(t *testing.T, raw string) anthropic.BetaUsage {
	t.Helper()
	var u anthropic.BetaUsage
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestPriceUsage(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		raw      string
		want     float64
		tokens   Tokens
		unpriced []string
	}{
		{
			name: "top-level counts on the requested model", model: "claude-opus-5-5",
			raw:    `{"input_tokens": 1000, "output_tokens": 500, "cache_read_input_tokens": 10000, "cache_creation_input_tokens": 2000}`,
			want:   (1000*4.0 + 500*20.0 + 10000*0.20 + 2000*5.0) / 1e6,
			tokens: Tokens{Input: 1000, Output: 500, CacheRead: 10000, CacheWrite: 2000},
		},
		{
			name: "1-hour cache writes from the breakdown", model: "claude-sonnet-5-5",
			raw:    `{"input_tokens": 0, "output_tokens": 0, "cache_creation_input_tokens": 3000, "cache_creation": {"ephemeral_5m_input_tokens": 1000, "ephemeral_1h_input_tokens": 2000}}`,
			want:   (1000*2.50 + 2000*4.0) / 1e6,
			tokens: Tokens{CacheWrite: 3000, CacheWrite1h: 2000},
		},
		{
			name: "web searches cost $10 per 1,000, fetches nothing", model: "claude-opus-5-5",
			raw:    `{"input_tokens": 100, "output_tokens": 0, "server_tool_use": {"web_search_requests": 4, "web_fetch_requests": 2}}`,
			want:   100*4.0/1e6 + 0.04,
			tokens: Tokens{Input: 100},
		},
		{
			name: "a fallback hop bills at its own model's rates", model: "claude-opus-5-5",
			raw: `{"input_tokens": 412, "output_tokens": 264, "iterations": [
				{"type": "message", "model": "claude-opus-5-5", "input_tokens": 535, "output_tokens": 0, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0},
				{"type": "fallback_message", "model": "claude-opus-4-8", "input_tokens": 412, "output_tokens": 264, "cache_read_input_tokens": 1000, "cache_creation_input_tokens": 0}]}`,
			want:   535*4.0/1e6 + (412*5.0+264*25.0+1000*0.50)/1e6,
			tokens: Tokens{Input: 947, Output: 264, CacheRead: 1000},
		},
		{
			name: "sonnet falls back to sonnet 5", model: "claude-sonnet-5-5",
			raw: `{"input_tokens": 10, "output_tokens": 10, "iterations": [
				{"type": "message", "model": "claude-sonnet-5-5", "input_tokens": 1000, "output_tokens": 100, "cache_read_input_tokens": 10000},
				{"type": "fallback_message", "model": "claude-sonnet-5", "input_tokens": 1000, "output_tokens": 100, "cache_read_input_tokens": 10000}]}`,
			want:   (1000*2.0+100*10.0+10000*0.10)/1e6 + (1000*2.0+100*10.0+10000*0.20)/1e6,
			tokens: Tokens{Input: 2000, Output: 200, CacheRead: 20000},
		},
		{
			name: "a server tool loop's iterations, and a compaction on the requested model", model: "claude-opus-5-5",
			raw: `{"input_tokens": 3000, "output_tokens": 300, "server_tool_use": {"web_search_requests": 2}, "iterations": [
				{"type": "message", "model": "claude-opus-5-5", "input_tokens": 1000, "output_tokens": 100},
				{"type": "message", "model": "claude-opus-5-5", "input_tokens": 2000, "output_tokens": 200},
				{"type": "compaction", "input_tokens": 500, "output_tokens": 50}]}`,
			want:   (3500*4.0+350*20.0)/1e6 + 0.02,
			tokens: Tokens{Input: 3500, Output: 350},
		},
		{
			name: "an unknown model is priced at the requested model's card, not at $0", model: "claude-opus-5-5",
			raw: `{"input_tokens": 0, "output_tokens": 0, "iterations": [
				{"type": "fallback_message", "model": "claude-mystery-9", "input_tokens": 1000000, "output_tokens": 0}]}`,
			want:     4.00,
			tokens:   Tokens{Input: 1_000_000},
			unpriced: []string{"claude-mystery-9"},
		},
		{
			name: "an unpriced requested model falls back to the dearest card", model: "claude-mystery-9",
			raw:      `{"input_tokens": 1000000, "output_tokens": 0}`,
			want:     5.00,
			tokens:   Tokens{Input: 1_000_000},
			unpriced: []string{"claude-mystery-9"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, unpriced := priceUsage(tt.model, usage(t, tt.raw))
			if math.Abs(got.CostUSD-tt.want) > 1e-9 {
				t.Errorf("cost = %.9f, want %.9f", got.CostUSD, tt.want)
			}
			if got.Tokens != tt.tokens || got.Model != tt.model || got.Calls != 1 {
				t.Errorf("usage = %+v, want tokens %+v", got, tt.tokens)
			}
			if !slices.Equal(unpriced, tt.unpriced) {
				t.Errorf("unpriced = %v, want %v", unpriced, tt.unpriced)
			}
		})
	}
}

func TestUsageAdd(t *testing.T) {
	a := Usage{Model: "claude-opus-5-5", Tokens: Tokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4, CacheWrite1h: 1}, WebSearches: 1, CostUSD: 0.5, Calls: 1}
	b := Usage{Model: "other", Tokens: Tokens{Input: 10, Output: 20}, WebFetches: 2, CostUSD: 0.25, Calls: 2}
	got := Usage{}.Add(a).Add(b)
	want := Usage{Model: "claude-opus-5-5", Tokens: Tokens{Input: 11, Output: 22, CacheRead: 3, CacheWrite: 4, CacheWrite1h: 1},
		WebSearches: 1, WebFetches: 2, CostUSD: 0.75, Calls: 3}
	if got != want {
		t.Errorf("Add = %+v, want %+v", got, want)
	}
	if got.Prompt() != 18 || got.Total() != 40 {
		t.Errorf("prompt %d, total %d", got.Prompt(), got.Total())
	}
}
