package tools

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

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

var update = flag.Bool("update", false, "rewrite the golden tool definitions")

func allTools() []agent.Tool {
	return All(Deps{Inputs: Fixed{}, TMDB: &fakeTMDB{}, Titles: &fakeTitles{}})
}

// The definitions sit in the cached prompt prefix: any change shows up
// here first. Run with -update to accept one.
func TestTools_Golden(t *testing.T) {
	var names []string
	for _, tl := range allTools() {
		names = append(names, tl.Name())
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
		got := buf.Bytes()
		path := filepath.Join("testdata", tl.Name()+".json")
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (run go test -update to write it)", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s's definition changed:\n%s\nwant\n%s", tl.Name(), got, want)
		}
	}
	if want := []string{"get_taste_profile", "get_candidates", "tmdb_search", "tmdb_details", "check_seen"}; !reflect.DeepEqual(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

// unsupported are JSON Schema keywords strict tool use refuses.
var unsupported = []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
	"minLength", "maxLength", "minItems", "maxItems", "uniqueItems", "pattern", "patternProperties", "$ref", "default"}

// Every schema is strict-compatible: every object closes
// additionalProperties and requires every property (optional ones are
// nullable), and no keyword strict mode refuses appears. The bytes are
// compact and the same on every call.
func TestTools_StrictSchemas(t *testing.T) {
	for _, tl := range allTools() {
		schema := tl.Schema()
		if !bytes.Equal(schema, tl.Schema()) {
			t.Errorf("%s: the schema isn't byte-stable", tl.Name())
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, schema); err != nil || !bytes.Equal(buf.Bytes(), schema) {
			t.Errorf("%s: the schema isn't compact JSON (%v)", tl.Name(), err)
		}
		var root map[string]any
		if err := json.Unmarshal(schema, &root); err != nil {
			t.Fatal(err)
		}
		if root["type"] != "object" {
			t.Errorf("%s: the root isn't an object", tl.Name())
		}
		checkStrict(t, tl.Name(), "$", root)
	}
}

func checkStrict(t *testing.T, tool, path string, node any) {
	switch n := node.(type) {
	case map[string]any:
		for _, k := range unsupported {
			if _, ok := n[k]; ok {
				t.Errorf("%s %s: %s isn't supported in strict mode", tool, path, k)
			}
		}
		if n["type"] == "object" {
			if n["additionalProperties"] != false {
				t.Errorf("%s %s: additionalProperties isn't false", tool, path)
			}
			props, _ := n["properties"].(map[string]any)
			var keys, required []string
			for k := range props {
				keys = append(keys, k)
			}
			for _, r := range n["required"].([]any) {
				required = append(required, r.(string))
			}
			sort.Strings(keys)
			sort.Strings(required)
			if !reflect.DeepEqual(keys, required) && !(len(keys) == 0 && len(required) == 0) {
				t.Errorf("%s %s: required %v, properties %v", tool, path, required, keys)
			}
		}
		for k, v := range n {
			checkStrict(t, tool, path+"."+k, v)
		}
	case []any:
		for i, v := range n {
			checkStrict(t, tool, fmt.Sprintf("%s[%d]", path, i), v)
		}
	}
}

// fakeTitles answers TitleStates from a table.
type fakeTitles struct {
	states map[library.TitleRef]library.TitleState
	calls  int
	err    error
}

func (f *fakeTitles) TitleStates(_ context.Context, refs []library.TitleRef) ([]library.TitleState, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([]library.TitleState, len(refs))
	for i, ref := range refs {
		s := f.states[ref]
		s.TitleRef = ref
		out[i] = s
	}
	return out, nil
}

// fakeTMDB answers search and details.
type fakeTMDB struct {
	searchErr map[string]error
	queries   []string
}

