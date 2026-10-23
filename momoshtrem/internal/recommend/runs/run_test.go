package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
)

// A full run on the fake API: a web search and get_candidates, then five
// shelves, one of them with titles the server drops (watched, the wrong
// name for its id, an id TMDB doesn't know), and the hero picks. The
// request prefix is byte-stable, the steps come in the order reached with
// their tallies, the usage is summed after every call, and the shelves are
// published only at the end.
func TestExecute_FullRun(t *testing.T) {
	first := shelfOf("because-severance", "because", 1002, 8)
	items := first["items"].([]map[string]any)
	items[0]["title"] = "Wrong Title"                                    // 1002: another title's name
	items = append([]map[string]any{item(1001, "Watched it")}, items...) // 1001: watched
	items = append(items, item(9999, "Not on TMDB"))
	first["items"] = items

	h := newHarness(t,
		fake.Response{Blocks: []fake.Block{
			fake.Thinking("sig1"),
			fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "new series October 2026"}),
			fake.WebSearchResult("srvtoolu_1", "https://example.com/new"),
			fake.ToolUse("toolu_1", "get_candidates", map[string]any{"media_type": "any", "genres": []string{}, "min_runtime": 0,
				"max_runtime": 0, "year_from": 0, "year_to": 0, "language": "", "anime": "either", "limit": 10}),
		}, Usage: fake.Usage{InputTokens: 9000, OutputTokens: 800, WebSearchRequests: 1}},
		fake.Response{Blocks: []fake.Block{
			fake.ToolUse("toolu_2", NameSubmitShelf, first),
			fake.ToolUse("toolu_3", NameSubmitShelf, shelfOf("new-this-month", "new", 1020, 6)),
		}, Usage: fake.Usage{CacheReadInputTokens: 9000, InputTokens: 600, OutputTokens: 900}},
		fake.Response{Blocks: []fake.Block{
			fake.ToolUse("toolu_4", NameSubmitShelf, shelfOf("gems", "gems", 1030, 6)),
			fake.ToolUse("toolu_5", NameSubmitShelf, shelfOf("more", "other", 1040, 6)),
			fake.ToolUse("toolu_6", NameSubmitShelf, shelfOf("coming", "coming_soon", 1050, 7)),
			fake.ToolUse("toolu_7", NameSubmitHeroPicks, heroOf(1003, 1020, 1030, 1040)),
		}, Usage: fake.Usage{CacheReadInputTokens: 10000, InputTokens: 700, OutputTokens: 1200}},
		fake.Response{Blocks: []fake.Block{fake.Text("Five shelves and four picks saved.")},
			Usage: fake.Usage{CacheReadInputTokens: 11000, InputTokens: 100, OutputTokens: 20}},
	)
	out := h.execute(t)

	if out.Status != recommend.StatusSucceeded || out.Shelves != 5 || out.HeroPicks != 4 || out.Usage.Calls != 4 {
		t.Fatalf("outcome = %+v", out)
	}
	if !h.store.published || h.store.failed != "" || h.store.profileID != 7 || h.profiles.saved != 1 {
		t.Fatalf("store = %+v", h.store)
	}
	if h.store.syncs != 5 {
		t.Errorf("usage synced %d times, want after each of the 4 calls and once at the end", h.store.syncs)
	}
	// The run's member's history; the prune keeps the budget month's runs.
	if !reflect.DeepEqual(h.members, []int64{2}) {
		t.Errorf("ran on members %v, want the run's, 2", h.members)
	}
	if h.store.pruned != 1 || !h.store.prunedSince.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("pruned %d times since %v, want once since 1 October", h.store.pruned, h.store.prunedSince)
	}

	// The first shelf without the three dropped titles; meta snapshotted;
	// signals from the pool.
	s := h.store.shelves[0]
	var ids []int
	for _, it := range s.Items {
		ids = append(ids, it.TMDBID)
	}
	if want := []int{1003, 1004, 1005, 1006, 1007, 1008, 1009}; s.Slug != "because-severance" || !reflect.DeepEqual(ids, want) {
		t.Errorf("first shelf = %s %v, want %v", s.Slug, ids, want)
	}
	var meta recommend.ItemMeta
	if err := json.Unmarshal(s.Items[0].Meta, &meta); err != nil || meta.Title != "Movie 1003" || *meta.Year != 2024 ||
		*meta.RuntimeMinutes != 101 || *meta.VoteAverage != 7.4 || meta.Badge != nil || meta.Seasons != nil {
		t.Errorf("meta = %+v, %v", meta, err)
	}
	if string(s.Items[0].Signals) != `{"pool":true,"rank":3,"why":["rec:Severance (2022)"]}` {
		t.Errorf("signals = %s", s.Items[0].Signals)
	}
	if len(h.store.picks) != 4 || h.store.picks[0].Kicker != "Tonight's pick" {
		t.Errorf("picks = %+v", h.store.picks)
	}

	// The model read why each title was dropped.
	reqs := h.srv.Requests()
	if len(reqs) != 4 {
		t.Fatalf("requests = %d", len(reqs))
	}
	results := toolResults(t, reqs[2])
	for _, want := range []string{`"why":"can't be picked: watched"`, `on TMDB, not “Wrong Title (2024)”`, `TMDB has no movie 9999`,
		`"saved":"because-severance"`, `"titles":7`, `"shelves_done":1`} {
		if !strings.Contains(results["toolu_2"], want) {
			t.Errorf("submit_shelf's result lacks %s:\n%s", want, results["toolu_2"])
		}
	}

	// The prefix is byte-stable: the same tools and system prompt on every
	// call, and each call's messages start with the last call's.
	for i, r := range reqs {
		if !bytes.Equal(r.Field("tools"), reqs[0].Field("tools")) || !bytes.Equal(r.Field("system"), reqs[0].Field("system")) {
			t.Errorf("request %d: the tools or the system prompt changed", i)
		}
		if i > 0 {
			prev, cur := reqs[i-1].Messages(), r.Messages()
			for j := range prev {
				if !bytes.Equal(prev[j], cur[j]) {
					t.Errorf("request %d: message %d changed", i, j)
				}
			}
		}
	}
	var system []map[string]any
	_ = json.Unmarshal(reqs[0].Field("system"), &system)
	if len(system) != 1 || system[0]["text"] != systemPrompt {
		t.Errorf("system = %s", reqs[0].Field("system"))
	}
	var tools []map[string]any
	_ = json.Unmarshal(reqs[0].Field("tools"), &tools)
	var names []string
	for _, tl := range tools {
		names = append(names, tl["name"].(string))
		if tl["type"] == nil && tl["strict"] != true {
			t.Errorf("%s isn't strict", tl["name"])
		}
	}
	want := []string{"get_taste_profile", "get_candidates", "tmdb_search", "tmdb_details", "check_seen", NameSubmitShelf,
		NameSubmitHeroPicks, "web_search", "web_fetch"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
	if tools[7]["max_uses"] != float64(WebSearchMaxUses) || tools[8]["max_uses"] != float64(WebFetchMaxUses) {
		t.Errorf("server tools = %v %v", tools[7], tools[8])
	}
	firstTurn := userText(t, reqs[0].Messages()[0])
	for _, want := range []string{"Today is Friday, 16 October 2026", "<taste_profile>", `"title":"Severance"`,
		`<candidates total="60" shown="60">`, `"title":"Movie 1060"`} {
		if !strings.Contains(firstTurn, want) {
			t.Errorf("the first turn lacks %s", want)
		}
	}
	if !strings.Contains(string(reqs[0].Field("output_config")), `"effort":"medium"`) {
		t.Errorf("output_config = %s", reqs[0].Field("output_config"))
	}

	// The steps, in the order reached, all done, with their tallies.
	final := h.store.final
	got := []string{}
	for _, st := range final.Steps {
		got = append(got, st.Step+":"+st.State+":"+deref(st.Detail))
	}
	wantSteps := []string{"history:done:2,184 plays · 312 ratings", "tmdb:done:60 candidates · 23 already seen",
		"web:done:1 search", "write:done:5 of 5 shelves"}
	if !reflect.DeepEqual(got, wantSteps) || final.ShelvesDone != 5 || final.ShelvesTarget != 5 {
		t.Errorf("final progress = %v (%d of %d), want %v", got, final.ShelvesDone, final.ShelvesTarget, wantSteps)
	}
	// While it ran: one step active at a time, and the shelves counted up.
	var counts []int
	for _, p := range h.store.progress {
		active := 0
		for _, st := range p.Steps {
			if st.State == recommend.StepActive {
				active++
			}
		}
		if active != 1 {
			t.Errorf("progress %+v has %d active steps", p, active)
		}
		if len(counts) == 0 || counts[len(counts)-1] != p.ShelvesDone {
			counts = append(counts, p.ShelvesDone)
		}
	}
	if !reflect.DeepEqual(counts, []int{0, 1, 2, 3, 4, 5}) {
		t.Errorf("shelves_done went %v", counts)
	}
}

