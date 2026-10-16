package aicore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
)

// recorder keeps the usage rows a run records.
type recorder struct {
	mu   sync.Mutex
	rows []row
}

type row struct {
	feature string
	ref     int64
	usage   aicore.Usage
}

func (r *recorder) Record(_ context.Context, feature string, ref int64, u aicore.Usage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, row{feature, ref, u})
	return nil
}

// tool is a custom tool that answers from a function and logs its calls.
type tool struct {
	name string
	run  func(input json.RawMessage) (json.RawMessage, error)
	log  *[]string
}

func (t tool) Name() string        { return t.name }
func (t tool) Description() string { return "Test tool " + t.name + "." }
func (t tool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`)
}

func (t tool) Run(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
	if t.log != nil {
		*t.log = append(*t.log, "run "+t.name+" "+string(input))
	}
	if t.run != nil {
		return t.run(input)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func newClient(t *testing.T, srv *fake.Server, rec *recorder) *aicore.Client {
	t.Helper()
	cfg := aicore.Config{APIKey: "test-key", Options: srv.Options()}
	if rec != nil {
		cfg.Recorder = rec
	}
	return aicore.New(cfg)
}

func request(tools ...aicore.Tool) aicore.Request {
	return aicore.Request{
		Model:    "claude-opus-5-5",
		System:   "You recommend films.",
		Tools:    tools,
		Messages: []aicore.Message{aicore.UserText("Something like Severance?")},
		Feature:  "ask",
		Ref:      7,
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRun_ToolsRunOneAtATimeInBlockOrder(t *testing.T) {
	srv := fake.New(t,
		fake.Response{
			Blocks: []fake.Block{
				fake.Thinking("sig_1"),
				fake.Text("Let me look. "),
				fake.ToolUse("toolu_a", "alpha", map[string]any{"q": "first"}),
				fake.ToolUse("toolu_b", "beta", map[string]any{"q": "second"}),
			},
			Usage: fake.Usage{InputTokens: 1000, OutputTokens: 100},
		},
		fake.Response{Blocks: []fake.Block{fake.Text("Try Severance's cousin.")}, Usage: fake.Usage{InputTokens: 1200, OutputTokens: 50}},
	)
	rec := &recorder{}
	var events []string
	hooks := aicore.Hooks{
		BlockStart: func(b aicore.Block) {
			events = append(events, fmt.Sprintf("block %d.%d %s %s", b.Call, b.Index, b.Type, b.Name))
		},
		Text:      func(d string) { events = append(events, "text "+d) },
		ToolStart: func(c aicore.ToolCall) { events = append(events, "start "+c.Name) },
		ToolResult: func(c aicore.ToolCall, r aicore.ToolResult) {
			events = append(events, fmt.Sprintf("result %s %s error=%v", c.Name, r.Output, r.IsError))
		},
		Call: func(c aicore.CallInfo) { events = append(events, fmt.Sprintf("call %d %s", c.N, c.StopReason)) },
	}
	req := request(tool{name: "alpha", log: &events}, tool{name: "beta", log: &events, run: func(json.RawMessage) (json.RawMessage, error) {
		return nil, &aicore.ToolError{Message: "No such title."}
	}})
	req.Hooks = hooks
	res, err := newClient(t, srv, rec).Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"block 1.0 thinking ", "block 1.1 text ", "text Let me ", "text look. ",
		"block 1.2 tool_use alpha", "block 1.3 tool_use beta", "call 1 tool_use",
		"start alpha", `run alpha {"q":"first"}`, `result alpha {"ok":true} error=false`,
		"start beta", `run beta {"q":"second"}`, "result beta  error=true",
		"block 2.0 text ", "text Try Severance's ", "text cousin.", "call 2 end_turn",
	}
	if !slices.Equal(events, want) {
		t.Errorf("events:\n%s\nwant:\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"))
	}

	// Both results go back in one user message, in block order.
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	msgs := reqs[1].Messages()
	var results struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error"`
		} `json:"content"`
	}
	if len(msgs) != 3 || json.Unmarshal(msgs[2], &results) != nil {
		t.Fatalf("second request's messages: %s", reqs[1].Field("messages"))
	}
	if results.Role != "user" || len(results.Content) != 2 ||
		results.Content[0].ToolUseID != "toolu_a" || results.Content[0].Content != `{"ok":true}` || results.Content[0].IsError ||
		results.Content[1].ToolUseID != "toolu_b" || results.Content[1].Content != "No such title." || !results.Content[1].IsError {
		t.Errorf("tool results = %s", msgs[2])
	}

	if res.ToolCalls != 2 || res.StopReason != "end_turn" || res.Text() != "Try Severance's cousin." || len(res.Messages) != 3 {
		t.Errorf("result = calls %d stop %s text %q messages %d", res.ToolCalls, res.StopReason, res.Text(), len(res.Messages))
	}
	// One usage row per call, labelled.
	if len(rec.rows) != 2 || rec.rows[0].feature != "ask" || rec.rows[0].ref != 7 || rec.rows[1].usage.Input != 1200 {
		t.Errorf("rows = %+v", rec.rows)
	}
	wantCost := (2200*4.0 + 150*20.0) / 1e6
	if res.Usage.Calls != 2 || !approx(res.Usage.CostUSD, wantCost) {
		t.Errorf("usage = %+v, want cost %v", res.Usage, wantCost)
	}
}

