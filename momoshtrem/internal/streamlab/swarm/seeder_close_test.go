package swarm

import (
	"context"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
)

// Closing a paced seeder while peer requests wait on its pace doesn't
// crash (#237). anacrolix v1.60.0 reads a peer request's chunk with the
// client unlocked; if the read fails and the torrent is closed by the time
// it relocks, the request is left unread and servePeerRequest panics. Each
// rep connects leechers, lets the seeder spend its burst so every
// connection's next read waits on the pace, and closes the seeder: the
// leechers lose it, and nothing panics.
func TestSeeder_CloseWithPeerRequestsInFlight(t *testing.T) {
	l, mi, _ := buildPack(t)
	for range 8 {
		// 128 KiB/s: a 16 KiB burst, then a chunk every 125 ms.
		closeMidPace(t, SeederConfig{Layout: l, Metainfo: mi, UploadRate: 128 << 10})
	}
}

// closeMidPace starts a seeder, connects leechers that want every piece,
// and closes the seeder once it's sending: the connections' next reads
// wait on the pace. The leechers must then lose it.
func closeMidPace(t *testing.T, cfg SeederConfig) {
	t.Helper()
	const leechers = 3
	s, err := StartSeeder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			s.Close()
		}
	}()
	tors := make([]*atorrent.Torrent, leechers)
	for i := range tors {
		cc := atorrent.NewDefaultClientConfig()
		Loopback(cc, "test-leecher")
		cc.DisableIPv6 = true
		cc.DataDir = t.TempDir()
		cl, err := atorrent.NewClient(cc)
		if err != nil {
			t.Fatal(err)
		}
		defer cl.Close()
		spec, err := atorrent.TorrentSpecFromMagnetUri(s.Magnet())
		if err != nil {
			t.Fatal(err)
		}
		spec.InfoBytes = cfg.Metainfo.InfoBytes
		tor, _, err := cl.AddTorrentSpec(spec)
		if err != nil {
			t.Fatal(err)
		}
		<-tor.GotInfo()
		tor.DownloadAll()
		tors[i] = tor
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	waitFor(ctx, t, "the leechers to connect and the seeder to send", func() bool {
		st := s.Stats()
		return st.ActivePeers == leechers && st.BytesWrittenData.Int64() >= ChunkSize
	})
	// The leechers' requests are in: every connection's server waits on the
	// pace (a chunk every 125 ms, shared).
	time.Sleep(20 * time.Millisecond)
	closed = true
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// The seeder is gone: each leecher loses it (a panic in the seeder's
	// peer-request servers fires about now).
	waitFor(ctx, t, "the leechers to lose the seeder", func() bool {
		for _, tor := range tors {
			if tor.Stats().ActivePeers != 0 {
				return false
			}
		}
		return true
	})
}

// waitFor polls cond until it holds, failing the test at ctx's end.
func waitFor(ctx context.Context, t *testing.T, what string, cond func() bool) {
	t.Helper()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
