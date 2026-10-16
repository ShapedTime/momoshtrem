package quality

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/release"
)

var now = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

const (
	month = 30 * 24 * time.Hour
	year  = 365 * 24 * time.Hour
)

func gb(v float64) int64 { return int64(v * 1e9) }
func mb(v float64) int64 { return int64(v * 1e6) }
func ago(d time.Duration) time.Time { return now.Add(-d) }

// candidate is one search result: its raw name and what search knows.
type candidate struct {
	id    string
	name  string
	facts Facts
}

type result struct {
	candidate
	Evaluation
}

func evaluateAll(t *testing.T, cands []candidate, p library.QualityProfile) []result {
	t.Helper()
	out := make([]result, len(cands))
	for i, c := range cands {
		c.facts.Now = now
		out[i] = result{c, Evaluate(release.Parse(c.name), c.facts, p)}
		t.Logf("%-3s %3d  %-75s reasons=%q notes=%q warnings=%q flags=%q", c.id, out[i].Score, c.name,
			out[i].Reasons, out[i].Notes, out[i].Warnings, out[i].Flags)
	}
	return out
}

// ranked lists the ids by score, highest first.
func ranked(results []result) []string {
	sorted := slices.Clone(results)
	slices.SortStableFunc(sorted, func(a, b result) int { return b.Score - a.Score })
	ids := make([]string, len(sorted))
	for i, r := range sorted {
		ids[i] = r.id
	}
	return ids
}

// best is the id Best picks, or "" for none.
func best(results []result) string {
	evals := make([]Evaluation, len(results))
	for i, r := range results {
		evals[i] = r.Evaluation
	}
	if i := Best(evals); i >= 0 {
		return results[i].id
	}
	return ""
}

func byID(t *testing.T, results []result, id string) result {
	t.Helper()
	for _, r := range results {
		if r.id == id {
			return r
		}
	}
	t.Fatalf("no result %q", id)
	return result{}
}