// The request: adaptive thinking at medium effort, the fallbacks beta and
// field, cached system block, top-level caching, strict tools, server tools
// with max_uses, and never a tool_choice.
func TestRun_RequestShape(t *testing.T) {
	srv := fake.New(t, fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})
	req := request(tool{name: "alpha"})
	req.WebSearchMaxUses, req.WebFetchMaxUses = 8, 4
	if _, err := newClient(t, srv, nil).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := srv.Requests()[0]
	if got.Header.Get("X-Api-Key") != "test-key" {
		t.Errorf("api key header = %q", got.Header.Get("X-Api-Key"))
	}
	if betas := got.Betas(); !slices.Equal(betas, []string{"server-side-fallback-2026-07-01"}) {
		t.Errorf("betas = %v", betas)
	}
	fields := got.Fields()
	checks := map[string]string{
		"model":         `"claude-opus-5-5"`,
		"max_tokens":    `64000`,
		"fallbacks":     `"default"`,
		"thinking":      `{"type":"adaptive"}`,
		"output_config": `{"effort":"medium"}`,
		"cache_control": `{"type":"ephemeral"}`,
		"system":        `[{"text":"You recommend films.","type":"text","cache_control":{"type":"ephemeral"}}]`,
	}
	for name, want := range checks {
		if !jsonEqual(fields[name], want) {
			t.Errorf("%s = %s, want %s", name, fields[name], want)
		}
	}
	if _, ok := fields["tool_choice"]; ok {
		t.Errorf("tool_choice sent: %s", fields["tool_choice"])
	}
	wantTools := `[
		{"name":"alpha","description":"Test tool alpha.","strict":true,
		 "input_schema":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}},
		{"type":"web_search_20260209","name":"web_search","max_uses":8},
		{"type":"web_fetch_20260209","name":"web_fetch","max_uses":4}]`
	if !jsonEqual(fields["tools"], wantTools) {
		t.Errorf("tools = %s", fields["tools"])
	}

	// Ask's drop_block adds its beta and the binding.
	srv = fake.New(t, fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})
	req = request()
	req.DropMismatchedThinking, req.Effort, req.MaxTokens = true, "low", 16000
	if _, err := newClient(t, srv, nil).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got = srv.Requests()[0]
	if betas := got.Betas(); !slices.Equal(betas, []string{"server-side-fallback-2026-07-01", "thinking-binding-controls-2026-08-01"}) {
		t.Errorf("betas = %v", betas)
	}
	if f := got.Fields(); !jsonEqual(f["thinking"], `{"type":"adaptive","block_binding":{"prefix_mismatch_behavior":"drop_block"}}`) ||
		!jsonEqual(f["output_config"], `{"effort":"low"}`) || string(f["max_tokens"]) != "16000" {
		t.Errorf("thinking = %s, output_config = %s, max_tokens = %s", f["thinking"], f["output_config"], f["max_tokens"])
	}
}

