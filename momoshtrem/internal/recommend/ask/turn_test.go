package ask_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
)

// steps names a stream's status events as "step:state[:detail]" and the
// rest by their name.
func steps(t *testing.T, events []sseEvent) string {
	t.Helper()
	var out []string
	for _, e := range events {
		if e.Name != "status" {
			out = append(out, e.Name)
			continue
		}
		s := field[string](t, e, "step") + ":" + field[string](t, e, "state")
		if d := field[*string](t, e, "detail"); d != nil {
			s += ":" + *d
		}
		out = append(out, s)
	}
	return strings.Join(out, " | ")
}

// Two tools in one response: the second's step starts when it runs, after
// the first answered, so the step it ends keeps its detail.
func TestAsk_ParallelToolsKeepDetails(t *testing.T) {
	h := newHarness(t, newMemStore(),
		fake.Response{Blocks: []fake.Block{fake.Thinking("sig_1"),
			toolUse("toolu_1", "get_taste_profile", map[string]any{}), toolUse("toolu_2", "get_candidates", anyCandidates)}},
		fake.Response{Blocks: []fake.Block{toolUse("toolu_3", "show_picks", picksInput("One pick.", []string{"Older"},
			pick("tv", 1001, "Workplace", "Like Severance.")))}},
		fake.Response{Blocks: []fake.Block{fake.Text("Enjoy.")}},
	)
	got := steps(t, h.open(`{"message":"anime"}`).all())
	want := "start | history:active | history:done:2,184 plays · you loved Severance, The Bear, Ted Lasso | tmdb:active | " +
		"tmdb:done:23 candidates · 6 already seen | write:active | text | item | write:done:1 pick | done"
	if got != want {
		t.Errorf("events =\n%s\nwant\n%s", got, want)
	}
}

// write is the last step: a tool after show_picks starts nothing, and
// write's detail counts every pick when the turn ends.
func TestAsk_WriteStaysLast(t *testing.T) {
	h := newHarness(t, newMemStore(),
		fake.Response{Blocks: []fake.Block{toolUse("toolu_1", "show_picks", picksInput("Two picks.", nil,
			pick("tv", 1001, "Workplace", "Like Severance."), pick("movie", 2003, "Seen", "Watched.")))}},
		fake.Response{Blocks: []fake.Block{toolUse("toolu_2", "check_seen",
			map[string]any{"titles": []map[string]any{{"media_type": "tv", "tmdb_id": 1002}}})}},
		fake.Response{Blocks: []fake.Block{toolUse("toolu_3", "show_picks", picksInput("", nil,
			pick("tv", 1002, "Ensemble", "The Bear's warmth.")))}},
		fake.Response{Blocks: []fake.Block{fake.Text("Enjoy.")}},
	)
	h.titles.set(library.TitleState{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 2003}, Plays: 2})
	got := steps(t, h.open(`{"message":"comedies"}`).all())
	if want := "start | write:active | text | item | item | write:done:2 picks | done"; got != want {
		t.Errorf("events = %s\nwant %s", got, want)
	}
}

// A cap reached right after show_picks showed picks lets one more call end
// the turn: the shown answer is done, not an error. Without picks the cap
// ends the turn with limit_reached.
func TestAsk_CapAfterPicks(t *testing.T) {
	checks := func(n int) []fake.Response {
		var out []fake.Response
		for i := 0; i < n; i++ {
			out = append(out, fake.Response{Blocks: []fake.Block{toolUse("toolu_c", "check_seen",
				map[string]any{"titles": []map[string]any{{"media_type": "tv", "tmdb_id": 1001}}})}})
		}
		return out
	}
	// The 12th call is the calls cap.
	script := append(checks(11),
		fake.Response{Blocks: []fake.Block{toolUse("toolu_p", "show_picks", picksInput("One.", nil, pick("tv", 1001, "W", "R.")))}},
		fake.Response{Blocks: []fake.Block{fake.Text("Enjoy.")}},
	)
	h := newHarness(t, newMemStore(), script...)
	events := h.open(`{"message":"anime"}`).all()
	if last := events[len(events)-1]; last.Name != "done" || len(h.api.Requests()) != 13 {
		t.Errorf("past the cap with picks: %s (%d requests)", dump(events[len(events)-2:]), len(h.api.Requests()))
	}

	h = newHarness(t, newMemStore(), checks(13)...)
	events = h.open(`{"message":"anime"}`).all()
	if last := events[len(events)-1]; last.Name != "error" || field[string](t, last, "code") != recommend.AskErrorLimitReached ||
		len(h.api.Requests()) != 12 {
		t.Errorf("at the cap without picks: %s (%d requests)", dump(events[len(events)-1:]), len(h.api.Requests()))
	}
}

