package identify

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// classifyOne classifies a single-file torrent for a fixture show.
func classifyOne(t *testing.T, showName, path string) Proposal {
	t.Helper()
	res := Classify(Input{TorrentName: path, Files: []TorrentFile{{Path: path, Size: 1 << 30}}, Show: show(t, showName), Now: corpusNow})
	if len(res.Proposals) != 1 {
		t.Fatalf("%d proposals, want 1", len(res.Proposals))
	}
	return res.Proposals[0]
}

func assertTarget(t *testing.T, p Proposal, kind Kind, want string, conf Confidence) {
	t.Helper()
	if p.Kind != kind || targetLabel(p.Targets) != want || p.Confidence != conf {
		t.Errorf("%s: got %s %s [%s] %q, want %s %s [%s]",
			p.Path, p.Kind, targetLabel(p.Targets), p.Confidence, p.Reason, kind, want, conf)
	}
}

// PLAN §3: every special was rejected (special_not_supported). Specials now
// map to season 0.
func TestPlanFailure_Special(t *testing.T) {
	p := classifyOne(t, "doctor-who", "Doctor.Who.2005.S00E04.Time.Crash.720p.HDTV.x264-GRP.mkv")
	assertTarget(t, p, KindSpecial, "S00E04", ConfidenceHigh)

	// A scene number TMDB doesn't use, found by the episode's name.
	p = classifyOne(t, "doctor-who", "Doctor.Who.2005.S04E00.Voyage.of.the.Damned.720p.BluRay.x264-GRP.mkv")
	assertTarget(t, p, KindSpecial, "S00E05", ConfidenceHigh)
}

// PLAN §3: "One Piece - 1071" was S10E71; it's absolute episode 1071.
func TestPlanFailure_OnePiece1071(t *testing.T) {
	p := classifyOne(t, "one-piece", "One Piece - 1071.mkv")
	assertTarget(t, p, KindEpisode, "S21E79", ConfidenceHigh)
	if !strings.Contains(p.Reason, "1071") {
		t.Errorf("reason %q doesn't quote the number", p.Reason)
	}
}

// PLAN §3: "[SubsPlease] Frieren - 05 (1080p)" wasn't identified. It is
// now; "Frieren" alone is only the start of the show's title ("Frieren:
// Beyond Journey's End"), so it's not sure enough to apply on its own. The
// title or an alias is.
func TestPlanFailure_SubsPleaseFrieren05(t *testing.T) {
	p := classifyOne(t, "frieren", "[SubsPlease] Frieren - 05 (1080p).mkv")
	assertTarget(t, p, KindEpisode, "S01E05", ConfidenceMedium)

	p = classifyOne(t, "frieren", "[SubsPlease] Sousou no Frieren - 05 (1080p).mkv")
	assertTarget(t, p, KindEpisode, "S01E05", ConfidenceHigh)
}

// PLAN §3: "1920x1080" was S20E108.
func TestPlanFailure_Resolution1920x1080(t *testing.T) {
	p := classifyOne(t, "the-boys", "The.Boys.S02E05.1920x1080.WEB.x264-GRP.mkv")
	assertTarget(t, p, KindEpisode, "S02E05", ConfidenceHigh)

	p = classifyOne(t, "frieren", "[Group] Sousou no Frieren - 07 [1920x1080].mkv")
	assertTarget(t, p, KindEpisode, "S01E07", ConfidenceHigh)

	p = classifyOne(t, "frieren", "Frieren.1920x1080.mkv")
	assertTarget(t, p, KindUnknown, "null", ConfidenceNone)
}

// PLAN §3: a year (".2019.") was S20E19.
func TestPlanFailure_Year2019(t *testing.T) {
	p := classifyOne(t, "the-boys", "The.Boys.2019.S01E01.1080p.WEB.H264-GRP.mkv")
	assertTarget(t, p, KindEpisode, "S01E01", ConfidenceHigh)

	p = classifyOne(t, "the-boys", "The.Boys.2019.1080p.WEB.H264-GRP.mkv")
	assertTarget(t, p, KindUnknown, "null", ConfidenceNone)
}

