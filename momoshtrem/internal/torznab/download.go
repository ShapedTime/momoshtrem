package torznab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Download fetches a result's Link and returns either a magnet link or the
// .torrent file's bytes (at most 5 MB, within 15 s). A magnet Link comes
// back as it is.
//
// The SSRF guard: only the link's path and query are fetched, from the
// configured Jackett, never from the link's own host. Jackett's links point
// at its own download proxy, {base}/dl/{indexer}/?jackett_apikey=…, so any
// other path is ErrInvalidLink. Redirects aren't followed: one to a magnet
// link is the answer, and any other is ErrRedirect.
func (c *Client) Download(ctx context.Context, link string) (magnet string, torrent []byte, err error) {
	defer func() {
		if err != nil && !errors.Is(err, context.Canceled) {
			c.log.Warn("Torznab download failed", "component", "torznab", "error", c.SafeError(err))
		}
	}()
	if isMagnet(link) {
		return strings.TrimSpace(link), nil, nil
	}
	target, err := c.rebase(link)
	if err != nil {
		return "", nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", nil, ErrInvalidLink
	}
	start := time.Now()
	resp, err := c.noRedir.Do(req)
	if err != nil {
		return "", nil, c.transportError(ctx, err, start)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
		if loc := resp.Header.Get("Location"); isMagnet(loc) {
			return strings.TrimSpace(loc), nil, nil
		}
		return "", nil, ErrRedirect
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrent+1))
	if err != nil {
		return "", nil, fmt.Errorf("reading the .torrent file: %w", c.transportError(ctx, err, start))
	}
	if e, ok := c.torznabError(body); ok {
		return "", nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", nil, &HTTPError{StatusCode: resp.StatusCode, Message: c.jsonErrorText(body)}
	}
	if len(body) > maxTorrent {
		return "", nil, ErrTooLarge
	}
	if text := bytes.TrimSpace(body); isMagnet(string(text)) {
		return string(text), nil, nil
	}
	if len(body) == 0 {
		return "", nil, errors.New("jackett returned an empty .torrent file")
	}
	return "", body, nil
}

// rebase moves a Jackett download link onto the configured base URL,
// keeping only its path and query.
func (c *Client) rebase(link string) (string, error) {
	if c.base == nil {
		return "", ErrInvalidBaseURL
	}
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.HasPrefix(u.Path, c.base.Path+"/dl/") || strings.Contains(u.Path, "..") {
		return "", ErrInvalidLink
	}
	target := *c.base
	target.Path, target.RawPath = u.Path, u.RawPath
	target.RawQuery = u.RawQuery
	return target.String(), nil
}

func isMagnet(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "magnet:?")
}
