package ask_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
)

// Ask per household member (#246): each member's threads, slot, daily
// limit and taste are theirs. The harness's household is Me (1, the
// default without a header) and Sam (2).

// waitRequests waits until the fake API has had n requests.
func waitRequests(t *testing.T, h *harness, n int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); len(h.api.Requests()) < n; {
		if time.Now().After(deadline) {
			t.Fatalf("API requests = %d, want %d", len(h.api.Requests()), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// startOf is a stream's start event's thread and turn.
func startOf(t *testing.T, events []sseEvent) (threadID, turnID int64) {
	t.Helper()
	start := events[0]
	if start.Name != "start" {
		t.Fatalf("first event = %s %s, want start", start.Name, start.Data)
	}
	return field[int64](t, start, "thread_id"), field[int64](t, start, "turn_id")
}

// Two members ask at once: Sam's turn runs while Me's streams, and each
// still has one turn at a time (ask_busy is the asker's, the store's
// one-streaming index too).
func TestAsk_MembersStreamAtOnce(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store,
			fake.Response{Blocks: []fake.Block{fake.Text("…")}, Hang: true}, // Me's
			fake.Response{Blocks: []fake.Block{fake.Text("Hi, Sam.")}},
		)
		mine := h.open(`{"message":"anime"}`)
		myThread, myTurn := startOf(t, mine.until(t, named("start")))
		waitRequests(t, h, 1)
		if e := h.refused(`{"message":"something else"}`); e.status != 409 || e.code != "ask_busy" {
			t.Errorf("Me's second ask = %+v, want ask_busy", e)
		}

		events := h.openAs(2, `{"message":"anime"}`).all()
		if last := events[len(events)-1]; last.Name != "done" {
			t.Fatalf("Sam's ask while Me's streams: %s", dump(events))
		}
		samThread, samTurn := startOf(t, events)
		if samThread == myThread {
			t.Errorf("Sam's ask went into Me's thread %d", myThread)
		}
		if turn := turnByID(t, store, samThread, samTurn); turn.Status != library.AskTurnDone {
			t.Errorf("Sam's turn = %s, want done", turn.Status)
		}
		if turn := turnByID(t, store, myThread, myTurn); turn.Status != library.AskTurnStreaming {
			t.Errorf("Me's turn = %s, want it still streaming", turn.Status)
		}

		mine.cancel()
		waitTurn(t, store, myThread, myTurn, library.AskTurnCancelled)
	})
}

// The daily limit is the asker's: Me's 30 turns today leave Sam's 30.
func TestAsk_MembersDailyLimit(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		seed(t, store, 0, recommend.AskDailyLimit)
		h := newHarness(t, store, fake.Response{Blocks: []fake.Block{fake.Text("Hi, Sam.")}})
		if e := h.refused(`{"message":"anime"}`); e.status != 429 || e.code != "ask_limit" {
			t.Errorf("Me's 31st ask = %+v, want ask_limit", e)
		}
		if e := h.refusedAs(1, `{"message":"anime"}`); e.code != "ask_limit" {
			t.Errorf("Me's 31st ask, named = %+v, want ask_limit", e)
		}
		if events := h.openAs(2, `{"message":"anime"}`).all(); events[len(events)-1].Name != "done" {
			t.Errorf("Sam's first ask: %s", dump(events))
		}
	})
}

// Threads are private: another member's is not found, to read and to
// follow up, and each lists only their own.
func TestAsk_MembersThreadsArePrivate(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		mine := seed(t, store, 0, 1)
		h := newHarness(t, store, fake.Response{Blocks: []fake.Block{fake.Text("Hi, Sam.")}})

		if status, body := h.getAs(2, "/api/recommendations/threads"); status != 200 || string(body) != `{"items":[]}` {
			t.Errorf("Sam's threads = %d %s, want none", status, body)
		}
		if status, body := h.getAs(2, "/api/recommendations/threads/"+itoa(mine)); status != 404 {
			t.Errorf("Sam reading Me's thread = %d %s, want 404", status, body)
		}
		if e := h.refusedAs(2, `{"thread_id":`+itoa(mine)+`,"message":"more"}`); e.status != 404 || e.code != "not_found" {
			t.Errorf("Sam following up Me's thread = %+v, want not_found", e)
		}

		events := h.openAs(2, `{"message":"anime"}`).all()
		sams, _ := startOf(t, events)
		var list struct {
			Items []recommend.ThreadSummary `json:"items"`
		}
		status, body := h.getAs(2, "/api/recommendations/threads")
		if json.Unmarshal(body, &list) != nil || status != 200 || len(list.Items) != 1 || list.Items[0].ID != sams {
			t.Errorf("Sam's threads = %d %s, want their one", status, body)
		}
		status, body = h.get("/api/recommendations/threads")
		if json.Unmarshal(body, &list) != nil || status != 200 || len(list.Items) != 1 || list.Items[0].ID != mine {
			t.Errorf("Me's threads = %d %s, want Me's one", status, body)
		}
		if status, _ := h.get("/api/recommendations/threads/" + itoa(sams)); status != 404 {
			t.Errorf("Me reading Sam's thread = %d, want 404", status)
		}
		if status, _ := h.getAs(2, "/api/recommendations/threads/"+itoa(sams)); status != 200 {
			t.Errorf("Sam reading their thread = %d", status)
		}
		if n := len(h.api.Requests()); n != 1 {
			t.Errorf("API requests = %d, want Sam's one ask only", n)
		}
		if status, body := h.getAs(9, "/api/recommendations/threads"); status != 400 || !bytes.Contains(body, []byte(`"unknown_member"`)) {
			t.Errorf("an unknown member's threads = %d %s, want 400 unknown_member", status, body)
		}
	})
}

