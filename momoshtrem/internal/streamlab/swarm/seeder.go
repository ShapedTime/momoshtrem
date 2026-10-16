package swarm

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// minUploadBurst is one BitTorrent chunk: anacrolix panics when the upload
// limiter's burst can't fit the chunk it's sending.
const minUploadBurst = 16 << 10

// SeederConfig configures a seeder.
type SeederConfig struct {
	Layout *media.Layout
	// Metainfo is the layout's torrent (media.BuildMetainfo or a Cache).
	Metainfo *metainfo.MetaInfo
	// UploadRate caps what the seeder sends, in bytes per second; 0 is
	// unlimited. The burst is a tenth of a second's worth, at least one
	// chunk.
	UploadRate int64
	// Name names the seeder's logs; "streamlab-seeder" when empty.
	Name string
}

// Seeder is an anacrolix client seeding one generated torrent on loopback.
// It accepts connections and never dials: leechers find it through the
// x.pe address in Magnet.
type Seeder struct {
	client *atorrent.Client
	t      *atorrent.Torrent
	addr   string
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
	cc.DefaultStorage = NewStorage(cfg.Layout)
	cc.UploadRateLimiter = uploadLimiter(cfg.UploadRate)

	client, err := atorrent.NewClient(cc)
	if err != nil {
		return nil, fmt.Errorf("swarm: starting the seeder's client: %w", err)
	}
	s := &Seeder{client: client}
	s.t, err = client.AddTorrent(cfg.Metainfo)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("swarm: adding the seeder's torrent: %w", err)
	}
	<-s.t.GotInfo() // the metainfo carries it
	if done, size := s.t.BytesCompleted(), cfg.Layout.Size(); done != size {
		s.Close()
		return nil, fmt.Errorf("swarm: the seeder has %d of %d bytes", done, size)
	}
	port := client.LocalPort()
	if port == 0 {
		s.Close()
		return nil, errors.New("swarm: the seeder isn't listening")
	}
	s.addr = net.JoinHostPort(loopbackHost, strconv.Itoa(port))
	return s, nil
}

// uploadLimiter is a limiter of bps bytes per second; unlimited for 0.
func uploadLimiter(bps int64) *rate.Limiter {
	if bps <= 0 {
		// The client gives an unlimited limiter its default burst.
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(rate.Limit(bps), max(minUploadBurst, int(bps/10)))
}

// Addr is the seeder's listen address, 127.0.0.1:port.
func (s *Seeder) Addr() string { return s.addr }

// InfoHash is the torrent's info hash, lowercase hex.
func (s *Seeder) InfoHash() string { return s.t.InfoHash().HexString() }

// Magnet is the torrent's magnet with the seeder's address as x.pe (BEP 9).
// anacrolix adds x.pe addresses as peers, so a leecher that adds the
// magnet finds the seeder through the same path as any magnet.
func (s *Seeder) Magnet() string {
	return metainfo.Magnet{
		InfoHash:    s.t.InfoHash(),
		DisplayName: s.t.Name(),
		Params:      url.Values{"x.pe": {s.addr}},
	}.String()
}

// Stats is the seeder's torrent stats: BytesWrittenData is what it sent.
func (s *Seeder) Stats() atorrent.TorrentStats { return s.t.Stats() }

// WriteStatus writes anacrolix's status of the seeder's client: its
// connections, their requests and what it has sent.
func (s *Seeder) WriteStatus(w io.Writer) { s.client.WriteStatus(w) }

// Close stops the seeder, dropping its connections.
func (s *Seeder) Close() error {
	return errors.Join(s.client.Close()...)
}
