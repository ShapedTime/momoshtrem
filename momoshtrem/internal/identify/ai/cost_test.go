package ai

import (
	"math"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

func TestCostUSD(t *testing.T) {
	tests := []struct {
		name string
		u    identify.Usage
		want float64
	}{
		{"haiku, input and output", identify.Usage{Model: "claude-haiku-5-5", InputTokens: 10_000, OutputTokens: 10_000}, (10_000*0.10 + 10_000*0.50) / 1e6},
		{"haiku, cache reads and writes", identify.Usage{Model: "claude-haiku-5-5", CacheReadTokens: 50_000, CacheWriteTokens: 50_000}, (50_000*0.01 + 50_000*0.125) / 1e6},
		{"haiku, a prompt of exactly 100K tokens", identify.Usage{Model: "claude-haiku-5-5", InputTokens: 60_000, CacheReadTokens: 40_000, OutputTokens: 1000},
			(60_000*0.10 + 40_000*0.01 + 1000*0.50) / 1e6},
		{"haiku, a prompt over 100K tokens costs more, output too", identify.Usage{Model: "claude-haiku-5-5", InputTokens: 60_000, CacheReadTokens: 40_001, OutputTokens: 1000},
			(60_000*0.50 + 40_001*0.05 + 1000*2.50) / 1e6},
		{"sonnet", identify.Usage{Model: "claude-sonnet-5-5", InputTokens: 1_000_000, OutputTokens: 100_000}, 2.00 + 1.00},
		{"opus", identify.Usage{Model: "claude-opus-5-5", InputTokens: 100_000, CacheReadTokens: 1_000_000}, 0.40 + 0.20},
		{"a model without a price", identify.Usage{Model: "other", InputTokens: 1_000_000}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := costUSD(tt.u); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("cost = %.9f, want %.9f", got, tt.want)
			}
		})
	}
}

func TestUsageOf(t *testing.T) {
	u := usageOf(DefaultModel, anthropic.Usage{InputTokens: 10, OutputTokens: 20, CacheReadInputTokens: 30, CacheCreationInputTokens: 40})
	want := identify.Usage{Model: DefaultModel, InputTokens: 10, OutputTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 40}
	want.CostUSD = costUSD(want)
	if u != want || u.CostUSD == 0 {
		t.Errorf("usageOf = %+v, want %+v", u, want)
	}
}

func TestEveryPricedModelHasAFullCard(t *testing.T) {
	for model, cards := range prices {
		if len(cards) == 0 || cards[len(cards)-1].upTo != 0 {
			t.Errorf("%s: the last card must apply to any length", model)
		}
		for _, c := range cards {
			if c.input <= 0 || c.output <= 0 || c.cacheRead <= 0 || c.cacheWrite <= 0 {
				t.Errorf("%s: a rate is missing: %+v", model, c)
			}
		}
	}
}
