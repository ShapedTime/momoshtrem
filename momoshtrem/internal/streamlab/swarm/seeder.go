package swarm

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// SeederConfig configures a seeder.
type SeederConfig struct {
	Layout *media.Layout
	// Metainfo is the layout's torrent (media.BuildMetainfo or a Cache).
	Metainfo *metainfo.MetaInfo
	// UploadRate caps what the seeder sends, in bytes per second; 0 is
	// unlimited. It paces the storage's reads (StorageOptions.Rate), not
	// anacrolix's UploadRateLimiter: the limiter's retry timer is one of
	// #195's lost writer wakeups (signature D), a lab artifact for
	// downloads.
	UploadRate int64
	// Corrupt are pieces the seeder serves wrong, on the first
	// CorruptServes serves of each chunk (every serve when 0), holding the
	// last chunk of the first serve for CorruptHold: the leecher's hash
	// check fails them (StorageOptions).
	Corrupt       []int
	CorruptServes int
	CorruptHold   time.Duration
	// Missing are pieces the seeder doesn't have: a partial seeder.
	Missing []int
	// Name names the seeder's logs; "streamlab-seeder" when empty.
	Name string
}

// Seeder is an anacrolix client seeding one generated torrent on loopback.
// It accepts connections and never dials: leechers find it through the
// x.pe address in Magnet.
type Seeder struct {
	client  *atorrent.Client
	t       *atorrent.Torrent
	storage *Storage
	addr    string
}

// StartSeeder starts a seeder listening on 127.0.0.1 on a free port.
func StartSeeder(cfg SeederConfig) (*Seeder, error) {
	if cfg.Layout == nil || cfg.Metainfo == nil {
		return nil, errors.New("swarm: a seeder needs a layout and its metainfo")
	}
	name := cfg.Name
	if name == "" {
		name = "streamlab-seeder"
	}
	cc := atorrent.NewDefaultClientConfig()
	Loopback(cc, name)
	cc.Seed = true
	cc.DialForPeerConns = false
	cc.DisablePEX = true
	cc.DisableIPv6 = true
	st := NewStorageWith(cfg.Layout, StorageOptions{Rate: cfg.UploadRate, Corrupt: cfg.Corrupt,
		CorruptServes: cfg.CorruptServes, CorruptHold: cfg.CorruptHold, Missing: cfg.Missing})
	cc.DefaultStorage = st

	client, err := atorrent.NewClient(cc)
	if err != nil {
		st.Stop()
		return nil, fmt.Errorf("swarm: starting the seeder's client: %w", err)
	}
	s := &Seeder{client: client, storage: st}
	s.t, err = client.AddTorrent(cfg.Metainfo)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("swarm: adding the seeder's torrent: %w", err)
	}
	<-s.t.GotInfo() // the metainfo carries it
	want := cfg.Layout.Size()
	for i := range cfg.Layout.NumPieces() {
		if !st.Has(i) {
			want -= s.t.Piece(i).Info().Length()
		}
	}
	if done := s.t.BytesCompleted(); done != want {
		s.Close()
		return nil, fmt.Errorf("swarm: the seeder has %d of its %d bytes", done, want)
	}
	port := client.LocalPort()
	if port == 0 {
		s.Close()
		return nil, errors.New("swarm: the seeder isn't listening")
	}
	s.addr = net.JoinHostPort(loopbackHost, strconv.Itoa(port))
	return s, nil
}

// Addr is the seeder's listen address, 127.0.0.1:port.
func (s *Seeder) Addr() string { return s.addr }

// InfoHash is the torrent's info hash, lowercase hex.
func (s *Seeder) InfoHash() string { return s.t.InfoHash().HexString() }

// Magnet is the torrent's magnet with the seeder's address as x.pe (BEP 9).
// anacrolix adds x.pe addresses as peers, so a leecher that adds the
// magnet finds the seeder through the same path as any magnet.
func (s *Seeder) Magnet() string { return s.MagnetVia(s.addr) }

// MagnetVia is the torrent's magnet with peers as its x.pe addresses: a
// proxy's in place of the seeder's, or several seeders'. With none, the
// leecher finds no peer until one is added (Torrent.AddPeers), as a
// tracker or the DHT would.
func (s *Seeder) MagnetVia(peers ...string) string {
	m := metainfo.Magnet{InfoHash: s.t.InfoHash(), DisplayName: s.t.Name()}
	if len(peers) > 0 {
		m.Params = url.Values{"x.pe": peers}
	}
	return m.String()
}

// BadServes is how many times the seeder served a corrupt piece's flipped
// byte (SeederConfig.Corrupt).
func (s *Seeder) BadServes() int { return s.storage.BadServes() }

// Stats is the seeder's torrent stats: BytesWrittenData is what it sent.
func (s *Seeder) Stats() atorrent.TorrentStats { return s.t.Stats() }

// WriteStatus writes anacrolix's status of the seeder's client: its
// connections, their requests and what it has sent.
func (s *Seeder) WriteStatus(w io.Writer) { s.client.WriteStatus(w) }

// Close stops the seeder, dropping its connections.
func (s *Seeder) Close() error {
	s.storage.Stop()
	return errors.Join(s.client.Close()...)
}

// aliasHost is the lab's second loopback identity: connecting to 0.0.0.0
// connects to the local host, on Linux and macOS alike, and the dialer
// sees the peer as 0.0.0.0.
const aliasHost = "0.0.0.0"

// Alias is a loopback address (127.0.0.1:port) under the lab's second
// identity, 0.0.0.0:port, for a leecher to dial: the connection reaches
// 127.0.0.1:port, but anacrolix sees the peer's IP as 0.0.0.0.
//
// anacrolix bans peers by IP: a piece that fails its hash bans its one
// untrusted contributor, and once the piece passes, smart ban bans every
// IP that sent a block of it that differed. Every lab peer is 127.0.0.1,
// so one ban would drop the whole lab swarm and refuse it for good, where
// production drops the one bad peer. A seeder that sends bad data is
// therefore dialled by its alias: its ban drops it alone. (A name, such as
// localhost, doesn't do: anacrolix refuses a peer address without an IP.)
func Alias(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host != loopbackHost {
		return addr
	}
	return net.JoinHostPort(aliasHost, port)
}
