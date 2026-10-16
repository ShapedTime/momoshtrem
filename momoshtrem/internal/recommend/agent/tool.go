// Package agent is the recommendation agent's custom tools and the shape
// every one of them has (PLAN §7 R1–R3). The streaming tool loop that runs
// them is the AI core's (#154): our own, not the SDK's tool runner, which
// in anthropic-sdk-go v1.81 rewrites params.Tools (dropping strict,
// cache_control and server tools) and runs tools in parallel.
package agent

import (
	"context"
	"encoding/json"
)

// Tool is one custom tool the agent can call: get_taste_profile,
// get_candidates, tmdb_search, tmdb_details and check_seen (R1), and the
// tools that submit results (submit_shelf and submit_hero_picks for runs,
// R2; Ask's, R3).
//
// Its definition is sent with `strict: true`, so the schema must be a JSON
// Schema object with "additionalProperties": false and every property in
// "required" (optional values are nullable instead). Name, Description and
// Schema must be byte-stable across calls: they sit in the cached prompt
// prefix.
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
