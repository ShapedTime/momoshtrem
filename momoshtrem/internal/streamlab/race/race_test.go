package race_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/race"
)

// closeRace is a report of the kind check finds: a read goroutine that
// outlived its closed file, against the close.
const closeRace = `==================
WARNING: DATA RACE
Write at 0x00c000a1c0f8 by goroutine 214:
  github.com/anacrolix/torrent.(*reader).initStorageReader()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:441 +0x88
  github.com/anacrolix/torrent.(*reader).readOnceAt()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:275 +0x1d0
  github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Read()
      /src/momoshtrem/internal/streaming/reader.go:131 +0x1c4
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1()
      /src/momoshtrem/internal/vfs/torrent_file.go:474 +0x6c

Previous write at 0x00c000a1c0f8 by goroutine 198:
  github.com/anacrolix/torrent.(*reader).clearStorageReader()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:452 +0x7c
  github.com/anacrolix/torrent.(*reader).Close()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:388 +0xd8
  github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Close()
      /src/momoshtrem/internal/streaming/reader.go:210 +0x2a0
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).Close()
      /src/momoshtrem/internal/vfs/torrent_file.go:268 +0x10c
  github.com/shapedtime/momoshtrem/internal/vfs.(*sessionFile).Close()
      /src/momoshtrem/internal/vfs/session_file.go:94 +0x5c
  github.com/shapedtime/momoshtrem/internal/webdav.(*webdavFile).Close()
      /src/momoshtrem/internal/webdav/server.go:157 +0x48

Goroutine 214 (running) created at:
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext()
      /src/momoshtrem/internal/vfs/torrent_file.go:473 +0x2b8

Goroutine 198 (finished) created at:
  net/http.(*Server).Serve()
      /usr/local/go/src/net/http/server.go:3454 +0x7c8
==================
`

// The same race with the accesses the other way round, at other lines.
const closeRaceSwapped = `==================
WARNING: DATA RACE
Read at 0x00c000b2d0f8 by goroutine 77:
  github.com/anacrolix/torrent.(*reader).clearStorageReader()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:446 +0x7c
  github.com/anacrolix/torrent.(*reader).Close()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:388 +0xd8
  github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Close()
      /src/momoshtrem/internal/streaming/reader.go:212 +0x2a0
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).Close()
      /src/momoshtrem/internal/vfs/torrent_file.go:270 +0x10c
  github.com/shapedtime/momoshtrem/internal/vfs.(*sessionFile).Close()
      /src/momoshtrem/internal/vfs/session_file.go:94 +0x5c

Previous write at 0x00c000b2d0f8 by goroutine 80:
  github.com/anacrolix/torrent.(*reader).initStorageReader()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:441 +0x88
  github.com/anacrolix/torrent.(*reader).readOnceAt()
      /Users/x/go/pkg/mod/github.com/anacrolix/torrent@v1.60.0/reader.go:275 +0x1d0
  github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Read()
      /src/momoshtrem/internal/streaming/reader.go:131 +0x1c4
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1()
      /src/momoshtrem/internal/vfs/torrent_file.go:476 +0x6c

Goroutine 77 (running) created at:
  net/http.(*Server).Serve()
      /usr/local/go/src/net/http/server.go:3454 +0x7c8

Goroutine 80 (finished) created at:
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext()
      /src/momoshtrem/internal/vfs/torrent_file.go:475 +0x2b8
==================
`

// mapRace runs no momoshtrem code: its signature keeps the top frames.
const mapRace = `==================
WARNING: DATA RACE
Write at 0x00c0001a40f0 by goroutine 11:
  runtime.mapaccess2_fast64()
      /usr/local/go/src/internal/runtime/maps/runtime_fast64.go:85 +0x26c
  main.main.func4()
      /tmp/racesample/main.go:29 +0x4c

Previous write at 0x00c0001a40f0 by main goroutine:
  runtime.mapaccess2_fast64()
      /usr/local/go/src/internal/runtime/maps/runtime_fast64.go:85 +0x26c
  main.main()
      /tmp/racesample/main.go:28 +0x4c

Goroutine 11 (running) created at:
  main.main()
      /tmp/racesample/main.go:29 +0x2a4
==================
`

// swappedWrite is closeRaceSwapped with the close side a write, as in
// closeRace: the same race, caught from the other side.
var swappedWrite = strings.Replace(closeRaceSwapped, "Read at", "Write at", 1)

