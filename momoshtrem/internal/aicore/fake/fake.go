// Package fake is a scripted Anthropic Messages API for tests. Each POST
// /v1/messages takes the next scripted Response and streams it as
// server-sent events the way the API does (message_start, the blocks'
// start, delta and stop events, message_delta with the stop reason and
// usage, message_stop), or answers an HTTP error. Every request is
// recorded with its headers and body, byte for byte, so tests can assert
// what was sent: the tools, the system prompt, the replayed messages, the
// betas. No test touches the network.
//
//	srv := fake.New(t,
//		fake.Response{Blocks: []fake.Block{fake.ToolUse("toolu_1", "get_candidates", map[string]any{"limit": 5})}},
//		fake.Response{Blocks: []fake.Block{fake.Text("Here are five picks.")}},
//	)
//	client := aicore.New(aicore.Config{APIKey: "test", Options: srv.Options()})
package fake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultModel is the model a response names when it names none.
const DefaultModel = "claude-opus-5-5"

// Response is one scripted answer.
type Response struct {
	// Model is the model message_start names; empty means DefaultModel.
	// After a pre-output refusal fallback it's the fallback's.
	Model string
	// Blocks are the content blocks, streamed in order.
	Blocks []Block
	// StopReason is message_delta's; empty means tool_use when a block is
	// a tool_use, else end_turn.
	StopReason string
	// RefusalCategory fills stop_details for a refusal ("cyber", "bio", …).
	RefusalCategory string
	// Usage is message_delta's usage (message_start carries its prompt
	// counts).
	Usage Usage

	// Status, when set, answers this HTTP error instead of a stream (429,
	// 529, 400, …); ErrorType names it ("rate_limit_error", …), else it
	// follows from the status.
	Status    int
	ErrorType string
	// StreamError ends the stream with an error event of this type
	// ("overloaded_error") after the blocks, instead of the message's end.
	StreamError string
	// Hang streams message_start and the blocks, then holds the stream
	// open until the client goes away: a call to cancel.
	Hang bool
}

// Usage is a response's usage.
type Usage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
	WebSearchRequests        int64
	WebFetchRequests         int64
	// Iterations is usage.iterations: one entry per sampling attempt.
	Iterations []Iteration
}

// Iteration is one usage.iterations entry.
type Iteration struct {
	// Type is "message" (the default), "fallback_message" or "compaction".
	Type                     string
	Model                    string
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
}

// Block is one content block as it streams: its content_block_start and
// its deltas.
type Block struct {
	start  map[string]any
	deltas []map[string]any
}

// Text is a text block, streamed two words per delta.
func Text(text string) Block {
	b := Block{start: map[string]any{"type": "text", "text": ""}}
	for _, chunk := range chunks(text) {
		b.deltas = append(b.deltas, map[string]any{"type": "text_delta", "text": chunk})
	}
	return b
}

// Thinking is a thinking block as Opus 5.5 streams one by default: empty
// text (display omitted) and a signature.
func Thinking(signature string) Block {
	return Block{
		start:  map[string]any{"type": "thinking", "thinking": "", "signature": ""},
		deltas: []map[string]any{{"type": "signature_delta", "signature": signature}},
	}
}

// ToolUse is a custom tool call; its input streams as JSON in two pieces.
func ToolUse(id, name string, input any) Block {
	return toolBlock("tool_use", id, name, input)
}

// ServerToolUse is a server tool call ("web_search", "web_fetch").
func ServerToolUse(id, name string, input any) Block {
	return toolBlock("server_tool_use", id, name, input)
}

// WebSearchResult is a web search's result, for the server_tool_use id.
func WebSearchResult(toolUseID string, urls ...string) Block {
	results := []any{}
	for _, u := range urls {
		results = append(results, map[string]any{
			"type": "web_search_result", "url": u, "title": "Result for " + u,
			"encrypted_content": "enc_" + u, "page_age": nil,
		})
	}
	return Block{start: map[string]any{"type": "web_search_tool_result", "tool_use_id": toolUseID, "content": results}}
}

// Fallback marks a refusal fallback's handoff from one model to another.
func Fallback(from, to string) Block {
	return Block{start: map[string]any{"type": "fallback", "from": map[string]any{"model": from}, "to": map[string]any{"model": to}}}
}

func toolBlock(typ, id, name string, input any) Block {
	data, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Sprintf("fake: tool input: %v", err))
	}
	half := len(data) / 2
	return Block{
		start: map[string]any{"type": typ, "id": id, "name": name, "input": map[string]any{}},
		deltas: []map[string]any{
			{"type": "input_json_delta", "partial_json": string(data[:half])},
			{"type": "input_json_delta", "partial_json": string(data[half:])},
		},
	}
}

