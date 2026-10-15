package hdrezka

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// maxSubtitle caps a subtitle file: a feature film's WebVTT is about
// 200 KB.
const maxSubtitle = 2 << 20

// Subtitle downloads a subtitle track (a signed VTT link from
// Stream.Subtitles) from the CDN, retried on 5xx, 429 and timeouts like
// ContentLength. A refused link is ErrLinkExpired; a file over 2 MB is
// ErrSubtitleTooLarge. Errors never carry the link.
func (c *Client) Subtitle(ctx context.Context, link string) ([]byte, error) {
	var body []byte
	err := c.retry(ctx, func() (bool, error) {
		b, retry, err := c.subtitle(ctx, link)
		body = b
		return retry, err
	})
	return body, err
}

func (c *Client) subtitle(ctx context.Context, link string) (body []byte, retry bool, err error) {
	resp, retry, err := c.cdnDo(ctx, http.MethodGet, link, "")
	if err != nil {
		return nil, retry, err
	}
	defer resp.Body.Close()
	switch code := resp.StatusCode; {
	case code == http.StatusOK:
	case linkRefused(code):
		return nil, false, ErrLinkExpired
	case code == http.StatusTooManyRequests || code >= 500:
		return nil, true, &StatusError{StatusCode: code}
	default:
		return nil, false, &StatusError{StatusCode: code}
	}
	if resp.ContentLength > maxSubtitle {
		return nil, false, ErrSubtitleTooLarge
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxSubtitle+1))
	if err != nil {
		err = withoutURL(err)
		return nil, isTimeout(err) && ctx.Err() == nil, fmt.Errorf("failed to read the subtitle: %w", err)
	}
	if len(body) > maxSubtitle {
		return nil, false, ErrSubtitleTooLarge
	}
	return body, false, nil
}