func (f *fakeTMDB) Search(_ context.Context, mediaType, query string, year int) (*tmdb.ResultsPage, error) {
	f.queries = append(f.queries, fmt.Sprintf("%s %s %d", mediaType, query, year))
	if err := f.searchErr[mediaType]; err != nil {
		return nil, err
	}
	if mediaType == tmdb.MediaMovie {
		return &tmdb.ResultsPage{Results: []tmdb.ListItem{
			{ID: 693134, Title: "Dune: Part Two", ReleaseDate: "2024-02-27", OriginalLanguage: "en", GenreIDs: []int{878, 999},
				VoteAverage: 8.14, VoteCount: 6000, Popularity: 120, Overview: strings.Repeat("Spice. ", 40)},
			{ID: 438631, Title: "Dune", ReleaseDate: "2021-09-15", VoteAverage: 7.8, VoteCount: 12, Popularity: 300},
		}}, nil
	}
	return &tmdb.ResultsPage{Results: []tmdb.ListItem{
		{ID: 90228, Name: "Dune: Prophecy", FirstAirDate: "2024-11-17", OriginalLanguage: "en", GenreIDs: []int{10765}, Popularity: 200},
	}}, nil
}

func (f *fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	if id != 693134 {
		return nil, tmdb.ErrNotFound
	}
	return &tmdb.MovieDetails{ID: id, Title: "Dune: Part Two", OriginalTitle: "Dune: Part Two", ReleaseDate: "2024-02-27", Status: "Released",
		Runtime: 166, OriginalLanguage: "en", OriginCountry: []string{"US"}, Genres: []tmdb.Genre{{ID: 878, Name: "Science Fiction"}},
		Keywords: tmdb.Keywords{Keywords: []tmdb.Keyword{{ID: 1, Name: "desert"}, {ID: 2, Name: "messiah"}}},
		Credits: tmdb.Credits{Crew: []tmdb.CrewMember{{Name: "Denis Villeneuve", Job: "Director"}},
			Cast: []tmdb.CastMember{{Name: "Timothée Chalamet"}, {Name: "Zendaya"}}},
		VoteAverage: 8.14, VoteCount: 6000, Tagline: "Long live the fighters.", Overview: "Paul Atreides unites with the Fremen."}, nil
}

func (f *fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	if id == 500 {
		return nil, &tmdb.StatusError{StatusCode: 503}
	}
	if id != 209867 {
		return nil, tmdb.ErrNotFound
	}
	return &tmdb.ShowDetails{ID: id, Name: "Frieren: Beyond Journey's End", OriginalName: "葬送のフリーレン", FirstAirDate: "2023-09-29",
		LastAirDate: "2024-03-22", Status: "Ended", NumberOfEpisodes: 28, EpisodeRunTime: []int{24}, OriginalLanguage: "ja",
		OriginCountry: []string{"JP"}, Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}},
		Seasons:   []tmdb.SeasonSummary{{SeasonNumber: 0}, {SeasonNumber: 1}},
		CreatedBy: []tmdb.Creator{{Name: "Kanehito Yamada"}}, VoteAverage: 8.8, VoteCount: 400}, nil
}

func (f *fakeTMDB) GenreList(_ context.Context, mediaType string) ([]tmdb.Genre, error) {
	if mediaType == tmdb.MediaTV {
		return []tmdb.Genre{{ID: 10765, Name: "Sci-Fi & Fantasy"}}, nil
	}
	return []tmdb.Genre{{ID: 878, Name: "Science Fiction"}}, nil
}

func run(t *testing.T, tl agent.Tool, input string) (string, error) {
	t.Helper()
	out, err := tl.Run(context.Background(), json.RawMessage(input))
	return string(out), err
}

func toolError(err error) string {
	var te *agent.ToolError
	if errors.As(err, &te) {
		return te.Message
	}
	return ""
}

func TestGetTasteProfile(t *testing.T) {
	p := &profile.Profile{Counts: profile.Counts{Loved: 7, Plays: 120}, Genres: []profile.Weighted{{ID: 18, Name: "Drama", W: 0.6}}}
	tl := NewGetTasteProfile(Fixed{P: p})
	out, err := run(t, tl, `{}`)
	if err != nil || !strings.Contains(out, `"counts":{"loved":7,"completed":0,"dropped":0,"rated":0,"plays":120}`) ||
		!strings.Contains(out, `"genres":[{"id":18,"name":"Drama","w":0.6}]`) {
		t.Errorf("profile = %s, %v", out, err)
	}
	if _, err := run(t, tl, `{"extra": 1}`); toolError(err) == "" {
		t.Errorf("unknown field = %v, want a tool error", err)
	}
}

