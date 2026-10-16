package recommend

import (
	"errors"
	"fmt"
	"time"
)

// Errors the recommendations, Ask and AI settings services return; the API
// maps each to its status and code (internal/api/recommendations_routes.go).
var (
	// ErrUnavailable: no ANTHROPIC_API_KEY (503 unavailable).
	ErrUnavailable = errors.New("AI isn't configured")
	// ErrDisabled: refresh while rec_enabled is off (409 recs_disabled).
	ErrDisabled = errors.New("recommendations are off")
	// ErrNoHistory: refresh without Trakt connected or with fewer than 5
	// loved or completed titles (409 no_history).
	ErrNoHistory = errors.New("not enough watch history")
	// ErrNotFound: an unknown run or thread (404 not_found).
	ErrNotFound = errors.New("not found")
	// ErrAskBusy: another Ask turn is streaming (409 ask_busy).
	ErrAskBusy = errors.New("another ask is streaming")
	// ErrThreadFull: the thread has ThreadMaxTurns turns (409 thread_full).
	ErrThreadFull = errors.New("the thread is full")
)

// LimitError is a limit reached until ResetsAt: the monthly budget
// (budget_exceeded, the 1st of next month) or Ask's daily limit
// (ask_limit, the next server-local midnight). The API answers 429 with
// resets_at.
type LimitError struct {
	Code     string // CodeBudgetExceeded or CodeAskLimit
	ResetsAt time.Time
}

// Limit codes.
const (
	CodeBudgetExceeded = "budget_exceeded"
	CodeAskLimit       = "ask_limit"
)

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s until %s", e.Code, e.ResetsAt.Format(time.RFC3339))
}

// FieldError is an invalid PUT /settings/ai field (422 validation_error
// with error.field).
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }
