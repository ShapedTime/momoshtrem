// Package aicore is the Claude core every AI feature shares (#154): the
// client, our streaming tool loop, the price table, usage recording and the
// monthly budget. The nightly picks (R2) and Ask (R3) run on its loop; the
// episode classifier (identify/ai) uses its prices, errors and ledger.
//
// # The loop
//
// Client.Run is our own streaming tool loop over the Messages API (beta),
// not the SDK's tool runner: in anthropic-sdk-go v1.81 the runner rewrites
// params.Tools, dropping strict, cache_control and server tools, and runs
// tools in parallel. Run:
//   - streams each response (Beta.Messages.NewStreaming, accumulated with
//     BetaMessage.Accumulate), calling the Hooks as blocks start and text
//     arrives;
//   - runs the custom tools one at a time, in block order, and sends all of
//     a response's tool_result blocks back in one user message;
//   - passes server tools (web_search_20260209, web_fetch_20260209) through,
//     and resumes a pause_turn by sending the paused turn back, with no
//     user message, at most MaxContinuations times;
//   - stops on end_turn, on refusal (ErrRefused), on max_tokens or the
//     context window (ErrTruncated), and on a response with no content
//     (ErrEmptyAnswer), which it never keeps: it couldn't be replayed;
//   - records every API call's usage (Recorder) and, after each call,
//     checks the run's caps (ErrCapReached; Request.AllowFinal can let one
//     last call past them end the turn);
//   - stops at once when ctx is cancelled, recording the usage the cut-off
//     call had reported.
//
// The request prefix is byte-stable across a run's calls: the tool
// definitions are encoded once, the system prompt is fixed, and messages
// are only appended, each kept as the raw JSON first sent. Run returns the
// messages it appended as raw blocks, so a caller can store them and replay
// them unchanged (Ask's threads).
//
// # Requests
//
// Adaptive thinking at an explicit effort (medium by default: Opus 5.5's
// default is medium, Sonnet 5.5's high, and neither can turn thinking off);
// server-side refusal fallbacks (fallbacks: "default", beta
// server-side-fallback-2026-07-01); cache_control on the system block and
// top-level automatic caching; optionally thinking.block_binding
// drop_block (beta thinking-binding-controls-2026-08-01) for a history
// replayed across turns. tool_choice is never sent: a forced choice is a
// 400 on Opus 5.5 and Sonnet 5.5, so tools are auto, strict, and steered by
// the prompt.
//
// The monthly budget isn't checked here: callers check it (Ledger.Check)
// before a run, an Ask turn or a classifier run.
package aicore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

const (
	// DefaultMaxTokens bounds one response: room for adaptive thinking and
	// the reply. Streaming keeps a long response clear of HTTP timeouts;
	// the run's caps bound the spend.
	DefaultMaxTokens = 64000
	// DefaultEffort is the effort a request sets when it names none.
	DefaultEffort = "medium"
	// MaxContinuations is how many times a run resumes a pause_turn.
	MaxContinuations = 5
	// recordTimeout bounds writing one call's usage.
	recordTimeout = 5 * time.Second
)

// Config configures a Client.
type Config struct {
	// APIKey is ANTHROPIC_API_KEY. Empty disables the AI features: New
	// returns nil.
	APIKey string
	// Recorder stores every API call's usage; nil records nothing.
	Recorder Recorder
	// Options are added to the SDK client's: the fake server's base URL in
	// tests (fake.Server.Options).
	Options []option.RequestOption
}

// Client runs the AI features' tool loops.
type Client struct {
	api      anthropic.Client
	recorder Recorder
	log      *slog.Logger
}

// New returns a client, or nil without an API key.
func New(cfg Config) *Client {
	if cfg.APIKey == "" {
		return nil
	}
	opts := append([]option.RequestOption{option.WithAPIKey(cfg.APIKey)}, cfg.Options...)
	return &Client{
		api:      anthropic.NewClient(opts...),
		recorder: cfg.Recorder,
		log:      slog.With("component", "aicore"),
	}
}

// Request is one run: a conversation that ends with the user's turn, the
// tools the model may call, and the limits.
type Request struct {
	// Model must be priced (Priced): claude-opus-5-5 or claude-sonnet-5-5
	// for runs and Ask.
	Model string
	// System is the system prompt. Keep it byte-stable: it's cached, after
	// the tools. Per-run facts (the date, the profile) go in a user message.
	System string
	// Tools are the custom tools, sent strict and in this order.
	Tools []Tool
	// WebSearchMaxUses and WebFetchMaxUses add the server-side
	// web_search_20260209 and web_fetch_20260209 tools with that max_uses
	// per call; 0 leaves them out.
	WebSearchMaxUses int
	WebFetchMaxUses  int
	// Messages are the conversation so far, ending with a user message:
	// replayed unchanged, so earlier turns' thinking blocks stay valid.
	Messages []Message
	// MaxTokens bounds each response; 0 means DefaultMaxTokens.
	MaxTokens int64
	// Effort is output_config.effort; "" means DefaultEffort.
	Effort string
	// DropMismatchedThinking sets thinking.block_binding to drop_block
	// (beta thinking-binding-controls-2026-08-01): a replayed thinking
	// block that fails the conversation check is dropped instead of failing
	// the request. For Ask, whose history comes back from the database.
	DropMismatchedThinking bool
	// Caps bound the run.
	Caps Caps
	// AllowFinal, when set, is asked once a cap is reached after a round of
	// tools: true lets the run make one more call, past the cap, so the
	// model can end a turn whose answer is already shown (Ask, after
	// show_picks). That call may only end the turn: if it asks for tools
	// again, the run stops with the cap's error before running them.
	AllowFinal func() bool
	// Hooks follow the run as it streams.
	Hooks Hooks
	// Feature and Ref label the ai_usage rows: library.AIFeatureRecs and
	// the run's id, or library.AIFeatureAsk and the turn's.
	Feature string
	Ref     int64
}

// Caps bound a run. They're checked after each API call, once its tools
// have run (they're paid for): when one is reached and the run would make
// another call, it stops with a *CapError. Zero means no cap.
type Caps struct {
	// Calls is how many API calls a run may make.
	Calls int
	// Tokens is how many tokens its calls may use: every prompt token
	// (cache reads and writes included) and every output token.
	Tokens int64
	// CostUSD is how much its calls may cost.
	CostUSD float64
	// ToolCalls is how many custom tool calls it may make.
	ToolCalls int
	// WebSearches is how many web searches its calls may make in all, as
	// usage counts them (max_uses bounds one call only).
	WebSearches int64
}

func (c Caps) reached(res *Result) error {
	switch {
	case c.Calls > 0 && res.Usage.Calls >= c.Calls:
		return &CapError{Cap: CapCalls}
	case c.Tokens > 0 && res.Usage.Total() >= c.Tokens:
		return &CapError{Cap: CapTokens}
	case c.CostUSD > 0 && res.Usage.CostUSD >= c.CostUSD:
		return &CapError{Cap: CapCost}
	case c.ToolCalls > 0 && res.ToolCalls >= c.ToolCalls:
		return &CapError{Cap: CapToolCalls}
	case c.WebSearches > 0 && res.Usage.WebSearches >= c.WebSearches:
		return &CapError{Cap: CapWebSearches}
	}
	return nil
}

// Hooks follow a run. Each is optional and runs on the run's goroutine, in
// the order things happen, so a caller can turn them into events in
// order. A hook blocks the stream while it runs: one that sends on a
// channel should select on the run's context too.
type Hooks struct {
	// BlockStart: a content block starts streaming. Server tools show up
	// here (Type "server_tool_use", Name "web_search"), as do text,
	// thinking, tool_use and fallback blocks.
	BlockStart func(Block)
	// Text: a piece of a text block, as it streams.
	Text func(delta string)
	// ToolStart: a custom tool is about to run.
	ToolStart func(ToolCall)
	// ToolResult: it ran; the result is what the model will see.
	ToolResult func(ToolCall, ToolResult)
	// Call: an API call ended (or was cut short), its usage recorded.
	Call func(CallInfo)
}

