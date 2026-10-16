package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// fakeAsk streams script, then closes; with hold it waits for the client to
// go away instead, and reports it on cancelled.
type fakeAsk struct {
	err       error
	script    []recommend.Event
	gap       time.Duration // between events
	hold      bool
	requests  []recommend.AskRequest
	cancelled chan struct{}
}

func (f *fakeAsk) Threads(context.Context) ([]recommend.ThreadSummary, error) { return nil, f.err }

// Thread 12 has one turn with nil lists, 13 is a zero value; the handler
// answers their lists as [].
func (f *fakeAsk) Thread(_ context.Context, id int64) (*recommend.Thread, error) {
	switch {
	case f.err != nil:
		return nil, f.err
	case id == 12:
		turn := recommend.Turn{ID: 46, Message: "anime", Status: recommend.TurnStatusCancelled}
		return &recommend.Thread{ThreadSummary: recommend.ThreadSummary{ID: 12, Title: "Anime", TurnCount: 1}, Model: recommend.ModelOpus, Turns: []recommend.Turn{turn}}, nil
	case id == 13:
		return &recommend.Thread{}, nil
	}
	return nil, nil
}

func (f *fakeAsk) Ask(ctx context.Context, req recommend.AskRequest) (<-chan recommend.Event, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	events := make(chan recommend.Event)
	go func() {
		defer close(events)
		for i, e := range f.script {
			if i > 0 {
				time.Sleep(f.gap)
			}
			select {
			case events <- e:
			case <-ctx.Done():
				f.cancelled <- struct{}{}
				return
			}
		}
		if f.hold {
			<-ctx.Done()
			f.cancelled <- struct{}{}
		}
	}()
	return events, nil
}

var _ Ask = (*fakeAsk)(nil)

func detail(s string) *string { return &s }

var askScript = []recommend.Event{
	{Name: recommend.EventStart, Data: recommend.StartEvent{ThreadID: 13, TurnID: 46, Title: "Something like Severance but lighter"}},
	{Name: recommend.EventStatus, Data: recommend.Step{Step: recommend.StepHistory, State: recommend.StepDone, Detail: detail("2,184 plays")}},
	{Name: recommend.EventText, Data: recommend.TextEvent{Delta: "Five picks"}},
	{Name: recommend.EventDone, Data: recommend.DoneEvent{ThreadID: 13, TurnID: 46, ElapsedMS: 9100, Refinements: []string{"Lighter"}, Usage: recommend.TurnUsage{CostUSD: 0.02}}},
}

// done's refinements are [] even when the service leaves them nil.
func TestAskRoutes_DoneWithoutRefinements(t *testing.T) {
	for _, data := range []any{recommend.DoneEvent{ThreadID: 1, TurnID: 2}, &recommend.DoneEvent{ThreadID: 1, TurnID: 2}} {
		f := &fakeAsk{script: []recommend.Event{{Name: recommend.EventDone, Data: data}}}
		rec := doBody(t, newTestServer(Options{Ask: f}), "POST", "/api/recommendations/ask", `{"message":"anime"}`)
		if !strings.Contains(rec.Body.String(), `"refinements":[]`) {
			t.Errorf("%T: stream = %s, want refinements []", data, rec.Body)
		}
	}
}

func TestAskRoutes_Stream(t *testing.T) {
	f := &fakeAsk{script: askScript}
	s := newTestServer(Options{Ask: f})
	rec := doBody(t, s, "POST", "/api/recommendations/ask", `{"thread_id":12,"message":"  something like Severance but lighter  "}`)
	if rec.Code != 200 {
		t.Fatalf("ask = %d %s", rec.Code, rec.Body)
	}
	for header, want := range map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-cache, no-transform", "X-Accel-Buffering": "no"} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	want := "event: start\ndata: {\"thread_id\":13,\"turn_id\":46,\"title\":\"Something like Severance but lighter\"}\n\n" +
		"event: status\ndata: {\"step\":\"history\",\"state\":\"done\",\"detail\":\"2,184 plays\"}\n\n" +
		"event: text\ndata: {\"delta\":\"Five picks\"}\n\n" +
		"event: done\ndata: {\"thread_id\":13,\"turn_id\":46,\"elapsed_ms\":9100,\"refinements\":[\"Lighter\"],\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"web_searches\":0,\"cost_usd\":0.02}}\n\n"
	if rec.Body.String() != want {
		t.Errorf("stream =\n%s\nwant\n%s", rec.Body, want)
	}
	if len(f.requests) != 1 || *f.requests[0].ThreadID != 12 || f.requests[0].Message != "something like Severance but lighter" {
		t.Errorf("request = %+v", f.requests)
	}
}

func TestAskRoutes_Ping(t *testing.T) {
	defer func(old time.Duration) { askPingInterval = old }(askPingInterval)
	askPingInterval = 10 * time.Millisecond
	f := &fakeAsk{script: askScript[:2], gap: 60 * time.Millisecond}
	rec := doBody(t, newTestServer(Options{Ask: f}), "POST", "/api/recommendations/ask", `{"message":"anime"}`)
	body := rec.Body.String()
	start, status := strings.Index(body, "event: start\n"), strings.Index(body, "event: status\n")
	if start < 0 || status < start || !strings.Contains(body[start:status], ": ping\n\n") {
		t.Errorf("stream = %q, want pings between the events", body)
	}
}

// A disconnect cancels generation: the service sees its context end.
func TestAskRoutes_Disconnect(t *testing.T) {
	f := &fakeAsk{script: askScript[:1], hold: true, cancelled: make(chan struct{}, 1)}
	server := httptest.NewServer(newTestServer(Options{Ask: f}).Handler())
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/recommendations/ask", strings.NewReader(`{"message":"anime"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "event: start\n" {
		t.Fatalf("first line = %q, %v", line, err)
	}
	cancel() // Stop
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the turn wasn't cancelled when the client went away")
	}
}

