package library

import (
	"context"
	"testing"
	"time"
)

// openAskDB is a migrated database in a schema of its own: the
// one-streaming index and the daily count are the test's.
func openAskDB(t *testing.T) *AskRepository {
	t.Helper()
	db := openBareSchemaDB(t, "ask")
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewAskRepository(db)
}

// Messages come back byte for byte, whatever their whitespace, key order
// or escapes (ask_messages.content is json, not jsonb): a replay must be
// identical.
func TestAskRepository_MessagesAreExact(t *testing.T) {
	r := openAskDB(t)
	ctx := context.Background()
	th, tu, err := r.StartTurn(ctx, NewAskTurn{Title: "Anime", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "anime"})
	if err != nil {
		t.Fatal(err)
	}
	contents := []string{
		`[{"type":"text","text":"Today is Friday.\n\n<ask>anime</ask>"}]`,
		`[{"type":"thinking","thinking":"","signature":"sig/+=="},{"type":"tool_use","id":"toolu_1","name":"get_candidates","input":{"z":1,"a":[ ],"é":"é"}}]`,
		`[ {"tool_use_id":"toolu_1","type":"tool_result","content":"{\"items\":[]}"} ]`,
		`[{"type":"text","text":"Enjoy 🐈"}]`,
	}
	var msgs []AskMessage
	for i, c := range contents {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, AskMessage{Role: role, Content: []byte(c)})
	}
	ok, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu.ID, Status: AskTurnDone, View: []byte(`{"intro":"Hi"}`), CostUSD: 0.0213, Messages: msgs})
	if err != nil || !ok {
		t.Fatalf("EndTurn = %v, %v", ok, err)
	}
	got, err := r.Messages(ctx, th.ID)
	if err != nil || len(got) != len(contents) {
		t.Fatalf("Messages = %d, %v", len(got), err)
	}
	for i, m := range got {
		if string(m.Content) != contents[i] || m.Role != msgs[i].Role {
			t.Errorf("message %d = %s %s\nwant %s %s", i, m.Role, m.Content, msgs[i].Role, contents[i])
		}
	}

	// A second done turn appends after them.
	_, tu2, err := r.StartTurn(ctx, NewAskTurn{ThreadID: th.ID, Message: "lighter"})
	if err != nil || tu2.Position != 1 {
		t.Fatalf("second turn = %+v, %v", tu2, err)
	}
	if _, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu2.ID, Status: AskTurnDone, View: []byte(`{}`),
		Messages: []AskMessage{{Role: "user", Content: []byte(`[{"type":"text","text":"lighter"}]`)}, {Role: "assistant", Content: []byte(`[{"type":"text","text":"ok"}]`)}}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Messages(ctx, th.ID); len(got) != 6 || string(got[4].Content) != `[{"type":"text","text":"lighter"}]` {
		t.Errorf("after the second turn: %d messages", len(got))
	}
	thread, err := r.Thread(ctx, th.ID)
	if err != nil || thread.TurnCount != 2 || thread.Title != "Anime" || thread.PromptVersion != "ask-v1" {
		t.Errorf("thread = %+v, %v", thread, err)
	}
	turns, _ := r.Turns(ctx, th.ID)
	if len(turns) != 2 || turns[0].CostUSD != 0.0213 || string(turns[0].View) != `{"intro": "Hi"}` || turns[0].FinishedAt == nil {
		t.Errorf("turns = %+v", turns)
	}
}

