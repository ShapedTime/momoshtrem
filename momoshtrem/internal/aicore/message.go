package aicore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one message as the API takes it, its content blocks raw
// JSON. A run returns the messages it appended (the assistant's replies and
// the tool results) this way; Ask stores them (ask_messages.content) and
// replays them unchanged on the next turn: thinking blocks and server tool
// results only stay valid in an append-only history.
type Message struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

// UserText is a user message with one text block.
func UserText(text string) Message {
	return Message{Role: RoleUser, Content: []json.RawMessage{mustMarshal(textBlock{Type: "text", Text: text})}}
}

// Text is the message's text: its text blocks, joined.
func (m Message) Text() string {
	var b strings.Builder
	for _, raw := range m.Content {
		var block textBlock
		if json.Unmarshal(raw, &block) == nil && block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("aicore: marshal %T: %v", v, err)) // plain structs of strings never fail
	}
	return data
}

// echo turns a response into the assistant message the next call sends
// back, each block as its request param, raw. It also returns the blocks
// kept, whose tool_use blocks are the calls to run.
//
// After a refusal fallback the blocks before the final fallback block that
// the continuing model can't take are dropped, as the API asks: thinking,
// redacted_thinking and client tool_use blocks, a server_tool_use without
// its result, and any block type besides text and paired server tool
// blocks. Fallback blocks stay where they were: the API checks the thinking
// blocks around them by their position.
func echo(m *anthropic.BetaMessage) (Message, []anthropic.BetaContentBlockUnion, error) {
	lastFallback := -1
	for i, b := range m.Content {
		if b.Type == "fallback" {
			lastFallback = i
		}
	}
	paired := map[string]bool{} // server_tool_use ids with a result
	if lastFallback > 0 {
		for _, b := range m.Content[:lastFallback] {
			if b.ToolUseID != "" {
				paired[b.ToolUseID] = true
			}
		}
	}
	msg := Message{Role: RoleAssistant}
	var kept []anthropic.BetaContentBlockUnion
	for i, b := range m.Content {
		if i < lastFallback && !keptBeforeFallback(b, paired) {
			continue
		}
		raw, err := json.Marshal(b.ToParam())
		if err != nil {
			return Message{}, nil, fmt.Errorf("encode a %s block: %w", b.Type, err)
		}
		msg.Content = append(msg.Content, raw)
		kept = append(kept, b)
	}
	return msg, kept, nil
}

func keptBeforeFallback(b anthropic.BetaContentBlockUnion, paired map[string]bool) bool {
	switch {
	case b.Type == "text", b.Type == "fallback":
		return true
	case b.Type == "server_tool_use" || b.Type == "mcp_tool_use":
		return paired[b.ID]
	case b.ToolUseID != "": // a server tool's result
		return true
	}
	return false
}
