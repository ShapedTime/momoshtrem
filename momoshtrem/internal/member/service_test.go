package member

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStore keeps members in memory and records the calls.
type fakeStore struct {
	mu        sync.Mutex
	members   []Member
	next      int64
	lists     int
	listErr   error
	createErr error
	deleteErr error
	calls     []string
	created   []string // name/color as Create got them
	deleteCtx context.Context
	listGate  chan struct{} // the next List waits on it (gateList)
	listRead  chan struct{}
}

func newFakeStore(names ...string) *fakeStore {
	f := &fakeStore{next: 1}
	for i, name := range names {
		f.members = append(f.members, Member{ID: f.next, Name: name, Color: Colors[i%len(Colors)], CreatedAt: time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)})
		f.next++
	}
	return f
}

func (f *fakeStore) record(call string) {
	f.calls = append(f.calls, call)
}

func (f *fakeStore) List(context.Context) ([]Member, error) {
	f.mu.Lock()
	f.lists++
	list, err := slices.Clone(f.members), f.listErr
	gate, read := f.listGate, f.listRead
	f.listGate = nil // only the next List waits
	f.mu.Unlock()
	if gate != nil {
		close(read)
		<-gate // read, but not answered yet
	}
	if err != nil {
		return nil, err
	}
	return list, nil
}

// gateList makes the next List wait, once it has read the members, until
// the returned func is called; read is closed once it has.
func (f *fakeStore) gateList() (read <-chan struct{}, release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listGate, f.listRead = make(chan struct{}), make(chan struct{})
	gate := f.listGate
	return f.listRead, func() { close(gate) }
}

func (f *fakeStore) Create(_ context.Context, name, color string) (Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("create")
	f.created = append(f.created, name+"/"+color)
	if f.createErr != nil {
		return Member{}, f.createErr
	}
	m := Member{ID: f.next, Name: name, Color: color}
	f.next++
	f.members = append(f.members, m)
	return m, nil
}

func (f *fakeStore) Update(_ context.Context, id int64, p Patch) (Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("update")
	for i := range f.members {
		if f.members[i].ID != id {
			continue
		}
		if p.Name != nil {
			f.members[i].Name = *p.Name
		}
		if p.Color != nil {
			f.members[i].Color = *p.Color
		}
		return f.members[i], nil
	}
	return Member{}, ErrNotFound
}

func (f *fakeStore) Delete(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("delete")
	f.deleteCtx = ctx
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.members = slices.DeleteFunc(f.members, func(m Member) bool { return m.ID == id })
	return nil
}

func (f *fakeStore) listCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists
}

func TestParseID(t *testing.T) {
	for _, tt := range []struct {
		header string
		want   int64
	}{
		{"1", 1}, {"42", 42}, {"007", 7}, {"9223372036854775807", 9223372036854775807},
	} {
		if got, err := ParseID(tt.header); err != nil || got != tt.want {
			t.Errorf("ParseID(%q) = %d, %v, want %d", tt.header, got, err, tt.want)
		}
	}
	for _, header := range []string{"", "0", "000", "-1", "+1", "1.5", "1e3", "0x1", "abc", "1 ", " 1", "9223372036854775808", "١"} {
		if got, err := ParseID(header); !errors.Is(err, ErrMalformed) {
			t.Errorf("ParseID(%q) = %d, %v, want ErrMalformed", header, got, err)
		}
	}
}