func TestGetCandidates(t *testing.T) {
	pool := &candidates.Pool{Stats: candidates.Stats{Considered: 40, Excluded: 6, Kept: 3}, Items: []candidates.Candidate{
		{MediaType: "tv", TMDBID: 1, Title: "Dark", Genres: []string{"Drama"}, Runtime: 55, Why: []string{"rec:Severance"}},
		{MediaType: "movie", TMDBID: 2, Title: "Annihilation", Genres: []string{"Science Fiction"}, Runtime: 115, Why: []string{"kw"}},
		{MediaType: "tv", TMDBID: 3, Title: "Disliked Since", Genres: []string{"Drama"}, Runtime: 50, Why: []string{"trending"}},
	}}
	titles := &fakeTitles{states: map[library.TitleRef]library.TitleState{
		{MediaType: "tv", TMDBID: 3}: {Feedback: "dislike"},
	}}
	tl := NewGetCandidates(Fixed{C: pool}, titles)
	nulls := `"media_type":null,"genres":null,"min_runtime":null,"max_runtime":null,"year_from":null,"year_to":null,"language":null,"anime":null`

	out, err := run(t, tl, `{`+nulls+`,"limit":null}`)
	want := `{"matched":2,"pool":{"considered":40,"excluded":6,"kept":3},"items":[` +
		`{"media_type":"tv","tmdb_id":1,"title":"Dark","genres":["Drama"],"runtime":55,"why":["rec:Severance"]},` +
		`{"media_type":"movie","tmdb_id":2,"title":"Annihilation","genres":["Science Fiction"],"runtime":115,"why":["kw"]}]}`
	if err != nil || out != want {
		t.Errorf("all = %s, %v\nwant %s", out, err, want)
	}
	out, err = run(t, tl, `{"media_type":"tv","genres":["drama"],"min_runtime":null,"max_runtime":60,"year_from":null,"year_to":null,"language":null,"anime":null,"limit":1}`)
	if err != nil || !strings.HasPrefix(out, `{"matched":1,`) || !strings.Contains(out, `"title":"Dark"`) {
		t.Errorf("filtered = %s, %v", out, err)
	}
	if _, err := run(t, tl, `{`+nulls+`,"limit":-1}`); toolError(err) == "" {
		t.Errorf("negative limit = %v, want a tool error", err)
	}
	if _, err := run(t, tl, `{"media_type":"show","genres":null,"min_runtime":null,"max_runtime":null,"year_from":null,"year_to":null,"language":null,"anime":null,"limit":null}`); toolError(err) == "" {
		t.Errorf("bad media type = %v, want a tool error", err)
	}
	// The store failing ends the run: it isn't the model's to fix
	titles.err = errors.New("db down")
	if _, err := run(t, tl, `{`+nulls+`,"limit":null}`); err == nil || toolError(err) != "" {
		t.Errorf("db error = %v, want a plain error", err)
	}
}