// Every call resends the same tools and system prompt, and the earlier
// messages, byte for byte: the cached prefix holds. The history a caller
// passes in is sent unchanged.
func TestRun_PrefixIsByteIdenticalAcrossCalls(t *testing.T) {
	srv := fake.New(t,
		fake.Response{Blocks: []fake.Block{fake.Thinking("sig_1"), fake.ToolUse("toolu_1", "alpha", map[string]any{"q": "a <b> & c"})}},
		fake.Response{Blocks: []fake.Block{fake.Thinking("sig_2"), fake.ToolUse("toolu_2", "alpha", map[string]any{"q": "ünïcode"})}},
		fake.Response{Blocks: []fake.Block{fake.Text("Done.")}},
	)
	earlier := aicore.Message{Role: "assistant", Content: []json.RawMessage{
		json.RawMessage(`{"type":"thinking","thinking":"","signature":"sig_old"}`),
		json.RawMessage(`{"type":"text","text":"Earlier answer with <tags> & “quotes”."}`),
	}}
	req := request(tool{name: "alpha"})
	req.WebSearchMaxUses = 3
	req.Messages = []aicore.Message{aicore.UserText("First ask"), earlier, aicore.UserText("A follow-up")}
	res, err := newClient(t, srv, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("%d requests", len(reqs))
	}
	for i := 1; i < len(reqs); i++ {
		prev, cur := reqs[i-1], reqs[i]
		for _, f := range []string{"tools", "system", "model", "thinking", "output_config", "fallbacks", "cache_control"} {
			if !bytes.Equal(prev.Field(f), cur.Field(f)) {
				t.Errorf("request %d's %s differs:\n%s\n%s", i+1, f, prev.Field(f), cur.Field(f))
			}
		}
		pm, cm := prev.Messages(), cur.Messages()
		if len(cm) != len(pm)+2 {
			t.Fatalf("request %d has %d messages after %d", i+1, len(cm), len(pm))
		}
		for j := range pm {
			if !bytes.Equal(pm[j], cm[j]) {
				t.Errorf("request %d's message %d changed:\n%s\n%s", i+1, j, pm[j], cm[j])
			}
		}
		// The prefix is the very same bytes in the body: messages start
		// where they did.
		if !bytes.HasPrefix(cur.Body, prev.Body[:bytes.Index(prev.Body, pm[len(pm)-1])]) {
			t.Errorf("request %d's body doesn't start with request %d's", i+1, i)
		}
	}
	// The history went out as given.
	first := reqs[0].Messages()
	if !jsonEqual(first[1], string(mustJSON(t, earlier))) {
		t.Errorf("replayed message = %s", first[1])
	}
	// The appended messages are what the next call sent.
	last := reqs[2].Messages()
	for i, m := range res.Messages[:4] {
		if !jsonEqual(last[3+i], string(mustJSON(t, m))) {
			t.Errorf("appended message %d = %s, sent %s", i, mustJSON(t, m), last[3+i])
		}
	}
	// Thinking blocks go back unchanged, signature included.
	if !strings.Contains(string(last[3]), `"signature":"sig_1"`) {
		t.Errorf("thinking block lost: %s", last[3])
	}
}

