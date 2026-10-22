package trakt

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/secret"
	"github.com/shapedtime/momoshtrem/internal/trakt/trakttest"
)

// The household's Trakt (#244): a Registry over a real database, one
// fake Trakt holding every member's account. Its clock moves only when a
// test says (manualClock), so the schedules never fire on their own.

// regEnv is a household of members on the fake Trakt.
type regEnv struct {
	t     *testing.T
	ctx   context.Context
	clock *manualClock
	srv   *trakttest.Server
	db    *library.DB
	box   *secret.Box
	ids   []int64 // every member's, by id: 1 is seeded
	reg   *Registry

	mu       sync.Mutex
	imported []int64 // OnImported's members, in order
	synced   atomic.Int32
}

// newRegEnv is a household of n members, its registry built (start with
// start). wrap, when set, wraps each member's sync store.
func newRegEnv(t *testing.T, n int, wrap func(id int64, s SyncStore) SyncStore) *regEnv {
	t.Helper()
	db := openSchemaDB(t)
	e := &regEnv{t: t, ctx: context.Background(), clock: newManualClock(), srv: trakttest.New(t), db: db,
		box: mustBox(t, testKey), ids: []int64{1}}
	e.srv.SetClock(e.clock.Now)
	members := library.NewMemberRepository(db)
	for i := 2; i <= n; i++ {
		m, err := members.Create(e.ctx, fmt.Sprintf("Member %d", i), "")
		if err != nil {
			t.Fatal(err)
		}
		e.ids = append(e.ids, m.ID)
	}
	auths, syncs := library.NewTraktAuthRepository(db), library.NewTraktSyncStore(db)
	e.reg = NewRegistry(RegistryDeps{
		Client: newTestClient(e.srv, e.clock), Box: e.box, Clock: e.clock,
		Auths: func(id int64) Store { return auths.ForMember(id) },
		Syncs: func(id int64) SyncStore {
			if wrap != nil {
				return wrap(id, syncs.ForMember(id))
			}
			return syncs.ForMember(id)
		},
		Settings: func(id int64) Settings { return members.TraktSettingsFor(id) },
	})
	e.reg.SetOnImported(func(_ context.Context, id int64) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.imported = append(e.imported, id)
	})
	e.reg.SetOnSynced(func() { e.synced.Add(1) })
	t.Cleanup(e.reg.Stop)
	(&syncEnv{t: t, srv: e.srv}).serveAccount()
	return e
}

// start starts the registry with every member, and waits for their
// schedules to wait.
func (e *regEnv) start() {
	e.t.Helper()
	e.reg.Start(e.ctx, e.ids)
	waitFor(e.t, "the schedules to wait", func() bool { return e.clock.sleeping() == len(e.ids) })
}

func (e *regEnv) as(id int64) context.Context { return member.WithID(e.ctx, id) }

// connect connects member id to the account slug through the device code:
// the code polled once (the clock moved by its interval), approved.
func (e *regEnv) connect(id int64, username, slug string) Integration {
	e.t.Helper()
	e.srv.SetUser(username, slug)
	sleeping := e.clock.sleeping()
	code, err := e.reg.Connect(e.as(id))
	if err != nil {
		e.t.Fatalf("member %d's Connect: %v", id, err)
	}
	waitFor(e.t, "the code's poll to wait", func() bool { return e.clock.sleeping() == sleeping+1 })
	e.clock.Advance(time.Duration(code.Interval) * time.Second)
	waitFor(e.t, "the code to end", func() bool { return e.integration(id).State != StatePending })
	return e.integration(id)
}

func (e *regEnv) integration(id int64) Integration {
	e.t.Helper()
	in, err := e.reg.Integration(e.as(id))
	if err != nil {
		e.t.Fatalf("member %d's Integration: %v", id, err)
	}
	return in
}

// syncer is member id's sync.
func (e *regEnv) syncer(id int64) *Syncer {
	e.t.Helper()
	m := e.reg.get(id)
	if m == nil {
		e.t.Fatalf("the registry doesn't hold member %d", id)
	}
	return m.sync
}

// waitSyncs waits until member id's syncs have started runs times in all
// and none runs.
func (e *regEnv) waitSyncs(id int64, runs int) {
	e.t.Helper()
	s := e.syncer(id)
	waitFor(e.t, fmt.Sprintf("member %d's sync", id), func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.seq >= runs && !s.running
	})
}

