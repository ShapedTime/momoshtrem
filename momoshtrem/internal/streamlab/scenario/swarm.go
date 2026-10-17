package scenario

import (
	"errors"
	"fmt"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/streamlab/cdn"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
)

// Peer is one seeder of a title's torrent, and how the leecher reaches it.
type Peer struct {
	Seeder *swarm.Seeder
	// Proxy is the TCP proxy in front of it; nil when the leecher dials
	// the seeder.
	Proxy *swarm.Proxy
	// Addr is what the leecher dials: the seeder's address, its proxy's,
	// or its alias.
	Addr string
	Spec SeederSpec
}

// torrentSwarm is one torrent of the media and its seeders.
type torrentSwarm struct {
	layout *media.Layout
	mi     *metainfo.MetaInfo
	hash   string
	magnet string
	peers  []*Peer
}

// swarmSet is the media's torrents and their seeders, and the fake CDN
// when the media has HDRezka movies.
type swarmSet struct {
	torrents []*torrentSwarm
	cdn      *cdn.CDN
	links    *cdn.Links
	remote   []*remoteMedia
	closers  []func()
}

// remoteMedia is an HDRezka movie's content on the CDN.
type remoteMedia struct {
	layout *media.Layout
	file   *cdn.File
}

// close stops the seeders, proxies and CDN, last started first.
func (ss *swarmSet) close() {
	for i := len(ss.closers) - 1; i >= 0; i-- {
		ss.closers[i]()
	}
	ss.closers = nil
}

// startSwarm starts each torrent's seeders (sw.Seeders, or one at
// SeederRate), their proxies, and the CDN for the media's HDRezka movies.
// With discovery set, the magnets carry no x.pe: discover adds the peers.
// On an error, what started is stopped.
func startSwarm(m Media, sw Swarm, discovery time.Duration, seed uint64, cache *media.Cache) (_ *swarmSet, err error) {
	ss := &swarmSet{}
	defer func() {
		if err != nil {
			ss.close()
		}
	}()
	specs := sw.Seeders
	if len(specs) == 0 {
		specs = []SeederSpec{{Rate: sw.SeederRate}}
	}
	for i, spec := range m.Torrents {
		l, err := media.New(spec)
		if err != nil {
			return nil, toolErr("torrent %d: %w", i, err)
		}
		mi, _, err := cache.Metainfo(l)
		if err != nil {
			return nil, toolErr("torrent %d's metainfo: %w", i, err)
		}
		ts := &torrentSwarm{layout: l, mi: mi, hash: mi.HashInfoBytes().HexString()}
		var addrs []string
		for j, ps := range specs {
			name := fmt.Sprintf("streamlab-seeder-%d", i)
			if len(specs) > 1 {
				name = fmt.Sprintf("streamlab-seeder-%d.%d", i, j)
			}
			s, err := swarm.StartSeeder(swarm.SeederConfig{Layout: l, Metainfo: mi, UploadRate: ps.Rate, Name: name,
				Corrupt: ps.Corrupt, CorruptServes: ps.CorruptServes, CorruptHold: ps.CorruptHold, Missing: ps.Missing})
			if err != nil {
				return nil, toolErr("seeder %s: %w", name, err)
			}
			ss.closers = append(ss.closers, func() { s.Close() })
			peer := &Peer{Seeder: s, Addr: s.Addr(), Spec: ps}
			if ps.Proxy != nil {
				cfg := *ps.Proxy
				cfg.Seed = seed + uint64(i*len(specs)+j)
				px, err := swarm.StartProxy(s.Addr(), cfg)
				if err != nil {
					return nil, toolErr("proxy of %s: %w", name, err)
				}
				ss.closers = append(ss.closers, func() { px.Close() })
				peer.Proxy, peer.Addr = px, px.Addr()
			}
			if ps.Alias {
				peer.Addr = swarm.Alias(peer.Addr)
			}
			ts.peers = append(ts.peers, peer)
			addrs = append(addrs, peer.Addr)
		}
		if discovery > 0 {
			addrs = nil
		}
		ts.magnet = ts.peers[0].Seeder.MagnetVia(addrs...)
		ss.torrents = append(ss.torrents, ts)
	}
	if len(m.Remote) > 0 {
		ss.cdn = cdn.Start()
		ss.closers = append(ss.closers, ss.cdn.Close)
		ss.links = cdn.NewLinks(ss.cdn)
		for i, ref := range m.Remote {
			l, err := media.New(ref.Spec)
			if err != nil {
				return nil, toolErr("HDRezka movie %d: %w", i, err)
			}
			f := service.StreamFile(ref.Stream)
			ss.remote = append(ss.remote, &remoteMedia{layout: l, file: ss.cdn.Add(l, 0, f.Key, f.Quality)})
		}
	}
	return ss, nil
}

// infos is the torrents' metainfo, in order.
func (ss *swarmSet) infos() []*metainfo.MetaInfo {
	out := make([]*metainfo.MetaInfo, len(ss.torrents))
	for i, ts := range ss.torrents {
		out[i] = ts.mi
	}
	return out
}

// discover adds each torrent's peers to the leecher's torrent delay after
// the leecher adds it, as a tracker's answer (or the DHT) would: untrusted
// peers from a tracker. It returns at once; the adds stop with stop.
func (ss *swarmSet) discover(s *stack.Stack, delay time.Duration, stop <-chan struct{}) {
	for _, ts := range ss.torrents {
		go func() {
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			var tor *atorrent.Torrent
			for tor == nil {
				select {
				case <-stop:
					return
				case <-tick.C:
					tor, _ = s.Torrent(ts.hash)
				}
			}
			t := time.NewTimer(delay)
			defer t.Stop()
			select {
			case <-stop:
				return
			case <-t.C:
			}
			tor.AddPeers(trackerPeers(ts.peers...))
		}()
	}
}

// trackerPeers are peers as a tracker gives them: untrusted.
func trackerPeers(peers ...*Peer) []atorrent.PeerInfo {
	out := make([]atorrent.PeerInfo, len(peers))
	for i, p := range peers {
		out[i] = atorrent.PeerInfo{Addr: atorrent.StringAddr(p.Addr), Source: atorrent.PeerSourceTracker}
	}
	return out
}

// AddPeers adds peers of t's torrent to the leecher's torrent, as a
// tracker's answer would: untrusted, from a tracker. It's how a seeder that
// vanished is found again. It returns how many the leecher took.
func (e *Env) AddPeers(t *Title, peers ...*Peer) (int, error) {
	tor, ok := e.Torrent(t)
	if !ok {
		return 0, errors.New("the torrent isn't loaded")
	}
	return tor.AddPeers(trackerPeers(peers...)), nil
}

// remoteLibrary adds the media's HDRezka movies to the stack's library
// and returns their titles.
func (ss *swarmSet) remoteLibrary(m Media, so *stack.Options) []*Title {
	var titles []*Title
	for i, ref := range m.Remote {
		rm := ss.remote[i]
		stream := ref.Stream
		mv := stack.Movie{Title: ref.Title, Year: ref.Year, Source: stack.Source{Layout: rm.layout, Stream: &stream}}
		so.Movies = append(so.Movies, mv)
		f := rm.layout.Files()[0]
		titles = append(titles, &Title{Name: fmt.Sprintf("%s (%d)", ref.Title, ref.Year), Path: mv.Path(), Dir: mv.Dir(),
			Layout: rm.layout, Offset: f.Offset, Size: f.Size, Integrity: newIntegrity(rm.layout), Remote: rm.file})
	}
	return titles
}
