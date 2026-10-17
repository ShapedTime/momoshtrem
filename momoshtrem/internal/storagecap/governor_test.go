package storagecap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// fakeCache is a piece cache: a trim evicts down to the cap, but never
// what's pinned, nor the unpinned bytes below the reserve.
type fakeCache struct {
	mu       sync.Mutex
	capacity int64
	filled   int64
	pinned   int64
	reserve  int64
	sets     []int64 // every SetCapacity
	trims    []int64 // what each TrimToCapacity evicted
}

func (c *fakeCache) Info() CacheInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheInfo{Capacity: c.capacity, Filled: c.filled, Pinned: c.pinned}
}

func (c *fakeCache) SetCapacity(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capacity = n
	c.sets = append(c.sets, n)
}

func (c *fakeCache) SetReserve(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reserve = n
}

func (c *fakeCache) TrimToCapacity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	before := c.filled
	if c.capacity >= 0 && c.filled > c.capacity {
		c.filled = min(c.filled, max(c.capacity, c.pinned+c.reserve))
	}
	c.trims = append(c.trims, before-c.filled)
}

func (c *fakeCache) state() (capacity, filled int64, sets, trims []int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capacity, c.filled, append([]int64(nil), c.sets...), append([]int64(nil), c.trims...)
}

func (c *fakeCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sets, c.trims = nil, nil
}

// fakeDisk is statfs over a disk the test changes.
type fakeDisk struct {
	mu          sync.Mutex
	total, free int64
	err         error
}

func (d *fakeDisk) statfs(string) (int64, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.total, d.free, d.err
}

func (d *fakeDisk) set(total, free float64, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.total, d.free, d.err = gib(total), gib(free), err
}

// fakeSettings keeps the document in memory. broken is a stored document
// that doesn't decode: reads fail, a merge keeps it broken, and only a
// replace repairs it.
type fakeSettings struct {
	mu        sync.Mutex
	settings  library.StorageSettings
	readErr   error
	broken    bool
	readDelay time.Duration // widens the race between a PUT's read and its write
	updates   int
	replaces  int
}

func newFakeSettings() *fakeSettings {
	return &fakeSettings{settings: library.DefaultStorageSettings()}
}

var errBrokenDocument = fmt.Errorf("%w: json: cannot unmarshal string into Go struct field", library.ErrInvalidStorageSettings)

func (f *fakeSettings) StorageSettings(context.Context) (library.StorageSettings, error) {
	f.mu.Lock()
	settings, readErr, broken, delay := f.settings, f.readErr, f.broken, f.readDelay
	f.mu.Unlock()
	time.Sleep(delay)
	switch {
	case readErr != nil:
		return library.DefaultStorageSettings(), readErr
	case broken:
		return library.DefaultStorageSettings(), errBrokenDocument
	}
	return settings, nil
}

func (f *fakeSettings) UpdateStorageSettings(_ context.Context, p library.StorageSettingsPatch) (library.StorageSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	if f.broken {
		return library.DefaultStorageSettings(), errBrokenDocument
	}
	f.settings = p.Apply(f.settings)
	return f.settings, nil
}

func (f *fakeSettings) ReplaceStorageSettings(_ context.Context, p library.StorageSettingsPatch) (library.StorageSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaces++
	f.broken = false
	f.settings = p.Apply(library.DefaultStorageSettings())
	return f.settings, nil
}

func (f *fakeSettings) failReads(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readErr = err
}

// fakeClock fires the loop's minute when the test says.
type fakeClock struct {
	mu      sync.Mutex
	waiters []chan time.Time
}

func (c *fakeClock) After(time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, ch)
	return ch
}

// Tick fires the minutes waited on.
func (c *fakeClock) Tick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.waiters {
		ch <- time.Time{}
	}
	c.waiters = nil
}

func (c *fakeClock) waiting() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

type fakeDownloads struct{ usage DownloadUsage }

func (f fakeDownloads) DownloadUsage(context.Context) (DownloadUsage, error) { return f.usage, nil }