func TestRun_PauseTurnResumes(t *testing.T) {
	paused := fake.Response{
		Blocks: []fake.Block{
			fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "new thrillers 2026"}),
			fake.WebSearchResult("srvtoolu_1", "https://example.com/a"),
		},
		StopReason: "pause_turn",
		Usage:      fake.Usage{InputTokens: 500, OutputTokens: 40, WebSearchRequests: 1},
	}
	srv := fake.New(t, paused, fake.Response{Blocks: []fake.Block{fake.Text("Found three.")}})
	var blocks []string
	req := request()
	req.WebSearchMaxUses = 8
	req.Hooks.BlockStart = func(b aicore.Block) { blocks = append(blocks, b.Type+" "+b.Name) }
	res, err := newClient(t, srv, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 2 || res.Continuations != 1 {
		t.Fatalf("%d requests, %d continuations", len(reqs), res.Continuations)
	}
	// The paused turn goes back as it came, with no user message after it.
	msgs := reqs[1].Messages()
	var last struct {
		Role    string            `json:"role"`
		Content []json.RawMessage `json:"content"`
	}
	if len(msgs) != 2 || json.Unmarshal(msgs[1], &last) != nil || last.Role != "assistant" || len(last.Content) != 2 {
		t.Fatalf("resume messages = %s", reqs[1].Field("messages"))
	}
	if !strings.Contains(string(last.Content[0]), `"server_tool_use"`) || !strings.Contains(string(last.Content[1]), `"web_search_tool_result"`) {
		t.Errorf("paused turn = %s", msgs[1])
	}
	if !slices.Equal(blocks, []string{"server_tool_use web_search", "web_search_tool_result ", "text "}) {
		t.Errorf("blocks = %v", blocks)
	}
	if res.Text() != "Found three." || res.Usage.WebSearches != 1 {
		t.Errorf("result = %q, %d searches", res.Text(), res.Usage.WebSearches)
	}

	// At most MaxContinuations resumes.
	var script []fake.Response
	for range aicore.MaxContinuations + 1 {
		script = append(script, paused)
	}
	srv = fake.New(t, script...)
	res, err = newClient(t, srv, nil).Run(context.Background(), req)
	var capErr *aicore.CapError
	if !errors.As(err, &capErr) || capErr.Cap != aicore.CapContinuations || !errors.Is(err, aicore.ErrCapReached) {
		t.Fatalf("err = %v, want the continuations cap", err)
	}
	if res.Continuations != aicore.MaxContinuations || len(srv.Requests()) != aicore.MaxContinuations+1 {
		t.Errorf("%d continuations, %d requests", res.Continuations, len(srv.Requests()))
	}
}

func TestRun_StopsOnRefusalAndMaxTokens(t *testing.T) {
	srv := fake.New(t,
		fake.Response{Blocks: []fake.Block{fake.Text("Partial")}, StopReason: "refusal", RefusalCategory: "cyber",
			Usage: fake.Usage{InputTokens: 300, OutputTokens: 5}},
	)
	rec := &recorder{}
	req := request(tool{name: "alpha"})
	res, err := newClient(t, srv, rec).Run(context.Background(), req)
	var refusal *aicore.RefusalError
	if !errors.As(err, &refusal) || refusal.Category != "cyber" || !errors.Is(err, aicore.ErrRefused) {
		t.Fatalf("err = %v, want a cyber refusal", err)
	}
	if len(res.Messages) != 0 || len(rec.rows) != 1 || res.Usage.Input != 300 {
		t.Errorf("a refusal appended %d messages, recorded %d rows, usage %+v", len(res.Messages), len(rec.rows), res.Usage)
	}

	srv = fake.New(t,
		fake.Response{Blocks: []fake.Block{fake.ToolUse("toolu_1", "alpha", map[string]any{"q": "x"})}, StopReason: "max_tokens"},
	)
	var ran []string
	res, err = newClient(t, srv, nil).Run(context.Background(), request(tool{name: "alpha", log: &ran}))
	if !errors.Is(err, aicore.ErrTruncated) || len(ran) != 0 || len(res.Messages) != 0 {
		t.Errorf("max_tokens: err %v, ran %v, %d messages", err, ran, len(res.Messages))
	}
}

