package streamtrace

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type obj = map[string]any

// canon renders a JSON value with sorted keys, numbers as written.
func canon(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decodeLines(t *testing.T, out []byte) []obj {
	t.Helper()
	if len(out) > 0 && out[len(out)-1] != '\n' {
		t.Fatal("output doesn't end with a newline")
	}
	var lines []obj
	for _, line := range bytes.Split(bytes.TrimSuffix(out, []byte("\n")), []byte("\n")) {
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var o obj
		if err := dec.Decode(&o); err != nil {
			t.Fatalf("line %s: %v", line, err)
		}
		lines = append(lines, o)
	}
	return lines
}

func TestWriteNDJSON(t *testing.T) {
	r := New(64)
	r.SetLevel(LevelDetailed)
	path := r.Intern("/Movies/A (2001)/A.mkv")
	ua := r.Intern("Infuse-Direct/8.1")
	hash := r.Intern("0123abcd")
	get := r.Intern("GET")
	client := r.Intern("192.168.1.20")
	mkv := r.Intern("mkv")
	stream := r.Intern("hdrezka:123/238/1080p")
	const (
		pathS   = "/Movies/A (2001)/A.mkv"
		uaS     = "Infuse-Direct/8.1"
		hashS   = "0123abcd"
		clientS = "192.168.1.20"
		streamS = "hdrezka:123/238/1080p"
	)

	tests := []struct {
		emit   func()
		kind   string
		level  string
		fields obj
	}{
		{func() { r.HTTPStart(7, get, path, ua, client, 100, -1, true) }, "http.start", "summary",
			obj{"req": 7, "method": "GET", "path": pathS, "ua": uaS, "client": clientS, "range": []int{100, -1}, "if_range": true}},
		{func() { r.HTTPStart(8, get, path, 0, 0, -1, -1, false) }, "http.start", "summary",
			obj{"req": 8, "method": "GET", "path": pathS}},
		{func() { r.HTTPEnd(7, 206, 5000, 2*time.Millisecond, 3*time.Second, true) }, "http.end", "summary",
			obj{"req": 7, "status": 206, "bytes": 5000, "ttfb_ns": 2000000, "dur_ns": 3000000000, "aborted": true}},
		{func() { r.HTTPSeek(7, -10, 2, 990) }, "http.seek", "summary",
			obj{"req": 7, "offset": -10, "whence": 2, "pos": 990}},
		{func() { r.VFSOpen(7, 9, path, hash, FileTorrent, 1<<30, 5*time.Millisecond, true) }, "vfs.open", "summary",
			obj{"req": 7, "file": 9, "path": pathS, "hash": hashS, "type": "torrent", "size": 1 << 30, "dur_ns": 5000000, "failed": true}},
		{func() {
			r.TFRead(9, 4096, 32768, 600*time.Millisecond, 10*time.Millisecond, ReadSlow|ReadDrain|ReadFirstByte)
		}, "tf.read", "summary",
			obj{"file": 9, "off": 4096, "n": 32768, "dur_ns": 600000000, "drain_ns": 10000000, "slow": true, "drain": true, "first_byte": true}},
		{func() { r.TFRead(9, 0, 100, time.Millisecond, 0, 0) }, "tf.read", "detailed",
			obj{"file": 9, "off": 0, "n": 100, "dur_ns": 1000000, "drain_ns": 0}},
		{func() { r.TFRead(9, 0, 0, time.Second, 0, ReadTimeout|ReadSeek|ReadError) }, "tf.read", "summary",
			obj{"file": 9, "off": 0, "n": 0, "dur_ns": 1000000000, "drain_ns": 0, "timeout": true, "seek": true, "error": true}},
		{func() { r.TFActivation(9, hash, 1500*time.Millisecond, true) }, "tf.activation", "summary",
			obj{"file": 9, "hash": hashS, "wait_ns": 1500000000, "timed_out": true}},
		{func() { r.TFClose(9, 45*time.Second, 123456) }, "tf.close", "summary",
			obj{"file": 9, "open_ns": 45000000000, "pos": 123456}},
		{func() { r.ReaderCreate(11, 9, hash, path, 1<<30, 4096, 1<<21) }, "reader.create", "summary",
			obj{"reader": 11, "file": 9, "hash": hashS, "path": pathS, "size": 1 << 30, "offset": 4096, "piece_len": 1 << 21}},
		{func() { r.ReaderClose(11, 31*time.Second, 777, true) }, "reader.close", "summary",
			obj{"reader": 11, "open_ns": 31000000000, "pos": 777, "reset": true}},
		{func() { r.FormatStart(11) }, "format.start", "summary", obj{"reader": 11}},
		{func() { r.FormatEnd(11, mkv, 0, 0, 3*time.Millisecond) }, "format.end", "summary",
			obj{"reader": 11, "format": "mkv", "moov_off": 0, "moov_size": 0, "dur_ns": 3000000}},
		{func() { r.PrioUpdate(11, PrioSeek, true, 100, 200, 200, 300, 70000, 2*time.Microsecond) }, "prio.update", "summary",
			obj{"reader": 11, "type": "seek", "off": 100, "now": []int{100, 200}, "ra": []int{200, 300}, "downgraded": 65535, "loop_ns": 2000, "forward": true}},
		{func() { r.PrioUpdate(11, PrioInitial, false, 0, 10, 90, 100, 0, time.Hour) }, "prio.update", "summary",
			obj{"reader": 11, "type": "initial", "off": 0, "now": []int{0, 10}, "ra": []int{90, 100}, "downgraded": 0, "loop_ns": 1<<40 - 1}},
		{func() { r.PrioUpdate(11, PrioDebounced, false, 5, 0, 0, 0, 0, 0) }, "prio.update", "detailed",
			obj{"reader": 11, "type": "seek_debounced", "off": 5}},
		{func() { r.TorrentAdd(hash, 2*time.Second, 1000, 3, AddFromStore|AddTimeout) }, "torrent.add", "summary",
			obj{"hash": hashS, "dur_ns": 2000000000, "pieces": 1000, "files": 3, "from_store": true, "timeout": true}},
		{func() { r.TorrentAdd(hash, time.Second, 0, 0, AddFailed) }, "torrent.add", "summary",
			obj{"hash": hashS, "dur_ns": 1000000000, "pieces": 0, "files": 0, "failed": true}},
		{func() { r.TorrentRemove(hash, true, true) }, "torrent.remove", "summary",
			obj{"hash": hashS, "loaded": true, "delete_data": true}},
		{func() { r.TorrentState(hash, StateIdle, StateActive) }, "torrent.state", "summary",
			obj{"hash": hashS, "from": "idle", "to": "active"}},
		{func() { r.TorrentState(hash, StateNone, StatePaused) }, "torrent.state", "summary",
			obj{"hash": hashS, "from": "none", "to": "paused"}},
		{func() { r.SessionBegin(12, path, ua, client) }, "session.begin", "summary",
			obj{"session": 12, "path": pathS, "ua": uaS, "client": clientS}},
		{func() { r.SessionEnd(12, 1<<20, 10*time.Minute) }, "session.end", "summary",
			obj{"session": 12, "bytes": 1 << 20, "dur_ns": 600000000000}},
		{func() { r.Remote(RemoteOpen, 13, 0, 0, 80*time.Millisecond, false) }, "remote.open", "summary",
			obj{"file": 13, "off": 0, "n": 0, "dur_ns": 80000000}},
		{func() { r.Remote(RemoteReuse, 13, 0, 32768, time.Millisecond, false) }, "remote.reuse", "detailed",
			obj{"file": 13, "off": 0, "n": 32768, "dur_ns": 1000000}},
		{func() { r.Remote(RemoteSkip, 13, 1000, 200, 4*time.Millisecond, false) }, "remote.skip", "summary",
			obj{"file": 13, "off": 1000, "n": 200, "dur_ns": 4000000}},
		{func() { r.Remote(RemoteReopen, 13, 5000, 0, 0, true) }, "remote.reopen", "summary",
			obj{"file": 13, "off": 5000, "n": 0, "dur_ns": 0, "failed": true}},
		{func() { r.Remote(RemoteTimeout, 13, 5000, 0, 30*time.Second, true) }, "remote.timeout", "summary",
			obj{"file": 13, "off": 5000, "n": 0, "dur_ns": 30000000000, "failed": true}},
		{func() { r.CDN(CDNOpen, stream, 100, 206, 300*time.Millisecond, false) }, "cdn.open", "summary",
			obj{"stream": streamS, "off": 100, "status": 206, "dur_ns": 300000000}},
		{func() { r.CDN(CDNRefresh, stream, 100, 403, 0, false) }, "cdn.refresh", "summary",
			obj{"stream": streamS, "off": 100, "status": 403, "dur_ns": 0}},
		{func() { r.CDN(CDNRetry, stream, 0, 503, time.Second, true) }, "cdn.retry", "summary",
			obj{"stream": streamS, "off": 0, "status": 503, "dur_ns": 1000000000, "failed": true}},
		{func() { r.CDN(CDNSlot, stream, 0, 0, 5*time.Millisecond, false) }, "cdn.slot", "summary",
			obj{"stream": streamS, "off": 0, "status": 0, "dur_ns": 5000000}},
	}
	for _, tt := range tests {
		tt.emit()
	}
	// Ops out of range record nothing.
	r.Remote(RemoteOp(9), 1, 0, 0, 0, false)
	r.CDN(CDNOp(9), stream, 0, 0, 0, false)

	events, next, lost := r.Since(0, 0)
	if len(events) != len(tests) || lost != 0 || next != uint64(len(tests))+1 {
		t.Fatalf("got %d events (next %d, lost %d), want %d", len(events), next, lost, len(tests))
	}
	var out bytes.Buffer
	if err := r.WriteNDJSON(&out, events); err != nil {
		t.Fatal(err)
	}
	lines := decodeLines(t, out.Bytes())
	if len(lines) != len(tests) {
		t.Fatalf("got %d lines, want %d", len(lines), len(tests))
	}
	for i, tt := range tests {
		got := lines[i]
		if seq := got["seq"]; canon(t, seq) != canon(t, i+1) {
			t.Errorf("line %d: seq %v", i, seq)
		}
		if tn, ok := got["t_ns"].(json.Number); !ok || strings.HasPrefix(tn.String(), "-") {
			t.Errorf("line %d: t_ns %v", i, got["t_ns"])
		}
		if got["kind"] != tt.kind || got["level"] != tt.level {
			t.Errorf("line %d: kind %v level %v, want %s %s", i, got["kind"], got["level"], tt.kind, tt.level)
		}
		for _, k := range []string{"seq", "t_ns", "kind", "level"} {
			delete(got, k)
		}
		if g, w := canon(t, got), canon(t, tt.fields); g != w {
			t.Errorf("%s (line %d):\n got %s\nwant %s", tt.kind, i, g, w)
		}
	}
}

func TestWriteNDJSONEscapesStrings(t *testing.T) {
	r := New(8)
	r.SetLevel(LevelSummary)
	weird := "/a \"b\"\\c\n\t\r\x01é😀\xff/<tag>&"
	r.SessionBegin(1, r.Intern(weird), 0, 0)
	events, _, _ := r.Since(0, 0)
	var out bytes.Buffer
	if err := r.WriteNDJSON(&out, events); err != nil {
		t.Fatal(err)
	}
	lines := decodeLines(t, out.Bytes())
	want := "/a \"b\"\\c\n\t\r\x01é😀\uFFFD/<tag>&"
	if got := lines[0]["path"]; got != want {
		t.Fatalf("path %q, want %q", got, want)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteNDJSONBatches(t *testing.T) {
	r := New(4096)
	r.SetLevel(LevelDetailed)
	path := r.Intern(strings.Repeat("p", 200))
	for i := range 3000 {
		r.SessionBegin(uint64(i), path, 0, 0)
	}
	events, _, _ := r.Since(0, 0)

	var out bytes.Buffer
	if err := r.WriteNDJSON(&out, events); err != nil {
		t.Fatal(err)
	}
	if n := len(decodeLines(t, out.Bytes())); n != 3000 {
		t.Fatalf("got %d lines, want 3000", n)
	}

	boom := errors.New("boom")
	if err := r.WriteNDJSON(failingWriter{boom}, events); !errors.Is(err, boom) {
		t.Fatalf("error %v, want boom", err)
	}
	if err := r.WriteNDJSON(failingWriter{boom}, events[:1]); !errors.Is(err, boom) {
		t.Fatalf("short batch: error %v, want boom", err)
	}
}
