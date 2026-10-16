// Package recommend is AI recommendations and Ask momoshtrem (PLAN §6
// "Recommendations (R1–R3)", #9): the taste profile and candidates (R1),
// nightly runs that write shelves and hero picks (R2), and Ask over SSE (R3).
//
// This file holds the wire shapes, exactly as PLAN §6 and
// web/src/lib/api/contract/{recommendations,settings}.ts have them. Times
// are server-local (TZ).
package recommend

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
)

// Models picks and Ask run on (rec_model), and the classifier's.
const (
	ModelOpus   = "claude-opus-5-5"
	ModelSonnet = "claude-sonnet-5-5"
	ModelHaiku  = "claude-haiku-5-5"
)

// RecModels are rec_model's values; ClassifierModels are classifier_model's.
var (
	RecModels        = []string{ModelOpus, ModelSonnet}
	ClassifierModels = []string{ModelHaiku, ModelSonnet, ModelOpus}
)

// Schedules (rec_schedule): runs at 03:00 server-local; weekly on Sunday.
const (
	ScheduleNightly = "nightly"
	ScheduleWeekly  = "weekly"
	ScheduleManual  = "manual"
)

// Run kinds.
const (
	KindScheduled = "scheduled"
	KindManual    = "manual"
	KindFirstSync = "first_sync"
)

// Run statuses: at most one run is queued or running.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
)

// Skip reasons: a scheduled or first-sync run meeting these is stored as
// skipped; refresh answers 429 budget_exceeded or 409 no_history instead.
const (
	SkipBudgetExceeded = "budget_exceeded"
	SkipNoHistory      = "no_history"
)

// Steps. A run goes through history, web, tmdb and write; an Ask turn too,
// or rerank and write for a refinement.
const (
	StepHistory = "history"
	StepWeb     = "web"
	StepTMDB    = "tmdb"
	StepRerank  = "rerank"
	StepWrite   = "write"
)

// Step states.
const (
	StepActive = "active"
	StepDone   = "done"
)

// Shelf kinds.
const (
	ShelfBecause    = "because"
	ShelfNew        = "new"
	ShelfAnime      = "anime"
	ShelfGems       = "gems"
	ShelfComingSoon = "coming_soon"
	ShelfOther      = "other"
)

// Feedback signals: dislike, not_interested and seen hide a title from
// every AI surface and from the candidate pools; seen also counts as a
// completed, neutral title in the profile.
const (
	SignalLike          = "like"
	SignalDislike       = "dislike"
	SignalNotInterested = "not_interested"
	SignalSeen          = "seen"
)

// Feedback sources: where it was given.
const (
	SourceShelf = "shelf"
	SourceHero  = "hero"
	SourceAsk   = "ask"
	SourceTitle = "title"
)

// Ask turn statuses.
const (
	TurnStatusStreaming = "streaming"
	TurnStatusDone      = "done"
	TurnStatusError     = "error"
	TurnStatusCancelled = "cancelled"
)

// Ask's error event codes.
const (
	AskErrorRefused      = "refused"       // Claude declined, fallbacks included
	AskErrorLimitReached = "limit_reached" // the turn hit its token or tool-call cap
	AskErrorUpstream     = "upstream"      // the Anthropic API failed after retries
	AskErrorInternal     = "internal"
)

// Limits from the contract.
const (
	ReasonMaxChars          = 140
	MatchTagMaxChars        = 32
	ShelfMinItems           = 6
	ShelfMaxItems           = 15
	HeroPicksPerDay         = 3
	HeroPicksMax            = 9
	AskMessageMaxChars      = 500
	AskDailyLimit           = 30
	ThreadMaxTurns          = 20
	RefinementsMax          = 5
	DefaultMonthlyBudgetUSD = 15.0 // an explicit null is no cap
)

// ItemMeta is a title's facts as a run snapshots them (rec_items.meta), so
// reads never call TMDB.
type ItemMeta struct {
	Title          string   `json:"title"`
	Year           *int     `json:"year"`
	YearEnd        *int     `json:"year_end"` // shows: the last year; null while running, and for movies
	PosterPath     *string  `json:"poster_path"`
	Seasons        *int     `json:"seasons"`         // shows, Specials left out; null for movies
	RuntimeMinutes *int     `json:"runtime_minutes"` // a movie's, a show's episode runtime
	VoteAverage    *float64 `json:"vote_average"`
	ReleaseDate    *string  `json:"release_date"` // YYYY-MM-DD: a movie's release, a show's first air date
	Badge          *string  `json:"badge"`        // a short poster label ("Jan 16"), mostly coming_soon
}

