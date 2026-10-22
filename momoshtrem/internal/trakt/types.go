package trakt

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// What Trakt answers.

// DeviceAuthorization is POST /oauth/device/code's answer. DeviceCode is
// what the poller sends; the user enters UserCode at VerificationURL.
type DeviceAuthorization struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"` // seconds
	Interval        int    `json:"interval"`   // seconds between polls
}

// TokenSet is what the token endpoints answer.
type TokenSet struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"` // Unix seconds
}

// Created is when Trakt issued the tokens.
func (t TokenSet) Created() time.Time { return time.Unix(t.CreatedAt, 0).UTC() }

// Expires is when the access token stops working.
func (t TokenSet) Expires() time.Time {
	return t.Created().Add(time.Duration(t.ExpiresIn) * time.Second)
}

// UserSettings is the part of GET /users/settings momoshtrem reads.
type UserSettings struct {
	User struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		VIP      bool   `json:"vip"`
		IDs      struct {
			Slug string `json:"slug"`
		} `json:"ids"`
	} `json:"user"`
}

// What momoshtrem answers (PLAN §6 "Trakt (B5)", web/src/lib/api/contract/trakt.ts).

// State is the connection's: off → pending (a device code) → connected ↔
// reauth.
type State string

const (
	StateOff       State = "off"
	StatePending   State = "pending"
	StateConnected State = "connected"
	StateReauth    State = "reauth"
)

// ConnectError is why the last device code didn't connect.
type ConnectError string

const (
	ConnectExpired ConnectError = "expired"
	ConnectDenied  ConnectError = "denied" // free accounts can connect only 2 apps
	ConnectFailed  ConnectError = "failed"
	// ConnectAccountInUse: another household member has connected the
	// account (an account connects to one member).
	ConnectAccountInUse ConnectError = "account_in_use"
)

// DeviceCode is the live device code for the user (TraktConnect).
type DeviceCode struct {
	UserCode        string    `json:"user_code"`
	VerificationURL string    `json:"verification_url"`
	ExpiresAt       time.Time `json:"expires_at"`
	Interval        int       `json:"interval"` // seconds between the backend's checks
}

// Integration is GET /integrations/trakt (TraktIntegration).
type Integration struct {
	Configured   bool                  `json:"configured"`
	State        State                 `json:"state"`
	Connected    bool                  `json:"connected"`
	Username     *string               `json:"username"`
	Pending      *DeviceCode           `json:"pending"`
	ConnectError *ConnectError         `json:"connect_error"`
	ReauthReason *string               `json:"reauth_reason"`
	LastSyncAt   *time.Time            `json:"last_sync_at"`
	NextSyncAt   *time.Time            `json:"next_sync_at"`
	Sync         SyncRun               `json:"sync"`
	Counts       library.TraktCounts   `json:"counts"`
	Settings     library.TraktSettings `json:"settings"`
}

// SyncStatus is the sync's part of the integration, from Hooks.SyncStatus.
type SyncStatus struct {
	// LastSyncAt is when the last sync that finished without an error did.
	LastSyncAt *time.Time
	// NextSyncAt is the next scheduled check (left out unless connected).
	NextSyncAt *time.Time
	// Run is the running sync, else the last one.
	Run SyncRun
}

// SyncState is a sync's status.
type SyncState string

const (
	SyncIdle    SyncState = "idle"
	SyncSyncing SyncState = "syncing"
	SyncError   SyncState = "error"
)

// SyncKind is what a sync does; initial is the first import after a
// connect.
type SyncKind string

const (
	SyncInitial     SyncKind = "initial"
	SyncIncremental SyncKind = "incremental"
	SyncReconcile   SyncKind = "reconcile"
)

// SyncStep is an import step, in order.
type SyncStep string

const (
	StepHistory   SyncStep = "history"
	StepRatings   SyncStep = "ratings"
	StepWatchlist SyncStep = "watchlist"
	StepProgress  SyncStep = "progress"
	StepDetails   SyncStep = "details"
)

// SyncRun is the running sync, else the last one (TraktSync). The zero
// value, with Status idle, is "never synced".
type SyncRun struct {
	Status     SyncState    `json:"status"`
	Kind       *SyncKind    `json:"kind"` // null before the first sync
	Step       *SyncStep    `json:"step"` // the running step or the one that failed; null while idle
	Progress   SyncProgress `json:"progress"`
	Error      *string      `json:"error"` // for the user, while error
	StartedAt  *time.Time   `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at"` // null while syncing
}

// SyncProgress is what a run imported; PlaysTotal is Trakt's count, 0
// until it's known.
type SyncProgress struct {
	Plays      int `json:"plays"`
	PlaysTotal int `json:"plays_total"`
	Ratings    int `json:"ratings"`
	Watchlist  int `json:"watchlist"`
	Shows      int `json:"shows"`
}