// A run that stops short gets one nudge turn, on the same prefix; still
// short, it fails and publishes nothing.
func TestExecute_NudgeThenFail(t *testing.T) {
	h := newHarness(t,
		fake.Response{Blocks: []fake.Block{
			fake.ToolUse("toolu_1", NameSubmitShelf, shelfOf("gems", "gems", 1002, 6)),
		}},
		fake.Response{Blocks: []fake.Block{fake.Text("Here are some thoughts on your taste.")}},
		fake.Response{Blocks: []fake.Block{fake.Text("I'd rather not.")}},
	)
	out := h.execute(t)
	if out.Status != recommend.StatusFailed || out.Shelves != 1 || h.store.published {
		t.Fatalf("outcome = %+v, published %v", out, h.store.published)
	}
	if h.store.failed != "Noir saved 1 of the 4 shelves a run needs." {
		t.Errorf("error = %q", h.store.failed)
	}
	reqs := h.srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3 (the run and one nudge)", len(reqs))
	}
	nudged := reqs[2].Messages()
	prev := reqs[1].Messages()
	if len(nudged) != len(prev)+2 {
		t.Fatalf("the nudge sent %d messages after %d", len(nudged), len(prev))
	}
	for i := range prev {
		if !bytes.Equal(prev[i], nudged[i]) {
			t.Errorf("message %d changed for the nudge", i)
		}
	}
	text := userText(t, nudged[len(nudged)-1])
	if text != "You've saved 1 shelf. Still missing: 4 of the 5 shelves (at least 4) and Tonight's pick. Save them with submit_shelf and submit_hero_picks now, then end." {
		t.Errorf("nudge = %q", text)
	}
	for _, st := range h.store.final.Steps {
		if st.State != recommend.StepDone {
			t.Errorf("a failed run's step %s is %s", st.Step, st.State)
		}
	}
}

