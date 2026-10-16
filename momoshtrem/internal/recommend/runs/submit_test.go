package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

var update = flag.Bool("update", false, "rewrite the golden tool definitions")

func newTestBoard(titles *fakeTitles) (*board, *[]int) {
	var counts []int
	pool := &candidates.Pool{Items: []candidates.Candidate{{MediaType: "movie", TMDBID: 1003, Why: []string{"kw"}}}}
	b := newBoard(&validator{titles: titles, details: fakeTMDB{}, now: func() time.Time { return now }}, pool,
		func(n int) { counts = append(counts, n) })
	return b, &counts
}

func runTool(t *testing.T, tl agent.Tool, input any) (map[string]any, string) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tl.Run(context.Background(), raw)
	var te *agent.ToolError
	if errors.As(err, &te) {
		return nil, te.Message
	}
	if err != nil {
		t.Fatalf("%s: %v", tl.Name(), err)
	}
	var res map[string]any
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	return res, ""
}

// submit_shelf validates every title and saves the shelf only with 6–15
// that pass; the same slug replaces a shelf, a new one past six is
// refused, and a title already on another shelf is dropped.
func TestSubmitShelf(t *testing.T) {
	titles := &fakeTitles{watched: map[int]bool{1001: true},
		feedback: map[library.TitleRef]string{{MediaType: "movie", TMDBID: 1010}: recommend.SignalDislike}}
	b, counts := newTestBoard(titles)
	submitShelf := b.tools()[0]

	// Too few pass: not saved, and the model reads why.
	short := shelfOf("because-dark", "because", 1001, 6) // 1001 watched
	short["items"] = append(short["items"].([]map[string]any), item(1010, "disliked"))
	_, msg := runTool(t, submitShelf, short)
	if !strings.Contains(msg, `The shelf "because-dark" wasn't saved: only 5 of its titles can be picked`) ||
		!strings.Contains(msg, `"why":"can't be picked: watched"`) || !strings.Contains(msg, `"why":"can't be picked: disliked"`) {
		t.Errorf("message = %s", msg)
	}
	if b.done() != 0 || len(*counts) != 0 {
		t.Fatalf("saved %d shelves", b.done())
	}

	// Saved with what passed; a title twice and a missing reason dropped;
	// the reason clipped; the slug made from a messy one.
	ok := shelfOf("Because Dark!", "because", 1002, 7)
	items := ok["items"].([]map[string]any)
	items[1]["reason"] = strings.Repeat("A long, careful reason about Dark ", 10)
	items = append(items, item(1002, "twice"), item(1020, "  "))
	ok["items"] = items
	ok["subtitle"] = "  Time loops and family secrets  "
	res, msg := runTool(t, submitShelf, ok)
	if msg != "" || res["saved"] != "because-dark" || res["titles"] != 7.0 || res["shelves_done"] != 1.0 {
		t.Fatalf("result = %v %s", res, msg)
	}
	whys := []string{}
	for _, d := range res["dropped"].([]any) {
		whys = append(whys, d.(map[string]any)["why"].(string))
	}
	if !reflect.DeepEqual(whys, []string{"listed twice", "no reason"}) {
		t.Errorf("dropped = %v", whys)
	}
	s := b.shelves[0]
	if *s.subtitle != "Time loops and family secrets" || len([]rune(s.items[1].reason)) > recommend.ReasonMaxChars ||
		!strings.HasSuffix(s.items[1].reason, "…") {
		t.Errorf("shelf = %+v, reason %q", s, s.items[1].reason)
	}
	if string(s.items[1].signals) != `{"pool":true,"rank":1,"why":["kw"]}` || string(s.items[0].signals) != `{"pool":false}` {
		t.Errorf("signals = %s, %s", s.items[1].signals, s.items[0].signals)
	}

	// Another shelf can't take a title of this one.
	dup := shelfOf("gems", "gems", 1020, 6)
	dup["items"] = append(dup["items"].([]map[string]any), item(1003, "again"))
	res, _ = runTool(t, submitShelf, dup)
	if d := res["dropped"].([]any); len(d) != 1 || d[0].(map[string]any)["why"] != "already on the shelf because-dark" {
		t.Errorf("dropped = %v", res["dropped"])
	}

	// The same slug replaces the shelf in place, its own titles allowed.
	res, _ = runTool(t, submitShelf, shelfOf("gems", "gems", 1020, 7))
	if res["shelves_done"] != 2.0 || res["titles"] != 7.0 || len(res["dropped"].([]any)) != 0 {
		t.Errorf("replace = %v", res)
	}
	replaced := shelfOf("because-dark", "because", 1030, 6)
	if res, _ = runTool(t, submitShelf, replaced); res["shelves_done"] != 2.0 || b.shelves[0].items[0].ref.TMDBID != 1030 {
		t.Errorf("replace = %v, first shelf %v", res, b.shelves[0].items[0].ref)
	}

	// Up to six shelves; a seventh slug is refused.
	for i := range 4 {
		if _, msg := runTool(t, submitShelf, shelfOf(fmt.Sprintf("s%d", i), "other", 1100+10*i, 6)); msg != "" {
			t.Fatal(msg)
		}
	}
	if _, msg := runTool(t, submitShelf, shelfOf("s9", "other", 1200, 6)); !strings.Contains(msg, "already 6 shelves") {
		t.Errorf("a seventh = %q", msg)
	}
	if !reflect.DeepEqual(*counts, []int{1, 2, 2, 2, 3, 4, 5, 6}) {
		t.Errorf("shelves_done went %v", *counts)
	}

	// More than 15: the first 15 are kept.
	b2, _ := newTestBoard(&fakeTitles{})
	res, _ = runTool(t, b2.tools()[0], shelfOf("big", "other", 1300, 18))
	if res["titles"] != 15.0 || len(res["dropped"].([]any)) != 3 {
		t.Errorf("18 titles = %v", res)
	}
}

