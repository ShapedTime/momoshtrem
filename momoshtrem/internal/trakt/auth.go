package trakt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/secret"
)

const (
	// refreshTimeout bounds a refresh, detached from the caller: one cut
	// short after Trakt rotated the tokens would lose them.
	refreshTimeout    = 30 * time.Second
	deviceCodeTimeout = 15 * time.Second
	stopSyncTimeout   = 10 * time.Second
	revokeTimeout     = 5 * time.Second
	// slowDown is what a 429 adds to the polling interval.
	slowDown = 5 * time.Second
	// refreshMargin: a token is refreshed once less than a quarter of its
	// lifetime or this, whichever is shorter, is left.
	refreshMargin = 24 * time.Hour

	defaultInterval  = 5 * time.Second
	defaultCodeTTL   = 10 * time.Minute
	aadAccessToken   = "trakt:access"
	aadRefreshToken  = "trakt:refresh"
	reasonRejected   = "Trakt stopped accepting momoshtrem’s sign-in. Connect again to keep syncing."
	reasonUnreadable = "momoshtrem can’t read its Trakt sign-in (SECRET_KEY changed). Connect again to keep syncing."
)

// Store keeps the connection (trakt_auth) and purges what was imported
// with it. *library.TraktAuthRepository implements it.
type Store interface {
	Get(ctx context.Context) (*library.TraktAuth, error)
	Connect(ctx context.Context, a library.TraktAuth) (time.Time, error)
	UpdateTokens(ctx context.Context, connectedAt time.Time, t library.TraktTokens) error
	SetReauth(ctx context.Context, connectedAt time.Time, reason string) error
	Purge(ctx context.Context) error
	Counts(ctx context.Context) (library.TraktCounts, error)
}

var _ Store = (*library.TraktAuthRepository)(nil)

// Settings are the Trakt settings (app_settings key trakt).
// *library.SettingsRepository implements it.
type Settings interface {
	TraktSettings(ctx context.Context) (library.TraktSettings, error)
	UpdateTraktSettings(ctx context.Context, patch library.TraktSettingsPatch) (library.TraktSettings, error)
}

var _ Settings = (*library.SettingsRepository)(nil)

// Hooks plug the sync (#132) into the connection. Every field is optional.
type Hooks struct {
	// OnConnected runs once a connect has stored the tokens, outside any
	// lock: the initial import starts there. It mustn't block.
	OnConnected func()
	// StopSync stops the running sync and waits for it until ctx ends (10
	// s); a disconnect purges after it.
	StopSync func(ctx context.Context)
	// SyncNow starts a sync, or joins the running one (POST
	// /integrations/trakt/sync). Without it that route answers
	// ErrSyncUnavailable.
	SyncNow func(ctx context.Context) error
	// SyncStatus reports the sync for GET /integrations/trakt. Without it:
	// never synced.
	SyncStatus func(ctx context.Context) (SyncStatus, error)
}

// Token is an access token and the connection it belongs to.
type Token struct {
	// Access goes in the Authorization header.
	Access string
	// Generation is the connection's (trakt_auth.connected_at): sync
	// writes check it, so nothing lands after a disconnect or reconnect.
	Generation time.Time
}

// AuthDeps are what an Auth needs. Without Client or Box Trakt isn't
// configured: the state is off and Connect answers ErrNotConfigured.
type AuthDeps struct {
	Client   *Client
	Box      *secret.Box
	Store    Store
	Settings Settings
	Clock    Clock // nil: real time
}

// Auth is the Trakt connection (PLAN §6 "Trakt (B5)"): the device-code
// connect, the tokens (sealed with SECRET_KEY and refreshed before they
// expire), reauth and disconnect. It is safe for concurrent use.
//
// Lock order: life, then refreshMu, then mu.
type Auth struct {
	client   *Client
	box      *secret.Box
	store    Store
	settings Settings
	clock    Clock
	log      *slog.Logger

	life      sync.Mutex // one connect, finished connect or disconnect at a time
	refreshMu sync.Mutex // one refresh at a time: refresh tokens are single-use
	pollers   sync.WaitGroup

	mu           sync.Mutex
	hooks        Hooks
	loaded       bool
	closed       bool
	state        State
	username     string
	gen          time.Time // the stored connection's connected_at
	tokens       *tokens   // nil when there are none (or they can't be read)
	reauthReason string
	connectError ConnectError
	flow         *flow // the live device code while pending
}

