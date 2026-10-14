package subtitle

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
)

// loadFixture reads an OpenSubtitles /subtitles response from testdata.
func loadFixture(t *testing.T, name string) *opensubtitles.SearchResponse {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var resp opensubtitles.SearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &resp
}

func fileIDs(cands []Candidate) []int {
	ids := make([]int, len(cands))
	for i, c := range cands {
		ids[i] = c.FileID
	}
	return ids
}

func byFileID(t *testing.T, cands []Candidate, id int) Candidate {
	t.Helper()
	for _, c := range cands {
		if c.FileID == id {
			return c
		}
	}
	t.Fatalf("file %d not in %v", id, fileIDs(cands))
	return Candidate{}
}

// PLAN B7 acceptance: for a YIFY release the YIFY subtitle ranks first,
// above a 720p WEB-DL with far more downloads.
func TestRank_YIFYReleaseRanksFirst(t *testing.T) {
	cands := candidatesFrom(loadFixture(t, "opensubtitles_interstellar.json"))
	if len(cands) != 8 {
		t.Fatalf("candidates = %d, want 8 (the one without files is skipped)", len(cands))
	}

	// A YTS magnet's display name.
	ranked := Rank("Interstellar (2014) [1080p] [BluRay] [5.1] [YTS.MX]", cands)
	for _, c := range ranked {
		t.Logf("%3d %-16s %7d  %-50s %s", c.Score, c.Match, c.Downloads, c.ReleaseName, c.Reason)
	}

	yify := ranked[0]
	if yify.FileID != 1002 {
		t.Fatalf("first = %d (%q), want 1002 (YIFY); order %v", yify.FileID, yify.ReleaseName, fileIDs(ranked))
	}
	if yify.Match != MatchExactRelease || yify.Score < 90 {
		t.Errorf("YIFY = %s %d, want exact_release ≥ 90", yify.Match, yify.Score)
	}
	if yify.Reason != "Same group (YTS) and source (BluRay)" {
		t.Errorf("YIFY reason = %q", yify.Reason)
	}

	webdl := byFileID(t, ranked, 1001)
	if webdl.Downloads <= yify.Downloads {
		t.Fatal("fixture: the WEB-DL should have more downloads")
	}
	if webdl.Score >= yify.Score {
		t.Errorf("720p WEB-DL score %d ≥ YIFY %d", webdl.Score, yify.Score)
	}

	want := []int{
		1002, // YIFY
		1008, // YIFY, hearing impaired, fewer downloads
		1005, // YTS, AI translated
		1004, // Same source, another group
		1003, // IMAX cut
		1001, // 720p WEB-DL
		1007, // HDCAM
		1006, // No release name
	}
	if got := fileIDs(ranked); !equalInts(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}

	wantMatch := map[int]Match{
		1002: MatchExactRelease, 1008: MatchExactRelease,
		1005: MatchSameSource, 1004: MatchSameSource,
		1003: MatchDifferentTiming, 1001: MatchDifferentTiming, 1007: MatchDifferentTiming,
		1006: MatchUnknown,
	}
	for id, m := range wantMatch {
		if c := byFileID(t, ranked, id); c.Match != m {
			t.Errorf("file %d (%q) match = %s (score %d, %q), want %s", id, c.ReleaseName, c.Match, c.Score, c.Reason, m)
		}
	}

	if c := byFileID(t, ranked, 1005); !c.MachineTranslated || !strings.HasSuffix(c.Reason, "; machine translated") {
		t.Errorf("AI-translated candidate = %+v, want flagged in the reason", c)
	}
	if c := byFileID(t, ranked, 1003); c.Reason != "Different cut (IMAX vs theatrical): the timing may drift" {
		t.Errorf("IMAX reason = %q", c.Reason)
	}
	if c := byFileID(t, ranked, 1006); c.Score != 0 || c.Reason != "The subtitle doesn't name its release" {
		t.Errorf("no release name = %d %q", c.Score, c.Reason)
	}
}

