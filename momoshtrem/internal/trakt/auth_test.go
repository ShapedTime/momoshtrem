package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/secret"
	"github.com/shapedtime/momoshtrem/internal/trakt/trakttest"
)

const testKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// openSchemaDB opens the test database in a private, freshly migrated
// schema: trakt_auth is one row, so tests can't share it.
func openSchemaDB(t *testing.T) *library.DB {
	t.Helper()
	dsn := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	base, err := library.NewDB(dsn)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { base.Close() })
	schema := fmt.Sprintf("trakt_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := library.NewDB(dsn + sep + "search_path=" + schema)
	if err != nil {
		t.Fatalf("NewDB in %s: %v", schema, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type env struct {
	t         *testing.T
	ctx       context.Context
	srv       *trakttest.Server
	clock     *fakeClock
	db        *library.DB
	store     *library.TraktAuthRepository
	settings  *library.SettingsRepository
	box       *secret.Box
	auth      *Auth
	connected atomic.Int32 // OnConnected calls
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := openSchemaDB(t)
	e := &env{
		t: t, ctx: context.Background(), srv: trakttest.New(t), clock: newFakeClock(), db: db,
		store: library.NewTraktAuthRepository(db), settings: library.NewSettingsRepository(db), box: mustBox(t, testKey),
	}
	e.srv.SetClock(e.clock.Now)
	e.auth = e.restart(e.box)
	return e
}

// restart is a new Auth over the same database, as after a restart.
func (e *env) restart(box *secret.Box) *Auth {
	a := NewAuth(AuthDeps{Client: newTestClient(e.srv, e.clock), Box: box, Store: e.store, Settings: e.settings, Clock: e.clock})
	a.SetHooks(Hooks{OnConnected: func() { e.connected.Add(1) }})
	e.t.Cleanup(a.Close)
	return a
}

func (e *env) integration(a *Auth) Integration {
	e.t.Helper()
	in, err := a.Integration(e.ctx)
	if err != nil {
		e.t.Fatalf("Integration: %v", err)
	}
	return in
}

// connect runs the device flow to connected.
func (e *env) connect(a *Auth) {
	e.t.Helper()
	if _, err := a.Connect(e.ctx); err != nil {
		e.t.Fatalf("Connect: %v", err)
	}
	waitFor(e.t, "connected", func() bool { return a.Connected() })
}

func (e *env) storedTokens() (access, refresh string) {
	e.t.Helper()
	row, err := e.store.Get(e.ctx)
	if err != nil || row == nil {
		e.t.Fatalf("stored connection = %+v, %v", row, err)
	}
	a, err1 := e.box.Open(row.AccessTokenEnc, aadAccessToken)
	r, err2 := e.box.Open(row.RefreshTokenEnc, aadRefreshToken)
	if err1 != nil || err2 != nil {
		e.t.Fatalf("stored tokens don't open: %v %v", err1, err2)
	}
	return string(a), string(r)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func mustBox(t *testing.T, key string) *secret.Box {
	t.Helper()
	box, err := secret.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestAuth_DeviceFlowOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		answers   []int
		expiresIn int // seconds; 0 keeps 600
		wantState State
		wantError ConnectError
		wantPolls int
	}{
		{"approved", []int{400, 400, 200}, 0, StateConnected, "", 3},
		{"expired on Trakt", []int{400, 410}, 0, StateOff, ConnectExpired, 2},
		{"expired here", []int{400}, 30, StateOff, ConnectExpired, 5},
		{"denied", []int{418}, 0, StateOff, ConnectDenied, 1},
		{"an unknown code", []int{404}, 0, StateOff, ConnectFailed, 1},
		{"a used code", []int{409}, 0, StateOff, ConnectFailed, 1},
		{"Trakt down for a while", []int{503, 502, 200}, 0, StateConnected, "", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.srv.SetDeviceAnswers(tt.answers...)
			if tt.expiresIn > 0 {
				e.srv.SetDeviceCode(5, tt.expiresIn)
			}

			code, err := e.auth.Connect(e.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if code.UserCode != trakttest.UserCode || code.VerificationURL != trakttest.VerificationURL || code.Interval != 5 {
				t.Errorf("code = %+v", code)
			}
			waitFor(t, "the flow to end", func() bool { return e.integration(e.auth).State != StatePending })

			in := e.integration(e.auth)
			if in.State != tt.wantState || ptrValue(in.ConnectError) != tt.wantError || in.Pending != nil {
				t.Errorf("state %s, connect_error %v, pending %v; want %s and %q", in.State, ptrValue(in.ConnectError), in.Pending, tt.wantState, tt.wantError)
			}
			if n := e.srv.Count("/oauth/device/token"); n != tt.wantPolls {
				t.Errorf("polls = %d, want %d", n, tt.wantPolls)
			}
			if tt.wantState == StateConnected {
				if ptrValue(in.Username) != "Teymur" || !in.Connected || e.connected.Load() != 1 {
					t.Errorf("username %v, connected %v, OnConnected %d", ptrValue(in.Username), in.Connected, e.connected.Load())
				}
			} else if in.Username != nil || e.connected.Load() != 0 {
				t.Errorf("username %v, OnConnected %d after a code that didn't connect", ptrValue(in.Username), e.connected.Load())
			}
		})
	}
}

func TestAuth_SlowDown(t *testing.T) {
	e := newEnv(t)
	e.srv.SetDeviceAnswers(429, 429, 200)
	e.connect(e.auth)
	if got, want := e.clock.Sleeps(), []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("waits = %v, want %v (each 429 adds 5 s)", got, want)
	}
}

func TestAuth_Connect(t *testing.T) {
	t.Run("the live code comes back", func(t *testing.T) {
		e := newEnv(t)
		e.clock.Block(true) // the poller never polls
		first, err := e.auth.Connect(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		again, err := e.auth.Connect(e.ctx)
		if err != nil || again != first || e.srv.Count("/oauth/device/code") != 1 {
			t.Errorf("second connect = %+v, %v after %d device codes", again, err, e.srv.Count("/oauth/device/code"))
		}
		in := e.integration(e.auth)
		if in.State != StatePending || in.Pending == nil || *in.Pending != first || in.Username != nil {
			t.Errorf("pending integration = %+v", in)
		}
		if !first.ExpiresAt.Equal(e.clock.Now().Add(10 * time.Minute)) {
			t.Errorf("expires_at = %v", first.ExpiresAt)
		}

		// DELETE while pending cancels the code only.
		if err := e.auth.Disconnect(e.ctx); err != nil {
			t.Fatal(err)
		}
		if in := e.integration(e.auth); in.State != StateOff || in.Pending != nil || in.ConnectError != nil {
			t.Errorf("after the cancel = %+v", in)
		}
	})
	t.Run("409 while connected", func(t *testing.T) {
		e := newEnv(t)
		e.connect(e.auth)
		if _, err := e.auth.Connect(e.ctx); !errors.Is(err, ErrConnected) {
			t.Errorf("Connect = %v, want ErrConnected", err)
		}
	})
	t.Run("Trakt's device-code endpoint fails", func(t *testing.T) {
		e := newEnv(t)
		for range 4 {
			e.srv.Queue("/oauth/device/code", trakttest.Reply{Status: 503})
		}
		if _, err := e.auth.Connect(e.ctx); !errors.Is(err, ErrUnavailable) {
			t.Errorf("Connect = %v, want ErrUnavailable", err)
		}
		if in := e.integration(e.auth); in.State != StateOff {
			t.Errorf("state = %s", in.State)
		}
	})
	t.Run("not configured", func(t *testing.T) {
		e := newEnv(t)
		e.connect(e.auth)
		a := NewAuth(AuthDeps{Client: newTestClient(e.srv, e.clock), Store: e.store, Settings: e.settings}) // no SECRET_KEY
		if _, err := a.Connect(e.ctx); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Connect = %v", err)
		}
		if _, err := a.Token(e.ctx); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Token = %v", err)
		}
		if in := e.integration(a); in.Configured || in.State != StateOff || in.Username != nil || a.Connected() {
			t.Errorf("unconfigured integration = %+v", in)
		}
	})
}

// The database holds ciphertexts; a restart with the key reads them, one
// with another key is reauth (and never crashes).
func TestAuth_TokensAtRest(t *testing.T) {
	e := newEnv(t)
	e.connect(e.auth)
	access := e.srv.AccessToken()

	var accessEnc, refreshEnc []byte
	if err := e.db.QueryRow(`SELECT access_token_enc, refresh_token_enc FROM trakt_auth`).Scan(&accessEnc, &refreshEnc); err != nil {
		t.Fatal(err)
	}
	for _, enc := range [][]byte{accessEnc, refreshEnc} {
		if bytes.Contains(enc, []byte("access-")) || bytes.Contains(enc, []byte("refresh-")) {
			t.Errorf("the stored bytes hold a token: %q", enc)
		}
	}

	restarted := e.restart(e.box)
	if err := restarted.Load(e.ctx); err != nil {
		t.Fatal(err)
	}
	tok, err := restarted.Token(e.ctx)
	if err != nil || tok.Access != access || !restarted.Connected() {
		t.Errorf("after a restart Token = %+v, %v; want %q", tok, err, access)
	}

	otherKey := e.restart(mustBox(t, strings.Repeat("another key, 32 characters long!", 1)))
	in := e.integration(otherKey)
	if in.State != StateReauth || ptrValue(in.ReauthReason) != reasonUnreadable || ptrValue(in.Username) != "Teymur" {
		t.Errorf("with another SECRET_KEY = %+v", in)
	}
	if _, err := otherKey.Token(e.ctx); !errors.Is(err, ErrReauth) {
		t.Errorf("Token with another SECRET_KEY = %v", err)
	}
	// The data stays for the right key.
	if row, _ := e.store.Get(e.ctx); row == nil || row.NeedsReauth {
		t.Errorf("another key changed the stored connection: %+v", row)
	}
}

func TestAuth_RefreshIsSingleFlightAndRotates(t *testing.T) {
	e := newEnv(t)
	e.connect(e.auth)
	oldAccess, oldRefresh := e.storedTokens()

	// A 7-day token is refreshed once less than 24 h is left.
	e.clock.Advance(6*24*time.Hour - time.Minute)
	if tok, _ := e.auth.Token(e.ctx); tok.Access != oldAccess || e.srv.Count("/oauth/token") != 0 {
		t.Fatalf("refreshed %d times with over a day left", e.srv.Count("/oauth/token"))
	}
	e.clock.Advance(2 * time.Minute)

	var wg sync.WaitGroup
	got := make([]string, 10)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := e.auth.Token(e.ctx)
			if err != nil {
				t.Error(err)
			}
			got[i] = tok.Access
		}()
	}
	wg.Wait()
	if n := e.srv.Count("/oauth/token"); n != 1 {
		t.Fatalf("refreshes = %d, want 1", n)
	}
	newAccess, newRefresh := e.storedTokens()
	for _, a := range got {
		if a != newAccess || a == oldAccess {
			t.Fatalf("tokens = %v, want all %q (stored)", got, newAccess)
		}
	}
	if newAccess != e.srv.AccessToken() || newRefresh == oldRefresh {
		t.Errorf("stored %q / %q, Trakt's newest %q", newAccess, newRefresh, e.srv.AccessToken())
	}
	// The old refresh token is spent.
	if _, err := newTestClient(e.srv, e.clock).RefreshToken(e.ctx, oldRefresh); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("reusing the old refresh token = %v", err)
	}
}