// guardedDownloads is a CommitGuard that records when it's held.
type guardedDownloads struct {
	settings *fakeSettings
	events   []string
}

func (g *guardedDownloads) DownloadUsage(context.Context) (DownloadUsage, error) {
	g.events = append(g.events, "usage")
	return DownloadUsage{}, nil
}

func (g *guardedDownloads) LockCommits() func() {
	g.events = append(g.events, "lock")
	return func() { g.events = append(g.events, fmt.Sprintf("unlock after %d update", g.settings.updates)) }
}

type harness struct {
	g        *Governor
	cache    *fakeCache
	disk     *fakeDisk
	settings *fakeSettings
	clock    *fakeClock
}

// newHarness is a Governor over the VM's 200 GiB disk with 160 free and a
// cache opened at limit, 37 GiB filled.
func newHarness(t *testing.T, config *int64) *harness {
	t.Helper()
	h := &harness{
		cache:    &fakeCache{capacity: -1, filled: gib(37)},
		disk:     &fakeDisk{total: gib(200), free: gib(160)},
		settings: newFakeSettings(),
		clock:    &fakeClock{},
	}
	h.g = New(Deps{
		Settings: h.settings, DiskPath: "/data", ConfigBytes: config,
		Statfs: h.disk.statfs, Clock: h.clock, StepPause: time.Microsecond,
	})
	t.Cleanup(h.g.Close)
	return h
}

// start opens the cache at the startup limit and starts the Governor,
// waiting for the loop to wait on its minute.
func (h *harness) start(t *testing.T) {
	t.Helper()
	l := h.g.StartupLimit(context.Background())
	h.cache.SetCapacity(l.LimitBytes)
	h.g.Start(context.Background(), h.cache)
	h.waitIdle(t)
}

// waitIdle waits for the loop to finish a pass and wait for its minute.
func (h *harness) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for h.clock.waiting() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the Governor's loop never waited")
		}
		time.Sleep(time.Millisecond)
	}
}

// tick runs one minute's pass.
func (h *harness) tick(t *testing.T) {
	t.Helper()
	h.clock.Tick()
	h.waitIdle(t)
}

func (h *harness) wantCap(t *testing.T, want int64) {
	t.Helper()
	if capacity, _, _, _ := h.cache.state(); capacity != want {
		t.Fatalf("capacity = %d (%.2f GiB), want %d (%.2f GiB)", capacity, float64(capacity)/float64(GiB), want, float64(want)/float64(GiB))
	}
}

// assertSteps checks every trim evicted at most TrimStepBytes, and every
// lowering moved the cap by at most that.
func assertSteps(t *testing.T, sets, trims []int64, from int64) {
	t.Helper()
	for _, n := range trims {
		if n > TrimStepBytes {
			t.Errorf("a trim evicted %d bytes, more than %d", n, TrimStepBytes)
		}
	}
	prev := from
	for _, s := range sets {
		if prev-s > TrimStepBytes {
			t.Errorf("the cap went from %d to %d in one step", prev, s)
		}
		prev = s
	}
}

func TestGovernor_StartupLimit(t *testing.T) {
	t.Run("auto: 75% of the disk, no floor before the cache is open", func(t *testing.T) {
		h := newHarness(t, nil)
		h.disk.set(200, 1, nil) // the floor would bind, but the fill is unknown
		if l := h.g.StartupLimit(context.Background()); l.Source != SourceAuto || l.LimitBytes != gib(150) || l.EffectiveBytes != gib(150) || l.Floored {
			t.Errorf("StartupLimit = %+v", l)
		}
	})
	t.Run("config.yaml", func(t *testing.T) {
		h := newHarness(t, ptr(int64(53760)<<20))
		if l := h.g.StartupLimit(context.Background()); l.Source != SourceConfig || l.LimitBytes != 53760<<20 {
			t.Errorf("StartupLimit = %+v", l)
		}
	})
	t.Run("without the settings: config.yaml, else auto", func(t *testing.T) {
		h := newHarness(t, nil)
		h.settings.settings = withLimit(ptr(gib(20)))
		h.settings.failReads(errors.New("db down"))
		if l := h.g.StartupLimit(context.Background()); l.Source != SourceAuto || l.LimitBytes != gib(150) {
			t.Errorf("StartupLimit = %+v", l)
		}
	})
	t.Run("without the disk: the setting, else 4 GiB", func(t *testing.T) {
		h := newHarness(t, nil)
		h.disk.set(0, 0, errors.New("no statfs"))
		if l := h.g.StartupLimit(context.Background()); l.LimitBytes != MinBytes {
			t.Errorf("StartupLimit = %+v", l)
		}
		h.settings.settings = withLimit(ptr(gib(20)))
		if l := h.g.StartupLimit(context.Background()); l.Source != SourceSetting || l.LimitBytes != gib(20) {
			t.Errorf("StartupLimit = %+v", l)
		}
	})
}

