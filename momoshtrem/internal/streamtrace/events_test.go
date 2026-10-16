package streamtrace

import (
	"bufio"
	"bytes"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// emitAll calls every constructor once: a plain tf.read (detailed only),
// a debounced prio.update and a remote.reuse (detailed only), and one of
// everything else (summary).
func emitAll(r *Recorder, path, ua, hash ID) {
	r.HTTPStart(1, path, path, ua, ua, 0, -1, true)
	r.HTTPEnd(1, 206, 1<<20, time.Millisecond, time.Second, false)
	r.HTTPSeek(1, 0, 2, 1<<30)
	r.VFSOpen(1, 2, path, hash, FileTorrent, 1<<30, time.Millisecond, false)
	r.TFRead(2, 0, 32<<10, time.Millisecond, 0, 0)
	r.TFRead(2, 0, 32<<10, time.Second, 0, ReadSlow)
	r.TFActivation(2, hash, time.Second, true)
	r.TFClose(2, time.Minute, 1<<20)
	r.ReaderCreate(3, 2, hash, path, 1<<30, 0, 1<<20)
	r.ReaderClose(3, time.Minute, 1<<20, true)
	r.FormatStart(3)
	r.FormatEnd(3, path, 100, 200, time.Millisecond)
	r.PrioUpdate(3, PrioSeek, true, 0, 8<<20, 8<<20, 40<<20, 2, time.Microsecond)
	r.PrioUpdate(3, PrioDebounced, false, 1, 0, 0, 0, 0, 0)
	r.TorrentAdd(hash, time.Second, 100, 2, AddFromStore)
	r.TorrentRemove(hash, true, false)
	r.TorrentState(hash, StateIdle, StateActive)
	r.SessionBegin(4, path, ua, ua)
	r.SessionEnd(4, 1<<20, time.Minute)
	r.Remote(RemoteOpen, 5, 0, 0, time.Millisecond, false)
	r.Remote(RemoteReuse, 5, 0, 32<<10, time.Millisecond, false)
	r.CDN(CDNOpen, path, 0, 206, time.Millisecond, false)
}

const (
	emitAllEvents         = 22
	emitAllDetailedEvents = 3 // the plain tf.read, the debounced update, the reuse
)

func TestLevels(t *testing.T) {
	r := New(256)
	if r.Intern("x") != 0 || r.NewID() != 0 {
		t.Fatal("off: Intern and NewID must return 0")
	}
	emitAll(r, 0, 0, 0)
	if r.slots != nil || r.Stats().Head != 0 {
		t.Fatal("off: an emit recorded something, or the ring was allocated")
	}

	r.SetLevel(LevelSummary)
	if r.slots == nil {
		t.Fatal("the first level above off didn't allocate the ring")
	}
	emitAll(r, 0, 0, 0)
	summary, _, _ := r.Since(0, 0)
	if len(summary) != emitAllEvents-emitAllDetailedEvents {
		t.Fatalf("summary: got %d events, want %d", len(summary), emitAllEvents-emitAllDetailedEvents)
	}
	for _, e := range summary {
		if e.Level != LevelSummary {
			t.Errorf("summary: %v at level %v", e.Kind, e.Level)
		}
		if e.Kind == KindRemoteReuse || (e.Kind == KindPrioUpdate && PrioType(e.p[1]&0xf) == PrioDebounced) {
			t.Errorf("summary: recorded %v", e.Kind)
		}
		if e.Kind == KindTFRead && ReadFlags(hi32(e.p[2]))&ReadSlow == 0 {
			t.Error("summary: recorded a plain tf.read")
		}
	}

	r.SetLevel(LevelDetailed)
	cursor := r.Stats().Head + 1
	emitAll(r, 0, 0, 0)
	detailed, _, _ := r.Since(cursor, 0)
	if len(detailed) != emitAllEvents {
		t.Fatalf("detailed: got %d events, want %d", len(detailed), emitAllEvents)
	}
	n := 0
	for _, e := range detailed {
		if e.Level == LevelDetailed {
			n++
		}
	}
	if n != emitAllDetailedEvents {
		t.Fatalf("detailed: %d events at level detailed, want %d", n, emitAllDetailedEvents)
	}

	// Off again: nothing new, and the ring keeps what it has.
	r.SetLevel(LevelOff)
	head := r.Stats().Head
	emitAll(r, 0, 0, 0)
	if r.Stats().Head != head {
		t.Fatal("off: an emit recorded something")
	}
	if events, _, _ := r.Since(0, 0); len(events) != int(head) {
		t.Fatalf("off: Since returned %d events, want the %d recorded", len(events), head)
	}
}

func TestSetLevelClamps(t *testing.T) {
	r := New(8)
	r.SetLevel(LevelDetailed + 5)
	if r.Level() != LevelDetailed || !r.On(LevelDetailed) {
		t.Fatalf("level %v", r.Level())
	}
	r.SetLevel(-1)
	if r.Level() != LevelOff || r.On(LevelSummary) {
		t.Fatalf("level %v", r.Level())
	}
}

func TestEmitDoesNotAllocate(t *testing.T) {
	r := New(1024)
	for _, l := range []Level{LevelOff, LevelSummary, LevelDetailed} {
		r.SetLevel(l)
		path, ua, hash := r.Intern("/Movies/A (2001)/A.mkv"), r.Intern("Infuse/7"), r.Intern("abc")
		if allocs := testing.AllocsPerRun(100, func() { emitAll(r, path, ua, hash) }); allocs != 0 {
			t.Errorf("%v: %v allocations per emitAll", l, allocs)
		}
		if allocs := testing.AllocsPerRun(100, func() { r.Intern("Infuse/7"); r.NewID() }); allocs != 0 {
			t.Errorf("%v: %v allocations interning a known string", l, allocs)
		}
	}
}

func TestNewID(t *testing.T) {
	r := New(8)
	r.SetLevel(LevelSummary)
	if a, b := r.NewID(), r.NewID(); a == 0 || b == a {
		t.Fatalf("NewID: %d, %d", a, b)
	}
}

func TestParseLevel(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Level
		ok   bool
	}{
		{"off", LevelOff, true},
		{"summary", LevelSummary, true},
		{" Detailed ", LevelDetailed, true},
		{"", LevelOff, false},
		{"on", LevelOff, false},
		{"debug", LevelOff, false},
	} {
		got, err := ParseLevel(tt.in)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("ParseLevel(%q) = %v, %v", tt.in, got, err)
		}
	}
	for _, l := range []Level{LevelOff, LevelSummary, LevelDetailed} {
		if got, err := ParseLevel(l.String()); err != nil || got != l {
			t.Errorf("round trip of %v: %v, %v", l, got, err)
		}
	}
}