func (e *regEnv) seq(id int64) int {
	s := e.syncer(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// rows are query's rows, each its one column.
func (e *regEnv) rows(query string, args ...any) []string {
	e.t.Helper()
	rows, err := e.db.Query(query, args...)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// memberRows counts member id's rows of every Trakt table.
func (e *regEnv) memberRows(id int64) int {
	e.t.Helper()
	n := 0
	for _, table := range []string{"trakt_history", "trakt_watched_movies", "trakt_watched_episodes", "trakt_ratings",
		"trakt_watchlist", "trakt_show_progress", "trakt_hidden", "trakt_calendar", "trakt_sync_state", "trakt_auth"} {
		var c int
		if err := e.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE member_id = $1`, id).Scan(&c); err != nil {
			e.t.Fatal(err)
		}
		n += c
	}
	return n
}

// serveTwoAccounts gives ann and bob their own movie history.
func (e *regEnv) serveTwoAccounts() {
	e.srv.ListFor("ann", "/sync/history/movies", []obj{moviePlay(1001, "2026-10-14T21:00:00.000Z", matrix)})
	e.srv.ListFor("ann", "/sync/history/episodes", []obj{})
	e.srv.ListFor("bob", "/sync/history/movies", []obj{
		moviePlay(3002, "2026-10-15T21:00:00.000Z", dune), moviePlay(3001, "2026-10-12T21:00:00.000Z", dune)})
	e.srv.ListFor("bob", "/sync/history/episodes", []obj{})
}

// Two members connect their own accounts and each sync imports only
// theirs, into their own rows, telling the hooks whose import it was.
func TestRegistry_MembersConnectAndSyncInIsolation(t *testing.T) {
	e := newRegEnv(t, 2, nil)
	e.serveTwoAccounts()
	e.start()
	ann, bob := e.ids[0], e.ids[1]

	if in := e.connect(ann, "Ann", "ann"); !in.Connected || ptrValue(in.Username) != "Ann" {
		t.Fatalf("Ann's connection = %+v", in)
	}
	e.waitSyncs(ann, 1)
	if in := e.integration(bob); in.State != StateOff || in.Username != nil || in.Counts.History != 0 {
		t.Errorf("Bob's integration after Ann connected = %+v", in)
	}
	if in := e.connect(bob, "Bob", "bob"); !in.Connected || ptrValue(in.Username) != "Bob" {
		t.Fatalf("Bob's connection = %+v", in)
	}
	e.waitSyncs(bob, 1)

	if got, want := e.rows(`SELECT concat_ws(' ', member_id, id, tmdb_id) FROM trakt_history ORDER BY member_id, id`),
		[]string{fmt.Sprintf("%d 1001 603", ann), fmt.Sprintf("%d 3001 693134", bob), fmt.Sprintf("%d 3002 693134", bob)}; !reflect.DeepEqual(got, want) {
		t.Errorf("history = %q, want %q", got, want)
	}
	if got, want := e.rows(`SELECT concat_ws(' ', member_id, username) FROM trakt_auth ORDER BY member_id`),
		[]string{fmt.Sprintf("%d Ann", ann), fmt.Sprintf("%d Bob", bob)}; !reflect.DeepEqual(got, want) {
		t.Errorf("connections = %q, want %q", got, want)
	}
	if a, b := e.integration(ann), e.integration(bob); a.Counts.History != 1 || b.Counts.History != 2 ||
		a.Sync.Status != SyncIdle || b.Sync.Status != SyncIdle || a.LastSyncAt == nil || b.LastSyncAt == nil {
		t.Errorf("Ann's integration %+v, Bob's %+v", a, b)
	}
	e.mu.Lock()
	imported := slices.Clone(e.imported)
	e.mu.Unlock()
	if !slices.Equal(imported, []int64{ann, bob}) || e.synced.Load() != 2 {
		t.Errorf("OnImported for %v, OnSynced %d times; want [%d %d] and twice", imported, e.synced.Load(), ann, bob)
	}

	// /members' summary, /status's connected and the picks' connection.
	for _, id := range []int64{ann, bob} {
		name := map[int64]string{ann: "Ann", bob: "Bob"}[id]
		if s := e.reg.Summary(id); s.State != "connected" || ptrValue(s.Username) != name {
			t.Errorf("member %d's summary = %+v", id, s)
		}
		if !e.reg.ConnectedFor(e.as(id)) || !e.reg.Connection(id).Connected() {
			t.Errorf("member %d isn't connected", id)
		}
	}
	if s := e.reg.Summary(99); s.State != "off" || s.Username != nil {
		t.Errorf("an unknown member's summary = %+v", s)
	}
	if e.reg.ConnectedFor(e.ctx) || e.reg.ConnectedFor(e.as(99)) {
		t.Error("connected without a member, or for an unknown one")
	}

	// One member's disconnect is theirs only.
	if err := e.reg.Disconnect(e.as(bob)); err != nil {
		t.Fatal(err)
	}
	if e.memberRows(bob) != 0 || e.memberRows(ann) == 0 || !e.reg.Connected(ann) || e.reg.Connection(bob).Connected() {
		t.Errorf("after Bob's disconnect: Bob's rows %d, Ann's %d, Ann connected %v", e.memberRows(bob), e.memberRows(ann), e.reg.Connected(ann))
	}
}

// An account connects to one member: approving a code for an account
// another member has connected ends the flow with account_in_use, nothing
// stored nor revoked, and the other member's connection keeps working.
func TestRegistry_AccountInUse(t *testing.T) {
	e := newRegEnv(t, 2, nil)
	e.start()
	ann, other := e.ids[0], e.ids[1]
	e.connect(ann, "Ann", "ann")
	e.waitSyncs(ann, 1)
	annToken := e.srv.AccessToken()

	in := e.connect(other, "Ann", "ann")
	if in.State != StateOff || in.Username != nil || ptrValue(in.ConnectError) != ConnectAccountInUse || in.Pending != nil {
		t.Errorf("the second member's integration = %+v, want off with account_in_use", in)
	}
	if got := e.srv.Revoked(); len(got) != 0 {
		t.Errorf("revoked %v, want nothing", got)
	}
	if got := e.rows(`SELECT member_id::text FROM trakt_auth`); !reflect.DeepEqual(got, []string{fmt.Sprint(ann)}) {
		t.Errorf("connections of members %q, want only Ann's", got)
	}
	var settings UserSettings
	if _, err := e.reg.get(ann).auth.Get(e.ctx, "/users/settings", nil, &settings); err != nil || settings.User.IDs.Slug != "ann" {
		t.Errorf("Ann's connection after the refused code = %+v, %v", settings, err)
	}
	if tok, err := e.reg.get(ann).auth.Token(e.ctx); err != nil || tok.Access != annToken {
		t.Errorf("Ann's token = %+v, %v; want %q", tok, err, annToken)
	}

	// Another account connects, and the next code clears the error.
	if in := e.connect(other, "Bob", "bob"); !in.Connected || in.ConnectError != nil || ptrValue(in.Username) != "Bob" {
		t.Errorf("connecting another account = %+v", in)
	}
}

// Forget lets go of a member in any state: connected, it stops the sync,
// purges their rows and revokes the token; with a code pending, it
// cancels it. Their requests are unknown from then on.
func TestRegistry_Forget(t *testing.T) {
	e := newRegEnv(t, 3, nil)
	e.serveTwoAccounts()
	e.start()
	ann, bob, pending := e.ids[0], e.ids[1], e.ids[2]
	e.connect(ann, "Ann", "ann")
	e.waitSyncs(ann, 1)
	e.connect(bob, "Bob", "bob")
	e.waitSyncs(bob, 1)
	bobToken := e.srv.AccessToken()
	bobSync := e.syncer(bob)

	sleeping := e.clock.sleeping()
	if _, err := e.reg.Connect(e.as(pending)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the code's poll to wait", func() bool { return e.clock.sleeping() == sleeping+1 })
	polls := e.srv.Count("/oauth/device/token")

	e.reg.Forget(e.ctx, bob)
	if got := e.srv.Revoked(); !slices.Equal(got, []string{bobToken}) {
		t.Errorf("revoked %v, want Bob's %q", got, bobToken)
	}
	if n := e.memberRows(bob); n != 0 {
		t.Errorf("Bob has %d Trakt rows left", n)
	}
	if e.memberRows(ann) == 0 || !e.reg.Connected(ann) {
		t.Error("Forget touched Ann's Trakt")
	}
	bobSync.mu.Lock()
	stopped := bobSync.stopped
	bobSync.mu.Unlock()
	if !stopped {
		t.Error("Bob's sync still runs")
	}
	if _, err := e.reg.Integration(e.as(bob)); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("Bob's Integration = %v, want member.ErrUnknown", err)
	}
	if e.reg.Summary(bob).State != "off" || e.reg.Connected(bob) {
		t.Error("Bob still shows connected")
	}

	e.reg.Forget(e.ctx, pending)
	e.clock.Advance(time.Minute / 2) // past the code's interval: nobody polls it
	if n := e.srv.Count("/oauth/device/token"); n != polls {
		t.Errorf("the forgotten code was polled %d more times", n-polls)
	}
	if got := e.srv.Revoked(); len(got) != 1 {
		t.Errorf("revoked %v after forgetting a pending code", got)
	}
	if _, err := e.reg.Connect(e.as(pending)); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("a forgotten member's Connect = %v", err)
	}
	e.reg.Forget(e.ctx, pending) // again: nothing

	// A member back (a delete that failed after the listeners) starts off.
	e.reg.Add(e.ctx, bob)
	if in := e.integration(bob); in.State != StateOff || in.Counts != (library.TraktCounts{}) {
		t.Errorf("Bob back = %+v", in)
	}
	if in := e.connect(bob, "Bob", "bob"); !in.Connected {
		t.Errorf("Bob reconnecting = %+v", in)
	}
}

// Removing a member while their sync is in the middle of a write: the
// registry's OnRemoving stops the sync and purges before the member's row
// goes, so the delete's cascade never meets the write (which holds the
// connection's row, then needs the member's for the FK: a deadlock
// Postgres would end by aborting one of them).
func TestRegistry_MemberDeletedMidWrite(t *testing.T) {
	e := newRegEnv(t, 2, nil)
	e.serveTwoAccounts()
	e.start()
	members := member.NewService(library.NewMemberRepository(e.db))
	members.OnAdded(e.reg.Add)
	members.OnRemoving(e.reg.Forget)
	ann, bob := e.ids[0], e.ids[1]
	e.connect(ann, "Ann", "ann")
	e.waitSyncs(ann, 1)

	// Bob's import writes its history while the test holds a lock the
	// write waits for, inside its transaction.
	for _, q := range []string{
		`CREATE FUNCTION hold_history() RETURNS trigger LANGUAGE plpgsql AS $$
		 BEGIN PERFORM pg_advisory_xact_lock(4244); RETURN NEW; END $$`,
		`CREATE TRIGGER hold_history BEFORE INSERT ON trakt_history FOR EACH ROW EXECUTE FUNCTION hold_history()`,
	} {
		if _, err := e.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	lock, err := e.db.Conn(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(e.ctx, `SELECT pg_advisory_lock(4244)`); err != nil {
		t.Fatal(err)
	}
	if in := e.connect(bob, "Bob", "bob"); !in.Connected {
		t.Fatalf("Bob's connection = %+v", in)
	}
	waitFor(t, "Bob's history write to wait", func() bool {
		var n int
		err := e.db.QueryRow(`SELECT COUNT(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&n)
		return err == nil && n == 1
	})

	deleted := make(chan error, 1)
	go func() { deleted <- members.Delete(e.ctx, bob) }()
	time.Sleep(300 * time.Millisecond) // the delete meets the write still waiting
	if _, err := lock.ExecContext(e.ctx, `SELECT pg_advisory_unlock(4244)`); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-deleted:
		if err != nil {
			t.Fatalf("Delete = %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the delete never finished")
	}

	if ids, err := members.IDs(e.ctx); err != nil || !slices.Equal(ids, []int64{ann}) {
		t.Errorf("members %v, %v; want only Ann", ids, err)
	}
	if n := e.memberRows(bob); n != 0 {
		t.Errorf("Bob has %d Trakt rows left", n)
	}
	if e.memberRows(ann) == 0 || !e.reg.Connected(ann) || e.reg.get(bob) != nil {
		t.Error("Ann's Trakt was touched, or Bob's is still held")
	}
}

// Nudge (GET /me/up-next) syncs the request's member only.
func TestRegistry_NudgeIsPerMember(t *testing.T) {
	e := newRegEnv(t, 2, nil)
	e.serveTwoAccounts()
	e.start()
	ann, bob := e.ids[0], e.ids[1]
	e.connect(ann, "Ann", "ann")
	e.waitSyncs(ann, 1)
	e.connect(bob, "Bob", "bob")
	e.waitSyncs(bob, 1)

	// Both last checked long ago.
	for _, id := range e.ids {
		s := e.syncer(id)
		s.mu.Lock()
		s.lastCheck = time.Time{}
		s.mu.Unlock()
	}
	e.reg.Nudge(e.as(bob))
	e.waitSyncs(bob, 2)
	time.Sleep(20 * time.Millisecond)
	if e.seq(ann) != 1 || e.seq(bob) != 2 {
		t.Errorf("syncs: Ann %d, Bob %d; want 1 and 2", e.seq(ann), e.seq(bob))
	}
	e.reg.Nudge(e.ctx)     // no member: nothing
	e.reg.Nudge(e.as(99))  // an unknown one: nothing
	e.reg.Nudge(e.as(bob)) // checked just now: nothing
	time.Sleep(20 * time.Millisecond)
	if e.seq(ann) != 1 || e.seq(bob) != 2 {
		t.Errorf("after the other nudges: Ann %d, Bob %d", e.seq(ann), e.seq(bob))
	}
}

// The members' first checks are staggered at startup: a minute after it,
// then 2 minutes apart. A member added later checks a minute after.
func TestRegistry_StaggeredFirstChecks(t *testing.T) {
	e := newRegEnv(t, 3, nil)
	start := e.clock.Now()
	e.start()
	for i, id := range e.ids {
		s := e.syncer(id)
		s.mu.Lock()
		next := s.nextAt
		s.mu.Unlock()
		if want := start.Add(time.Minute + time.Duration(i)*2*time.Minute); !next.Equal(want) {
			t.Errorf("member %d's first check at %v, want %v", id, next.Sub(start), want.Sub(start))
		}
	}

	m, err := library.NewMemberRepository(e.db).Create(e.ctx, "Late", "")
	if err != nil {
		t.Fatal(err)
	}
	e.reg.Add(e.ctx, m.ID)
	waitFor(t, "the new schedule to wait", func() bool { return e.clock.sleeping() == len(e.ids)+1 })
	s := e.syncer(m.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nextAt.Equal(start.Add(time.Minute)) {
		t.Errorf("the added member's first check at %v", s.nextAt.Sub(start))
	}
}

// stuckStore is a sync store whose State, once its context ends, waits
// until every member's has ended too: a Stop that stopped the members one
// after another would wait for each in turn.
type stuckStore struct {
	SyncStore
	entered chan<- int64
	id      int64
	ended   *atomic.Int32
	members int32
}

func (s stuckStore) State(ctx context.Context) (library.TraktSyncState, error) {
	if ctx.Err() == nil {
		s.entered <- s.id
	}
	<-ctx.Done()
	s.ended.Add(1)
	deadline := time.Now().Add(30 * time.Second)
	for s.ended.Load() < s.members && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	return library.TraktSyncState{}, ctx.Err()
}

// Stop stops every member's sync at once, so shutdown waits for the
// slowest member, not their sum.
func TestRegistry_StopIsConcurrent(t *testing.T) {
	const members = 3
	entered, ended := make(chan int64, members), &atomic.Int32{}
	e := newRegEnv(t, members, func(id int64, s SyncStore) SyncStore {
		return stuckStore{SyncStore: s, entered: entered, id: id, ended: ended, members: members}
	})
	// Every member connected, so each first check starts a sync.
	for _, id := range e.ids {
		access, refresh := e.srv.Grant()
		created := e.clock.Now().UTC().Truncate(time.Second)
		if _, err := library.NewTraktAuthRepository(e.db).ForMember(id).Connect(e.ctx, library.TraktAuth{
			Username: fmt.Sprintf("user%d", id), UserSlug: fmt.Sprintf("user%d", id),
			AccessTokenEnc: e.box.Seal([]byte(access), aadAccessToken), RefreshTokenEnc: e.box.Seal([]byte(refresh), aadRefreshToken),
			TokenCreatedAt: created, ExpiresAt: created.Add(7 * 24 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	e.start()
	e.clock.Advance(10 * time.Minute) // every first check is due
	for range members {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("not every member's check started")
		}
	}

	began := time.Now()
	e.reg.Stop()
	if took := time.Since(began); took > 5*time.Second {
		t.Errorf("Stop took %v: the members stopped one after another", took)
	}
	if got := ended.Load(); got != members {
		t.Errorf("%d of %d checks ended", got, members)
	}
	e.reg.Add(e.ctx, 99) // stopped: nothing new starts
	if e.reg.get(99) != nil {
		t.Error("Add after Stop added a member")
	}
}