// PLAN §3: "Law.and.Order.Special.Victims.Unit.S01E01" was a special.
func TestPlanFailure_LawAndOrderSVU(t *testing.T) {
	p := classifyOne(t, "law-and-order-svu", "Law.and.Order.Special.Victims.Unit.S01E01.mkv")
	assertTarget(t, p, KindEpisode, "S01E01", ConfidenceHigh)
}

// PLAN §3: "Trailer.Park.Boys" files were skipped as trailers.
func TestPlanFailure_TrailerParkBoys(t *testing.T) {
	p := classifyOne(t, "trailer-park-boys", "Trailer.Park.Boys.S01E01.mkv")
	assertTarget(t, p, KindEpisode, "S01E01", ConfidenceHigh)

	// The show's actual trailer is still a trailer.
	p = classifyOne(t, "trailer-park-boys", "Trailer.Park.Boys.Season.12.Official.Trailer.1080p.mkv")
	if p.Kind != KindExtra || p.ExtraType != ExtraTrailer {
		t.Errorf("trailer: got %s/%s, want extra/trailer", p.Kind, p.ExtraType)
	}
}

// "Special" after the episode number is the episode's name, not a marker.
func TestSpecialWordInEpisodeTitle(t *testing.T) {
	p := classifyOne(t, "frieren", "[Group] Sousou no Frieren - 05 - The Special One [1080p].mkv")
	assertTarget(t, p, KindEpisode, "S01E05", ConfidenceHigh)

	p = classifyOne(t, "attack-on-titan", "[Group] Shingeki no Kyojin - OVA - 03 [BD].mkv")
	assertTarget(t, p, KindSpecial, "S00E03", ConfidenceMedium)
}

// High needs the name's title to be the show's: another show's torrent,
// a spin-off, a strict prefix of a longer title or a year more than one off
// can't be sure. The right show's usual names stay high.
func TestWrongShowGuard(t *testing.T) {
	tests := []struct {
		show, path string
		want       Confidence
	}{
		{"law-and-order-svu", "Law.and.Order.S20E05.1080p.WEB.mkv", ConfidenceMedium},
		{"9-1-1", "9-1-1.Lone.Star.S04E05.1080p.WEB.mkv", ConfidenceMedium},
		{"the-boys", "The.Boys.Presents.Diabolical.S01E05.1080p.WEB.mkv", ConfidenceMedium},
		{"the-boys", "Gen.V.S01E05.1080p.WEB.mkv", ConfidenceMedium},
		{"the-boys", "Lost.S01E05.1080p.BluRay.mkv", ConfidenceMedium},
		{"doctor-who", "Doctor.Who.2023.S01E01.Space.Babies.1080p.mkv", ConfidenceMedium},
		{"doctor-who", "Doctor.Who.1963.S01E01.An.Unearthly.Child.mkv", ConfidenceMedium},
		{"the-office", "The.Office.UK.S01E01.Downsize.mkv", ConfidenceMedium},
		{"doctor-who", "Doctor.Who.2005.S01E01.Rose.mkv", ConfidenceHigh},
		{"doctor-who", "Doctor.Who.2006.S02E01.mkv", ConfidenceHigh}, // one year off
		{"the-office", "The.Office.US.S01E01.Pilot.mkv", ConfidenceHigh},
		{"the-office", "The.Office.S01E01.Pilot.mkv", ConfidenceHigh},
		{"law-and-order-svu", "Law.and.Order.SVU.S20E05.1080p.WEB.mkv", ConfidenceHigh},
		{"1923", "1923.S01E01.1080p.mkv", ConfidenceHigh},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if p := classifyOne(t, tt.show, tt.path); p.Confidence != tt.want {
				t.Errorf("got %s %s [%s] %q, want [%s]", p.Kind, targetLabel(p.Targets), p.Confidence, p.Reason, tt.want)
			}
		})
	}

	// The country tag must be the show's even without the alias that has it.
	cat := *show(t, "the-office")
	cat.Aliases = nil
	for path, want := range map[string]Confidence{
		"The.Office.US.S01E01.Pilot.mkv": ConfidenceHigh,
		"The.Office.UK.S01E01.mkv":       ConfidenceMedium,
	} {
		res := Classify(Input{TorrentName: path, Files: []TorrentFile{{Path: path, Size: 1 << 30}}, Show: &cat, Now: corpusNow})
		if got := res.Proposals[0].Confidence; got != want {
			t.Errorf("%s without aliases: [%s], want [%s]", path, got, want)
		}
	}
}

