package scenario

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"

	"github.com/shapedtime/momoshtrem/internal/streamlab/cdn"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/player"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Prometheus names the env reads.
const (
	metricReadTimeouts    = "momoshtrem_streaming_read_timeouts_total"
	metricPriorityUpdates = "momoshtrem_streaming_priority_updates_total"
)

// Title is a library entry a scenario plays: a file of a lab torrent.
type Title struct {
	// Name is "Film (2020)" or "Show S01E02".
	Name string
	// Path is the file's WebDAV path, Dir its folder's.
	Path, Dir string
	Hash      string
	Layout    *media.Layout
	File      int
	// Offset is the file's torrent offset, Size its length.
	Offset, Size int64
	Integrity    *checks.Integrity
	// Seeder is the torrent's first seeder, Peers all of them; nil for an
	// HDRezka movie.
	Seeder *swarm.Seeder
	Peers  []*Peer
	// Remote is an HDRezka movie's file on the fake CDN; nil for a
	// torrent's.
	Remote *cdn.File
}

// Piece is the torrent piece holding the file offset off.
func (t *Title) Piece(off int64) int {
	off = min(max(off, 0), t.Size-1)
	return int((t.Offset + off) / t.Layout.PieceLength())
}

// Pieces is the file's piece range, [begin, end).
func (t *Title) Pieces() (begin, end int) {
	return t.Piece(0), t.Piece(t.Size-1) + 1
}

// Env is what a script drives: the stack, the titles, devices and
// players. Every connection, player and probe it hands out is recorded
// for the checks and the report. Its methods are safe for concurrent use.
type Env struct {
	Ctx context.Context
	P   Params
	// RNG is seeded with the seed param and the scenario's name.
	RNG    *rand.Rand
	Stack  *stack.Stack
	Titles []*Title

	rngMu       sync.Mutex
	rec         recorder
	completions *completions
	// dir is the repetition's folder for artifacts; "" keeps none.
	dir string
	// swarm is the seeders, proxies and fake CDN (Remote).
	swarm *swarmSet
}

// recorder keeps what a repetition did.
type recorder struct {
	mu         sync.Mutex
	devices    map[string]string // User-Agent -> device name
	conns      []*client.Conn
	connDevice map[*client.Conn]client.Device
	players    []playerRec
	failures   []failure
	httpErrors []string
	aborts     []clientAbort
	propfinds  []time.Duration
	probes     []time.Duration
	metrics    map[string]float64
	checks     map[string]report.RepCheck
	notes      []string
	probesDone int
	artifacts  map[string]string
	expects    []*Expectation
}

type playerRec struct {
	device string
	title  *Title
	s      *player.Session
}

// quickBreak is the most silence before a break that F11's signature
// allows: a read of an evicted piece fails at once, after anacrolix's
// three tries (measured 0.9-1.5 s on a loaded machine), far from the
// read timeout #195's signature needs half of.
const quickBreak = 3 * time.Second

// failure is a body that broke, or a request that failed: a playback's or
// a probe's, with what the lab saw when it did.
type failure struct {
	device string
	title  *Title
	at     time.Time
	offset int64
	piece  int
	err    string
	// silence is how long the body had been silent before it broke: since
	// its last byte, or since the request when none came.
	silence time.Duration
	// timedOut: silent for half the read timeout or more, as a server read
	// that timed out leaves a body (the server then ends it early).
	timedOut bool
	// stalled: the piece incomplete, a seeder connected, downloads on
	// (the torrent neither idle nor paused).
	stalled bool
	// evicted: the piece cache doesn't hold the piece complete, and
	// wasComplete: anacrolix had reported it complete before (F11).
	evicted     bool
	wasComplete bool
	broke       bool // a body that ended early, not a request that failed
	// expected is why the script expected it (Env.Expect): a fault
	// meant to break it. play-ok leaves it out.
	expected string
}

