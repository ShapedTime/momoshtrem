package identify

import (
	"reflect"
	"testing"
)

func TestConflicts(t *testing.T) {
	ep := func(ids ...int64) []Target {
		out := make([]Target, len(ids))
		for i, id := range ids {
			out[i] = Target{EpisodeID: id}
		}
		return out
	}
	tests := []struct {
		name  string
		props []Proposal
		want  map[int][]int
	}{
		{"distinct episodes", []Proposal{
			{Kind: KindEpisode, Targets: ep(1)}, {Kind: KindEpisode, Targets: ep(2)},
		}, map[int][]int{}},
		{"two files on one episode", []Proposal{
			{Kind: KindEpisode, Targets: ep(1)}, {Kind: KindEpisode, Targets: ep(1)},
		}, map[int][]int{0: {1}, 1: {0}}},
		{"a multi-episode file overlapping another", []Proposal{
			{Kind: KindEpisode, Targets: ep(1, 2)}, {Kind: KindEpisode, Targets: ep(2)}, {Kind: KindEpisode, Targets: ep(3)},
		}, map[int][]int{0: {1}, 1: {0}}},
		{"a multi-episode file alone", []Proposal{
			{Kind: KindEpisode, Targets: ep(1, 2)},
		}, map[int][]int{}},
		{"subtitles and extras never conflict", []Proposal{
			{Kind: KindEpisode, Targets: ep(1)}, {Kind: KindSubtitle, Targets: ep(1)}, {Kind: KindExtra},
		}, map[int][]int{}},
		{"specials conflict too", []Proposal{
			{Kind: KindSpecial, Targets: ep(9)}, {Kind: KindSpecial, Targets: ep(9)}, {Kind: KindSpecial, Targets: ep(9)},
		}, map[int][]int{0: {1, 2}, 1: {0, 2}, 2: {0, 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Conflicts(tt.props); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Conflicts = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDedupe(t *testing.T) {
	type file struct {
		path string
		size int64
	}
	type want struct {
		kind    Kind
		extra   ExtraType
		targets string
		maxConf Confidence
	}
	tests := []struct {
		name  string
		show  string
		files []file
		want  []want
	}{
		{"v2 supersedes v1", "frieren", []file{
			{"[Group] Sousou no Frieren - 14 [1080p].mkv", 1 << 30},
			{"[Group] Sousou no Frieren - 14v2 [1080p].mkv", 1 << 30},
		}, []want{
			{KindExtra, ExtraSuperseded, "null", ConfidenceNone},
			{KindEpisode, "", "S01E14", ConfidenceHigh},
		}},
		{"v3 supersedes v2 and v1", "frieren", []file{
			{"[Group] Sousou no Frieren - 03v2 [1080p].mkv", 1 << 30},
			{"[Group] Sousou no Frieren - 03v3 [1080p].mkv", 1 << 30},
			{"[Group] Sousou no Frieren - 03 [1080p].mkv", 1 << 30},
		}, []want{
			{KindExtra, ExtraSuperseded, "null", ConfidenceNone},
			{KindEpisode, "", "S01E03", ConfidenceHigh},
			{KindExtra, ExtraSuperseded, "null", ConfidenceNone},
		}},
		{"a tiny copy is a sample", "the-boys", []file{
			{"The.Boys.S01E01.1080p.WEB.mkv", 3 << 30},
			{"the.boys.s01e01.1080p.mkv", 20 << 20},
		}, []want{
			{KindEpisode, "", "S01E01", ConfidenceHigh},
			{KindSample, ExtraSample, "null", ConfidenceNone},
		}},
		{"the better quality is kept", "the-boys", []file{
			{"720p/The.Boys.S01E02.720p.WEB.mkv", 1 << 30},
			{"1080p/The.Boys.S01E02.1080p.WEB.mkv", 3 << 30},
		}, []want{
			{KindExtra, ExtraSuperseded, "null", ConfidenceNone},
			{KindEpisode, "", "S01E02", ConfidenceHigh},
		}},
		{"a REPACK beats a larger file", "the-boys", []file{
			{"The.Boys.S03E01.Payback.1080p.WEB.mkv", 4 << 30},
			{"The.Boys.S03E01.Payback.REPACK.1080p.WEB.mkv", 3 << 30},
		}, []want{
			{KindExtra, ExtraSuperseded, "null", ConfidenceNone},
			{KindEpisode, "", "S03E01", ConfidenceHigh},
		}},
		{"a bracketed version", "frieren", []file{
			{"[Group] Sousou no Frieren - 14 (v2) [1080p].mkv", 1 << 30},
			{"[Group] Sousou no Frieren - 14 [1080p].mkv", 1 << 30},
		}, []want{
			{KindEpisode, "", "S01E14", ConfidenceHigh},
			{KindExtra, ExtraSuperseded, "null", ConfidenceNone},
		}},
		{"different files on one episode conflict", "the-boys", []file{
			{"The.Boys.S01E02.Cherry.1080p.WEB.mkv", 3 << 30},
			{"The.Boys.S01E02.Get.Some.1080p.WEB.mkv", 3 << 30},
		}, []want{
			{KindEpisode, "", "S01E02", ConfidenceMedium},
			{KindEpisode, "", "S01E02", ConfidenceMedium},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files []TorrentFile
			for _, f := range tt.files {
				files = append(files, TorrentFile{Path: "Pack/" + f.path, Size: f.size})
			}
			res := Classify(Input{TorrentName: "Pack", Files: files, Show: show(t, tt.show), Now: corpusNow})
			for i, w := range tt.want {
				p := res.Proposals[i]
				if p.Kind != w.kind || p.ExtraType != w.extra || targetLabel(p.Targets) != w.targets || p.Confidence != w.maxConf {
					t.Errorf("%s: got %s/%s %s [%s] %q, want %s/%s %s [%s]", tt.files[i].path,
						p.Kind, p.ExtraType, targetLabel(p.Targets), p.Confidence, p.Reason, w.kind, w.extra, w.targets, w.maxConf)
				}
			}
			if tt.name == "v2 supersedes v1" && res.Proposals[0].SupersededBy != files[1].Path {
				t.Errorf("SupersededBy = %q", res.Proposals[0].SupersededBy)
			}
		})
	}
}
