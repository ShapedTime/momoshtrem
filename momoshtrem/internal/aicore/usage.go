package aicore

// Usage is what one or more API calls used and cost.
type Usage struct {
	// Model is the model asked for (ai_usage.model). A refusal fallback's
	// tokens count here too, priced on the fallback model's card.
	Model string
	Tokens
	// WebSearches cost WebSearchUSD each; web fetches are free.
	WebSearches int64
	WebFetches  int64
	// CostUSD prices every token at its rate, plus the web searches.
	CostUSD float64
	// Calls is how many API calls this is.
	Calls int
}

// Add sums two usages; the model is the first non-empty one.
func (u Usage) Add(o Usage) Usage {
	if u.Model == "" {
		u.Model = o.Model
	}
	u.Tokens = u.Tokens.add(o.Tokens)
	u.WebSearches += o.WebSearches
	u.WebFetches += o.WebFetches
	u.CostUSD += o.CostUSD
	u.Calls += o.Calls
	return u
}