func TestCleanNameAndColor(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"Teymur", "Teymur"},
		{"  Sam \t", "Sam"},
		{strings.Repeat("é", MaxNameLen), strings.Repeat("é", MaxNameLen)}, // code points, not bytes
		{" " + strings.Repeat("a", MaxNameLen) + " ", strings.Repeat("a", MaxNameLen)},
	} {
		if got, err := CleanName(tt.in); err != nil || got != tt.want {
			t.Errorf("CleanName(%q) = %q, %v", tt.in, got, err)
		}
	}
	for _, in := range []string{"", "   ", strings.Repeat("a", MaxNameLen+1), strings.Repeat("é", MaxNameLen+1)} {
		var v *ValidationError
		if _, err := CleanName(in); !errors.As(err, &v) || v.Field != "name" {
			t.Errorf("CleanName(%q) = %v, want a name ValidationError", in, err)
		}
	}

	for _, c := range Colors {
		if err := CheckColor(c); err != nil {
			t.Errorf("CheckColor(%q) = %v", c, err)
		}
	}
	for _, c := range []string{"", "Teal", "blue", " teal"} {
		var v *ValidationError
		if err := CheckColor(c); !errors.As(err, &v) || v.Field != "color" || v.Message != "color must be rose, orchid, periwinkle, teal, lime or sand" {
			t.Errorf("CheckColor(%q) = %v", c, err)
		}
	}

	for _, tt := range []struct {
		used []string
		want string
	}{
		{nil, "rose"},
		{[]string{"teal"}, "rose"},
		{[]string{"teal", "rose"}, "orchid"},
		{[]string{"rose", "orchid", "periwinkle", "teal", "lime"}, "sand"},
		{[]string{"rose", "rose", "orchid", "periwinkle", "teal", "lime", "sand"}, "rose"}, // all used: the first
	} {
		if got := DefaultColor(tt.used); got != tt.want {
			t.Errorf("DefaultColor(%v) = %s, want %s", tt.used, got, tt.want)
		}
	}
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	if _, ok := IDFrom(ctx); ok {
		t.Error("IDFrom(Background) is set")
	}
	if _, err := Require(ctx); !errors.Is(err, ErrNoMember) {
		t.Errorf("Require(Background) = %v", err)
	}
	ctx = WithID(ctx, 2)
	if id, ok := IDFrom(ctx); !ok || id != 2 {
		t.Errorf("IDFrom = %d, %v", id, ok)
	}
	if id, err := Require(WithID(ctx, 3)); err != nil || id != 3 {
		t.Errorf("Require = %d, %v", id, err)
	}
}

func TestService_Resolve(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur", "Gone", "Sam")
	store.members = slices.Delete(store.members, 1, 2) // ids 1 and 3
	s := NewService(store)

	// No header: the lowest id, the list read once.
	if id, err := s.Resolve(ctx, ""); err != nil || id != 1 || store.listCount() != 1 {
		t.Errorf(`Resolve("") = %d, %v after %d reads`, id, err, store.listCount())
	}
	if id, err := s.Resolve(ctx, "3"); err != nil || id != 3 || store.listCount() != 1 {
		t.Errorf(`Resolve("3") = %d, %v after %d reads`, id, err, store.listCount())
	}
	// A malformed value never reads the list.
	if _, err := s.Resolve(ctx, "two"); !errors.Is(err, ErrMalformed) || store.listCount() != 1 {
		t.Errorf(`Resolve("two") = %v after %d reads`, err, store.listCount())
	}
	// An id nobody has: the list is read again once.
	if _, err := s.Resolve(ctx, "2"); !errors.Is(err, ErrUnknown) || store.listCount() != 2 {
		t.Errorf(`Resolve("2") = %v after %d reads, want ErrUnknown after 2`, err, store.listCount())
	}
	// One added elsewhere (another process) is found on that read.
	store.mu.Lock()
	store.members = append(store.members, Member{ID: 4, Name: "Elsewhere", Color: "lime"})
	store.mu.Unlock()
	if id, err := s.Resolve(ctx, "4"); err != nil || id != 4 || store.listCount() != 3 {
		t.Errorf(`Resolve("4") = %d, %v after %d reads`, id, err, store.listCount())
	}
	if ids, err := s.IDs(ctx); err != nil || !slices.Equal(ids, []int64{1, 3, 4}) {
		t.Errorf("IDs = %v, %v", ids, err)
	}
}

// A Load that fails at startup is tried again on first use; a first use
// that reads the list doesn't read it twice for an unknown id.
func TestService_LoadFails(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur")
	store.listErr = errors.New("db down")
	s := NewService(store)
	if err := s.Load(ctx); err == nil {
		t.Fatal("Load = nil, want the store's error")
	}
	if _, err := s.Resolve(ctx, ""); err == nil || errors.Is(err, ErrUnknown) {
		t.Errorf("Resolve while the store fails = %v", err)
	}
	store.mu.Lock()
	store.listErr = nil
	store.mu.Unlock()
	before := store.listCount()
	if _, err := s.Resolve(ctx, "9"); !errors.Is(err, ErrUnknown) || store.listCount() != before+1 {
		t.Errorf(`Resolve("9") = %v after %d reads, want one`, err, store.listCount()-before)
	}
	if id, err := s.Default(ctx); err != nil || id != 1 {
		t.Errorf("Default = %d, %v", id, err)
	}
}