// submit_hero_picks takes 3–9; a call replaces the picks; shows and badges
// snapshot from TMDB.
func TestSubmitHeroPicks(t *testing.T) {
	b, _ := newTestBoard(&fakeTitles{watched: map[int]bool{1001: true}})
	submit := b.tools()[1]
	if _, msg := runTool(t, submit, heroOf(1001, 1002, 1003)); !strings.Contains(msg, "only 2 can be picked") {
		t.Errorf("message = %q", msg)
	}
	in := heroOf(1002, 1003, 1999)
	in["picks"].([]map[string]any)[2]["year"] = 2027
	in["picks"] = append(in["picks"].([]map[string]any), map[string]any{"media_type": "tv", "tmdb_id": 2001, "title": "Show 2001",
		"kicker": strings.Repeat("Kicker ", 10), "reason": "Back next month."})
	res, msg := runTool(t, submit, in)
	if msg != "" || res["saved"] != 4.0 {
		t.Fatalf("result = %v %s", res, msg)
	}
	if len([]rune(b.picks[3].kicker)) > kickerMaxChars || *b.picks[2].meta.Badge != "Jan 16" {
		t.Errorf("picks = %+v", b.picks)
	}
	show := b.picks[3].meta
	if show.Title != "Show 2001" || *show.Seasons != 2 || *show.RuntimeMinutes != 50 || show.YearEnd != nil || show.Badge != nil {
		t.Errorf("show meta = %+v", show)
	}
	if _, msg := runTool(t, submit, heroOf(1004, 1005, 1006)); msg != "" || b.picks[0].ref.TMDBID != 1004 || len(b.picks) != 3 {
		t.Errorf("replace = %q, %+v", msg, b.picks)
	}
	many := heroOf(1010, 1011, 1012, 1013, 1014, 1015, 1016, 1017, 1018, 1019, 1020)
	if res, _ := runTool(t, submit, many); res["saved"] != 9.0 || len(b.picks) != 9 {
		t.Errorf("eleven = %v", res)
	}
}

// A coming_soon shelf badges a show's next episode; other shelves badge
// only a premiere still to come.
func TestBadges(t *testing.T) {
	v := &validator{titles: &fakeTitles{}, details: fakeTMDB{}, now: func() time.Time { return now }}
	show := proposed{Ref: library.TitleRef{MediaType: "tv", TMDBID: 2001}, Title: "Show 2001"}
	for _, tt := range []struct {
		comingSoon bool
		want       string
	}{{true, "Nov 20"}, {false, ""}} {
		out, err := v.check(context.Background(), []proposed{show}, tt.comingSoon)
		if err != nil {
			t.Fatal(err)
		}
		if got := deref(out[0].Meta.Badge); got != tt.want {
			t.Errorf("coming soon %v: badge %q, want %q", tt.comingSoon, got, tt.want)
		}
	}
	for _, tt := range []struct{ day, want string }{
		{"2026-10-16", "Oct 16"}, {"2026-10-15", ""}, {"2027-10-16", "Oct 16"}, {"2027-10-17", "2027"}, {"", ""},
	} {
		if got := deref(badge(tt.day, "2026-10-16")); got != tt.want {
			t.Errorf("badge(%q) = %q, want %q", tt.day, got, tt.want)
		}
	}
}

