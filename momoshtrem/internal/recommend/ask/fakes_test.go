package ask_test

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// ---- Stores: the service's tests run on an in-memory store and, with
// MOMOSHTREM_TEST_DATABASE_URL, on Postgres too (each in a schema of its
// own, so the one-streaming index and the daily count are the test's).

type storeKind struct {
	name string
	open func(t *testing.T) ask.Store
}

func stores(t *testing.T) []storeKind {
	kinds := []storeKind{{"memory", func(*testing.T) ask.Store { return newMemStore() }}}
	if os.Getenv("MOMOSHTREM_TEST_DATABASE_URL") != "" {
		kinds = append(kinds, storeKind{"postgres", func(t *testing.T) ask.Store {
			return library.NewAskRepository(openSchemaDB(t))
		}})
	}
	return kinds
}

// forEachStore runs a test on each store.
func forEachStore(t *testing.T, test func(t *testing.T, store ask.Store)) {
	for _, kind := range stores(t) {
		t.Run(kind.name, func(t *testing.T) { test(t, kind.open(t)) })
	}
}

// openSchemaDB opens the test database in a new schema, migrated.
func openSchemaDB(t *testing.T) *library.DB {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	base, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { base.Close() })
	schema := fmt.Sprintf("ask_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	sqlDB, err := sql.Open("pgx", url+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db := library.NewDBFromSQL(sqlDB)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

// memStore is library.AskRepository's behaviour in memory.
type memStore struct {
	mu         sync.Mutex
	threads    map[int64]*library.AskThread
	turns      []*library.AskTurn
	messages   map[int64][]library.AskMessage
	nextThread int64
	nextTurn   int64
}

func newMemStore() *memStore {
	return &memStore{threads: map[int64]*library.AskThread{}, messages: map[int64][]library.AskMessage{}}
}

func (m *memStore) count(threadID int64) int {
	n := 0
	for _, t := range m.turns {
		if t.ThreadID == threadID {
			n++
		}
	}
	return n
}

func (m *memStore) threadCopy(id int64) *library.AskThread {
	t, ok := m.threads[id]
	if !ok {
		return nil
	}
	c := *t
	c.TurnCount = m.count(id)
	return &c
}

func (m *memStore) Thread(_ context.Context, id int64) (*library.AskThread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.threadCopy(id), nil
}

func (m *memStore) Threads(_ context.Context, limit int) ([]library.AskThread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []library.AskThread
	for id := range m.threads {
		out = append(out, *m.threadCopy(id))
	}
	slices.SortFunc(out, func(a, b library.AskThread) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	return out[:min(len(out), limit)], nil
}

func (m *memStore) Turns(_ context.Context, threadID int64) ([]library.AskTurn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []library.AskTurn
	for _, t := range m.turns {
		if t.ThreadID == threadID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (m *memStore) Messages(_ context.Context, threadID int64) ([]library.AskMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.messages[threadID]), nil
}

func (m *memStore) StartTurn(_ context.Context, in library.NewAskTurn) (*library.AskThread, *library.AskTurn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.turns {
		if t.Status == library.AskTurnStreaming {
			return nil, nil, library.ErrAskTurnStreaming
		}
	}
	now := time.Now()
	id := in.ThreadID
	if id == 0 {
		m.nextThread++
		id = m.nextThread
		m.threads[id] = &library.AskThread{ID: id, Title: in.Title, Model: in.Model, PromptVersion: in.PromptVersion, CreatedAt: now}
	}
	m.threads[id].UpdatedAt = now
	m.nextTurn++
	turn := &library.AskTurn{ID: m.nextTurn, ThreadID: id, Position: m.count(id), Message: in.Message,
		Status: library.AskTurnStreaming, CreatedAt: now}
	m.turns = append(m.turns, turn)
	c := *turn
	return m.threadCopy(id), &c, nil
}

func (m *memStore) turn(id int64) *library.AskTurn {
	for _, t := range m.turns {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (m *memStore) CancelTurn(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.turn(id); t != nil && t.Status == library.AskTurnStreaming {
		now := time.Now()
		t.Status, t.FinishedAt = library.AskTurnCancelled, &now
	}
	return nil
}

func (m *memStore) EndTurn(_ context.Context, end library.AskTurnEnd) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if end.Status != library.AskTurnDone && len(end.Messages) > 0 {
		return false, errors.New("only a done turn's messages are stored")
	}
	t := m.turn(end.TurnID)
	if t == nil || !(t.Status == library.AskTurnStreaming || (t.Status == library.AskTurnCancelled && end.Status == library.AskTurnCancelled)) {
		return false, nil
	}
	now := time.Now()
	t.Status, t.View, t.ErrorCode, t.CostUSD = end.Status, slices.Clone(end.View), end.ErrorCode, end.CostUSD
	if t.FinishedAt == nil {
		t.FinishedAt = &now
	}
	m.messages[t.ThreadID] = append(m.messages[t.ThreadID], end.Messages...)
	m.threads[t.ThreadID].UpdatedAt = now
	return true, nil
}

func (m *memStore) FailStreaming(_ context.Context, code string, view []byte) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, t := range m.turns {
		if t.Status == library.AskTurnStreaming {
			t.Status, t.ErrorCode, t.View = library.AskTurnError, code, slices.Clone(view)
			n++
		}
	}
	return n, nil
}

func (m *memStore) TurnsSince(_ context.Context, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, t := range m.turns {
		if !t.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) MeanDoneCost(_ context.Context, n int) (*float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sum float64
	count := 0
	for i := len(m.turns) - 1; i >= 0 && count < n; i-- {
		if m.turns[i].Status == library.AskTurnDone {
			sum += m.turns[i].CostUSD
			count++
		}
	}
	if count == 0 {
		return nil, nil
	}
	mean := sum / float64(count)
	return &mean, nil
}

var _ ask.Store = (*memStore)(nil)
var _ ask.Store = (*library.AskRepository)(nil)

// ---- What the tools and the picks read.

// titles answers every title's state: unknown titles are unwatched.
type titles struct {
	mu     sync.Mutex
	states map[library.TitleRef]library.TitleState
}

func (f *titles) set(s library.TitleState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = map[library.TitleRef]library.TitleState{}
	}
	f.states[s.TitleRef] = s
}

func (f *titles) TitleStates(_ context.Context, refs []library.TitleRef) ([]library.TitleState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]library.TitleState, len(refs))
	for i, ref := range refs {
		s, ok := f.states[ref]
		if !ok {
			s = library.TitleState{TitleRef: ref}
		}
		out[i] = s
	}
	return out, nil
}

// libraryStates says where titles are in the library.
type libraryStates map[catalog.TitleRef]*catalog.SimilarLibrary

func (l libraryStates) LibraryStates(_ context.Context, ids library.TMDBIDs) (map[catalog.TitleRef]*catalog.SimilarLibrary, error) {
	out := map[catalog.TitleRef]*catalog.SimilarLibrary{}
	for _, id := range ids.Movies {
		if s := l[catalog.TitleRef{MediaType: "movie", TMDBID: id}]; s != nil {
			out[catalog.TitleRef{MediaType: "movie", TMDBID: id}] = s
		}
	}
	for _, id := range ids.Shows {
		if s := l[catalog.TitleRef{MediaType: "tv", TMDBID: id}]; s != nil {
			out[catalog.TitleRef{MediaType: "tv", TMDBID: id}] = s
		}
	}
	return out, nil
}

// fakeTMDB knows a few titles; anything else is not found.
type fakeTMDB struct {
	movies map[int]*tmdb.MovieDetails
	shows  map[int]*tmdb.ShowDetails
}

func (f fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	if m, ok := f.movies[id]; ok {
		return m, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	if s, ok := f.shows[id]; ok {
		return s, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f fakeTMDB) Search(context.Context, string, string, int) (*tmdb.ResultsPage, error) {
	return &tmdb.ResultsPage{}, nil
}

func (f fakeTMDB) GenreList(context.Context, string) ([]tmdb.Genre, error) { return nil, nil }

// budget answers Check with err.
type budget struct{ err error }

func (b *budget) Check(context.Context) error { return b.err }

// settings answers rec_model.
type settings struct{ model string }

func (s settings) AISettings(context.Context) (library.AISettings, error) {
	a := library.DefaultAISettings()
	if s.model != "" {
		a.RecModel = s.model
	}
	return a, nil
}

// recorder keeps the usage rows; block, when set, holds a row until it's
// closed.
type recorder struct {
	mu    sync.Mutex
	rows  []usageRow
	block func(ref int64, n int) chan struct{}
}

type usageRow struct {
	feature string
	ref     int64
	usage   aicore.Usage
}

func (r *recorder) Record(_ context.Context, feature string, ref int64, u aicore.Usage) error {
	r.mu.Lock()
	n := 0
	for _, row := range r.rows {
		if row.ref == ref {
			n++
		}
	}
	var wait chan struct{}
	if r.block != nil {
		wait = r.block(ref, n+1)
	}
	r.mu.Unlock()
	if wait != nil {
		<-wait
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, usageRow{feature, ref, u})
	return nil
}

func (r *recorder) forRef(ref int64) []usageRow {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []usageRow
	for _, row := range r.rows {
		if row.ref == ref {
			out = append(out, row)
		}
	}
	return out
}