// known is the known issue whose signature the failure carries: #195's
// stall (a body silent through a read timeout, at an incomplete piece
// with the seeder connected and downloads on), or F11's quick break at a
// piece anacrolix had complete and the cache no longer holds.
func (f failure) known() string {
	switch {
	case f.timedOut && f.stalled:
		return "#195"
	case f.silence < quickBreak && f.broke && f.evicted && f.wasComplete:
		return "F11"
	}
	return ""
}

func (f failure) diagnosis() string {
	silent := fmt.Sprintf("silent %v", f.silence.Round(time.Millisecond))
	if f.expected != "" {
		return fmt.Sprintf("expected (%s): %s", f.expected, silent)
	}
	switch f.known() {
	case "#195":
		return fmt.Sprintf("known stall (#195): %s, piece %d incomplete, seeder connected, downloads on", silent, f.piece)
	case "F11":
		return fmt.Sprintf("F11: broke at once (%s) at piece %d, complete before, which the piece cache no longer holds", silent, f.piece)
	}
	var why []string
	if f.timedOut {
		why = append(why, "timed out")
	}
	if f.stalled {
		why = append(why, "piece incomplete, seeder connected, downloads on")
	}
	if f.evicted {
		why = append(why, "piece not in the cache")
	}
	if f.wasComplete {
		why = append(why, "piece was complete")
	}
	if f.title != nil && f.title.Remote != nil {
		return fmt.Sprintf("no known signature: %s, an HDRezka file", silent)
	}
	return fmt.Sprintf("no known signature: %s, piece %d (%s)", silent, f.piece, strings.Join(why, ", "))
}

// clientAbort is a request a script dropped outside a player.
type clientAbort struct {
	ua, path, rng string
	at            time.Time
}

// Rand is a random int in [0, n) from the seeded RNG.
func (e *Env) Rand(n int64) int64 {
	e.rngMu.Lock()
	defer e.rngMu.Unlock()
	return e.RNG.Int64N(n)
}

// Device is a simulated player named name: its own User-Agent, so its
// streaming sessions and requests are its own.
func (e *Env) Device(name string) client.Device {
	ua := "Infuse-Direct/7.7 (streamlab " + name + ")"
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.devices[ua] = name
	return client.Device{BaseURL: e.Stack.URL, UserAgent: ua}
}

// Conn opens a connection of d. Run closes it after the script.
func (e *Env) Conn(d client.Device) *client.Conn {
	c := d.Conn()
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.conns = append(e.rec.conns, c)
	e.rec.connDevice[c] = d
	return c
}

func (e *Env) deviceOf(c *client.Conn) client.Device {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	return e.rec.connDevice[c]
}

func (e *Env) deviceName(ua string) string {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	if n, ok := e.rec.devices[ua]; ok {
		return n
	}
	return ua
}

// PlayerConfig is the player model the params describe.
func (e *Env) PlayerConfig() player.Config {
	return player.Config{
		Bitrate:   e.P.Bytes("bitrate"),
		MaxBuffer: e.P.Bytes("max_buffer"),
		Playable:  e.P.Bytes("playable_bytes"),
		Resume:    e.P.Bytes("resume_bytes"),
	}.WithDefaults()
}

// Player is device d watching t. Every byte is checked, and a body that
// breaks is diagnosed at once.
func (e *Env) Player(d client.Device, t *Title) *player.Session {
	name := e.deviceName(d.UserAgent)
	s := player.NewSession(e.PlayerConfig(), player.Options{
		Path:    t.Path,
		Size:    t.Size,
		Check:   func(off int64, p []byte) { t.Integrity.Check(t.File, off, p) },
		OnError: func(off int64, silence time.Duration, err error) { e.failed(name, t, off, silence, err) },
		Retries: int(e.P.Int("retries")),
	})
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.players = append(e.rec.players, playerRec{device: name, title: t, s: s})
	return s
}

