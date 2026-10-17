package downloads

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Errors of the API (PLAN §6 "Downloads (#77)").
var (
	// ErrNotAnObject is a body that isn't a JSON object: 400.
	ErrNotAnObject = errors.New("the body isn't a JSON object")
	// ErrNotFound is an id that isn't an active download (unknown,
	// released, or a smart pick while smart downloads are off): 404.
	ErrNotFound = errors.New("no such download")
	// ErrNotDownloadable is a Keep on files that all play from HDRezka:
	// 422 not_downloadable.
	ErrNotDownloadable = errors.New("this plays from HDRezka, so it can't be kept on disk")
	// ErrRepickBlocked is a smart pick of an episode the re-pick rules
	// rule out (UpsertSmart).
	ErrRepickBlocked = errors.New("the episode can't be a smart pick")
)

// FieldError is a bad value: 422 validation_error naming the field.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// StorageFullError is a Keep that doesn't fit the kept cap: 409
// storage_full. NeededBytes is what it adds to committed_kept_bytes (files
// another kept download has need nothing); AvailableBytes is what the cap
// has left.
type StorageFullError struct {
	NeededBytes    int64
	AvailableBytes int64
}

func (e *StorageFullError) Error() string {
	return fmt.Sprintf("Not enough space for kept downloads: this needs %s and %s is left.",
		gigabytes(e.NeededBytes), gigabytes(e.AvailableBytes))
}

func gigabytes(n int64) string { return fmt.Sprintf("%.1f GB", float64(n)/1e9) }

// keepRequest is POST /downloads' body.
type keepRequest struct {
	target       library.DownloadTarget
	untilWatched *bool
	setExpiry    bool
	expiresAt    *time.Time
}

// durationPatch is PATCH /downloads/{id}'s body, or a Keep's duration.
type durationPatch struct {
	untilWatched *bool
	setExpiry    bool
	expiresAt    *time.Time
}

// object decodes a JSON object's fields; ErrNotAnObject for anything else.
func object(raw []byte) (map[string]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, ErrNotAnObject
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, ErrNotAnObject
	}
	return fields, nil
}

// parseKeep reads a Keep, checking scope, id, season_number, until_watched
// and expires_at in that order.
func parseKeep(raw []byte, now time.Time) (keepRequest, error) {
	fields, err := object(raw)
	if err != nil {
		return keepRequest{}, err
	}
	var req keepRequest
	var scope string
	if json.Unmarshal(fields["scope"], &scope) != nil || !library.DownloadScope(scope).Valid() {
		return req, &FieldError{"scope", "scope must be movie, show, season or episode"}
	}
	req.target.Scope = library.DownloadScope(scope)
	id, ok := wholeNumber(fields["id"])
	if !ok || id <= 0 {
		return req, &FieldError{"id", "id must be a library id"}
	}
	req.target.ID = id
	if req.target.Scope == library.DownloadScopeSeason {
		n, ok := wholeNumber(fields["season_number"])
		if !ok || n < 0 || n > math.MaxInt32 {
			return req, &FieldError{"season_number", "season_number must be a season of the show"}
		}
		req.target.SeasonNumber = int(n)
	}
	d, err := parseDuration(fields, now)
	if err != nil {
		return req, err
	}
	req.untilWatched, req.setExpiry, req.expiresAt = d.untilWatched, d.setExpiry, d.expiresAt
	return req, nil
}

// parseDuration reads until_watched and expires_at: a boolean, and null or
// a time in the future.
func parseDuration(fields map[string]json.RawMessage, now time.Time) (durationPatch, error) {
	var d durationPatch
	if raw, ok := fields["until_watched"]; ok {
		var b bool
		if json.Unmarshal(raw, &b) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return d, &FieldError{"until_watched", "until_watched must be true or false"}
		}
		d.untilWatched = &b
	}
	if raw, ok := fields["expires_at"]; ok {
		d.setExpiry = true
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			var s string
			if json.Unmarshal(raw, &s) != nil {
				return d, &FieldError{"expires_at", "expires_at must be a time (RFC 3339) or null"}
			}
			at, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return d, &FieldError{"expires_at", "expires_at must be a time (RFC 3339) or null"}
			}
			if !at.After(now) {
				return d, &FieldError{"expires_at", "expires_at must be in the future"}
			}
			d.expiresAt = &at
		}
	}
	return d, nil
}

// wholeNumber reads a JSON integer (1, 1.0); false for anything else.
func wholeNumber(raw json.RawMessage) (int64, bool) {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, false
	}
	return int64(f), true
}

// ParseTarget reads GET /downloads/estimate's query: the same target as
// POST. A bad one is a *FieldError (the route answers 400 bad_request).
func ParseTarget(scope, id, seasonNumber string) (library.DownloadTarget, error) {
	t := library.DownloadTarget{Scope: library.DownloadScope(scope)}
	if !t.Scope.Valid() {
		return t, &FieldError{"scope", "scope must be movie, show, season or episode"}
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return t, &FieldError{"id", "id must be a library id"}
	}
	t.ID = n
	if t.Scope == library.DownloadScopeSeason {
		season, err := strconv.Atoi(seasonNumber)
		if err != nil || season < 0 || season > math.MaxInt32 {
			return t, &FieldError{"season_number", "season_number must be a season of the show"}
		}
		t.SeasonNumber = season
	}
	return t, nil
}