// tokens are the connection's, decrypted.
type tokens struct {
	access    string
	refresh   string
	createdAt time.Time
	expiresAt time.Time
	// rejected: the API answered 401 for access, so it's never handed out
	// again.
	rejected bool
	// afterRejection: a refresh forced by a 401 brought these, so a 401
	// for them means reauth.
	afterRejection bool
	// unsaved: storing them failed; Token stores them before using them.
	unsaved bool
}

// flow is a device code being polled.
type flow struct {
	code       DeviceCode
	deviceCode string
	from       State // off or reauth: where a code that doesn't connect goes back to
	cancel     context.CancelFunc
}

// NewAuth creates the connection. It reads the stored one on first use
// (or Load).
func NewAuth(d AuthDeps) *Auth {
	a := &Auth{
		store:    d.Store,
		settings: d.Settings,
		clock:    d.Clock,
		log:      slog.With("component", "trakt"),
		state:    StateOff,
	}
	if d.Client != nil && d.Box != nil {
		a.client, a.box = d.Client, d.Box
	}
	if a.clock == nil {
		a.clock = realClock{}
	}
	return a
}

// SetHooks plugs the sync in. Call it before serving.
func (a *Auth) SetHooks(h Hooks) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hooks = h
}

// Configured reports whether TRAKT_CLIENT_ID and a
// valid SECRET_KEY are set.
func (a *Auth) Configured() bool { return a.client != nil }

// Connected reports whether the state is connected (/status).
func (a *Auth) Connected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state == StateConnected
}

// Load reads the stored connection. Every method loads it first when it
// hasn't been, so a Load that fails at startup is tried again. Tokens that
// don't decrypt (another SECRET_KEY) mean reauth, until a restart with the
// right key.
func (a *Auth) Load(ctx context.Context) error {
	a.mu.Lock()
	loaded := a.loaded
	a.mu.Unlock()
	if loaded {
		return nil
	}
	row, err := a.store.Get(ctx)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loaded {
		return nil
	}
	a.loaded = true
	if row == nil || !a.Configured() {
		return nil
	}
	a.gen, a.username = row.ConnectedAt, row.Username
	access, err1 := a.box.Open(row.AccessTokenEnc, aadAccessToken)
	refresh, err2 := a.box.Open(row.RefreshTokenEnc, aadRefreshToken)
	switch {
	case err1 != nil || err2 != nil:
		a.log.Warn("Trakt tokens don't decrypt with this SECRET_KEY; connect again (or restore the old key)")
		a.state, a.reauthReason = StateReauth, reasonUnreadable
		return nil
	case row.NeedsReauth:
		a.state, a.reauthReason = StateReauth, row.ReauthReason
		if a.reauthReason == "" {
			a.reauthReason = reasonRejected
		}
	default:
		a.state = StateConnected
	}
	a.tokens = &tokens{access: string(access), refresh: string(refresh), createdAt: row.TokenCreatedAt, expiresAt: row.ExpiresAt}
	return nil
}

