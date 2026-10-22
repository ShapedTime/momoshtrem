package trakt

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/secret"
)

// staggerStep spaces the members' first checks at startup: the first
// member's runs a minute after it (firstSyncDelay), each next member's
// this much later, so their imports don't all start at once.
const staggerStep = 2 * time.Minute

// RegistryDeps are what a Registry builds each member's connection and
// sync from. Without Client or Box Trakt isn't configured: every member's
// state is off and Connect answers ErrNotConfigured.
type RegistryDeps struct {
	Client  *Client
	Box     *secret.Box
	Artwork Artwork // nil: no details step
	Clock   Clock   // nil: real time
	// Auths, Syncs and Settings are a member's stores, bound to them:
	// TraktAuthRepository.ForMember, TraktSyncStore.ForMember and
	// MemberRepository.TraktSettingsFor.
	Auths    func(memberID int64) Store
	Syncs    func(memberID int64) SyncStore
	Settings func(memberID int64) Settings
}

// Registry is the household's Trakt (PLAN §6 "Household members"): one
// connection (Auth) and one sync (Syncer) per member, each over the
// member's own stores, so each member connects their own account and
// syncs it on their own schedule. Its request-scoped methods are the
// context's member's (member.Require), which serves /integrations/trakt
// (api.Trakt); a member it doesn't hold is member.ErrUnknown. It is safe
// for concurrent use.
type Registry struct {
	d   RegistryDeps
	log *slog.Logger

	life sync.Mutex // one Start, Add, Forget or Stop changes the members at a time

	mu         sync.RWMutex
	members    map[int64]*memberTrakt
	stopped    bool
	onImported func(ctx context.Context, memberID int64)
	onSynced   func()
}

// memberTrakt is a member's connection and sync.
type memberTrakt struct {
	auth *Auth
	sync *Syncer
}

// NewRegistry creates the household's Trakt, with no member yet: Start
// adds them.
func NewRegistry(d RegistryDeps) *Registry {
	return &Registry{d: d, log: slog.With("component", "trakt"), members: map[int64]*memberTrakt{}}
}

// Configured reports whether TRAKT_CLIENT_ID and a valid SECRET_KEY are
// set.
func (r *Registry) Configured() bool { return r.d.Client != nil && r.d.Box != nil }

// SetOnImported installs a function called with the member when their
// initial import has stored everything, before their sync reports it idle
// (Syncer.SetOnImported). It can be set at any time.
func (r *Registry) SetOnImported(fn func(ctx context.Context, memberID int64)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onImported = fn
}

// SetOnSynced installs a function called after any member's sync that
// ends well (Syncer.SetOnSynced). It must not block. It can be set at any
// time.
func (r *Registry) SetOnSynced(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onSynced = fn
}

// Start adds every member of ids (in id order), each with their stored
// connection read and, configured, their sync's schedule running: the
// first member's first check a minute after startup, each next member's 2
// minutes after the one before. Call it before serving.
func (r *Registry) Start(ctx context.Context, ids []int64) {
	r.life.Lock()
	defer r.life.Unlock()
	for i, id := range ids {
		r.add(ctx, id, firstSyncDelay+time.Duration(i)*staggerStep)
	}
}

// Add adds a member (member.Service.OnAdded): their connection, off, and
// their sync. A member it holds already stays as it is.
func (r *Registry) Add(ctx context.Context, id int64) {
	r.life.Lock()
	defer r.life.Unlock()
	r.add(ctx, id, firstSyncDelay)
}

// add creates a member's connection and sync, unless it holds them or
// it's stopped. Holds life.
func (r *Registry) add(ctx context.Context, id int64, firstDelay time.Duration) {
	r.mu.RLock()
	_, held := r.members[id]
	stopped := r.stopped
	r.mu.RUnlock()
	if held || stopped {
		return
	}
	auth := NewAuth(AuthDeps{Client: r.d.Client, Box: r.d.Box, Store: r.d.Auths(id), Settings: r.d.Settings(id),
		Clock: r.d.Clock, Member: id})
	syncer := NewSyncer(SyncDeps{Auth: auth, Store: r.d.Syncs(id), Artwork: r.d.Artwork, Clock: r.d.Clock,
		FirstDelay: firstDelay, Member: id})
	auth.SetHooks(syncer.Hooks())
	syncer.SetOnImported(func(ctx context.Context) { r.imported(ctx, id) })
	syncer.SetOnSynced(r.synced)
	if err := auth.Load(ctx); err != nil {
		r.log.Error("Failed to read the Trakt connection; trying again on first use", "member", id, "error", err)
	}

	r.mu.Lock()
	r.members[id] = &memberTrakt{auth: auth, sync: syncer}
	r.mu.Unlock()
	if auth.Configured() {
		syncer.Start()
	}
}

