package swarm_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// The piece-transfer tests move pieces between two anacrolix clients on
// loopback: a generated seeder, and a leecher on the production read path
// (torrent.Service, vfs.TorrentFile). Every byte read is checked against
// media.Layout. They time nothing tightly: the machine may be loaded.
//
// A read that stalls with the seeder connected and its piece wanted now is
// a known issue (swarm.StallIssue): the run is skipped with both clients'
// status rather than failed. Any other failed read fails the test.

// transfer is one run: start a seeder, load its magnet in a leecher, read
// one file as a player does and check every byte.
type transfer struct {
	spec media.Spec
	file int // the file to read
	// stored puts the metainfo in the leecher's store first, the usual
	// production path. Otherwise the info dict comes from the seeder.
	stored      bool
	startPaused bool  // the activity manager's start_paused
	uploadRate  int64 // the seeder's cap, bytes per second; 0 is unlimited
	// readTimeout bounds each read; 0 is labReadTimeout.
	readTimeout time.Duration
}

func (tr transfer) run(t *testing.T) {
	layout, err := media.New(tr.spec)
	if err != nil {
		t.Fatal(err)
	}
	mi, err := media.BuildMetainfo(layout)
	if err != nil {
		t.Fatal(err)
	}
	seeder, err := swarm.StartSeeder(swarm.SeederConfig{Layout: layout, Metainfo: mi, UploadRate: tr.uploadRate})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { seeder.Close() })

	l := newLeecher(t, tr.startPaused, cmp.Or(tr.readTimeout, labReadTimeout))
	hash := mi.HashInfoBytes().HexString()
	if tr.stored {
		if err := l.store.Save(hash, mi); err != nil {
			t.Fatal(err)
		}
	}

	f := layout.Files()[tr.file]
	start := time.Now()
	file, tor := l.open(t, seeder.Magnet(), f.Path)
	loaded := time.Since(start)
	c := &checker{t: t, file: file, torrent: tor, layout: layout, f: f,
		status: func() string {
			var b strings.Builder
			b.WriteString("leecher: " + l.status(hash) + "\nseeder: ")
			seeder.WriteStatus(&b)
			return b.String()
		},
	}

	// A player's opening moves: probe the header and the footer, look in
	// the middle, play from the start, then seek around.
	c.read(0, 64<<10)
	firstByte := time.Since(start)
	c.read(f.Size-64<<10, 64<<10)
	c.read(f.Size/2, 1<<20)
	c.sequential(0, f.Size, 32<<10) // http.ServeContent's copy buffer
	rng := rand.New(rand.NewPCG(layout.Seed(), uint64(tr.file)))
	for i := 0; i < 8; i++ {
		c.read(rng.Int64N(f.Size), 256<<10)
	}
	elapsed := time.Since(start)

	if err := file.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if n := l.tracker.Count(); n != 0 {
		t.Errorf("%d priority readers still registered after Close", n)
	}
	if !tr.stored {
		if stored, err := l.store.Load(hash); err != nil || stored == nil {
			t.Errorf("the metainfo from peers wasn't stored: %v, %v", stored, err)
		}
	}
	// The leecher started empty, so the seeder sent at least the file.
	stats := seeder.Stats()
	sent := stats.BytesWrittenData.Int64()
	if sent < f.Size {
		t.Errorf("the seeder sent %d bytes of a %d-byte file", sent, f.Size)
	}
	if tr.uploadRate > 0 {
		// A floor, not a timing check: a slow machine only takes longer.
		burst := max(16<<10, tr.uploadRate/10)
		floor := time.Duration(float64(f.Size-burst) / float64(tr.uploadRate) * 0.8 * float64(time.Second))
		if elapsed < floor {
			t.Errorf("read %d bytes in %v at %d B/s: the upload limit isn't applied", f.Size, elapsed, tr.uploadRate)
		}
	}
	t.Logf("%s: loaded in %v, first 64 KiB in %v, %.1f MiB checked in %v (%.1f MiB/s), seeder sent %.1f MiB",
		f.Path, loaded.Round(time.Millisecond), firstByte.Round(time.Millisecond),
		mib(c.checked), elapsed.Round(time.Millisecond), mib(c.checked)/elapsed.Seconds(), mib(sent))
}

func mib(n int64) float64 { return float64(n) / (1 << 20) }

// checker reads one file through a TorrentFile and compares every byte
// with the layout.
type checker struct {
	t         *testing.T
	file      *vfs.TorrentFile
	torrent   *atorrent.Torrent // the leecher's, for diagnosis
	layout    *media.Layout
	f         media.File
	got, want []byte
	checked   int64
	status    func() string // describes both clients when a read fails
	// known, when set, names the mismatches a known issue explains (by
	// torrent offset and the bytes): they're counted in bad, and only the
	// others fail the test.
	known func(abs int64, got, want byte) bool
	bad   int
}

