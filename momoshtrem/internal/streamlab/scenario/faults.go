package scenario

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
)

// The swarm-fault scenarios (PLAN §5 "Tiers"): the stack under a swarm
// that loses its best peer, one slower than the bitrate, and one with a
// peer that serves a corrupt piece.

func init() {
	catalog = append(catalog, seederLoss, slowSwarm, corruptPiece)
	Checks = append(Checks, faultChecks...)
	Metrics = append(Metrics, faultMetrics...)
}

// UnverifiedReads is the known issue of the responsive reader serving a
// chunk before its piece's hash check (#231).
const UnverifiedReads = "#231"

var faultChecks = []CheckDef{
	{ID: "swarm-recovers", Doc: "The fast seeder the proxy cut off was found again once it came back: the leecher reconnected through the proxy and downloaded from it again, and the playback carried on."},
	{ID: "corrupt-caught", Doc: "The corrupt seeder's piece failed the leecher's hash check and came again, anacrolix banned the corrupt seeder alone (its alias), and the good seeder stayed connected. n/a when the corrupt seeder never served its bad chunks."},
	{ID: "swarm-shaped", Doc: "The swarm was as slow as the scenario says: every byte came through the proxy, its first connection came no sooner than discovery after the first request, and the download ran at most 15% over the seeder's rate. A pacing, proxy or discovery that silently does nothing fails it."},
}

var faultMetrics = []report.MetricDef{
	{Name: "swarm_rate", Kind: report.Count, Unit: "B/s", Better: report.None, Doc: "What came through the slow seeder's proxy, per second from its first connection to its last byte: the pacing as the leecher saw it."},
	{Name: "hash_failures", Kind: report.Count, Unit: "pieces", Better: report.Lower, Doc: "Pieces that failed the leecher's hash check: PiecesDirtiedBad."},
	{Name: "bad_chunks_served", Kind: report.Count, Unit: "chunks", Better: report.None, Doc: "Chunks the lab's corrupt seeders served wrong."},
}

var seederLoss = &Scenario{
	Name: "seeder-loss",
	Tier: Gate,
	Doc: `A movie plays from two seeders: a fast one (seeder_rate) behind a TCP proxy, and a slow one (slow_rate, under the bitrate) dialled directly. Mid-play the fast seeder vanishes for outage: the proxy cuts its connection and refuses new ones. The playback goes on from the slow seeder, draining its buffer (a stall, if it runs dry, is a timing metric, expected). Once the fast seeder is back, the lab adds it again rediscover later (Torrent.AddPeers), as a tracker's next answer would, and the playback catches up.

No read may break meanwhile: the slow seeder keeps every read under the read timeout. swarm-recovers checks the leecher reconnected to the fast seeder and downloaded from it again.`,
	Params: []Param{
		{Name: "slow_rate", Kind: Bytes, Default: "512KiB", Doc: "The slow seeder's upload cap, bytes per second."},
		{Name: "outage", Kind: Duration, Default: "8s", Doc: "How long the fast seeder is gone."},
		{Name: "rediscover", Kind: Duration, Default: "1s", Doc: "How long after it's back the lab adds the fast seeder again."},
		{Name: "watch", Kind: Duration, Default: "4s", Doc: "How much media to play before the outage, and twice that after."},
		{Name: "max_buffer", Kind: Bytes, Default: "6MiB"},
	},
	Swarm: func(p Params) Swarm {
		return Swarm{Seeders: []SeederSpec{
			{Rate: p.Bytes("seeder_rate"), Proxy: &swarm.ProxyConfig{}},
			{Rate: p.Bytes("slow_rate")},
		}}
	},
	Checks: []string{"swarm-recovers"},
	Key:    []string{"stall_time", "rebuffer_ratio", "read_timeouts", "bytes_downloaded"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		fast := t.Peers[0]
		dev := e.Device("infuse")
		watch := e.P.Duration("watch")
		p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		outage := e.P.Duration("outage")
		cut := fast.Proxy.Vanish(outage)
		gone := time.Now()
		if err := e.Live(p.Watch(outage)); err != nil {
			return err
		}
		if err := e.Sleep(time.Until(gone.Add(outage + e.P.Duration("rediscover")))); err != nil {
			return err
		}
		back := fast.Proxy.Stats()
		added, err := e.AddPeers(t, fast)
		if err != nil {
			return err
		}
		if err := e.Live(p.Watch(2 * watch)); err != nil {
			return err
		}
		p.Stop()
		after := fast.Proxy.Stats()
		ev := fmt.Sprintf("the proxy cut %d connections; after the outage the leecher took the fast seeder again (%d), reconnected %d times, and got %s through it",
			cut, added, after.Accepted-back.Accepted, report.Bytes(after.Down-back.Down))
		switch {
		case cut == 0:
			e.Check("swarm-recovers", report.NA, "", "the fast seeder had no connection when it vanished: "+ev)
		case after.Accepted > back.Accepted && after.Down > back.Down:
			e.Check("swarm-recovers", report.Pass, "", ev)
		default:
			e.Check("swarm-recovers", report.Fail, "", ev)
		}
		return nil
	},
}