// One turn streams at a time, in any thread; a cancelled turn frees the
// slot at once and takes only a cancelled end; only a done turn stores
// messages.
func TestAskRepository_TurnLifecycle(t *testing.T) {
	r := openAskDB(t)
	ctx := context.Background()
	th, tu, err := r.StartTurn(ctx, NewAskTurn{Title: "One", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.StartTurn(ctx, NewAskTurn{Title: "Two", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "two"}); err != ErrAskTurnStreaming {
		t.Fatalf("a second streaming turn = %v, want ErrAskTurnStreaming", err)
	}
	if threads, _ := r.Threads(ctx, 50); len(threads) != 1 {
		t.Errorf("threads = %d: the refused turn's thread must not stay", len(threads))
	}

	if err := r.CancelTurn(ctx, tu.ID); err != nil {
		t.Fatal(err)
	}
	_, tu2, err := r.StartTurn(ctx, NewAskTurn{ThreadID: th.ID, Message: "again"})
	if err != nil {
		t.Fatalf("after a cancel: %v", err)
	}
	// The cancelled turn takes its view and cost, never done or messages.
	if ok, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu.ID, Status: AskTurnDone, View: []byte(`{}`),
		Messages: []AskMessage{{Role: "user", Content: []byte(`[]`)}}}); ok || err != nil {
		t.Errorf("done after cancel = %v, %v", ok, err)
	}
	if ok, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu.ID, Status: AskTurnCancelled, View: []byte(`{"steps":[]}`), CostUSD: 0.01}); !ok || err != nil {
		t.Errorf("cancelled end = %v, %v", ok, err)
	}
	if _, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu2.ID, Status: AskTurnError, ErrorCode: "upstream", View: []byte(`{}`),
		Messages: []AskMessage{{Role: "user", Content: []byte(`[]`)}}}); err == nil {
		t.Error("an error end with messages was accepted")
	}
	if ok, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu2.ID, Status: AskTurnError, ErrorCode: "upstream", View: []byte(`{}`)}); !ok || err != nil {
		t.Errorf("error end = %v, %v", ok, err)
	}
	if msgs, _ := r.Messages(ctx, th.ID); len(msgs) != 0 {
		t.Errorf("messages = %d, want none", len(msgs))
	}
	turns, _ := r.Turns(ctx, th.ID)
	if len(turns) != 2 || turns[0].Status != AskTurnCancelled || turns[0].CostUSD != 0.01 || turns[1].Status != AskTurnError || turns[1].ErrorCode != "upstream" {
		t.Errorf("turns = %+v", turns)
	}

	// A crash's streaming turn ends as an error at startup.
	_, tu3, err := r.StartTurn(ctx, NewAskTurn{ThreadID: th.ID, Message: "crash"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.FailStreaming(ctx, "internal", []byte(`{"error":{"code":"internal"}}`)); n != 1 || err != nil {
		t.Errorf("FailStreaming = %d, %v", n, err)
	}
	if ok, _ := r.EndTurn(ctx, AskTurnEnd{TurnID: tu3.ID, Status: AskTurnDone, View: []byte(`{}`)}); ok {
		t.Error("a failed turn took a done end")
	}
	if n, _ := r.TurnsSince(ctx, time.Now().Add(-time.Minute)); n != 3 {
		t.Errorf("TurnsSince = %d, want 3 (every status)", n)
	}
	if n, _ := r.TurnsSince(ctx, time.Now().Add(time.Minute)); n != 0 {
		t.Errorf("TurnsSince later = %d", n)
	}
	if mean, err := r.MeanDoneCost(ctx, 20); mean != nil || err != nil {
		t.Errorf("MeanDoneCost without a done turn = %v, %v", mean, err)
	}
}

// AbandonTurn, the last resort when a turn's end can't be written, frees
// the one-streaming index with the status alone, and leaves an ended turn
// as it is.
func TestAskRepository_AbandonTurn(t *testing.T) {
	r := openAskDB(t)
	ctx := context.Background()
	th, tu, err := r.StartTurn(ctx, NewAskTurn{Title: "One", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.AbandonTurn(ctx, tu.ID); err != nil {
		t.Fatal(err)
	}
	turns, _ := r.Turns(ctx, th.ID)
	if turns[0].Status != AskTurnError || turns[0].ErrorCode != "internal" || turns[0].View != nil || turns[0].FinishedAt == nil {
		t.Errorf("abandoned turn = %+v", turns[0])
	}
	_, tu2, err := r.StartTurn(ctx, NewAskTurn{ThreadID: th.ID, Message: "two"})
	if err != nil {
		t.Fatalf("after abandoning: %v", err)
	}
	if _, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu2.ID, Status: AskTurnDone, View: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if err := r.AbandonTurn(ctx, tu2.ID); err != nil {
		t.Fatal(err)
	}
	if turns, _ := r.Turns(ctx, th.ID); turns[1].Status != AskTurnDone {
		t.Errorf("abandoning a done turn made it %s", turns[1].Status)
	}
}

// The threads list is the newest updated first, with turn counts; the
// mean cost is over the newest done turns.
func TestAskRepository_ListAndMean(t *testing.T) {
	r := openAskDB(t)
	ctx := context.Background()
	var ids []int64
	for _, cost := range []float64{0.01, 0.02, 0.06} {
		th, tu, err := r.StartTurn(ctx, NewAskTurn{Title: "T", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu.ID, Status: AskTurnDone, View: []byte(`{}`), CostUSD: cost}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, th.ID)
	}
	threads, err := r.Threads(ctx, 2)
	if err != nil || len(threads) != 2 || threads[0].ID != ids[2] || threads[1].ID != ids[1] || threads[0].TurnCount != 1 {
		t.Errorf("threads = %+v, %v", threads, err)
	}
	if mean, _ := r.MeanDoneCost(ctx, 2); mean == nil || *mean < 0.0399 || *mean > 0.0401 {
		t.Errorf("mean of the newest 2 = %v, want 0.04", mean)
	}
	if missing, err := r.Thread(ctx, 999999); missing != nil || err != nil {
		t.Errorf("unknown thread = %v, %v", missing, err)
	}
}