func TestService_Create(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur")
	s := NewService(store)
	var added []int64
	s.OnAdded(func(_ context.Context, id int64) { added = append(added, id) })

	for _, tt := range []struct {
		name  string
		color *string
		field string
	}{
		{"   ", nil, "name"},
		{strings.Repeat("x", MaxNameLen+1), nil, "name"},
		{"Sam", ptr("blue"), "color"},
	} {
		var v *ValidationError
		if _, err := s.Create(ctx, tt.name, tt.color); !errors.As(err, &v) || v.Field != tt.field {
			t.Errorf("Create(%q, %v) = %v, want a %s ValidationError", tt.name, tt.color, err, tt.field)
		}
	}
	if len(store.created) != 0 || len(added) != 0 {
		t.Fatalf("invalid creates reached the store: %v, listeners %v", store.created, added)
	}

	// Trimmed; no colour leaves it to the store (the first unused).
	sam, err := s.Create(ctx, "  Sam ", nil)
	if err != nil || sam.ID != 2 || sam.Name != "Sam" || sam.Trakt.State != "off" || sam.Trakt.Username != nil {
		t.Fatalf("Create = %+v, %v", sam, err)
	}
	if _, err := s.Create(ctx, "Ari", ptr("lime")); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Sam/", "Ari/lime"}; !slices.Equal(store.created, want) {
		t.Errorf("store got %v, want %v", store.created, want)
	}
	if !slices.Equal(added, []int64{2, 3}) {
		t.Errorf("OnAdded got %v", added)
	}
	// The list is read again after each change, so the new ids resolve
	// without a miss.
	reads := store.listCount()
	if id, err := s.Resolve(ctx, "3"); err != nil || id != 3 || store.listCount() != reads {
		t.Errorf(`Resolve("3") = %d, %v after %d more reads`, id, err, store.listCount()-reads)
	}

	// The store's errors come back as they are, and tell nobody.
	store.createErr = ErrLimit
	if _, err := s.Create(ctx, "Seventh", nil); !errors.Is(err, ErrLimit) || len(added) != 2 {
		t.Errorf("Create when full = %v, listeners %v", err, added)
	}
}