// Item is RecItem: a recommended title. Library, OnWatchlist and Feedback
// are joined at read time; a title with negative feedback is left out.
type Item struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	ItemMeta
	Reason      string                  `json:"reason"` // ≤ 140 characters
	Library     *catalog.SimilarLibrary `json:"library"`
	OnWatchlist bool                    `json:"on_watchlist"`
	Feedback    *string                 `json:"feedback"` // "like" or null
}

// AskItem is a title Ask found.
type AskItem struct {
	Item
	MatchTag string `json:"match_tag"` // ≤ 32 characters
}

// Shelf is one of a run's 4–6 shelves (6–15 items when written).
type Shelf struct {
	Slug     string  `json:"slug"`
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
	Subtitle *string `json:"subtitle"`
	Items    []Item  `json:"items"`
}

// HeroPick is "Tonight's pick": a run writes 3–9 and /home shows three a
// server-local day.
type HeroPick struct {
	MediaType string  `json:"media_type"`
	TMDBID    int     `json:"tmdb_id"`
	Kicker    string  `json:"kicker"`
	Reason    string  `json:"reason"`
	Feedback  *string `json:"feedback"` // "like" or null
}

// RunSummary is a run as lists show it.
type RunSummary struct {
	ID         int64      `json:"id"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Model      string     `json:"model"`
	StartedAt  *time.Time `json:"started_at"` // null while queued, and for a skipped run
	FinishedAt *time.Time `json:"finished_at"`
	CostUSD    float64    `json:"cost_usd"` // so far, while it runs
	SkipReason *string    `json:"skip_reason"`
	Error      *string    `json:"error"` // for the user, while failed
}

// Step is a run's step (history, web, tmdb, write) or an Ask turn's
// (history, rerank, web, tmdb, write).
type Step struct {
	Step   string  `json:"step"`
	State  string  `json:"state"`
	Detail *string `json:"detail"` // the step's tally: "2,184 plays · 312 ratings", "2 of 5 shelves"
}

// RunProgress is a run with its progress (rec_runs.progress): the steps
// reached so far in order, at most one active; all done once it ends.
type RunProgress struct {
	RunSummary
	Steps         []Step `json:"steps"`
	ShelvesDone   int    `json:"shelves_done"`
	ShelvesTarget int    `json:"shelves_target"` // 0 until it starts
}

// Usage is a run's API usage: its ai_usage rows, summed.
type Usage struct {
	APICalls        int     `json:"api_calls"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	WebSearches     int     `json:"web_searches"`
	CostUSD         float64 `json:"cost_usd"`
}

// RunDetail is GET /recommendations/runs/{id}.
type RunDetail struct {
	RunProgress
	Usage Usage `json:"usage"`
}

// Home is GET /recommendations/home. Without a key or with rec_enabled off
// it's {enabled: false} with everything else empty.
type Home struct {
	Enabled   bool         `json:"enabled"`
	Run       *RunSummary  `json:"run"`        // the newest succeeded run; null before the first
	ActiveRun *RunProgress `json:"active_run"` // the queued or running run
	HeroPicks []HeroPick   `json:"hero_picks"` // the day's three
	Shelves   []Shelf      `json:"shelves"`
}

// RunAccepted is POST /recommendations/refresh's 202.
type RunAccepted struct {
	RunID int64 `json:"run_id"`
}

// Feedback is POST /recommendations/feedback's body: an upsert.
type Feedback struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Signal    string `json:"signal"`
	Source    string `json:"source"`
}

// ---- Ask ----

// AskRequest is POST /recommendations/ask's body; without ThreadID a new
// thread starts.
type AskRequest struct {
	ThreadID *int64 `json:"thread_id,omitempty"`
	Message  string `json:"message"` // 1–500 characters, trimmed
}

// ThreadSummary is a thread as GET /recommendations/threads lists it.
type ThreadSummary struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TurnCount int       `json:"turn_count"`
}

// Thread is GET /recommendations/threads/{id}.
type Thread struct {
	ThreadSummary
	Model string `json:"model"` // the thread keeps the model it started on
	Turns []Turn `json:"turns"`
}

// TurnError is an Ask turn's failure.
type TurnError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Turn is one ask and its answer, as ask_turns.view saved it when it
// ended; a streaming turn has its message only.
type Turn struct {
	ID          int64      `json:"id"`
	Message     string     `json:"message"`
	Status      string     `json:"status"`
	Steps       []Step     `json:"steps"`
	Intro       string     `json:"intro"`
	Items       []AskItem  `json:"items"`
	Refinements []string   `json:"refinements"`
	Error       *TurnError `json:"error"`
	ElapsedMS   *int64     `json:"elapsed_ms"` // null while streaming
	CreatedAt   time.Time  `json:"created_at"`
}