// Block is a content block as it starts.
type Block struct {
	// Call is the run's API call it's in (1, 2, …); Index its position in
	// that response.
	Call  int
	Index int
	// Type is the block's type: text, thinking, tool_use, server_tool_use,
	// web_search_tool_result, fallback, …
	Type string
	// Name is a tool call's tool ("get_candidates", "web_search"); ID its
	// id.
	Name string
	ID   string
}

// CallInfo is one API call, as it ended.
type CallInfo struct {
	// N is the call's number in the run (1, 2, …).
	N int
	// Model is the model that served it: a fallback's after a refusal.
	Model      string
	StopReason string
	// Usage is this call's; Total the run's so far.
	Usage Usage
	Total Usage
	// Err is why it failed, if it did.
	Err error
}

// Result is what a run did. Run returns it with or without an error: on an
// error it holds the usage spent and the messages appended so far.
type Result struct {
	// Messages are the messages the run appended, in order: each assistant
	// reply and the tool results that answered it. A refused or cut-off
	// reply isn't one. Only a run that returns no error ends on a complete
	// assistant turn, the one a caller may store and replay.
	Messages []Message
	// StopReason is the last response's stop_reason.
	StopReason string
	// Model is the model that served the last response.
	Model string
	// Usage sums the run's API calls.
	Usage Usage
	// ToolCalls counts the custom tool calls; Continuations the pause_turn
	// resumes.
	ToolCalls     int
	Continuations int
}

// Text is the final reply's text: the last assistant message's text
// blocks.
func (r *Result) Text() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == RoleAssistant {
			return r.Messages[i].Text()
		}
	}
	return ""
}

// run is one Run's state: the encoded request parts, which never change
// within it, and the history, which only grows.
type run struct {
	c     *Client
	req   Request
	tools []anthropic.BetaToolUnionParam
	named map[string]Tool
	wire  []anthropic.BetaMessageParam
	res   *Result
	// final is the cap the run went past for its one final call
	// (AllowFinal); nil before.
	final error
}

// Run runs a conversation to its end: until the model ends its turn, is
// refused, is cut off, answers nothing, a cap is reached, a call fails, or
// ctx is done. It always returns a Result (see Result).
//
// Its errors, for Ask's error codes: *RefusalError (refused); ErrTruncated
// and *CapError (limit_reached); *APIError and ErrEmptyAnswer (upstream);
// ctx's error (cancelled); a request Validate rejects, or a tool's own
// failure (internal).
func (c *Client) Run(ctx context.Context, req Request) (*Result, error) {
	r := &run{c: c, req: req, res: &Result{Usage: Usage{Model: req.Model}}}
	if err := r.prepare(); err != nil {
		return r.res, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return r.res, err
		}
		resp, err := r.call(ctx)
		if err != nil {
			return r.res, err
		}
		r.res.StopReason, r.res.Model = string(resp.StopReason), string(resp.Model)
		switch resp.StopReason {
		case anthropic.BetaStopReasonRefusal:
			return r.res, &RefusalError{Category: string(resp.StopDetails.Category), Model: string(resp.Model)}
		case anthropic.BetaStopReasonMaxTokens, anthropic.BetaStopReasonModelContextWindowExceeded:
			return r.res, fmt.Errorf("%w (%s)", ErrTruncated, resp.StopReason)
		}
		msg, kept, err := echo(resp)
		if err != nil {
			return r.res, err
		}
		if len(msg.Content) == 0 {
			// Never kept: an assistant message without content can't be
			// replayed, and a thread that stored it would 400 on every
			// later turn.
			return r.res, fmt.Errorf("%w (%s)", ErrEmptyAnswer, resp.StopReason)
		}
		if err := r.append(msg); err != nil {
			return r.res, err
		}
		switch resp.StopReason {
		case anthropic.BetaStopReasonPauseTurn:
			// The server paused its tool loop: send the turn back as it is,
			// and the API resumes from its trailing server_tool_use.
			if r.res.Continuations >= MaxContinuations {
				return r.res, &CapError{Cap: CapContinuations}
			}
			r.res.Continuations++
		case anthropic.BetaStopReasonToolUse:
			if r.final != nil {
				// The call past the cap may only end the turn.
				return r.res, r.final
			}
			results, err := r.runTools(ctx, kept)
			if err != nil {
				return r.res, err
			}
			if err := r.append(results); err != nil {
				return r.res, err
			}
		default: // end_turn, stop_sequence
			return r.res, nil
		}
		if err := r.req.Caps.reached(r.res); err != nil {
			if r.final != nil || r.req.AllowFinal == nil || !r.req.AllowFinal() {
				return r.res, err
			}
			r.final = err
		}
	}
}

