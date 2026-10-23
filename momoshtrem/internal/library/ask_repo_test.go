package library

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/member"
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
	th, tu, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, Title: "Anime", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "anime"})
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
	_, tu2, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, ThreadID: th.ID, Message: "lighter"})
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
	th, tu, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, Title: "One", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, Title: "Two", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "two"}); err != ErrAskTurnStreaming {
		t.Fatalf("a second streaming turn = %v, want ErrAskTurnStreaming", err)
	}
	if threads, _ := r.Threads(ctx, 1, 50); len(threads) != 1 {
		t.Errorf("threads = %d: the refused turn's thread must not stay", len(threads))
	}

	if err := r.CancelTurn(ctx, tu.ID); err != nil {
		t.Fatal(err)
	}
	_, tu2, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, ThreadID: th.ID, Message: "again"})
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
	_, tu3, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, ThreadID: th.ID, Message: "crash"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.FailStreaming(ctx, "internal", []byte(`{"error":{"code":"internal"}}`)); n != 1 || err != nil {
		t.Errorf("FailStreaming = %d, %v", n, err)
	}
	if ok, _ := r.EndTurn(ctx, AskTurnEnd{TurnID: tu3.ID, Status: AskTurnDone, View: []byte(`{}`)}); ok {
		t.Error("a failed turn took a done end")
	}
	if n, _ := r.TurnsSince(ctx, 1, time.Now().Add(-time.Minute)); n != 3 {
		t.Errorf("TurnsSince = %d, want 3 (every status)", n)
	}
	if n, _ := r.TurnsSince(ctx, 1, time.Now().Add(time.Minute)); n != 0 {
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
	th, tu, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, Title: "One", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "one"})
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
	_, tu2, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, ThreadID: th.ID, Message: "two"})
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
		th, tu, err := r.StartTurn(ctx, NewAskTurn{MemberID: 1, Title: "T", Model: "claude-opus-5-5", PromptVersion: "ask-v1", Message: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu.ID, Status: AskTurnDone, View: []byte(`{}`), CostUSD: cost}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, th.ID)
	}
	threads, err := r.Threads(ctx, 1, 2)
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