func TestGovernor_FirstPassAppliesTheFloor(t *testing.T) {
	h := newHarness(t, nil)
	h.cache.filled = gib(150)
	h.disk.set(200, 2, nil) // filled + free − 5 = 147
	h.g.Start(context.Background(), h.cache)
	// Start's pass runs before it returns.
	h.wantCap(t, gib(147))
	_, filled, sets, trims := h.cache.state()
	if filled != gib(147) {
		t.Errorf("filled = %d, want 147 GiB", filled)
	}
	assertSteps(t, sets, trims, gib(150))
}

// A cache opened at a cap below its fill (InitStorage after the limit came
// down) is trimmed in steps by the first pass, not in one go by the
// cache's first write.
func TestGovernor_FirstPassStepsDownAnOverfullCache(t *testing.T) {
	h := newHarness(t, nil)
	h.cache.filled = gib(60)
	h.settings.settings = withLimit(ptr(gib(50)))
	h.start(t) // opens the cache at 50 GiB
	h.wantCap(t, gib(50))
	_, filled, sets, trims := h.cache.state()
	if filled != gib(50) || len(trims) != 5 {
		t.Errorf("filled = %d, %d trims %v, want 50 GiB in 5", filled, len(trims), trims)
	}
	assertSteps(t, sets[1:], trims, gib(60)) // sets[0] is the open
}

// The floor follows the disk on each minute: something else fills it, the
// cap shrinks; it frees up, the cap grows back to the limit.
func TestGovernor_FloorShrinksWithTheDisk(t *testing.T) {
	h := newHarness(t, nil)
	h.start(t)
	h.wantCap(t, gib(150))

	h.disk.set(200, 20, nil) // 37 + 20 − 5 = 52
	h.cache.reset()
	h.tick(t)
	h.wantCap(t, gib(52))
	if _, _, _, trims := h.cache.state(); len(trims) != 0 {
		t.Errorf("lowering the cap above the fill trimmed: %v", trims)
	}

	h.disk.set(200, 3, nil) // 37 + 3 − 5 = 35: below the fill, trimmed in steps
	h.cache.reset()
	h.tick(t)
	h.wantCap(t, gib(35))
	_, filled, sets, trims := h.cache.state()
	if filled != gib(35) || len(sets) != 1 {
		t.Errorf("filled = %d, sets = %v", filled, sets)
	}
	assertSteps(t, sets, trims, gib(37))

	h.disk.set(200, 150, nil)
	h.tick(t)
	h.wantCap(t, gib(150))
}

