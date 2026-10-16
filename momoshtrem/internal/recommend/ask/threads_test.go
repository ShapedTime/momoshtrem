package ask_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
)

func (h *harness) get(path string) (int, []byte) {
	h.t.Helper()
	resp, err := http.Get(h.server.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// Threads read their turns' saved views with library, watchlist and
// feedback joined now: a title turned down since is left out, a like
// shows. A streaming turn has its message only.
func TestAsk_ThreadReads(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store, append(fullTurn(), fake.Response{Blocks: []fake.Block{fake.Text("…")}, Hang: true})...)
		h.open(`{"message":"something like Severance but lighter"}`).all()

		h.titles.set(library.TitleState{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 2002}, Feedback: "dislike"})
		h.titles.set(library.TitleState{TitleRef: library.TitleRef{MediaType: "tv", TMDBID: 1001}, Feedback: "like", OnWatchlist: true})
		s := h.open(`{"thread_id":1,"message":"anime instead"}`)
		s.until(t, named("start"))

		status, body := h.get("/api/recommendations/threads/1")
		if status != 200 {
			t.Fatalf("thread = %d %s", status, body)
		}
		var thread recommend.Thread
		if err := json.Unmarshal(body, &thread); err != nil {
			t.Fatal(err)
		}
		if thread.Title != "Something like Severance but lighter" || thread.TurnCount != 2 || thread.Model != recommend.ModelOpus || len(thread.Turns) != 2 {
			t.Fatalf("thread = %s", body)
		}
		done, streaming := thread.Turns[0], thread.Turns[1]
		if done.Status != "done" || len(done.Steps) != 4 || done.Intro == "" || done.ElapsedMS == nil || done.Error != nil ||
			strings.Join(done.Refinements, "|") != "Lighter|Only movies|Older" {
			t.Errorf("done turn = %+v", done)
		}
		if len(done.Items) != 1 || done.Items[0].TMDBID != 1001 || done.Items[0].Feedback == nil || *done.Items[0].Feedback != "like" ||
			!done.Items[0].OnWatchlist || done.Items[0].Library == nil || done.Items[0].Title != "Mythic Quest" ||
			done.Items[0].MatchTag != "Workplace satire · lighter" {
			t.Errorf("items = %+v, want Mythic Quest only, liked and on the watchlist", done.Items)
		}
		if !strings.Contains(string(body), `"steps":[],"intro":"","items":[],"refinements":[],"error":null,"elapsed_ms":null`) ||
			streaming.Status != "streaming" || streaming.Message != "anime instead" {
			t.Errorf("streaming turn = %+v in %s", streaming, body)
		}

		status, body = h.get("/api/recommendations/threads")
		var list struct {
			Items []recommend.ThreadSummary `json:"items"`
		}
		if json.Unmarshal(body, &list) != nil || status != 200 || len(list.Items) != 1 || list.Items[0].TurnCount != 2 {
			t.Errorf("threads = %d %s", status, body)
		}
		if status, _ := h.get("/api/recommendations/threads/99"); status != 404 {
			t.Errorf("unknown thread = %d", status)
		}
	})
}

