package sessions

import (
	"cmp"
	"context"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// The session rules.
const (
	// Timeout ends a session this long after its last read. A read still
	// in flight (a torrent waiting for pieces) keeps it going.
	Timeout = 30 * time.Second
	// SniffBytes and SniffDuration tell a sniff: an open that read under
	// SniffBytes in all and closed within SniffDuration never makes a
	// session.
	SniffBytes    = 64 << 10
	SniffDuration = 2 * time.Second
	// RateTimeConstant smooths a session's rate (an EWMA), like the
	// torrents' speeds.
	RateTimeConstant = 5 * time.Second
	// itemTTL is how long a resolved item is kept: titles and runtimes
	// rarely change.
	itemTTL = 5 * time.Minute
)

// Source is where a streamed file's bytes come from.
type Source string

const (
	SourceTorrent Source = "torrent"
	SourceHDRezka Source = "hdrezka"
)

// ItemRef names the library item a file plays as.
type ItemRef struct {
	Type library.ItemType // movie or episode
	ID   int64
}

// File is a video the VFS opened: what its sessions report.
type File struct {
	// Path is the WebDAV path: one session per client and path.
	Path string
	Name string // the file name WebDAV shows
	Size int64
	// Source is torrent or hdrezka; InfoHash and TorrentPath are a torrent
	// file's hash and its path inside the torrent.
	Source      Source
	InfoHash    string
	TorrentPath string
	// Item is the library item; nil when the VFS entry doesn't know it.
	Item *ItemRef
	// BufferedAhead returns how many bytes from offset on are complete,
	// contiguously; nil for HDRezka.
	BufferedAhead func(offset int64) int64
}

// ItemResolver loads the items sessions play as.
// *library.CatalogRepository implements it.
type ItemResolver interface {
	StreamItems(ctx context.Context, movieIDs, episodeIDs []int64) ([]library.StreamItem, error)
}

var _ ItemResolver = (*library.CatalogRepository)(nil)

// Registry tracks the streaming sessions.
type Registry struct {
	mu       sync.Mutex
	now      func() time.Time
	items    ItemResolver
	sessions map[key]*session
	nextID   uint64
	resolved map[ItemRef]resolvedItem
	log      *slog.Logger
}

// Option configures a Registry.
type Option func(*Registry)

// WithClock swaps the clock, for tests.
func WithClock(now func() time.Time) Option {
	return func(r *Registry) { r.now = now }
}

// WithItems resolves each session's item (title, episode, runtime); without
// it items and positions are null.
func WithItems(items ItemResolver) Option {
	return func(r *Registry) { r.items = items }
}

// NewRegistry creates an empty Registry. Expiry is lazy: every read,
// listing and count drops the sessions that ended, so it runs no goroutine.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{
		now:      time.Now,
		sessions: map[key]*session{},
		resolved: map[ItemRef]resolvedItem{},
		log:      slog.With("component", "sessions"),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type key struct{ addr, ua, path string }

type resolvedItem struct {
	item *library.StreamItem // nil: gone from the library
	at   time.Time
}

// session is one client playing one file.
type session struct {
	id         string
	file       File
	req        Request
	startedAt  time.Time
	lastReadAt time.Time
	offset     int64
	rate       float64 // bytes per second at rateAt
	rateAt     time.Time
	handles    map[*Handle]struct{} // open handles
	inflight   int                  // reads in flight
	qualified  bool                 // one of its opens wasn't a sniff
	ended      bool
}

// Handle is one open of a file, by one request. Its methods do nothing on
// a nil Handle.
type Handle struct {
	r        *Registry
	key      key
	file     File
	req      Request
	openedAt time.Time
	reads    int
	bytes    int64
	s        *session // nil until the first read
}

// Open starts tracking a file a GET opened; nil (nothing tracked) for any
// other method, or a context without a Request. A Handle registers nothing
// until its first read, so the opens of a listing cost nothing.
func (r *Registry) Open(ctx context.Context, f File) *Handle {
	if r == nil {
		return nil
	}
	req, ok := RequestFrom(ctx)
	if !ok || req.Method != http.MethodGet {
		return nil
	}
	return &Handle{
		r:        r,
		key:      key{addr: req.RemoteAddr, ua: req.UserAgent, path: f.Path},
		file:     f,
		req:      req,
		openedAt: r.now(),
	}
}

// Begin marks a read starting.
func (h *Handle) Begin() {
	if h == nil {
		return
	}
	r := h.r
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if h.s == nil || h.s.ended {
		h.attach(now)
	}
	h.reads++
	h.s.inflight++
	h.s.lastReadAt = now
}

// End marks a read that started with Begin done: n bytes at off.
func (h *Handle) End(off int64, n int) {
	if h == nil {
		return
	}
	r := h.r
	r.mu.Lock()
	defer r.mu.Unlock()
	s := h.s
	if s == nil {
		return // End without Begin
	}
	now := r.now()
	s.inflight = max(0, s.inflight-1)
	s.lastReadAt = now
	if n > 0 {
		h.bytes += int64(n)
		s.offset = off + int64(n)
		s.rate = s.rateNow(now) + float64(n)/RateTimeConstant.Seconds()
		s.rateAt = now
	}
	if h.counts(now) {
		s.qualified = true
	}
}

// Close marks the open closed. A session of nothing but sniffs is dropped
// with its last open.
func (h *Handle) Close() {
	if h == nil {
		return
	}
	r := h.r
	r.mu.Lock()
	defer r.mu.Unlock()
	s := h.s
	if s == nil {
		return
	}
	if h.counts(r.now()) {
		s.qualified = true
	}
	delete(s.handles, h)
	if !s.qualified && len(s.handles) == 0 && r.sessions[h.key] == s {
		delete(r.sessions, h.key)
	}
}

// counts reports whether the open isn't a sniff: it read SniffBytes, or
// has been open (with a read) for SniffDuration. The caller holds r.mu.
func (h *Handle) counts(now time.Time) bool {
	return h.reads > 0 && (h.bytes >= SniffBytes || now.Sub(h.openedAt) >= SniffDuration)
}

// attach joins the client's session for the file, or starts one. The
// caller holds r.mu.
func (h *Handle) attach(now time.Time) {
	r := h.r
	r.sweep(now)
	s := r.sessions[h.key]
	if s == nil {
		r.nextID++
		s = &session{
			id:        strconv.FormatUint(r.nextID, 10),
			file:      h.file,
			req:       h.req,
			startedAt: now,
			rateAt:    now,
			handles:   map[*Handle]struct{}{},
		}
		r.sessions[h.key] = s
	}
	s.handles[h] = struct{}{}
	h.s = s
}

// rateNow is the rate decayed to now.
func (s *session) rateNow(now time.Time) float64 {
	dt := now.Sub(s.rateAt).Seconds()
	if dt <= 0 {
		return s.rate
	}
	return s.rate * math.Exp(-dt/RateTimeConstant.Seconds())
}

// visible reports whether the session shows: one of its opens wasn't a
// sniff, and it read within Timeout (or is reading). The caller holds r.mu.
func (s *session) visible(now time.Time) bool {
	if !s.qualified {
		for h := range s.handles {
			if h.counts(now) {
				s.qualified = true
				break
			}
		}
	}
	return s.qualified && (s.inflight > 0 || now.Sub(s.lastReadAt) < Timeout)
}

// sweep drops the sessions that ended, and those of sniffs whose opens all
// closed. Open handles of an ended session start a new one on their next
// read. The caller holds r.mu.
func (r *Registry) sweep(now time.Time) {
	for k, s := range r.sessions {
		if s.visible(now) {
			continue
		}
		if s.qualified || len(s.handles) == 0 {
			s.ended = true
			delete(r.sessions, k)
		}
	}
}

// Count returns how many sessions are streaming now.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.sweep(now)
	n := 0
	for _, s := range r.sessions {
		if s.visible(now) {
			n++
		}
	}
	return n
}

