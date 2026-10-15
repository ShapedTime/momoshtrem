package ai

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

// rate is a price card in US dollars per million tokens. A card applies to
// prompts of up to upTo tokens (input, cache writes and cache reads
// together); 0 means any length. Cache writes use the 5-minute TTL rate,
// 1.25x input.
type rate struct {
	upTo                                 int
	input, output, cacheRead, cacheWrite float64
}

// prices are the models the classifier may run on, from the claude-api
// skill's model table (cached 2026-10-06). Claude Haiku 5.5 has a second
// card for prompts over 100K tokens. A model outside this table can't be
// configured (New fails), so every call has a cost.
var prices = map[string][]rate{
	"claude-haiku-5-5": {
		{upTo: 100_000, input: 0.10, output: 0.50, cacheRead: 0.01, cacheWrite: 0.125},
		{input: 0.50, output: 2.50, cacheRead: 0.05, cacheWrite: 0.625},
	},
	"claude-sonnet-5-5": {{input: 2.00, output: 10.00, cacheRead: 0.20, cacheWrite: 2.50}},
	"claude-opus-5-5":   {{input: 4.00, output: 20.00, cacheRead: 0.20, cacheWrite: 5.00}},
}

// costUSD prices a usage on its model's card for the prompt's length.
func costUSD(u identify.Usage) float64 {
	cards, ok := prices[u.Model]
	if !ok {
		return 0
	}
	prompt := u.InputTokens + u.CacheWriteTokens + u.CacheReadTokens
	card := cards[len(cards)-1]
	for _, c := range cards {
		if c.upTo == 0 || prompt <= c.upTo {
			card = c
			break
		}
	}
	const perToken = 1e-6
	return perToken * (float64(u.InputTokens)*card.input + float64(u.OutputTokens)*card.output +
		float64(u.CacheReadTokens)*card.cacheRead + float64(u.CacheWriteTokens)*card.cacheWrite)
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
