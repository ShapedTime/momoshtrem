// Package scenario is streamlab's scenarios (docs/streamlab/PLAN.md §5
// "Tiers"): what a player like Infuse does to the stack, defined in Go,
// and Run, which runs one repetition in this process and measures it.
//
// A scenario declares its media (torrents and the library entries that
// play them), its swarm, the stack's config, its params and its script;
// the script drives devices and players through an Env. Run adds what
// every scenario gets: the common checks (script, integrity, http,
// play-ok, quiescence) and metrics, the residual download and resource
// peaks.
package scenario

import (
	"fmt"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	lib "github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
)

// Tier groups scenarios by purpose and cost.
type Tier string

const (
	// Gate scenarios take 15-45 s each, with the seeder rate-limited above
	// the bitrate unless the scenario says otherwise.
	Gate Tier = "gate"
)

// Scenario is one Infuse-like workload.
type Scenario struct {
	Name string
	Tier Tier
	// Doc says what the scenario does and why; its first paragraph is
	// its summary.
	Doc string
	// Guards are the commits and suspects (PLAN §3) it protects.
	Guards []string
	// Params are its own params, and common params with another default.
	Params []Param
	// Media is its content; one MKV movie (MovieMedia) when nil.
	Media func(Params) Media
	// Swarm is its seeders; one per torrent at seeder_rate when nil.
	Swarm func(Params) Swarm
	// Stack is the stack's config beyond the common params; none when nil.
	Stack func(Params) Stack
	// Script drives the stack. An error means it couldn't run its steps
	// (the "script" check fails); a playback that breaks isn't an error:
	// the play-ok check judges it.
	Script func(*Env) error
	// Checks are its own checks; every scenario also gets CommonChecks.
	Checks []string
	// Key are the metrics a run's index shows for it.
	Key []string
	// Timeout is a repetition's hard timeout; DefaultTimeout when 0.
	Timeout time.Duration
}

// DefaultTimeout is a repetition's hard timeout when the scenario sets
// none: a gate scenario takes under a minute, settling included.
const DefaultTimeout = 3 * time.Minute

// HardTimeout is the scenario's hard timeout.
func (sc *Scenario) HardTimeout() time.Duration {
	if sc.Timeout > 0 {
		return sc.Timeout
	}
	return DefaultTimeout
}

// AllParams are the common params with the scenario's laid over them,
// sorted by name.
func (sc *Scenario) AllParams() []Param {
	byName := map[string]Param{}
	for _, p := range CommonParams {
		byName[p.Name] = p
	}
	for _, p := range sc.Params {
		if c, ok := byName[p.Name]; ok && p.Doc == "" {
			p.Doc = c.Doc
		}
		byName[p.Name] = p
	}
	out := make([]Param, 0, len(byName))
	for _, p := range byName {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Param) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}
		return 0
	})
	return out
}

// AllChecks are the common checks, then the scenario's.
func (sc *Scenario) AllChecks() []string {
	return append(slices.Clone(CommonChecks), sc.Checks...)
}

// Resolve checks sets against the scenario's params.
func (sc *Scenario) Resolve(sets map[string]string) (Params, error) {
	p, err := resolve(sc.AllParams(), sets)
	if err != nil {
		return Params{}, fmt.Errorf("%s: %w", sc.Name, err)
	}
	return p, nil
}

// Media is a scenario's content: torrents and the library entries that
// play their files, and HDRezka movies (Remote).
type Media struct {
	Torrents []media.Spec
	Movies   []MovieRef
	Episodes []EpisodeRef
	// Remote are HDRezka movies, which the fake CDN serves.
	Remote []RemoteRef
}

// RemoteRef is an HDRezka movie: Stream, in its quality, whose bytes are
// the first file of Spec, served by the fake CDN.
type RemoteRef struct {
	Title  string
	Year   int
	Spec   media.Spec
	Stream lib.StreamRef
}

// MovieRef is a movie playing file File of torrent Torrent.
type MovieRef struct {
	Title         string
	Year          int
	Torrent, File int
}

