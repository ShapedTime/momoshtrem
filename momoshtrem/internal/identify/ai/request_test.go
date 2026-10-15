package ai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

func paramsJSON(t *testing.T, q request) string {
	t.Helper()
	data, err := json.Marshal(q.params(DefaultModel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBuildRequestsIsDeterministic(t *testing.T) {
	in := testInput()
	scope := effectiveScope(in)
	a := buildRequests(in, scope)
	b := buildRequests(in, scope)
	if paramsJSON(t, a[0]) != paramsJSON(t, b[0]) {
		t.Error("two builds of the same input differ")
	}

	// The torrent's file order and the scope's order don't matter.
	shuffled := testInput()
	slices.Reverse(shuffled.Rules)
	slices.Reverse(shuffled.Files)
	rev := slices.Clone(scope)
	slices.Reverse(rev)
	if c := buildRequests(shuffled, rev); paramsJSON(t, a[0]) != paramsJSON(t, c[0]) {
		t.Errorf("file order changed the request:\n%s\n---\n%s", a[0].context+a[0].ask, c[0].context+c[0].ask)
	}
}

func TestSystemPromptIsStableAndCached(t *testing.T) {
	one := buildRequests(testInput(), effectiveScope(testInput()))[0].params(DefaultModel)
	other := testInput()
	other.TorrentName = "Something.Else.S01.1080p"
	other.Hint = "These are specials"
	two := buildRequests(other, effectiveScope(other))[0].params(DefaultModel)

	for _, p := range []struct {
		name string
		sys  string
		ttl  string
	}{{"first", one.System[0].Text, string(one.System[0].CacheControl.Type)}, {"second", two.System[0].Text, string(two.System[0].CacheControl.Type)}} {
		if p.sys != systemPrompt || p.ttl != "ephemeral" {
			t.Errorf("%s request: system prompt isn't the cached constant (cache_control %q)", p.name, p.ttl)
		}
	}
	if len(one.System) != 1 || len(two.System) != 1 {
		t.Errorf("system has %d and %d blocks, want 1", len(one.System), len(two.System))
	}
	// Claude Haiku 5.5 caches prompts from 512 tokens: at roughly 4
	// characters a token, the system prompt must be well over 2,048.
	if len(systemPrompt) < 4000 {
		t.Errorf("system prompt is %d characters, too short to cache", len(systemPrompt))
	}
	for _, volatile := range []string{"Frieren", "Something.Else", "These are specials"} {
		if strings.Contains(systemPrompt, volatile) {
			t.Errorf("system prompt contains %q", volatile)
		}
	}
	if one.OutputConfig.Effort != "low" || one.OutputConfig.Format.Schema == nil {
		t.Errorf("output config = %+v, want low effort and a JSON schema", one.OutputConfig)
	}
	if one.Model != DefaultModel {
		t.Errorf("model = %q", one.Model)
	}
}

func TestScope(t *testing.T) {
	in := testInput()
	tests := []struct {
		name  string
		scope []string
		want  []string // listed paths in <resolve>, in order
	}{
		{"empty means the unsure files", nil,
			[]string{"Frieren S2 - 03.mkv", "Frieren S2 - 04.mkv"}},
		{"an explicit scope", []string{folder + "/Frieren S2 - 01.mkv", folder + "/Frieren S2 - 04.mkv"},
			[]string{"Frieren S2 - 01.mkv", "Frieren S2 - 04.mkv"}},
		{"subtitles, fonts and unknown paths are dropped",
			[]string{folder + "/Frieren S2 - 04.en.srt", folder + "/Fonts/a.ttf", "elsewhere.mkv", folder + "/Frieren S2 - 02.mkv"},
			[]string{"Frieren S2 - 02.mkv"}},
		{"every main video", Scope(in.Rules, true),
			[]string{"Frieren S2 - 01.mkv", "Frieren S2 - 02.mkv", "Frieren S2 - 03.mkv", "Frieren S2 - 04.mkv"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := testInput()
			in.Scope = tt.scope
			reqs := buildRequests(in, effectiveScope(in))
			if len(reqs) != 1 {
				t.Fatalf("%d requests, want 1", len(reqs))
			}
			var got []string
			for _, p := range reqs[0].paths {
				got = append(got, reqs[0].listed(p))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("asked about %q, want %q", got, tt.want)
			}
			for _, p := range tt.want {
				if !strings.Contains(reqs[0].ask, p) {
					t.Errorf("<resolve> doesn't list %q", p)
				}
			}
		})
	}
}

func TestPromptContent(t *testing.T) {
	in := testInput()
	in.Hint = "Season 2 is the second part of season 1"
	q := buildRequests(in, effectiveScope(in))[0]
	text := q.context + q.ask
	for _, want := range []string{
		"name: " + folder,
		"folder: " + folder,
		"files: 6, of which 4 videos",
		"title: Frieren: Beyond Journey's End",
		"other titles: Frieren",
		"anime: yes",
		"S00 (specials): 2 episodes",
		"S01: 32 episodes, #1-#32, aired in 2 parts: E01-E28 2023-09-29 to 2024-04-05; E29-E32 2026-01-16 to 2026-02-06",
		"S01E29 #29 2026-01-16 | Episode 29",
		"S00E02 2024-03-15 | Recap",
		"1.0 GB | Frieren S2 - 01.mkv | rules: S01E29 high",
		"(also 1 subtitle, 1 font, not listed)",
		"<hint>\nSeason 2 is the second part of season 1\n</hint>",
		"1. Frieren S2 - 03.mkv\n   size 1.0 GB; rules: S01E03 medium; why: 03 read in season 1",
		"2. Frieren S2 - 04.mkv\n   size 1.0 GB; rules: no match; why: No episode 4 in season 2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt lacks %q\n%s", want, text)
		}
	}

	// Without the rules' answers (a caller that didn't classify), none are
	// shown.
	in.Rules = nil
	in.Scope = []string{folder + "/Frieren S2 - 04.mkv"}
	q = buildRequests(in, effectiveScope(in))[0]
	if strings.Contains(q.context+q.ask, "rules:") || !strings.Contains(q.ask, "Frieren S2 - 04.mkv") {
		t.Errorf("prompt without rules:\n%s", q.context+q.ask)
	}
}

// bigTorrent is a 500-video absolute-numbered batch.
func bigTorrent(n int) identify.ResolveInput {
	in := identify.ResolveInput{TorrentName: "Big", Show: testShow()}
	for i := 1; i <= n; i++ {
		p := identify.Proposal{Path: fmt.Sprintf("Big/Show - %03d.mkv", i), Size: 1 << 30, Kind: identify.KindEpisode,
			Targets: []identify.Target{{EpisodeID: int64(100 + i), Season: 1, Episode: i}}, Confidence: identify.ConfidenceHigh}
		in.Rules = append(in.Rules, p)
		in.Files = append(in.Files, identify.TorrentFile{Path: p.Path, Size: p.Size})
	}
	return in
}

func TestFilesCollapseOver400(t *testing.T) {
	in := bigTorrent(500)
	in.Scope = []string{"Big/Show - 250.mkv", "Big/Show - 400.mkv"}
	q := buildRequests(in, effectiveScope(in))[0]
	files := q.context[strings.Index(q.context, "<files>"):]
	for _, want := range []string{
		"| Show - 249.mkv |", "| Show - 250.mkv |", "| Show - 251.mkv |",
		"| Show - 399.mkv |", "| Show - 400.mkv |", "| Show - 401.mkv |",
		"[248 more files, Show - 001.mkv to Show - 248.mkv; rules: 248 high]",
		"[147 more files, Show - 252.mkv to Show - 398.mkv; rules: 147 high]",
		"[99 more files, Show - 402.mkv to Show - 500.mkv; rules: 99 high]",
	} {
		if !strings.Contains(files, want) {
			t.Errorf("<files> lacks %q", want)
		}
	}
	if strings.Contains(files, "| Show - 100.mkv |") {
		t.Error("a file far from the scope is listed")
	}

	// At 400 files or fewer, every file is listed.
	small := bigTorrent(400)
	small.Scope = []string{"Big/Show - 250.mkv"}
	q = buildRequests(small, effectiveScope(small))[0]
	if strings.Contains(q.context, "more files") || !strings.Contains(q.context, "| Show - 100.mkv |") {
		t.Error("a 400-file torrent was collapsed")
	}
}

// longShow is a One Piece-like catalog: 3 specials and 1,100 episodes over
// 11 seasons of 100, numbered absolutely, aired daily from 2000.
func longShow() *identify.ShowCatalog {
	cat := &identify.ShowCatalog{Title: "One Piece", IsAnime: true, Ordering: identify.OrderingAbsolute}
	for e := 1; e <= 3; e++ {
		cat.Episodes = append(cat.Episodes, identify.Episode{ID: int64(e), Season: 0, Number: e, Name: fmt.Sprintf("Special %d", e)})
	}
	for abs := 1; abs <= 1100; abs++ {
		s, e := (abs-1)/100+1, (abs-1)%100+1
		cat.Episodes = append(cat.Episodes, identify.Episode{
			ID: int64(1000 + abs), Season: s, Number: e, Absolute: abs,
			AirDate: fmt.Sprintf("%04d-%02d-%02d", 2000+abs/365, abs%365/31+1, abs%31%28+1),
			Name:    fmt.Sprintf("Episode %d", abs),
		})
	}
	return cat
}

// unsureFiles is an input of files the rules couldn't place.
func unsureFiles(cat *identify.ShowCatalog, paths ...string) identify.ResolveInput {
	in := identify.ResolveInput{TorrentName: "x", Show: cat}
	for _, p := range paths {
		in.Rules = append(in.Rules, identify.Proposal{Path: p, Size: 1 << 30, Kind: identify.KindUnknown})
		in.Files = append(in.Files, identify.TorrentFile{Path: p, Size: 1 << 30})
	}
	return in
}

// A long show lists the episodes around the numbers in the names, however
// a release writes them, and every special.
func TestEpisodeWindow(t *testing.T) {
	for _, name := range []string{
		"[Erai-raws] One Piece - 1071 [1080p][ABCD1234].mkv",
		"[Erai-raws] One Piece - 1071v2 [1080p][ABCD1234].mkv",
		"[HorribleSubs]_One_Piece_-_1071_[1080p].mkv",
		"One.Piece.E1071.1080p.WEB.x264-GRP.mkv",
		"One Piece Ep1071 1080p.mkv",
		"One Piece - EP1071.mkv",
		"One Piece - 1071 (H.265 10bit AAC 5.1).mkv",
		"[Group] One Piece - 1071 [2160p][1920x1080][2022].mkv",
		"[Group] One Piece/[Group] One Piece - 1071.mkv",
		"One Piece #1071.mkv",
		"One Piece 第1071話.mkv",
	} {
		t.Run(name, func(t *testing.T) {
			q := buildRequests(unsureFiles(longShow(), name), []string{name})[0]
			if !strings.Contains(q.context, `<episodes listed="24 of 1103"`) || q.episodes != 24 {
				t.Errorf("listed %d episodes: %s", q.episodes, q.context[strings.Index(q.context, "<episodes"):][:60])
			}
			for _, want := range []string{"S00E01", "S00E03", "S11E61 #1061", "S11E71 #1071", "S11E81 #1081"} {
				if !strings.Contains(q.context, want) {
					t.Errorf("window lacks %s", want)
				}
			}
			for _, unwanted := range []string{"#1060 ", "#1082 ", "#500 "} {
				if strings.Contains(q.context, unwanted) {
					t.Errorf("window has %s", unwanted)
				}
			}
		})
	}

	// A range lists the episodes it covers; a season mention reads bare
	// numbers in that season too.
	q := buildRequests(unsureFiles(longShow(), "[Grp] One Piece (1071-1090)/x.mkv"), []string{"[Grp] One Piece (1071-1090)/x.mkv"})[0]
	if !strings.Contains(q.context, "#1061 ") || !strings.Contains(q.context, "#1085 ") || !strings.Contains(q.context, "#1100 ") || strings.Contains(q.context, "#1060 ") {
		t.Errorf("range window: %d episodes", q.episodes)
	}
	q = buildRequests(unsureFiles(longShow(), "Season 3/05.mkv"), []string{"Season 3/05.mkv"})[0]
	if !strings.Contains(q.context, "S03E05 #205") {
		t.Errorf("season folder window: %d episodes", q.episodes)
	}

	// A show of 300 episodes or fewer is listed whole.
	small := testInput()
	if q := buildRequests(small, effectiveScope(small))[0]; q.episodes != 34 || strings.Contains(q.context, "listed=") {
		t.Errorf("a short show was windowed: %d episodes", q.episodes)
	}
}

func TestEpisodeWindowByDateAndTitle(t *testing.T) {
	cat := longShow()
	cat.Episodes = append(cat.Episodes, identify.Episode{ID: 9999, Season: 12, Number: 1, Absolute: 1101, Name: "The Payback Job", AirDate: "2030-01-01"})
	file := "Show.2003.01.15.1080p.WEB.mkv"
	byTitle := "One Piece - The Payback Job.mkv" // "payback" is its one word: the show's own don't count
	in := unsureFiles(cat, file, byTitle)
	q := buildRequests(in, effectiveScope(in))[0]
	if !strings.Contains(q.context, "| The Payback Job") {
		t.Error("the window lacks the episode named in a title-only file")
	}
	dated := 0
	for _, line := range strings.Split(q.context, "\n") {
		if strings.Contains(line, " 2003-01-") {
			dated++
		}
	}
	if dated == 0 {
		t.Error("the window lacks the episodes around 2003-01-15")
	}
}

// Title words common to a long show's episode names (or its own title)
// don't pull in the whole show.
func TestEpisodeWindowCommonTitleWords(t *testing.T) {
	cat := longShow()
	cat.Aliases = []string{"Wan Pisu"}
	for i := range cat.Episodes {
		if cat.Episodes[i].Season > 0 {
			cat.Episodes[i].Name = fmt.Sprintf("Luffy and the Pirate Crew, Part %d", cat.Episodes[i].Absolute)
		}
	}
	cat.Episodes[600].Name = "The Pirate Who Will Become King"
	for _, tt := range []struct {
		file string
		max  int
	}{
		{"One Piece - The Pirate Who Will Become King.mkv", 4}, // two shared words: the one episode, and the specials
		{"One Piece - Pirate Adventures.mkv", 3},               // one shared word of two: specials only
		{"Wan Pisu - Luffy and the Pirate Crew.mkv", 3},        // every name has these words: too many, none listed
		{"One Piece - Episode Special Part.mkv", 3},            // stop words only
	} {
		q := buildRequests(unsureFiles(cat, tt.file), []string{tt.file})[0]
		if q.episodes > tt.max {
			t.Errorf("%s: %d episodes listed, want at most %d", tt.file, q.episodes, tt.max)
		}
	}
	q := buildRequests(unsureFiles(cat, "One Piece - The Pirate Who Will Become King.mkv"), []string{"One Piece - The Pirate Who Will Become King.mkv"})[0]
	if !strings.Contains(q.context, "| The Pirate Who Will Become King") {
		t.Error("the episode named by the file isn't listed")
	}
}

func TestChunksAndMaxTokens(t *testing.T) {
	one := testInput()
	one.Scope = []string{folder + "/Frieren S2 - 04.mkv"}
	q := buildRequests(one, effectiveScope(one))[0]
	if q.maxTokens() != 4150 || q.shared {
		t.Errorf("one file: max_tokens %d, shared %v", q.maxTokens(), q.shared)
	}
	if p := q.params(DefaultModel); p.Messages[0].Content[0].OfText.CacheControl.Type != "" {
		t.Error("a single request caches its context: a write that's never read")
	}

	in := bigTorrent(130)
	in.Scope = Scope(in.Rules, true)
	reqs := buildRequests(in, effectiveScope(in))
	if len(reqs) != 3 {
		t.Fatalf("%d requests for 130 files, want 3", len(reqs))
	}
	total := 0
	for i, q := range reqs {
		total += len(q.paths)
		// One wave: the requests run together, so none could read what
		// another cached.
		if len(q.paths) > filesPerCall || q.shared || q.context != reqs[0].context {
			t.Errorf("request %d: %d files, shared %v", i, len(q.paths), q.shared)
		}
		if q.maxTokens() > 16000 {
			t.Errorf("request %d: max_tokens %d over the non-streaming limit", i, q.maxTokens())
		}
		if !strings.Contains(q.ask, fmt.Sprintf("Part %d of 3", i+1)) {
			t.Errorf("request %d doesn't say which part it is", i)
		}
	}
	if total != 130 {
		t.Errorf("the requests ask about %d files, want 130", total)
	}
	if reqs[0].maxTokens() != 13000 || reqs[2].maxTokens() != 5500 {
		t.Errorf("max_tokens = %d and %d, want 13000 and 5500", reqs[0].maxTokens(), reqs[2].maxTokens())
	}

	// More than one wave: later waves read the context the first cached.
	in = bigTorrent(300)
	in.Scope = Scope(in.Rules, true)
	reqs = buildRequests(in, effectiveScope(in))
	if len(reqs) != 5 || Waves(300) != 2 {
		t.Fatalf("%d requests and %d waves for 300 files, want 5 and 2", len(reqs), Waves(300))
	}
	for i, q := range reqs {
		if p := q.params(DefaultModel); !q.shared || p.Messages[0].Content[0].OfText.CacheControl.Type != "ephemeral" {
			t.Errorf("request %d: the shared context isn't cached", i)
		}
	}
}

func TestWaves(t *testing.T) {
	for _, tt := range []struct{ files, waves int }{{0, 1}, {1, 1}, {60, 1}, {240, 1}, {241, 2}, {1000, 5}} {
		if got := Waves(tt.files); got != tt.waves {
			t.Errorf("Waves(%d) = %d, want %d", tt.files, got, tt.waves)
		}
	}
}

// A 1,000-file batch (One Piece 1-1000, ai_always): 17 requests in five
// waves, each within max_tokens, the context listed once and cached.
func TestHugeBatch(t *testing.T) {
	var paths []string
	for abs := 1; abs <= 1000; abs++ {
		paths = append(paths, fmt.Sprintf("[Grp] One Piece [1-1000]/[Grp] One Piece - %04d [1080p].mkv", abs))
	}
	in := unsureFiles(longShow(), paths...)
	reqs := buildRequests(in, paths)
	if len(reqs) != 17 || Waves(len(paths)) != 5 {
		t.Fatalf("%d requests, %d waves", len(reqs), Waves(len(paths)))
	}
	for i, q := range reqs {
		if q.maxTokens() > 16000 || !q.shared || q.context != reqs[0].context {
			t.Errorf("request %d: max_tokens %d, shared %v", i, q.maxTokens(), q.shared)
		}
	}
}

// The request on the wire: structured output at low effort, adaptive
// thinking left to the model's default, two cache breakpoints at most.
func TestWireShape(t *testing.T) {
	in := bigTorrent(300)
	data, err := json.Marshal(buildRequests(in, Scope(in.Rules, true))[0].params(DefaultModel))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{`"model":"claude-haiku-5-5"`, `"effort":"low"`, `"type":"json_schema"`, `"max_tokens":13000`} {
		if !strings.Contains(s, want) {
			t.Errorf("the request lacks %s", want)
		}
	}
	if strings.Contains(s, `"thinking"`) || strings.Contains(s, `"temperature"`) {
		t.Error("the request sets thinking or sampling: Claude Haiku 5.5 runs adaptive thinking by default")
	}
	if n := strings.Count(s, `"cache_control"`); n != 2 {
		t.Errorf("%d cache breakpoints, want 2 (system prompt, shared context)", n)
	}
}

func TestScopeOfRules(t *testing.T) {
	tgt := func(id int64) []identify.Target {
		return []identify.Target{{EpisodeID: id, Season: 1, Episode: int(id)}}
	}
	rules := []identify.Proposal{
		{Path: "a.mkv", Kind: identify.KindEpisode, Targets: tgt(1), Confidence: identify.ConfidenceHigh},
		{Path: "b.mkv", Kind: identify.KindEpisode, Targets: tgt(2), Confidence: identify.ConfidenceMedium},
		{Path: "c.mkv", Kind: identify.KindEpisode, Targets: tgt(3), Confidence: identify.ConfidenceLow},
		{Path: "d.mkv", Kind: identify.KindUnknown},
		{Path: "e.mkv", Kind: identify.KindSpecial},
		{Path: "f.mkv", Kind: identify.KindEpisode, Targets: tgt(6), Confidence: identify.ConfidenceHigh},
		{Path: "g.mkv", Kind: identify.KindEpisode, Targets: tgt(6), Confidence: identify.ConfidenceHigh},
		{Path: "ncop.mkv", Kind: identify.KindExtra, ExtraType: identify.ExtraOpening},
		{Path: "sample.mkv", Kind: identify.KindSample, ExtraType: identify.ExtraSample},
		{Path: "d.srt", Kind: identify.KindSubtitle, LinkedVideo: "d.mkv"},
	}
	want := []string{"b.mkv", "c.mkv", "d.mkv", "e.mkv", "f.mkv", "g.mkv"}
	if got := Scope(rules, false); !slices.Equal(got, want) {
		t.Errorf("Scope(auto) = %q, want %q", got, want)
	}
	// ai_always: every main video; extras, samples and subtitles never.
	wantAll := []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv", "e.mkv", "f.mkv", "g.mkv"}
	if got := Scope(rules, true); !slices.Equal(got, wantAll) {
		t.Errorf("Scope(ai_always) = %q, want %q", got, wantAll)
	}
	if got := Scope(rules[:1], false); len(got) != 0 {
		t.Errorf("Scope of one high file = %q", got)
	}
}

// Answers are mapped by n, the path checking it: a path in another
// Unicode form, or shortened to the file name, still maps; one naming
// another file falls back to the path; an n out of range too.
func TestParseByNumber(t *testing.T) {
	nfd := "Show/Pokémon S01E01.mkv" // e + combining acute, as macOS writes it
	nfc := "Show/Pokémon S01E01.mkv"
	other := "Show/Other S01E02.mkv"
	q := buildRequests(unsureFiles(longShow(), nfd, other), []string{nfd, other})[0]
	if q.paths[0] != other || q.paths[1] != nfd {
		t.Fatalf("paths = %q", q.paths)
	}
	tests := []struct {
		name string
		n    int
		path string
		want string // "" for dropped
	}{
		{"n and the file name", 2, "Pokémon S01E01.mkv", nfd},
		{"n and the file name in NFC", 2, "Pokémon S01E01.mkv", nfd},
		{"n and the full path in NFC", 2, nfc, nfd},
		{"n and no path", 2, "", nfd},
		{"n names one file, the path another: the path decides", 1, "Pokémon S01E01.mkv", nfd},
		{"n out of range: the path", 7, "Other S01E02.mkv", other},
		{"no n: the path", 0, nfc, nfd},
		{"n and a path of no file", 2, "Elsewhere.mkv", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, _ := json.Marshal(map[string]any{"files": []map[string]any{{
				"n": tt.n, "path": tt.path, "kind": "episode", "season": 1, "episodes": []int{1},
				"absolute_number": nil, "confidence": "high", "reason": "x",
			}}})
			files, err := q.parse(message(t, "end_turn", string(text), ""))
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if len(files) == 1 {
				got = files[0].Path
			}
			if got != tt.want {
				t.Errorf("mapped to %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNaturalCompare(t *testing.T) {
	paths := []string{"Show - 10.mkv", "Show - 9.mkv", "Show - 009.mkv", "Show - 100.mkv", "Show - 1.mkv"}
	slices.SortFunc(paths, naturalCompare)
	want := []string{"Show - 1.mkv", "Show - 009.mkv", "Show - 9.mkv", "Show - 10.mkv", "Show - 100.mkv"}
	if !slices.Equal(paths, want) {
		t.Errorf("sorted = %q, want %q", paths, want)
	}
}
