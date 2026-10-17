package swarm_test

import (
	"os"
	"strings"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
)

// The fault tests run the production read path (newLeecher) against a
// seeder under a fault: a corrupt piece, a proxy that shapes or cuts the
// link, a peer found late. Every byte read is checked, and nothing is
// timed tightly.

// faultRun is a seeder, maybe behind a proxy, and a leecher with the
// metainfo stored, reading one file.
type faultRun struct {
	layout *media.Layout
	mi     *metainfo.MetaInfo
	seeder *swarm.Seeder
	l      *leecher
	hash   string
}

func newFaultRun(t *testing.T, spec media.Spec, sc swarm.SeederConfig) *faultRun {
	t.Helper()
	layout, err := media.New(spec)
	if err != nil {
		t.Fatal(err)
	}
	mi, err := media.BuildMetainfo(layout)
	if err != nil {
		t.Fatal(err)
	}
	sc.Layout, sc.Metainfo = layout, mi
	seeder, err := swarm.StartSeeder(sc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { seeder.Close() })
	r := &faultRun{layout: layout, mi: mi, seeder: seeder, l: newLeecher(t, false, smokeReadTimeout),
		hash: mi.HashInfoBytes().HexString()}
	if err := r.l.store.Save(r.hash, mi); err != nil {
		t.Fatal(err)
	}
	return r
}

// checker opens the file through magnet and returns a checker of it.
func (r *faultRun) checker(t *testing.T, magnet string) *checker {
	t.Helper()
	f := r.layout.Files()[0]
	file, tor := r.l.open(t, magnet, f.Path)
	t.Cleanup(func() { file.Close() })
	return &checker{t: t, file: file, torrent: tor, layout: r.layout, f: f,
		status: func() string {
			var b strings.Builder
			b.WriteString("leecher: " + r.l.status(r.hash) + "\nseeder: ")
			r.seeder.WriteStatus(&b)
			return b.String()
		},
	}
}

// unverified explains the mismatches of a read that the responsive reader
// served before the hash check: a corrupt piece's flipped bytes
// (swarm.BadByte), and nothing else.
func unverified(layout *media.Layout, bad int) func(abs int64, got, want byte) bool {
	return func(abs int64, got, want byte) bool {
		piece := abs / layout.PieceLength()
		return int(piece) == bad && swarm.BadByte(abs-piece*layout.PieceLength()) && got == want^0xff
	}
}

// Piece 5 is only on a seeder that serves it wrong, once: the leecher's
// hash check fails it, the piece comes again, right this time, and the
// read carries on. That seeder is dialled by its alias (swarm.Alias), so
// the smart ban that follows the piece passing bans it alone, and the
// other seeder carries on, as in production.
//
// The reader is responsive: it reads chunks before their piece is
// verified, so it can get the flipped bytes before the hash check fails
// them. Those, and only those, are tolerated here (unverified); the
// corrupt-piece scenario shows them every time.
func TestFaults_CorruptPiece(t *testing.T) {
	const bad = 5
	r := newFaultRun(t, media.SingleFile("Film.2020.mkv", 41, 256<<10, 4<<20, media.MKV),
		swarm.SeederConfig{Missing: []int{bad}})
	corrupt, err := swarm.StartSeeder(swarm.SeederConfig{Layout: r.layout, Metainfo: r.mi,
		Corrupt: []int{bad}, CorruptServes: 1, Name: "streamlab-seeder-corrupt"})
	if err != nil {
		t.Fatal(err)
	}
	defer corrupt.Close()
	c := r.checker(t, r.seeder.MagnetVia(r.seeder.Addr(), swarm.Alias(corrupt.Addr())))
	c.known = unverified(r.layout, bad)
	c.sequential(0, c.f.Size, 32<<10)
	chunks := int(r.layout.PieceLength() / swarm.ChunkSize)
	if n := corrupt.BadServes(); n != chunks {
		t.Errorf("the corrupt seeder served %d bad chunks, want the piece's %d once", n, chunks)
	}
	st := c.torrent.Stats()
	if st.PiecesDirtiedBad.Int64() < 1 {
		t.Errorf("the leecher saw no piece fail its hash: %+v", st.ConnStats)
	}
	if banned := r.l.client.BadPeerIPs(); len(banned) != 1 || banned[0] != "0.0.0.0" {
		t.Errorf("banned IPs %v, want the corrupt seeder's alias alone", banned)
	}
	if st.ActivePeers != 1 {
		t.Errorf("%d peers connected after the ban, want the good seeder\n%s", st.ActivePeers, c.status())
	}
	t.Logf("%d flipped bytes read before the hash check failed them", c.bad)
}