// show_picks checks each pick: it drops what can't be shown and says why
// in its answer; the rest are cards at once, at most 10 a turn.
func TestAsk_ShowPicksChecks(t *testing.T) {
	var items []map[string]any
	items = append(items,
		pick("movie", 2003, "Seen", "Watched already."),
		pick("tv", 1001, "Workplace", strings.Repeat("A long reason. ", 20)),
		pick("tv", 1001, "Again", "Twice."),
		pick("tv", 9999, "Unknown", "Not on TMDB."),
		pick("movie", 2099, "Soon", "Out in 2099."),
	)
	for i := 1; i <= 10; i++ {
		items = append(items, pick("tv", 3000+i, "Filler", "One of many."))
	}
	h := newHarness(t, newMemStore(),
		fake.Response{Blocks: []fake.Block{toolUse("toolu_1", "show_picks", picksInput("  Here goes.  ",
			[]string{" Lighter ", "lighter", "", "Only anime", "Shorter", "Older", "Newer", "Sixth"}, items...))}},
		fake.Response{Blocks: []fake.Block{fake.Text("Enjoy.")}},
	)
	h.titles.set(library.TitleState{TitleRef: library.TitleRef{MediaType: "movie", TMDBID: 2003}, Plays: 1, Started: true})
	events := h.open(`{"message":"anything"}`).all()

	var shown []string
	for _, e := range events {
		if e.Name == "item" {
			shown = append(shown, field[string](t, e, "title"))
		}
	}
	// 2003 is watched, 1001 comes twice and 9999 is unknown to TMDB: the
	// valid picks fill the turn's 10, and the rest are past the 10th.
	if len(shown) != 10 || shown[0] != "Mythic Quest" || shown[1] != "Coming Up" || shown[9] != "Filler 3008" {
		t.Errorf("shown = %v", shown)
	}
	for _, e := range events {
		if e.Name == "item" && field[float64](t, e, "tmdb_id") == 1001 {
			if reason := field[string](t, e, "reason"); len([]rune(reason)) != recommend.ReasonMaxChars || !strings.HasSuffix(reason, "…") {
				t.Errorf("reason = %q, want it cut to 140", reason)
			}
		}
		if e.Name == "item" && field[float64](t, e, "tmdb_id") == 2099 {
			if badge := field[*string](t, e, "badge"); badge == nil || *badge != "2099" {
				t.Errorf("badge = %v, want 2099", badge)
			}
		}
		if e.Name == "text" && field[string](t, e, "delta") != "Here goes." {
			t.Errorf("text = %s", e.Data)
		}
	}
	last := events[len(events)-1]
	if got := field[[]string](t, last, "refinements"); strings.Join(got, "|") != "Lighter|Only anime|Shorter|Older|Newer" {
		t.Errorf("refinements = %v", got)
	}

	// The tool's answer, as the model read it.
	msgs := h.api.Requests()[1].Messages()
	var result struct {
		Content []struct {
			Content string `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(msgs[len(msgs)-1], &result); err != nil {
		t.Fatal(err)
	}
	var answer struct {
		Shown   []map[string]any `json:"shown"`
		Dropped []struct {
			TMDBID int    `json:"tmdb_id"`
			Why    string `json:"why"`
		} `json:"dropped"`
		Total int    `json:"total_shown"`
		Room  int    `json:"room"`
		Next  string `json:"next"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Content), &answer); err != nil {
		t.Fatalf("show_picks answered %s: %v", result.Content[0].Content, err)
	}
	why := map[int]string{}
	for _, d := range answer.Dropped {
		if why[d.TMDBID] == "" {
			why[d.TMDBID] = d.Why
		}
	}
	if why[2003] != "watched" || why[1001] != "already shown" || why[9999] != "TMDB has no such title" ||
		why[3009] != "past the 10th pick" || why[3010] != "past the 10th pick" || answer.Total != 10 || answer.Room != 0 ||
		!strings.Contains(answer.Next, "has its 10") {
		t.Errorf("show_picks answered %s", result.Content[0].Content)
	}
}

