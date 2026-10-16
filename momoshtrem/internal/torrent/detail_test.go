package torrent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/anacrolix/torrent/types"
)

func run(n int, complete, partial bool, pri types.PiecePriority) torrent.PieceStateRun {
	return torrent.PieceStateRun{
		PieceState: torrent.PieceState{Completion: storage.Completion{Ok: true, Complete: complete}, Partial: partial, Priority: pri},
		Length:     n,
	}
}

func TestPieceRuns(t *testing.T) {
	tests := []struct {
		name string
		runs torrent.PieceStateRuns
		want []PieceRun
	}{
		{"none", nil, []PieceRun{}},
		{"all complete", torrent.PieceStateRuns{run(40, true, false, types.PiecePriorityNone)}, []PieceRun{{PieceHave, 40}}},
		{
			"a stream: have, the reader's piece arriving, the readahead window, then nothing wanted",
			torrent.PieceStateRuns{
				run(10, true, false, types.PiecePriorityNone),
				run(1, false, true, types.PiecePriorityNow),
				run(2, false, false, types.PiecePriorityNext),
				run(3, false, false, types.PiecePriorityReadahead),
				run(4, false, false, types.PiecePriorityHigh),
				run(5, false, false, types.PiecePriorityNormal),
				run(6, false, false, types.PiecePriorityNone),
			},
			// High and above is priority; Normal and None are missing, merged.
			[]PieceRun{{PieceHave, 10}, {PiecePartial, 1}, {PiecePriority, 9}, {PieceMissing, 11}},
		},
		{
			"complete wins over a priority; runs merge across priorities",
			torrent.PieceStateRuns{
				run(3, true, false, types.PiecePriorityNow),
				run(2, true, false, types.PiecePriorityNone),
				run(1, false, true, types.PiecePriorityNone),
				run(0, false, false, types.PiecePriorityNone), // empty runs are skipped
				run(1, false, true, types.PiecePriorityHigh),
			},
			[]PieceRun{{PieceHave, 5}, {PiecePartial, 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PieceRuns(tt.runs); !slices.Equal(got, tt.want) {
				t.Errorf("PieceRuns = %v, want %v", got, tt.want)
			}
		})
	}
}

// The stored metainfo of a multi-file torrent reads as the client names
// its files, without loading it.
func TestMetainfoStore_Summary(t *testing.T) {
	info := metainfo.Info{
		Name:        "Frieren.S01",
		PieceLength: 1 << 18,
		Pieces:      make([]byte, 20*3),
		Files: []metainfo.FileInfo{
			{Path: []string{"Frieren - 01.mkv"}, Length: 500_000},
			{Path: []string{"Extras", "NCOP.mkv"}, Length: 100_000},
		},
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes, AnnounceList: [][]string{
		{"udp://tracker.opentrackr.org:1337/announce"}, {"udp://open.stealth.si:80/announce", "udp://tracker.opentrackr.org:1337/announce"}}}
	hash := mi.HashInfoBytes().HexString()
	store := NewMetainfoStore(filepath.Join(t.TempDir(), "metainfo"))
	if err := store.Save(hash, mi); err != nil {
		t.Fatal(err)
	}

	got, err := store.Summary(hash)
	if err != nil || got == nil {
		t.Fatalf("Summary = %+v, %v", got, err)
	}
	if got.Name != "Frieren.S01" || got.TotalSize != 600_000 || got.PieceLength != 1<<18 || got.PiecesTotal != 3 {
		t.Errorf("summary = %+v", got)
	}
	wantFiles := []FileEntry{{Path: "Frieren.S01/Frieren - 01.mkv", Size: 500_000}, {Path: "Frieren.S01/Extras/NCOP.mkv", Size: 100_000}}
	if !slices.Equal(got.Files, wantFiles) {
		t.Errorf("files = %v, want %v", got.Files, wantFiles)
	}
	if want := []string{"udp://tracker.opentrackr.org:1337/announce", "udp://open.stealth.si:80/announce"}; !slices.Equal(got.Trackers, want) {
		t.Errorf("trackers = %v, want %v (distinct, in order)", got.Trackers, want)
	}

	// Nothing stored, and a corrupt file.
	other := "0123456789abcdef0123456789abcdef01234567"
	if got, err := store.Summary(other); got != nil || err != nil {
		t.Errorf("Summary(nothing stored) = %+v, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(store.dir, other+".torrent"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Summary(other); err == nil {
		t.Error("Summary(corrupt) = nil error")
	}
}

// A loaded torrent's detail, over a real client: one piece of three in the
// cache. Nothing loads one that isn't.
func TestService_Detail(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	f := newFixture(t)
	f.mi.AnnounceList = [][]string{{"udp://tracker.opentrackr.org:1337/announce"}}
	if err := e.store.Save(f.hash, f.mi); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Detail(f.hash); !errors.Is(err, ErrTorrentNotFound) {
		t.Fatalf("Detail before it loads = %v, want ErrTorrentNotFound", err)
	}
	if e.svc.loaded(f.hash) != nil {
		t.Fatal("Detail loaded the torrent")
	}

	ts, err := storage.NewClient(e.pieces).OpenTorrent(context.Background(), &f.info, f.mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	p := f.info.Piece(0)
	if _, err := ts.Piece(p).WriteAt(f.data[:p.Length()], 0); err != nil {
		t.Fatal(err)
	}
	if err := ts.Piece(p).MarkComplete(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddTorrent(f.magnet()); err != nil {
		t.Fatal(err)
	}

	d, err := e.svc.Detail(strings.ToUpper(f.hash))
	if err != nil {
		t.Fatal(err)
	}
	if !d.HasInfo || d.Name != "Film.2020.1080p.mkv" || d.TotalSize != int64(len(f.data)) || d.PieceLength != testPieceLength ||
		d.PiecesTotal != 3 || d.PiecesHave != 1 {
		t.Errorf("detail = %+v", d)
	}
	if want := []PieceRun{{PieceHave, 1}, {PieceMissing, 2}}; !slices.Equal(d.Pieces, want) {
		t.Errorf("pieces = %v, want %v (AddTorrent wants none)", d.Pieces, want)
	}
	if len(d.Files) != 1 || d.Files[0].Path != "Film.2020.1080p.mkv" || d.Files[0].Size != int64(len(f.data)) ||
		d.Files[0].BytesCompleted != testPieceLength {
		t.Errorf("files = %+v", d.Files)
	}
	if !slices.Equal(d.Trackers, []string{"udp://tracker.opentrackr.org:1337/announce"}) {
		t.Errorf("trackers = %v", d.Trackers)
	}
	if !strings.Contains(d.Magnet, "xt=urn:btih:"+f.hash) || d.Peers != 0 || d.DownloadedBytes != 0 {
		t.Errorf("magnet %q, peers %d, downloaded %d", d.Magnet, d.Peers, d.DownloadedBytes)
	}
}
