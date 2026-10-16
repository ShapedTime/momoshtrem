package ai

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/identify"
)

// costUSD prices a usage on its model's card for the prompt's length, from
// the AI core's price table. A model without a price can't be configured
// (New and Resolve refuse it), so every call has a cost.
func costUSD(u identify.Usage) float64 {
	cost, _ := aicore.Cost(u.Model, aicore.Tokens{
		Input:      int64(u.InputTokens),
		CacheWrite: int64(u.CacheWriteTokens),
		CacheRead:  int64(u.CacheReadTokens),
		Output:     int64(u.OutputTokens),
	})
	return cost
}

// usageOf reads a response's usage and prices it.
func usageOf(model string, u anthropic.Usage) identify.Usage {
	out := identify.Usage{
		Model:            model,
		InputTokens:      int(u.InputTokens),
		OutputTokens:     int(u.OutputTokens),
		CacheReadTokens:  int(u.CacheReadInputTokens),
		CacheWriteTokens: int(u.CacheCreationInputTokens),
	}
	out.CostUSD = costUSD(out)
	return out
}
