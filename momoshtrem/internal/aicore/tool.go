package aicore

import (
	"context"
	"encoding/json"
)

// Tool is one custom tool a run can call (recommend/agent.Tool is this
// interface).
//
// Its definition is sent with `strict: true`, so the schema must be a JSON
// Schema object with "additionalProperties": false and every property in
// "required" (optional values are nullable instead). Name, Description and
// Schema must be byte-stable across calls: they sit in the cached prompt
// prefix, ahead of the system prompt.
type Tool interface {
	// Name is the tool's name, e.g. "get_candidates".
	Name() string
	// Description tells the model when and how to use it.
	Description() string
	// Schema is the input's strict JSON Schema.
	Schema() json.RawMessage
	// Run executes one call with the model's input (valid against Schema)
	// and returns the tool_result content as JSON. An error the model
	// should see and recover from (an unknown TMDB id, say) is a result
	// with is_error, returned as *ToolError; any other error ends the run.
	Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// ToolError is a failure the model sees (tool_result with is_error: true)
// and can recover from, such as an id that doesn't exist.
type ToolError struct {
	Message string
}

func (e *ToolError) Error() string { return e.Message }

// ToolCall is a custom tool call from the model, in the order its blocks
// came.
type ToolCall struct {
	// ID is the tool_use block's id.
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult is what a call answered, as the model sees it.
type ToolResult struct {
	// Output is the tool's JSON; nil for an error.
	Output json.RawMessage
	// IsError marks a *ToolError (or an unknown tool), whose Message the
	// model reads instead.
	IsError bool
	Message string
}

// content is the tool_result's content.
func (r ToolResult) content() string {
	if r.IsError {
		return r.Message
	}
	if len(r.Output) == 0 {
		return "{}"
	}
	return string(r.Output)
}
