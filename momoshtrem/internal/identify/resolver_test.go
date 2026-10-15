package identify

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func intp(n int) *int { return &n }

func TestNoopResolver(t *testing.T) {
	var r Resolver = NoopResolver{}
	out, err := r.Resolve(context.Background(), ResolveInput{TorrentName: "x"})
	if err != nil || len(out.Files) != 0 || out.Usage != (Usage{}) {
		t.Errorf("NoopResolver = %+v, %v", out, err)
	}
}

func TestMergeAI(t *testing.T) {
	cat := show(t, "frieren")
	// The rules' view of a small Frieren torrent.
	rules := func() []Proposal {
		return []Proposal{
			{Path: "a.mkv", Kind: KindEpisode, Targets: []Target{{fixtureID(1, 1), 1, 1}}, Confidence: ConfidenceHigh, Method: MethodSxxEyy, Source: SourceRules, Reason: "rules"},
			{Path: "b.mkv", Kind: KindEpisode, Targets: []Target{{fixtureID(1, 5), 1, 5}}, Confidence: ConfidenceMedium, Method: MethodFolderSeason, Source: SourceRules, Reason: "rules"},
			{Path: "c.mkv", Kind: KindUnknown, Source: SourceRules, Reason: "rules"},
			{Path: "c.en.srt", Kind: KindSubtitle, LinkedVideo: "c.mkv", Source: SourceRules, Reason: "rules"},
			{Path: "d.mkv", Kind: KindSpecial, Source: SourceRules, Reason: "rules"},
		}
	}

	tests := []struct {
		name   string
		ai     []AIFile
		path   string
		want   string // target label after the merge
		source Source
		conf   Confidence
	}{
		{"fills an unmatched file", []AIFile{{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{29}, Confidence: ConfidenceHigh, Reason: "S2E01 is S01E29"}},
			"c.mkv", "S01E29", SourceAI, ConfidenceHigh},
		{"the subtitle follows its video", []AIFile{{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{29}, Confidence: ConfidenceMedium}},
			"c.en.srt", "S01E29", SourceAI, ConfidenceMedium},
		{"absolute numbers work too", []AIFile{{Path: "c.mkv", Kind: KindEpisode, AbsoluteNumber: intp(30), Confidence: ConfidenceLow}},
			"c.mkv", "S01E30", SourceAI, ConfidenceLow},
		{"replaces a medium rules answer", []AIFile{{Path: "b.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{6}, Confidence: ConfidenceMedium}},
			"b.mkv", "S01E06", SourceAI, ConfidenceMedium},
		{"specials map to season 0", []AIFile{{Path: "d.mkv", Kind: KindSpecial, Season: intp(0), Episodes: []int{3}, Confidence: ConfidenceMedium}},
			"d.mkv", "S00E03", SourceAI, ConfidenceMedium},
		{"never overrides a high rules answer", []AIFile{{Path: "a.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{2}, Confidence: ConfidenceHigh}},
			"a.mkv", "S01E01", SourceRules, ConfidenceHigh},
		{"rejects an episode that doesn't exist", []AIFile{{Path: "c.mkv", Kind: KindEpisode, Season: intp(2), Episodes: []int{1}, Confidence: ConfidenceHigh}},
			"c.mkv", "null", SourceRules, ConfidenceNone},
		{"rejects an absolute number that doesn't exist", []AIFile{{Path: "c.mkv", Kind: KindEpisode, AbsoluteNumber: intp(99), Confidence: ConfidenceHigh}},
			"c.mkv", "null", SourceRules, ConfidenceNone},
		{"rejects a multi-episode answer with a missing episode", []AIFile{{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{38, 39}, Confidence: ConfidenceHigh}},
			"c.mkv", "null", SourceRules, ConfidenceNone},
		{"rejects an episode a high rules answer has", []AIFile{{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{1}, Confidence: ConfidenceHigh}},
			"c.mkv", "null", SourceRules, ConfidenceNone},
		{"ignores unknown paths", []AIFile{{Path: "zzz.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{2}, Confidence: ConfidenceHigh}},
			"c.mkv", "null", SourceRules, ConfidenceNone},
		{"ignores answers for subtitles", []AIFile{{Path: "c.en.srt", Kind: KindEpisode, Season: intp(1), Episodes: []int{2}, Confidence: ConfidenceHigh}},
			"c.en.srt", "null", SourceRules, ConfidenceNone},
		{"an unknown answer keeps the rules'", []AIFile{{Path: "b.mkv", Kind: KindUnknown, Confidence: ConfidenceLow}},
			"b.mkv", "S01E05", SourceRules, ConfidenceMedium},
		{"an invalid confidence becomes low", []AIFile{{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{2}, Confidence: "certain"}},
			"c.mkv", "S01E02", SourceAI, ConfidenceLow},
		{"the first answer for a path wins", []AIFile{
			{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{2}, Confidence: ConfidenceMedium},
			{Path: "c.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{3}, Confidence: ConfidenceHigh},
		}, "c.mkv", "S01E02", SourceAI, ConfidenceMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := rules()
			out := MergeAI(in, tt.ai, cat)
			var got *Proposal
			for i := range out {
				if out[i].Path == tt.path {
					got = &out[i]
				}
			}
			if got == nil {
				t.Fatalf("no proposal for %s", tt.path)
			}
			if targetLabel(got.Targets) != tt.want || got.Source != tt.source || got.Confidence != tt.conf {
				t.Errorf("%s: got %s %s [%s], want %s %s [%s]", tt.path, targetLabel(got.Targets), got.Source, got.Confidence, tt.want, tt.source, tt.conf)
			}
			if !reflectEqualProposals(in, rules()) {
				t.Error("MergeAI modified its input")
			}
		})
	}
}

func TestMergeAIExtrasAndReasons(t *testing.T) {
	cat := show(t, "frieren")
	rules := []Proposal{
		{Path: "x.mkv", Kind: KindUnknown, Source: SourceRules},
		{Path: "x.srt", Kind: KindSubtitle, LinkedVideo: "x.mkv", Targets: nil, Source: SourceRules},
		{Path: "y.ttf", Kind: KindExtra, ExtraType: ExtraFont, Source: SourceRules},
	}
	out := MergeAI(rules, []AIFile{
		{Path: "x.mkv", Kind: KindExtra, Confidence: ConfidenceMedium, Reason: strings.Repeat("long ", 60)},
		{Path: "y.ttf", Kind: KindSample},
	}, cat)
	if out[0].Kind != KindExtra || out[0].ExtraType != ExtraOther || out[0].Source != SourceAI {
		t.Errorf("x.mkv = %+v", out[0])
	}
	if n := utf8.RuneCountInString(out[0].Reason); n > MaxReasonRunes {
		t.Errorf("reason has %d runes", n)
	}

	if out[1].HasTarget() || !strings.HasPrefix(out[1].Reason, "Subtitle of an extra") {
		t.Errorf("x.srt = %+v", out[1])
	}
	if out[2].Kind != KindExtra || out[2].Source != SourceRules {
		t.Errorf("a font can't become a sample: %+v", out[2])
	}
	out = MergeAI(rules, []AIFile{{Path: "x.mkv", Kind: KindEpisode, Season: intp(1), Episodes: []int{2},
		Confidence: ConfidenceMedium, Reason: "AI: release S2 numbering"}}, cat)
	if out[0].Reason != "release S2 numbering" {
		t.Errorf("reason = %q, want no AI: prefix", out[0].Reason)
	}
}

func reflectEqualProposals(a, b []Proposal) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Kind != b[i].Kind || a[i].Confidence != b[i].Confidence ||
			a[i].Source != b[i].Source || targetLabel(a[i].Targets) != targetLabel(b[i].Targets) {
			return false
		}
	}
	return true
}
