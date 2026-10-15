package hdrezka

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// StreamKey names one stream: a movie, or one episode of a series, in one
// translation. library.StreamRef stores every field, so a stream resolves
// again without fetching the title page.
type StreamKey struct {
	PostID int
	// Path is the title page, sent as the Referer. It isn't part of the
	// stream's identity.
	Path         string
	TranslatorID int
	// The translation's flags: one translator id can come in several cuts.
	// The site only takes them for movies.
	Camrip   bool
	Ads      bool
	Director bool
	// Season and Episode are HDRezka's numbers; 0 for a movie.
	Season  int
	Episode int
}

// IsEpisode reports whether the key names an episode rather than a movie.
func (k StreamKey) IsEpisode() bool { return k.Episode > 0 }

// streamID is a key's identity: everything but the Referer.
type streamID struct {
	post, translator      int
	camrip, ads, director bool
	season, episode       int
}

func (k StreamKey) id() streamID {
	return streamID{k.PostID, k.TranslatorID, k.Camrip, k.Ads, k.Director, k.Season, k.Episode}
}

func (id streamID) String() string {
	return fmt.Sprintf("%d/%d/%t/%t/%t/%d/%d", id.post, id.translator, id.camrip, id.ads, id.director, id.season, id.episode)
}

// StreamFetcher fetches a stream's links from the site. *Client implements
// it.
type StreamFetcher interface {
	Movie(ctx context.Context, ref Ref, t Translator) (*Stream, error)
	Episode(ctx context.Context, ref Ref, translatorID, season, episode int) (*Stream, error)
}

var _ StreamFetcher = (*Client)(nil)

const (
	// maxLinkTTL caps how long resolved links are reused. They're signed
	// for about 20 hours, and may be bound to the IP that resolved them.
	maxLinkTTL = 4 * time.Hour
	// linkExpiryMargin is how long before a link's own expiry it's resolved
	// again, so a long stream never reaches it.
	linkExpiryMargin = time.Hour
	// minLinkTTL keeps a link that's about to expire (or an expiry misread)
	// from being resolved again on every open. The streamer resolves again
	// when the CDN refuses it anyway.
	minLinkTTL = time.Minute
	// resolveTimeout bounds one resolve. It outlives the caller that started
	// it, since others may be waiting for the same stream.
	resolveTimeout = 30 * time.Second
)

// linkExpiryRe finds the expiry in a signed link's path:
// "…/0be3bfb771ec26f0:2026101618:dlFOU2k3VzR6/…" expires at 2026-10-16 18h.
var linkExpiryRe = regexp.MustCompile(`/[0-9a-fA-F]{8,}:(\d{10}):[^/]+/`)

// expiryZone reads the expiry hour as Moscow time. The site doesn't say:
// MSK is the easternmost likely reading, so a wrong guess expires the cache
// early rather than late.
var expiryZone = time.FixedZone("MSK", 3*60*60)

// Resolver resolves streams to their signed links and caches them per
// stream (post, translator and its flags, season and episode) for at most
// 4 hours, and never past an hour before the links say they expire.
// Concurrent resolves of one stream share a single request. Safe for
// concurrent use.
type Resolver struct {
	fetch StreamFetcher
	now   func() time.Time
	group singleflight.Group

	mu    sync.Mutex
	cache map[streamID]resolvedStream
}

// resolvedStream is a cached resolve.
type resolvedStream struct {
	stream  *Stream
	expires time.Time
}

// NewResolver creates a Resolver that fetches links with fetch.
func NewResolver(fetch StreamFetcher) *Resolver {
	return &Resolver{fetch: fetch, now: time.Now, cache: map[streamID]resolvedStream{}}
}

// Resolve returns the stream's qualities and subtitles with their links,
// from the cache while they're fresh. The Stream is shared with other
// callers: don't modify it. Errors aren't cached.
func (r *Resolver) Resolve(ctx context.Context, key StreamKey) (*Stream, error) {
	id := key.id()
	if s, ok := r.cached(id); ok {
		return s, nil
	}
	ch := r.group.DoChan(id.String(), func() (any, error) {
		// A resolve that finished between the check above and here.
		if s, ok := r.cached(id); ok {
			return s, nil
		}
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resolveTimeout)
		defer cancel()
		s, err := r.fetchStream(fctx, key)
		if err != nil {
			return nil, err
		}
		r.store(id, s)
		return s, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*Stream), nil
	}
}

// Link returns the MP4 link of one quality of the stream (its label, e.g.
// "1080p"). It's ErrNoQuality when the stream no longer offers it,
// ErrPremium when it needs a Premium account and ErrNoMP4 when it's HLS
// only.
func (r *Resolver) Link(ctx context.Context, key StreamKey, quality string) (string, error) {
	s, err := r.Resolve(ctx, key)
	if err != nil {
		return "", err
	}
	q, ok := s.Find(quality)
	switch {
	case !ok:
		return "", ErrNoQuality
	case q.Premium:
		return "", ErrPremium
	case q.MP4 == "":
		return "", ErrNoMP4
	}
	return q.MP4, nil
}

// Invalidate forgets the stream's links: the CDN refused one, so the next
// Resolve fetches them again.
func (r *Resolver) Invalidate(key StreamKey) {
	r.mu.Lock()
	delete(r.cache, key.id())
	r.mu.Unlock()
}

func (r *Resolver) fetchStream(ctx context.Context, key StreamKey) (*Stream, error) {
	ref := Ref{PostID: key.PostID, Path: key.Path}
	if key.IsEpisode() {
		return r.fetch.Episode(ctx, ref, key.TranslatorID, key.Season, key.Episode)
	}
	return r.fetch.Movie(ctx, ref, Translator{
		ID: key.TranslatorID, Camrip: key.Camrip, Ads: key.Ads, Director: key.Director,
	})
}

func (r *Resolver) cached(id streamID) (*Stream, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cache[id]
	if !ok || !r.now().Before(c.expires) {
		return nil, false
	}
	return c.stream, true
}

// store caches s and drops the entries that have expired.
func (r *Resolver) store(id streamID, s *Stream) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, c := range r.cache {
		if !now.Before(c.expires) {
			delete(r.cache, k)
		}
	}
	r.cache[id] = resolvedStream{stream: s, expires: now.Add(linkTTL(s, now))}
}

// linkTTL is how long s's links are reused: min(4 h, the earliest expiry
// its MP4 links carry − 1 h), at least a minute.
func linkTTL(s *Stream, now time.Time) time.Duration {
	ttl := maxLinkTTL
	for _, q := range s.Qualities {
		if exp, ok := linkExpiry(q.MP4); ok {
			ttl = min(ttl, exp.Sub(now)-linkExpiryMargin)
		}
	}
	return max(ttl, minLinkTTL)
}

// linkExpiry reads the YYYYMMDDHH expiry from a signed link's path.
func linkExpiry(link string) (time.Time, bool) {
	m := linkExpiryRe.FindStringSubmatch(link)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006010215", m[1], expiryZone)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