func TestTMDBSearch(t *testing.T) {
	nine := 9
	titles := &fakeTitles{states: map[library.TitleRef]library.TitleState{
		{MediaType: "movie", TMDBID: 693134}: {Plays: 2, Started: true, Rating: &nine, InLibrary: true},
	}}
	tm := &fakeTMDB{}
	tl := NewTMDBSearch(tm, titles)

	out, err := run(t, tl, `{"query":"  dune ","media_type":null,"year":null}`)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	// Both types, most popular first; a vote needs 50 votes
	var order []string
	for _, r := range res.Results {
		order = append(order, r["title"].(string))
	}
	if !reflect.DeepEqual(order, []string{"Dune", "Dune: Prophecy", "Dune: Part Two"}) {
		t.Errorf("order = %v", order)
	}
	part2 := res.Results[2]
	if part2["excluded"] != "watched" || part2["your_rating"] != float64(9) || part2["in_library"] != true || part2["vote"] != 8.1 ||
		!reflect.DeepEqual(part2["genres"], []any{"Science Fiction"}) || len([]rune(part2["overview"].(string))) != searchOverview {
		t.Errorf("Dune: Part Two = %v", part2)
	}
	if _, ok := res.Results[0]["vote"]; ok {
		t.Errorf("a 12-vote average is shown: %v", res.Results[0])
	}
	if !reflect.DeepEqual(tm.queries, []string{"movie dune 0", "tv dune 0"}) {
		t.Errorf("queries = %v", tm.queries)
	}

	tm.queries = nil
	if _, err := run(t, tl, `{"query":"dune","media_type":"tv","year":2024}`); err != nil || !reflect.DeepEqual(tm.queries, []string{"tv dune 2024"}) {
		t.Errorf("tv 2024: %v, queries %v", err, tm.queries)
	}
	if _, err := run(t, tl, `{"query":" ","media_type":null,"year":null}`); toolError(err) == "" {
		t.Errorf("empty query = %v", err)
	}
	// One type failing still answers; both failing is the model's to know
	tm.searchErr = map[string]error{"tv": &tmdb.StatusError{StatusCode: 500}}
	if out, err := run(t, tl, `{"query":"dune","media_type":null,"year":null}`); err != nil || strings.Contains(out, "Prophecy") {
		t.Errorf("tv down = %s, %v", out, err)
	}
	tm.searchErr["movie"] = &tmdb.StatusError{StatusCode: 500}
	if _, err := run(t, tl, `{"query":"dune","media_type":null,"year":null}`); !strings.Contains(toolError(err), "TMDB didn't answer") {
		t.Errorf("TMDB down = %v", err)
	}
	tm.searchErr["movie"] = context.Canceled
	tm.searchErr["tv"] = context.Canceled
	if _, err := run(t, tl, `{"query":"dune","media_type":null,"year":null}`); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled = %v, want it to end the run", err)
	}
}

func TestTMDBDetails(t *testing.T) {
	titles := &fakeTitles{states: map[library.TitleRef]library.TitleState{
		{MediaType: "tv", TMDBID: 209867}: {Started: true, Aired: 28, Completed: 10, Feedback: "like", OnWatchlist: true},
	}}
	tl := NewTMDBDetails(&fakeTMDB{}, titles)

	out, err := run(t, tl, `{"media_type":"movie","tmdb_id":693134}`)
	want := `{"media_type":"movie","tmdb_id":693134,"title":"Dune: Part Two","date":"2024-02-27","tmdb_status":"Released","runtime":166,` +
		`"genres":["Science Fiction"],"keywords":["desert","messiah"],"lang":"en","countries":["US"],"creators":["Denis Villeneuve"],` +
		`"cast":["Timothée Chalamet","Zendaya"],"vote":8.1,"votes":6000,"tagline":"Long live the fighters.","overview":"Paul Atreides unites with the Fremen."}`
	if err != nil || out != want {
		t.Errorf("movie = %s, %v\nwant %s", out, err, want)
	}
	out, err = run(t, tl, `{"media_type":"tv","tmdb_id":209867}`)
	want = `{"media_type":"tv","tmdb_id":209867,"title":"Frieren: Beyond Journey's End","original_title":"葬送のフリーレン","date":"2023-09-29",` +
		`"year_end":2024,"tmdb_status":"Ended","runtime":24,"seasons":1,"episodes":28,"genres":["Animation"],"keywords":[],"lang":"ja",` +
		`"countries":["JP"],"anime":true,"creators":["Kanehito Yamada"],"vote":8.8,"votes":400,"excluded":"started","liked":true,"watchlist":true}`
	if err != nil || out != want {
		t.Errorf("show = %s, %v\nwant %s", out, err, want)
	}
	if _, err := run(t, tl, `{"media_type":"movie","tmdb_id":1}`); toolError(err) != "TMDB has no movie 1." {
		t.Errorf("unknown = %v", err)
	}
	if _, err := run(t, tl, `{"media_type":"tv","tmdb_id":500}`); !strings.Contains(toolError(err), "didn't answer") {
		t.Errorf("TMDB down = %v", err)
	}
	if _, err := run(t, tl, `{"media_type":"tv","tmdb_id":0}`); toolError(err) == "" {
		t.Errorf("id 0 = %v", err)
	}
}