// A season folder whose numbering restarts reads its numbers in that
// season; one past the season's end gets no episode and doesn't flip the
// others to absolute numbers.
func TestSeasonFolderOverflow(t *testing.T) {
	files := []TorrentFile{
		{Path: "[Group] Shingeki no Kyojin Season 2 [BD 1080p]/[Group] Shingeki no Kyojin - 01 [BD 1080p].mkv", Size: 1 << 30},
		{Path: "[Group] Shingeki no Kyojin Season 2 [BD 1080p]/[Group] Shingeki no Kyojin - 12 [BD 1080p].mkv", Size: 1 << 30},
		{Path: "[Group] Shingeki no Kyojin Season 2 [BD 1080p]/[Group] Shingeki no Kyojin - 13 [BD 1080p].mkv", Size: 1 << 30},
	}
	res := Classify(Input{TorrentName: "[Group] Shingeki no Kyojin Season 2 [BD 1080p]", Files: files, Show: show(t, "attack-on-titan"), Now: corpusNow})
	want := []string{"S02E01", "S02E12", "null"}
	for i, p := range res.Proposals {
		if got := targetLabel(p.Targets); got != want[i] {
			t.Errorf("%s → %s [%s] %q, want %s", files[i].Path, got, p.Confidence, p.Reason, want[i])
		}
	}
	if r := res.Proposals[2].Reason; !strings.Contains(r, "past S02") {
		t.Errorf("overflow reason = %q", r)
	}
}

// A later season named in brackets before the number ("(The Final
// Season) - 17") means the number restarts: the rules ask, with or without
// the show's aliases.
func TestBracketedSeasonTitle(t *testing.T) {
	for _, path := range []string{
		"[SubsPlease] Shingeki no Kyojin (The Final Season) - 17 (1080p) [B3C4D5E6].mkv",
		"[SubsPlease] Shingeki no Kyojin - The Final Season - 01 (1080p).mkv",
	} {
		for _, cat := range []*ShowCatalog{show(t, "attack-on-titan"), withoutAliases(show(t, "attack-on-titan"))} {
			res := Classify(Input{TorrentName: path, Files: []TorrentFile{{Path: path, Size: 1 << 30}}, Show: cat, Now: corpusNow})
			if p := res.Proposals[0]; p.HasTarget() {
				t.Errorf("%s (aliases %v) → %s [%s]", path, cat.Aliases, targetLabel(p.Targets), p.Confidence)
			}
		}
	}
	// Release words in brackets don't count: "(Uncensored)", "[Dual Audio]".
	p := classifyOne(t, "frieren", "[Group] Sousou no Frieren (Uncensored) - 05 [Dual Audio][1080p].mkv")
	assertTarget(t, p, KindEpisode, "S01E05", ConfidenceHigh)
}

func TestSpecialsOff(t *testing.T) {
	res := Classify(Input{
		TorrentName: "x",
		Files:       []TorrentFile{{Path: "Doctor.Who.2005.S00E04.Time.Crash.mkv", Size: 1 << 30}},
		Show:        withoutSpecials(show(t, "doctor-who")),
		Now:         corpusNow,
	})
	p := res.Proposals[0]
	if p.Kind != KindSpecial || p.HasTarget() || p.Reason != "Specials are off for this show" {
		t.Errorf("got %s %s %q", p.Kind, targetLabel(p.Targets), p.Reason)
	}
}

func TestHintEpisode(t *testing.T) {
	season, episode := 3, 49
	res := Classify(Input{
		TorrentName: "bluey.mp4",
		Files:       []TorrentFile{{Path: "bluey.mp4", Size: 400 << 20}},
		Show:        show(t, "bluey"),
		Hint:        &Hint{Season: &season, Episode: &episode},
		Now:         corpusNow,
	})
	p := res.Proposals[0]
	if targetLabel(p.Targets) != "S03E49" || p.Method != MethodHint || p.Confidence == ConfidenceHigh {
		t.Errorf("hinted file: got %s [%s] via %s", targetLabel(p.Targets), p.Confidence, p.Method)
	}
}