// A refusal fallback: every usage.iterations hop is priced on its own
// model's card, and the blocks before the handoff the next model can't
// take aren't echoed (nor their tool calls run).
func TestRun_FallbackHopPricedOnItsOwnModel(t *testing.T) {
	srv := fake.New(t,
		fake.Response{
			Blocks: []fake.Block{
				fake.Thinking("sig_declined"),
				fake.Text("Partial "),
				fake.ToolUse("toolu_x", "alpha", map[string]any{"q": "declined"}),
				fake.Fallback("claude-opus-5-5", "claude-opus-4-8"),
				fake.Thinking("sig_fallback"),
				fake.ToolUse("toolu_y", "alpha", map[string]any{"q": "kept"}),
			},
			Usage: fake.Usage{InputTokens: 900, OutputTokens: 80, Iterations: []fake.Iteration{
				{Model: "claude-opus-5-5", InputTokens: 1000, OutputTokens: 30},
				{Type: "fallback_message", Model: "claude-opus-4-8", InputTokens: 900, OutputTokens: 80, CacheReadInputTokens: 2000},
			}},
		},
		fake.Response{Model: "claude-opus-4-8", Blocks: []fake.Block{fake.Text("Served by the fallback.")}},
	)
	var ran []string
	var calls []aicore.CallInfo
	req := request(tool{name: "alpha", log: &ran})
	req.Hooks.Call = func(c aicore.CallInfo) { calls = append(calls, c) }
	res, err := newClient(t, srv, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{`run alpha {"q":"kept"}`}) {
		t.Errorf("ran %v, want only the call after the handoff", ran)
	}
	want := (1000*4.0+30*20.0)/1e6 + (900*5.0+80*25.0+2000*0.50)/1e6
	if !approx(calls[0].Usage.CostUSD, want) {
		t.Errorf("first call cost %.9f, want %.9f", calls[0].Usage.CostUSD, want)
	}
	if calls[0].Usage.Input != 1900 || calls[0].Usage.Output != 110 || calls[0].Usage.CacheRead != 2000 || calls[0].Model != "claude-opus-4-8" {
		t.Errorf("first call = %+v", calls[0])
	}
	var echoed struct {
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Signature string `json:"signature"`
			ID        string `json:"id"`
		} `json:"content"`
	}
	if err := json.Unmarshal(srv.Requests()[1].Messages()[1], &echoed); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, b := range echoed.Content {
		kinds = append(kinds, b.Type+":"+b.Text+b.Signature+b.ID)
	}
	if !slices.Equal(kinds, []string{"text:Partial ", "fallback:", "thinking:sig_fallback", "tool_use:toolu_y"}) {
		t.Errorf("echoed = %v", kinds)
	}
	if res.Model != "claude-opus-4-8" {
		t.Errorf("model = %s", res.Model)
	}
}

func TestRun_WebSearchesArePriced(t *testing.T) {
	srv := fake.New(t, fake.Response{
		Blocks: []fake.Block{
			fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "a"}),
			fake.WebSearchResult("srvtoolu_1", "https://example.com"),
			fake.Text("Done."),
		},
		Usage: fake.Usage{InputTokens: 10_000, OutputTokens: 500, WebSearchRequests: 3, WebFetchRequests: 2},
	})
	req := request()
	req.Model = "claude-sonnet-5-5"
	req.WebSearchMaxUses, req.WebFetchMaxUses = 8, 4
	res, err := newClient(t, srv, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := (10_000*2.0+500*10.0)/1e6 + 3*0.01
	if !approx(res.Usage.CostUSD, want) || res.Usage.WebSearches != 3 || res.Usage.WebFetches != 2 {
		t.Errorf("usage = %+v, want cost %v", res.Usage, want)
	}
}

