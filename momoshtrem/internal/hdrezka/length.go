package hdrezka

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ContentLength returns the size of the file behind a CDN link. It asks with
// HEAD, following the CDN's redirect to an edge host, and when that gives no
// length, with a one-byte Range request whose Content-Range carries the
// total. A refused link is ErrLinkExpired. CDN requests aren't rate-limited.
func (c *Client) ContentLength(ctx context.Context, link string) (int64, error) {
	var size int64
	err := c.retry(ctx, func() (bool, error) {
		n, retry, err := c.headLength(ctx, link)
		size = n
		return retry, err
	})
	if err != nil || size > 0 {
		return size, err
	}
	err = c.retry(ctx, func() (bool, error) {
		n, retry, err := c.rangeLength(ctx, link)
		size = n
		return retry, err
	})
	return size, err
}

// headLength returns the Content-Length of a 200 answer to HEAD, or 0 when
// there is none or the CDN doesn't answer HEAD.
func (c *Client) headLength(ctx context.Context, link string) (size int64, retry bool, err error) {
	resp, retry, err := c.cdnDo(ctx, http.MethodHead, link, "")
	if err != nil {
		return 0, retry, err
	}
	resp.Body.Close()
	switch code := resp.StatusCode; {
	case code == http.StatusOK:
		return max(resp.ContentLength, 0), false, nil
	case linkRefused(code):
		return 0, false, ErrLinkExpired
	case code == http.StatusTooManyRequests || code >= 500:
		return 0, true, &StatusError{StatusCode: code}
	default:
		return 0, false, nil // e.g. 405: ask with Range instead
	}
}

// rangeLength asks for the first byte and reads the total size from the
// 206's Content-Range.
func (c *Client) rangeLength(ctx context.Context, link string) (size int64, retry bool, err error) {
	resp, retry, err := c.cdnDo(ctx, http.MethodGet, link, "bytes=0-0")
	if err != nil {
		return 0, retry, err
	}
	// Close without draining: a 200 here is the whole file.
	defer resp.Body.Close()
	switch code := resp.StatusCode; {
	case code == http.StatusPartialContent:
		_, _, total, err := ParseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return 0, false, err
		}
		if total < 0 {
			return 0, false, errors.New("the HDRezka CDN didn't say the file's size")
		}
		return total, false, nil
	case code == http.StatusOK:
		return 0, false, ErrRangeIgnored
	case linkRefused(code):
		return 0, false, ErrLinkExpired
	case code == http.StatusTooManyRequests || code >= 500:
		return 0, true, &StatusError{StatusCode: code}
	default:
		return 0, false, &StatusError{StatusCode: code}
	}
}

// cdnDo makes one CDN request with the client's user agent and an optional
// Range header.
func (c *Client) cdnDo(ctx context.Context, method, link, byteRange string) (*http.Response, bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, link, nil)
	if err != nil {
		return nil, false, errors.New("invalid CDN link")
	}
	req.Header.Set("User-Agent", c.userAgent)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = withoutURL(err)
		return nil, isTimeout(err) && ctx.Err() == nil, fmt.Errorf("CDN request failed: %w", err)
	}
	return resp, false, nil
}

// linkRefused reports the statuses the CDN gives an expired or foreign
// signed link.
func linkRefused(code int) bool {
	return code == http.StatusForbidden || code == http.StatusNotFound || code == http.StatusGone
}

// ParseContentRange reads a 206's "bytes 0-1023/3322740623". total is -1
// when the server wrote "*" (unknown).
func ParseContentRange(h string) (start, end, total int64, err error) {
	bad := fmt.Errorf("invalid Content-Range %q", h)
	spec, ok := strings.CutPrefix(strings.TrimSpace(h), "bytes ")
	if !ok {
		return 0, 0, 0, bad
	}
	span, size, ok := strings.Cut(spec, "/")
	if !ok {
		return 0, 0, 0, bad
	}
	first, last, ok := strings.Cut(span, "-")
	if !ok {
		return 0, 0, 0, bad
	}
	if start, err = strconv.ParseInt(first, 10, 64); err != nil || start < 0 {
		return 0, 0, 0, bad
	}
	if end, err = strconv.ParseInt(last, 10, 64); err != nil || end < start {
		return 0, 0, 0, bad
	}
	if size == "*" {
		return start, end, -1, nil
	}
	if total, err = strconv.ParseInt(size, 10, 64); err != nil || total <= end {
		return 0, 0, 0, bad
	}
	return start, end, total, nil
}