// failed records a body that broke (or a request that failed) at the
// file offset off after silence without a byte, with what the torrent was
// doing at that moment.
func (e *Env) failed(device string, t *Title, off int64, silence time.Duration, err error) {
	f := failure{device: device, title: t, at: time.Now(), offset: off, piece: t.Piece(off), err: err.Error(),
		silence: silence, timedOut: silence >= e.P.Duration("read_timeout")/2, broke: errors.Is(err, io.ErrUnexpectedEOF)}
	if tor, ok := e.Stack.Torrent(t.Hash); ok {
		f.stalled = e.stuck(tor, t.Hash, f.piece)
		f.evicted = !e.cached(tor, f.piece)
	}
	f.wasComplete = e.completions.was(t.Hash, f.piece)
	e.rec.mu.Lock()
	f.expected = e.rec.expected(f)
	e.rec.failures = append(e.rec.failures, f)
	n := len(e.rec.failures)
	e.rec.mu.Unlock()
	if f.timedOut {
		e.dumpStatus(fmt.Sprintf("failure-%d.txt", n), f, t)
	}
}

// knownSince is "#195" when the bodies that broke since t all carry
// #195's signature (at least one did), "" otherwise.
func (e *Env) knownSince(t time.Time) string {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	n := 0
	for _, f := range e.rec.failures {
		if f.at.Before(t) {
			continue
		}
		if f.known() != "#195" {
			return ""
		}
		n++
	}
	if n == 0 {
		return ""
	}
	return "#195"
}

// stuck reports whether piece i is #195's stall: incomplete, with a seeder
// connected and downloads on. swarm.Stalled also wants the piece above
// None, but it's measured when the client sees the break, after the
// timed-out handler closed its reader; a reader open 30 s or more resets
// the file's priorities to None on close (F6), so the piece the read was
// waiting for reads unwanted then. What the clause is for, that an idle or
// paused torrent's reads aren't the stall, is checked directly.
func (e *Env) stuck(tor *atorrent.Torrent, hash string, i int) bool {
	if swarm.Stalled(tor, i) {
		return true
	}
	if tor.Info() == nil || i < 0 || i >= tor.NumPieces() || tor.Stats().ConnectedSeeders < 1 || tor.PieceState(i).Complete {
		return false
	}
	return e.Stack.Activity == nil || e.Stack.Activity.GetState(hash) == torrent.StateActive
}

// dumpStatus writes both clients' status to the repetition's folder, for
// a read that timed out: which side of #195 stalled (PLAN §3, #195's
// signatures A-D) shows in the connections' request queues.
func (e *Env) dumpStatus(name string, f failure, t *Title) {
	if e.dir == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s at %d, piece %d: %s\n%s\n\n== leecher ==\n", f.device, t.Name, f.offset, f.piece, f.err, f.diagnosis())
	e.Stack.Client.WriteStatus(&b)
	for i, p := range t.Peers {
		fmt.Fprintf(&b, "\n== seeder %d (the leecher dials %s) ==\n", i, p.Addr)
		p.Seeder.WriteStatus(&b)
	}
	if t.Remote != nil && e.swarm != nil && e.swarm.cdn != nil {
		e.writeRemoteStatus(&b)
	}
	if os.WriteFile(filepath.Join(e.dir, name), []byte(b.String()), 0o644) == nil {
		e.rec.mu.Lock()
		e.rec.artifacts[strings.TrimSuffix(name, ".txt")] = name
		e.rec.mu.Unlock()
	}
}

func (e *Env) httpError(err error) {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.httpErrors = append(e.rec.httpErrors, err.Error())
}

// Probe reads n bytes of t at off with one Range GET on conn, as a player
// sniffs a header or a footer, and checks the answer and the bytes. A
// probe that breaks is a play-ok failure, a wrong answer an http one;
// only a cancelled context is returned.
func (e *Env) Probe(conn *client.Conn, t *Title, off, n int64) error {
	start := time.Now()
	defer func() {
		e.rec.mu.Lock()
		e.rec.probes = append(e.rec.probes, time.Since(start))
		e.rec.probesDone++
		e.rec.mu.Unlock()
	}()
	device := e.deviceName(e.deviceOf(conn).UserAgent)
	st, err := conn.Get(e.Ctx, t.Path, http.Header{"Range": {rangeOf(off, n)}})
	if err != nil {
		if ctxErr := e.Ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		e.failed(device, t, off, time.Since(start), fmt.Errorf("probe of %d bytes at %d: %w", n, off, err))
		return nil
	}
	defer st.Close()
	if herr := checks.RangeResponse(st.Response, off, off+n-1, t.Size); herr != nil {
		e.httpError(herr)
		return nil
	}
	return e.body(device, t, st, off, n, start)
}