// efforts are output_config.effort's values.
var efforts = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true}

// webToolModels are the priced models that take the _20260209 web search
// and fetch tools (Opus and Sonnet 4.6 and later; Haiku doesn't).
var webToolModels = map[string]bool{
	"claude-opus-5-5": true, "claude-sonnet-5-5": true,
	"claude-opus-5": true, "claude-opus-4-8": true, "claude-sonnet-5": true,
}

// Validate checks a request without calling the API: Run does too, but a
// caller can check first, before the budget check and before it starts a
// run or a turn. An error here is a bug in the caller, not the API's.
func (req Request) Validate() error {
	switch {
	case !Priced(req.Model):
		return fmt.Errorf("%w: %q", ErrUnknownModel, req.Model)
	case (req.WebSearchMaxUses > 0 || req.WebFetchMaxUses > 0) && !webToolModels[baseModel(req.Model)]:
		return fmt.Errorf("aicore: %s can't use the web search and fetch tools", req.Model)
	case req.Effort != "" && !efforts[req.Effort]:
		return fmt.Errorf("aicore: effort %q isn't low, medium, high, xhigh or max", req.Effort)
	case req.Feature == "":
		return errors.New("aicore: a run needs a feature for its usage")
	case len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Role != RoleUser:
		return errors.New("aicore: a run starts from a conversation that ends with a user message")
	}
	for i, m := range req.Messages {
		if len(m.Content) == 0 {
			return fmt.Errorf("aicore: message %d (%s) has no content", i, m.Role)
		}
	}
	seen := make(map[string]bool, len(req.Tools))
	for _, t := range req.Tools {
		name := t.Name()
		if seen[name] || name == "web_search" || name == "web_fetch" {
			return fmt.Errorf("aicore: tool %q is defined twice, or takes a server tool's name", name)
		}
		if !json.Valid(t.Schema()) {
			return fmt.Errorf("aicore: tool %q has an invalid schema", name)
		}
		seen[name] = true
	}
	return nil
}