func TestService_Update(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur", "Sam")
	s := NewService(store)

	var v *ValidationError
	if _, err := s.Update(ctx, 2, Patch{Name: ptr(" ")}); !errors.As(err, &v) || v.Field != "name" {
		t.Errorf("Update(blank name) = %v", err)
	}
	if _, err := s.Update(ctx, 2, Patch{Color: ptr("red")}); !errors.As(err, &v) || v.Field != "color" {
		t.Errorf("Update(red) = %v", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("invalid updates reached the store: %v", store.calls)
	}
	got, err := s.Update(ctx, 2, Patch{Name: ptr(" Samira "), Color: ptr("sand")})
	if err != nil || got.ID != 2 || got.Name != "Samira" || got.Color != "sand" {
		t.Errorf("Update = %+v, %v", got, err)
	}
	if got, err := s.Update(ctx, 2, Patch{}); err != nil || got.Name != "Samira" {
		t.Errorf("an empty patch = %+v, %v", got, err)
	}
	if _, err := s.Update(ctx, 9, Patch{Color: ptr("rose")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(9) = %v", err)
	}
	// A bad value is refused before the member is looked up (422 before 404).
	if _, err := s.Update(ctx, 9, Patch{Color: ptr("red")}); !errors.As(err, &v) || v.Field != "color" {
		t.Errorf("Update(9, red) = %v, want a color ValidationError", err)
	}
}

func TestService_Delete(t *testing.T) {
	t.Run("removes after the listeners, whatever the client does", func(t *testing.T) {
		store := newFakeStore("Teymur", "Sam")
		s := NewService(store)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var order []string
		s.OnRemoving(func(lctx context.Context, id int64) {
			cancel() // the client goes away mid-delete
			order = append(order, "trakt")
			if lctx.Err() != nil || id != 2 {
				t.Errorf("listener got %d with %v", id, lctx.Err())
			}
		})
		s.OnRemoving(func(_ context.Context, id int64) { order = append(order, "picks") })
		s.OnAdded(func(context.Context, int64) { order = append(order, "added") })

		if err := s.Delete(ctx, 2); err != nil {
			t.Fatal(err)
		}
		if want := []string{"trakt", "picks"}; !slices.Equal(order, want) || !slices.Equal(store.calls, []string{"delete"}) {
			t.Errorf("order = %v, store calls %v", order, store.calls)
		}
		if store.deleteCtx.Err() != nil {
			t.Errorf("the store's delete ran canceled: %v", store.deleteCtx.Err())
		}
		if _, err := s.Resolve(context.Background(), "2"); !errors.Is(err, ErrUnknown) {
			t.Errorf(`Resolve("2") after the delete = %v`, err)
		}
	})
	t.Run("checks first", func(t *testing.T) {
		store := newFakeStore("Teymur")
		s := NewService(store)
		told := 0
		s.OnRemoving(func(context.Context, int64) { told++ })
		if err := s.Delete(context.Background(), 1); !errors.Is(err, ErrLastMember) {
			t.Errorf("Delete(the last) = %v", err)
		}
		if err := s.Delete(context.Background(), 5); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete(5) = %v", err)
		}
		if told != 0 || len(store.calls) != 0 {
			t.Errorf("listeners told %d times, store calls %v", told, store.calls)
		}
	})
	t.Run("a row that stays is added back", func(t *testing.T) {
		for _, tt := range []struct {
			err   error
			added []int64
		}{
			{ErrLastMember, []int64{2}}, // another process removed the other member first
			{errors.New("db down"), []int64{2}},
			{ErrNotFound, nil}, // gone already: nothing to add back
		} {
			store := newFakeStore("Teymur", "Sam")
			store.deleteErr = tt.err
			s := NewService(store)
			var added []int64
			s.OnAdded(func(_ context.Context, id int64) { added = append(added, id) })
			if err := s.Delete(context.Background(), 2); !errors.Is(err, tt.err) || !slices.Equal(added, tt.added) {
				t.Errorf("%v: Delete = %v, added back %v, want %v", tt.err, err, added, tt.added)
			}
		}
	})
}

// A member being removed is gone for Resolve, Default and IDs while the
// listeners run (they take seconds), and comes back if the row stays.
func TestService_DeleteHidesTheMember(t *testing.T) {
	ctx := context.Background()
	for _, fails := range []bool{false, true} {
		store := newFakeStore("Teymur", "Sam")
		if fails {
			store.deleteErr = errors.New("db down")
		}
		s := NewService(store)
		var reads int
		s.OnRemoving(func(context.Context, int64) {
			reads = store.listCount()
			if _, err := s.Resolve(ctx, "1"); !errors.Is(err, ErrUnknown) {
				t.Errorf(`Resolve("1") while removing = %v, want ErrUnknown`, err)
			}
			if id, err := s.Default(ctx); err != nil || id != 2 {
				t.Errorf("Default while removing 1 = %d, %v, want 2", id, err)
			}
			if ids, err := s.IDs(ctx); err != nil || !slices.Equal(ids, []int64{2}) {
				t.Errorf("IDs while removing = %v, %v", ids, err)
			}
			if store.listCount() != reads {
				t.Errorf("the member being removed cost %d reads", store.listCount()-reads)
			}
		})
		// A member that stays is shown again only once it's added back.
		s.OnAdded(func(context.Context, int64) {
			if _, err := s.Resolve(ctx, "1"); !errors.Is(err, ErrUnknown) {
				t.Errorf(`Resolve("1") while adding it back = %v, want ErrUnknown`, err)
			}
		})
		err := s.Delete(ctx, 1)
		id, rerr := s.Resolve(ctx, "1")
		def, _ := s.Default(ctx)
		switch {
		case fails && (err == nil || rerr != nil || id != 1 || def != 1):
			t.Errorf("a failed delete = %v; then Resolve = %d, %v, Default %d, want member 1 back", err, id, rerr, def)
		case !fails && (err != nil || !errors.Is(rerr, ErrUnknown) || def != 2):
			t.Errorf("a delete = %v; then Resolve = %v, Default %d", err, rerr, def)
		}
	}
}

// A listener's panic (gin recovers it) leaves the member shown and the
// next change free to run.
func TestService_DeletePanics(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur", "Sam")
	s := NewService(store)
	s.OnRemoving(func(context.Context, int64) { panic("listener") })
	func() {
		defer func() { _ = recover() }()
		_ = s.Delete(ctx, 2)
	}()
	if id, err := s.Resolve(ctx, "2"); err != nil || id != 2 {
		t.Errorf(`Resolve("2") after the panic = %d, %v, want 2`, id, err)
	}
	if ids, err := s.IDs(ctx); err != nil || !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("IDs after the panic = %v, %v", ids, err)
	}
	if _, err := s.Create(ctx, "Ari", nil); err != nil {
		t.Errorf("Create after the panic = %v", err)
	}
}

