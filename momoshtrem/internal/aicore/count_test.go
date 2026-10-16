package aicore_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/shapedtime/momoshtrem/internal/aicore"
)

// CountTokens sends Run's first request, less the fallbacks, to the free
// count_tokens endpoint (recommend-run -dry-run).
func TestCountTokens(t *testing.T) {
	var path string
	var body map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"input_tokens": 12345}`)
	}))
	t.Cleanup(srv.Close)
	client := aicore.New(aicore.Config{APIKey: "test-key", Options: []option.RequestOption{
		option.WithBaseURL(srv.URL + "/"), option.WithMaxRetries(0)}})

	req := request(tool{name: "get_candidates"})
	req.WebSearchMaxUses, req.Effort = 8, "high"
	n, err := client.CountTokens(t.Context(), req)
	if err != nil || n != 12345 {
		t.Fatalf("CountTokens = %d, %v", n, err)
	}
	if path != "/v1/messages/count_tokens" {
		t.Errorf("path = %s", path)
	}
	for field, want := range map[string]string{
		"model":         `"claude-opus-5-5"`,
		"thinking":      `{"type":"adaptive"}`,
		"output_config": `{"effort":"high"}`,
	} {
		if got := string(body[field]); got != want {
			t.Errorf("%s = %s, want %s", field, got, want)
		}
	}
	for _, want := range []string{`"strict":true`, `"name":"get_candidates"`, `"type":"web_search_20260209"`, `"max_uses":8`} {
		if !strings.Contains(string(body["tools"]), want) {
			t.Errorf("tools lack %s: %s", want, body["tools"])
		}
	}
	if !strings.Contains(string(body["system"]), `"cache_control":{"type":"ephemeral"}`) || len(body["messages"]) == 0 {
		t.Errorf("system = %s, messages = %s", body["system"], body["messages"])
	}
	if body["fallbacks"] != nil || body["stream"] != nil {
		t.Errorf("sent fallbacks %s, stream %s", body["fallbacks"], body["stream"])
	}

	if _, err := client.CountTokens(t.Context(), aicore.Request{Model: "gpt-5", Feature: "recs"}); err == nil {
		t.Error("an invalid request was counted")
	}
}

// RequestBody is the first call's body, as Run sends it.
func TestRequestBody(t *testing.T) {
	body, err := aicore.RequestBody(request(tool{name: "get_candidates"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"fallbacks":"default"`, `"model":"claude-opus-5-5"`, `"effort":"medium"`, `"name":"get_candidates"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body lacks %s", want)
		}
	}
}