// Integration is GET /integrations/trakt: the connection, the sync's
// report (Hooks.SyncStatus), what's imported and the settings.
func (a *Auth) Integration(ctx context.Context) (Integration, error) {
	if err := a.Load(ctx); err != nil {
		return Integration{}, err
	}
	settings, err := a.settings.TraktSettings(ctx)
	if err != nil {
		return Integration{}, err
	}
	counts, err := a.store.Counts(ctx)
	if err != nil {
		return Integration{}, err
	}
	a.mu.Lock()
	report := a.hooks.SyncStatus
	a.mu.Unlock()
	var sync SyncStatus
	if report != nil {
		if sync, err = report(ctx); err != nil {
			return Integration{}, fmt.Errorf("failed to read the Trakt sync: %w", err)
		}
	}
	if sync.Run.Status == "" {
		sync.Run.Status = SyncIdle
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	out := Integration{
		Configured: a.Configured(),
		State:      a.state,
		Connected:  a.state == StateConnected,
		LastSyncAt: sync.LastSyncAt,
		Sync:       sync.Run,
		Counts:     counts,
		Settings:   settings,
	}
	if a.state == StateConnected {
		out.NextSyncAt = sync.NextSyncAt
	}
	if a.linkedLocked() {
		username := a.username
		out.Username = &username
	}
	if a.flow != nil {
		code := a.flow.code
		out.Pending = &code
	}
	if a.connectError != "" {
		e := a.connectError
		out.ConnectError = &e
	}
	if a.state == StateReauth {
		reason := a.reauthReason
		out.ReauthReason = &reason
	}
	return out, nil
}

// linkedLocked: an account is linked (connected, reauth, or a code pending
// from reauth).
func (a *Auth) linkedLocked() bool {
	return a.state == StateConnected || a.state == StateReauth || (a.flow != nil && a.flow.from == StateReauth)
}

// Connect starts the device-code flow from off or reauth and answers the
// code for the user; while a code is live, the same one. A poller asks
// Trakt every interval until the user approves it, denies it or it
// expires. ErrNotConfigured, ErrConnected, or ErrUnavailable when Trakt's
// device-code endpoint fails.
func (a *Auth) Connect(ctx context.Context) (DeviceCode, error) {
	if !a.Configured() {
		return DeviceCode{}, ErrNotConfigured
	}
	if err := a.Load(ctx); err != nil {
		return DeviceCode{}, err
	}
	a.life.Lock()
	defer a.life.Unlock()

	a.mu.Lock()
	if a.state == StateConnected {
		a.mu.Unlock()
		return DeviceCode{}, ErrConnected
	}
	if f := a.flow; f != nil {
		if a.clock.Now().Before(f.code.ExpiresAt) {
			a.mu.Unlock()
			return f.code, nil
		}
		a.endFlowLocked(f, ConnectExpired) // its poller hasn't noticed yet
	}
	from := a.state
	a.mu.Unlock()

	dctx, cancel := context.WithTimeout(ctx, deviceCodeTimeout)
	auth, err := a.client.DeviceCode(dctx)
	cancel()
	if err != nil {
		return DeviceCode{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	interval := time.Duration(auth.Interval) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	ttl := time.Duration(auth.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = defaultCodeTTL
	}
	fctx, fcancel := context.WithCancel(context.Background())
	f := &flow{
		code: DeviceCode{
			UserCode:        auth.UserCode,
			VerificationURL: auth.VerificationURL,
			ExpiresAt:       a.clock.Now().Add(ttl).UTC().Truncate(time.Second),
			Interval:        int(interval / time.Second),
		},
		deviceCode: auth.DeviceCode,
		from:       from,
		cancel:     fcancel,
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		fcancel()
		return DeviceCode{}, fmt.Errorf("%w: momoshtrem is shutting down", ErrUnavailable)
	}
	a.flow, a.state, a.connectError = f, StatePending, ""
	a.pollers.Add(1)
	go a.poll(fctx, f)
	return f.code, nil
}

// poll asks Trakt about the code every interval until it connects, fails
// or is cancelled.
func (a *Auth) poll(ctx context.Context, f *flow) {
	defer a.pollers.Done()
	interval := time.Duration(f.code.Interval) * time.Second
	for {
		if a.clock.Sleep(ctx, interval) != nil {
			return
		}
		if !a.clock.Now().Before(f.code.ExpiresAt) {
			a.endFlow(f, ConnectExpired)
			return
		}
		set, err := a.client.DeviceToken(ctx, f.deviceCode)
		var se *StatusError
		switch {
		case err == nil:
			a.finishConnect(ctx, f, set)
			return
		case ctx.Err() != nil:
			return
		case errors.Is(err, ErrAuthorizationPending):
		case errors.Is(err, ErrSlowDown):
			interval += slowDown
		case errors.Is(err, ErrDeviceCodeExpired):
			a.endFlow(f, ConnectExpired)
			return
		case errors.Is(err, ErrDeviceCodeDenied):
			a.endFlow(f, ConnectDenied)
			return
		case errors.Is(err, ErrInvalidDeviceCode), errors.Is(err, ErrDeviceCodeUsed),
			errors.As(err, &se) && se.StatusCode < 500:
			a.log.Warn("Trakt refused the device code", "error", err)
			a.endFlow(f, ConnectFailed)
			return
		default: // a network error or a 5xx: ask again
			a.log.Warn("Trakt device-code poll failed; trying again", "error", err)
		}
	}
}

// finishConnect stores the approved tokens with the account they belong
// to: connected, and the initial import starts (Hooks.OnConnected).
func (a *Auth) finishConnect(ctx context.Context, f *flow, set *TokenSet) {
	user, err := a.client.UserSettings(ctx, set.AccessToken)
	if err != nil || user.User.IDs.Slug == "" {
		a.log.Warn("Trakt approved the code, but its account can't be read", "error", err)
		a.revoke(set.AccessToken)
		a.endFlow(f, ConnectFailed)
		return
	}

	a.life.Lock()
	if ctx.Err() != nil { // cancelled meanwhile
		a.life.Unlock()
		a.revoke(set.AccessToken)
		return
	}
	t := newTokens(set)
	gen, err := a.store.Connect(ctx, library.TraktAuth{
		Username:        user.User.Username,
		UserSlug:        user.User.IDs.Slug,
		AccessTokenEnc:  a.box.Seal([]byte(t.access), aadAccessToken),
		RefreshTokenEnc: a.box.Seal([]byte(t.refresh), aadRefreshToken),
		TokenCreatedAt:  t.createdAt,
		ExpiresAt:       t.expiresAt,
	})
	if err != nil {
		a.life.Unlock()
		a.log.Error("Failed to store the Trakt connection", "error", err)
		a.revoke(set.AccessToken)
		a.endFlow(f, ConnectFailed)
		return
	}
	a.mu.Lock()
	a.flow, a.state, a.username, a.gen, a.tokens = nil, StateConnected, user.User.Username, gen, t
	a.reauthReason, a.connectError = "", ""
	onConnected := a.hooks.OnConnected
	a.mu.Unlock()
	a.life.Unlock()
	f.cancel()

	a.log.Info("Trakt connected", "username", user.User.Username, "token_expires_at", t.expiresAt)
	if onConnected != nil {
		onConnected()
	}
}

// endFlow ends a code that didn't connect: back to where it came from,
// with why.
func (a *Auth) endFlow(f *flow, why ConnectError) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.endFlowLocked(f, why)
}

func (a *Auth) endFlowLocked(f *flow, why ConnectError) {
	if a.flow != f {
		return
	}
	f.cancel()
	a.flow, a.state, a.connectError = nil, f.from, why
}

// Disconnect cancels a pending code (back to off, or to reauth while an
// account is linked). Otherwise it stops the sync, purges every Trakt
// table in one transaction and revokes the token (best effort, 5 s). The
// settings stay. Calling it again changes nothing.
func (a *Auth) Disconnect(ctx context.Context) error {
	ctx = context.WithoutCancel(ctx)
	if err := a.Load(ctx); err != nil {
		return err
	}
	a.life.Lock()
	defer a.life.Unlock()

	a.mu.Lock()
	if f := a.flow; f != nil {
		f.cancel()
		a.flow, a.state = nil, f.from
		a.mu.Unlock()
		return nil
	}
	stop := a.hooks.StopSync
	a.mu.Unlock()

	if stop != nil {
		sctx, cancel := context.WithTimeout(ctx, stopSyncTimeout)
		stop(sctx)
		cancel()
	}
	// No refresh in flight: the token revoked is the last one.
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	if err := a.store.Purge(ctx); err != nil {
		return err
	}
	a.mu.Lock()
	t := a.tokens
	a.state, a.username, a.gen, a.tokens, a.reauthReason, a.connectError = StateOff, "", time.Time{}, nil, "", ""
	a.mu.Unlock()
	if t != nil {
		a.revoke(t.access)
	}
	a.log.Info("Trakt disconnected")
	return nil
}

// revoke revokes a token, best effort.
func (a *Auth) revoke(token string) {
	if a.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), revokeTimeout)
	defer cancel()
	if err := a.client.RevokeToken(ctx, token); err != nil {
		a.log.Warn("Failed to revoke the Trakt token", "error", err)
	}
}