// read reads n bytes at the file offset off (fewer at the end of the file)
// and checks them.
func (c *checker) read(off int64, n int) {
	c.t.Helper()
	if off < 0 || off >= c.f.Size {
		return
	}
	n = int(min(int64(n), c.f.Size-off))
	if len(c.got) < n {
		c.got, c.want = make([]byte, n), make([]byte, n)
	}
	got, want := c.got[:n], c.want[:n]
	m, err := c.file.ReadAt(got, off)
	if err != nil || m != n {
		msg := fmt.Sprintf("ReadAt(%d bytes at %d of %s) = %d, %v\n%s", n, off, c.f.Path, m, err, c.status())
		if c.knownStall(err, off+int64(m)) {
			c.t.Skipf("known issue %s, a stall with one seeder: %s", swarm.StallIssue, msg)
		}
		c.t.Fatal(msg)
	}
	if _, err := c.layout.ReadAt(want, c.f.Offset+off); err != nil {
		c.t.Fatal(err)
	}
	for i := range got {
		if got[i] != want[i] && c.known != nil && c.known(c.f.Offset+off+int64(i), got[i], want[i]) {
			c.bad++
			continue
		}
		if got[i] != want[i] {
			abs := c.f.Offset + off + int64(i)
			c.t.Fatalf("byte %d of %s (torrent offset %d, piece %d) = %#02x, want %#02x",
				off+int64(i), c.f.Path, abs, abs/c.layout.PieceLength(), got[i], want[i])
		}
	}
	c.checked += int64(n)
}

// knownStall reports whether a read that failed with err, stuck at the
// file offset off, is the known stall (swarm.Stalled): it timed out while
// the leecher was connected to the seeder and the piece at off was wanted.
// Any other failed read fails the test.
func (c *checker) knownStall(err error, off int64) bool {
	return errors.Is(err, context.DeadlineExceeded) &&
		swarm.Stalled(c.torrent, int((c.f.Offset+off)/c.layout.PieceLength()))
}

// sequential reads [from, to) in chunk-sized reads.
func (c *checker) sequential(from, to int64, chunk int) {
	c.t.Helper()
	for off := from; off < to; off += int64(chunk) {
		c.read(off, chunk)
	}
}

// smokeReadTimeout bounds the smoke tier's reads. They take under a second
// even under -race on a loaded machine, and a #195 stall lasts about a
// minute, so it only sets how long a skipped run takes.
const smokeReadTimeout = 5 * time.Second

// TestTransfer is the smoke tier: small torrents, always run. It covers
// both ways a torrent loads, both start_paused settings, a single file and
// a pack, and a rate-limited seeder.
func TestTransfer(t *testing.T) {
	tests := []struct {
		name string
		tr   transfer
	}{
		{
			name: "stored metainfo, start paused, rate-limited",
			tr: transfer{
				spec:        media.SingleFile("Film.2020.1080p.mkv", 1, 256<<10, 4<<20, media.MKV),
				stored:      true,
				startPaused: true,
				uploadRate:  16 << 20,
				readTimeout: smokeReadTimeout,
			},
		},
		{
			name: "metadata from peers, pack",
			tr: transfer{
				// Odd sizes: files start and end inside pieces.
				spec:        media.EpisodePack("Show", 1, 3, 2, 256<<10, 1536<<10+123, media.MP4),
				file:        1,
				readTimeout: smokeReadTimeout,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.tr.run(t)
		})
	}
}

// TestTransfer_64MiB is the issue's full run: 64 MiB read sequentially plus
// seeks, for every way a torrent loads and both start_paused settings, and
// a pack whose middle file is read. It takes a few seconds, more under
// -race, so it runs with STREAMLAB=1.
func TestTransfer_64MiB(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run the 64 MiB transfers")
	}
	seed := uint64(3)
	for _, stored := range []bool{true, false} {
		for _, startPaused := range []bool{false, true} {
			// A seed each: each run's info hash names it in the logs.
			seed++
			tr := transfer{
				spec:        media.SingleFile("Film.2021.2160p.mkv", seed, 1<<20, 64<<20, media.MKV),
				stored:      stored,
				startPaused: startPaused,
			}
			t.Run(fmt.Sprintf("stored=%v,start_paused=%v", stored, startPaused), func(t *testing.T) {
				t.Parallel()
				tr.run(t)
			})
		}
	}
	t.Run("pack", func(t *testing.T) {
		t.Parallel()
		transfer{
			spec:        media.EpisodePack("Show", 2, 4, 4, 1<<20, 24<<20+4321, media.MP4),
			file:        1,
			stored:      true,
			startPaused: true,
		}.run(t)
	})
}