func TestAuth_FailedRefreshMeansReauth(t *testing.T) {
	e := newEnv(t)
	e.connect(e.auth)
	e.srv.RevokeAll() // the user revoked the app on trakt.tv
	e.clock.Advance(6*24*time.Hour + time.Hour)

	if _, err := e.auth.Token(e.ctx); !errors.Is(err, ErrReauth) {
		t.Fatalf("Token = %v, want ErrReauth", err)
	}
	in := e.integration(e.auth)
	if in.State != StateReauth || ptrValue(in.ReauthReason) != reasonRejected || ptrValue(in.Username) != "Teymur" || in.Connected {
		t.Errorf("integration = %+v", in)
	}
	if row, _ := e.store.Get(e.ctx); row == nil || !row.NeedsReauth || row.ReauthReason != reasonRejected {
		t.Errorf("stored = %+v", row)
	}
	if in := e.integration(e.restart(e.box)); in.State != StateReauth {
		t.Errorf("after a restart state = %s", in.State)
	}

	// Connecting again goes through pending, keeping the account.
	e.clock.Block(true)
	if _, err := e.auth.Connect(e.ctx); err != nil {
		t.Fatal(err)
	}
	if in := e.integration(e.auth); in.State != StatePending || ptrValue(in.Username) != "Teymur" {
		t.Errorf("pending from reauth = %+v", in)
	}
	if _, err := e.auth.Token(e.ctx); !errors.Is(err, ErrReauth) {
		t.Errorf("Token while pending from reauth = %v", err)
	}
	if err := e.auth.Disconnect(e.ctx); err != nil { // cancels the code: back to reauth
		t.Fatal(err)
	}
	if in := e.integration(e.auth); in.State != StateReauth {
		t.Errorf("after cancelling the code state = %s", in.State)
	}
	e.clock.Block(false)
	e.connect(e.auth)
	if in := e.integration(e.auth); in.ReauthReason != nil || ptrValue(in.Username) != "Teymur" {
		t.Errorf("reconnected = %+v", in)
	}
}

