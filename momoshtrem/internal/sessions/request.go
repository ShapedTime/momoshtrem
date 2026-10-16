// Package sessions knows who is streaming what over WebDAV (PLAN §6
// "Torrents and activity (B6)"): Infuse on the Apple TV, which episode, how
// far in, how fast and, for a torrent, how much is buffered ahead.
//
// The WebDAV server puts each request's method, User-Agent and remote
// address in its context (WithRequest). The VFS opens a Handle for every
// video a GET opens and reports its reads to it. Reads with the same
// remote address, User-Agent and WebDAV path form one session: Infuse opens
// several Range GETs per playback, and they're one session. A session ends
// 30 s after its last read. Sniffs (an open that read under 64 KB and
// closed within 2 s) never show, and neither do PROPFIND and HEAD opens,
// which read nothing.
package sessions

import (
	"context"
	"net"
	"net/http"
)

// Request is what a session knows of the WebDAV request that opened a
// file.
type Request struct {
	Method    string
	UserAgent string
	// RemoteAddr is the client's address without the port: Infuse reads
	// one file over several connections.
	RemoteAddr string
}

type requestKey struct{}

// WithRequest returns ctx carrying r.
func WithRequest(ctx context.Context, r Request) context.Context {
	return context.WithValue(ctx, requestKey{}, r)
}

// RequestFrom returns the request WithRequest put in ctx.
func RequestFrom(ctx context.Context) (Request, bool) {
	r, ok := ctx.Value(requestKey{}).(Request)
	return r, ok
}

// RequestOf reads an HTTP request's method, User-Agent and remote host.
// The WebDAV server sits behind no proxy it trusts, so the address is the
// socket's.
func RequestOf(r *http.Request) Request {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return Request{Method: r.Method, UserAgent: r.UserAgent(), RemoteAddr: host}
}

// Middleware puts each request's Request in its context, for the sessions
// of the files it opens.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithRequest(r.Context(), RequestOf(r))))
	})
}
