package member

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
)

// Store keeps the members (library.MemberRepository). Its Create and
// Delete hold the cap and "never zero members" against concurrent calls,
// from any process.
type Store interface {
	// List is every member, by id.
	List(ctx context.Context) ([]Member, error)
	// Create adds a member; an empty color is DefaultColor of the others'.
	// ErrLimit, ErrNameTaken.
	Create(ctx context.Context, name, color string) (Member, error)
	// Update changes the fields set. ErrNotFound, ErrNameTaken.
	Update(ctx context.Context, id int64, p Patch) (Member, error)
	// Delete removes the member, and what's theirs with it. ErrNotFound,
	// ErrLastMember.
	Delete(ctx context.Context, id int64) error
}

// TraktSummary is a member's Trakt connection as /members shows it; GET
// /integrations/trakt has the rest.
type TraktSummary struct {
	State    string  `json:"state"` // off, pending, connected or reauth
	Username *string `json:"username"`
}

// View is a member as /members answers it.
type View struct {
	Member
	Trakt TraktSummary `json:"trakt"`
}

// Listener is told a member's id as it's added or removed.
type Listener func(ctx context.Context, id int64)

// Service is the household's members: /members, and the member every API
// request is for (Resolve). It keeps the list (six at most) in memory,
// read at start, again after each change and once on an id it doesn't
// know. A member being removed is gone for Resolve, Default, IDs and List from
// the start of the delete. It is safe for concurrent use.
//
// Lock order: write, then load, then mu.
type Service struct {
	store Store
	log   *slog.Logger

	write sync.Mutex // one Create, Update or Delete at a time
	load  sync.Mutex // one read of the list at a time, kept in the order read

	mu       sync.RWMutex
	members  []Member // by id, as last read
	loaded   bool
	leaving  map[int64]bool // members being removed
	added    []Listener
	removing []Listener
	summary  func(id int64) TraktSummary
}

// NewService creates the members' service. It reads them on first use, or
// Load.
func NewService(store Store) *Service {
	return &Service{store: store, log: slog.With("component", "members"), leaving: map[int64]bool{}}
}

// OnAdded adds a listener told after each member is added. Call it before
// serving.
func (s *Service) OnAdded(fn Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added = append(s.added, fn)
}

// OnRemoving adds a listener told before a member's row goes, to let go of
// what it runs for them (their Trakt sync, picks run, Ask turns). The
// listeners run in order, without the request's cancellation, and bound
// their own time. Call it before serving.
func (s *Service) OnRemoving(fn Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removing = append(s.removing, fn)
}

// SetTraktSummary sets where a member's Trakt summary comes from. Without
// it every member's is off. Call it before serving.
func (s *Service) SetTraktSummary(fn func(id int64) TraktSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary = fn
}

// Load reads the members. Resolve and Default read them first when they
// haven't been, so a Load that fails at startup is tried again.
func (s *Service) Load(ctx context.Context) error {
	_, err := s.reload(ctx)
	return err
}

// reload reads the list and keeps it. One read runs at a time, so a read
// that started before a change never replaces the one read after it.
func (s *Service) reload(ctx context.Context) ([]Member, error) {
	s.load.Lock()
	defer s.load.Unlock()
	list, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the members: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.members, s.loaded = list, true
	return list, nil
}

// cached is the list in memory, read first when it hasn't been (fresh),
// without the members being removed.
func (s *Service) cached(ctx context.Context) (list []Member, fresh bool, err error) {
	s.mu.RLock()
	list, loaded := s.members, s.loaded
	s.mu.RUnlock()
	if !loaded {
		if list, err = s.reload(ctx); err != nil {
			return nil, true, err
		}
	}
	return s.staying(list), !loaded, nil
}

// staying is list without the members being removed.
func (s *Service) staying(list []Member) []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.leaving) == 0 {
		return list
	}
	return slices.DeleteFunc(slices.Clone(list), func(m Member) bool { return s.leaving[m.ID] })
}

func (s *Service) isLeaving(id int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.leaving[id]
}

// setLeaving hides a member being removed from Resolve, Default, IDs and List,
// or shows it again.
func (s *Service) setLeaving(id int64, leaving bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if leaving {
		s.leaving[id] = true
	} else {
		delete(s.leaving, id)
	}
}

// refresh reads the list after a change; when that fails the next use
// reads it.
func (s *Service) refresh(ctx context.Context) {
	if _, err := s.reload(ctx); err != nil {
		s.log.Warn("Failed to read the members after a change; reading them on next use", "error", err)
		s.mu.Lock()
		s.loaded = false
		s.mu.Unlock()
	}
}