func TestParse(t *testing.T) {
	text := "some test output\n" + closeRace + "--- more output\n" + swappedWrite + mapRace + closeRace + "Found 4 data race(s)\n"
	races, err := race.Parse(strings.NewReader(text), "rep-1/race.123")
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 2 {
		for _, r := range races {
			t.Logf("%s", r.Signature)
		}
		t.Fatalf("got %d races, want 2: the swapped report is the same race", len(races))
	}
	r := races[0]
	const want = "write anacrolix/torrent.(*reader).clearStorageReader < internal/streaming.(*PriorityReader).Close < " +
		"internal/vfs.(*TorrentFile).Close < internal/vfs.(*sessionFile).Close | " +
		"write anacrolix/torrent.(*reader).initStorageReader < internal/streaming.(*PriorityReader).Read < " +
		"internal/vfs.(*TorrentFile).readContext.func1"
	if r.Signature != want || r.Count != 3 {
		t.Errorf("signature = %q, count %d; want %q, 3", r.Signature, r.Count, want)
	}
	if len(r.Accesses) != 2 || r.Accesses[0].Op != "write" || r.Accesses[0].Goroutine != "214" || r.Accesses[1].Goroutine != "198" {
		t.Fatalf("accesses = %+v", r.Accesses)
	}
	if got := r.Accesses[0].Frames[0]; got != "github.com/anacrolix/torrent.(*reader).initStorageReader (torrent@v1.60.0/reader.go:441)" {
		t.Errorf("frame = %q", got)
	}
	if got := r.Accesses[0].Created; len(got) != 1 || !strings.Contains(got[0], "readContext (vfs/torrent_file.go:473)") {
		t.Errorf("created = %q", got)
	}
	if !strings.HasPrefix(r.Text, "==================\nWARNING: DATA RACE\nWrite at") || len(r.Where) != 1 || r.Where[0] != "rep-1/race.123" {
		t.Errorf("text %q…, where %v", r.Text[:60], r.Where)
	}
	m := races[1]
	if m.Signature != "write runtime.mapaccess2_fast64 < main.main | write runtime.mapaccess2_fast64 < main.main.func4" {
		t.Errorf("map race signature = %q", m.Signature)
	}
	if m.Accesses[1].Goroutine != "main" {
		t.Errorf("main goroutine = %q", m.Accesses[1].Goroutine)
	}
}

// Reports of one race differ only by which access the detector caught
// first; an access's kind still tells two races on one field apart.
func TestSignatureIsOrderFree(t *testing.T) {
	a, _ := race.Parse(strings.NewReader(closeRace), "")
	b, _ := race.Parse(strings.NewReader(closeRaceSwapped), "")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("parsed %d and %d", len(a), len(b))
	}
	// closeRace's close side writes; the swapped report's reads: they
	// differ, and they must.
	if a[0].Signature == b[0].Signature {
		t.Errorf("a read and a write of the close side share a signature: %q", a[0].Signature)
	}
	c, _ := race.Parse(strings.NewReader(swappedWrite), "")
	if c[0].Signature != a[0].Signature {
		t.Errorf("swapped accesses:\n%q\n%q", c[0].Signature, a[0].Signature)
	}
}

func TestParseNothing(t *testing.T) {
	races, err := race.Parse(strings.NewReader("ok  \tgithub.com/x\t0.1s\nPASS\n"), "")
	if err != nil || len(races) != 0 {
		t.Errorf("Parse = %v, %v", races, err)
	}
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "race.10"), []byte(closeRace), 0o644)
	os.WriteFile(filepath.Join(dir, "race.11"), []byte(closeRace+mapRace), 0o644)
	os.WriteFile(filepath.Join(dir, "child.log"), []byte(mapRace), 0o644) // not a race log
	races, err := race.ParseDir(dir, "rep-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(races) != 2 || races[0].Count != 2 || races[1].Count != 1 {
		t.Fatalf("races = %+v", races)
	}
	if strings.Join(races[0].Where, " ") != "rep-2/race.10 rep-2/race.11" {
		t.Errorf("where = %v", races[0].Where)
	}
	if got := race.Env("/x/rep-1"); got != "GORACE=log_path=/x/rep-1/race halt_on_error=0 exitcode=0" {
		t.Errorf("Env = %q", got)
	}
}
