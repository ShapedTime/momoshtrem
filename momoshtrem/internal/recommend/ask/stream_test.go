package ask_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
)

// The SSE contract (PLAN §6 "Ask momoshtrem"): start, a status pair per
// step as its first tool starts (history, web, tmdb, then write at
// show_picks), the intro, an item per pick as it resolves (library,
// watchlist and feedback joined), write's end, then done with the
// refinements and the usage. The closing line after show_picks isn't
// shown. The turn is stored done, with its messages, before done is sent.
func TestAsk_StreamContract(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store, fullTurn()...)
		h.titles.set(library.TitleState{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 2002}, OnWatchlist: true, Feedback: "like"})
		s := h.open(`{"message":"  something like Severance but lighter "}`)
		if got := s.resp.Header.Get("Content-Type"); got != "text/event-stream" {
			t.Errorf("Content-Type = %q", got)
		}
		events := s.all()

		want := []sseEvent{
			{"start", `{"thread_id":1,"turn_id":1,"title":"Something like Severance but lighter"}`},
			{"status", `{"step":"history","state":"active","detail":null}`},
			{"status", `{"step":"history","state":"done","detail":"2,184 plays · you loved Severance, The Bear, Ted Lasso"}`},
			{"status", `{"step":"web","state":"active","detail":null}`},
			{"status", `{"step":"web","state":"done","detail":"1 search"}`},
			{"status", `{"step":"tmdb","state":"active","detail":null}`},
			{"status", `{"step":"tmdb","state":"done","detail":"23 candidates · 6 already seen"}`},
			{"status", `{"step":"write","state":"active","detail":null}`},
			{"text", `{"delta":"Two comedies with Severance's office dread, only warmer."}`},
			{"item", `{"media_type":"tv","tmdb_id":1001,"title":"Mythic Quest","year":2020,"year_end":2025,"poster_path":"/mq.jpg",` +
				`"seasons":4,"runtime_minutes":30,"vote_average":7.6,"release_date":"2020-02-07","badge":null,` +
				`"reason":"The same deadpan office absurdity as Severance, half the dread.",` +
				`"library":{"library_id":7,"availability_stage":"ready","episodes_total":null,"episodes_ready":null},` +
				`"on_watchlist":false,"feedback":null,"match_tag":"Workplace satire · lighter"}`},
			{"item", `{"media_type":"movie","tmdb_id":2002,"title":"The Menu","year":2022,"year_end":null,"poster_path":"/menu.jpg",` +
				`"seasons":null,"runtime_minutes":107,"vote_average":7.2,"release_date":"2022-11-17","badge":null,` +
				`"reason":"A sharp, contained satire like the ones you rated 9.","library":null,"on_watchlist":true,"feedback":"like",` +
				`"match_tag":"Dark comedy · one night"}`},
			{"status", `{"step":"write","state":"done","detail":"2 picks"}`},
		}
		if len(events) != len(want)+1 {
			t.Fatalf("events = %d:\n%s", len(events), dump(events))
		}
		for i, w := range want {
			if events[i] != w {
				t.Errorf("event %d =\n  %s %s\nwant\n  %s %s", i, events[i].Name, events[i].Data, w.Name, w.Data)
			}
		}

		done := events[len(events)-1]
		var d struct {
			ThreadID    int64    `json:"thread_id"`
			TurnID      int64    `json:"turn_id"`
			ElapsedMS   *int64   `json:"elapsed_ms"`
			Refinements []string `json:"refinements"`
			Usage       struct {
				InputTokens  int64   `json:"input_tokens"`
				OutputTokens int64   `json:"output_tokens"`
				WebSearches  int     `json:"web_searches"`
				CostUSD      float64 `json:"cost_usd"`
			} `json:"usage"`
		}
		if done.Name != "done" || json.Unmarshal([]byte(done.Data), &d) != nil {
			t.Fatalf("last event = %s %s, want done", done.Name, done.Data)
		}
		var cost float64
		rows := h.recorder.forRef(1)
		for _, r := range rows {
			cost += r.usage.CostUSD
			if r.feature != library.AIFeatureAsk {
				t.Errorf("usage feature = %q, want ask", r.feature)
			}
		}
		if d.ThreadID != 1 || d.TurnID != 1 || d.ElapsedMS == nil || strings.Join(d.Refinements, "|") != "Lighter|Only movies|Older" ||
			d.Usage.InputTokens != 15600 || d.Usage.OutputTokens != 512 || d.Usage.WebSearches != 1 || len(rows) != 4 ||
			math.Abs(d.Usage.CostUSD-cost) > 1e-9 || cost <= 0 {
			t.Errorf("done = %s (usage rows %d, $%v)", done.Data, len(rows), cost)
		}

		// Stored before done was sent: done, its view, and the turn's 8
		// messages (the ask, 4 replies, 3 tool results).
		turn := turnByID(t, store, 1, 1)
		msgs, _ := store.Messages(context.Background(), 1)
		if turn.Status != library.AskTurnDone || math.Abs(turn.CostUSD-cost) > 1e-6 || len(msgs) != 8 {
			t.Fatalf("turn = %s $%v, %d messages", turn.Status, turn.CostUSD, len(msgs))
		}
		var v struct {
			Steps       []recommend.Step `json:"steps"`
			Intro       string           `json:"intro"`
			Items       []map[string]any `json:"items"`
			Refinements []string         `json:"refinements"`
			Error       *json.RawMessage `json:"error"`
			ElapsedMS   *int64           `json:"elapsed_ms"`
		}
		if err := json.Unmarshal(turn.View, &v); err != nil || len(v.Steps) != 4 || v.Intro == "" || len(v.Items) != 2 ||
			len(v.Refinements) != 3 || v.Error != nil || v.ElapsedMS == nil {
			t.Errorf("view = %s (%v)", turn.View, err)
		}
		for _, step := range v.Steps {
			if step.State != recommend.StepDone || step.Detail == nil {
				t.Errorf("stored step %+v, want done with a detail", step)
			}
		}
	})
}

