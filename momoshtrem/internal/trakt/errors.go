package trakt

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	// ErrUnauthorized is an API 401: Trakt didn't accept the access token.
	// Stop using it: Auth.Invalidate (Auth.Get does it).
	ErrUnauthorized = errors.New("trakt rejected the access token (HTTP 401)")
	// ErrInvalidGrant means Trakt refused the refresh token (HTTP 400 or
	// 401): the connection needs reauth.
	ErrInvalidGrant = errors.New("trakt refused the refresh token")

	// The answers to a device-code poll (POST /oauth/device/token).
	ErrAuthorizationPending = errors.New("the device code isn't approved yet") // 400
	ErrSlowDown             = errors.New("polling the device code too fast")   // 429
	ErrInvalidDeviceCode    = errors.New("trakt doesn't know the device code") // 404
	ErrDeviceCodeUsed       = errors.New("the device code was already used")   // 409
	ErrDeviceCodeExpired    = errors.New("the device code expired")            // 410
	ErrDeviceCodeDenied     = errors.New("the user denied the device code")    // 418

	// ErrNotConfigured means TRAKT_CLIENT_ID, TRAKT_CLIENT_SECRET or a valid
	// SECRET_KEY is missing.
	ErrNotConfigured = errors.New("trakt isn't configured")
	// ErrConnected answers a connect while connected.
	ErrConnected = errors.New("trakt is already connected")
	// ErrNotConnected means there are no tokens to use: off, or a code
	// pending from off.
	ErrNotConnected = errors.New("trakt isn't connected")
	// ErrReauth means Trakt stopped accepting the tokens: connect again.
	ErrReauth = errors.New("trakt needs connecting again")
	// ErrUnavailable means Trakt's device-code endpoint failed.
	ErrUnavailable = errors.New("trakt is unavailable")
	// ErrSyncUnavailable answers POST /integrations/trakt/sync until the
	// sync (#132) is plugged in.
	ErrSyncUnavailable = errors.New("trakt sync isn't available")
)

// StatusError is an HTTP status Trakt answered with. Code is the OAuth
// error code when Trakt sent one (e.g. "invalid_grant"); nothing from a
// token or a key ever gets in.
type StatusError struct {
	StatusCode int
	Code       string
}

func (e *StatusError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("trakt answered HTTP %d (%s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("trakt answered HTTP %d", e.StatusCode)
}

// Is makes a 401 ErrUnauthorized.
func (e *StatusError) Is(target error) bool {
	return target == ErrUnauthorized && e.StatusCode == http.StatusUnauthorized
}
