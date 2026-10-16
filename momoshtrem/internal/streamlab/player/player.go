package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
)

// Options are what a Session needs besides its Config.
type Options struct {
	// Path is the file's WebDAV path and Size its length.
	Path string
	Size int64
	// Check checks bytes received at a file offset (checks.Integrity).
	Check func(off int64, p []byte)
	// OnError is called when a body breaks or a request fails, at once,
	// with the file offset it happened at and how long the body had been
	// silent (since its last byte, or since the request when none came):
	// the caller can see what the server and the torrent were doing then
	// (a known stall, say). Optional.
	OnError func(off int64, silence time.Duration, err error)
	// Retries is how many times a playback reconnects after a failed read
	// before it gives up, as a player retries a broken stream.
	Retries int
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// ReadError is a read that failed: the body broke before the file's end.
type ReadError struct {
	At     time.Time
	Offset int64
	Err    string
}

// Abort is a request the player dropped before its body ended: a seek
// away, a scrub, a reconnect or a stop. Range is the request's Range
// header, which names it in the server's request log.
type Abort struct {
	At    time.Time
	Range string
}

// Session is one viewer playing one file: a playback from where it
// starts, then one per seek. Each playback runs over one Range GET, or
// more when it reconnects. Every byte received is checked.
type Session struct {
	cfg  Config
	opts Options

	mu         sync.Mutex
	plays      []*Play
	readErrors []ReadError
	httpErrors []string
	aborts     []Abort
}

// NewSession plays the file in opts with the buffer model cfg.
func NewSession(cfg Config, opts Options) *Session {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Check == nil {
		opts.Check = func(int64, []byte) {}
	}
	return &Session{cfg: cfg.WithDefaults(), opts: opts}
}

// Config is the session's buffer model.
func (s *Session) Config() Config { return s.cfg }

// Path is the file's WebDAV path.
func (s *Session) Path() string { return s.opts.Path }

// Open starts a playback at from on conn: an open-ended Range GET. It
// returns once the answer's headers are in.
func (s *Session) Open(ctx context.Context, conn *client.Conn, from int64) (*Play, error) {
	return s.open(ctx, conn, from, time.Time{})
}

// Seek ends cur, dropping its request, and starts a playback at to on
// conn. Its seek latency runs from now to playable. cur may be nil.
func (s *Session) Seek(ctx context.Context, cur *Play, conn *client.Conn, to int64) (*Play, error) {
	at := s.opts.Now()
	if cur != nil {
		cur.Stop()
	}
	return s.open(ctx, conn, to, at)
}

func (s *Session) open(ctx context.Context, conn *client.Conn, from int64, seekAt time.Time) (*Play, error) {
	start := s.opts.Now()
	p := &Play{
		s:      s,
		ctx:    ctx,
		conn:   conn,
		model:  NewModel(s.cfg, from, start),
		pos:    from,
		buf:    make([]byte, s.cfg.Chunk),
		seekAt: seekAt,
	}
	s.mu.Lock()
	s.plays = append(s.plays, p)
	s.mu.Unlock()
	if err := p.request(); err != nil {
		p.end()
		return p, err
	}
	return p, nil
}

func (s *Session) httpError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.httpErrors = append(s.httpErrors, err.Error())
}

func (s *Session) aborted(at time.Time, from int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aborts = append(s.aborts, Abort{At: at, Range: fmt.Sprintf("bytes=%d-", from)})
}

// Play is one playback: a buffer model fed by Range GETs from one offset.
// Drive it with Watch, Scrub, Fill or UntilEOF, from one goroutine.
type Play struct {
	s     *Session
	ctx   context.Context
	conn  *client.Conn
	model *Model

	// mu guards stream and reqFrom: Abort may run on another goroutine.
	mu     sync.Mutex
	stream *client.Stream
	// reqFrom is the current request's first byte.
	reqFrom int64

	// pos is the file offset of the next byte to read; lastData is when
	// the current request was sent or its last byte arrived.
	pos      int64
	lastData time.Time
	buf      []byte
	seekAt   time.Time

	requests int
	retries  int
	err      error
	aborting atomic.Bool
	ended    bool
	stats    Stats
}