// prepare checks the request and encodes its fixed parts.
func (r *run) prepare() error {
	req := &r.req
	if err := req.Validate(); err != nil {
		return err
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = DefaultMaxTokens
	}
	if req.Effort == "" {
		req.Effort = DefaultEffort
	}
	r.named = make(map[string]Tool, len(req.Tools))
	for _, t := range req.Tools {
		name := t.Name()
		r.named[name] = t
		def, err := json.Marshal(struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
			Strict      bool            `json:"strict"`
		}{name, t.Description(), t.Schema(), true})
		if err != nil {
			return fmt.Errorf("aicore: encode tool %q: %w", name, err)
		}
		r.tools = append(r.tools, param.Override[anthropic.BetaToolUnionParam](json.RawMessage(def)))
	}
	if req.WebSearchMaxUses > 0 {
		r.tools = append(r.tools, anthropic.BetaToolUnionParam{OfWebSearchTool20260209: &anthropic.BetaWebSearchTool20260209Param{
			MaxUses: anthropic.Int(int64(req.WebSearchMaxUses)),
		}})
	}
	if req.WebFetchMaxUses > 0 {
		r.tools = append(r.tools, anthropic.BetaToolUnionParam{OfWebFetchTool20260209: &anthropic.BetaWebFetchTool20260209Param{
			MaxUses: anthropic.Int(int64(req.WebFetchMaxUses)),
		}})
	}
	for _, m := range req.Messages {
		if err := r.encode(m); err != nil {
			return err
		}
	}
	return nil
}

// append adds a message the run made to the history and the result.
func (r *run) append(m Message) error {
	if err := r.encode(m); err != nil {
		return err
	}
	r.res.Messages = append(r.res.Messages, m)
	return nil
}

// encode adds a message to the wire history, as raw JSON encoded once.
func (r *run) encode(m Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("aicore: encode a %s message: %w", m.Role, err)
	}
	r.wire = append(r.wire, param.Override[anthropic.BetaMessageParam](json.RawMessage(raw)))
	return nil
}

// params is the next call's request.
func (r *run) params() anthropic.BetaMessageNewParams {
	req := &r.req
	adaptive := anthropic.BetaThinkingConfigAdaptiveParam{}
	betas := []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
	if req.DropMismatchedThinking {
		adaptive.BlockBinding = anthropic.BetaThinkingBlockBindingParam{
			PrefixMismatchBehavior: anthropic.BetaThinkingPrefixMismatchBehaviorDropBlock,
		}
		betas = append(betas, anthropic.AnthropicBetaThinkingBindingControls2026_08_01)
	}
	p := anthropic.BetaMessageNewParams{
		Model:        req.Model,
		MaxTokens:    req.MaxTokens,
		Messages:     r.wire,
		Tools:        r.tools,
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &adaptive},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(req.Effort)},
		Fallbacks:    anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		Betas:        betas,
	}
	if req.System != "" {
		p.System = []anthropic.BetaTextBlockParam{{Text: req.System, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}}
	}
	return p
}

// errStreamEnded: the stream closed before the message ended.
var errStreamEnded = errors.New("the response stream ended early")