func TestRun_CapsHold(t *testing.T) {
	loop := func() fake.Response {
		return fake.Response{
			Blocks: []fake.Block{fake.ToolUse("toolu_1", "alpha", map[string]any{"q": "again"})},
			Usage:  fake.Usage{InputTokens: 10_000, OutputTokens: 1000},
		}
	}
	// Each call: 11,000 tokens, $0.06 on Opus 5.5.
	for _, tt := range []struct {
		name  string
		caps  aicore.Caps
		cap   string
		calls int
	}{
		{"calls", aicore.Caps{Calls: 2}, aicore.CapCalls, 2},
		{"tokens", aicore.Caps{Tokens: 30_000}, aicore.CapTokens, 3},
		{"cost", aicore.Caps{CostUSD: 0.10}, aicore.CapCost, 2},
		{"tool calls", aicore.Caps{ToolCalls: 1}, aicore.CapToolCalls, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := fake.New(t, loop(), loop(), loop(), loop())
			var ran []string
			req := request(tool{name: "alpha", log: &ran})
			req.Caps = tt.caps
			res, err := newClient(t, srv, nil).Run(context.Background(), req)
			var capErr *aicore.CapError
			if !errors.As(err, &capErr) || capErr.Cap != tt.cap {
				t.Fatalf("err = %v, want cap %s", err, tt.cap)
			}
			if n := len(srv.Requests()); n != tt.calls || res.Usage.Calls != tt.calls {
				t.Errorf("%d requests, %d calls; want %d", n, res.Usage.Calls, tt.calls)
			}
			// The last call's tools ran (they were paid for), and the
			// messages end on their results.
			if len(ran) != tt.calls || res.Messages[len(res.Messages)-1].Role != "user" {
				t.Errorf("ran %d tools, last message %s", len(ran), res.Messages[len(res.Messages)-1].Role)
			}
		})
	}
	// A run that ends within its caps doesn't fail on them.
	srv := fake.New(t, fake.Response{Blocks: []fake.Block{fake.Text("Done.")}, Usage: fake.Usage{InputTokens: 50_000}})
	req := request()
	req.Caps = aicore.Caps{Calls: 1, Tokens: 10, CostUSD: 0.01}
	if _, err := newClient(t, srv, nil).Run(context.Background(), req); err != nil {
		t.Errorf("a finished run failed on its caps: %v", err)
	}
}

// AllowFinal lets one call past a cap end the turn (Ask, once its answer
// is shown); that call may not run tools, and without AllowFinal's yes the
// cap holds as before.
func TestRun_AllowFinal(t *testing.T) {
	again := func() fake.Response {
		return fake.Response{Blocks: []fake.Block{fake.ToolUse("toolu_1", "alpha", map[string]any{"q": "again"})}}
	}
	end := fake.Response{Blocks: []fake.Block{fake.Text("Enjoy.")}}
	for _, tt := range []struct {
		name     string
		allow    bool
		script   []fake.Response
		wantErr  bool
		requests int
	}{
		{"ends the turn", true, []fake.Response{again(), end}, false, 2},
		{"no tools past the cap", true, []fake.Response{again(), again()}, true, 2},
		{"not allowed", false, []fake.Response{again(), end}, true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := fake.New(t, tt.script...)
			var ran []string
			req := request(tool{name: "alpha", log: &ran})
			req.Caps = aicore.Caps{Calls: 1}
			asked := 0
			req.AllowFinal = func() bool { asked++; return tt.allow }
			res, err := newClient(t, srv, nil).Run(context.Background(), req)
			var capErr *aicore.CapError
			switch {
			case tt.wantErr && (!errors.As(err, &capErr) || capErr.Cap != aicore.CapCalls):
				t.Errorf("err = %v, want the calls cap", err)
			case !tt.wantErr && (err != nil || res.Text() != "Enjoy."):
				t.Errorf("err = %v, text %q; want the turn ended", err, res.Text())
			}
			if n := len(srv.Requests()); n != tt.requests || len(ran) != 1 || asked != 1 {
				t.Errorf("%d requests, ran %v, asked %d times", n, ran, asked)
			}
		})
	}
}