func TestRank_SeasonPackAgainstEpisodeReleases(t *testing.T) {
	cands := candidatesFrom(loadFixture(t, "opensubtitles_severance_s02e05.json"))
	ranked := Rank("Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", cands)

	if got, want := fileIDs(ranked), []int{2003, 2002, 2004, 2001}; !equalInts(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	wantMatch := []Match{MatchExactRelease, MatchSameSource, MatchDifferentTiming, MatchDifferentTiming}
	for i, c := range ranked {
		if c.Match != wantMatch[i] {
			t.Errorf("%q = %s (%d, %q), want %s", c.ReleaseName, c.Match, c.Score, c.Reason, wantMatch[i])
		}
	}
	if got := ranked[1].Reason; got != "Same source (WEB), different group (NTb)" {
		t.Errorf("NTb reason = %q", got)
	}
	if got := ranked[3].Reason; got != "Different source (HDTV vs WEB): the timing may drift" {
		t.Errorf("HDTV reason = %q", got)
	}
}

// With nothing assigned every candidate is unknown, most downloaded first.
func TestRank_NothingAssigned(t *testing.T) {
	ranked := Rank("", candidatesFrom(loadFixture(t, "opensubtitles_interstellar.json")))
	if ranked[0].FileID != 1001 {
		t.Errorf("first = %d, want the most downloaded (1001)", ranked[0].FileID)
	}
	for _, c := range ranked {
		if c.Match != MatchUnknown || c.Score != 0 || c.Reason != "Nothing assigned to compare with" {
			t.Errorf("%d = %s %d %q, want unknown 0", c.FileID, c.Match, c.Score, c.Reason)
		}
	}
	// AI translation still sinks a candidate among equals.
	if last := ranked[len(ranked)-1]; last.FileID != 1005 {
		t.Errorf("last = %d, want the AI-translated one (1005)", last.FileID)
	}
}

func TestRank_Buckets(t *testing.T) {
	const flux = "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX"
	tests := []struct {
		name       string
		assigned   string
		candidate  string
		mt         bool
		want       Match
		wantScore  int // -1: don't check
		wantReason string
	}{
		{"exact name", flux, flux, false, MatchExactRelease, 100, "Made for this exact release"},
		{"exact apart from punctuation and extension", flux, strings.ReplaceAll(flux, ".", " ") + ".srt", false, MatchExactRelease, 100, "Made for this exact release"},
		{"same group and source, other resolution", flux, "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-FLUX", false, MatchExactRelease, -1, "Same group (FLUX) and source (WEB)"},
		{"WEB-DL vs WEBRip, same group", flux, "Dune.Part.Two.2024.2160p.WEBRip.x265-FLUX", false, MatchExactRelease, -1, "Same group (FLUX) and source (WEB)"},
		{"same source, other group", flux, "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", false, MatchSameSource, -1, "Same source (WEB), different group (NTb)"},
		{"other source", flux, "Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS", false, MatchDifferentTiming, -1, "Different source (BluRay vs WEB): the timing may drift"},
		{"other cut, same group and source", "Aliens.1986.1080p.BluRay.x264-AMIABLE", "Aliens.1986.Extended.1080p.BluRay.x264-AMIABLE", false, MatchDifferentTiming, -1, "Different cut (Extended vs theatrical): the timing may drift"},
		{"fansub group, no source", "[SubsPlease] Sousou no Frieren - 28 (1080p) [A1B2C3D4]", "[SubsPlease] Sousou no Frieren - 28 (1080p)", false, MatchSameSource, -1, "Same group (SubsPlease)"},
		{"nothing in common", flux, "Dune 2", false, MatchDifferentTiming, -1, "No shared group or source: the timing may drift"},
		{"exact but machine translated", flux, flux, true, MatchSameSource, 80, "Made for this exact release; machine translated"},
		{"nothing assigned", "", flux, false, MatchUnknown, 0, "Nothing assigned to compare with"},
		{"no release name", flux, "", false, MatchUnknown, 0, "The subtitle doesn't name its release"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Rank(tt.assigned, []Candidate{{FileID: 1, ReleaseName: tt.candidate, MachineTranslated: tt.mt}})[0]
			if got.Match != tt.want {
				t.Errorf("match = %s (score %d), want %s", got.Match, got.Score, tt.want)
			}
			if tt.wantScore >= 0 && got.Score != tt.wantScore {
				t.Errorf("score = %d, want %d", got.Score, tt.wantScore)
			}
			if got.Score < 0 || got.Score > 100 {
				t.Errorf("score %d out of 0–100", got.Score)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestMatchStored(t *testing.T) {
	const assigned = "Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX"
	tests := []struct {
		name     string
		assigned string
		sub      Subtitle
		want     Match
	}{
		{"torrent subtitles are the release's", assigned, Subtitle{Source: SourceTorrent}, MatchExactRelease},
		{"downloaded for the same group", assigned, Subtitle{Source: SourceOpenSubtitles, ReleaseName: "Severance.S02E05.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX"}, MatchExactRelease},
		{"downloaded for another source", assigned, Subtitle{Source: SourceOpenSubtitles, ReleaseName: "Severance.S02E05.720p.HDTV.x264-SYNCOPY"}, MatchDifferentTiming},
		{"downloaded without a release name", assigned, Subtitle{Source: SourceOpenSubtitles}, MatchUnknown},
		{"nothing assigned", "", Subtitle{Source: SourceOpenSubtitles, ReleaseName: assigned}, MatchUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchStored(tt.assigned, &tt.sub); got != tt.want {
				t.Errorf("MatchStored = %s, want %s", got, tt.want)
			}
		})
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