// chunks splits text into pieces of two words, keeping every byte.
func chunks(text string) []string {
	var out []string
	words := strings.SplitAfter(text, " ")
	if words[len(words)-1] == "" {
		words = words[:len(words)-1]
	}
	for i := 0; i < len(words); i += 2 {
		out = append(out, strings.Join(words[i:min(i+2, len(words))], ""))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// Request is a recorded request.
type Request struct {
	Method string
	Path   string
	Header http.Header
	// Body is the request body as sent.
	Body []byte
}

// Fields is the body's top-level fields, raw.
func (r Request) Fields() map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(r.Body, &fields)
	return fields
}

// Field is one top-level field, raw; nil when absent.
func (r Request) Field(name string) json.RawMessage { return r.Fields()[name] }

// Messages is the body's messages, each raw.
func (r Request) Messages() []json.RawMessage {
	var msgs []json.RawMessage
	_ = json.Unmarshal(r.Field("messages"), &msgs)
	return msgs
}

// Betas is the anthropic-beta header's values.
func (r Request) Betas() []string {
	var out []string
	for _, h := range r.Header.Values("anthropic-beta") {
		for _, b := range strings.Split(h, ",") {
			if b = strings.TrimSpace(b); b != "" {
				out = append(out, b)
			}
		}
	}
	return out
}

// Server is the fake API.
type Server struct {
	t   testing.TB
	srv *httptest.Server

	mu         sync.Mutex
	script     []Response
	requests   []Request
	served     int
	unexpected []string
}

// New starts a fake API answering the script in order. It's closed when
// the test ends, which fails if a request came with the script used up.
func New(t testing.TB, script ...Response) *Server {
	t.Helper()
	s := &Server{t: t, script: script}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(func() {
		s.srv.Close()
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, u := range s.unexpected {
			t.Errorf("fake Anthropic API: %s", u)
		}
	})
	return s
}

// Push adds responses to the script.
func (s *Server) Push(rs ...Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script = append(s.script, rs...)
}

// URL is the server's base URL.
func (s *Server) URL() string { return s.srv.URL }

// Options point an SDK client at the server, without retries.
func (s *Server) Options() []option.RequestOption {
	return []option.RequestOption{option.WithBaseURL(s.srv.URL + "/"), option.WithMaxRetries(0)}
}

// Requests are the requests so far, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Remaining is how many scripted responses are left.
func (s *Server) Remaining() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.script) - s.served
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
	n := len(s.requests)
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/v1/messages") {
		s.unexpected = append(s.unexpected, fmt.Sprintf("request %d: unexpected %s %s", n, r.Method, r.URL.Path))
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "not_found_error")
		return
	}
	if s.served >= len(s.script) {
		s.unexpected = append(s.unexpected, fmt.Sprintf("request %d: no scripted response left", n))
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "api_error")
		return
	}
	resp := s.script[s.served]
	s.served++
	s.mu.Unlock()

	if resp.Status != 0 {
		typ := resp.ErrorType
		if typ == "" {
			typ = errorType(resp.Status)
		}
		writeError(w, resp.Status, typ)
		return
	}
	stream(w, r, n, resp)
}

func errorType(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status == 529:
		return "overloaded_error"
	case status == http.StatusUnauthorized:
		return "authentication_error"
	case status >= 500:
		return "api_error"
	}
	return "invalid_request_error"
}

func writeError(w http.ResponseWriter, status int, typ string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("request-id", "req_fake")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error", "error": map[string]any{"type": typ, "message": "fake " + typ}, "request_id": "req_fake",
	})
}

func stream(w http.ResponseWriter, r *http.Request, n int, resp Response) {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("request-id", fmt.Sprintf("req_fake_%d", n))
	w.WriteHeader(http.StatusOK)
	send := func(event string, data any) {
		payload, err := json.Marshal(data)
		if err != nil {
			panic(fmt.Sprintf("fake: event %s: %v", event, err))
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
		if flusher != nil {
			flusher.Flush()
		}
	}

	model := resp.Model
	if model == "" {
		model = DefaultModel
	}
	u := resp.Usage
	send("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": fmt.Sprintf("msg_fake_%d", n), "type": "message", "role": "assistant", "model": model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens": u.InputTokens, "cache_creation_input_tokens": u.CacheCreationInputTokens,
			"cache_read_input_tokens": u.CacheReadInputTokens, "output_tokens": 1,
		},
	}})
	send("ping", map[string]any{"type": "ping"})

	hasToolUse := false
	for i, b := range resp.Blocks {
		if b.start["type"] == "tool_use" {
			hasToolUse = true
		}
		send("content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": b.start})
		for _, d := range b.deltas {
			send("content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": d})
		}
		send("content_block_stop", map[string]any{"type": "content_block_stop", "index": i})
	}

	switch {
	case resp.Hang:
		<-r.Context().Done()
		return
	case resp.StreamError != "":
		send("error", map[string]any{"type": "error", "error": map[string]any{"type": resp.StreamError, "message": "fake " + resp.StreamError}})
		return
	}

	stop := resp.StopReason
	if stop == "" {
		stop = "end_turn"
		if hasToolUse {
			stop = "tool_use"
		}
	}
	var details any
	if stop == "refusal" {
		var category any
		if resp.RefusalCategory != "" {
			category = resp.RefusalCategory
		}
		details = map[string]any{"type": "refusal", "category": category, "explanation": nil}
	}
	usage := map[string]any{
		"input_tokens": u.InputTokens, "cache_creation_input_tokens": u.CacheCreationInputTokens,
		"cache_read_input_tokens": u.CacheReadInputTokens, "output_tokens": u.OutputTokens,
		"server_tool_use": map[string]any{"web_search_requests": u.WebSearchRequests, "web_fetch_requests": u.WebFetchRequests},
	}
	if len(u.Iterations) > 0 {
		var its []any
		for _, it := range u.Iterations {
			typ := it.Type
			if typ == "" {
				typ = "message"
			}
			entry := map[string]any{
				"type": typ, "input_tokens": it.InputTokens, "output_tokens": it.OutputTokens,
				"cache_read_input_tokens": it.CacheReadInputTokens, "cache_creation_input_tokens": it.CacheCreationInputTokens,
			}
			if typ != "compaction" {
				entry["model"] = it.Model
			}
			its = append(its, entry)
		}
		usage["iterations"] = its
	}
	send("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil, "stop_details": details},
		"usage": usage,
	})
	send("message_stop", map[string]any{"type": "message_stop"})
}