// A PUT that lowers the limit wakes the Governor, which trims in steps of
// at most 2 GiB.
func TestGovernor_SteppedTrims(t *testing.T) {
	h := newHarness(t, nil)
	h.cache.filled = gib(120)
	h.start(t)
	h.wantCap(t, gib(150))
	h.cache.reset()

	limit := gib(100.5)
	if _, err := h.g.UpdateStorage(context.Background(), library.StorageSettingsPatch{SetLimit: true, LimitBytes: &limit}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if capacity, _, _, _ := h.cache.state(); capacity == limit {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the PUT's limit was never applied")
		}
		time.Sleep(time.Millisecond)
	}
	_, filled, sets, trims := h.cache.state()
	if filled != limit {
		t.Errorf("filled = %d, want %d", filled, limit)
	}
	// From the fill (120), not the old cap (150): 118, 116, …, 102, 100.5.
	if len(sets) != 10 || len(trims) != 10 {
		t.Errorf("%d steps (%v), want 10", len(sets), sets)
	}
	assertSteps(t, sets, trims, gib(120))

	// Raising is one SetCapacity, no trim.
	h.cache.reset()
	if _, err := h.g.UpdateStorage(context.Background(), library.StorageSettingsPatch{SetLimit: true}); err != nil {
		t.Fatal(err)
	}
	h.waitIdle(t)
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
		if capacity, _, _, _ := h.cache.state(); capacity == gib(150) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("auto was never applied")
		}
	}
	if _, _, sets, trims := h.cache.state(); len(sets) != 1 || len(trims) != 0 {
		t.Errorf("raising: sets %v, trims %v", sets, trims)
	}
}

// Pinned pieces can't be evicted (#201), nor the streaming reserve below
// them: the steps end where the trims can't go further, and the cap at the
// target. Minute after minute the cap then stays there: no steps over bytes
// nothing can evict (#202).
func TestGovernor_TrimsAroundPins(t *testing.T) {
	h := newHarness(t, nil)
	h.cache.filled, h.cache.pinned = gib(40), gib(30)
	h.settings.settings = withLimit(ptr(gib(10)))
	h.start(t)
	h.wantCap(t, gib(10))
	// The reserve is 25% of the 10 GiB limit.
	_, filled, sets, trims := h.cache.state()
	if filled != gib(32.5) {
		t.Errorf("filled = %d, want the pinned 30 GiB and the 2.5 GiB reserve", filled)
	}
	assertSteps(t, sets[1:len(sets)-1], trims[:len(trims)-1], gib(40))

	h.cache.reset()
	h.tick(t)
	h.wantCap(t, gib(10))
	if _, _, sets, trims := h.cache.state(); len(sets) != 0 || len(trims) != 0 {
		t.Errorf("a minute later: sets %v, trims %v; want none", sets, trims)
	}
}

func TestGovernor_PassWithoutSettingsOrDisk(t *testing.T) {
	h := newHarness(t, nil)
	h.settings.settings = withLimit(ptr(gib(60)))
	h.start(t)
	h.wantCap(t, gib(60))

	// The settings can't be read: the last ones read still apply, floor
	// included.
	h.settings.failReads(errors.New("db down"))
	h.disk.set(200, 10, nil) // 37 + 10 − 5 = 42
	h.tick(t)
	h.wantCap(t, gib(42))

	// The disk can't be read: nothing changes.
	h.settings.failReads(nil)
	h.disk.set(0, 0, errors.New("statfs failed"))
	h.tick(t)
	h.wantCap(t, gib(42))
	if _, err := h.g.Storage(context.Background()); err == nil {
		t.Error("GET without the disk succeeded")
	}
}

func TestGovernor_Close(t *testing.T) {
	h := newHarness(t, nil)
	h.start(t)
	h.g.Close()
	h.g.Close() // twice is fine
	h.disk.set(200, 6, nil)
	h.clock.Tick()
	time.Sleep(20 * time.Millisecond)
	h.wantCap(t, gib(150))

	// Close before Start is fine too.
	New(Deps{Settings: newFakeSettings()}).Close()
}