func TestDefault(t *testing.T) {
	if Default() != Default() {
		t.Fatal("Default isn't one recorder")
	}
	st := Default().Stats()
	if st.Level != LevelOff || st.Size != DefaultSize || st.Start.IsZero() {
		t.Fatalf("Default: %+v", st)
	}
	if st.Start.After(time.Now()) {
		t.Fatal("Default's clock starts in the future")
	}
	if std.slots != nil {
		t.Fatal("Default allocated its ring while off")
	}
}

func TestNewRoundsUp(t *testing.T) {
	for in, want := range map[int]int{0: DefaultSize, -3: DefaultSize, 1: 1, 5: 8, 64: 64, 100: 128} {
		if got := New(in).Stats().Size; got != want {
			t.Errorf("New(%d): size %d, want %d", in, got, want)
		}
	}
}

func TestKindNames(t *testing.T) {
	seen := map[string]bool{}
	for k := Kind(1); k < kindCount; k++ {
		name := k.String()
		if strings.HasPrefix(name, "kind(") || seen[name] {
			t.Errorf("kind %d: name %q", k, name)
		}
		seen[name] = true
		if w := kindWords[k]; w < 1 || w > payloadWords {
			t.Errorf("%v: %d payload words", k, w)
		}
	}
	if kindCount > maxKinds {
		t.Fatalf("%d kinds don't fit the header's 6 bits", kindCount)
	}
	if got := Kind(200).String(); got != "kind(200)" {
		t.Errorf("unknown kind: %q", got)
	}
}

// TestHooksInline checks with the compiler that every hook inlines (R1:
// off, a hook is one atomic load). A hook that stops inlining costs a call
// even when tracing is off.
func TestHooksInline(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the compiler")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	out, err := exec.Command(goBin, "build", "-gcflags=-m", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build -gcflags=-m: %v\n%s", err, out)
	}
	inlinable := map[string]bool{}
	// Anchored: "(*Recorder).SetLevel.deferwrap1" is a closure, not SetLevel.
	re := regexp.MustCompile(`can inline \(\*Recorder\)\.(\w+)$`)
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			inlinable[m[1]] = true
		}
	}

	notHooks := map[string]bool{"SetLevel": true, "Since": true, "WriteNDJSON": true, "Stats": true}
	typ := reflect.TypeFor[*Recorder]()
	for i := range typ.NumMethod() {
		name := typ.Method(i).Name
		if !notHooks[name] && !inlinable[name] {
			t.Errorf("(*Recorder).%s doesn't inline", name)
		}
	}
	if !regexp.MustCompile(`can inline Default\b`).Match(out) {
		t.Error("Default doesn't inline")
	}
}