var slowSwarm = &Scenario{
	Name: "slow-swarm",
	Tier: Gate,
	Doc: `A fresh magnet on a poor swarm: one seeder at swarm_ratio of the bitrate (0.8×), behind a proxy with latency and jitter, found after a discovery lag (no x.pe: the lab adds it as a tracker would), and the info dict fetched from it (stored=false). The player plays from the start.

The player stalls whenever its buffer runs dry: those stalls are expected here, and stall counts and times are timing metrics, which the gate doesn't judge. What it judges is that no read breaks (a slow swarm is slow, not broken: each piece still arrives well within the read timeout), every byte is right, and nothing is left behind; and, so the scenario can't pass by testing nothing, that the swarm was as slow as it says (swarm-shaped: every byte through the proxy, found no sooner than discovery, paced at the seeder's rate).`,
	Params: []Param{
		{Name: "swarm_ratio", Kind: Float, Default: "0.8", Doc: "The seeder's upload cap as a fraction of the bitrate."},
		{Name: "latency", Kind: Duration, Default: "40ms", Doc: "The proxy's latency, each way."},
		{Name: "jitter", Kind: Duration, Default: "20ms", Doc: "The proxy's jitter, each way."},
		{Name: "stored", Kind: Bool, Default: "false"},
		{Name: "discovery", Kind: Duration, Default: "2s"},
		{Name: "watch", Kind: Duration, Default: "20s", Doc: "How much media to play."},
	},
	Swarm: func(p Params) Swarm {
		rate := int64(p.Float("swarm_ratio") * float64(p.Bytes("bitrate")))
		return Swarm{Seeders: []SeederSpec{{Rate: rate,
			Proxy: &swarm.ProxyConfig{Latency: p.Duration("latency"), Jitter: p.Duration("jitter")}}}}
	},
	Checks: []string{"swarm-shaped"},
	Key:    []string{"time_to_playable", "stall_count", "swarm_rate", "read_timeouts"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(e.P.Duration("watch"))); err != nil {
			return err
		}
		p.Stop()
		e.swarmShaped(t, int64(e.P.Float("swarm_ratio")*float64(e.P.Bytes("bitrate"))), e.P.Duration("discovery"))
		return nil
	},
}

// swarmShaped checks a one-seeder swarm behind a proxy was what the
// scenario set up (swarm-shaped): all its bytes through the proxy, found
// no sooner than discovery after the first request, and paced at rate.
func (e *Env) swarmShaped(t *Title, rate int64, discovery time.Duration) {
	px := t.Peers[0].Proxy
	st := px.Stats()
	reqs := e.Stack.Requests.Requests()
	downloaded := e.Downloaded()
	if st.FirstConn.IsZero() || len(reqs) == 0 || downloaded == 0 {
		e.Check("swarm-shaped", report.Fail, "", fmt.Sprintf("nothing came through the proxy (%+v), %d requests, %s downloaded",
			st, len(reqs), report.Bytes(downloaded)))
		return
	}
	found := st.FirstConn.Sub(reqs[0].Start)
	measured := float64(st.Down) / st.LastDown.Sub(st.FirstConn).Seconds()
	e.Metric("swarm_rate", measured)
	ev := fmt.Sprintf("%s through the proxy for %s downloaded; found %v after the first request (discovery %v); %s/s against the seeder's %s/s",
		report.Bytes(st.Down), report.Bytes(downloaded), found.Round(time.Millisecond), discovery,
		report.Bytes(int64(measured)), report.Bytes(rate))
	var wrong []string
	if st.Down < downloaded {
		wrong = append(wrong, "data came around the proxy")
	}
	if found < discovery {
		wrong = append(wrong, "the seeder was found before the discovery lag")
	}
	if measured > 1.15*float64(rate) {
		wrong = append(wrong, "the download ran faster than the seeder's rate")
	}
	if len(wrong) == 0 {
		e.Check("swarm-shaped", report.Pass, "", ev)
	} else {
		e.Check("swarm-shaped", report.Fail, "", ev+": "+strings.Join(wrong, ", "))
	}
}

