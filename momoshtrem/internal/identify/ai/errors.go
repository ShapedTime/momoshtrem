package ai

import (
	"context"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// Errors a caller can branch on with errors.Is. Their messages are shown
// to users (a job's ai_error), so they never carry request details.
var (
	// ErrBudgetExceeded: this month's AI spend has reached the budget, so
	// Resolve didn't call the API.
	ErrBudgetExceeded = errors.New("this month's AI budget is spent")
	// ErrRefused: the model declined the request (stop_reason refusal).
	// Claude Haiku 5.5 has no server-side fallback, so nothing was answered.
	ErrRefused = errors.New("the model declined to answer")
	// ErrTruncated: the answer hit max_tokens or the context window.
	ErrTruncated = errors.New("the model's answer was cut off")
	// ErrBadAnswer: the answer wasn't the JSON the schema asks for.
	ErrBadAnswer = errors.New("the model's answer couldn't be read")
	// ErrUnknownModel: the model (classifier_model) isn't in the price
	// table, so its cost couldn't be counted. Use claude-haiku-5-5,
	// claude-sonnet-5-5 or claude-opus-5-5.
	ErrUnknownModel = errors.New("the AI model isn't supported")

	// ErrRateLimited: Anthropic answered 429. Worth retrying later.
	ErrRateLimited = errors.New("Anthropic is rate limiting requests")
	// ErrUnavailable: a 5xx (529 overloaded included), a timeout or a
	// network error. Worth retrying later.
	ErrUnavailable = errors.New("Anthropic is unavailable")
	// ErrRejected: another 4xx, such as a bad key (401) or a request the API
	// won't take (400). Retrying won't help.
	ErrRejected = errors.New("Anthropic rejected the request")
)

// APIError is a failed call to the Anthropic API. errors.Is matches it
// against ErrRateLimited, ErrUnavailable or ErrRejected, and against the
// underlying error.
type APIError struct {
	// StatusCode is the HTTP status; 0 for a timeout or a network error.
	StatusCode int
	// Type is the API's error type, such as "rate_limit_error" or
	// "overloaded_error"; empty when there was no response.
	Type      string
	RequestID string

	kind error
	err  error
}

func (e *APIError) Error() string {
	switch {
	case e.StatusCode == 0:
		return fmt.Sprintf("%v (%s)", e.kind, transportReason(e.err))
	case e.Type != "":
		return fmt.Sprintf("%v (%d %s)", e.kind, e.StatusCode, e.Type)
	default:
		return fmt.Sprintf("%v (%d)", e.kind, e.StatusCode)
	}
}

// Unwrap gives errors.Is both the kind and the cause.
func (e *APIError) Unwrap() []error { return []error{e.kind, e.err} }

// apiError classifies an error from the SDK. The SDK has already retried
// 429s, 5xx and connection errors (twice by default).
func apiError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		// The caller gave up (job cancelled, shutdown): not the API's fault.
		return fmt.Errorf("resolve: %w", ctx.Err())
	}
	var sdkErr *anthropic.Error
	if errors.As(err, &sdkErr) {
		kind := ErrRejected
		switch {
		case sdkErr.StatusCode == 429:
			kind = ErrRateLimited
		case sdkErr.StatusCode >= 500:
			kind = ErrUnavailable
		}
		return &APIError{StatusCode: sdkErr.StatusCode, Type: string(sdkErr.Type()), RequestID: sdkErr.RequestID, kind: kind, err: err}
	}
	// No response: the request timed out or the connection failed.
	return &APIError{kind: ErrUnavailable, err: err}
}

func transportReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	return "no response"
}
