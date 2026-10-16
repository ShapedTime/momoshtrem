package aicore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// CountTokens is the prompt of req's first call, in tokens: POST
// /v1/messages/count_tokens, which is free (recommend-run -dry-run). The
// request is Run's first call (the tools, the system prompt, thinking,
// effort and the messages) less the refusal fallbacks, which
// count_tokens doesn't take. Its errors are ClassifyError's.
func (c *Client) CountTokens(ctx context.Context, req Request) (int64, error) {
	r := &run{c: c, req: req, res: &Result{}}
	if err := r.prepare(); err != nil {
		return 0, err
	}
	p := r.params()
	list := make([]anthropic.BetaMessageCountTokensParamsToolUnion, len(p.Tools))
	for i, t := range p.Tools {
		raw, err := json.Marshal(t)
		if err != nil {
			return 0, fmt.Errorf("aicore: encode a tool: %w", err)
		}
		list[i] = param.Override[anthropic.BetaMessageCountTokensParamsToolUnion](json.RawMessage(raw))
	}
	params := anthropic.BetaMessageCountTokensParams{
		Model:        p.Model,
		Messages:     p.Messages,
		Tools:        list,
		System:       anthropic.BetaMessageCountTokensParamsSystemUnion{OfBetaTextBlockArray: p.System},
		Thinking:     p.Thinking,
		OutputConfig: p.OutputConfig,
		CacheControl: p.CacheControl,
	}
	if req.DropMismatchedThinking {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaThinkingBindingControls2026_08_01}
	}
	res, err := c.api.Beta.Messages.CountTokens(ctx, params)
	if err != nil {
		return 0, ClassifyError(ctx, err)
	}
	return res.InputTokens, nil
}

// RequestBody is the JSON body of req's first call as Run sends it, for
// its size without a key (recommend-run -dry-run).
func RequestBody(req Request) ([]byte, error) {
	r := &run{req: req, res: &Result{}}
	if err := r.prepare(); err != nil {
		return nil, err
	}
	return json.Marshal(r.params())
}