// Token answers the access token, refreshing it first when less than a
// quarter of its lifetime or 24 h (whichever is shorter) is left. One
// refresh runs at a time and stores both new tokens before handing either
// out. ErrNotConfigured, ErrNotConnected (off, or pending from off), or
// ErrReauth. A refresh that fails another way (network, 5xx) keeps the old
// token while it's valid.
func (a *Auth) Token(ctx context.Context) (Token, error) {
	if err := a.Load(ctx); err != nil {
		return Token{}, err
	}
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	cur, gen, err := a.current()
	if err != nil {
		return Token{}, err
	}
	if cur.unsaved {
		if err := a.saveTokens(ctx, gen, &cur); err != nil {
			return Token{}, err
		}
	}
	if !cur.rejected && a.clock.Now().Before(cur.expiresAt.Add(-refreshMarginFor(cur))) {
		return Token{Access: cur.access, Generation: gen}, nil
	}

	next, err := a.refresh(ctx, cur, gen)
	switch {
	case err == nil:
		return Token{Access: next.access, Generation: gen}, nil
	case next == nil && !errors.Is(err, ErrReauth) && !errors.Is(err, ErrNotConnected) &&
		!cur.rejected && a.clock.Now().Before(cur.expiresAt):
		a.log.Warn("Trakt token refresh failed; using the current token while it lasts", "error", err, "expires_at", cur.expiresAt)
		return Token{Access: cur.access, Generation: gen}, nil
	default:
		return Token{}, err
	}
}