// Each title comes with its year: more than a year off TMDB's is another
// title (a remake, "Dark" for "Dark Matter"); 0 is unsure and passes.
func TestSubmitYear(t *testing.T) {
	b, _ := newTestBoard(&fakeTitles{})
	in := shelfOf("years", "other", 1002, 6)
	items := in["items"].([]map[string]any)
	items[0]["year"] = 2019 // TMDB: 2024
	items[1]["year"] = 0
	items[2]["year"] = 2025
	items = append(items, item(1020, "x"), item(1021, "y"))
	in["items"] = items
	res, msg := runTool(t, b.tools()[0], in)
	if msg != "" || res["titles"] != 7.0 {
		t.Fatalf("result = %v %s", res, msg)
	}
	d := res["dropped"].([]any)
	if len(d) != 1 || d[0].(map[string]any)["tmdb_id"] != 1002.0 ||
		d[0].(map[string]any)["why"] != "that tmdb_id is “Movie 1002 (2024)” on TMDB, not “Movie 1002 (2019)”: look the title up with tmdb_search" {
		t.Errorf("dropped = %v", d)
	}
	for _, tt := range []struct {
		given, actual int
		want          bool
	}{{2024, 2024, true}, {2023, 2024, true}, {2025, 2024, true}, {2022, 2024, false}, {0, 2024, true}, {2024, 0, true}} {
		if got := sameYear(tt.given, tt.actual); got != tt.want {
			t.Errorf("sameYear(%d, %d) = %v", tt.given, tt.actual, got)
		}
	}
}

func TestSameTitle(t *testing.T) {
	for _, tt := range []struct {
		given string
		names []string
		want  bool
	}{
		{"Amélie", []string{"Amelie"}, true},
		{"The Office", []string{"The Office (US)"}, true},
		{"Frieren", []string{"Frieren: Beyond Journey's End", "葬送のフリーレン"}, true},
		{"葬送のフリーレン", []string{"Frieren: Beyond Journey's End", "葬送のフリーレン"}, true},
		{"Law & Order", []string{"Law and Order"}, true},
		{"Dark", []string{"Darkness"}, false},
		{"It", []string{"It Follows"}, false},
		{"Severance", []string{"Silo"}, false},
		{"", []string{"Silo"}, false},
	} {
		if got := sameTitle(tt.given, tt.names...); got != tt.want {
			t.Errorf("sameTitle(%q, %q) = %v", tt.given, tt.names, got)
		}
	}
}

func TestClipAndSlug(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip = %q", got)
	}
	if got := clip("one two three four five", 12); got != "one two…" {
		t.Errorf("clip = %q", got)
	}
	for in, want := range map[string]string{"Because You Loved: Dark!": "because-you-loved-dark", "  ": "",
		"The Bear": "bear", strings.Repeat("ab ", 30): "ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab"} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// allTools is a run's custom tools: R1's and the submit tools, in order.
func allTools() []agent.Tool {
	list := tools.All(tools.Deps{Inputs: tools.Fixed{}, TMDB: fakeTMDB{}, Titles: &fakeTitles{}})
	return append(list, newBoard(nil, nil, nil).tools()...)
}

// The submit tools' definitions sit in the cached prefix: any change shows
// here first. Run with -update to accept one.
func TestSubmitTools_Golden(t *testing.T) {
	for _, tl := range newBoard(nil, nil, nil).tools() {
		def := struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		}{tl.Name(), tl.Description(), tl.Schema()}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(def); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", tl.Name()+".json")
		if *update {
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (run go test -update to write it)", err)
		}
		if !bytes.Equal(buf.Bytes(), want) {
			t.Errorf("%s's definition changed:\n%s\nwant\n%s", tl.Name(), buf.Bytes(), want)
		}
	}
}