// The nudge is enough when it brings the shelves.
func TestExecute_NudgeThenSucceed(t *testing.T) {
	var calls []fake.Block
	for i, slug := range []string{"a", "b", "c", "d"} {
		calls = append(calls, fake.ToolUse("toolu_s"+slug, NameSubmitShelf, shelfOf(slug, "other", 1002+10*i, 6)))
	}
	h := newHarness(t,
		fake.Response{Blocks: []fake.Block{fake.Text("Thinking about it.")}},
		fake.Response{Blocks: append(calls, fake.ToolUse("toolu_h", NameSubmitHeroPicks, heroOf(1002, 1012, 1022)))},
		fake.Response{Blocks: []fake.Block{fake.Text("Done.")}},
	)
	if out := h.execute(t); out.Status != recommend.StatusSucceeded || out.Shelves != 4 || out.HeroPicks != 3 {
		t.Fatalf("outcome = %+v", out)
	}
}

// Before a run starts: a spent budget and too little history skip it,
// with no API call.
func TestExecute_Skips(t *testing.T) {
	t.Run("budget", func(t *testing.T) {
		h := newHarness(t)
		h.runner.d.Budget = fakeBudget{err: &aicore.BudgetError{SpentUSD: 15.2, BudgetUSD: 15}}
		out := h.execute(t)
		if out.Status != recommend.StatusSkipped || h.store.skipped != recommend.SkipBudgetExceeded || h.store.started {
			t.Errorf("outcome = %+v, store = %+v", out, h.store)
		}
	})
	t.Run("no history", func(t *testing.T) {
		h := newHarness(t)
		h.profiles.ready = profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 4}}
		out := h.execute(t)
		if out.Status != recommend.StatusSkipped || h.store.skipped != recommend.SkipNoHistory || h.store.started {
			t.Errorf("outcome = %+v, store = %+v", out, h.store)
		}
	})
	t.Run("trakt disconnected", func(t *testing.T) {
		h := newHarness(t)
		h.profiles.ready = profile.Readiness{Connected: false, Counts: profile.Counts{Loved: 40}}
		if out := h.execute(t); h.store.skipped != recommend.SkipNoHistory {
			t.Errorf("outcome = %+v", out)
		}
	})
	t.Run("budget unreadable", func(t *testing.T) {
		h := newHarness(t)
		h.runner.d.Budget = fakeBudget{err: errors.New("db down")}
		if out := h.execute(t); out.Status != recommend.StatusFailed || h.store.failed != "Couldn't check the AI budget." {
			t.Errorf("outcome = %+v, failed %q", out, h.store.failed)
		}
	})
	t.Run("picks off", func(t *testing.T) {
		h := newHarness(t)
		settings := library.DefaultAISettings()
		settings.RecEnabled = false
		h.runner.d.Settings = fakeSettings{settings}
		if out := h.execute(t); out.Status != recommend.StatusFailed || h.store.started {
			t.Errorf("outcome = %+v", out)
		}
	})
}