// A turn's end is written before done goes out: done waits for it.
func TestAsk_DoneWaitsForTheWrite(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		faulty := &faultyStore{Store: store, gate: make(chan struct{})}
		h := newHarness(t, faulty, fullTurn()...)
		s := h.open(`{"message":"something like Severance"}`)
		s.until(t, func(e sseEvent) bool { return e.Data == `{"step":"write","state":"done","detail":"2 picks"}` })

		next := make(chan sseEvent, 1)
		go func() {
			if e, ok := s.next(); ok {
				next <- e
			}
			close(next)
		}()
		select {
		case e := <-next:
			t.Fatalf("%s arrived while the turn's end was still being written", e.Name)
		case <-time.After(150 * time.Millisecond):
		}
		if turn := turnByID(t, store, 1, 1); turn.Status != library.AskTurnStreaming {
			t.Fatalf("turn = %s before its end was written", turn.Status)
		}
		close(faulty.gate)
		e := <-next
		if e.Name != "done" {
			t.Fatalf("after the write: %s", e.Name)
		}
		if turn := turnByID(t, store, 1, 1); turn.Status != library.AskTurnDone {
			t.Errorf("turn = %s when done arrived", turn.Status)
		}
	})
}

// A failing store doesn't leave a turn streaming: the end is retried, then
// written as an error, then abandoned (status only), and failing even that
// the next ask ends it before starting. No ask meets ask_busy for it.
func TestAsk_EndWriteFailures(t *testing.T) {
	opts := harnessOpts{writeTimeout: 300 * time.Millisecond}
	forEachStore(t, func(t *testing.T, store ask.Store) {
		t.Run("retried", func(t *testing.T) {
			faulty := &faultyStore{Store: store, endFails: 2}
			h := newHarnessWith(t, faulty, opts, fullTurn()...)
			events := h.open(`{"message":"something like Severance"}`).all()
			start := field[float64](t, events[0], "turn_id")
			turn := turnByID(t, store, int64(field[float64](t, events[0], "thread_id")), int64(start))
			if events[len(events)-1].Name != "done" || turn.Status != library.AskTurnDone {
				t.Errorf("after two failed writes: %s, turn %s", events[len(events)-1].Name, turn.Status)
			}
		})
		t.Run("abandoned", func(t *testing.T) {
			faulty := &faultyStore{Store: store, endFails: 1000}
			h := newHarnessWith(t, faulty, opts, append(fullTurn(), fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})...)
			events := h.open(`{"message":"something like Severance"}`).all()
			last := events[len(events)-1]
			threadID, turnID := int64(field[float64](t, events[0], "thread_id")), int64(field[float64](t, events[0], "turn_id"))
			turn := turnByID(t, store, threadID, turnID)
			if last.Name != "error" || field[string](t, last, "code") != recommend.AskErrorInternal || turn.Status != library.AskTurnError {
				t.Fatalf("a turn whose end can't be written: %s %s, turn %s", last.Name, last.Data, turn.Status)
			}
			thread, err := h.svc.Thread(context.Background(), threadID)
			if err != nil || thread.Turns[len(thread.Turns)-1].Error == nil {
				t.Errorf("the abandoned turn reads %+v, %v", thread.Turns, err)
			}
			faulty.set(0, 0)
			if again := h.open(`{"message":"anime"}`).all(); again[len(again)-1].Name != "done" {
				t.Errorf("the next ask: %s", dump(again))
			}
		})
		t.Run("healed by the next ask", func(t *testing.T) {
			faulty := &faultyStore{Store: store, endFails: 1000, abandonFails: 1000}
			h := newHarnessWith(t, faulty, opts, append(fullTurn(), fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})...)
			events := h.open(`{"message":"something like Severance"}`).all()
			threadID, turnID := int64(field[float64](t, events[0], "thread_id")), int64(field[float64](t, events[0], "turn_id"))
			if turn := turnByID(t, store, threadID, turnID); turn.Status != library.AskTurnStreaming {
				t.Fatalf("turn = %s, want it still streaming in the store", turn.Status)
			}
			faulty.set(0, 0)
			if again := h.open(`{"message":"anime"}`).all(); again[len(again)-1].Name != "done" {
				t.Errorf("the next ask: %s", dump(again))
			}
			if turn := turnByID(t, store, threadID, turnID); turn.Status != library.AskTurnError {
				t.Errorf("the stuck turn = %s, want error", turn.Status)
			}
		})
	})
}

// ask-v1's system prompt and tool definitions are pinned: a thread replays
// them as they were. When this fails, R1's tools, show_picks, the system
// prompt or the core's encoding changed: add a new prompt version for new
// threads, and retire ask-v1 from prompts (its threads then answer
// thread_full), since the old definitions aren't kept. Only then update
// the hash.
func TestAsk_PromptVersionPinned(t *testing.T) {
	pinned := map[string]string{
		"ask-v1": "9bddc3bca2e935ba0bb5621bb27dedd9b3525bb6a365cec9e7c78db24865c1cb",
	}
	h := newHarness(t, newMemStore(), fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})
	h.open(`{"message":"hi"}`).all()
	req := h.api.Requests()[0]
	sum := sha256.Sum256(append(append(req.Field("system"), '\n'), req.Field("tools")...))
	if got := hex.EncodeToString(sum[:]); got != pinned[ask.PromptVersion] {
		t.Errorf("%s's system prompt and tools changed (sha256 %s): bump the prompt version, don't just update the hash", ask.PromptVersion, got)
	}
}
