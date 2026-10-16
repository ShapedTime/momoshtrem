package ai

import (
	"context"
	"errors"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/aicore"
)

// Errors a caller can branch on with errors.Is. Their messages are shown
// to users (a job's ai_error), so they never carry request details. All but
// ErrBadAnswer are the AI core's (aicore), shared by every AI feature.
var (
	// ErrBudgetExceeded: this month's AI spend has reached the budget, so
	// Resolve didn't call the API.
	ErrBudgetExceeded = aicore.ErrBudgetExceeded
	// ErrRefused: the model declined the request (stop_reason refusal).
	// Claude Haiku 5.5 has no server-side fallback, so nothing was answered.
	ErrRefused = aicore.ErrRefused
	// ErrTruncated: the answer hit max_tokens or the context window.
	ErrTruncated = aicore.ErrTruncated
	// ErrBadAnswer: the answer wasn't the JSON the schema asks for.
	ErrBadAnswer = errors.New("the model's answer couldn't be read")
	// ErrUnknownModel: the model (classifier_model) isn't in the price
	// table, so its cost couldn't be counted. Use claude-haiku-5-5,
	// claude-sonnet-5-5 or claude-opus-5-5.
	ErrUnknownModel = aicore.ErrUnknownModel

	// ErrRateLimited: Anthropic answered 429. Worth retrying later.
	ErrRateLimited = aicore.ErrRateLimited
	// ErrUnavailable: a 5xx (529 overloaded included), a timeout or a
	// network error. Worth retrying later.
	ErrUnavailable = aicore.ErrUnavailable
	// ErrRejected: another 4xx, such as a bad key (401) or a request the API
	// won't take (400). Retrying won't help.
	ErrRejected = aicore.ErrRejected
)

// APIError is a failed call to the Anthropic API. errors.Is matches it
// against ErrRateLimited, ErrUnavailable or ErrRejected, and against the
// underlying error.
type APIError = aicore.APIError

// apiError classifies an error from the SDK (aicore.ClassifyError).
func apiError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		// The caller gave up (job cancelled, shutdown): not the API's fault.
		return fmt.Errorf("resolve: %w", ctx.Err())
	}
	return aicore.ClassifyError(ctx, err)
}