// EpisodeRef is an episode playing file File of torrent Torrent.
type EpisodeRef struct {
	Show                  string
	Year, Season, Episode int
	Name                  string
	Torrent, File         int
}

// Swarm is a scenario's seeders: one per torrent, or Seeders.
type Swarm struct {
	// SeederRate caps each seeder's upload, in bytes per second; 0 is
	// unlimited.
	SeederRate int64
	// Seeders, when set, are each torrent's seeders, in place of the one
	// at SeederRate.
	Seeders []SeederSpec
}

// SeederSpec is one seeder of a torrent, and how the leecher reaches it.
type SeederSpec struct {
	// Rate caps its upload, in bytes per second; 0 is unlimited.
	Rate int64
	// Proxy puts it behind a TCP proxy shaped so (latency, jitter,
	// bandwidth), whose address is the one the leecher dials; nil: the
	// leecher dials the seeder.
	Proxy *swarm.ProxyConfig
	// Alias has the leecher dial it by its alias (swarm.Alias), so
	// anacrolix sees an IP of its own: banning it drops it alone.
	Alias bool
	// Corrupt, CorruptServes, CorruptHold and Missing are its
	// swarm.SeederConfig's.
	Corrupt       []int
	CorruptServes int
	CorruptHold   time.Duration
	Missing       []int
}

// Stack is a scenario's stack config beyond the common params.
type Stack struct {
	// Tune changes the config after the common params are applied.
	Tune func(*config.Config)
	// IdleCheckInterval is the activity manager's (SetCheckInterval); 0
	// keeps production's 30 s.
	IdleCheckInterval time.Duration
	// HDRezkaReadTimeout bounds a remote file's opens and silent reads;
	// production's 30 s when 0 (stack.HDRezkaReadTimeout).
	HDRezkaReadTimeout time.Duration
}

// CommonParams are every scenario's params. A scenario redeclares one to
// change its default.
var CommonParams = []Param{
	{Name: "seed", Kind: Int, Default: "1", Doc: "Seeds the media's content (so its info hash) and the script's RNG."},
	{Name: "bitrate", Kind: Bytes, Default: "1MiB", Doc: "The player's bitrate, bytes per second."},
	{Name: "max_buffer", Kind: Bytes, Default: "16MiB", Doc: "The player stops reading while its buffer holds this much."},
	{Name: "playable_bytes", Kind: Bytes, Default: "0", Doc: "Bytes buffered before playback starts; 0 is 2 s of the bitrate."},
	{Name: "resume_bytes", Kind: Bytes, Default: "0", Doc: "Bytes buffered again that end a stall; 0 is playable_bytes."},
	{Name: "retries", Kind: Int, Default: "2", Doc: "How often a playback reconnects after a broken body before it gives up."},
	{Name: "seeder_rate", Kind: Bytes, Default: "3MiB", Doc: "Each seeder's upload cap, bytes per second; 0 is unlimited. Above the bitrate unless a scenario says otherwise."},
	{Name: "piece_length", Kind: Bytes, Default: "512KiB", Doc: "The torrents' piece length."},
	{Name: "file_size", Kind: Bytes, Default: "96MiB", Doc: "The movie's size."},
	{Name: "stored", Kind: Bool, Default: "true", Doc: "The metainfo is in the leecher's store, as for most production torrents; false: the info dict comes from the seeder."},
	{Name: "start_paused", Kind: Bool, Default: "true", Doc: "torrent.start_paused, production's default. Set after --config: it overrides the file's."},
	{Name: "read_timeout", Kind: Duration, Default: "10s", Doc: "torrent.read_timeout, set after --config: it overrides the file's. Production's is 120 s (--set read_timeout=120s); a #195 stall lasts about a minute, so the lab's shorter timeout bounds what one costs."},
	{Name: "settle", Kind: Duration, Default: "15s", Doc: "How long the quiescence check waits for the stack to be quiet after the last request."},
	{Name: "residual_window", Kind: Duration, Default: "3s", Doc: "How long after quiescence the residual download keeps being measured."},
	{Name: "probe_bytes", Kind: Bytes, Default: "64KiB", Doc: "A header or footer probe's size."},
	{Name: "discovery", Kind: Duration, Default: "0s", Doc: "How long the leecher takes to find the seeders: the magnet carries no x.pe, and the lab adds them as a tracker would (Torrent.AddPeers, untrusted) this long after the torrent loads. 0: x.pe, at once."},
}