// Invalidate reports that the API answered 401 to used. When used is
// still the current token it's never handed out again: it's refreshed at
// once, and a 401 for the token that refresh brought means reauth. A token
// already replaced is ignored. nil means Token has another token to try.
func (a *Auth) Invalidate(ctx context.Context, used Token) error {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	cur, gen, err := a.current()
	if err != nil {
		return err
	}
	if cur.access != used.Access || !gen.Equal(used.Generation) {
		return nil
	}
	if cur.afterRejection {
		a.setReauth(ctx, gen, reasonRejected)
		return ErrReauth
	}
	a.mu.Lock()
	if a.tokens != nil && a.tokens.access == cur.access {
		a.tokens.rejected = true
	}
	a.mu.Unlock()
	cur.rejected = true
	_, err = a.refresh(ctx, cur, gen)
	return err
}

// Get is Client.Get with the connection's token. A 401 invalidates the
// token and the request runs once more with the next one; a second 401
// means reauth (ErrReauth).
func (a *Auth) Get(ctx context.Context, path string, query url.Values, v any) (http.Header, error) {
	if !a.Configured() {
		return nil, ErrNotConfigured
	}
	for attempt := 0; ; attempt++ {
		tok, err := a.Token(ctx)
		if err != nil {
			return nil, err
		}
		header, err := a.client.Get(ctx, tok.Access, path, query, v)
		if !errors.Is(err, ErrUnauthorized) {
			return header, err
		}
		if ierr := a.Invalidate(ctx, tok); ierr != nil || attempt > 0 {
			return nil, errors.Join(err, ierr)
		}
	}
}

// current is a copy of the tokens Token may hand out, and their
// connection.
func (a *Auth) current() (tokens, time.Time, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case !a.Configured():
		return tokens{}, time.Time{}, ErrNotConfigured
	case a.state == StateConnected && a.tokens != nil:
		return *a.tokens, a.gen, nil
	case a.linkedLocked():
		return tokens{}, time.Time{}, ErrReauth
	default:
		return tokens{}, time.Time{}, ErrNotConnected
	}
}