// The same corruption from a seeder dialled by its IP: once the piece
// passes, smart ban bans 127.0.0.1, which is every lab peer. It documents
// why swarm.Alias exists; STREAMLAB=1 runs it.
func TestFaults_CorruptPieceByIPBansTheLabSwarm(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run it")
	}
	const bad = 5
	r := newFaultRun(t, media.SingleFile("Film.2020.mkv", 42, 256<<10, 4<<20, media.MKV),
		swarm.SeederConfig{Corrupt: []int{bad}, CorruptServes: 1})
	c := r.checker(t, r.seeder.Magnet())
	c.known = unverified(r.layout, bad)
	c.read(0, int(r.layout.PieceLength())*(bad+1)) // through the bad piece
	deadline := time.Now().Add(10 * time.Second)
	for c.torrent.Stats().ActivePeers > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := c.torrent.Stats().ActivePeers; n != 0 {
		t.Fatalf("%d peers still connected: the lab swarm wasn't banned", n)
	}
	t.Logf("as expected: banned %v, every lab peer dropped", r.l.client.BadPeerIPs())
}

// Through a proxy with latency, jitter and a bandwidth cap, every byte
// still arrives right, and all of it went through the proxy.
func TestFaults_Proxy(t *testing.T) {
	r := newFaultRun(t, media.SingleFile("Film.2020.mkv", 43, 256<<10, 4<<20, media.MKV), swarm.SeederConfig{})
	p, err := swarm.StartProxy(r.seeder.Addr(), swarm.ProxyConfig{Latency: 10 * time.Millisecond,
		Jitter: 10 * time.Millisecond, Rate: 8 << 20, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c := r.checker(t, r.seeder.MagnetVia(p.Addr()))
	c.sequential(0, c.f.Size, 32<<10)
	if st := p.Stats(); st.Down < c.f.Size || st.Accepted != 1 {
		t.Errorf("proxy stats %+v: the %d bytes didn't all pass through it", st, c.f.Size)
	}
}

// Without x.pe the leecher has no peer: its read waits until the seeder
// is added, as a tracker's answer or the DHT would add it.
func TestFaults_DiscoveryDelay(t *testing.T) {
	r := newFaultRun(t, media.SingleFile("Film.2020.mkv", 44, 256<<10, 2<<20, media.MKV), swarm.SeederConfig{})
	c := r.checker(t, r.seeder.MagnetVia())
	const delay = 500 * time.Millisecond
	start := time.Now()
	go func() {
		time.Sleep(delay)
		c.torrent.AddPeers([]atorrent.PeerInfo{{Addr: atorrent.StringAddr(r.seeder.Addr()), Source: atorrent.PeerSourceTracker}})
	}()
	c.read(0, 64<<10)
	if waited := time.Since(start); waited < delay {
		t.Errorf("the first read returned after %v, before the seeder was added (%v)", waited, delay)
	}
	c.sequential(64<<10, c.f.Size, 32<<10)
}

// The seeder vanishes mid-read and comes back; once it's added again, the
// read carries on. STREAMLAB=1 runs it: it waits through the outage.
func TestFaults_SeederLoss(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run it")
	}
	r := newFaultRun(t, media.SingleFile("Film.2020.mkv", 45, 256<<10, 8<<20, media.MKV), swarm.SeederConfig{UploadRate: 1 << 20})
	p, err := swarm.StartProxy(r.seeder.Addr(), swarm.ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c := r.checker(t, r.seeder.MagnetVia(p.Addr()))
	c.sequential(0, 2<<20, 32<<10)
	const outage = time.Second
	if n := p.Vanish(outage); n != 1 {
		t.Fatalf("Vanish cut %d connections, want 1", n)
	}
	go func() {
		time.Sleep(outage + 200*time.Millisecond)
		c.torrent.AddPeers([]atorrent.PeerInfo{{Addr: atorrent.StringAddr(p.Addr()), Source: atorrent.PeerSourceTracker}})
	}()
	c.sequential(2<<20, c.f.Size, 32<<10)
	if st := p.Stats(); st.Accepted != 2 || st.Dropped != 1 {
		t.Errorf("proxy stats %+v: want the first connection dropped and a second one", st)
	}
}
