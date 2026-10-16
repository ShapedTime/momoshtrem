package stack

import (
	"context"
	"net/http"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RequestLabel is the pprof label the lab middleware runs each request
// under, valued with its Request.ID. Goroutines inherit their creator's
// labels, so the ones a request spawns (TorrentFile.readContext's reads,
// PriorityReader.detectFormat) carry it too, and a leaked goroutine names
// the request it came from.
const RequestLabel = "slreq"

// Request is one HTTP request the stack served, as the lab middleware saw
// it: outside everything production wraps the WebDAV handler in.
type Request struct {
	// ID is the stack's ID, a dot and the request's number from 1
	// ("s1.7"): unique in the process, so the checks of one stack ignore
	// another's goroutines.
	ID        string
	Method    string
	Path      string
	Range     string // the Range header
	UserAgent string
	Start     time.Time
	// FirstByte is the server-side TTFB: from Start to the first body byte
	// written; 0 until one is.
	FirstByte time.Duration
	// End is when the handler returned; zero while it runs.
	End    time.Time
	Status int   // the status written; 200 when the handler wrote none
	Bytes  int64 // body bytes written
	// Aborted is set when the client went away before the handler
	// returned: the request's context was cancelled or a write failed.
	Aborted bool
}

// Done reports whether the handler returned.
func (r Request) Done() bool { return !r.End.IsZero() }

// Duration is how long the handler ran: until End, or until now.
func (r Request) Duration() time.Duration {
	if r.Done() {
		return r.End.Sub(r.Start)
	}
	return time.Since(r.Start)
}

// RequestLog records every request the stack served, in arrival order.
// It's safe for concurrent use.
type RequestLog struct {
	prefix string // the stack's ID and a dot
	mu     sync.Mutex
	reqs   []*Request
}

// Owns reports whether the request ID is one of this log's.
func (l *RequestLog) Owns(id string) bool { return strings.HasPrefix(id, l.prefix) }

// Requests returns a copy of every request so far.
func (l *RequestLog) Requests() []Request {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Request, len(l.reqs))
	for i, r := range l.reqs {
		out[i] = *r
	}
	return out
}

// Active returns the requests whose handler hasn't returned.
func (l *RequestLog) Active() []Request {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Request
	for _, r := range l.reqs {
		if !r.Done() {
			out = append(out, *r)
		}
	}
	return out
}

// begin records a request arriving and returns its entry.
func (l *RequestLog) begin(r *http.Request) *Request {
	l.mu.Lock()
	defer l.mu.Unlock()
	req := &Request{
		ID:        l.prefix + strconv.Itoa(len(l.reqs)+1),
		Method:    r.Method,
		Path:      r.URL.Path,
		Range:     r.Header.Get("Range"),
		UserAgent: r.UserAgent(),
		Start:     time.Now(),
	}
	l.reqs = append(l.reqs, req)
	return req
}

// end records the handler returning.
func (l *RequestLog) end(req *Request, w *recorder, aborted bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	req.End = time.Now()
	req.Status = w.status
	if req.Status == 0 {
		req.Status = http.StatusOK
	}
	req.Bytes = w.bytes
	if !w.firstByte.IsZero() {
		req.FirstByte = w.firstByte.Sub(req.Start)
	}
	req.Aborted = aborted || w.writeErr != nil
}

// middleware runs each request under the RequestLabel pprof label and
// records it in log.
func middleware(log *RequestLog, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := log.begin(r)
		rec := &recorder{ResponseWriter: w}
		var aborted bool
		pprof.Do(r.Context(), pprof.Labels(RequestLabel, req.ID), func(ctx context.Context) {
			next.ServeHTTP(rec, r.WithContext(ctx))
			// The server cancels the context when the client closes the
			// connection, and only after the handler returns otherwise.
			aborted = ctx.Err() != nil
		})
		log.end(req, rec, aborted)
	})
}

// recorder notes what a handler writes. Only the handler's goroutine uses
// it, until the middleware reads it.
type recorder struct {
	http.ResponseWriter
	status    int
	bytes     int64
	firstByte time.Time
	writeErr  error
}

func (w *recorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *recorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	if n > 0 && w.firstByte.IsZero() {
		w.firstByte = time.Now()
	}
	w.bytes += int64(n)
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	return n, err
}

// Unwrap gives http.ResponseController the server's writer.
func (w *recorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// errorLog keeps what http.Server logs: handler panics it recovered, among
// others. It's safe for concurrent use.
type errorLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *errorLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func (l *errorLog) entries() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}