// call makes one streamed API call, calling the hooks as it streams, and
// records its usage, even when it was cut short.
func (r *run) call(ctx context.Context) (*anthropic.BetaMessage, error) {
	n := r.res.Usage.Calls + 1
	hooks := r.req.Hooks
	start := time.Now()
	stream := r.c.api.Beta.Messages.NewStreaming(ctx, r.params())
	defer stream.Close()

	var msg anthropic.BetaMessage
	started := false
	var err error
	for stream.Next() {
		ev := stream.Current()
		if err = msg.Accumulate(ev); err != nil {
			break
		}
		switch ev.Type {
		case "message_start":
			started = true
		case "content_block_start":
			if hooks.BlockStart != nil {
				hooks.BlockStart(Block{Call: n, Index: int(ev.Index), Type: ev.ContentBlock.Type, Name: ev.ContentBlock.Name, ID: ev.ContentBlock.ID})
			}
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && hooks.Text != nil {
				hooks.Text(ev.Delta.Text)
			}
		}
	}
	if serr := stream.Err(); serr != nil {
		err = serr
	}
	if err == nil && msg.StopReason == "" {
		err = errStreamEnded
	}
	if err != nil {
		err = ClassifyError(ctx, err)
	}

	usage := Usage{Model: r.req.Model, Calls: 1}
	if started {
		// What the call reported so far: a cut-off call still spent its
		// prompt.
		usage = PriceUsage(r.req.Model, msg.Usage)
		if r.c.recorder != nil {
			// Recorded even when ctx is done (the tokens were spent), within
			// its own deadline.
			rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
			if rerr := r.c.recorder.Record(rctx, r.req.Feature, r.req.Ref, usage); rerr != nil {
				r.c.log.Warn("Failed to record AI usage", "error", rerr, "feature", r.req.Feature, "ref", r.req.Ref)
			}
			cancel()
		}
	}
	r.res.Usage = r.res.Usage.Add(usage)
	if dropped := droppedThinking(msg.InputTransformations); len(dropped) > 0 {
		// A replayed history whose prefix changed (or a model switch): the
		// API dropped those thinking blocks (drop_block) instead of failing.
		r.c.log.Warn("AI call: the API dropped replayed thinking blocks", "feature", r.req.Feature, "ref", r.req.Ref,
			"call", n, "blocks", len(dropped), "reasons", strings.Join(dropped, ","))
	}
	r.c.log.Info("AI call", "feature", r.req.Feature, "ref", r.req.Ref, "call", n,
		"model", r.req.Model, "served_by", msg.Model, "stop_reason", msg.StopReason,
		"input_tokens", usage.Input, "cache_write_tokens", usage.CacheWrite, "cache_read_tokens", usage.CacheRead,
		"output_tokens", usage.Output, "web_searches", usage.WebSearches, "cost_usd", usage.CostUSD,
		"duration", time.Since(start), "error", err)
	if hooks.Call != nil {
		hooks.Call(CallInfo{N: n, Model: string(msg.Model), StopReason: string(msg.StopReason), Usage: usage, Total: r.res.Usage, Err: err})
	}
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// droppedThinking is why each thinking block the API dropped from the
// request was dropped (input_transformations' thinking_dropped entries:
// prefix_binding_mismatch, model_binding_mismatch, …).
func droppedThinking(ts []anthropic.BetaInputTransformationUnion) []string {
	var reasons []string
	for _, t := range ts {
		if t.Type == "thinking_dropped" {
			reasons = append(reasons, t.Reason)
		}
	}
	return reasons
}

// runTools runs a response's custom tool calls one at a time, in block
// order, and returns the user message with their results.
func (r *run) runTools(ctx context.Context, blocks []anthropic.BetaContentBlockUnion) (Message, error) {
	hooks := r.req.Hooks
	out := Message{Role: RoleUser}
	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Message{}, err
		}
		call := ToolCall{ID: b.ID, Name: b.Name, Input: b.Input}
		if hooks.ToolStart != nil {
			hooks.ToolStart(call)
		}
		result, err := r.runTool(ctx, call)
		if err != nil {
			return Message{}, err
		}
		r.res.ToolCalls++
		if hooks.ToolResult != nil {
			hooks.ToolResult(call, result)
		}
		out.Content = append(out.Content, mustMarshal(toolResultBlock{
			Type: "tool_result", ToolUseID: call.ID, Content: result.content(), IsError: result.IsError,
		}))
	}
	if len(out.Content) == 0 {
		return Message{}, errors.New("aicore: the model stopped for tools but called none")
	}
	return out, nil
}

func (r *run) runTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	tool, ok := r.named[call.Name]
	if !ok {
		return ToolResult{IsError: true, Message: fmt.Sprintf("There's no tool named %q.", call.Name)}, nil
	}
	output, err := tool.Run(ctx, call.Input)
	var toolErr *ToolError
	switch {
	case err == nil:
		return ToolResult{Output: output}, nil
	case errors.As(err, &toolErr):
		msg := strings.TrimSpace(toolErr.Message)
		if msg == "" {
			msg = "The tool failed."
		}
		return ToolResult{IsError: true, Message: msg}, nil
	case ctx.Err() != nil:
		return ToolResult{}, ctx.Err()
	}
	return ToolResult{}, fmt.Errorf("tool %s: %w", call.Name, err)
}