// The caps stop a run: short of shelves it fails, saying which cap;
// with enough shelves saved it publishes them.
func TestExecute_Caps(t *testing.T) {
	saved := Caps
	t.Cleanup(func() { Caps = saved })

	t.Run("short of shelves", func(t *testing.T) {
		Caps = aicore.Caps{Calls: 1, Tokens: 1e9, CostUSD: 100, ToolCalls: 100}
		h := newHarness(t, fake.Response{Blocks: []fake.Block{
			fake.ToolUse("toolu_1", NameSubmitShelf, shelfOf("gems", "gems", 1002, 6)),
		}})
		out := h.execute(t)
		if out.Status != recommend.StatusFailed || h.store.failed != "The run reached its limit on API calls with 1 of 4 shelves saved." {
			t.Errorf("outcome = %+v, failed %q", out, h.store.failed)
		}
		if len(h.srv.Requests()) != 1 {
			t.Errorf("requests = %d after the cap", len(h.srv.Requests()))
		}
	})
	t.Run("cost, with enough shelves", func(t *testing.T) {
		Caps = aicore.Caps{Calls: 10, Tokens: 1e9, CostUSD: 0.01, ToolCalls: 100}
		var calls []fake.Block
		for i, slug := range []string{"a", "b", "c", "d"} {
			calls = append(calls, fake.ToolUse("toolu_"+slug, NameSubmitShelf, shelfOf(slug, "other", 1002+10*i, 6)))
		}
		h := newHarness(t, fake.Response{Blocks: calls, Usage: fake.Usage{InputTokens: 10000, OutputTokens: 1000}})
		out := h.execute(t)
		if out.Status != recommend.StatusSucceeded || !h.store.published || len(h.store.shelves) != 4 || len(h.store.picks) != 0 {
			t.Errorf("outcome = %+v, store = %+v", out, h.store)
		}
	})
	t.Run("no room for the nudge", func(t *testing.T) {
		Caps = aicore.Caps{Calls: 1, Tokens: 1e9, CostUSD: 100, ToolCalls: 100}
		h := newHarness(t, fake.Response{Blocks: []fake.Block{fake.Text("Hmm.")}})
		if out := h.execute(t); out.Status != recommend.StatusFailed || !strings.Contains(h.store.failed, "API calls") {
			t.Errorf("outcome = %+v, failed %q", out, h.store.failed)
		}
	})
}