// body reads n bytes of st, a range from the file offset off requested at
// start, checking every byte. A body that ends early is a failure, with
// how long it had been silent; only a cancelled context is returned.
func (e *Env) body(device string, t *Title, st *client.Stream, off, n int64, start time.Time) error {
	buf := make([]byte, 32<<10)
	var got int64
	last := start
	for {
		m, err := st.Read(buf)
		if m > 0 {
			t.Integrity.Check(t.File, off+got, buf[:m])
			got += int64(m)
			last = time.Now()
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) && got == n {
			return nil
		}
		if ctxErr := e.Ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		e.failed(device, t, off+got, time.Since(last), fmt.Errorf("range %d+%d: %d bytes, then: %w", off, n, got, err))
		return nil
	}
}

// Head asks for t's headers and checks them: 200, the size and
// Accept-Ranges. A transport error is returned; a wrong answer is an http
// failure.
func (e *Env) Head(conn *client.Conn, t *Title) (*http.Response, error) {
	resp, err := conn.Head(e.Ctx, t.Path)
	if err != nil {
		return nil, fmt.Errorf("HEAD %s: %w", t.Path, err)
	}
	if resp.StatusCode != http.StatusOK || resp.ContentLength != t.Size || resp.Header.Get("Accept-Ranges") != "bytes" {
		e.httpError(fmt.Errorf("HEAD %s = %d, Content-Length %d, Accept-Ranges %q; want 200, %d, bytes",
			t.Path, resp.StatusCode, resp.ContentLength, resp.Header.Get("Accept-Ranges"), t.Size))
	}
	return resp, nil
}

// Propfind lists p at depth on conn and records its latency.
func (e *Env) Propfind(conn *client.Conn, p string, depth int) ([]client.Entry, error) {
	start := time.Now()
	entries, err := conn.Propfind(e.Ctx, p, depth)
	e.rec.mu.Lock()
	e.rec.propfinds = append(e.rec.propfinds, time.Since(start))
	e.rec.mu.Unlock()
	return entries, err
}

// F3 lists dir (Depth 1) on conn as checks.F3 does, and reports the
// propfind-no-load check and torrents_loaded_by_propfind.
func (e *Env) F3(conn *client.Conn, dir string) (checks.F3Result, error) {
	start := time.Now()
	r, err := checks.F3(e.Ctx, e.Stack, conn, dir)
	e.rec.mu.Lock()
	e.rec.propfinds = append(e.rec.propfinds, time.Since(start))
	e.rec.mu.Unlock()
	if err != nil {
		return r, fmt.Errorf("PROPFIND %s: %w", dir, err)
	}
	e.Metric("torrents_loaded_by_propfind", float64(len(r.Loaded)))
	switch {
	case !r.Conclusive():
		e.Check("propfind-no-load", report.NA, "", r.String())
	case r.LoadedByPropfind():
		e.Check("propfind-no-load", report.Fail, "F3", r.String())
	default:
		e.Check("propfind-no-load", report.Pass, "", r.String())
	}
	return r, nil
}

// Abandon records that the script dropped a request it sent on conn
// outside a player (Range header rng, "" for none): its handler's lag
// after the abort is measured.
func (e *Env) Abandon(conn *client.Conn, path, rng string) {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.aborts = append(e.rec.aborts, clientAbort{ua: e.rec.connDevice[conn].UserAgent, path: path, rng: rng, at: time.Now()})
}

// Go runs fns at once and waits for them all; their errors joined.
func (e *Env) Go(fns ...func() error) error {
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = fn()
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Sleep waits d, or until the context is done.
func (e *Env) Sleep(d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-e.Ctx.Done():
		return e.Ctx.Err()
	case <-t.C:
		return nil
	}
}