// The first request: the thread's model, R1's tools then show_picks (all
// strict, show_picks without a union), web search and fetch, drop_block,
// and the user turn with today's date and the ask.
func TestAsk_Request(t *testing.T) {
	h := newHarnessWith(t, newMemStore(), harnessOpts{model: recommend.ModelSonnet}, fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})
	h.open(`{"message":"anime with a Frieren vibe"}`).all()
	req := h.api.Requests()[0]
	var body struct {
		Model    string `json:"model"`
		Thinking struct {
			Type         string `json:"type"`
			BlockBinding struct {
				PrefixMismatchBehavior string `json:"prefix_mismatch_behavior"`
			} `json:"block_binding"`
		} `json:"thinking"`
		Tools []struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Strict  bool   `json:"strict"`
			MaxUses int    `json:"max_uses"`
		} `json:"tools"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range body.Tools {
		names = append(names, tl.Name)
		if tl.Type == "" && !tl.Strict {
			t.Errorf("tool %s isn't strict", tl.Name)
		}
	}
	if got := strings.Join(names, ","); got != "get_taste_profile,get_candidates,tmdb_search,tmdb_details,check_seen,show_picks,web_search,web_fetch" {
		t.Errorf("tools = %s", got)
	}
	if body.Tools[6].MaxUses != 4 || body.Tools[7].MaxUses != 2 {
		t.Errorf("web max_uses = %d, %d", body.Tools[6].MaxUses, body.Tools[7].MaxUses)
	}
	if body.Model != recommend.ModelSonnet || body.Thinking.Type != "adaptive" || body.Thinking.BlockBinding.PrefixMismatchBehavior != "drop_block" {
		t.Errorf("model %s, thinking %+v", body.Model, body.Thinking)
	}
	today := time.Now().Format("Monday, 2 January 2006")
	if len(body.Messages) != 1 || body.Messages[0].Content[0].Text != "Today is "+today+".\n\n<ask>anime with a Frieren vibe</ask>" {
		t.Errorf("messages = %+v", body.Messages)
	}
	thread, _ := h.svc.Thread(context.Background(), 1)
	if thread.Model != recommend.ModelSonnet {
		t.Errorf("thread model = %s", thread.Model)
	}
}

// Events are flushed one at a time: the client reads the first steps while
// the API is still answering.
func TestAsk_StreamsAsItGoes(t *testing.T) {
	h := newHarness(t, newMemStore(),
		fake.Response{Blocks: []fake.Block{toolUse("toolu_1", "get_taste_profile", map[string]any{})}},
		fake.Response{Blocks: []fake.Block{fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "x"})}, Hang: true},
	)
	s := h.open(`{"message":"anime"}`)
	got := s.until(t, func(e sseEvent) bool { return e.Data == `{"step":"web","state":"active","detail":null}` })
	if len(got) != 4 || got[0].Name != "start" {
		t.Errorf("before the API finished: %s", dump(got))
	}
}

// A refinement chip re-ranks; its request starts with the first turn's
// bytes (the thread's messages replayed unchanged, thinking blocks
// included), with the same system prompt and tools.
func TestAsk_AppendOnlyReplay(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		script := append(fullTurn(),
			fake.Response{Blocks: []fake.Block{fake.Thinking("sig_5"), toolUse("toolu_5", "check_seen",
				map[string]any{"titles": []map[string]any{{"media_type": "tv", "tmdb_id": 1003}, {"media_type": "tv", "tmdb_id": 1004}}})}},
			fake.Response{Blocks: []fake.Block{toolUse("toolu_6", "show_picks", picksInput("", nil,
				pick("tv", 1002, "Workplace comedy · gentler", "Cosier than Severance, with The Bear's ensemble warmth.")))}},
			fake.Response{Blocks: []fake.Block{fake.Text("Lighter it is.")}},
		)
		h := newHarness(t, store, script...)
		h.open(`{"message":"something like Severance but lighter"}`).all()
		events := h.open(`{"thread_id":1,"message":"lighter"}`).all()

		var names []string
		for _, e := range events {
			if e.Name == "status" {
				names = append(names, field[string](t, e, "step")+":"+field[string](t, e, "state"))
			} else {
				names = append(names, e.Name)
			}
		}
		if got := strings.Join(names, " "); got != "start rerank:active rerank:done write:active item write:done done" {
			t.Errorf("refinement events = %s\n%s", got, dump(events))
		}
		if got := field[*string](t, events[2], "detail"); got == nil || *got != "2 checked" {
			t.Errorf("rerank detail = %v", got)
		}

		reqs := h.api.Requests()
		if len(reqs) != 7 {
			t.Fatalf("requests = %d", len(reqs))
		}
		first, next := reqs[3].Field("messages"), reqs[4].Field("messages")
		if !bytes.HasPrefix(next, first[:len(first)-1]) {
			t.Errorf("turn 2 doesn't start with turn 1's bytes:\n%s\n%s", first, next)
		}
		for _, f := range []string{"system", "tools", "thinking", "model"} {
			if !bytes.Equal(reqs[0].Field(f), reqs[4].Field(f)) {
				t.Errorf("%s changed between the turns:\n%s\n%s", f, reqs[0].Field(f), reqs[4].Field(f))
			}
		}
		msgs := reqs[4].Messages()
		if len(msgs) != 9 || !bytes.Contains(msgs[7], []byte("Enjoy these two.")) || !bytes.Contains(msgs[8], []byte(`\u003crefine\u003elighter\u003c/refine\u003e`)) ||
			!bytes.Contains(msgs[1], []byte(`"signature":"sig_1"`)) {
			t.Errorf("turn 2's messages:\n%s", reqs[4].Field("messages"))
		}
		stored, _ := store.Messages(context.Background(), 1)
		if len(stored) != 8+6 {
			t.Errorf("stored messages = %d, want 14", len(stored))
		}
	})
}

// A disconnect cancels the turn: the API call is cut off, the turn ends
// cancelled with what it spent recorded, the slot is free before that
// usage is written, and the next turn replays nothing of it (no orphaned
// tool_use).
func TestAsk_Disconnect(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store,
			fake.Response{Blocks: []fake.Block{fake.Thinking("sig_1"), toolUse("toolu_1", "get_taste_profile", map[string]any{})},
				Usage: fake.Usage{InputTokens: 3000, OutputTokens: 100}},
			fake.Response{Blocks: []fake.Block{fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "x"})},
				Usage: fake.Usage{InputTokens: 800}, Hang: true},
			fake.Response{Blocks: []fake.Block{fake.Text("Hello.")}},
			fake.Response{Blocks: []fake.Block{fake.Text("Here.")}},
		)
		// The cut-off call's usage waits until hold closes.
		hold := make(chan struct{})
		h.recorder.block = func(ref int64, n int) chan struct{} {
			if ref == 1 && n == 2 {
				return hold
			}
			return nil
		}

		s := h.open(`{"message":"anime"}`)
		s.until(t, func(e sseEvent) bool { return e.Data == `{"step":"web","state":"active","detail":null}` })
		s.cancel() // Stop

		select {
		case <-h.upstream.cancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("the API call wasn't cut off")
		}
		deadline := time.Now().Add(5 * time.Second)
		for turnByID(t, store, 1, 1).Status != library.AskTurnCancelled {
			if time.Now().After(deadline) {
				t.Fatal("the turn wasn't marked cancelled")
			}
			time.Sleep(5 * time.Millisecond)
		}
		// Its usage isn't written yet, and the slot is already free.
		if rows := h.recorder.forRef(1); len(rows) != 1 {
			t.Fatalf("usage rows = %d before the cut-off call's, want 1", len(rows))
		}
		other := h.open(`{"message":"a 90-minute thriller"}`).all()
		if other[len(other)-1].Name != "done" {
			t.Fatalf("an ask right after Stop: %s", dump(other))
		}
		close(hold)

		turn := waitTurn(t, store, 1, 1, library.AskTurnCancelled)
		rows := h.recorder.forRef(1)
		if len(rows) != 2 || rows[1].usage.Input != 800 {
			t.Fatalf("usage rows = %+v, want the cut-off call's too", rows)
		}
		if want := rows[0].usage.CostUSD + rows[1].usage.CostUSD; math.Abs(turn.CostUSD-want) > 1e-6 {
			t.Errorf("turn cost = %v, want %v", turn.CostUSD, want)
		}
		var v struct {
			Steps []recommend.Step `json:"steps"`
			Error any              `json:"error"`
		}
		if err := json.Unmarshal(turn.View, &v); err != nil || len(v.Steps) != 1 || v.Steps[0].Step != "history" ||
			v.Steps[0].State != "done" || v.Error != nil {
			t.Errorf("view = %s, want history done only (web never finished)", turn.View)
		}
		if msgs, _ := store.Messages(context.Background(), 1); len(msgs) != 0 {
			t.Errorf("a cancelled turn stored %d messages", len(msgs))
		}

		// A follow-up replays nothing of it.
		h.open(`{"thread_id":1,"message":"older ones"}`).all()
		reqs := h.api.Requests()
		last := reqs[len(reqs)-1]
		if n := len(last.Messages()); n != 1 || bytes.Contains(last.Field("messages"), []byte("tool_use")) {
			t.Errorf("the follow-up sent %d messages: %s", n, last.Field("messages"))
		}
	})
}

// The web closes the stream as soon as it reads done: the turn was ended
// before done was written, so the disconnect changes nothing.
func TestAsk_DisconnectAfterDone(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store, fullTurn()...)
		s := h.open(`{"message":"something like Severance"}`)
		s.until(t, named("done"))
		// Ended and stored before done reached the client.
		if turn := turnByID(t, store, 1, 1); turn.Status != library.AskTurnDone {
			t.Fatalf("turn = %s when done arrived", turn.Status)
		}
		s.cancel()
		time.Sleep(50 * time.Millisecond)
		turn := turnByID(t, store, 1, 1)
		msgs, _ := store.Messages(context.Background(), 1)
		if turn.Status != library.AskTurnDone || len(msgs) != 8 {
			t.Errorf("after the disconnect: %s, %d messages", turn.Status, len(msgs))
		}
	})
}

// A failed turn sends error last, keeps what it showed (no active step),
// stores no messages, and never replays.
func TestAsk_Errors(t *testing.T) {
	for _, tt := range []struct {
		name string
		last fake.Response
		code string
	}{
		{"refused", fake.Response{StopReason: "refusal", RefusalCategory: "cyber"}, recommend.AskErrorRefused},
		{"cut off", fake.Response{Blocks: []fake.Block{fake.Text("Well")}, StopReason: "max_tokens"}, recommend.AskErrorLimitReached},
		{"overloaded", fake.Response{Status: 529}, recommend.AskErrorUpstream},
		{"empty", fake.Response{StopReason: "end_turn"}, recommend.AskErrorUpstream},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore()
			h := newHarness(t, store,
				fake.Response{Blocks: []fake.Block{toolUse("toolu_1", "get_taste_profile", map[string]any{})}},
				tt.last,
				fake.Response{Blocks: []fake.Block{fake.Text("Here.")}},
			)
			events := h.open(`{"message":"anime"}`).all()
			last := events[len(events)-1]
			if len(events) != 3 || events[1].Data != `{"step":"history","state":"active","detail":null}` || last.Name != "error" ||
				field[string](t, last, "code") != tt.code || field[string](t, last, "message") == "" {
				t.Fatalf("events:\n%s", dump(events))
			}
			turn := turnByID(t, store, 1, 1)
			var v struct {
				Steps []recommend.Step     `json:"steps"`
				Error *recommend.TurnError `json:"error"`
			}
			if err := json.Unmarshal(turn.View, &v); err != nil || turn.Status != library.AskTurnError || turn.ErrorCode != tt.code ||
				len(v.Steps) != 0 || v.Error == nil || v.Error.Code != tt.code {
				t.Errorf("turn %s %s view %s", turn.Status, turn.ErrorCode, turn.View)
			}
			h.open(`{"thread_id":1,"message":"try again"}`).all()
			reqs := h.api.Requests()
			if n := len(reqs[len(reqs)-1].Messages()); n != 1 {
				t.Errorf("the next turn replayed %d messages, want only its own", n)
			}
		})
	}
}

// Shutdown cancels the turn in flight: its stream closes, the turn ends
// cancelled, and new asks are refused.
func TestAsk_Close(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store, fake.Response{Blocks: []fake.Block{fake.Text("Thinking about")}, Hang: true})
	s := h.open(`{"message":"anime"}`)
	s.until(t, named("start"))
	closed := make(chan struct{})
	go func() {
		h.svc.Close()
		close(closed)
	}()
	if rest := s.all(); len(rest) != 0 {
		t.Errorf("after Close: %s", dump(rest))
	}
	<-closed
	if turn := turnByID(t, store, 1, 1); turn.Status != library.AskTurnCancelled || len(turn.View) == 0 {
		t.Errorf("turn = %s %s", turn.Status, turn.View)
	}
	if _, resp := h.post(`{"message":"anime"}`); resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("an ask after Close = %v", resp)
	}
}

// An answer without show_picks is text, shown when its response ends; a
// note before a tool call never is.
func TestAsk_TextAnswer(t *testing.T) {
	h := newHarness(t, newMemStore(),
		fake.Response{Blocks: []fake.Block{fake.Text("Let me check."), toolUse("toolu_1", "check_seen",
			map[string]any{"titles": []map[string]any{{"media_type": "tv", "tmdb_id": 1001}}})}},
		fake.Response{Blocks: []fake.Block{fake.Thinking("sig"), fake.Text("You haven't seen Mythic Quest yet. Start there.")}},
	)
	events := h.open(`{"message":"have I seen Mythic Quest?"}`).all()
	var got []string
	for _, e := range events {
		got = append(got, e.Name+" "+e.Data)
	}
	want := []string{
		`status {"step":"history","state":"active","detail":null}`,
		`status {"step":"history","state":"done","detail":"1 title checked"}`,
		`status {"step":"write","state":"active","detail":null}`,
		`text {"delta":"You haven't seen Mythic Quest yet. Start there."}`,
		`status {"step":"write","state":"done","detail":null}`,
	}
	if len(got) != 7 || strings.Join(got[1:6], "\n") != strings.Join(want, "\n") || events[6].Name != "done" ||
		!strings.Contains(events[6].Data, `"refinements":[]`) {
		t.Errorf("events:\n%s", strings.Join(got, "\n"))
	}
}

func dump(events []sseEvent) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.Name + " " + e.Data + "\n")
	}
	return b.String()
}