func TestAskRoutes_BadRequests(t *testing.T) {
	f := &fakeAsk{}
	s := newTestServer(Options{Ask: f})
	for _, body := range []string{
		`not json`,
		`{}`,
		`{"message":"   "}`,
		`{"message":"` + strings.Repeat("é", 501) + `"}`,
		`{"message":"hi","thread_id":0}`,
		`{"message":"hi","thread_id":"12"}`,
	} {
		if rec := doBody(t, s, "POST", "/api/recommendations/ask", body); rec.Code != 400 {
			t.Errorf("%s = %d, want 400", body, rec.Code)
		}
	}
	if rec := doBody(t, s, "POST", "/api/recommendations/ask", `{"message":"`+strings.Repeat("é", 500)+`","thread_id":null}`); rec.Code != 200 {
		t.Errorf("500 characters = %d, want 200", rec.Code)
	}
	if len(f.requests) != 1 {
		t.Errorf("requests = %d, want only the valid one", len(f.requests))
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/threads/abc", ""); rec.Code != 400 {
		t.Errorf("thread abc = %d, want 400", rec.Code)
	}
}

// Errors before the stream are JSON, with their codes; the 429s carry resets_at.
func TestAskRoutes_Errors(t *testing.T) {
	midnight := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{recommend.ErrUnavailable, 503, CodeUnavailable},
		{recommend.ErrNotFound, 404, CodeNotFound},
		{recommend.ErrThreadFull, 409, CodeThreadFull},
		{recommend.ErrAskBusy, 409, CodeAskBusy},
		{&recommend.LimitError{Code: recommend.CodeAskLimit, ResetsAt: midnight}, 429, recommend.CodeAskLimit},
		{&recommend.LimitError{Code: recommend.CodeBudgetExceeded, ResetsAt: midnight}, 429, recommend.CodeBudgetExceeded},
		{errors.New("db down"), 500, CodeInternalError},
	} {
		rec := doBody(t, newTestServer(Options{Ask: &fakeAsk{err: tt.err}}), "POST", "/api/recommendations/ask", `{"message":"anime"}`)
		e := errorOf(t, rec.Body.Bytes())
		if rec.Code != tt.status || e.Code != tt.code || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("%v = %d %s, want %d %s", tt.err, rec.Code, rec.Body, tt.status, tt.code)
		}
		if tt.status == 429 && (e.ResetsAt == nil || !e.ResetsAt.Equal(midnight)) {
			t.Errorf("%v: resets_at = %v", tt.err, e.ResetsAt)
		}
	}
}

func TestAskRoutes_Threads(t *testing.T) {
	s := newTestServer(Options{Ask: &fakeAsk{}})
	if rec := doBody(t, s, "GET", "/api/recommendations/threads", ""); rec.Code != 200 || rec.Body.String() != `{"items":[]}` {
		t.Errorf("threads = %d %s", rec.Code, rec.Body)
	}
	rec := doBody(t, s, "GET", "/api/recommendations/threads/12", "")
	var thread struct {
		TurnCount int `json:"turn_count"`
		Model     string
		Turns     []map[string]json.RawMessage
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &thread) != nil || thread.TurnCount != 1 || thread.Model != recommend.ModelOpus || len(thread.Turns) != 1 {
		t.Fatalf("thread = %d %s", rec.Code, rec.Body)
	}
	for _, list := range []string{"steps", "items", "refinements"} {
		if got := string(thread.Turns[0][list]); got != "[]" {
			t.Errorf("turn %s = %s, want []", list, got)
		}
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/threads/13", ""); !strings.Contains(rec.Body.String(), `"turns":[]`) {
		t.Errorf("zero thread = %s, want turns []", rec.Body)
	}
	if rec := doBody(t, s, "GET", "/api/recommendations/threads/99", ""); rec.Code != 404 {
		t.Errorf("unknown thread = %d, want 404", rec.Code)
	}
}
