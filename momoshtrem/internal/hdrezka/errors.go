package hdrezka

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound means HDRezka has no such title, or nothing to play for
	// the requested translation, season or episode.
	ErrNotFound = errors.New("not found on HDRezka")
	// ErrInvalidPath means a URL or path isn't an HDRezka title page.
	ErrInvalidPath = errors.New("not an HDRezka title URL or path")
	// ErrChallenge means the site answered with an anti-bot page (Anubis,
	// Cloudflare or DDoS-Guard) instead of content. Retrying doesn't help.
	ErrChallenge = errors.New("HDRezka answered with an anti-bot challenge page")
	// ErrPremium means the translation needs an HDRezka Premium account:
	// without one the CDN serves a teaser instead of the video.
	ErrPremium = errors.New("this translation needs an HDRezka Premium account")
	// ErrNoMP4 means the streams are HLS only, with no progressive MP4.
	ErrNoMP4 = errors.New("HDRezka offers no MP4 file for this stream")
	// ErrLinkExpired means the CDN refused a signed link (403, 404 or 410):
	// it has expired or is bound to another IP. Resolve the stream again.
	ErrLinkExpired = errors.New("the HDRezka CDN link has expired")
	// ErrRangeIgnored means the CDN answered a Range request with the whole
	// file (200), so the file can't be seeked.
	ErrRangeIgnored = errors.New("the HDRezka CDN ignored a Range request")
	// ErrNoQuality means the stream no longer offers the quality asked for.
	ErrNoQuality = errors.New("HDRezka no longer offers this quality")
	// ErrSizeMismatch means the CDN's file isn't the size stored when it was
	// assigned: the upload changed, so the bytes no longer line up with what
	// the player read before.
	ErrSizeMismatch = errors.New("the HDRezka file's size changed")
)

// ParseError means a page or response no longer has the shape the client
// expects: the site's markup changed.
type ParseError struct {
	What string // what was missing, e.g. "player init call"
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("HDRezka markup changed: cannot read the %s", e.What)
}

// StatusError is an HTTP status HDRezka or its CDN answered with that has no
// sentinel.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HDRezka answered HTTP %d", e.StatusCode)
}

// CDNError is a get_cdn_series answer with success=false. Message is the
// site's own text (usually Russian), e.g. for an episode that doesn't exist.
type CDNError struct {
	Message string
}

func (e *CDNError) Error() string {
	if e.Message == "" {
		return "HDRezka refused the stream request"
	}
	return "HDRezka refused the stream request: " + e.Message
}