var corruptPiece = &Scenario{
	Name: "corrupt-piece",
	Tier: Gate,
	Doc: `A player resumes a movie at piece k (corrupt_at), which only one of the two seeders has: the good one lacks it (a partial seeder), and the other serves it wrong three times (the first byte of each chunk flipped), then right. The leecher's hash check fails k three times, k comes a fourth time, right, and anacrolix smart-bans the corrupt seeder; the playback carries on from the good one. corrupt-caught checks the hash failures, the ban of the corrupt seeder alone and that the good seeder stayed.

The corrupt seeder is dialled by its alias (swarm.Alias, 0.0.0.0): every lab peer is 127.0.0.1, and a ban of that would drop the good seeder too, where production drops the bad peer alone.

The reader is responsive (PriorityReader sets anacrolix's SetResponsive): it returns chunks before their piece is verified. Resuming at k, it waits at k's first chunk and gets it, flipped, before the hash check fails the piece. The integrity check shows those bytes every time: #231, whose signature is that every mismatched byte is one the corrupt seeder flipped. (A bad serve the reader misses needs k's first chunk to arrive last, 1 in 32; three bad serves make every one missed about 1 in 30000.)`,
	Guards: []string{UnverifiedReads},
	Params: []Param{
		{Name: "corrupt_at", Kind: Bytes, Default: "24MiB", Doc: "Where the player resumes and the corrupt piece is: the piece holding this offset of the movie, past the header the first reader prioritizes."},
		{Name: "corrupt_serves", Kind: Int, Default: "3", Doc: "How many times the corrupt seeder serves the piece wrong before it serves it right."},
		{Name: "hold", Kind: Duration, Default: "500ms", Doc: "How long the corrupt seeder holds the last chunk of its first bad serve, the piece unverified meanwhile."},
		{Name: "file_size", Kind: Bytes, Default: "48MiB"},
		{Name: "watch", Kind: Duration, Default: "10s", Doc: "How much media to play from the corrupt piece on."},
	},
	Swarm: func(p Params) Swarm {
		k := int(p.Bytes("corrupt_at") / p.Bytes("piece_length"))
		return Swarm{Seeders: []SeederSpec{
			{Rate: p.Bytes("seeder_rate"), Missing: []int{k}},
			{Rate: p.Bytes("seeder_rate"), Alias: true, Corrupt: []int{k}, CorruptServes: int(p.Int("corrupt_serves")),
				CorruptHold: p.Duration("hold")},
		}}
	},
	Checks: []string{"corrupt-caught"},
	Key:    []string{"integrity_errors", "hash_failures", "bad_chunks_served", "stall_time"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		good, bad := t.Peers[0], t.Peers[1]
		dev := e.Device("infuse")
		from := e.P.Bytes("corrupt_at") / e.P.Bytes("piece_length") * e.P.Bytes("piece_length")
		p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), from)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(e.P.Duration("watch"))); err != nil {
			return err
		}
		p.Stop()
		served := bad.Seeder.BadServes()
		tor, ok := e.Torrent(t)
		if !ok {
			return fmt.Errorf("the torrent of %s isn't loaded", t.Name)
		}
		st := tor.Stats()
		banned := e.Stack.Client.BadPeerIPs()
		goodOn := false
		for _, pc := range tor.PeerConns() {
			if pc.RemoteAddr.String() == good.Addr {
				goodOn = true
			}
		}
		ev := fmt.Sprintf("%d bad chunks served, %d pieces failed the hash check, banned %v, the good seeder %s",
			served, st.PiecesDirtiedBad.Int64(), banned, map[bool]string{true: "connected", false: "gone"}[goodOn])
		aliasHost := strings.Split(bad.Addr, ":")[0]
		switch {
		case served == 0:
			e.Check("corrupt-caught", report.NA, "", ev)
		case st.PiecesDirtiedBad.Int64() >= 1 && slices.Equal(banned, []string{aliasHost}) && goodOn:
			e.Check("corrupt-caught", report.Pass, "", ev)
		default:
			e.Check("corrupt-caught", report.Fail, "", ev)
		}
		return nil
	},
}

// explainUnverified has t's torrent's integrity check explain the bytes
// its corrupt seeders flip as UnverifiedReads: a mismatch at a corrupt
// piece's chunk start, the media's byte flipped. Any other mismatch stays
// unexplained.
func explainUnverified(ts *torrentSwarm, integrity *checks.Integrity) {
	corrupt := map[int]bool{}
	for _, p := range ts.peers {
		for _, k := range p.Spec.Corrupt {
			corrupt[k] = true
		}
	}
	if len(corrupt) == 0 {
		return
	}
	pl := ts.layout.PieceLength()
	integrity.Explain(func(b checks.BadByte) string {
		if corrupt[b.Piece] && swarm.BadByte(b.Torrent-int64(b.Piece)*pl) && b.Got == b.Want^0xff {
			return UnverifiedReads
		}
		return ""
	})
}

// faultMetrics sets the swarm's fault counts: the hash failures, and the
// bad chunks when a seeder serves any.
func (c *collector) faultMetrics() {
	ss := c.e.swarm
	if ss == nil || len(ss.torrents) == 0 {
		return
	}
	var hashFailures, badChunks int64
	corrupt := false
	for _, ts := range ss.torrents {
		for _, p := range ts.peers {
			if len(p.Spec.Corrupt) > 0 {
				corrupt = true
				badChunks += int64(p.Seeder.BadServes())
			}
		}
		if tor, ok := c.e.Stack.Torrent(ts.hash); ok {
			st := tor.Stats()
			hashFailures += st.PiecesDirtiedBad.Int64()
		}
	}
	c.set("hash_failures", float64(hashFailures))
	if corrupt {
		c.set("bad_chunks_served", float64(badChunks))
	}
}