func TestGovernor_Storage(t *testing.T) {
	h := newHarness(t, ptr(int64(53760)<<20))
	h.cache.pinned = gib(7)
	h.g.SetDownloads(fakeDownloads{DownloadUsage{KeptBytes: gib(5), SmartBytes: gib(2), CommittedKeptBytes: gib(9)}})
	h.start(t)
	r, err := h.g.Storage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	// The contract's GET shape: config.yaml's 52.5 GiB, on the VM's disk.
	want := `{"limit_bytes":null,"source":"config","auto_bytes":161061273600,"config_bytes":56371445760,` +
		`"limit_effective_bytes":56371445760,"effective_bytes":56371445760,"floored":false,"floor_free_bytes":5368709120,` +
		`"min_bytes":4294967296,"max_bytes":193273528320,"reserve_bytes":14092861440,` +
		`"disk":{"total_bytes":214748364800,"free_bytes":171798691840},` +
		`"usage":{"kept_bytes":5368709120,"smart_bytes":2147483648,"cache_bytes":32212254720},` +
		`"committed_kept_bytes":9663676416,` +
		`"kept":{"max_bytes":null,"effective_max_bytes":28185722880,"default_until_watched":true,"default_days":30},` +
		`"smart":{"enabled":false,"episodes_ahead":2,"max_bytes":null,"effective_max_bytes":8455716864},` +
		`"background":{"enabled":true,"pause_while_streaming":true,"windows":[]}}`
	if string(got) != want {
		t.Errorf("GET =\n%s\nwant\n%s", got, want)
	}
}

func TestGovernor_UpdateStorage(t *testing.T) {
	ctx := context.Background()
	t.Run("stores, wakes the Governor and calls onChange", func(t *testing.T) {
		h := newHarness(t, nil)
		h.start(t)
		changes := 0
		h.g.SetOnChange(func() { changes++ })
		r, err := h.g.UpdateStorage(ctx, library.StorageSettingsPatch{
			SetLimit: true, LimitBytes: ptr(gib(100)),
			Smart: library.SmartSettingsPatch{Enabled: ptr(true)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if r.Source != SourceSetting || *r.LimitBytes != gib(100) || r.EffectiveBytes != gib(100) || !r.Smart.Enabled || changes != 1 {
			t.Errorf("report = %+v, changes %d", r, changes)
		}
		for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
			if capacity, _, _, _ := h.cache.state(); capacity == gib(100) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the new limit was never applied")
			}
		}
	})

	t.Run("holds the downloads' commit guard from the usage to the store", func(t *testing.T) {
		h := newHarness(t, nil)
		g := &guardedDownloads{settings: h.settings}
		h.g.SetDownloads(g)
		if _, err := h.g.UpdateStorage(ctx, library.StorageSettingsPatch{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(50))}}); err != nil {
			t.Fatal(err)
		}
		if want := []string{"lock", "usage", "unlock after 1 update"}; !reflect.DeepEqual(g.events, want) {
			t.Errorf("events = %v, want %v", g.events, want)
		}
	})

	t.Run("422 changes nothing", func(t *testing.T) {
		h := newHarness(t, nil)
		_, err := h.g.UpdateStorage(ctx, library.StorageSettingsPatch{SetLimit: true, LimitBytes: ptr(gib(190))})
		var field *FieldError
		if !errors.As(err, &field) || field.Field != "limit_bytes" || h.settings.updates != 0 {
			t.Errorf("err = %v, updates %d", err, h.settings.updates)
		}
	})

	t.Run("409: the kept cap below what's committed", func(t *testing.T) {
		h := newHarness(t, nil)
		h.g.SetDownloads(fakeDownloads{DownloadUsage{CommittedKeptBytes: gib(80)}})
		// Default kept cap: 75 GiB of 150, already short of 80.
		for _, tt := range []struct {
			name  string
			patch library.StorageSettingsPatch
			want  bool // 409
		}{
			{"a lower limit", library.StorageSettingsPatch{SetLimit: true, LimitBytes: ptr(gib(100))}, true},
			{"a lower kept cap", library.StorageSettingsPatch{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(60))}}, true},
			{"a raise that's still short helps", library.StorageSettingsPatch{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(78))}}, false},
			{"a change that doesn't touch the kept cap", library.StorageSettingsPatch{Smart: library.SmartSettingsPatch{Enabled: ptr(true)}}, false},
			{"enough for what's committed", library.StorageSettingsPatch{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(100))}}, false},
			{"back down below it", library.StorageSettingsPatch{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(79))}}, true},
		} {
			before := h.settings.updates
			_, err := h.g.UpdateStorage(ctx, tt.patch)
			var committed *CommittedError
			got := errors.As(err, &committed)
			if got != tt.want || (!tt.want && err != nil) {
				t.Errorf("%s: err = %v, want 409 %v", tt.name, err, tt.want)
			}
			if got && (committed.CommittedKeptBytes != gib(80) || h.settings.updates != before) {
				t.Errorf("%s: %+v, updates %d → %d", tt.name, committed, before, h.settings.updates)
			}
		}
	})
}