// Live drops a playback's error, which the play-ok check judges, and
// returns only a cancelled context: what should stop a script.
func (e *Env) Live(err error) error {
	if ctxErr := e.Ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return nil
}

// Metric records a scenario metric; name must be in Metrics.
func (e *Env) Metric(name string, v float64) {
	if _, ok := LookupMetric(name); !ok {
		panic(fmt.Sprintf("scenario: undeclared metric %q", name))
	}
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.metrics[name] = v
}

// Check records the outcome of one of the scenario's checks. known names
// the known issue whose signature a failure carries.
func (e *Env) Check(id string, status report.Status, known, evidence string) {
	if _, ok := LookupCheck(id); !ok {
		panic(fmt.Sprintf("scenario: undeclared check %q", id))
	}
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.checks[id] = report.RepCheck{ID: id, Status: status, Known: known, Evidence: evidence}
}

// Note adds a line to the repetition's notes.
func (e *Env) Note(format string, args ...any) {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.notes = append(e.rec.notes, fmt.Sprintf(format, args...))
}

// Counter is a Prometheus counter or gauge of the stack, summed over its
// labels.
func (e *Env) Counter(name string) float64 { return checks.Value(e.Stack.Registry, name) }

// ReadersCreated is how many PriorityReaders the stack created: one
// InitialPrioritize each.
func (e *Env) ReadersCreated() float64 {
	return checks.LabelValue(e.Stack.Registry, metricPriorityUpdates, "type", "initial")
}

// WaitReadersCreated waits until ReadersCreated reaches n, up to d.
func (e *Env) WaitReadersCreated(n float64, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for e.ReadersCreated() < n {
		if time.Now().After(deadline) || e.Ctx.Err() != nil {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// Torrent is the leecher's torrent of t, once loaded.
func (e *Env) Torrent(t *Title) (*atorrent.Torrent, bool) { return e.Stack.Torrent(t.Hash) }

// Downloaded is the data the leecher downloaded, over the library's
// torrents.
func (e *Env) Downloaded() int64 {
	var n int64
	for _, h := range e.Stack.Torrents() {
		if tor, ok := e.Stack.Torrent(h); ok {
			st := tor.Stats()
			n += st.BytesReadData.Int64()
		}
	}
	return n
}

// ServerPositions are the positions of t's open PriorityReaders, server
// side (streaming.ReaderTracker), file offsets.
func (e *Env) ServerPositions(t *Title) []int64 {
	var out []int64
	path := t.Layout.Files()[t.File].Path
	for _, r := range e.Stack.Readers.GetReaders(t.Hash) {
		if r.FilePath == path {
			out = append(out, r.Position)
		}
	}
	return out
}

// WaitServerSettled waits until t's server-side reader positions have
// stayed put for quiet, up to limit: a paused player's handler has filled
// the connection's buffers and stopped reading.
func (e *Env) WaitServerSettled(t *Title, quiet, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	last := fmt.Sprint(e.ServerPositions(t))
	since := time.Now()
	for time.Now().Before(deadline) {
		if e.Sleep(20*time.Millisecond) != nil {
			return false
		}
		cur := fmt.Sprint(e.ServerPositions(t))
		if cur != last {
			last, since = cur, time.Now()
			continue
		}
		if time.Since(since) >= quiet {
			return true
		}
	}
	return false
}

// RequestsDone waits until every request of the stack's log that matches
// keep has ended, up to limit.
func (e *Env) RequestsDone(keep func(stack.Request) bool, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for {
		busy := false
		for _, r := range e.Stack.Requests.Active() {
			if keep(r) {
				busy = true
				break
			}
		}
		if !busy {
			return true
		}
		if time.Now().After(deadline) || e.Sleep(5*time.Millisecond) != nil {
			return false
		}
	}
}

// rangeOf is a Range header of [off, off+n) ("bytes=off-" for n 0).
func rangeOf(off, n int64) string {
	if n <= 0 {
		return "bytes=" + strconv.FormatInt(off, 10) + "-"
	}
	return fmt.Sprintf("bytes=%d-%d", off, off+n-1)
}

func quote(ss []string) string { return strings.Join(ss, "; ") }