// snapshot is a session's state, copied under the lock.
type snapshot struct {
	id                    string
	file                  File
	req                   Request
	startedAt, lastReadAt time.Time
	offset                int64
	rate                  float64
}

// Sessions returns the sessions streaming now, the most recently read
// first, with their items resolved. Never nil.
func (r *Registry) Sessions(ctx context.Context) []Session {
	r.mu.Lock()
	now := r.now()
	r.sweep(now)
	snaps := make([]snapshot, 0, len(r.sessions))
	for _, s := range r.sessions {
		if s.visible(now) {
			snaps = append(snaps, snapshot{id: s.id, file: s.file, req: s.req, startedAt: s.startedAt,
				lastReadAt: s.lastReadAt, offset: s.offset, rate: s.rateNow(now)})
		}
	}
	r.mu.Unlock()

	items := r.resolve(ctx, now, snaps)
	out := make([]Session, 0, len(snaps))
	for _, sn := range snaps {
		var it *library.StreamItem
		if ref := sn.file.Item; ref != nil {
			it = items[*ref]
		}
		out = append(out, build(sn, it))
	}
	slices.SortFunc(out, func(a, b Session) int {
		return cmp.Or(b.LastReadAt.Compare(a.LastReadAt), b.StartedAt.Compare(a.StartedAt), cmp.Compare(b.ID, a.ID))
	})
	return out
}

