// Package cdn is streamlab's fake HDRezka CDN (docs/streamlab/PLAN.md §5):
// an httptest server that serves lab media with http.ServeContent, as the
// CDN serves an HDRezka stream's MP4 with Range requests, and the faults a
// real one has: refused links once they expire (403, 404, 410), bursts of
// 503, and slow, dropped and stalled bodies.
//
// Links is a fake hdrezka.LinkResolver over it: the lab's stack wires it
// through production's hdrezka.Streamer and service.HDRezkaSource
// (stack.Options.HDRezka). Every byte the CDN serves comes from
// media.Layout.ReadAt, which the integrity check reads too.
package cdn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// segment is the most a body sends at a time: a slow or stalled body
// changes pace between segments.
const segment = 16 << 10

// File is a file the CDN serves: an HDRezka stream in one quality, whose
// bytes are a layout's file.
type File struct {
	// Key and Quality name the stream, as an assignment's StreamRef does
	// (service.StreamFile).
	Key     hdrezka.StreamKey
	Quality string
	// Size is the file's size, the one the assignment stores.
	Size int64

	id     string
	layout *media.Layout
	index  int
}

// Stats is what the CDN did.
type Stats struct {
	// Requests are the GETs it got; Bodies the ones it answered with the
	// file (206, or 200 for all of it).
	Requests, Bodies int
	// Refused are the requests whose link had expired (403, 404, 410), and
	// Unavailable the ones answered 503.
	Refused, Unavailable int
	// Stalled and Dropped count the bodies a fault stalled or cut.
	Stalled, Dropped int
	// Served is the body bytes it sent.
	Served int64
	// Open is the bodies open now, PeakOpen the most at once.
	Open, PeakOpen int
}

// Request is one request the CDN answered.
type Request struct {
	At time.Time
	// From is the first byte its Range asked for; Status what the CDN
	// answered.
	From   int64
	Status int
	// Sent is the body bytes sent so far, and End when its handler
	// returned (zero while it runs).
	Sent int64
	End  time.Time
	// Stalled and Dropped are set when a fault stalled or cut the body.
	Stalled, Dropped bool
}

// request is a Request as it's recorded: its handler updates it.
type request struct {
	Request
	sent atomic.Int64
}

// CDN is the fake CDN. Its methods are safe for concurrent use.
type CDN struct {
	srv  *httptest.Server
	done chan struct{} // closed by Close: stalled bodies end

	mu          sync.Mutex
	files       map[string]*File
	gen         int // links of an older generation are refused
	refuse      int // with this status
	unavailable int // the next requests answered 503
	rate        int64
	stallUntil  time.Time // bodies opened before it stall too
	bodies      map[*body]struct{}
	stats       Stats
	log         []*request
	served      atomic.Int64
}

// Start starts a CDN on 127.0.0.1. It closes each connection after its
// response: the streamer's transport then keeps no idle connection, whose
// goroutines carry the request label of the WebDAV request that dialed it
// and would read as a leak (production's pool of idle CDN connections is
// bounded and isn't one). Close it when done.
func Start() *CDN {
	c := &CDN{done: make(chan struct{}), files: map[string]*File{}, gen: 1, refuse: http.StatusForbidden,
		bodies: map[*body]struct{}{}}
	c.srv = httptest.NewUnstartedServer(http.HandlerFunc(c.serve))
	c.srv.Config.SetKeepAlivesEnabled(false)
	c.srv.Start()
	return c
}

// URL is the CDN's root, http://127.0.0.1:port.
func (c *CDN) URL() string { return c.srv.URL }

// Add serves file i of l as the stream key in quality, and returns it.
func (c *CDN) Add(l *media.Layout, i int, key hdrezka.StreamKey, quality string) *File {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := &File{Key: key, Quality: quality, Size: l.Files()[i].Size, id: strconv.Itoa(len(c.files) + 1), layout: l, index: i}
	c.files[f.id] = f
	return f
}

// mintAll signs a link to each of the stream's files, by quality, valid
// until the next Expire: what a resolve of the stream gives.
func (c *CDN) mintAll(stream hdrezka.StreamKey) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	links := map[string]string{}
	for _, f := range c.files {
		if streamID(f.Key) == stream {
			links[f.Quality] = fmt.Sprintf("%s/s/%d/%s.mp4", c.srv.URL, c.gen, f.id)
		}
	}
	return links
}

// Expire makes every link issued so far expire: from now on the CDN
// refuses them with status (403, 404 or 410), as the real one refuses an
// expired or foreign link. Bodies already open carry on.
func (c *CDN) Expire(status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.refuse = status
}

// Unavailable answers the next n requests with 503: a burst.
func (c *CDN) Unavailable(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unavailable += n
}

// SetRate caps each body at bps bytes per second, open ones included; 0
// is unlimited.
func (c *CDN) SetRate(bps int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rate = bps
	for b := range c.bodies {
		b.setRate(bps)
	}
}

// Stall stops every open body: it sends nothing more until its client
// goes away. It returns how many it stalled.
func (c *CDN) Stall() int { return c.StallFor(0) }

// StallFor stalls every open body, and for d every new one after its
// first segment (its headers and some bytes, as a CDN that stops
// mid-transfer): each sends nothing more until its client goes away. It
// returns how many open bodies it stalled.
func (c *CDN) StallFor(d time.Duration) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stallUntil = time.Now().Add(d)
	for b := range c.bodies {
		c.stall(b)
	}
	return len(c.bodies)
}

// stall stalls b once. Called with c.mu held.
func (c *CDN) stall(b *body) {
	if b.stalled.CompareAndSwap(false, true) {
		b.req.Stalled = true
		c.stats.Stalled++
	}
}

