package aicore

import (
	"log/slog"
	"regexp"
	"slices"

	"github.com/anthropics/anthropic-sdk-go"
)

// card is a price card in US dollars per million tokens. A card applies to
// prompts of up to upTo tokens (input, cache writes and cache reads
// together); 0 means any length.
type card struct {
	upTo                                                 int64
	input, output, cacheRead, cacheWrite5m, cacheWrite1h float64
}

// prices are the models AI features run on, plus the models a server-side
// refusal fallback may hand a request to (fallbacks: "default" picks them
// by refusal category: Claude Opus 5.5 falls back to claude-opus-5 or
// claude-opus-4-8, Claude Sonnet 5.5 to claude-sonnet-5). Checked against
// the pricing page (platform.claude.com/docs/en/about-claude/pricing) on
// 2026-10-16. Claude Haiku 5.5 has a second card for prompts over 100K
// tokens; Claude Opus 5.5 and Claude Sonnet 5.5 read the cache at 0.05x
// their input price, the others at 0.1x. Cache writes are 1.25x input for
// the 5-minute TTL and 2x for the 1-hour TTL.
var prices = map[string][]card{
	"claude-haiku-5-5": {
		{upTo: 100_000, input: 0.10, output: 0.50, cacheRead: 0.01, cacheWrite5m: 0.125, cacheWrite1h: 0.20},
		{input: 0.50, output: 2.50, cacheRead: 0.05, cacheWrite5m: 0.625, cacheWrite1h: 1.00},
	},
	"claude-sonnet-5-5": {{input: 2.00, output: 10.00, cacheRead: 0.10, cacheWrite5m: 2.50, cacheWrite1h: 4.00}},
	"claude-sonnet-5":   {{input: 2.00, output: 10.00, cacheRead: 0.20, cacheWrite5m: 2.50, cacheWrite1h: 4.00}},
	"claude-opus-5-5":   {{input: 4.00, output: 20.00, cacheRead: 0.20, cacheWrite5m: 5.00, cacheWrite1h: 8.00}},
	"claude-opus-5":     {{input: 5.00, output: 25.00, cacheRead: 0.50, cacheWrite5m: 6.25, cacheWrite1h: 10.00}},
	"claude-opus-4-8":   {{input: 5.00, output: 25.00, cacheRead: 0.50, cacheWrite5m: 6.25, cacheWrite1h: 10.00}},
}

// WebSearchUSD is what one web search costs on top of its tokens ($10 per
// 1,000). Web fetch has no surcharge.
const WebSearchUSD = 0.01

// datedSuffix is a snapshot date some APIs append to a model id
// ("claude-opus-4-8-20260501"); the price is the model's.
var datedSuffix = regexp.MustCompile(`-\d{8}$`)

// cards finds a model's price cards.
func cards(model string) ([]card, bool) {
	if c, ok := prices[model]; ok {
		return c, true
	}
	c, ok := prices[datedSuffix.ReplaceAllString(model, "")]
	return c, ok
}

// Priced reports whether a model has a price, so its calls can be counted
// against the budget. Features only call priced models.
func Priced(model string) bool {
	_, ok := cards(model)
	return ok
}