// An API failure fails the run with a message for the user.
func TestExecute_APIError(t *testing.T) {
	h := newHarness(t, fake.Response{Status: 529})
	out := h.execute(t)
	if out.Status != recommend.StatusFailed || h.store.failed != "Anthropic is unavailable (529 overloaded_error); the next run tries again." {
		t.Errorf("outcome = %+v, failed %q", out, h.store.failed)
	}
}

func fourShelves() []fake.Block {
	var calls []fake.Block
	for i, slug := range []string{"a", "b", "c", "d"} {
		calls = append(calls, fake.ToolUse("toolu_"+slug, NameSubmitShelf, shelfOf(slug, "other", 1002+10*i, 6)))
	}
	return calls
}

// A run that reaches its own deadline publishes what it saved, as at a
// cap, its final writes made past the deadline; short of shelves it fails
// saying so. Only shutdown (the parent context) fails it outright.
func TestExecute_Deadline(t *testing.T) {
	saved := Timeout
	t.Cleanup(func() { Timeout = saved })
	Timeout = 300 * time.Millisecond

	t.Run("enough shelves", func(t *testing.T) {
		h := newHarness(t,
			fake.Response{Blocks: fourShelves()},
			fake.Response{Blocks: []fake.Block{fake.Text("Now the hero")}, Hang: true},
		)
		out := h.execute(t)
		if out.Status != recommend.StatusSucceeded || !h.store.published || len(h.store.shelves) != 4 {
			t.Errorf("outcome = %+v, store = %+v", out, h.store)
		}
	})
	t.Run("short of shelves", func(t *testing.T) {
		h := newHarness(t, fake.Response{Blocks: []fake.Block{fake.Text("Thinking")}, Hang: true})
		out := h.execute(t)
		if out.Status != recommend.StatusFailed || !strings.HasPrefix(h.store.failed, "The run took longer than") {
			t.Errorf("outcome = %+v, failed %q", out, h.store.failed)
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		Timeout = time.Minute
		h := newHarness(t,
			fake.Response{Blocks: fourShelves()},
			fake.Response{Blocks: []fake.Block{fake.Text("Now the hero")}, Hang: true},
		)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		out, err := h.runner.Execute(ctx, library.RecRun{ID: 42, Kind: "manual", Status: "queued", Model: "claude-opus-5-5"})
		if err != nil || out.Status != recommend.StatusFailed || h.store.published || h.store.failed != "momoshtrem stopped during the run." {
			t.Errorf("outcome = %+v, %v, store = %+v", out, err, h.store)
		}
	})
}

// A final write that fails is tried again, so the run doesn't stay active
// (refreshing) until a restart.
func TestExecute_FinalWriteRetried(t *testing.T) {
	saved := writeRetries
	t.Cleanup(func() { writeRetries = saved })
	writeRetries = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	h := newHarness(t, fake.Response{Blocks: append(fourShelves(), fake.ToolUse("toolu_h", NameSubmitHeroPicks, heroOf(1002, 1012, 1022)))},
		fake.Response{Blocks: []fake.Block{fake.Text("Done.")}})
	h.store.failWrites = 2
	if out := h.execute(t); out.Status != recommend.StatusSucceeded || !h.store.published || h.store.writes != 3 {
		t.Errorf("outcome = %+v, writes %d", out, h.store.writes)
	}

	h = newHarness(t, fake.Response{Blocks: []fake.Block{fake.Text("Nothing.")}}, fake.Response{Blocks: []fake.Block{fake.Text("Still nothing.")}})
	h.store.failWrites = 10
	out, err := h.runner.Execute(context.Background(), library.RecRun{ID: 42, Kind: "manual", Status: "queued", Model: "claude-opus-5-5"})
	if err == nil || out.Status != recommend.StatusFailed || h.store.writes != len(writeRetries)+1 {
		t.Errorf("a write that keeps failing = %+v, %v after %d tries", out, err, h.store.writes)
	}
}

// Web searches are capped per run, counted from usage (max_uses bounds one
// call only); the web step's tally comes from usage too.
func TestExecute_WebSearchCap(t *testing.T) {
	saved := Caps
	t.Cleanup(func() { Caps = saved })
	Caps.WebSearches = 3
	h := newHarness(t,
		fake.Response{Blocks: append(append([]fake.Block{
			fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "new"}),
			fake.WebSearchResult("srvtoolu_1", "https://example.com"),
		}, fourShelves()...), fake.ToolUse("toolu_h", NameSubmitHeroPicks, heroOf(1002, 1012, 1022))),
			Usage: fake.Usage{InputTokens: 100, WebSearchRequests: 3, WebFetchRequests: 1}},
	)
	out := h.execute(t)
	if out.Status != recommend.StatusSucceeded || len(h.srv.Requests()) != 1 || out.Usage.WebSearches != 3 {
		t.Errorf("outcome = %+v after %d requests", out, len(h.srv.Requests()))
	}
	for _, st := range h.store.final.Steps {
		if st.Step == recommend.StepWeb && deref(st.Detail) != "3 searches · 1 page" {
			t.Errorf("web step = %q", deref(st.Detail))
		}
	}
	if _, reached := remaining(Caps, &aicore.Result{Usage: aicore.Usage{WebSearches: 3}}); reached != aicore.CapWebSearches {
		t.Errorf("remaining reached = %q", reached)
	}
	if left, reached := remaining(aicore.Caps{Calls: 5}, &aicore.Result{Usage: aicore.Usage{Calls: 2, WebSearches: 9}}); reached != "" ||
		left != (aicore.Caps{Calls: 3}) {
		t.Errorf("zero caps = %+v, %q; want them kept as no cap", left, reached)
	}
}