// CheckDef describes a check.
type CheckDef struct {
	ID  string
	Doc string
}

// CommonChecks are every scenario's checks.
var CommonChecks = []string{"script", "integrity", "http", "play-ok", "quiescence"}

// Checks describes every check a scenario can report.
var Checks = []CheckDef{
	{ID: "script", Doc: "The scenario ran its steps to the end: no request it needed failed outright."},
	{ID: "integrity", Doc: "Every byte received matched the media: zero mismatched bytes."},
	{ID: "http", Doc: "Every Range GET answered 206 with the right Content-Range and Content-Length, every HEAD 200 with the size and Accept-Ranges, and no handler panicked."},
	{ID: "play-ok", Doc: "Every playback and probe got its bytes: no body broke before its end and no request failed. Known signatures, when every failure of the repetition carries the same one: #195, a body silent for half the read timeout or more that broke at an incomplete piece with the seeder connected and downloads on, on at most two pieces; F11, a body that broke within 3 s of its last byte at a piece anacrolix had complete and the cache no longer holds. A script's Env.Expect excuses the bodies a fault it injects is meant to break, before their first byte at the offset it names, and no others."},
	{ID: "quiescence", Doc: "Within the settle time after the last request: no request, reader, open file, blocked read or in-flight request left, no goroutine a request leaked, and no session left once the sessions clock is skewed 31 s; with HDRezka, no CDN body open and all 8 streamer slots free. #225 explains a failure where nothing is left but goroutines in anacrolix's waitAvailable under TorrentFile.readContext, after a read timed out."},
	{ID: "propfind-no-load", Doc: "A Depth 1 PROPFIND of a folder loads none of its torrents (F3). n/a when one was loaded before (F3Result.Conclusive)."},
	{ID: "no-sniff-reads", Doc: "PROPFINDs and HEADs create no reader: nothing sniffs content at offset 0 (F1). Where the MIME table resolves .mkv it passes trivially."},
	{ID: "etag-stable", Doc: "HEADs and PROPFINDs of one file give one ETag, and a Range GET with that ETag as If-Range answers 206 (F2). F2's signature: several ETags, and the If-Range GET answered 200."},
	{ID: "abort-lag", Doc: "Every handler ends within abort_lag_max of its client dropping the request (F12). F12's signature: it ended within the read timeout plus a second. An HDRezka file's handler carries #232's: it ended within the remote read timeout plus a second."},
	{ID: "close-keeps-windows", Doc: "Closing a reader lowers no incomplete piece of another open reader's window (F6; its signature: every lowered piece went to None). n/a when that window had no incomplete wanted piece, or the closing handler didn't return."},
	{ID: "open-keeps-windows", Doc: "Opening a reader lowers no incomplete piece of another open reader's window (F13; its signature: every lowered piece is in the file's header or footer). n/a when that window had no incomplete piece above High where the new reader prioritizes."},
	{ID: "cache-holds-complete", Doc: "No piece anacrolix calls complete was evicted from the piece cache (F11). n/a when everything complete fits in the cache."},
	{ID: "idle-resume", Doc: "A pause past the idle timeout idles the torrent, and playback resumes after it. n/a when the playback broke before the pause. #195 explains a failure when every body that broke after the resume carries its signature."},
	{ID: "next-prefetch", Doc: "Opening an episode prefetches the next one's header at Normal: some of it is complete when the next episode opens (3907536)."},
}

// LookupCheck returns the check's description.
func LookupCheck(id string) (CheckDef, bool) {
	for _, c := range Checks {
		if c.ID == id {
			return c, true
		}
	}
	return CheckDef{}, false
}