// Ask's server-sent event names, in order: start, status…, text…, item…,
// then done or error.
const (
	EventStart  = "start"
	EventStatus = "status"
	EventText   = "text"
	EventItem   = "item"
	EventError  = "error"
	EventDone   = "done"
)

// Event is one server-sent event: Name is the SSE event, Data its JSON
// (StartEvent, Step, TextEvent, AskItem, TurnError or DoneEvent).
type Event struct {
	Name string
	Data any
}

// StartEvent opens the stream.
type StartEvent struct {
	ThreadID int64  `json:"thread_id"`
	TurnID   int64  `json:"turn_id"`
	Title    string `json:"title"`
}

// TextEvent is a piece of the turn's intro.
type TextEvent struct {
	Delta string `json:"delta"`
}

// TurnUsage is what one turn cost.
type TurnUsage struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	WebSearches  int     `json:"web_searches"`
	CostUSD      float64 `json:"cost_usd"`
}

// DoneEvent ends a turn that finished.
type DoneEvent struct {
	ThreadID    int64     `json:"thread_id"`
	TurnID      int64     `json:"turn_id"`
	ElapsedMS   int64     `json:"elapsed_ms"`
	Refinements []string  `json:"refinements"` // ≤ 5 chips, sent back as follow-ups
	Usage       TurnUsage `json:"usage"`
}

// ---- Settings and usage ----

// Settings is GET/PUT /settings/ai: app_settings key "ai", read over the
// defaults. library.AISettings is the classifier's share of it.
type Settings struct {
	RecEnabled              bool     `json:"rec_enabled"`
	RecModel                string   `json:"rec_model"`
	RecSchedule             string   `json:"rec_schedule"`
	MonthlyBudgetUSD        *float64 `json:"monthly_budget_usd"` // nil: no cap
	ClassifierEnabled       bool     `json:"classifier_enabled"`
	ClassifierModel         string   `json:"classifier_model"`
	AutoApplyHighConfidence bool     `json:"auto_apply_high_confidence"`
}

// DefaultSettings are a fresh install's.
func DefaultSettings() Settings {
	budget := DefaultMonthlyBudgetUSD
	return Settings{
		RecEnabled:              true,
		RecModel:                ModelOpus,
		RecSchedule:             ScheduleNightly,
		MonthlyBudgetUSD:        &budget,
		ClassifierEnabled:       true,
		ClassifierModel:         ModelHaiku,
		AutoApplyHighConfidence: true,
	}
}

// SettingsPatch is PUT /settings/ai's body: nil fields keep their value.
// MonthlyBudgetUSD needs two levels: SetBudget says it was sent, and a nil
// MonthlyBudgetUSD with it set means "no cap".
type SettingsPatch struct {
	RecEnabled              *bool
	RecModel                *string
	RecSchedule             *string
	SetBudget               bool
	MonthlyBudgetUSD        *float64
	ClassifierEnabled       *bool
	ClassifierModel         *string
	AutoApplyHighConfidence *bool
}

// Features ai_usage records.
const (
	FeatureRecs       = "recs"
	FeatureAsk        = "ask"
	FeatureClassifier = "classifier"
)

// FeatureUsage is one feature's spend this month.
type FeatureUsage struct {
	Feature  string  `json:"feature"`
	CostUSD  float64 `json:"cost_usd"`
	Requests int     `json:"requests"` // API calls
}

// RecsUsage is the picks' part of the usage page.
type RecsUsage struct {
	LastRun   *RunSummary  `json:"last_run"` // the newest finished run: succeeded, failed or skipped
	ActiveRun *RunProgress `json:"active_run"`
	NextRunAt *time.Time   `json:"next_run_at"` // null when manual, off or not configured
}

// Estimates are what a run and an ask cost lately.
type Estimates struct {
	RunUSD map[string]*float64 `json:"run_usd"` // per rec model: the mean of its last 5 succeeded runs
	AskUSD *float64            `json:"ask_usd"` // the mean of the last 20 done turns
}

// UsageReport is GET /settings/ai/usage, for the server-local month.
type UsageReport struct {
	Configured    bool           `json:"configured"`
	Month         string         `json:"month"`     // "2026-10"
	ResetsAt      time.Time      `json:"resets_at"` // the 1st of next month, 00:00
	TotalUSD      float64        `json:"total_usd"`
	BudgetUSD     *float64       `json:"budget_usd"`
	BudgetReached bool           `json:"budget_reached"`
	ByFeature     []FeatureUsage `json:"by_feature"` // recs, ask, classifier: always all three
	Recs          RecsUsage      `json:"recs"`
	Estimates     Estimates      `json:"estimates"`
	AsksToday     int            `json:"asks_today"`
	AskDailyLimit int            `json:"ask_daily_limit"`
}
