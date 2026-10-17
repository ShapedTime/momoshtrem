package scenario

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"runtime"
	"runtime/metrics"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// RunOptions are a repetition's options.
type RunOptions struct {
	// Rep numbers the repetition (0 is the warm-up).
	Rep int
	// Sets are the --set overrides of the scenario's params.
	Sets map[string]string
	// ConfigPath is a YAML config laid over the defaults (stack.Options).
	ConfigPath string
	// Cache keeps the media's metainfo; media.DefaultCache() when nil.
	Cache *media.Cache
	// Dir is the repetition's folder: artifacts (both clients' status at
	// a timed-out read) go there. Empty keeps none.
	Dir string
}

// ToolError is an error of the tool or the environment, not of the
// system under test: bad params, media that can't be built, a seeder or
// a stack that can't start.
type ToolError struct{ Err error }

func (e *ToolError) Error() string { return e.Err.Error() }
func (e *ToolError) Unwrap() error { return e.Err }

func toolErr(format string, args ...any) error {
	return &ToolError{Err: fmt.Errorf(format, args...)}
}

// Run runs one repetition of sc in this process: media, seeders and the
// stack, the script, then the end of every request, quiescence, the
// residual download, and the checks and metrics. Failures of the system
// under test are in the returned repetition's checks; the error is a
// ToolError, or the context's.
func Run(ctx context.Context, sc *Scenario, opts RunOptions) (*report.Rep, error) {
	started := time.Now()
	p, err := sc.Resolve(opts.Sets)
	if err != nil {
		return nil, &ToolError{Err: err}
	}
	seed := uint64(p.Int("seed"))
	h := fnv.New64a()
	h.Write([]byte(sc.Name))
	e := &Env{
		Ctx: ctx,
		P:   p,
		RNG: rand.New(rand.NewPCG(seed, h.Sum64())),
		rec: recorder{
			devices:    map[string]string{},
			connDevice: map[*client.Conn]client.Device{},
			metrics:    map[string]float64{},
			checks:     map[string]report.RepCheck{},
			artifacts:  map[string]string{},
		},
		dir: opts.Dir,
	}

	m := MovieMedia(p)
	if sc.Media != nil {
		m = sc.Media(p)
	}
	sw := Swarm{SeederRate: p.Bytes("seeder_rate")}
	if sc.Swarm != nil {
		sw = sc.Swarm(p)
	}
	var st Stack
	if sc.Stack != nil {
		st = sc.Stack(p)
	}

	cache := opts.Cache
	if cache == nil {
		cache = media.DefaultCache()
	}
	ss, err := startSwarm(m, sw, p.Duration("discovery"), seed, cache)
	if err != nil {
		return nil, err
	}
	defer ss.close()
	e.swarm = ss

	so, titles, err := library(m, ss)
	if err != nil {
		return nil, toolErr("%w", err)
	}
	so.ConfigPath = opts.ConfigPath
	so.IdleCheckInterval = st.IdleCheckInterval
	if ss.links != nil {
		so.HDRezka, so.HDRezkaReadTimeout = ss.links, st.HDRezkaReadTimeout
	}
	so.Tune = func(c *config.Config) {
		c.Torrent.ReadTimeout = int(math.Ceil(p.Duration("read_timeout").Seconds()))
		c.Torrent.StartPaused = p.Bool("start_paused")
		if st.Tune != nil {
			st.Tune(c)
		}
	}
	if p.Bool("stored") {
		so.Stored = ss.infos()
	}
	s, err := stack.Start(so)
	if err != nil {
		return nil, toolErr("%w", err)
	}
	defer s.Close()
	e.Stack, e.Titles = s, titles
	if d := p.Duration("discovery"); d > 0 {
		stopDiscovery := make(chan struct{})
		defer close(stopDiscovery)
		ss.discover(s, d, stopDiscovery)
	}

	samples := startSampler(s)
	e.watchCompletions()
	defer e.completions.stopWatching()
	scriptStart := time.Now()
	scriptErr := runScript(sc, e)
	wall := time.Since(scriptStart)

	// End every playback (an open request is an abort to the server) and
	// drop every connection: the last close.
	players := e.endPlayers()
	e.closeConns()
	downloadedAtClose := e.Downloaded()

	q := checks.Quiescence(context.WithoutCancel(ctx), s, p.Duration("settle"))
	rq := e.remoteQuiescence(p.Duration("settle") - q.Waited)
	residualWindow := p.Duration("residual_window")
	time.Sleep(residualWindow)
	downloadedAfter := e.Downloaded()
	res := samples.stop()

	rep := &report.Rep{
		Rep:     opts.Rep,
		Warmup:  opts.Rep == 0,
		Status:  report.RepOK,
		Started: started,
		Config:  report.Config{Fingerprint: s.Fingerprint, Path: opts.ConfigPath},
		Params:  p.Values(),
		Seed:    seed,
		Metrics: map[string]float64{},
	}
	c := collector{e: e, sc: sc, rep: rep, players: players, quiet: q, remote: rq, scriptErr: scriptErr}
	c.metrics(wall, downloadedAtClose, downloadedAfter, residualWindow)
	c.sessions()
	c.checks()
	res.Leaked, res.Inherited = leaks(q), q.Inherited
	rep.Resources = res
	rep.Notes = append(rep.Notes, e.rec.notes...)
	if len(e.rec.artifacts) > 0 {
		rep.Artifacts = e.rec.artifacts
	}
	if err := s.Close(); err != nil {
		rep.Notes = append(rep.Notes, "stack close: "+err.Error())
	}
	rep.WallMS = report.MS(time.Since(started))
	return rep, ctx.Err()
}