// refresh trades cur's refresh token for a new pair and stores it before
// it's used. Trakt refusing it is reauth. When the pair can't be stored it
// answers it with the error: the old pair is dead either way. Holds
// refreshMu.
func (a *Auth) refresh(ctx context.Context, cur tokens, gen time.Time) (*tokens, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
	defer cancel()
	set, err := a.client.RefreshToken(ctx, cur.refresh)
	if errors.Is(err, ErrInvalidGrant) {
		a.log.Warn("Trakt refused the refresh token; connect again", "error", err)
		a.setReauth(ctx, gen, reasonRejected)
		return nil, ErrReauth
	}
	if err != nil {
		return nil, fmt.Errorf("failed to refresh the Trakt token: %w", err)
	}
	next := newTokens(set)
	next.afterRejection = cur.rejected
	err = a.saveTokens(ctx, gen, next)
	if errors.Is(err, library.ErrTraktConnectionReplaced) {
		return nil, ErrNotConnected
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.gen.Equal(gen) || a.state != StateConnected {
		return nil, ErrNotConnected
	}
	// Unsaved, the new pair is still the only one that works: keep it, and
	// Token stores it before using it.
	next.unsaved = err != nil
	a.tokens = next
	if err != nil {
		return next, err
	}
	return next, nil
}

// saveTokens stores both tokens in one UPDATE.
func (a *Auth) saveTokens(ctx context.Context, gen time.Time, t *tokens) error {
	err := a.store.UpdateTokens(ctx, gen, library.TraktTokens{
		AccessTokenEnc:  a.box.Seal([]byte(t.access), aadAccessToken),
		RefreshTokenEnc: a.box.Seal([]byte(t.refresh), aadRefreshToken),
		CreatedAt:       t.createdAt,
		ExpiresAt:       t.expiresAt,
	})
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.tokens != nil && a.tokens.access == t.access {
		a.tokens.unsaved = false
	}
	a.mu.Unlock()
	t.unsaved = false
	return nil
}

// setReauth: Trakt stopped accepting the connection's tokens.
func (a *Auth) setReauth(ctx context.Context, gen time.Time, reason string) {
	a.mu.Lock()
	if !a.gen.Equal(gen) || a.state != StateConnected {
		a.mu.Unlock()
		return
	}
	a.state, a.reauthReason = StateReauth, reason
	a.mu.Unlock()
	if err := a.store.SetReauth(ctx, gen, reason); err != nil && !errors.Is(err, library.ErrTraktConnectionReplaced) {
		a.log.Error("Failed to store the Trakt reauth", "error", err)
	}
}

// SyncNow starts a sync or joins the running one (POST
// /integrations/trakt/sync) and answers the integration.
// ErrSyncUnavailable until the sync plugs in Hooks.SyncNow;
// ErrNotConnected unless connected.
func (a *Auth) SyncNow(ctx context.Context) (Integration, error) {
	if err := a.Load(ctx); err != nil {
		return Integration{}, err
	}
	a.mu.Lock()
	syncNow, connected := a.hooks.SyncNow, a.state == StateConnected
	a.mu.Unlock()
	if syncNow == nil {
		return Integration{}, ErrSyncUnavailable
	}
	if !connected {
		return Integration{}, ErrNotConnected
	}
	if err := syncNow(ctx); err != nil {
		return Integration{}, err
	}
	return a.Integration(ctx)
}

// UpdateSettings stores the settings patch sets, in any state, and
// answers all of them.
func (a *Auth) UpdateSettings(ctx context.Context, patch library.TraktSettingsPatch) (library.TraktSettings, error) {
	return a.settings.UpdateTraktSettings(ctx, patch)
}

// Close cancels a pending code and waits for its poller.
func (a *Auth) Close() {
	a.mu.Lock()
	a.closed = true
	if a.flow != nil {
		a.flow.cancel()
	}
	a.mu.Unlock()
	a.pollers.Wait()
}

func newTokens(set *TokenSet) *tokens {
	return &tokens{
		access:    set.AccessToken,
		refresh:   set.RefreshToken,
		createdAt: set.Created(),
		expiresAt: set.Expires(),
	}
}

// refreshMarginFor is how long before it expires a token is refreshed: a
// quarter of its lifetime or 24 h, whichever is shorter (a 24-hour token
// would otherwise always be due).
func refreshMarginFor(t tokens) time.Duration {
	return min(t.expiresAt.Sub(t.createdAt)/4, refreshMargin)
}