func assertOrder(t *testing.T, results []result, want ...string) {
	t.Helper()
	if got := ranked(results); !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// profile2160 prefers 2160p and Dolby Vision, like the prototype's.
func profile2160() library.QualityProfile {
	p := library.DefaultQualityProfile()
	p.PreferredResolution = "2160p"
	p.PreferDolbyVision = true
	return p
}

// The prototype's movie results: Perfect Days (2023), 124 minutes.
func TestEvaluate_PerfectDays(t *testing.T) {
	movie := func(size int64, seeders int, age time.Duration) Facts {
		return Facts{MediaType: library.JobMovie, RuntimeMinutes: 124, SizeBytes: size, Seeders: seeders, PublishedAt: ago(age)}
	}
	cands := []candidate{
		{"m1", "Perfect.Days.2023.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", movie(gb(15.6), 286, year)},
		{"m2", "Perfect.Days.2023.1080p.WEB-DL.DDP5.1.H.264-NTb", movie(gb(7.4), 410, year)},
		{"m3", "Perfect.Days.2023.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.Atmos.7.1-FGT", movie(gb(61.2), 52, 11*month)},
		{"m4", "Perfect Days 2023 HDCAM x264 AAC", movie(gb(1.2), 900, year)},
	}
	// No avoid terms, so the CAM competes on its own merits.
	p := profile2160()
	p.AvoidTerms = []string{}
	results := evaluateAll(t, cands, p)

	assertOrder(t, results, "m1", "m2", "m3", "m4")
	if got := best(results); got != "m1" {
		t.Errorf("best = %q, want m1", got)
	}

	m1 := byID(t, results, "m1")
	if want := []string{"2160p WEB-DL", "Dolby Vision", "286 seeders", "15.6 GB (7.5 GB/h)", "matches your quality profile"}; !reflect.DeepEqual(m1.Reasons, want) {
		t.Errorf("m1 reasons = %q, want %q", m1.Reasons, want)
	}
	if len(m1.Warnings) != 0 || len(m1.Flags) != 0 {
		t.Errorf("m1 warnings %q, flags %q; want none", m1.Warnings, m1.Flags)
	}

	m3 := byID(t, results, "m3")
	if !reflect.DeepEqual(m3.Flags, []string{FlagOverSizeLimit}) || !reflect.DeepEqual(m3.Warnings, []string{"Over your 8 GB/hour limit"}) {
		t.Errorf("m3 flags %q, warnings %q; want over_size_limit and its warning", m3.Flags, m3.Warnings)
	}
	if slices.Contains(m3.Reasons, "matches your quality profile") {
		t.Errorf("m3 reasons %q: an oversized release doesn't match the profile", m3.Reasons)
	}

	m4 := byID(t, results, "m4")
	if m4.Score >= 10 {
		t.Errorf("CAM score = %d, want under 10", m4.Score)
	}
	if !reflect.DeepEqual(m4.Flags, []string{FlagCAM}) || !reflect.DeepEqual(m4.Warnings, []string{"CAM recording · filmed in a cinema"}) {
		t.Errorf("CAM flags %q, warnings %q", m4.Flags, m4.Warnings)
	}
	if len(m4.Reasons) != 0 {
		t.Errorf("CAM reasons = %q, want none", m4.Reasons)
	}

	// The default avoid terms hide it outright.
	if e := Evaluate(release.Parse(cands[3].name), cands[3].facts, profile2160()); !e.Hidden || e.HiddenReason != `Has "HDCAM", a term you avoid` {
		t.Errorf("with the default avoid terms: hidden %v (%q)", e.Hidden, e.HiddenReason)
	}
}

// The prototype's episode results: Severance S02E08, with E08–E09
// missing and E01–E07 on disk.
func TestEvaluate_SeveranceEpisode(t *testing.T) {
	episode := func(size int64, seeders int) Facts {
		return Facts{
			MediaType: library.JobTV, RuntimeMinutes: 50, Scope: ScopeEpisode, Season: 2, Episode: 8,
			SizeBytes: size, Seeders: seeders, PublishedAt: ago(8 * month),
			Episodes: 1, Seasons: 1, ShowSeasons: 2, CoversRequested: true, MissingCovered: 1, MissingTotal: 2,
		}
	}
	pack := episode(gb(41.8), 198)
	pack.Episodes, pack.MissingCovered, pack.Have = 10, 2, 7
	cands := []candidate{
		{"s1", "Severance.S02E08.Sweet.Vitriol.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", episode(gb(4.1), 312)},
		{"s2", "Severance.S02E08.Sweet.Vitriol.1080p.ATVP.WEB-DL.DDP5.1.H.264-NTb", episode(gb(2.6), 540)},
		{"s3", "Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", pack},
		{"s4", "Severance S02E08 720p HEVC x265-MeGusta", episode(mb(410), 77)},
	}
	results := evaluateAll(t, cands, profile2160())

	assertOrder(t, results, "s1", "s3", "s2", "s4")
	if got := best(results); got != "s1" {
		t.Errorf("best = %q, want s1", got)
	}
	s1 := byID(t, results, "s1")
	if want := []string{"2160p WEB-DL", "Dolby Vision", "312 seeders", "4.1 GB", "matches your quality profile"}; !reflect.DeepEqual(s1.Reasons, want) {
		t.Errorf("s1 reasons = %q, want %q", s1.Reasons, want)
	}
	s3 := byID(t, results, "s3")
	if len(s3.Reasons) == 0 || s3.Reasons[0] != "Covers both missing episodes" {
		t.Errorf("s3 reasons = %q, want the missing episodes first", s3.Reasons)
	}
	if want := []string{"Includes 7 episodes you already have; the mapping review asks before replacing them"}; !reflect.DeepEqual(s3.Notes, want) {
		t.Errorf("s3 notes = %q, want %q", s3.Notes, want)
	}
}

// The prototype's season results: The Good Place season 1 (four seasons,
// 22-minute episodes), with NTb a preferred group.
func TestEvaluate_GoodPlaceSeason(t *testing.T) {
	season := func(size int64, seeders int, age time.Duration) Facts {
		return Facts{
			MediaType: library.JobTV, RuntimeMinutes: 22, Scope: ScopeSeason, Season: 1,
			SizeBytes: size, Seeders: seeders, PublishedAt: ago(age),
			Episodes: 13, Seasons: 1, ShowSeasons: 4, CoversRequested: true,
		}
	}
	complete := season(gb(22.7), 88, 3*year)
	complete.Episodes, complete.Seasons = 53, 4
	partial := season(gb(2.2), 300, 5*year)
	partial.Episodes, partial.CoversRequested = 2, false
	cands := []candidate{
		{"g1", "The.Good.Place.S01.1080p.NF.WEB-DL.DDP5.1.x264-NTb", season(gb(14.2), 141, 5*year)},
		{"g2", "The.Good.Place.S01-S04.COMPLETE.1080p.BluRay.x265.10bit.AAC5.1-YAWNiX", complete},
		{"g3", "The Good Place Season 1 Complete 720p WEB x264 [i_c]", season(gb(4.9), 37, 7*year)},
		{"g4", "The.Good.Place.S01.2160p.HDR.WEB.H265-CAKES", season(gb(31.0), 4, 2*year)},
		{"g5", "The.Good.Place.S01E01-E02.1080p.WEB.H264-GROUP", partial},
	}
	p := library.DefaultQualityProfile()
	p.PreferredGroups = []string{"ntb"}
	results := evaluateAll(t, cands, p)

	if got := ranked(results)[0]; got != "g1" {
		t.Errorf("top = %q, want the season pack g1", got)
	}
	if got := best(results); got != "g1" {
		t.Errorf("best = %q, want g1", got)
	}
	if g1, g5 := byID(t, results, "g1"), byID(t, results, "g5"); g1.Score <= g5.Score {
		t.Errorf("season pack %d <= two episodes %d", g1.Score, g5.Score)
	}
	g1 := byID(t, results, "g1")
	if want := []string{"Whole season 1", "1080p WEB-DL from NTb, a group you prefer", "141 seeders", "1.1 GB per episode", "matches your quality profile"}; !reflect.DeepEqual(g1.Reasons, want) {
		t.Errorf("g1 reasons = %q, want %q", g1.Reasons, want)
	}
	if g2 := byID(t, results, "g2"); !reflect.DeepEqual(g2.Notes, []string{"Covers all 4 seasons, 53 episodes"}) {
		t.Errorf("g2 notes = %q", g2.Notes)
	}
	g4 := byID(t, results, "g4")
	if !reflect.DeepEqual(g4.Warnings, []string{"Only 4 seeders · may be slow"}) || !reflect.DeepEqual(g4.Flags, []string{FlagLowSeeders}) {
		t.Errorf("g4 warnings %q, flags %q", g4.Warnings, g4.Flags)
	}
}

// The prototype's anime results: Frieren S01E29, with S01E29–E38 missing.
// A pack of just the missing run beats the episode itself.
func TestEvaluate_FrierenMissingRun(t *testing.T) {
	facts := func(size int64, seeders, episodes int) Facts {
		return Facts{
			MediaType: library.JobTV, RuntimeMinutes: 24, Scope: ScopeEpisode, Season: 1, Episode: 29,
			SizeBytes: size, Seeders: seeders, PublishedAt: ago(9 * month),
			Episodes: episodes, Seasons: 1, ShowSeasons: 1, CoversRequested: true,
			MissingCovered: episodes, MissingTotal: 10,
		}
	}
	scene := facts(gb(14.1), 188, 10)
	scene.SceneNote = `Scene "Season 2" = TMDB S01E29–E38`
	cands := []candidate{
		{"r1", "[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]", facts(gb(13.2), 412, 10)},
		{"r2", "Frieren.Beyond.Journeys.End.S02.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG", scene},
		{"r3", "[Erai-raws] Sousou no Frieren - 29 [1080p][Multiple Subtitle][7B1C0F1E].mkv", facts(gb(1.4), 96, 1)},
		{"r5", "[ASW] Sousou no Frieren - 29 [1080p HEVC x265 10Bit][AAC]", facts(mb(312), 64, 1)},
		{"r6", "Frieren - 29 [720p] [Judas]", facts(mb(220), 0, 1)},
	}
	results := evaluateAll(t, cands, library.DefaultQualityProfile())

	assertOrder(t, results, "r1", "r2", "r3", "r5", "r6")
	if got := best(results); got != "r1" {
		t.Errorf("best = %q, want r1", got)
	}
	r1 := byID(t, results, "r1")
	if want := []string{"Covers all 10 missing episodes", "1080p", "412 seeders", "1.3 GB per episode", "matches your quality profile"}; !reflect.DeepEqual(r1.Reasons, want) {
		t.Errorf("r1 reasons = %q, want %q", r1.Reasons, want)
	}
	if r2 := byID(t, results, "r2"); !reflect.DeepEqual(r2.Notes, []string{scene.SceneNote}) {
		t.Errorf("r2 notes = %q, want the scene note", r2.Notes)
	}
	r6 := byID(t, results, "r6")
	if !reflect.DeepEqual(r6.Flags, []string{FlagNoSeeders}) || !reflect.DeepEqual(r6.Warnings, []string{"No seeders"}) {
		t.Errorf("r6 flags %q, warnings %q", r6.Flags, r6.Warnings)
	}
}

// For an episode, the episode beats bigger packs of the same quality; for
// a season, the season pack beats the rest.
func TestEvaluate_CoverageFit(t *testing.T) {
	base := Facts{MediaType: library.JobTV, RuntimeMinutes: 45, Seeders: 120, PublishedAt: ago(year), ShowSeasons: 3}
	tests := []struct {
		name  string
		scope Scope
		cands []candidate
		want  []string
	}{
		{
			name:  "episode request",
			scope: ScopeEpisode,
			cands: []candidate{
				{"complete", "Show.Complete.Series.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(60), Episodes: 30, Seasons: 3}},
				{"season", "Show.S02.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(20), Episodes: 10, Seasons: 1}},
				{"episode", "Show.S02E05.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(2), Episodes: 1, Seasons: 1}},
				{"few", "Show.S02E04-E06.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(6), Episodes: 3, Seasons: 1}},
			},
			want: []string{"episode", "few", "season", "complete"},
		},
		{
			name:  "season request",
			scope: ScopeSeason,
			cands: []candidate{
				{"few", "Show.S02E04-E06.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(6), Episodes: 3, Seasons: 1}},
				{"complete", "Show.Complete.Series.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(60), Episodes: 30, Seasons: 3, CoversRequested: true}},
				{"season", "Show.S02.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(20), Episodes: 10, Seasons: 1, CoversRequested: true}},
			},
			want: []string{"season", "complete", "few"},
		},
		{
			name:  "season request, coverage unknown",
			scope: ScopeSeason,
			cands: []candidate{
				{"complete", "Show.Complete.Series.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(60)}},
				{"few", "Show.S02E04-E06.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(6)}},
				{"season", "Show.S02.1080p.WEB-DL.DDP5.1.H.264-GRP", Facts{SizeBytes: gb(20)}},
			},
			want: []string{"season", "complete", "few"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.cands {
				f := &tt.cands[i].facts
				f.MediaType, f.RuntimeMinutes, f.Seeders, f.PublishedAt, f.ShowSeasons = base.MediaType, base.RuntimeMinutes, base.Seeders, base.PublishedAt, base.ShowSeasons
				f.Scope, f.Season, f.Episode = tt.scope, 2, 5
				if tt.scope == ScopeEpisode {
					f.CoversRequested = true
				}
			}
			results := evaluateAll(t, tt.cands, library.DefaultQualityProfile())
			assertOrder(t, results, tt.want...)
			if got := best(results); got != tt.want[0] {
				t.Errorf("best = %q, want %q", got, tt.want[0])
			}
		})
	}
}

// A preferred group is a small boost: it decides between equals, but
// never beats a much healthier release or the size limit.
func TestEvaluate_PreferredGroup(t *testing.T) {
	movie := func(size int64, seeders int) Facts {
		return Facts{MediaType: library.JobMovie, RuntimeMinutes: 120, SizeBytes: size, Seeders: seeders, PublishedAt: ago(year)}
	}
	p := library.DefaultQualityProfile()
	p.PreferredGroups = []string{"FLUX"}
	tests := []struct {
		name string
		a, b candidate // a wins
	}{
		{"boosts an equal", candidate{"flux", "Film.2024.1080p.WEB-DL.DDP5.1.H.264-FLUX", movie(gb(8), 60)}, candidate{"other", "Film.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", movie(gb(8), 60)}},
		{"not above seeders", candidate{"healthy", "Film.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", movie(gb(8), 300)}, candidate{"flux", "Film.2024.1080p.WEB-DL.DDP5.1.H.264-FLUX", movie(gb(8), 8)}},
		{"not above the size limit", candidate{"fits", "Film.2024.1080p.BluRay.x264-NTb", movie(gb(14), 60)}, candidate{"flux", "Film.2024.1080p.BluRay.x264-FLUX", movie(gb(19), 60)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := evaluateAll(t, []candidate{tt.b, tt.a}, p)
			assertOrder(t, results, tt.a.id, tt.b.id)
		})
	}
}

func TestEvaluate_Hidden(t *testing.T) {
	limit := "1080p"
	p := library.DefaultQualityProfile()
	p.MaxResolution = &limit
	p.AvoidTerms = append(p.AvoidTerms, "korsub", "web dl")
	tests := []struct {
		name       string
		release    string
		wantReason string // "" when shown
	}{
		{"telesync", "Movie.2023.TS.x264", `Has "TS", a term you avoid`},
		{"cam in brackets", "Movie (2023) [HDCAM] [x264]", `Has "HDCAM", a term you avoid`},
		{"lower case", "movie.2023.hdts.x264", `Has "HDTS", a term you avoid`},
		{"hardcoded subs", "Movie.2023.1080p.HC.KORSUB.WEBRip.x264", `Has "korsub", a term you avoid`},
		{"hyphenated term", "Movie.2023.720p.WEB-DL.x264-GRP", `Has "web dl", a term you avoid`},
		{"above the limit", "Movie.2023.2160p.WEBRip.x265-GRP", "Above your 1080p limit"},
		{"a word containing a term", "Ghosts.2023.1080p.BluRay.x264-GRP", ""},
		{"at the limit", "Movie.2023.1080p.BluRay.x264-GRP", ""},
		{"no resolution", "Movie.2023.BluRay.x264-GRP", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Evaluate(release.Parse(tt.release), Facts{MediaType: library.JobMovie, Seeders: 50}, p)
			if e.Hidden != (tt.wantReason != "") || e.HiddenReason != tt.wantReason {
				t.Errorf("hidden = %v (%q), want %q", e.Hidden, e.HiddenReason, tt.wantReason)
			}
		})
	}
}

func TestEvaluate_Reasons(t *testing.T) {
	p := library.DefaultQualityProfile()
	p.PreferredResolution = "2160p"
	p.PreferHDR, p.PreferX265 = true, true

	// Every reason, in order.
	full := Evaluate(release.Parse("Show.S01.2160p.WEB-DL.DDP5.1.HDR10.H.265-GRP"), Facts{
		MediaType: library.JobTV, RuntimeMinutes: 50, Scope: ScopeSeason, Season: 1,
		SizeBytes: gb(20), Seeders: 1204, PublishedAt: ago(month), Now: now,
		Episodes: 10, Seasons: 1, ShowSeasons: 2, CoversRequested: true,
	}, p)
	want := []string{"Whole season 1", "2160p WEB-DL", "HDR10", "x265", "1,204 seeders", "2.0 GB per episode", "matches your quality profile"}
	if !reflect.DeepEqual(full.Reasons, want) {
		t.Errorf("reasons = %q, want %q", full.Reasons, want)
	}

	// A weak release has no negative reasons, only warnings.
	weak := Evaluate(release.Parse("Show.S01E01.720p.HDTV.x264-GRP"), Facts{
		MediaType: library.JobTV, RuntimeMinutes: 50, Scope: ScopeEpisode, Season: 1, Episode: 1,
		SizeBytes: gb(9), Seeders: 3, Episodes: 1, Seasons: 1, CoversRequested: true,
	}, p)
	if len(weak.Reasons) != 0 {
		t.Errorf("weak reasons = %q, want none", weak.Reasons)
	}
	if want := []string{"Only 3 seeders · may be slow", "Over your 8 GB/hour limit"}; !reflect.DeepEqual(weak.Warnings, want) {
		t.Errorf("weak warnings = %q, want %q", weak.Warnings, want)
	}
	if want := []string{FlagLowSeeders, FlagOverSizeLimit}; !reflect.DeepEqual(weak.Flags, want) {
		t.Errorf("weak flags = %q, want %q", weak.Flags, want)
	}

	// Dolby Vision answers both HDR preferences with one reason.
	p.PreferDolbyVision = true
	dv := Evaluate(release.Parse("Film.2024.2160p.WEB-DL.DDP5.1.DV.HDR.H.265-GRP"), Facts{
		MediaType: library.JobMovie, RuntimeMinutes: 100, SizeBytes: gb(12), Seeders: 15,
	}, p)
	if want := []string{"2160p WEB-DL", "Dolby Vision", "x265", "12.0 GB (7.2 GB/h)", "matches your quality profile"}; !reflect.DeepEqual(dv.Reasons, want) {
		t.Errorf("DV reasons = %q, want %q", dv.Reasons, want)
	}
}

func TestEvaluate_Size(t *testing.T) {
	const (
		small = "Unusually small for 2160p · may be fake or poor quality"
		large = "Unusually large for its runtime"
		over  = "Over your 7.5 GB/hour limit"
	)
	tests := []struct {
		name      string
		release   string
		facts     Facts
		wantFlags []string
		wantWarns []string
	}{
		{"plausible film", "Film.2024.2160p.WEB-DL.H.265-GRP", Facts{MediaType: library.JobMovie, RuntimeMinutes: 120, SizeBytes: gb(14)}, nil, nil},
		{"tiny 2160p film", "Film.2024.2160p.WEB-DL.H.265-GRP", Facts{MediaType: library.JobMovie, RuntimeMinutes: 120, SizeBytes: mb(500)}, []string{FlagSizeOutlier}, []string{small}},
		{"50 MB episode, runtime unknown", "Show.S01E01.WEB.x264-GRP", Facts{MediaType: library.JobTV, SizeBytes: mb(50), Episodes: 1}, []string{FlagSizeOutlier}, []string{"Unusually small · may be fake or poor quality"}},
		{"implausibly large", "Film.2024.1080p.BluRay.x264-GRP", Facts{MediaType: library.JobMovie, RuntimeMinutes: 90, SizeBytes: gb(120)}, []string{FlagSizeOutlier, FlagOverSizeLimit}, []string{large, over}},
		{"over the limit", "Film.2024.1080p.BluRay.x264-GRP", Facts{MediaType: library.JobMovie, RuntimeMinutes: 90, SizeBytes: gb(15)}, []string{FlagOverSizeLimit}, []string{over}},
		{"per episode in a pack", "Show.S01.1080p.BluRay.x264-GRP", Facts{MediaType: library.JobTV, RuntimeMinutes: 45, SizeBytes: gb(40), Episodes: 10}, nil, nil},
		{"no runtime, no limit", "Film.2024.1080p.BluRay.x264-GRP", Facts{MediaType: library.JobMovie, SizeBytes: gb(60)}, nil, nil},
		{"pack of unknown length", "Show.Complete.1080p.BluRay.x264-GRP", Facts{MediaType: library.JobTV, RuntimeMinutes: 45, SizeBytes: mb(30)}, nil, nil},
	}
	p := library.DefaultQualityProfile()
	p.MaxGBPerHour = 7.5
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.facts.Seeders = 50
			e := Evaluate(release.Parse(tt.release), tt.facts, p)
			if !slices.Equal(e.Flags, tt.wantFlags) || !slices.Equal(e.Warnings, tt.wantWarns) {
				t.Errorf("flags %q, warnings %q; want %q, %q", e.Flags, e.Warnings, tt.wantFlags, tt.wantWarns)
			}
		})
	}
}

func TestEvaluate_Freshness(t *testing.T) {
	name := "Film.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	score := func(published time.Time) int {
		return Evaluate(release.Parse(name), Facts{
			MediaType: library.JobMovie, RuntimeMinutes: 110, SizeBytes: gb(7), Seeders: 80, PublishedAt: published, Now: now,
		}, library.DefaultQualityProfile()).Score
	}
	fresh, old, unknown := score(ago(3*24*time.Hour)), score(ago(6*year)), score(time.Time{})
	if !(fresh > unknown && unknown > old) {
		t.Errorf("fresh %d, unknown %d, old %d; want fresh > unknown > old", fresh, unknown, old)
	}
	if fresh-old > weightFreshness {
		t.Errorf("freshness moved the score by %d, want at most %d", fresh-old, weightFreshness)
	}
}

func TestEvaluate_EmptyListsAreNotNil(t *testing.T) {
	e := Evaluate(release.Parse(""), Facts{}, library.DefaultQualityProfile())
	if e.Reasons == nil || e.Notes == nil || e.Warnings == nil || e.Flags == nil {
		t.Errorf("evaluation %+v has nil lists; the JSON needs []", e)
	}
	if e.Score < 0 || e.Score > 100 {
		t.Errorf("score = %d, want 0–100", e.Score)
	}
}

func TestBest(t *testing.T) {
	ev := func(score int, hidden bool, flags ...string) Evaluation {
		return Evaluation{Score: score, Hidden: hidden, Flags: flags}
	}
	tests := []struct {
		name  string
		evals []Evaluation
		want  int
	}{
		{"none", nil, -1},
		{"top score", []Evaluation{ev(70, false), ev(90, false), ev(80, false)}, 1},
		{"first of a tie", []Evaluation{ev(85, false), ev(85, false)}, 0},
		{"skips hidden", []Evaluation{ev(95, true), ev(60, false)}, 1},
		{"skips CAM", []Evaluation{ev(99, false, FlagCAM), ev(60, false)}, 1},
		{"skips zero seeders", []Evaluation{ev(99, false, FlagNoSeeders), ev(60, false, FlagLowSeeders)}, 1},
		{"needs 50", []Evaluation{ev(49, false), ev(30, false)}, -1},
		{"exactly 50", []Evaluation{ev(50, false)}, 0},
		{"only bad ones", []Evaluation{ev(80, false, FlagNoSeeders), ev(70, true), ev(9, false, FlagCAM)}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Best(tt.evals); got != tt.want {
				t.Errorf("Best = %d, want %d", got, tt.want)
			}
		})
	}
}

// A zero-seeder release is never the best match, however good it is.
func TestEvaluate_ZeroSeedersNeverBest(t *testing.T) {
	movie := func(seeders int) Facts {
		return Facts{MediaType: library.JobMovie, RuntimeMinutes: 124, SizeBytes: gb(15.6), Seeders: seeders, PublishedAt: ago(month)}
	}
	cands := []candidate{
		{"dead", "Perfect.Days.2023.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", movie(0)},
		{"alive", "Perfect.Days.2023.720p.WEBRip.x264-GRP", movie(30)},
	}
	results := evaluateAll(t, cands, profile2160())
	if got := best(results); got == "dead" {
		t.Error("best is the zero-seeder release")
	}
	if dead := byID(t, results, "dead"); !strings.Contains(strings.Join(dead.Warnings, "|"), "No seeders") {
		t.Errorf("dead warnings = %q", dead.Warnings)
	}
}