// A stored document that doesn't decode (a hand edit) never makes GET or
// PUT fail: they go on with the defaults, and a PUT replaces it with the
// defaults and its fields.
func TestGovernor_BrokenDocument(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, nil)
	h.settings.settings = withLimit(ptr(gib(20))) // what the broken document can't say
	h.settings.broken = true
	h.start(t)
	h.wantCap(t, gib(150)) // the defaults: auto

	r, err := h.g.Storage(ctx)
	if err != nil || r.Source != SourceAuto || r.LimitEffectiveBytes != gib(150) || r.Smart.EpisodesAhead != 2 {
		t.Fatalf("GET = %+v, %v; want the defaults", r, err)
	}
	if _, l, err := h.g.Limits(ctx); err != nil || l.LimitBytes != gib(150) {
		t.Errorf("Limits = %+v, %v", l, err)
	}

	r, err = h.g.UpdateStorage(ctx, library.StorageSettingsPatch{Smart: library.SmartSettingsPatch{EpisodesAhead: ptr(4)}})
	if err != nil {
		t.Fatalf("PUT over a broken document: %v", err)
	}
	if h.settings.replaces != 1 || h.settings.updates != 0 {
		t.Errorf("replaces %d, updates %d: want the document replaced, not merged", h.settings.replaces, h.settings.updates)
	}
	if r.Smart.EpisodesAhead != 4 || r.Source != SourceAuto || r.Kept.DefaultDays == nil || *r.Kept.DefaultDays != 30 {
		t.Errorf("after the repair = %+v", r)
	}
	// Repaired: the next PUT merges.
	if _, err := h.g.UpdateStorage(ctx, library.StorageSettingsPatch{Smart: library.SmartSettingsPatch{Enabled: ptr(true)}}); err != nil {
		t.Fatal(err)
	}
	if h.settings.replaces != 1 || h.settings.updates != 1 || !h.settings.settings.Smart.Enabled || h.settings.settings.Smart.EpisodesAhead != 4 {
		t.Errorf("after the next PUT: replaces %d, updates %d, %+v", h.settings.replaces, h.settings.updates, h.settings.settings.Smart)
	}

	// A failure to read is still an error.
	h.settings.failReads(errors.New("db down"))
	if _, err := h.g.Storage(ctx); err == nil {
		t.Error("GET without the database succeeded")
	}
}

// PUTs run one at a time: two that each fit alone, but not together, can't
// both pass their checks against the same read.
func TestGovernor_ConcurrentPUTs(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, nil)
	h.start(t)
	h.settings.mu.Lock()
	h.settings.readDelay = 20 * time.Millisecond
	h.settings.mu.Unlock()
	// Room: 150 − 20 = 130 GiB. Kept 120 fits alone (smart's default
	// shrinks); smart 30 fits beside kept's default 75; together they don't.
	patches := []library.StorageSettingsPatch{
		{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(120))}},
		{Smart: library.SmartSettingsPatch{SetMaxBytes: true, MaxBytes: ptr(gib(30))}},
	}
	errs := make([]error, len(patches))
	var wg sync.WaitGroup
	for i, p := range patches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = h.g.UpdateStorage(ctx, p)
		}()
	}
	wg.Wait()
	failed := 0
	for _, err := range errs {
		var field *FieldError
		if errors.As(err, &field) {
			failed++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	s := h.settings.settings
	if failed != 1 || (s.Kept.MaxBytes != nil && s.Smart.MaxBytes != nil) {
		t.Errorf("%d of 2 PUTs refused (%v); stored kept %v, smart %v: want exactly one through", failed, errs, s.Kept.MaxBytes, s.Smart.MaxBytes)
	}
}
