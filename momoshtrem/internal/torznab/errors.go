package torznab

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidBaseURL means JACKETT_URL isn't an absolute http(s) URL.
	ErrInvalidBaseURL = errors.New("invalid JACKETT_URL")
	// ErrUnknownIndexer means the indexer isn't in Jackett's configured list.
	ErrUnknownIndexer = errors.New("indexer is not configured in Jackett")
	// ErrUnsupported means the indexer can't run the query: it supports none
	// of the query's categories, or nothing searchable is left once the
	// parameters it doesn't support are dropped. No request is made.
	ErrUnsupported = errors.New("indexer can't run this query")
	// ErrTimeout means a request didn't finish in time.
	ErrTimeout = errors.New("timed out")
	// ErrInvalidLink means a download link isn't a Jackett /dl/ link.
	ErrInvalidLink = errors.New("not a Jackett download link")
	// ErrRedirect means Jackett redirected a download somewhere other than a
	// magnet link.
	ErrRedirect = errors.New("jackett redirected the download to something other than a magnet link")
	// ErrTooLarge means a .torrent file is over the 5 MB cap.
	ErrTooLarge = errors.New("the .torrent file is over 5 MB")
)

// Error is a Torznab error document, e.g.
// <error code="100" description="Invalid API Key"/>. Jackett answers with
// one for a bad API key (code 100) and for an indexer that failed (900).
// Description is cleaned: first line only, no URLs and no API key.
type Error struct {
	Code        int
	Description string
}

func (e *Error) Error() string {
	return "jackett error: " + e.Description
}

// HTTPError is an HTTP status Jackett answered with that carried no
// Torznab error document. Message is Jackett's own text when it sent one,
// cleaned like Error.Description.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	switch {
	case e.StatusCode == 401 || e.StatusCode == 403:
		return fmt.Sprintf("jackett rejected the API key (HTTP %d)", e.StatusCode)
	case e.Message != "":
		return fmt.Sprintf("jackett answered HTTP %d: %s", e.StatusCode, e.Message)
	default:
		return fmt.Sprintf("unexpected response (HTTP %d)", e.StatusCode)
	}
}