// Drop cuts every open body: its connection closes before the body's
// end. It returns how many it cut.
func (c *CDN) Drop() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	for b := range c.bodies {
		if b.dropped.CompareAndSwap(false, true) {
			b.req.Dropped = true
			c.stats.Dropped++
		}
	}
	return len(c.bodies)
}

// Open is how many bodies are open now: a request the CDN is answering
// with the file, whose handler hasn't returned.
func (c *CDN) Open() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

// Stats is what the CDN did so far.
func (c *CDN) Stats() Stats {
	c.mu.Lock()
	st := c.stats
	st.Open = len(c.bodies)
	c.mu.Unlock()
	st.Served = c.served.Load()
	return st
}

// Log is every request the CDN answered so far, in order.
func (c *CDN) Log() []Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Request, len(c.log))
	for i, r := range c.log {
		out[i] = r.Request
		out[i].Sent = r.sent.Load()
	}
	return out
}

// Close ends every body, stalled ones included, and stops the CDN.
func (c *CDN) Close() {
	select {
	case <-c.done:
		return
	default:
	}
	close(c.done)
	c.srv.CloseClientConnections()
	c.srv.Close()
}

// errCut ends a dropped or stalled body.
var errCut = errors.New("cdn: body cut")

// serve answers one request: /s/<generation>/<id>.mp4.
func (c *CDN) serve(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if r.Method != http.MethodGet || len(parts) != 3 || parts[0] != "s" {
		http.NotFound(w, r)
		return
	}
	gen, err := strconv.Atoi(parts[1])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req := &request{Request: Request{At: time.Now(), From: rangeFrom(r.Header.Get("Range"))}}
	c.mu.Lock()
	c.stats.Requests++
	c.log = append(c.log, req)
	f := c.files[strings.TrimSuffix(parts[2], ".mp4")]
	switch {
	case f == nil:
		req.Status = http.StatusNotFound
	case c.unavailable > 0:
		c.unavailable--
		c.stats.Unavailable++
		req.Status = http.StatusServiceUnavailable
	case gen < c.gen:
		c.stats.Refused++
		req.Status = c.refuse
	}
	if req.Status != 0 {
		req.End = time.Now()
		c.mu.Unlock()
		w.WriteHeader(req.Status)
		return
	}
	b := &body{cdn: c, req: req, ctx: r.Context(), stallNew: time.Now().Before(c.stallUntil)}
	b.setRate(c.rate)
	c.bodies[b] = struct{}{}
	c.stats.Bodies++
	c.stats.PeakOpen = max(c.stats.PeakOpen, len(c.bodies))
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.bodies, b)
		req.End = time.Now()
		c.mu.Unlock()
	}()
	fi := f.layout.Files()[f.index]
	b.content = io.NewSectionReader(f.layout, fi.Offset, fi.Size)
	http.ServeContent(&statusRecorder{ResponseWriter: w, cdn: c, req: req}, r, f.id+".mp4", time.Time{}, b)
}

// rangeFrom is the first byte of a "bytes=N-" Range header; 0 without one.
func rangeFrom(h string) int64 {
	from, _, _ := strings.Cut(strings.TrimPrefix(h, "bytes="), "-")
	n, _ := strconv.ParseInt(from, 10, 64)
	return n
}

// statusRecorder notes in the log the status ServeContent answers with,
// as it answers.
type statusRecorder struct {
	http.ResponseWriter
	cdn  *CDN
	req  *request
	done bool
}

func (w *statusRecorder) record(code int) {
	if w.done {
		return
	}
	w.done = true
	w.cdn.mu.Lock()
	w.req.Status = code
	w.cdn.mu.Unlock()
}

func (w *statusRecorder) WriteHeader(code int) {
	w.record(code)
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	w.record(http.StatusOK)
	return w.ResponseWriter.Write(b)
}

// body is one response's content as ServeContent reads it: the file,
// paced, stalled or cut by the faults.
type body struct {
	cdn      *CDN
	req      *request
	ctx      context.Context // the request's: done when the client goes away
	content  *io.SectionReader
	stalled  atomic.Bool
	dropped  atomic.Bool
	stallNew bool // opened during StallFor: stalls after its first segment

	mu  sync.Mutex
	lim *rate.Limiter // nil: unlimited
}

func (b *body) setRate(bps int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bps <= 0 {
		b.lim = nil
		return
	}
	if b.lim == nil {
		b.lim = rate.NewLimiter(rate.Limit(bps), int(max(segment, bps/10)))
		return
	}
	b.lim.SetLimit(rate.Limit(bps))
	b.lim.SetBurst(int(max(segment, bps/10)))
}

func (b *body) limiter() *rate.Limiter {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lim
}

func (b *body) Seek(off int64, whence int) (int64, error) { return b.content.Seek(off, whence) }

// Read reads the next segment of the file, once the faults let it.
func (b *body) Read(p []byte) (int, error) {
	if b.stallNew && b.req.sent.Load() > 0 {
		b.cdn.mu.Lock()
		b.cdn.stall(b)
		b.cdn.mu.Unlock()
	}
	if b.stalled.Load() {
		select {
		case <-b.ctx.Done():
		case <-b.cdn.done:
		}
		return 0, errCut
	}
	if b.dropped.Load() {
		return 0, errCut
	}
	p = p[:min(len(p), segment)]
	if lim := b.limiter(); lim != nil {
		if err := lim.WaitN(b.ctx, len(p)); err != nil {
			return 0, err
		}
	}
	n, err := b.content.Read(p)
	b.cdn.served.Add(int64(n))
	b.req.sent.Add(int64(n))
	return n, err
}