// runScript runs the script, turning a panic in it into its error: the
// stack must still be torn down and measured.
func runScript(sc *Scenario, e *Env) (err error) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 64<<10)
			buf = buf[:runtime.Stack(buf, false)]
			err = fmt.Errorf("script panicked: %v\n%s", r, buf)
		}
	}()
	if sc.Script == nil {
		return errors.New("no script")
	}
	return sc.Script(e)
}

// library builds the stack's library from the media: each title's paths,
// its torrent's magnet and seeders and its integrity check (one per
// torrent), then the HDRezka movies.
func library(m Media, ss *swarmSet) (stack.Options, []*Title, error) {
	var so stack.Options
	var titles []*Title
	integrity := make([]*checks.Integrity, len(ss.torrents))
	for i, ts := range ss.torrents {
		integrity[i] = newIntegrity(ts.layout)
		explainUnverified(ts, integrity[i])
	}
	source := func(torrent, file int) (stack.Source, error) {
		if torrent < 0 || torrent >= len(ss.torrents) || file < 0 || file >= len(ss.torrents[torrent].layout.Files()) {
			return stack.Source{}, fmt.Errorf("no file %d in torrent %d", file, torrent)
		}
		ts := ss.torrents[torrent]
		return stack.Source{Magnet: ts.magnet, Layout: ts.layout, File: file}, nil
	}
	title := func(name, path, dir string, src stack.Source, torrent int) *Title {
		f := src.Layout.Files()[src.File]
		ts := ss.torrents[torrent]
		return &Title{Name: name, Path: path, Dir: dir, Hash: ts.hash, Layout: src.Layout, File: src.File,
			Offset: f.Offset, Size: f.Size, Integrity: integrity[torrent], Seeder: ts.peers[0].Seeder, Peers: ts.peers}
	}
	for _, ref := range m.Movies {
		src, err := source(ref.Torrent, ref.File)
		if err != nil {
			return so, nil, fmt.Errorf("movie %q: %w", ref.Title, err)
		}
		mv := stack.Movie{Title: ref.Title, Year: ref.Year, Source: src}
		so.Movies = append(so.Movies, mv)
		titles = append(titles, title(fmt.Sprintf("%s (%d)", ref.Title, ref.Year), mv.Path(), mv.Dir(), src, ref.Torrent))
	}
	for _, ref := range m.Episodes {
		src, err := source(ref.Torrent, ref.File)
		if err != nil {
			return so, nil, fmt.Errorf("episode %q S%02dE%02d: %w", ref.Show, ref.Season, ref.Episode, err)
		}
		ep := stack.Episode{Show: ref.Show, Year: ref.Year, Season: ref.Season, Episode: ref.Episode, Name: ref.Name, Source: src}
		so.Episodes = append(so.Episodes, ep)
		titles = append(titles, title(fmt.Sprintf("%s S%02dE%02d", ref.Show, ref.Season, ref.Episode), ep.Path(), ep.Dir(), src, ref.Torrent))
	}
	titles = append(titles, ss.remoteLibrary(m, &so)...)
	return so, titles, nil
}

// MovieMedia is one movie: an MKV of file_size in a single-file torrent.
func MovieMedia(p Params) Media {
	spec := media.SingleFile("Film.2020.1080p.mkv", uint64(p.Int("seed")), p.Bytes("piece_length"), p.Bytes("file_size"), media.MKV)
	return Media{Torrents: []media.Spec{spec}, Movies: []MovieRef{{Title: "Film", Year: 2020}}}
}

// SeasonMedia is a season of episodes of one size: an episode pack, each
// file an episode of the library.
func SeasonMedia(p Params, episodes int, size int64) Media {
	spec := media.EpisodePack("Show", 1, episodes, uint64(p.Int("seed")), p.Bytes("piece_length"), size, media.MKV)
	m := Media{Torrents: []media.Spec{spec}}
	for i := range episodes {
		m.Episodes = append(m.Episodes, EpisodeRef{Show: "Show", Year: 2024, Season: 1, Episode: i + 1, File: i})
	}
	return m
}

// endPlayers ends every playback and returns the sessions' stats.
func (e *Env) endPlayers() []playerResult {
	e.rec.mu.Lock()
	players := append([]playerRec(nil), e.rec.players...)
	e.rec.mu.Unlock()
	out := make([]playerResult, len(players))
	for i, pr := range players {
		out[i] = playerResult{playerRec: pr, stats: pr.s.Stats()}
	}
	return out
}

func (e *Env) closeConns() {
	e.rec.mu.Lock()
	conns := append([]*client.Conn(nil), e.rec.conns...)
	e.rec.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

// sampler records the process's peaks while the scenario runs.
type sampler struct {
	stopc chan struct{}
	done  chan report.Resources
}

func startSampler(s *stack.Stack) *sampler {
	sm := &sampler{stopc: make(chan struct{}), done: make(chan report.Resources, 1)}
	go func() {
		var r report.Resources
		heap := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			r.PeakGoroutines = max(r.PeakGoroutines, runtime.NumGoroutine())
			r.PeakReaders = max(r.PeakReaders, s.Readers.Count())
			metrics.Read(heap)
			if heap[0].Value.Kind() == metrics.KindUint64 {
				r.PeakHeapBytes = max(r.PeakHeapBytes, heap[0].Value.Uint64())
			}
			select {
			case <-sm.stopc:
				sm.done <- r
				return
			case <-t.C:
			}
		}
	}()
	return sm
}

func (sm *sampler) stop() report.Resources {
	close(sm.stopc)
	return <-sm.done
}

func leaks(q checks.QuiescenceResult) []report.Leak {
	var out []report.Leak
	for _, g := range q.Leaked {
		out = append(out, report.Leak{Request: g.Request, Count: g.Count, Stack: g.Stack})
	}
	return out
}