// resolve returns the items of snaps, loading the ones not resolved within
// itemTTL in one query. A failed query leaves them null.
func (r *Registry) resolve(ctx context.Context, now time.Time, snaps []snapshot) map[ItemRef]*library.StreamItem {
	out := map[ItemRef]*library.StreamItem{}
	if r.items == nil {
		return out
	}
	var movies, episodes []int64
	r.mu.Lock()
	for ref, ri := range r.resolved {
		if now.Sub(ri.at) >= itemTTL {
			delete(r.resolved, ref)
		}
	}
	for _, sn := range snaps {
		ref := sn.file.Item
		if ref == nil {
			continue
		}
		if ri, ok := r.resolved[*ref]; ok {
			out[*ref] = ri.item
			continue
		}
		if _, queued := out[*ref]; queued {
			continue
		}
		out[*ref] = nil
		if ref.Type == library.ItemTypeMovie {
			movies = append(movies, ref.ID)
		} else {
			episodes = append(episodes, ref.ID)
		}
	}
	r.mu.Unlock()
	if len(movies) == 0 && len(episodes) == 0 {
		return out
	}

	found, err := r.items.StreamItems(ctx, movies, episodes)
	if err != nil {
		r.log.Warn("Can't load what the sessions play", "error", err)
		return out
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range found {
		it := found[i]
		out[ItemRef{Type: it.ItemType, ID: it.ItemID}] = &it
	}
	for _, id := range movies {
		ref := ItemRef{Type: library.ItemTypeMovie, ID: id}
		r.resolved[ref] = resolvedItem{item: out[ref], at: now}
	}
	for _, id := range episodes {
		ref := ItemRef{Type: library.ItemTypeEpisode, ID: id}
		r.resolved[ref] = resolvedItem{item: out[ref], at: now}
	}
	return out
}

// build is a session's JSON: the position and buffer from the item's
// runtime, the buffer from the torrent's pieces.
func build(sn snapshot, it *library.StreamItem) Session {
	client, device := ParseUserAgent(sn.req.UserAgent)
	s := Session{
		ID:          sn.id,
		Source:      sn.file.Source,
		FileName:    sn.file.Name,
		Client:      client,
		Device:      device,
		OffsetBytes: sn.offset,
		SizeBytes:   sn.file.Size,
		RateBps:     int64(math.Round(sn.rate)),
		StartedAt:   sn.startedAt,
		LastReadAt:  sn.lastReadAt,
	}
	if sn.file.Source == SourceTorrent {
		hash, path := sn.file.InfoHash, sn.file.TorrentPath
		s.InfoHash, s.FilePath = &hash, &path
	}
	var duration int64
	if it != nil {
		s.Item = itemOf(it)
		duration = int64(it.RuntimeMinutes) * 60
	}
	if duration > 0 && sn.file.Size > 0 {
		s.DurationSeconds = &duration
		pos := secondsOf(sn.offset, sn.file.Size, duration)
		s.PositionSeconds = &pos
	}
	if sn.file.BufferedAhead != nil {
		b := max(0, sn.file.BufferedAhead(sn.offset))
		s.BufferedAheadBytes = &b
		if s.DurationSeconds != nil {
			secs := secondsOf(b, sn.file.Size, duration)
			s.BufferedAheadSeconds = &secs
		}
	}
	return s
}

// secondsOf is bytes of a size-byte file that plays for duration seconds,
// in seconds: the file is taken to play at a constant bitrate.
func secondsOf(bytes, size, duration int64) int64 {
	return int64(math.Round(float64(bytes) / float64(size) * float64(duration)))
}

func itemOf(it *library.StreamItem) *Item {
	out := &Item{
		MediaType: string(it.MediaType),
		TMDBID:    it.TMDBID,
		LibraryID: it.LibraryID,
		Title:     it.Title,
	}
	if it.ItemType == library.ItemTypeEpisode {
		out.SeasonNumber, out.EpisodeNumber = it.SeasonNumber, it.EpisodeNumber
		name := it.EpisodeName
		if name != "" {
			out.EpisodeName = &name
		}
	}
	return out
}
