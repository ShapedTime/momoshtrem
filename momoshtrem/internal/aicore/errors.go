package aicore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Errors a caller can branch on with errors.Is. Their messages may be shown
// to users, so they never carry request details.
var (
	// ErrBudgetExceeded: this month's AI spend has reached the budget
	// (monthly_budget_usd), so no API call was made. *BudgetError says how
	// much and until when.
	ErrBudgetExceeded = errors.New("this month's AI budget is spent")
	// ErrRefused: the model declined the request (stop_reason refusal),
	// fallbacks included. *RefusalError carries the category.
	ErrRefused = errors.New("the model declined to answer")
	// ErrTruncated: the answer hit max_tokens or the context window.
	ErrTruncated = errors.New("the model's answer was cut off")
	// ErrEmptyAnswer: a response came back with no content, which can't
	// be replayed. Not the caller's fault: treat it like an upstream error
	// (Ask's upstream code).
	ErrEmptyAnswer = errors.New("the model's answer was empty")
	// ErrCapReached: a run reached one of its caps (*CapError says which)
	// and stopped before its next API call.
	ErrCapReached = errors.New("the AI run reached its limit")
	// ErrUnknownModel: the model has no price, so its cost couldn't be
	// counted. Use one of Models().
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

// BudgetError is ErrBudgetExceeded with the month's figures.
type BudgetError struct {
	SpentUSD, BudgetUSD float64
	// ResetsAt is the 1st of next month, 00:00 server-local.
	ResetsAt time.Time
}

func (e *BudgetError) Error() string {
	return fmt.Sprintf("%v: $%.2f of $%.2f", ErrBudgetExceeded, e.SpentUSD, e.BudgetUSD)
}

func (e *BudgetError) Unwrap() error { return ErrBudgetExceeded }

// RefusalError is ErrRefused with stop_details.
type RefusalError struct {
	// Category is stop_details.category ("cyber", "bio", …); empty when the
	// refusal doesn't map to one.
	Category string
	// Model is the model that declined last (a fallback's when one ran).
	Model string
}

func (e *RefusalError) Error() string {
	if e.Category != "" {
		return fmt.Sprintf("%v (%s)", ErrRefused, e.Category)
	}
	return ErrRefused.Error()
}

func (e *RefusalError) Unwrap() error { return ErrRefused }

// Caps a run can reach (CapError.Cap).
const (
	CapCalls         = "calls"
	CapTokens        = "tokens"
	CapCost          = "cost"
	CapToolCalls     = "tool_calls"
	CapContinuations = "continuations"
	CapWebSearches   = "web_searches"
)

// CapError is ErrCapReached naming the cap.
type CapError struct {
	Cap string
}

func (e *CapError) Error() string { return fmt.Sprintf("%v (%s)", ErrCapReached, e.Cap) }

func (e *CapError) Unwrap() error { return ErrCapReached }

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

// ClassifyError turns an error from the SDK into an *APIError. The SDK has
// already retried 429s, 5xx and connection errors (twice by default). When
// ctx is done the caller gave up (cancelled, shutdown): that isn't the
// API's fault, and ctx's error is returned as is.
func ClassifyError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var sdkErr *anthropic.Error
	if errors.As(err, &sdkErr) {
		typ := string(sdkErr.Type())
		// An error event in an open stream comes with the stream's 200: its
		// type tells what happened.
		midStream := sdkErr.StatusCode < 400
		kind := ErrRejected
		switch {
		case sdkErr.StatusCode == 429, midStream && typ == "rate_limit_error":
			kind = ErrRateLimited
		case sdkErr.StatusCode >= 500, midStream && (typ == "overloaded_error" || typ == "api_error"):
			kind = ErrUnavailable
		}
		return &APIError{StatusCode: sdkErr.StatusCode, Type: typ, RequestID: sdkErr.RequestID, kind: kind, err: err}
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
