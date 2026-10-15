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
		{"every video", Videos(in.Rules),
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

func TestEpisodeWindow(t *testing.T) {
	in := identify.ResolveInput{TorrentName: "[Erai-raws] One Piece - 1071 [1080p][ABCD1234].mkv", Show: longShow()}
	in.Rules = []identify.Proposal{{Path: in.TorrentName, Size: 1 << 30, Kind: identify.KindUnknown}}
	in.Files = []identify.TorrentFile{{Path: in.TorrentName, Size: 1 << 30}}
	q := buildRequests(in, effectiveScope(in))[0]

	if !strings.Contains(q.context, `<episodes listed="24 of 1103"`) {
		t.Errorf("window header: %s", q.context[strings.Index(q.context, "<episodes"):][:80])
	}
	for _, want := range []string{"S00E01", "S00E03", "S11E61 #1061", "S11E71 #1071", "S11E81 #1081"} {
		if !strings.Contains(q.context, want) {
			t.Errorf("window lacks %s", want)
		}
	}
	for _, unwanted := range []string{"#1060 ", "#1082 ", "#500 ", "#1080p"} {
		if strings.Contains(q.context, unwanted) {
			t.Errorf("window has %s", unwanted)
		}
	}
	if q.episodes != 24 {
		t.Errorf("episodes = %d, want 24", q.episodes)
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
	byTitle := "Show - The Payback Job.mkv"
	in := identify.ResolveInput{TorrentName: "x", Show: cat}
	for _, p := range []string{file, byTitle} {
		in.Rules = append(in.Rules, identify.Proposal{Path: p, Size: 1 << 30, Kind: identify.KindUnknown})
		in.Files = append(in.Files, identify.TorrentFile{Path: p, Size: 1 << 30})
	}
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
	in.Scope = Videos(in.Rules)
	reqs := buildRequests(in, effectiveScope(in))
	if len(reqs) != 3 {
		t.Fatalf("%d requests for 130 files, want 3", len(reqs))
	}
	total := 0
	for i, q := range reqs {
		total += len(q.paths)
		if len(q.paths) > filesPerCall || !q.shared || q.context != reqs[0].context {
			t.Errorf("request %d: %d files, shared %v", i, len(q.paths), q.shared)
		}
		if q.maxTokens() > 16000 {
			t.Errorf("request %d: max_tokens %d over the non-streaming limit", i, q.maxTokens())
		}
		p := q.params(DefaultModel)
		if p.Messages[0].Content[0].OfText.CacheControl.Type != "ephemeral" {
			t.Errorf("request %d: shared context isn't cached", i)
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
}

func TestUnsureFiles(t *testing.T) {
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
	if got := UnsureFiles(rules); !slices.Equal(got, want) {
		t.Errorf("UnsureFiles = %q, want %q", got, want)
	}
	wantVideos := []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv", "e.mkv", "f.mkv", "g.mkv", "ncop.mkv", "sample.mkv"}
	if got := Videos(rules); !slices.Equal(got, wantVideos) {
		t.Errorf("Videos = %q, want %q", got, wantVideos)
	}
	if got := UnsureFiles(rules[:1]); len(got) != 0 {
		t.Errorf("UnsureFiles of one high file = %q", got)
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