// The web search cap tripping before the shelves are written is stopping
// short: one nudge turn, told to write from what it has, on the same
// prefix (the tools and max_uses unchanged) with one search left. Shelves
// submitted in a call that searches again still count.
func TestExecute_WebCapNudge(t *testing.T) {
	saved := Caps
	t.Cleanup(func() { Caps = saved })
	Caps.WebSearches = 20
	searching := func(n int64) fake.Response {
		return fake.Response{Blocks: []fake.Block{
			fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "new"}),
			fake.WebSearchResult("srvtoolu_1", "https://example.com"),
			fake.ToolUse("toolu_c", "get_candidates", map[string]any{"media_type": "any", "genres": []string{}, "min_runtime": 0,
				"max_runtime": 0, "year_from": 0, "year_to": 0, "language": "", "anime": "either", "limit": 5}),
		}, Usage: fake.Usage{InputTokens: 100, WebSearchRequests: n}}
	}
	withHero := append(fourShelves(), fake.ToolUse("toolu_h", NameSubmitHeroPicks, heroOf(1002, 1012, 1022)))

	t.Run("the nudge writes", func(t *testing.T) {
		h := newHarness(t,
			searching(5), searching(5), searching(5), searching(5), // 20: the cap
			fake.Response{Blocks: withHero},
			fake.Response{Blocks: []fake.Block{fake.Text("Saved.")}},
		)
		out := h.execute(t)
		if out.Status != recommend.StatusSucceeded || !h.store.published || out.Shelves != 4 || out.HeroPicks != 3 {
			t.Fatalf("outcome = %+v, failed %q", out, h.store.failed)
		}
		reqs := h.srv.Requests()
		if len(reqs) != 6 {
			t.Fatalf("requests = %d", len(reqs))
		}
		nudged, before := reqs[4], reqs[3]
		if !bytes.Equal(nudged.Field("tools"), before.Field("tools")) || !bytes.Equal(nudged.Field("system"), before.Field("system")) {
			t.Error("the nudge changed the tools or the system prompt")
		}
		prev, cur := before.Messages(), nudged.Messages()
		for i := range prev {
			if !bytes.Equal(prev[i], cur[i]) {
				t.Errorf("message %d changed for the nudge", i)
			}
		}
		// The nudge rides on the tool results the loop stopped on.
		var last struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(cur[len(cur)-1], &last); err != nil {
			t.Fatal(err)
		}
		n := len(last.Content)
		if len(cur) != len(prev)+2 || last.Role != "user" || n < 2 || last.Content[0].Type != "tool_result" ||
			!strings.HasPrefix(last.Content[n-1].Text, "No more web searches: save the shelves and Tonight's pick from what you have.") {
			t.Errorf("nudge message = %s", cur[len(cur)-1])
		}
	})
	t.Run("a search in the nudge ends it", func(t *testing.T) {
		searchingShelves := fake.Response{Blocks: append([]fake.Block{
			fake.ServerToolUse("srvtoolu_2", "web_search", map[string]any{"query": "one more"}),
			fake.WebSearchResult("srvtoolu_2", "https://example.com/2"),
		}, fourShelves()...), Usage: fake.Usage{WebSearchRequests: 1}}
		h := newHarness(t, searching(20), searchingShelves)
		out := h.execute(t)
		if out.Status != recommend.StatusSucceeded || out.Shelves != 4 || len(h.srv.Requests()) != 2 || out.Usage.WebSearches != 21 {
			t.Errorf("outcome = %+v after %d requests, failed %q", out, len(h.srv.Requests()), h.store.failed)
		}
	})
	t.Run("still short", func(t *testing.T) {
		h := newHarness(t, searching(20), fake.Response{Blocks: []fake.Block{fake.Text("Nothing to add.")}})
		if out := h.execute(t); out.Status != recommend.StatusFailed || h.store.failed != "Noir saved 0 of the 4 shelves a run needs." {
			t.Errorf("outcome = %+v, failed %q", out, h.store.failed)
		}
	})
}

