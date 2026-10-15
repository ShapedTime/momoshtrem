package hdrezka

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// File is one HDRezka file: a stream in one quality.
type File struct {
	Key     StreamKey
	Quality string // its label, e.g. "1080p"
}

// LinkResolver gives a file's signed MP4 link, and forgets a stream's links
// once the CDN refuses one. *Resolver implements it.
type LinkResolver interface {
	Link(ctx context.Context, key StreamKey, quality string) (string, error)
	Invalidate(key StreamKey)
}

var _ LinkResolver = (*Resolver)(nil)

const (
	// defaultMaxBodies is how many CDN bodies may be open at once.
	defaultMaxBodies = 8
	// streamBackoff is the wait before the one retry after a 429 or 5xx.
	streamBackoff = time.Second
)

// Streamer opens the bytes of HDRezka files on the CDN with Range
// requests. Its HTTP client has transport timeouts only: a body is read for
// as long as the movie plays, so there's no overall timeout. At most 8
// bodies are open at once. Safe for concurrent use.
type Streamer struct {
	links     LinkResolver
	http      *http.Client
	userAgent string
	slots     chan struct{}
	backoff   time.Duration
	sleep     func(ctx context.Context, d time.Duration) error
}

// StreamerOption configures a Streamer.
type StreamerOption func(*Streamer)

// WithStreamHTTPClient replaces the streamer's HTTP client. Don't give it
// an overall Timeout: that would cut long streams.
func WithStreamHTTPClient(h *http.Client) StreamerOption {
	return func(s *Streamer) { s.http = h }
}

// WithStreamUserAgent replaces DefaultUserAgent.
func WithStreamUserAgent(ua string) StreamerOption {
	return func(s *Streamer) { s.userAgent = ua }
}

// WithMaxBodies sets how many bodies may be open at once (8 by default).
func WithMaxBodies(n int) StreamerOption {
	return func(s *Streamer) { s.slots = make(chan struct{}, max(n, 1)) }
}

// NewStreamer creates a Streamer that gets links from links.
func NewStreamer(links LinkResolver, opts ...StreamerOption) *Streamer {
	s := &Streamer{
		links:     links,
		http:      &http.Client{Transport: newStreamTransport()},
		userAgent: DefaultUserAgent,
		slots:     make(chan struct{}, defaultMaxBodies),
		backoff:   streamBackoff,
		sleep:     sleepContext,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// newStreamTransport bounds connecting and waiting for headers, but not
// reading the body. HTTP/1.1 only: each body gets its own connection, so
// closing one mid-stream (a seek) doesn't stall the others.
func newStreamTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
	}
}

// Open returns f's bytes from off to the end. size is the file's size as
// stored when it was assigned: the CDN must agree (ErrSizeMismatch
// otherwise). ctx bounds the opening only (a free slot, the link and the
// response headers); the body outlives it, and Close releases its slot.
//
// A link the CDN refuses (403, 404, 410) is resolved again and tried once
// more; a 429 or 5xx, or a transport error, is retried once after a
// second. Errors never carry the signed link.
func (s *Streamer) Open(ctx context.Context, f File, off, size int64) (io.ReadCloser, error) {
	if size <= 0 || off < 0 || off >= size {
		return nil, fmt.Errorf("invalid range: byte %d of %d", off, size)
	}
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	body, err := s.open(ctx, f, off, size)
	if err != nil {
		<-s.slots
		return nil, err
	}
	return body, nil
}

func (s *Streamer) open(ctx context.Context, f File, off, size int64) (io.ReadCloser, error) {
	refreshed, retried := false, false
	for {
		link, err := s.links.Link(ctx, f.Key, f.Quality)
		if err != nil {
			return nil, err
		}
		body, retry, err := s.get(ctx, link, off, size)
		switch {
		case err == nil:
			return body, nil
		case errors.Is(err, ErrLinkExpired) && !refreshed:
			refreshed = true
			s.links.Invalidate(f.Key)
		case retry && !retried:
			retried = true
			if err := s.sleep(ctx, s.backoff); err != nil {
				return nil, err
			}
		default:
			return nil, err
		}
	}
}

// get requests the link from off and checks the answer is that part of a
// file of size bytes. retry reports a transient failure.
func (s *Streamer) get(ctx context.Context, link string, off, size int64) (_ io.ReadCloser, retry bool, _ error) {
	// The request outlives ctx: ctx may only cancel it until the headers
	// are in, after which closing the body does.
	reqCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, link, nil)
	if err != nil {
		cancel()
		return nil, false, errors.New("invalid CDN link")
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(off, 10)+"-")

	resp, err := s.http.Do(req)
	if !stop() { // ctx ended while waiting: the request is cancelled
		if err == nil {
			resp.Body.Close()
		}
		cancel()
		return nil, false, ctx.Err()
	}
	if err != nil {
		// A timeout or a dropped connection: worth one more try.
		cancel()
		return nil, true, fmt.Errorf("CDN request failed: %w", withoutURL(err))
	}
	if retry, err := checkPart(resp, off, size); err != nil {
		resp.Body.Close() // without draining: it may be the whole file
		cancel()
		return nil, retry, err
	}
	return &streamBody{ReadCloser: resp.Body, cancel: cancel, release: func() { <-s.slots }}, false, nil
}

// checkPart accepts a 206 for exactly bytes off- of a size-byte file, or a
// 200 with the whole file when off is 0.
func checkPart(resp *http.Response, off, size int64) (retry bool, err error) {
	switch code := resp.StatusCode; {
	case code == http.StatusPartialContent:
		start, _, total, err := ParseContentRange(resp.Header.Get("Content-Range"))
		switch {
		case err != nil:
			return false, fmt.Errorf("the HDRezka CDN sent an invalid Content-Range: %w", err)
		case start != off:
			return false, fmt.Errorf("the HDRezka CDN answered from byte %d instead of %d", start, off)
		case total != size:
			return false, fmt.Errorf("%w: the CDN has %s bytes, %d were stored", ErrSizeMismatch, totalString(total), size)
		}
		return false, nil
	case code == http.StatusOK:
		if off != 0 {
			return false, ErrRangeIgnored
		}
		if resp.ContentLength != size {
			return false, fmt.Errorf("%w: the CDN has %s bytes, %d were stored", ErrSizeMismatch, totalString(resp.ContentLength), size)
		}
		return false, nil
	case code == http.StatusRequestedRangeNotSatisfiable:
		return false, fmt.Errorf("%w: the CDN's file ends before byte %d of %d", ErrSizeMismatch, off, size)
	case linkRefused(code):
		return false, ErrLinkExpired
	case code == http.StatusTooManyRequests || code >= 500:
		return true, &StatusError{StatusCode: code}
	default:
		return false, &StatusError{StatusCode: code}
	}
}

func totalString(n int64) string {
	if n < 0 {
		return "an unknown number of"
	}
	return strconv.FormatInt(n, 10)
}

// streamBody is an open CDN body. Close cancels its request, which also
// ends a Read blocked on it, and frees its slot; it's safe to call from
// another goroutine and more than once.
type streamBody struct {
	io.ReadCloser
	cancel  context.CancelFunc
	release func()
	once    sync.Once
}

func (b *streamBody) Close() error {
	var err error
	b.once.Do(func() {
		b.cancel()
		err = b.ReadCloser.Close()
		b.release()
	})
	return err
}
