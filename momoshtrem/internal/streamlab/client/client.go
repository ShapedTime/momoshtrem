// Package client is streamlab's WebDAV client (docs/streamlab/PLAN.md §5):
// the requests a player like Infuse makes, over real HTTP. A Device is one
// simulated player and a Conn one of its TCP connections.
package client

import (
	"cmp"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Device is a simulated player. Streaming sessions are keyed on the
// client's address, User-Agent and path, and every lab request comes from
// 127.0.0.1, so each device needs a User-Agent of its own.
type Device struct {
	// BaseURL is the WebDAV root, http://127.0.0.1:port.
	BaseURL   string
	UserAgent string
	// Username and Password are sent as Basic auth when Username is set.
	Username, Password string
}

// Conn opens a simulated connection: a Transport of its own, holding one
// TCP connection at a time. Its requests run one after another, as on one
// HTTP/1.1 connection, and never share a connection with another Conn's.
// Close it when done.
func (d Device) Conn() *Conn {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{}).DialContext,
		MaxConnsPerHost:     1,
		MaxIdleConnsPerHost: 1,
		DisableCompression:  true,
	}
	return &Conn{dev: d, tr: tr, hc: &http.Client{Transport: tr}}
}

// Conn is one simulated connection of a device.
type Conn struct {
	dev Device
	tr  *http.Transport
	hc  *http.Client
}

// Close drops the connection once its requests are done.
func (c *Conn) Close() { c.tr.CloseIdleConnections() }

// request builds a request for the WebDAV path p.
func (c *Conn) request(ctx context.Context, method, p string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.dev.BaseURL+(&url.URL{Path: p}).EscapedPath(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.dev.UserAgent)
	if c.dev.Username != "" {
		req.SetBasicAuth(c.dev.Username, c.dev.Password)
	}
	return req, nil
}

// do sends req and reads the whole body.
func (c *Conn) do(req *http.Request) (*http.Response, []byte, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, body, fmt.Errorf("%s %s: reading the body: %w", req.Method, req.URL.Path, err)
	}
	return resp, body, nil
}

// Entry is one resource of a PROPFIND answer.
type Entry struct {
	Path        string // unescaped; folders end in "/"
	IsDir       bool
	Size        int64
	ContentType string
	ETag        string
}

// propfindAll asks for every property (allprop): getcontenttype and
// getetag included, which is what makes a PROPFIND open each file it lists.
const propfindAll = `<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`

// Propfind lists p at depth (0 or 1), as a player browsing a folder does.
func (c *Conn) Propfind(ctx context.Context, p string, depth int) ([]Entry, error) {
	req, err := c.request(ctx, "PROPFIND", p, strings.NewReader(propfindAll))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", strconv.Itoa(depth))
	req.Header.Set("Content-Type", `application/xml; charset="utf-8"`)
	resp, body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("PROPFIND %s: %s", p, resp.Status)
	}
	return parseMultistatus(body)
}

// multistatus is a PROPFIND answer; the fields match any namespace.
type multistatus struct {
	Responses []struct {
		Href      string `xml:"href"`
		Propstats []struct {
			Status string `xml:"status"`
			Prop   struct {
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
				ContentLength string `xml:"getcontentlength"`
				ContentType   string `xml:"getcontenttype"`
				ETag          string `xml:"getetag"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func parseMultistatus(body []byte) ([]Entry, error) {
	var ms multistatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return nil, fmt.Errorf("PROPFIND: parsing the answer: %w", err)
	}
	entries := make([]Entry, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		p, err := url.PathUnescape(r.Href)
		if err != nil {
			return nil, fmt.Errorf("PROPFIND: href %q: %w", r.Href, err)
		}
		e := Entry{Path: p}
		for _, ps := range r.Propstats {
			if !strings.Contains(ps.Status, " 200 ") {
				continue
			}
			prop := ps.Prop
			e.IsDir = e.IsDir || prop.ResourceType.Collection != nil
			if prop.ContentLength != "" {
				if e.Size, err = strconv.ParseInt(prop.ContentLength, 10, 64); err != nil {
					return nil, fmt.Errorf("PROPFIND: %s: getcontentlength %q", p, prop.ContentLength)
				}
			}
			e.ContentType = cmp.Or(e.ContentType, prop.ContentType)
			e.ETag = cmp.Or(e.ETag, prop.ETag)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Head asks for p's headers, as a player does before playing.
func (c *Conn) Head(ctx context.Context, p string) (*http.Response, error) {
	req, err := c.request(ctx, http.MethodHead, p, nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := c.do(req)
	return resp, err
}

// Probe reads n bytes of p at off with one bounded Range GET, as a player
// sniffs a header or a footer. It returns the answer, its body already
// read, and the body.
func (c *Conn) Probe(ctx context.Context, p string, off, n int64) (*http.Response, []byte, error) {
	if off < 0 || n <= 0 {
		return nil, nil, fmt.Errorf("probe of %d bytes at %d", n, off)
	}
	req, err := c.request(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+n-1))
	return c.do(req)
}

// Play starts an open-ended Range GET of p from the byte from, as a player
// starts or resumes playback, and returns once the headers are in. Read
// the body from the Stream, then Close it, or Abort it to seek away.
func (c *Conn) Play(ctx context.Context, p string, from int64) (*Stream, error) {
	st, err := c.Get(ctx, p, http.Header{"Range": {fmt.Sprintf("bytes=%d-", from)}})
	if err != nil {
		return nil, err
	}
	st.From = from // the caller checks the answer is the range it asked for
	return st, nil
}

// Get sends a GET of p with the extra headers (Range, If-Range) and
// returns once the answer's headers are in: read the body from the
// Stream, then Close it, or Abort it. From is the first byte of a 206's
// Content-Range, 0 otherwise.
func (c *Conn) Get(ctx context.Context, p string, header http.Header) (*Stream, error) {
	ctx, cancel := context.WithCancel(ctx)
	req, err := c.request(ctx, http.MethodGet, p, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	var from int64
	if resp.StatusCode == http.StatusPartialContent {
		fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-", &from)
	}
	return &Stream{Response: resp, From: from, cancel: cancel}, nil
}

// ErrAborted is a read from a Stream after Abort.
var ErrAborted = errors.New("client: stream aborted")

// Stream is a playback's GET: its answer, and its body read as it
// arrives.
type Stream struct {
	// Response is the answer; read the body through the Stream.
	Response *http.Response
	// From is the offset the body starts at.
	From int64

	mu      sync.Mutex
	read    int64
	cancel  context.CancelFunc
	aborted bool
}

// Read reads the body.
func (s *Stream) Read(p []byte) (int, error) {
	n, err := s.Response.Body.Read(p)
	s.mu.Lock()
	s.read += int64(n)
	aborted := s.aborted
	s.mu.Unlock()
	if err != nil && aborted {
		err = ErrAborted
	}
	return n, err
}

// Pos is the file offset of the next byte to read: From plus what was read.
func (s *Stream) Pos() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.From + s.read
}

// Abort drops the request mid-body, as a player that seeks away does: the
// transport closes the connection, and the server sees the client gone.
func (s *Stream) Abort() {
	s.mu.Lock()
	s.aborted = true
	s.mu.Unlock()
	s.cancel()
	s.Response.Body.Close()
}

// Close closes the body. Closed before its end, the connection is dropped
// as with Abort.
func (s *Stream) Close() error {
	err := s.Response.Body.Close()
	s.cancel()
	return err
}