// request opens a GET from pos and checks its headers. A request that
// fails is reported like a broken body (OnError); a wrong answer is an
// HTTP error. Either ends the playback.
func (p *Play) request() error {
	p.aborting.Store(false)
	p.lastData = p.s.opts.Now()
	st, err := p.conn.Play(p.ctx, p.s.opts.Path, p.pos)
	p.requests++
	if err != nil {
		p.err = fmt.Errorf("GET %s from %d: %w", p.s.opts.Path, p.pos, err)
		if p.ctx.Err() == nil && p.s.opts.OnError != nil {
			p.s.opts.OnError(p.pos, p.s.opts.Now().Sub(p.lastData), p.err)
		}
		return p.err
	}
	if err := checks.RangeResponse(st.Response, p.pos, p.s.opts.Size-1, p.s.opts.Size); err != nil {
		st.Abort()
		p.s.httpError(err)
		p.err = err
		return err
	}
	p.setStream(st, p.pos)
	return nil
}

func (p *Play) setStream(st *client.Stream, from int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stream, p.reqFrom = st, from
}

func (p *Play) current() *client.Stream {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stream
}

// Model is the playback's buffer model.
func (p *Play) Model() *Model { return p.model }

// Pos is the file offset of the next byte to read.
func (p *Play) Pos() int64 { return p.pos }

// Playhead is the file offset of the next byte to play.
func (p *Play) Playhead() int64 { return p.model.From() + p.model.Consumed() }

// Err is the read error the playback couldn't recover from; nil if none.
func (p *Play) Err() error { return p.err }

// Watch plays d of media: until the playhead has advanced d at the
// bitrate, the file ends, the stream breaks for good, or ctx is done. A
// paused playback resumes first.
func (p *Play) Watch(d time.Duration) error {
	p.Resume()
	target := p.model.Consumed() + int64(d.Seconds()*float64(p.s.cfg.Bitrate))
	return p.drive(func() bool { return p.model.Consumed() >= target })
}

// UntilEOF plays to the end of the file.
func (p *Play) UntilEOF() error {
	p.Resume()
	return p.drive(func() bool { return false })
}

// UntilPlayable reads until the playback can start.
func (p *Play) UntilPlayable() error {
	p.Resume()
	return p.drive(func() bool { return !p.model.PlayableAt().IsZero() })
}

// Fill reads until n bytes of this playback have arrived (or the file
// ends), the playhead running.
func (p *Play) Fill(n int64) error {
	p.Resume()
	return p.drive(func() bool { return p.model.Received() >= n })
}

// Scrub reads for d of wall time, the playhead running, then drops the
// request even mid-read, as a player scrubbing past a position does. The
// playback ends.
func (p *Play) Scrub(d time.Duration) error {
	p.Resume()
	deadline := p.s.opts.Now().Add(d)
	t := time.AfterFunc(d, p.Abort)
	defer t.Stop()
	err := p.drive(func() bool { return !p.s.opts.Now().Before(deadline) })
	p.end()
	return err
}

// Pause stops the playhead and the reading, as a viewer pausing does:
// the request stays open, unread.
func (p *Play) Pause() {
	if !p.ended {
		p.model.Pause(p.s.opts.Now())
	}
}

// Resume restarts the playhead after Pause.
func (p *Play) Resume() {
	if !p.ended {
		p.model.Resume(p.s.opts.Now())
	}
}

// Abort drops the current request, as a player seeking away or losing
// its connection does: the playback reads no more (Reconnect continues
// it, Stop ends it). Safe to call from another goroutine while the
// playback reads, and more than once.
func (p *Play) Abort() {
	p.mu.Lock()
	st, from := p.stream, p.reqFrom
	p.mu.Unlock()
	if st != nil && p.aborting.CompareAndSwap(false, true) {
		p.s.aborted(p.s.opts.Now(), from)
		st.Abort()
	}
}

// Stop ends the playback, dropping its request if its body hadn't ended:
// to the server, that's an abort.
func (p *Play) Stop() Stats { return p.end() }

// Reconnect drops the current request and continues the playback with a
// new one from where the received bytes end, the buffer kept, as a player
// whose connection broke does.
func (p *Play) Reconnect() error {
	if p.ended {
		return errors.New("player: reconnect of an ended playback")
	}
	p.Abort()
	p.setStream(nil, p.pos)
	return p.request()
}

// end ends the playback once, dropping a request still open; the stats
// stay.
func (p *Play) end() Stats {
	if !p.ended {
		p.ended = true
		p.Abort()
		p.stats = p.model.End(p.s.opts.Now())
	}
	return p.stats
}