// Trakt unreachable: the old token serves while it lasts, and that's not
// reauth.
func TestAuth_RefreshNetworkErrorKeepsToken(t *testing.T) {
	e := newEnv(t)
	e.connect(e.auth)
	old, _ := e.storedTokens()
	e.clock.Advance(6*24*time.Hour + time.Hour)
	for range 4 {
		e.srv.Queue("/oauth/token", trakttest.Reply{Status: 503})
	}
	if tok, err := e.auth.Token(e.ctx); err != nil || tok.Access != old {
		t.Fatalf("Token = %+v, %v; want the old token", tok, err)
	}

	e.clock.Advance(24 * time.Hour) // expired
	for range 4 {
		e.srv.Queue("/oauth/token", trakttest.Reply{Status: 503})
	}
	if _, err := e.auth.Token(e.ctx); err == nil || errors.Is(err, ErrReauth) {
		t.Fatalf("Token with an expired token = %v, want a plain error", err)
	}
	if !e.auth.Connected() {
		t.Error("a network error ended the connection")
	}
	if tok, err := e.auth.Token(e.ctx); err != nil || tok.Access == old {
		t.Errorf("once Trakt answers Token = %+v, %v", tok, err)
	}
}

func TestAuth_Invalidate(t *testing.T) {
	e := newEnv(t)
	e.connect(e.auth)
	first, err := e.auth.Token(e.ctx)
	if err != nil {
		t.Fatal(err)
	}

	// A 401: refreshed at once, and the token isn't handed out again.
	if err := e.auth.Invalidate(e.ctx, first); err != nil {
		t.Fatal(err)
	}
	second, _ := e.auth.Token(e.ctx)
	if second.Access == first.Access || e.srv.Count("/oauth/token") != 1 {
		t.Fatalf("after a 401 Token = %q (%d refreshes)", second.Access, e.srv.Count("/oauth/token"))
	}
	// A 401 for a token already replaced changes nothing.
	if err := e.auth.Invalidate(e.ctx, first); err != nil || e.srv.Count("/oauth/token") != 1 {
		t.Errorf("Invalidate of a replaced token = %v (%d refreshes)", err, e.srv.Count("/oauth/token"))
	}
	// A second 401 is reauth.
	if err := e.auth.Invalidate(e.ctx, second); !errors.Is(err, ErrReauth) {
		t.Errorf("a second 401 = %v, want ErrReauth", err)
	}
	if _, err := e.auth.Token(e.ctx); !errors.Is(err, ErrReauth) || e.srv.Count("/oauth/token") != 1 {
		t.Errorf("Token after the second 401 = %v", err)
	}
}