// The dry run builds the same first request for the member, storing
// nothing.
func TestPrepare(t *testing.T) {
	h := newHarness(t)
	prep, err := h.runner.Prepare(t.Context(), 3, "claude-sonnet-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if prep.Request.Model != "claude-sonnet-5-5" || len(prep.Request.Tools) != 7 || prep.Pool == nil || h.profiles.saved != 0 ||
		h.store.started || !strings.Contains(prep.FirstTurn, "<taste_profile>") || !reflect.DeepEqual(h.members, []int64{3}) {
		t.Errorf("prepared = %+v for members %v", prep, h.members)
	}
	body, err := aicore.RequestBody(prep.Request)
	if err != nil || !bytes.Contains(body, []byte(`"web_search_20260209"`)) || !bytes.Contains(body, []byte(NameSubmitHeroPicks)) {
		t.Errorf("body = %.200s…, %v", body, err)
	}
}

func toolResults(t *testing.T, r fake.Request) map[string]string {
	t.Helper()
	msgs := r.Messages()
	var last struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error"`
		} `json:"content"`
	}
	if err := json.Unmarshal(msgs[len(msgs)-1], &last); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, b := range last.Content {
		if b.Type == "tool_result" {
			out[b.ToolUseID] = b.Content
			if b.IsError {
				out[b.ToolUseID] = "ERROR " + b.Content
			}
		}
	}
	return out
}

func userText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var m struct {
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Role != "user" || len(m.Content) == 0 {
		t.Fatalf("not a user message: %s", raw)
	}
	return m.Content[0].Text
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