// show_picks fits the request's strict schema budget: every object
// closed, every property required (no optional parameters), no unions
// across all of Ask's custom tools, and fewer than 20 strict tools.
func TestAsk_SchemaBudget(t *testing.T) {
	h := newHarness(t, newMemStore(), fake.Response{Blocks: []fake.Block{fake.Text("Hi.")}})
	h.open(`{"message":"hi"}`).all()
	var body struct {
		Tools []struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			Strict      bool   `json:"strict"`
			InputSchema any    `json:"input_schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(h.api.Requests()[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	unions, optional, strict := 0, 0, 0
	for _, tl := range body.Tools {
		if !tl.Strict {
			continue
		}
		strict++
		u, o := schemaCounts(t, tl.Name, tl.InputSchema)
		unions += u
		optional += o
	}
	if strict != 6 || unions > 2 || optional != 0 {
		t.Errorf("strict tools %d, union parameters %d (at most 16, R1 keeps to 2), optional %d", strict, unions, optional)
	}
}

// schemaCounts counts a schema's union parameters (anyOf or a type array)
// and optional properties, and checks every object is closed.
func schemaCounts(t *testing.T, tool string, node any) (unions, optional int) {
	switch v := node.(type) {
	case map[string]any:
		if _, ok := v["anyOf"]; ok {
			unions++
		}
		if _, ok := v["type"].([]any); ok {
			unions++
		}
		if v["type"] == "object" {
			if v["additionalProperties"] != false {
				t.Errorf("%s: an object isn't closed", tool)
			}
			props, _ := v["properties"].(map[string]any)
			required := map[string]bool{}
			list, _ := v["required"].([]any)
			for _, r := range list {
				required[r.(string)] = true
			}
			for name := range props {
				if !required[name] {
					optional++
				}
			}
		}
		for _, child := range v {
			u, o := schemaCounts(t, tool, child)
			unions, optional = unions+u, optional+o
		}
	case []any:
		for _, child := range v {
			u, o := schemaCounts(t, tool, child)
			unions, optional = unions+u, optional+o
		}
	}
	return unions, optional
}

// The usage page's Ask part: turns started today, and the mean done turn.
func TestAsk_UsageReporter(t *testing.T) {
	forEachStore(t, func(t *testing.T, store ask.Store) {
		h := newHarness(t, store)
		if est, err := h.svc.AskEstimate(context.Background()); err != nil || est != nil {
			t.Errorf("estimate before any = %v, %v", est, err)
		}
		seed(t, store, 0, 3)
		dayStart, _ := aicore.DayBounds(time.Now())
		n, err := h.svc.AsksToday(context.Background(), dayStart)
		est, err2 := h.svc.AskEstimate(context.Background())
		if err != nil || err2 != nil || n != 3 || est == nil || *est < 0.0199 || *est > 0.0201 {
			t.Errorf("asks today %d, estimate %v (%v, %v)", n, est, err, err2)
		}
		tomorrow := dayStart.Add(36 * time.Hour)
		if n, _ := h.svc.AsksToday(context.Background(), tomorrow); n != 0 {
			t.Errorf("asks since tomorrow = %d", n)
		}
	})
}

func TestThreadTitle(t *testing.T) {
	long := "something like Severance but lighter and with more cats in the office please"
	for _, tt := range []struct{ in, want string }{
		{"  anime with a Frieren vibe ", "Anime with a Frieren vibe"},
		{"ßtraße", "ßtraße"}, // ToUpper keeps one rune
		{"élan", "Élan"},
		{long, "Something like Severance but lighter and with more cats in…"},
		{strings.Repeat("a", 70), "A" + strings.Repeat("a", 59) + "…"},
		{"a " + strings.Repeat("b", 70), "A " + strings.Repeat("b", 58) + "…"},                         // the space is at 1: a hard cut
		{strings.Repeat("x", 31) + " " + strings.Repeat("y", 40), "X" + strings.Repeat("x", 30) + "…"}, // at 31: cut there
		{strings.Repeat("x", 30) + " " + strings.Repeat("y", 40), "X" + strings.Repeat("x", 29) + " " + strings.Repeat("y", 29) + "…"},
		{strings.Repeat("🐈", 61), strings.Repeat("🐈", 60) + "…"},
		{strings.Repeat("w", 59) + "\tzz", "W" + strings.Repeat("w", 58) + "…"}, // a hard cut's trailing whitespace is trimmed
	} {
		if got := ask.ThreadTitle(tt.in); got != tt.want {
			t.Errorf("ThreadTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