// Strict tool use: every object closes additionalProperties and requires
// every property, and no keyword strict mode refuses appears.
func TestSubmitTools_Strict(t *testing.T) {
	unsupported := []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems", "pattern", "$ref", "default", "format"}
	var walk func(tool, path string, node any)
	walk = func(tool, path string, node any) {
		switch n := node.(type) {
		case map[string]any:
			for _, k := range unsupported {
				if _, ok := n[k]; ok {
					t.Errorf("%s %s: %s isn't supported in strict mode", tool, path, k)
				}
			}
			if n["type"] == "object" {
				var keys, required []string
				for k := range n["properties"].(map[string]any) {
					keys = append(keys, k)
				}
				for _, r := range n["required"].([]any) {
					required = append(required, r.(string))
				}
				sort.Strings(keys)
				sort.Strings(required)
				if n["additionalProperties"] != false || !reflect.DeepEqual(keys, required) {
					t.Errorf("%s %s: required %v, properties %v, additionalProperties %v", tool, path, required, keys, n["additionalProperties"])
				}
			}
			for k, v := range n {
				walk(tool, path+"."+k, v)
			}
		case []any:
			for i, v := range n {
				walk(tool, fmt.Sprintf("%s[%d]", path, i), v)
			}
		}
	}
	for _, tl := range newBoard(nil, nil, nil).tools() {
		var root any
		if err := json.Unmarshal(tl.Schema(), &root); err != nil {
			t.Fatal(err)
		}
		walk(tl.Name(), "$", root)
	}
}

// The structured-outputs limits hold for a run's whole request: at most 16
// parameters with a union type (anyOf or a type array), 24 optional
// parameters and 20 strict tools, across every strict tool. A run uses
// none of the first two (sentinels, not nullables), so Ask's tools (#158)
// keep the whole budget.
func TestRunTools_SchemaBudget(t *testing.T) {
	unions, optional := 0, 0
	for _, tl := range allTools() {
		var root any
		if err := json.Unmarshal(tl.Schema(), &root); err != nil {
			t.Fatal(err)
		}
		u, o := countSchema(root)
		unions += u
		optional += o
	}
	if n := len(allTools()); n > 20 {
		t.Errorf("%d strict tools, want at most 20", n)
	}
	if unions != 0 || optional != 0 {
		t.Errorf("a run's tools use %d union and %d optional parameters, want none", unions, optional)
	}
}

// countSchema counts the schemas under node with a union type, and the
// properties an object doesn't require.
func countSchema(node any) (unions, optional int) {
	switch v := node.(type) {
	case map[string]any:
		if _, ok := v["anyOf"]; ok {
			unions++
		}
		if _, ok := v["type"].([]any); ok {
			unions++
		}
		if props, ok := v["properties"].(map[string]any); ok {
			required := map[string]bool{}
			if list, ok := v["required"].([]any); ok {
				for _, r := range list {
					required[r.(string)] = true
				}
			}
			for k := range props {
				if !required[k] {
					optional++
				}
			}
		}
		for _, child := range v {
			u, o := countSchema(child)
			unions, optional = unions+u, optional+o
		}
	case []any:
		for _, child := range v {
			u, o := countSchema(child)
			unions, optional = unions+u, optional+o
		}
	}
	return unions, optional
}

func TestCountSchema(t *testing.T) {
	var schema any
	_ = json.Unmarshal([]byte(`{"type":"object","properties":{"a":{"anyOf":[{"type":"string"},{"type":"null"}]},
		"b":{"type":["integer","null"]},"c":{"type":"array","items":{"type":"object","properties":{"x":{"type":"string"},"y":{"type":"string"}},"required":["x"]}}},"required":["a"]}`), &schema)
	if u, o := countSchema(schema); u != 2 || o != 3 {
		t.Errorf("countSchema = %d unions, %d optional; want 2 and 3", u, o)
	}
}

// The system prompt is byte-stable (cached): nothing in it varies by run.
func TestSystemPrompt_Stable(t *testing.T) {
	for _, volatile := range []string{"2026", "Today", "%s", "%d"} {
		if strings.Contains(systemPrompt, volatile) {
			t.Errorf("the system prompt holds %q", volatile)
		}
	}
	h := newHarness(t)
	a := h.runner.request("claude-opus-5-5", aicore.UserText("a"), nil, nil, 1)
	b := h.runner.request("claude-opus-5-5", aicore.UserText("b"), nil, nil, 2)
	if a.System != b.System || !bytes.Equal(toolDefs(t, a.Tools), toolDefs(t, b.Tools)) {
		t.Error("two runs' prefixes differ")
	}
}

func toolDefs(t *testing.T, list []agent.Tool) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, tl := range list {
		buf.WriteString(tl.Name() + "\n" + tl.Description() + "\n")
		buf.Write(tl.Schema())
	}
	return buf.Bytes()
}
