package identify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClassOf(t *testing.T) {
	tests := map[string]FileClass{
		"Show.S01E01.mkv": ClassVideo, "a/b/Show.MP4": ClassVideo, "Show.S01E01.en.srt": ClassSubtitle,
		"x.ass": ClassSubtitle, "Fonts/Arial.TTF": ClassFont, "Cover.jpg": ClassImage, "RARBG.txt": ClassInfo,
		"x.nfo": ClassInfo, "x.rar": ClassArchive, "x.r01": ClassArchive, "README": ClassOther,
		"Mr.Robot.S01E03.eps1.2_d3bug.mkv.1080p.mkv": ClassVideo, `C:\a\b.mkv`: ClassVideo,
	}
	for path, want := range tests {
		if got := ClassOf(path); got != want {
			t.Errorf("ClassOf(%q) = %s, want %s", path, got, want)
		}
	}
}

func TestSplitPath(t *testing.T) {
	dirs, name, ext := splitPath("Torrent/Season 1/02 - Cat's in the Bag....mkv")
	if strings.Join(dirs, "|") != "Torrent|Season 1" || name != "02 - Cat's in the Bag..." || ext != ".mkv" {
		t.Errorf("splitPath = %q %q %q", dirs, name, ext)
	}
	if _, name, ext := splitPath("Show.S01E01.v2"); name != "Show.S01E01.v2" || ext != "" {
		t.Errorf("unknown extension: %q %q", name, ext)
	}
}

func TestTextExtra(t *testing.T) {
	tests := []struct {
		words  string
		strict bool
		want   ExtraType
	}{
		{"ncop01", false, ExtraOpening},
		{"nced", true, ExtraEnding},
		{"creditless opening", false, ExtraOpening},
		{"non credit ending 2", false, ExtraEnding},
		{"official trailer", false, ExtraTrailer},
		{"behind the scenes", false, ExtraFeaturette},
		{"deleted scenes", false, ExtraFeaturette},
		{"making of", false, ExtraFeaturette},
		{"sample", true, ExtraSample},
		{"trailer", true, ""},
		{"making friends", false, ""},
		{"the opening", false, ExtraOpening},
		{"christmas special", false, ""},
	}
	for _, tt := range tests {
		if got, _ := textExtra(strings.Fields(tt.words), tt.strict); got != tt.want {
			t.Errorf("textExtra(%q, strict=%v) = %q, want %q", tt.words, tt.strict, got, tt.want)
		}
	}
}

func TestParseQuality(t *testing.T) {
	tests := map[string]Quality{
		"The.Boys.S04E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb.mkv":           {"1080p", "WEB-DL"},
		"Breaking.Bad.S01E01.720p.BluRay.x264-DEMAND.mkv":                  {"720p", "BluRay"},
		"[SubsPlease] Frieren - 05 (1080p) [ABCD1234].mkv":                 {"1080p", ""},
		"Show.2160p.WEBRip/show.s01e01.mkv":                                {"2160p", "WEBRip"},
		"[Group] Spy x Family - 12 (BD 1920x1080 x264 FLAC)[A1B2C3D4].mkv": {"1080p", "BluRay"},
		"Show.S01E01.HDTV.mkv":                                             {"", "HDTV"},
	}
	for path, want := range tests {
		if got := ParseQuality(path); got != want {
			t.Errorf("ParseQuality(%q) = %+v, want %+v", path, got, want)
		}
	}
}

func TestClipReason(t *testing.T) {
	short := "S01E05 in the name"
	if clipReason(short) != short {
		t.Error("a short reason changed")
	}
	long := strings.Repeat("ё", 200) // two bytes per rune
	got := clipReason(long)
	if n := utf8.RuneCountInString(got); n != MaxReasonRunes || !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Errorf("clipped to %d runes: %q", n, got)
	}
}

func TestNameSimilarity(t *testing.T) {
	tests := []struct {
		text, name string
		min, max   float64
	}{
		{"Time Crash", "Time Crash", 1, 1},
		{"The Next Doctor Christmas Special", "The Next Doctor", 0.92, 0.92},
		{"Ilse's Notebook", "Ilse's Notebook: Memoirs of a Scout Regiment Member", 0.9, 0.9},
		{"Lost in the Cruel World", "Lost Girls: Lost in the Cruel World", 0.9, 0.9},
		{"No Regrets Part 1", "No Regrets: Part 1", 1, 1},
		{"The End of Time Part 1", "The End of Time (2)", 0, 0.85},
		{"Pilot Commentary", "Pilot", 0, 0.85},
		{"The One After the Superbowl", "The One After the Super Bowl (1)", 0, 0.85},
	}
	for _, tt := range tests {
		got := nameSimilarity(nameWords(tt.text), nameWords(tt.name))
		if got < tt.min || got > tt.max {
			t.Errorf("nameSimilarity(%q, %q) = %.2f, want %.2f..%.2f", tt.text, tt.name, got, tt.min, tt.max)
		}
	}
}

func TestDecide(t *testing.T) {
	a := []*Episode{{ID: 1, Season: 1, Number: 1}}
	b := []*Episode{{ID: 2, Season: 1, Number: 2}}
	tests := []struct {
		name string
		opts []option
		want Confidence
		best int64
	}{
		{"one strong reading", []option{{eps: a, weight: wExplicit, method: MethodSxxEyy}}, ConfidenceHigh, 1},
		{"agreement lifts a medium reading", []option{
			{eps: a, weight: wFolderSeason, method: MethodFolderSeason}, {eps: a, weight: wAbsolute, method: MethodAbsolute},
		}, ConfidenceHigh, 1},
		{"a weak reading doesn't agree", []option{
			{eps: a, weight: wFolderSeason, method: MethodFolderSeason}, {eps: a, weight: wGuess, method: MethodAbsolute},
		}, ConfidenceMedium, 1},
		{"a strong contradiction drops both", []option{
			{eps: a, weight: wTitle, method: MethodTitle}, {eps: b, weight: wSpecialNumber, method: MethodSpecialNumber},
		}, ConfidenceMedium, 1},
		{"a weak contradiction keeps high", []option{
			{eps: a, weight: wTitle, method: MethodTitle}, {eps: b, weight: wRemapOverflow, method: MethodRemapOverflow},
		}, ConfidenceHigh, 1},
		{"a close second blocks high", []option{
			{eps: a, weight: wAbsolute, method: MethodAbsolute}, {eps: b, weight: contraMin - 0.02, method: MethodFolderSeason},
		}, ConfidenceMedium, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := decide(tt.opts)
			if got := confidenceOf(d.score, d.margin); got != tt.want || d.best.eps[0].ID != tt.best {
				t.Errorf("got %s on %d (score %.2f margin %.2f), want %s on %d", got, d.best.eps[0].ID, d.score, d.margin, tt.want, tt.best)
			}
		})
	}
}