// Cancelling stops the stream at once and records what the call had
// reported: its prompt.
func TestRun_CancelRecordsPartialUsage(t *testing.T) {
	srv := fake.New(t, fake.Response{
		Blocks: []fake.Block{fake.Text("Thinking about thrillers")},
		Usage:  fake.Usage{InputTokens: 5000, CacheReadInputTokens: 20_000},
		Hang:   true,
	})
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := request()
	req.Hooks.Text = func(string) { cancel() }
	var called aicore.CallInfo
	req.Hooks.Call = func(c aicore.CallInfo) { called = c }
	res, err := newClient(t, srv, rec).Run(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(rec.rows) != 1 || rec.rows[0].usage.Input != 5000 || rec.rows[0].usage.CacheRead != 20_000 || rec.rows[0].usage.CostUSD <= 0 {
		t.Fatalf("rows = %+v", rec.rows)
	}
	if res.Usage != rec.rows[0].usage || !errors.Is(called.Err, context.Canceled) || len(res.Messages) != 0 {
		t.Errorf("result usage %+v, call err %v, %d messages", res.Usage, called.Err, len(res.Messages))
	}

	// A context already done makes no call.
	srv = fake.New(t)
	if _, err := newClient(t, srv, nil).Run(ctx, request()); !errors.Is(err, context.Canceled) || len(srv.Requests()) != 0 {
		t.Errorf("done context: %v, %d requests", err, len(srv.Requests()))
	}
}

func TestRun_Errors(t *testing.T) {
	for _, tt := range []struct {
		name string
		resp fake.Response
		want error
	}{
		{"429", fake.Response{Status: 429}, aicore.ErrRateLimited},
		{"529", fake.Response{Status: 529}, aicore.ErrUnavailable},
		{"400", fake.Response{Status: 400}, aicore.ErrRejected},
		{"overloaded mid-stream", fake.Response{Blocks: []fake.Block{fake.Text("Hi")}, StreamError: "overloaded_error"}, aicore.ErrUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := fake.New(t, tt.resp)
			_, err := newClient(t, srv, nil).Run(context.Background(), request())
			var apiErr *aicore.APIError
			if !errors.Is(err, tt.want) || !errors.As(err, &apiErr) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	// A tool's own failure (not a *ToolError) ends the run.
	srv := fake.New(t, fake.Response{Blocks: []fake.Block{fake.ToolUse("toolu_1", "alpha", map[string]any{"q": "x"})}})
	boom := errors.New("database down")
	_, err := newClient(t, srv, nil).Run(context.Background(), request(tool{name: "alpha", run: func(json.RawMessage) (json.RawMessage, error) {
		return nil, boom
	}}))
	if !errors.Is(err, boom) {
		t.Errorf("tool failure: %v", err)
	}

	// Bad requests fail before any call.
	srv = fake.New(t)
	c := newClient(t, srv, nil)
	bad := request()
	bad.Model = "claude-unknown-1"
	if _, err := c.Run(context.Background(), bad); !errors.Is(err, aicore.ErrUnknownModel) {
		t.Errorf("unknown model: %v", err)
	}
	bad = request(tool{name: "alpha"}, tool{name: "alpha"})
	if _, err := c.Run(context.Background(), bad); err == nil {
		t.Error("a duplicate tool was accepted")
	}
	bad = request()
	bad.Messages = []aicore.Message{{Role: "assistant"}}
	if _, err := c.Run(context.Background(), bad); err == nil {
		t.Error("a conversation ending on the assistant was accepted")
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("%d requests for bad runs", len(srv.Requests()))
	}
}

// A response with no content is never kept: an assistant message without
// content can't be replayed, so a thread that stored it would 400 on every
// later turn. The run fails instead, and its messages end on the last good
// exchange.
func TestRun_EmptyAnswerIsNotKept(t *testing.T) {
	srv := fake.New(t,
		fake.Response{Blocks: []fake.Block{fake.ToolUse("toolu_1", "alpha", map[string]any{"q": "x"})}, Usage: fake.Usage{InputTokens: 100}},
		fake.Response{Usage: fake.Usage{InputTokens: 200}}, // end_turn, no blocks
	)
	rec := &recorder{}
	res, err := newClient(t, srv, rec).Run(context.Background(), request(tool{name: "alpha"}))
	if !errors.Is(err, aicore.ErrEmptyAnswer) {
		t.Fatalf("err = %v, want ErrEmptyAnswer", err)
	}
	if len(res.Messages) != 2 || res.Messages[0].Role != "assistant" || res.Messages[1].Role != "user" {
		t.Fatalf("messages = %+v", res.Messages)
	}
	for i, m := range res.Messages {
		if len(m.Content) == 0 {
			t.Errorf("message %d has no content", i)
		}
	}
	if res.StopReason != "end_turn" || len(rec.rows) != 2 || res.Usage.Input != 300 {
		t.Errorf("stop %s, %d rows, usage %+v", res.StopReason, len(rec.rows), res.Usage)
	}
}

func TestRequest_Validate(t *testing.T) {
	text := aicore.UserText("Hi")
	for _, tt := range []struct {
		name string
		edit func(*aicore.Request)
		ok   bool
	}{
		{"a plain request", func(*aicore.Request) {}, true},
		{"Sonnet 5.5 with web tools", func(r *aicore.Request) { r.Model, r.WebSearchMaxUses, r.WebFetchMaxUses = "claude-sonnet-5-5", 8, 4 }, true},
		{"a fallback model with web search", func(r *aicore.Request) { r.Model, r.WebSearchMaxUses = "claude-opus-4-8", 1 }, true},
		{"a dated id with web fetch", func(r *aicore.Request) { r.Model, r.WebFetchMaxUses = "claude-opus-5-20260501", 1 }, true},
		{"Haiku without web tools", func(r *aicore.Request) { r.Model = "claude-haiku-5-5" }, true},
		{"Haiku with web search", func(r *aicore.Request) { r.Model, r.WebSearchMaxUses = "claude-haiku-5-5", 8 }, false},
		{"Haiku with web fetch", func(r *aicore.Request) { r.Model, r.WebFetchMaxUses = "claude-haiku-5-5", 1 }, false},
		{"every effort level", func(r *aicore.Request) { r.Effort = "xhigh" }, true},
		{"max effort", func(r *aicore.Request) { r.Effort = "max" }, true},
		{"an unknown effort", func(r *aicore.Request) { r.Effort = "extreme" }, false},
		{"an unknown model", func(r *aicore.Request) { r.Model = "claude-unknown-1" }, false},
		{"no feature", func(r *aicore.Request) { r.Feature = "" }, false},
		{"no messages", func(r *aicore.Request) { r.Messages = nil }, false},
		{"ending on the assistant", func(r *aicore.Request) {
			r.Messages = []aicore.Message{text, {Role: "assistant", Content: text.Content}}
		}, false},
		{"a message without content", func(r *aicore.Request) {
			r.Messages = []aicore.Message{text, {Role: "assistant"}, text}
		}, false},
		{"a tool named like a server tool", func(r *aicore.Request) { r.Tools = []aicore.Tool{tool{name: "web_search"}} }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := request()
			tt.edit(&req)
			if err := req.Validate(); (err == nil) != tt.ok {
				t.Errorf("Validate = %v, want ok %v", err, tt.ok)
			}
		})
	}

	// Run validates before any call.
	srv := fake.New(t)
	req := request()
	req.Model, req.WebSearchMaxUses = "claude-haiku-5-5", 8
	if _, err := newClient(t, srv, nil).Run(context.Background(), req); err == nil || len(srv.Requests()) != 0 {
		t.Errorf("Haiku with web search: %v after %d requests", err, len(srv.Requests()))
	}
}

func TestNew_WithoutKey(t *testing.T) {
	if aicore.New(aicore.Config{}) != nil {
		t.Error("New without a key isn't nil")
	}
}

func jsonEqual(raw json.RawMessage, want string) bool {
	var a, b any
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal([]byte(want), &b) != nil {
		return false
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