// Odd input must not panic, and every proposal keeps its file's path.
func TestClassifyOddInput(t *testing.T) {
	paths := []string{
		"", "/", ".mkv", "[", "[]", "[Group]", "- - -.mkv", "S01E.mkv", "1x.mkv", "S99E9999.mkv",
		"Show - 99999999.mkv", "Show.2024.13.45.mkv", "Show ~ ~ ~.mkv", "Show.S01E05-E03.mkv",
		`C:\Torrents\Show\Show.S01E01.mkv`, "動画/ショー - 05.mkv", strings.Repeat("a.", 500) + "mkv",
		"Show.S01E01.E02.E03.E04.mkv", "Show.mkv.mkv", "Season 0/Specials/x.mkv", "Show.S00E00.mkv",
	}
	files := make([]TorrentFile, len(paths))
	for i, p := range paths {
		files[i] = TorrentFile{Path: p, Size: int64(i) << 20}
	}
	for _, cat := range []*ShowCatalog{nil, {}, show(t, "frieren"), show(t, "one-piece")} {
		res := Classify(Input{TorrentName: "odd", Files: files, Show: cat, Now: corpusNow})
		if len(res.Proposals) != len(files) {
			t.Fatalf("%d proposals for %d files", len(res.Proposals), len(files))
		}
		for i, p := range res.Proposals {
			if p.Path != files[i].Path {
				t.Errorf("proposal %d path %q, want %q", i, p.Path, files[i].Path)
			}
			if utf8.RuneCountInString(p.Reason) > MaxReasonRunes {
				t.Errorf("%q: reason too long", p.Path)
			}
			if p.HasTarget() != p.Confidence.Valid() {
				t.Errorf("%q: targets %v with confidence %q", p.Path, p.Targets, p.Confidence)
			}
		}
	}
}

func TestClassifyIsDeterministic(t *testing.T) {
	for _, c := range loadCorpus(t) {
		if !c.Golden {
			continue
		}
		in := c.input(t)
		a, b := Classify(in), Classify(in)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two runs differ", c.ID)
		}
	}
}

func TestClassifyDoesNotModifyCatalog(t *testing.T) {
	cat := show(t, "frieren")
	before := append([]Episode(nil), cat.Episodes...)
	Classify(Input{TorrentName: "x", Files: []TorrentFile{{Path: "Frieren.S02E01.mkv", Size: 1 << 30}}, Show: cat, Now: corpusNow})
	if !reflect.DeepEqual(before, cat.Episodes) {
		t.Error("Classify modified the catalog")
	}
}

func TestPackContext(t *testing.T) {
	var frierenS2, onePiece corpusCase
	for _, c := range loadCorpus(t) {
		switch c.ID {
		case "frieren-scene-s02":
			frierenS2 = c
		case "onepiece-batch":
			onePiece = c
		}
	}
	res := Classify(frierenS2.input(t))
	if res.Pack.Type != PackSeason || res.Pack.Group != "VARYG" {
		t.Errorf("Frieren S02 pack: %+v", res.Pack)
	}
	if s := res.Pack.DirSeason[frierenS2.Torrent]; s != 2 {
		t.Errorf("Frieren S02 folder season = %d, want 2", s)
	}

	res = Classify(onePiece.input(t))
	if res.Pack.NameRange != [2]int{1071, 1075} || res.Pack.DirMode[onePiece.Torrent] != DirAbsolute {
		t.Errorf("One Piece pack: %+v", res.Pack)
	}
}

func TestSuggestedHints(t *testing.T) {
	res := Classify(Input{TorrentName: "x", Files: []TorrentFile{
		{Path: "[SubsPlease] Dr. Stone - New World - 01 (1080p).mkv", Size: 1 << 30},
	}, Show: show(t, "dr-stone"), Now: corpusNow})
	if len(res.SuggestedHints) == 0 || !strings.Contains(res.SuggestedHints[0], "new world") {
		t.Errorf("hints = %q", res.SuggestedHints)
	}
}