// drive reads and plays until done, the playback ends or fails, or the
// context is done.
func (p *Play) drive(done func() bool) error {
	if p.ended {
		return p.err
	}
	cfg := p.s.cfg
	for {
		if p.err != nil {
			return p.err
		}
		now := p.s.opts.Now()
		p.model.Advance(now)
		if done() || p.model.State() == Ended {
			return nil
		}
		if err := p.ctx.Err(); err != nil {
			return err
		}
		if p.aborting.Load() {
			return nil
		}
		state := p.model.State()
		full := p.model.Buffer() >= cfg.MaxBuffer && (state == Playing || state == Paused)
		st := p.current()
		if st == nil || p.model.EOF() || full {
			// Nothing to read now: let the playhead drain a chunk.
			wait := time.Duration(float64(cfg.Chunk) / float64(cfg.Bitrate) * float64(time.Second))
			if !p.sleep(min(max(wait, 2*time.Millisecond), 100*time.Millisecond)) {
				return p.ctx.Err()
			}
			continue
		}
		n, err := st.Read(p.buf)
		now = p.s.opts.Now()
		if n > 0 {
			p.s.opts.Check(p.pos, p.buf[:n])
			p.pos += int64(n)
			p.lastData = now
			p.model.Receive(now, n)
		}
		switch {
		case err == nil:
		case p.aborting.Load():
			return nil // dropped on purpose
		case errors.Is(err, io.EOF) && p.pos >= p.s.opts.Size:
			p.model.SetEOF(now)
			st.Close()
			p.setStream(nil, p.pos)
		default:
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			if ctxErr := p.ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			st.Close()
			p.setStream(nil, p.pos)
			p.readFailed(now, err)
		}
	}
}

// readFailed records a broken body and reconnects while retries last.
func (p *Play) readFailed(now time.Time, err error) {
	p.s.mu.Lock()
	p.s.readErrors = append(p.s.readErrors, ReadError{At: now, Offset: p.pos, Err: err.Error()})
	p.s.mu.Unlock()
	if p.s.opts.OnError != nil {
		p.s.opts.OnError(p.pos, now.Sub(p.lastData), err)
	}
	if p.retries >= p.s.opts.Retries {
		p.err = fmt.Errorf("reading %s at %d: %w", p.s.opts.Path, p.pos, err)
		return
	}
	p.retries++
	p.request() // an error ends the playback (p.err)
}

func (p *Play) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// PlayRecord is one playback of a session.
type PlayRecord struct {
	Stats
	// Seek is set for a playback a seek started; SeekLatency runs from
	// the seek to playable (0 when it never was).
	Seek        bool
	SeekLatency time.Duration
	// Requests is how many GETs it took: 1, plus one per reconnect.
	Requests int
	Err      string
}

// SessionStats is what a viewer felt over a session.
type SessionStats struct {
	Path       string
	Plays      []PlayRecord
	ReadErrors []ReadError
	HTTPErrors []string
	Aborts     []Abort
}

// Stats ends every playback and returns the session's numbers.
func (s *Session) Stats() SessionStats {
	s.mu.Lock()
	plays := append([]*Play(nil), s.plays...)
	s.mu.Unlock()
	out := SessionStats{Path: s.opts.Path}
	for _, p := range plays {
		st := p.end()
		r := PlayRecord{Stats: st, Seek: !p.seekAt.IsZero(), Requests: p.requests}
		if r.Seek && st.Playable {
			r.SeekLatency = p.model.PlayableAt().Sub(p.seekAt)
		}
		if p.err != nil {
			r.Err = p.err.Error()
		}
		out.Plays = append(out.Plays, r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out.ReadErrors = append([]ReadError(nil), s.readErrors...)
	out.HTTPErrors = append([]string(nil), s.httpErrors...)
	out.Aborts = append([]Abort(nil), s.aborts...)
	return out
}

// Summary is a session's numbers in one line each.
type Summary struct {
	// TTFB and TimeToPlayable are the first playback's.
	TTFB, TimeToPlayable time.Duration
	// SeekLatencies are each seek's, in order; seeks that never became
	// playable are left out.
	SeekLatencies []time.Duration
	Stalls        int
	StallTime     time.Duration
	PlayTime      time.Duration
	// RebufferRatio is StallTime over StallTime plus PlayTime.
	RebufferRatio float64
	OverRead      int64
	Received      int64
}

// Summary sums the stats up.
func (st SessionStats) Summary() Summary {
	var sum Summary
	for i, p := range st.Plays {
		if i == 0 {
			sum.TTFB, sum.TimeToPlayable = p.TTFB, p.TimeToPlayable
		}
		if p.Seek && p.Playable {
			sum.SeekLatencies = append(sum.SeekLatencies, p.SeekLatency)
		}
		sum.Stalls += len(p.Stalls)
		sum.StallTime += p.StallTime
		sum.PlayTime += p.PlayTime
		sum.OverRead += p.OverRead
		sum.Received += p.Received
	}
	if total := sum.StallTime + sum.PlayTime; total > 0 {
		sum.RebufferRatio = float64(sum.StallTime) / float64(total)
	}
	return sum
}