// Forget lets go of a member being removed (member.Service.OnRemoving),
// in any state: no new code, a pending one cancelled, the sync and its
// schedule stopped (waiting at most 10 s), the member's rows purged and
// the token revoked (best effort, 5 s; Auth.Forget). Their requests
// answer member.ErrUnknown from the start.
//
// It runs before the member's row is deleted, and when it returns their
// sync can't write again: the purge takes the connection's row first, as
// every sync write does (TraktSyncStore), so it waits for a write still in
// flight (one Stop gave up on), and deletes the row the next write would
// need. So the delete's cascade never meets a sync write, which holds the
// connection's row and then needs the member's (the FK): those two would
// deadlock.
func (r *Registry) Forget(ctx context.Context, id int64) {
	r.life.Lock()
	r.mu.Lock()
	m := r.members[id]
	delete(r.members, id)
	r.mu.Unlock()
	r.life.Unlock()
	if m == nil {
		return
	}
	m.auth.Close() // no code from here on; a pending one's poller is done
	m.sync.Stop()  // no sync from here on; the running one is cancelled
	if err := m.auth.Forget(ctx); err != nil {
		r.log.Error("Failed to disconnect a removed member's Trakt", "member", id, "error", err)
	}
}

// Stop ends every member's sync and polling, all at once, so shutdown
// waits at most 10 s for the slowest (Syncer.Stop), not for their sum.
func (r *Registry) Stop() {
	r.life.Lock()
	r.mu.Lock()
	r.stopped = true
	members := make([]*memberTrakt, 0, len(r.members))
	for _, m := range r.members {
		members = append(members, m)
	}
	r.mu.Unlock()
	r.life.Unlock()

	var wg sync.WaitGroup
	for _, m := range members {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.sync.Stop()
			m.auth.Close()
		}()
	}
	wg.Wait()
}

// get is the member's connection and sync; nil when it doesn't hold them.
func (r *Registry) get(id int64) *memberTrakt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.members[id]
}

// of is the context's member's connection and sync: member.ErrNoMember
// without one, member.ErrUnknown for a member it doesn't hold.
func (r *Registry) of(ctx context.Context) (*memberTrakt, error) {
	id, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	if m := r.get(id); m != nil {
		return m, nil
	}
	return nil, member.ErrUnknown
}

// Integration is the context's member's GET /integrations/trakt
// (Auth.Integration).
func (r *Registry) Integration(ctx context.Context) (Integration, error) {
	m, err := r.of(ctx)
	if err != nil {
		return Integration{}, err
	}
	return m.auth.Integration(ctx)
}

// Connect starts the context's member's device-code flow (Auth.Connect).
func (r *Registry) Connect(ctx context.Context) (DeviceCode, error) {
	m, err := r.of(ctx)
	if err != nil {
		return DeviceCode{}, err
	}
	return m.auth.Connect(ctx)
}

// Disconnect is the context's member's DELETE /integrations/trakt
// (Auth.Disconnect).
func (r *Registry) Disconnect(ctx context.Context) error {
	m, err := r.of(ctx)
	if err != nil {
		return err
	}
	return m.auth.Disconnect(ctx)
}

// SyncNow starts the context's member's sync or joins it (Auth.SyncNow).
func (r *Registry) SyncNow(ctx context.Context) (Integration, error) {
	m, err := r.of(ctx)
	if err != nil {
		return Integration{}, err
	}
	return m.auth.SyncNow(ctx)
}

// UpdateSettings stores the context's member's settings patch
// (Auth.UpdateSettings).
func (r *Registry) UpdateSettings(ctx context.Context, patch library.TraktSettingsPatch) (library.TraktSettings, error) {
	m, err := r.of(ctx)
	if err != nil {
		return library.TraktSettings{}, err
	}
	return m.auth.UpdateSettings(ctx, patch)
}

// Nudge syncs the context's member in the background when their last
// check is over 5 minutes old (GET /me/up-next, Syncer.Nudge). Nothing
// for a member it doesn't hold.
func (r *Registry) Nudge(ctx context.Context) {
	if m, err := r.of(ctx); err == nil && r.Configured() {
		m.sync.Nudge()
	}
}

// ConnectedFor reports whether the context's member is connected
// (/status); false without a member, as for an unknown header.
func (r *Registry) ConnectedFor(ctx context.Context) bool {
	id, ok := member.IDFrom(ctx)
	return ok && r.Connected(id)
}

// Connected reports whether the member's state is connected.
func (r *Registry) Connected(id int64) bool {
	m := r.get(id)
	return m != nil && m.auth.Connected()
}

// Connection is the member's connection state for the picks
// (profile.Connection), read on each call, so it follows the member
// connecting and disconnecting.
func (r *Registry) Connection(id int64) MemberConnection {
	return MemberConnection{r: r, id: id}
}

// MemberConnection is one member's Connected (Registry.Connection).
type MemberConnection struct {
	r  *Registry
	id int64
}

// Connected reports whether the member is connected now.
func (c MemberConnection) Connected() bool { return c.r.Connected(c.id) }

// Summary is the member's Trakt as /members shows it (Auth.Snapshot); off
// for a member it doesn't hold.
func (r *Registry) Summary(id int64) member.TraktSummary {
	m := r.get(id)
	if m == nil {
		return member.TraktSummary{State: string(StateOff)}
	}
	state, username := m.auth.Snapshot()
	return member.TraktSummary{State: string(state), Username: username}
}

func (r *Registry) imported(ctx context.Context, id int64) {
	r.mu.RLock()
	fn := r.onImported
	r.mu.RUnlock()
	if fn != nil {
		fn(ctx, id)
	}
}

func (r *Registry) synced() {
	r.mu.RLock()
	fn := r.onSynced
	r.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// memberLog is a component's logger, tagged with the member it works for
// (0 leaves the member out).
func memberLog(component string, memberID int64) *slog.Logger {
	if memberID == 0 {
		return slog.With("component", component)
	}
	return slog.With("component", component, "member", memberID)
}