// Resolve is the member a request is for: the header's id, or the lowest
// (Default) without one. ErrMalformed for a value that isn't a positive
// integer, ErrUnknown for an id no member has, once the list is read
// again, or one being removed.
func (s *Service) Resolve(ctx context.Context, header string) (int64, error) {
	if header == "" {
		return s.Default(ctx)
	}
	id, err := ParseID(header)
	if err != nil {
		return 0, err
	}
	list, fresh, err := s.cached(ctx)
	if err != nil {
		return 0, err
	}
	if !has(list, id) && !fresh && !s.isLeaving(id) {
		if list, err = s.reload(ctx); err != nil {
			return 0, err
		}
		list = s.staying(list)
	}
	if !has(list, id) {
		return 0, ErrUnknown
	}
	return id, nil
}

// Default is the lowest id: the member of a request without the header
// (the CLI, scripts).
func (s *Service) Default(ctx context.Context) (int64, error) {
	list, _, err := s.cached(ctx)
	if err != nil {
		return 0, err
	}
	if len(list) == 0 {
		return 0, errors.New("the household has no members")
	}
	return list[0].ID, nil
}

// IDs are every member's, by id.
func (s *Service) IDs(ctx context.Context) ([]int64, error) {
	list, _, err := s.cached(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(list))
	for i, m := range list {
		ids[i] = m.ID
	}
	return ids, nil
}

// List is GET /members: every member, by id, but those being removed (so
// the picker never offers them).
func (s *Service) List(ctx context.Context) ([]View, error) {
	list, err := s.reload(ctx)
	if err != nil {
		return nil, err
	}
	list = s.staying(list)
	out := make([]View, len(list))
	for i, m := range list {
		out[i] = s.view(m)
	}
	return out, nil
}

// Create adds a member. The name is trimmed; a nil color is the first no
// member uses. *ValidationError, ErrLimit, ErrNameTaken.
func (s *Service) Create(ctx context.Context, name string, color *string) (View, error) {
	name, err := CleanName(name)
	if err != nil {
		return View{}, err
	}
	var c string
	if color != nil {
		if err := CheckColor(*color); err != nil {
			return View{}, err
		}
		c = *color
	}

	s.write.Lock()
	defer s.write.Unlock()
	m, err := s.store.Create(ctx, name, c)
	if err != nil {
		return View{}, err
	}
	ctx = context.WithoutCancel(ctx) // the member exists: the listeners must hear of it
	s.refresh(ctx)
	s.log.Info("Household member added", "member", m.ID)
	s.tell(ctx, &s.added, m.ID)
	return s.view(m), nil
}

// Update changes the name or colour, or both. *ValidationError,
// ErrNotFound, ErrNameTaken.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (View, error) {
	if p.Name != nil {
		name, err := CleanName(*p.Name)
		if err != nil {
			return View{}, err
		}
		p.Name = &name
	}
	if p.Color != nil {
		if err := CheckColor(*p.Color); err != nil {
			return View{}, err
		}
	}

	s.write.Lock()
	defer s.write.Unlock()
	m, err := s.store.Update(ctx, id, p)
	if err != nil {
		return View{}, err
	}
	s.refresh(ctx)
	return s.view(m), nil
}

// Delete removes a member (PLAN §6 "Household members"): the OnRemoving
// listeners let go of what's theirs, then the row goes and what's theirs
// with it. ErrNotFound, ErrLastMember. Once the checks pass it runs to the
// end, whether or not the client waits, and the member is gone for
// Resolve, Default, IDs and List from then on. Should the row stay after the
// listeners ran, the member comes back and the OnAdded listeners are told
// again, so it has nothing running (Trakt off).
func (s *Service) Delete(ctx context.Context, id int64) error {
	s.write.Lock()
	defer s.write.Unlock()
	list, err := s.reload(ctx)
	if err != nil {
		return err
	}
	if !has(list, id) {
		return ErrNotFound
	}
	if len(list) == 1 {
		return ErrLastMember
	}

	// Shown again once it's over: gone from the refreshed list, back after
	// the OnAdded listeners, or after a listener's panic.
	s.setLeaving(id, true)
	defer s.setLeaving(id, false)
	ctx = context.WithoutCancel(ctx)
	s.tell(ctx, &s.removing, id)
	err = s.store.Delete(ctx, id)
	s.refresh(ctx)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			s.log.Warn("Household member not removed; adding it back", "member", id, "error", err)
			s.tell(ctx, &s.added, id)
		}
		return err
	}
	s.log.Info("Household member removed", "member", id)
	return nil
}

// tell calls the listeners in of, in order.
func (s *Service) tell(ctx context.Context, of *[]Listener, id int64) {
	s.mu.RLock()
	fns := slices.Clone(*of)
	s.mu.RUnlock()
	for _, fn := range fns {
		fn(ctx, id)
	}
}

func (s *Service) view(m Member) View {
	s.mu.RLock()
	summary := s.summary
	s.mu.RUnlock()
	v := View{Member: m, Trakt: TraktSummary{State: "off"}}
	if summary != nil {
		v.Trakt = summary(m.ID)
	}
	return v
}

func has(list []Member, id int64) bool {
	return slices.ContainsFunc(list, func(m Member) bool { return m.ID == id })
}