// Each member's turns read their own taste: what the model is sent for
// Sam never holds Me's profile, nor Me's Sam's, even with a turn of Me's
// before and after. Each member's parts are built once, and their profile
// once within the hour, as wireAsk's tools.Cached keeps it.
func TestAsk_MembersTasteIsTheirs(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		profileTurn := []fake.Response{
			{Blocks: []fake.Block{toolUse("toolu_1", "get_taste_profile", map[string]any{})}},
			{Blocks: []fake.Block{fake.Text("Here's what I know.")}},
		}
		h := newHarness(t, store, slices.Concat(profileTurn, profileTurn, profileTurn)...)
		var details []string
		for _, asker := range []int64{0, 2, 1} {
			for _, e := range h.openAs(asker, `{"message":"what do I like"}`).all() {
				if e.Name == "status" && field[string](t, e, "step") == "history" && field[string](t, e, "state") == "done" {
					details = append(details, field[string](t, e, "detail"))
				}
			}
		}
		if want := []string{"2,184 plays · you loved Severance, The Bear, Ted Lasso",
			"311 plays · you loved Frieren: Beyond Journey's End, Arcane",
			"2,184 plays · you loved Severance, The Bear, Ted Lasso"}; !slices.Equal(details, want) {
			t.Errorf("history steps = %q\nwant %q", details, want)
		}

		requests := h.api.Requests()
		if len(requests) != 6 {
			t.Fatalf("API requests = %d, want 6", len(requests))
		}
		for i, r := range requests {
			sams := i == 2 || i == 3
			for _, title := range []string{"Ted Lasso", "The Bear", "Everything Everywhere"} {
				if sams && bytes.Contains(r.Body, []byte(title)) {
					t.Errorf("Sam's request %d holds Me's %q", i, title)
				}
			}
			for _, title := range []string{"Frieren", "Arcane"} {
				if !sams && bytes.Contains(r.Body, []byte(title)) {
					t.Errorf("Me's request %d holds Sam's %q", i, title)
				}
			}
		}
		if !bytes.Contains(requests[3].Body, []byte("Frieren: Beyond Journey's End")) || !bytes.Contains(requests[5].Body, []byte("Ted Lasso")) {
			t.Error("a profile wasn't sent to its member's turn")
		}
		for _, id := range []int64{1, 2} {
			if built, profiles := h.counts(id); built != 1 || profiles != 1 {
				t.Errorf("member %d: parts built %d times, profile %d; want once each", id, built, profiles)
			}
		}
	})
}

// Removing a member (member.Service.Delete, its OnRemoving listener)
// cancels their turn in flight before their row goes, the turn's end
// written, and drops their parts; the other member's turn streams on.
func TestAsk_ForgetMember(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store,
			fake.Response{Blocks: []fake.Block{fake.Text("…")}, Hang: true}, // Me's
			fake.Response{Blocks: []fake.Block{toolUse("toolu_1", "get_taste_profile", map[string]any{})},
				Usage: fake.Usage{InputTokens: 1000, OutputTokens: 50}},
			fake.Response{Blocks: []fake.Block{fake.Text("…")}, Hang: true}, // Sam's second call
		)
		mine := h.open(`{"message":"anime"}`)
		myThread, myTurn := startOf(t, mine.until(t, named("start")))
		waitRequests(t, h, 1)
		sams := h.openAs(2, `{"message":"anime"}`)
		samThread, samTurn := startOf(t, sams.until(t, named("start")))
		waitRequests(t, h, 3)

		if err := h.members.Delete(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
		turn := turnByID(t, store, samThread, samTurn)
		if turn.Status != library.AskTurnCancelled || len(turn.View) == 0 {
			t.Errorf("Sam's turn after the removal = %s (view %s), want cancelled with its view", turn.Status, turn.View)
		}
		for _, e := range sams.all() {
			if e.Name == "done" || e.Name == "error" {
				t.Errorf("Sam's cancelled stream sent %s", e.Name)
			}
		}
		if rows := h.recorder.forRef(samTurn); len(rows) == 0 {
			t.Error("Sam's spend wasn't recorded")
		}
		if turn := turnByID(t, store, myThread, myTurn); turn.Status != library.AskTurnStreaming {
			t.Errorf("Me's turn = %s, want it still streaming", turn.Status)
		}
		// Sam's parts went with them: reading as Sam (as nothing does once
		// their row is gone) builds new ones.
		if _, err := h.svc.Thread(memberCtx(2), samThread); err != nil {
			t.Fatal(err)
		}
		if built, _ := h.counts(2); built != 2 {
			t.Errorf("Sam's parts built %d times, want again after the removal", built)
		}
		if built, _ := h.counts(1); built != 1 {
			t.Errorf("Me's parts built %d times, want once", built)
		}

		mine.cancel()
		waitTurn(t, store, myThread, myTurn, library.AskTurnCancelled)
	})
}
