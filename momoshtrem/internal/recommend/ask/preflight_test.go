package ask_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
)

// apiError is a JSON error answered before the stream.
type apiError struct {
	status   int
	code     string
	resetsAt *time.Time
}

func (h *harness) refused(body string) apiError {
	h.t.Helper()
	s, resp := h.post(body)
	if s != nil {
		h.t.Fatalf("%s opened a stream: %s", body, dump(s.all()))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var e struct {
		Error struct {
			Code     string     `json:"code"`
			ResetsAt *time.Time `json:"resets_at"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		h.t.Fatalf("%s = %d %s", body, resp.StatusCode, raw)
	}
	return apiError{resp.StatusCode, e.Error.Code, e.Error.ResetsAt}
}

// seed adds n ended turns to a thread (0: a new one) and returns its id.
func seed(t *testing.T, store ask.Store, threadID int64, n int) int64 {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		th, tu, err := store.StartTurn(ctx, library.NewAskTurn{ThreadID: threadID, Title: "Seeded", Model: recommend.ModelOpus,
			PromptVersion: ask.PromptVersion, Message: "seeded"})
		if err != nil {
			t.Fatal(err)
		}
		threadID = th.ID
		if _, err := store.EndTurn(ctx, library.AskTurnEnd{TurnID: tu.ID, Status: library.AskTurnDone, View: []byte(`{}`), CostUSD: 0.02,
			Messages: []library.AskMessage{{Role: "user", Content: []byte(`[{"type":"text","text":"seeded"}]`)},
				{Role: "assistant", Content: []byte(`[{"type":"text","text":"ok"}]`)}}}); err != nil {
			t.Fatal(err)
		}
	}
	return threadID
}

// The checks before the stream, each a JSON error with no API call and
// nothing stored, in PLAN §6's order: unavailable, not_found, thread_full,
// ask_busy, ask_limit, budget_exceeded.
func TestAsk_PreflightErrors(t *testing.T) {
	midnight := func() time.Time { _, next := aicore.DayBounds(time.Now()); return next }
	monthEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local)
	forEachStore(t, func(t *testing.T, store ask.Store) {
		t.Run("no key", func(t *testing.T) {
			h := newHarnessWith(t, store, harnessOpts{noKey: true})
			if e := h.refused(`{"message":"anime"}`); e.status != 503 || e.code != "unavailable" {
				t.Errorf("no key = %+v", e)
			}
		})

		full := seed(t, store, 0, recommend.ThreadMaxTurns)
		open := seed(t, store, 0, 1)
		h := newHarness(t, store, fake.Response{Blocks: []fake.Block{fake.Text("…")}, Hang: true})

		if e := h.refused(`{"thread_id":999,"message":"anime"}`); e.status != 404 || e.code != "not_found" {
			t.Errorf("unknown thread = %+v", e)
		}
		if e := h.refused(`{"thread_id":` + itoa(full) + `,"message":"anime"}`); e.status != 409 || e.code != "thread_full" {
			t.Errorf("full thread = %+v", e)
		}

		h.budget.err = &aicore.BudgetError{SpentUSD: 15, BudgetUSD: 15, ResetsAt: monthEnd}
		if e := h.refused(`{"message":"anime"}`); e.status != 429 || e.code != "budget_exceeded" || e.resetsAt == nil || !e.resetsAt.Equal(monthEnd) {
			t.Errorf("budget = %+v", e)
		}
		h.budget.err = nil

		// A turn streaming: everything after thread_full answers ask_busy.
		s := h.open(`{"thread_id":` + itoa(open) + `,"message":"anime"}`)
		start := s.until(t, named("start"))
		streaming := field[int64](t, start[len(start)-1], "turn_id")
		if e := h.refused(`{"thread_id":` + itoa(full) + `,"message":"anime"}`); e.code != "thread_full" {
			t.Errorf("full and busy = %+v, want thread_full first", e)
		}
		h.budget.err = &aicore.BudgetError{ResetsAt: monthEnd}
		if e := h.refused(`{"message":"anime"}`); e.status != 409 || e.code != "ask_busy" {
			t.Errorf("busy = %+v", e)
		}
		for deadline := time.Now().Add(5 * time.Second); len(h.api.Requests()) == 0 && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
		s.cancel()
		waitTurn(t, store, open, streaming, library.AskTurnCancelled)

		// 21 seeded + the cancelled one: 8 more make the day's 30.
		seed(t, store, 0, 8)
		if e := h.refused(`{"message":"anime"}`); e.status != 429 || e.code != "ask_limit" || e.resetsAt == nil || !e.resetsAt.Equal(midnight()) {
			t.Errorf("limit (and budget) = %+v, want ask_limit first", e)
		}
		if n := len(h.api.Requests()); n != 1 {
			t.Errorf("API requests = %d, want only the one that streamed", n)
		}
		if n, _ := store.TurnsSince(context.Background(), time.Now().Add(-time.Hour)); n != 30 {
			t.Errorf("turns = %d, want 30: a refused ask stores nothing", n)
		}
	})
}

// A turn streaming that this process didn't start (another process's, say)
// is still ask_busy: the one-streaming index is the arbiter.
func TestAsk_BusyInTheStore(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		if _, _, err := store.StartTurn(context.Background(), library.NewAskTurn{Title: "Elsewhere", Model: recommend.ModelOpus,
			PromptVersion: ask.PromptVersion, Message: "elsewhere"}); err != nil {
			t.Fatal(err)
		}
		h := newHarness(t, store)
		if e := h.refused(`{"message":"anime"}`); e.status != http.StatusConflict || e.code != "ask_busy" {
			t.Errorf("busy = %+v", e)
		}
		// Recover ends it (startup), and asking works again.
		if n, err := h.svc.Recover(context.Background()); err != nil || n != 1 {
			t.Fatalf("Recover = %d, %v", n, err)
		}
		turn := turnByID(t, store, 1, 1)
		if turn.Status != library.AskTurnError || turn.ErrorCode != recommend.AskErrorInternal {
			t.Errorf("recovered turn = %s %s", turn.Status, turn.ErrorCode)
		}
		thread, err := h.svc.Thread(context.Background(), 1)
		if err != nil || thread.Turns[0].Error == nil || thread.Turns[0].Error.Code != recommend.AskErrorInternal {
			t.Errorf("recovered thread = %+v, %v", thread, err)
		}
	})
}

// A thread started on a prompt this build doesn't have (ask-v1, retired in
// #160) can't go on.
func TestAsk_UnknownPromptVersion(t *testing.T) {
	store := newMemStore()
	th, tu, _ := store.StartTurn(context.Background(), library.NewAskTurn{Title: "Old", Model: recommend.ModelOpus, PromptVersion: "ask-v1", Message: "old"})
	_, _ = store.EndTurn(context.Background(), library.AskTurnEnd{TurnID: tu.ID, Status: library.AskTurnDone, View: []byte(`{}`)})
	h := newHarness(t, store)
	if e := h.refused(`{"thread_id":` + itoa(th.ID) + `,"message":"more"}`); e.code != "thread_full" {
		t.Errorf("old prompt = %+v", e)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