func TestCheckSeen(t *testing.T) {
	eight := 8
	titles := &fakeTitles{states: map[library.TitleRef]library.TitleState{
		{MediaType: "movie", TMDBID: 1}: {Plays: 3, Started: true, Rating: &eight},
		{MediaType: "tv", TMDBID: 2}:    {Started: true, Aired: 38, Completed: 26, Plays: 26},
		{MediaType: "tv", TMDBID: 3}:    {Dropped: true},
		{MediaType: "movie", TMDBID: 4}: {Feedback: "not_interested"},
		{MediaType: "movie", TMDBID: 5}: {Feedback: "like", InLibrary: true},
	}}
	tl := NewCheckSeen(titles)
	out, err := run(t, tl, `{"titles":[{"media_type":"movie","tmdb_id":1},{"media_type":"tv","tmdb_id":2},{"media_type":"tv","tmdb_id":3},`+
		`{"media_type":"movie","tmdb_id":4},{"media_type":"movie","tmdb_id":5},{"media_type":"tv","tmdb_id":6}]}`)
	want := `{"items":[` +
		`{"media_type":"movie","tmdb_id":1,"can_pick":false,"plays":3,"excluded":"watched","your_rating":8},` +
		`{"media_type":"tv","tmdb_id":2,"can_pick":false,"plays":26,"progress":"26/38","excluded":"started"},` +
		`{"media_type":"tv","tmdb_id":3,"can_pick":false,"excluded":"dropped on Trakt"},` +
		`{"media_type":"movie","tmdb_id":4,"can_pick":false,"excluded":"not interested"},` +
		`{"media_type":"movie","tmdb_id":5,"can_pick":true,"liked":true,"in_library":true},` +
		`{"media_type":"tv","tmdb_id":6,"can_pick":true}],"can_pick":2,"excluded":4}`
	if err != nil || out != want {
		t.Errorf("check_seen = %s, %v\nwant %s", out, err, want)
	}
	if out, err := run(t, tl, `{"titles":[]}`); err != nil || out != `{"items":[],"can_pick":0,"excluded":0}` {
		t.Errorf("none = %s, %v", out, err)
	}
	var many []string
	for i := 1; i <= MaxCheckSeen+1; i++ {
		many = append(many, fmt.Sprintf(`{"media_type":"movie","tmdb_id":%d}`, i))
	}
	if _, err := run(t, tl, `{"titles":[`+strings.Join(many, ",")+`]}`); toolError(err) == "" {
		t.Errorf("51 titles = %v, want a tool error", err)
	}
	if _, err := run(t, tl, `{"titles":[{"media_type":"person","tmdb_id":1}]}`); toolError(err) == "" {
		t.Errorf("a person = %v, want a tool error", err)
	}
}

// Cached builds once, shares the build, and rebuilds when stale.
func TestCached(t *testing.T) {
	profiles, pools := 0, 0
	c := NewCached(
		func(context.Context) (*profile.Profile, error) {
			profiles++
			return &profile.Profile{Counts: profile.Counts{Loved: profiles}}, nil
		},
		func(_ context.Context, p *profile.Profile) (*candidates.Pool, error) {
			pools++
			return &candidates.Pool{Stats: candidates.Stats{Kept: p.Counts.Loved}}, nil
		}, time.Hour)
	clock := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return clock }
	ctx := context.Background()

	pool, err := c.Pool(ctx)
	if err != nil || pool.Stats.Kept != 1 {
		t.Fatalf("pool = %+v, %v", pool, err)
	}
	if p, _ := c.Profile(ctx); p.Counts.Loved != 1 || profiles != 1 || pools != 1 {
		t.Errorf("builds: %d profiles, %d pools", profiles, pools)
	}
	clock = clock.Add(2 * time.Hour)
	if pool, _ := c.Pool(ctx); pool.Stats.Kept != 2 || profiles != 2 || pools != 2 {
		t.Errorf("stale: kept %d, %d profiles, %d pools", pool.Stats.Kept, profiles, pools)
	}
	c.Invalidate()
	if p, _ := c.Profile(ctx); p.Counts.Loved != 3 {
		t.Errorf("after invalidate: %+v", p)
	}
	failing := NewCached(func(context.Context) (*profile.Profile, error) { return nil, errors.New("no db") }, nil, time.Hour)
	if _, err := failing.Pool(ctx); err == nil {
		t.Error("a failed profile build made a pool")
	}
}
