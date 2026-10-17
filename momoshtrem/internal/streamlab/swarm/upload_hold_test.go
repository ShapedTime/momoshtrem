package swarm

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
)

// A held seeder sends nothing: a read of a piece the leecher lacks waits.
// Released, it sends again and the read completes. (A lost writer wakeup,
// #195, can delay the release to the 1-minute keep-alive; the deadline
// allows for it.) The leecher connects first: a complete seeder that may
// not upload wants no connection, so one held from the start refuses the
// leecher's dial.
func TestSeeder_HoldUploads(t *testing.T) {
	l, mi, _ := buildPack(t)
	s, err := StartSeeder(SeederConfig{Layout: l, Metainfo: mi})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

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
	spec.InfoBytes = mi.InfoBytes
	tor, _, err := cl.AddTorrentSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	<-tor.GotInfo()
	tor.Piece(0).SetPriority(1) // connect: download the first piece
	connected, cancel0 := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel0()
	select {
	case <-tor.Complete().On():
	case <-pieceDone(tor, 0):
	case <-connected.Done():
		t.Fatal("the first piece never came")
	}

	s.HoldUploads()
	last := int64(tor.NumPieces()-1) * testPiece
	r := tor.NewReader()
	defer r.Close()
	r.SetResponsive()
	if _, err := r.Seek(last, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	before := tor.Stats().BytesReadData
	held, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	buf := make([]byte, 1024)
	if n, err := r.ReadContext(held, buf); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a read from a held seeder = %d, %v; want it to wait", n, err)
	}
	if after := tor.Stats().BytesReadData; after.Int64() > before.Int64()+16<<10 {
		t.Errorf("the held seeder sent %d bytes", after.Int64()-before.Int64())
	}

	s.ReleaseUploads()
	released, cancel2 := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel2()
	if _, err := r.Seek(last, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(readerCtx{r, released}, buf); err != nil {
		t.Fatalf("a read after the release: %v", err)
	}
	want := make([]byte, len(buf))
	l.ReadAt(want, last)
	if string(buf) != string(want) {
		t.Error("the bytes after the release don't match the layout")
	}
}

// pieceDone is closed once piece i is complete.
func pieceDone(tor *atorrent.Torrent, i int) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !tor.PieceState(i).Complete {
			time.Sleep(10 * time.Millisecond)
		}
	}()
	return done
}

type readerCtx struct {
	r   atorrent.Reader
	ctx context.Context
}

func (rc readerCtx) Read(p []byte) (int, error) { return rc.r.ReadContext(rc.ctx, p) }