// A read of the list that started before a delete never replaces the one
// read after it (GET /members racing DELETE).
func TestService_ReadsKeepTheirOrder(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur", "Sam")
	s := NewService(store)
	if err := s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	read, release := store.gateList()
	listed := make(chan struct{})
	go func() {
		defer close(listed)
		_, _ = s.List(ctx) // reads [1 2], then waits
	}()
	<-read
	deleted := make(chan error, 1)
	go func() { deleted <- s.Delete(ctx, 2) }()
	// The delete waits for the read in flight. Were it not to, it would
	// finish here and the read would then put member 2 back.
	select {
	case err := <-deleted:
		deleted <- err
	case <-time.After(100 * time.Millisecond):
	}
	release()
	<-listed
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, "2"); !errors.Is(err, ErrUnknown) {
		t.Errorf(`Resolve("2") after the delete = %v, want ErrUnknown`, err)
	}
	if ids, err := s.IDs(ctx); err != nil || !slices.Equal(ids, []int64{1}) {
		t.Errorf("IDs after the delete = %v, %v, want [1]", ids, err)
	}
}

// The OnAdded listeners hear of a new member even when the client went
// away.
func TestService_CreateTellsWithoutCancel(t *testing.T) {
	s := NewService(newFakeStore("Teymur"))
	got := errors.New("not told")
	s.OnAdded(func(ctx context.Context, _ int64) { got = ctx.Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Create(ctx, "Sam", nil); err != nil || got != nil {
		t.Errorf("Create = %v, the listener's context %v", err, got)
	}
}

func TestService_View(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore("Teymur", "Sam")
	s := NewService(store)

	list, err := s.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	raw, _ := json.Marshal(list[0])
	if want := `{"id":1,"name":"Teymur","color":"rose","created_at":"2026-10-22T08:00:00Z","trakt":{"state":"off","username":null}}`; string(raw) != want {
		t.Errorf("JSON = %s, want %s", raw, want)
	}

	s.SetTraktSummary(func(id int64) TraktSummary {
		if id == 1 {
			return TraktSummary{State: "connected", Username: ptr("teymur")}
		}
		return TraktSummary{State: "pending"}
	})
	list, _ = s.List(ctx)
	if list[0].Trakt.State != "connected" || *list[0].Trakt.Username != "teymur" || list[1].Trakt.State != "pending" {
		t.Errorf("summaries = %+v", list)
	}
}

// Requests resolve while members are added and removed (go test -race).
func TestService_Concurrent(t *testing.T) {
	ctx := context.Background()
	s := NewService(newFakeStore("Teymur"))
	s.SetTraktSummary(func(int64) TraktSummary { return TraktSummary{State: "off"} })
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if _, err := s.Resolve(ctx, ""); err != nil {
					t.Error(err)
					return
				}
				_, _ = s.Resolve(ctx, "2")
				_, _ = s.List(ctx)
			}
		}()
		if i%2 == 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m, err := s.Create(ctx, "Person "+string(rune('A'+i)), nil)
				if err == nil {
					_ = s.Delete(ctx, m.ID)
				}
			}()
		}
	}
	wg.Wait()
}

func ptr[T any](v T) *T { return &v }