// Threads and turns are a member's (014): each member has one streaming
// turn, their own list and daily count; a turn can't go into another
// member's thread (the composite FK), nor be a member's no one has; a
// removed member's threads go with them.
func TestAskRepository_PerMember(t *testing.T) {
	r := openAskDB(t)
	ctx := context.Background()
	if _, err := r.db.Exec(`INSERT INTO members (id, name, color) VALUES (2, 'Sam', 'rose')`); err != nil {
		t.Fatal(err)
	}
	start := func(memberID, threadID int64) (*AskThread, *AskTurn, error) {
		return r.StartTurn(ctx, NewAskTurn{MemberID: memberID, ThreadID: threadID, Title: "T", Model: "claude-opus-5-5",
			PromptVersion: "ask-v2", Message: "m"})
	}
	mine, _, err := start(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	sams, samTurn, err := start(2, 0)
	if err != nil {
		t.Fatalf("Sam's turn while Me's streams = %v", err)
	}
	if mine.MemberID != 1 || sams.MemberID != 2 {
		t.Errorf("threads' members = %d, %d", mine.MemberID, sams.MemberID)
	}
	for _, id := range []int64{1, 2} {
		if _, _, err := start(id, 0); !errors.Is(err, ErrAskTurnStreaming) {
			t.Errorf("member %d's second streaming turn = %v, want ErrAskTurnStreaming", id, err)
		}
	}

	// Another member's thread, or a member no one has, is refused, and
	// nothing is stored.
	if _, err := r.EndTurn(ctx, AskTurnEnd{TurnID: samTurn.ID, Status: AskTurnDone, View: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := start(2, mine.ID); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("Sam's turn in Me's thread = %v, want member.ErrUnknown", err)
	}
	if _, _, err := start(9, 0); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("an unknown member's thread = %v, want member.ErrUnknown", err)
	}
	if th, err := r.Thread(ctx, mine.ID); err != nil || th.TurnCount != 1 || th.MemberID != 1 {
		t.Errorf("Me's thread = %+v, %v; want its one turn", th, err)
	}
	var stray int
	if err := r.db.QueryRow(`SELECT count(*) FROM ask_turns WHERE member_id <> (SELECT member_id FROM ask_threads WHERE id = thread_id)`).
		Scan(&stray); err != nil || stray != 0 {
		t.Errorf("turns of another member's thread = %d (%v)", stray, err)
	}

	for id, want := range map[int64]int64{1: mine.ID, 2: sams.ID} {
		threads, err := r.Threads(ctx, id, 50)
		if err != nil || len(threads) != 1 || threads[0].ID != want {
			t.Errorf("member %d's threads = %+v, %v", id, threads, err)
		}
		if n, err := r.TurnsSince(ctx, id, time.Now().Add(-time.Minute)); err != nil || n != 1 {
			t.Errorf("member %d's turns today = %d, %v; want 1", id, n, err)
		}
	}
	if n, err := r.FailStreaming(ctx, "internal", []byte(`{}`)); n != 1 || err != nil {
		t.Errorf("FailStreaming = %d, %v; want Me's one (everyone's)", n, err)
	}

	// A removed member's threads, turns and messages go with them.
	if _, err := r.db.Exec(`DELETE FROM members WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if th, err := r.Thread(ctx, sams.ID); th != nil || err != nil {
		t.Errorf("Sam's thread after the removal = %+v, %v", th, err)
	}
	if th, err := r.Thread(ctx, mine.ID); th == nil || err != nil {
		t.Errorf("Me's thread after Sam's removal = %+v, %v", th, err)
	}
}

// A turn's end and its member's removal meet without a deadlock: EndTurn
// locks the thread before the turn, the order the removal's cascade takes
// (members, ask_threads, ask_turns), so the removal waits for the end to
// commit, then takes the thread with its turns and messages.
func TestAskRepository_EndTurnMeetsMemberRemoval(t *testing.T) {
	r := openAskDB(t)
	ctx := context.Background()
	exec := func(query string) {
		t.Helper()
		if _, err := r.db.Exec(query); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}
	exec(`INSERT INTO members (id, name, color) VALUES (2, 'Sam', 'rose')`)
	th, tu, err := r.StartTurn(ctx, NewAskTurn{MemberID: 2, Title: "T", Model: "claude-opus-5-5", PromptVersion: "ask-v2", Message: "m"})
	if err != nil {
		t.Fatal(err)
	}

	// The end's turn update waits, inside its transaction, for a lock the
	// test holds.
	exec(`CREATE FUNCTION hold_end() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM pg_advisory_xact_lock(4246); RETURN NEW; END $$`)
	exec(`CREATE TRIGGER hold_end AFTER UPDATE ON ask_turns FOR EACH ROW EXECUTE FUNCTION hold_end()`)
	lock, err := r.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, `SELECT pg_advisory_lock(4246)`); err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() {
		ok, err := r.EndTurn(ctx, AskTurnEnd{TurnID: tu.ID, Status: AskTurnDone, View: []byte(`{}`), CostUSD: 0.02,
			Messages: []AskMessage{{Role: "user", Content: []byte(`[{"type":"text","text":"m"}]`)},
				{Role: "assistant", Content: []byte(`[{"type":"text","text":"ok"}]`)}}})
		if err == nil && !ok {
			err = errors.New("the turn didn't take its end")
		}
		ended <- err
	}()
	for deadline := time.Now().Add(5 * time.Second); ; {
		var waiting int
		if err := r.db.QueryRow(`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the end never reached the held lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	deleted := make(chan error, 1)
	go func() {
		_, err := r.db.ExecContext(ctx, `DELETE FROM members WHERE id = 2`)
		deleted <- err
	}()
	select {
	case err := <-deleted:
		t.Fatalf("the removal = %v while the end is held, want it to wait", err)
	case <-time.After(time.Second):
	}
	if _, err := lock.ExecContext(ctx, `SELECT pg_advisory_unlock(4246)`); err != nil {
		t.Fatal(err)
	}
	for name, done := range map[string]chan error{"EndTurn": ended, "the removal": deleted} {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("%s = %v, want it to succeed (no deadlock)", name, err)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("%s never finished", name)
		}
	}

	if got, err := r.Thread(ctx, th.ID); got != nil || err != nil {
		t.Errorf("Sam's thread after the removal = %+v, %v", got, err)
	}
	for _, table := range []string{"ask_turns", "ask_messages"} {
		var n int
		if err := r.db.QueryRow(`SELECT count(*) FROM `+table+` WHERE thread_id = $1`, th.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s keeps %d rows of the removed member's thread (%v)", table, n, err)
		}
	}
}