func TestAuth_Get(t *testing.T) {
	t.Run("a 401 is retried once with a new token", func(t *testing.T) {
		e := newEnv(t)
		e.connect(e.auth)
		e.srv.Queue("/sync/last_activities", trakttest.Reply{Status: 401})
		e.srv.Handle("/sync/last_activities", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"all": "2026-10-16T08:00:00.000Z"}`)
		})
		var acts struct{ All string }
		if _, err := e.auth.Get(e.ctx, "/sync/last_activities", nil, &acts); err != nil || acts.All == "" {
			t.Fatalf("Get = %+v, %v", acts, err)
		}
		reqs := e.srv.Requests()
		last := reqs[len(reqs)-1]
		if last.Header.Get("Authorization") != "Bearer "+e.srv.AccessToken() || e.srv.Count("/oauth/token") != 1 {
			t.Errorf("retried with %q after %d refreshes", last.Header.Get("Authorization"), e.srv.Count("/oauth/token"))
		}
	})
	t.Run("two 401s are reauth", func(t *testing.T) {
		e := newEnv(t)
		e.connect(e.auth)
		e.srv.Queue("/sync/last_activities", trakttest.Reply{Status: 401}, trakttest.Reply{Status: 401})
		if _, err := e.auth.Get(e.ctx, "/sync/last_activities", nil, nil); !errors.Is(err, ErrReauth) {
			t.Fatalf("Get = %v, want ErrReauth", err)
		}
		if n := e.srv.Count("/sync/last_activities"); n != 2 {
			t.Errorf("requests = %d, want 2", n)
		}
		if _, err := e.auth.Get(e.ctx, "/sync/last_activities", nil, nil); !errors.Is(err, ErrReauth) || e.srv.Count("/sync/last_activities") != 2 {
			t.Error("a reauth token was used again")
		}
	})
}

func TestAuth_Disconnect(t *testing.T) {
	e := newEnv(t)
	var stopped atomic.Int32
	e.auth.SetHooks(Hooks{StopSync: func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("StopSync without a deadline")
		}
		stopped.Add(1)
	}})
	e.connect(e.auth)
	access, _ := e.storedTokens()
	for _, q := range []string{
		`INSERT INTO trakt_sync_state (key, value) VALUES ('last_sync_at', '"2026-10-16T08:00:00Z"')`,
		`INSERT INTO trakt_history (id, media_type, tmdb_id, trakt_id, title, watched_at) VALUES (9001, 'movie', 603, 481, 'The Matrix', NOW())`,
		`INSERT INTO trakt_watchlist (media_type, trakt_id, tmdb_id, title, listed_at) VALUES ('movie', 481, 603, 'The Matrix', NOW())`,
	} {
		if _, err := e.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	off := false
	if _, err := e.settings.UpdateTraktSettings(e.ctx, library.TraktSettingsPatch{ShowWatchlist: &off}); err != nil {
		t.Fatal(err)
	}
	if in := e.integration(e.auth); in.Counts.History != 1 || in.Counts.Watchlist != 1 {
		t.Errorf("counts before = %+v", in.Counts)
	}

	ctx, cancel := context.WithCancel(e.ctx)
	cancel() // the client went away: the disconnect still finishes
	if err := e.auth.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if stopped.Load() != 1 {
		t.Errorf("StopSync ran %d times", stopped.Load())
	}
	if got := e.srv.Revoked(); len(got) != 1 || got[0] != access {
		t.Errorf("revoked = %v, want [%s]", got, access)
	}
	var rows int
	if err := e.db.QueryRow(`SELECT (SELECT COUNT(*) FROM trakt_auth) + (SELECT COUNT(*) FROM trakt_sync_state) +
		(SELECT COUNT(*) FROM trakt_history) + (SELECT COUNT(*) FROM trakt_watchlist)`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("%d rows left (%v)", rows, err)
	}
	in := e.integration(e.auth)
	if in.State != StateOff || in.Username != nil || in.Counts != (library.TraktCounts{}) || in.Settings.ShowWatchlist {
		t.Errorf("after the disconnect = %+v", in)
	}
	if _, err := e.auth.Token(e.ctx); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Token = %v", err)
	}
	if err := e.auth.Disconnect(e.ctx); err != nil || len(e.srv.Revoked()) != 1 {
		t.Errorf("a second disconnect = %v (revoked %v)", err, e.srv.Revoked())
	}
}

// Snapshot has Integration's state and username in every state
// (/members' summary).
func TestAuth_Snapshot(t *testing.T) {
	e := newEnv(t)
	check := func(want State, wantUser string) {
		t.Helper()
		state, username := e.auth.Snapshot()
		in := e.integration(e.auth)
		if state != want || state != in.State || ptrValue(username) != wantUser || (username == nil) != (in.Username == nil) {
			t.Errorf("Snapshot = %s %q, Integration %s %v, want %s %q", state, ptrValue(username), in.State, in.Username, want, wantUser)
		}
	}
	check(StateOff, "")
	e.clock.Block(true)
	if _, err := e.auth.Connect(e.ctx); err != nil {
		t.Fatal(err)
	}
	check(StatePending, "")
	if err := e.auth.Disconnect(e.ctx); err != nil { // cancels the code
		t.Fatal(err)
	}
	e.clock.Block(false)
	e.connect(e.auth)
	check(StateConnected, "Teymur")

	e.srv.RevokeAll()
	e.clock.Advance(6*24*time.Hour + time.Hour)
	if _, err := e.auth.Token(e.ctx); !errors.Is(err, ErrReauth) {
		t.Fatalf("Token = %v, want ErrReauth", err)
	}
	check(StateReauth, "Teymur")
	e.clock.Block(true)
	if _, err := e.auth.Connect(e.ctx); err != nil {
		t.Fatal(err)
	}
	check(StatePending, "Teymur") // a code from reauth keeps the account
}

func TestAuth_SyncHooks(t *testing.T) {
	e := newEnv(t)
	if _, err := e.auth.SyncNow(e.ctx); !errors.Is(err, ErrSyncUnavailable) {
		t.Errorf("SyncNow without the sync = %v", err)
	}
	synced := 0
	at := e.clock.Now()
	e.auth.SetHooks(Hooks{
		SyncNow: func(context.Context) error { synced++; return nil },
		SyncStatus: func(context.Context) (SyncStatus, error) {
			kind := SyncIncremental
			return SyncStatus{LastSyncAt: &at, NextSyncAt: &at, Run: SyncRun{Status: SyncIdle, Kind: &kind}}, nil
		},
	})
	if _, err := e.auth.SyncNow(e.ctx); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SyncNow while off = %v", err)
	}
	if in := e.integration(e.auth); in.NextSyncAt != nil || in.LastSyncAt == nil {
		t.Errorf("next_sync_at %v while off, last_sync_at %v", in.NextSyncAt, in.LastSyncAt)
	}
	e.connect(e.auth)
	in, err := e.auth.SyncNow(e.ctx)
	if err != nil || synced != 1 || in.NextSyncAt == nil || ptrValue(in.Sync.Kind) != SyncIncremental {
		t.Errorf("SyncNow = %+v, %v (synced %d)", in, err, synced)
	}
}

// GET /integrations/trakt before anything happened, as the web contract
// has it.
func TestIntegration_JSON(t *testing.T) {
	e := newEnv(t)
	raw, err := json.Marshal(e.integration(e.auth))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"configured":true,"state":"off","connected":false,"username":null,"pending":null,"connect_error":null,
		"reauth_reason":null,"last_sync_at":null,"next_sync_at":null,
		"sync":{"status":"idle","kind":null,"step":null,"progress":{"plays":0,"plays_total":0,"ratings":0,"watchlist":0,"shows":0},
		"error":null,"started_at":null,"finished_at":null},
		"counts":{"history":0,"ratings":0,"watchlist":0,"watchlist_in_library":0},
		"settings":{"use_ratings":true,"show_watchlist":true,"include_paused":false}}`
	var got, wantV any
	_ = json.Unmarshal(raw, &got)
	if err := json.Unmarshal([]byte(want), &wantV); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint(wantV) {
		t.Errorf("JSON = %s\nwant %s", raw, want)
	}

	e.clock.Block(true)
	if _, err := e.auth.Connect(e.ctx); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(e.integration(e.auth).Pending)
	if want := `{"user_code":"A1B2C3D4","verification_url":"https://trakt.tv/activate","expires_at":"2026-10-16T12:10:00Z","interval":5}`; string(raw) != want {
		t.Errorf("pending = %s, want %s", raw, want)
	}
}

func ptrValue[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