// Models lists the priced models, sorted.
func Models() []string {
	out := make([]string, 0, len(prices))
	for m := range prices {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// Tokens are one attempt's token counts, each billed at its own rate.
type Tokens struct {
	// Input is the prompt tokens not read from or written to the cache.
	Input int64
	// CacheWrite is every prompt token written to the cache; CacheWrite1h
	// is the part of it written with the 1-hour TTL (the rest is 5-minute).
	CacheWrite   int64
	CacheWrite1h int64
	CacheRead    int64
	// Output includes thinking.
	Output int64
}

// Prompt is the prompt's length: input, cache writes and cache reads.
func (t Tokens) Prompt() int64 { return t.Input + t.CacheWrite + t.CacheRead }

// Total is every token: the prompt and the output.
func (t Tokens) Total() int64 { return t.Prompt() + t.Output }

func (t Tokens) add(o Tokens) Tokens {
	t.Input += o.Input
	t.CacheWrite += o.CacheWrite
	t.CacheWrite1h += o.CacheWrite1h
	t.CacheRead += o.CacheRead
	t.Output += o.Output
	return t
}

// Cost prices tokens on a model's card for the prompt's length. ok is false
// when the model has no price (the cost is then 0).
func Cost(model string, t Tokens) (usd float64, ok bool) {
	cs, ok := cards(model)
	if !ok {
		return 0, false
	}
	c := cs[len(cs)-1]
	for _, cand := range cs {
		if cand.upTo == 0 || t.Prompt() <= cand.upTo {
			c = cand
			break
		}
	}
	write1h := min(t.CacheWrite1h, t.CacheWrite)
	const perToken = 1e-6
	return perToken * (float64(t.Input)*c.input + float64(t.Output)*c.output + float64(t.CacheRead)*c.cacheRead +
		float64(t.CacheWrite-write1h)*c.cacheWrite5m + float64(write1h)*c.cacheWrite1h), true
}

// fallbackCard is the card for a model nothing else prices: Claude Opus 5's,
// the dearest model a feature may end up on, so an unknown model is never
// free.
const fallbackCard = "claude-opus-5"

// costOn prices tokens on model's card, else on requested's, else on
// fallbackCard's; unpriced reports a model that had no card of its own.
func costOn(model, requested string, t Tokens) (usd float64, unpriced bool) {
	if usd, ok := Cost(model, t); ok {
		return usd, false
	}
	if usd, ok := Cost(requested, t); ok {
		return usd, true
	}
	usd, _ = Cost(fallbackCard, t)
	return usd, true
}

// PriceUsage reads one API call's usage and prices it. With
// usage.iterations (a server-side tool loop, a refusal fallback, a
// compaction), each entry is priced on its own model's card: a fallback hop
// bills at the fallback model's rates, and the tokens are the entries'
// sums. A declined attempt is priced too, although a decline before any
// output is billed only in some refusal categories: the budget errs on the
// safe side. Without iterations the top-level counts are priced on the
// requested model. A model without a price is priced on the requested
// model's card, with a warning, never at $0. Web searches add WebSearchUSD
// each.
func PriceUsage(requested string, u anthropic.BetaUsage) Usage {
	out, unpriced := priceUsage(requested, u)
	for _, m := range unpriced {
		slog.Warn("No price for an AI model; priced at the requested model's card",
			"component", "aicore", "model", m, "requested", requested)
	}
	return out
}

func priceUsage(requested string, u anthropic.BetaUsage) (Usage, []string) {
	out := Usage{
		Model:       requested,
		WebSearches: u.ServerToolUse.WebSearchRequests,
		WebFetches:  u.ServerToolUse.WebFetchRequests,
		Calls:       1,
	}
	var unpriced []string
	if len(u.Iterations) == 0 {
		out.Tokens = Tokens{
			Input: u.InputTokens, CacheWrite: u.CacheCreationInputTokens,
			CacheWrite1h: u.CacheCreation.Ephemeral1hInputTokens,
			CacheRead:    u.CacheReadInputTokens, Output: u.OutputTokens,
		}
		usd, missing := costOn(requested, requested, out.Tokens)
		if missing {
			unpriced = append(unpriced, requested)
		}
		out.CostUSD = usd
	}
	for _, it := range u.Iterations {
		t := Tokens{
			Input: it.InputTokens, CacheWrite: it.CacheCreationInputTokens,
			CacheWrite1h: it.CacheCreation.Ephemeral1hInputTokens,
			CacheRead:    it.CacheReadInputTokens, Output: it.OutputTokens,
		}
		model := string(it.Model)
		if model == "" { // a compaction runs on the requested model
			model = requested
		}
		usd, missing := costOn(model, requested, t)
		if missing && !slices.Contains(unpriced, model) {
			unpriced = append(unpriced, model)
		}
		out.Tokens = out.Tokens.add(t)
		out.CostUSD += usd
	}
	out.CostUSD += float64(out.WebSearches) * WebSearchUSD
	return out, unpriced
}
