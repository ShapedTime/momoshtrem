// Package quality scores torrent search results against the quality
// profile (PLAN §6 "Search", "Profile effects"): whether the profile hides
// a release, its 0–100 score with the reasons for it, and the notes,
// warnings and flags its card shows. Best picks the best match.
//
// It's pure: search parses the name (internal/release) and gathers the
// Facts, such as the TMDB episodes a pack covers (identify.Cover).
package quality

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/release"
)

// Scope is what a TV search asked for. Its values are search.Scope's.
type Scope string

const (
	ScopeEpisode Scope = "episode"
	ScopeSeason  Scope = "season"
	ScopeAny     Scope = "any"
)

// Flags (Release.flags), in the contract's order. needs_torrent_fetch is
// search's.
const (
	FlagCAM           = "cam"
	FlagNoSeeders     = "no_seeders"
	FlagLowSeeders    = "low_seeders"     // 1–4 seeders
	FlagSizeOutlier   = "size_outlier"    // implausibly small or large for its runtime
	FlagOverSizeLimit = "over_size_limit" // over the profile's max_gb_per_hour
)

// BestMinScore is the lowest score a best match may have.
const BestMinScore = 50

// Facts are what search knows about a result and the request besides the
// release name.
type Facts struct {
	Seeders     int
	SizeBytes   int64
	PublishedAt time.Time // zero when unknown
	Now         time.Time // when the results are read; freshness needs it

	MediaType library.JobMediaType // JobMovie or JobTV
	// RuntimeMinutes is the movie's runtime, or a show's typical episode's.
	// 0 (unknown) skips the size checks: there's nothing to compare with.
	RuntimeMinutes int

	// TV only.
	Scope   Scope
	Season  int // the requested season (ScopeEpisode, ScopeSeason)
	Episode int // the requested episode (ScopeEpisode)
	// Episodes is how many TMDB episodes the release covers; 0 when that's
	// unknown, so the pack type alone decides.
	Episodes int
	// Seasons is how many regular TMDB seasons those episodes span, and
	// ShowSeasons how many the show has.
	Seasons, ShowSeasons int
	// CoversRequested: the release has the requested episode
	// (ScopeEpisode), or every aired episode of the requested season
	// (ScopeSeason).
	CoversRequested bool
	// MissingTotal is the length of the title's missing run when the
	// request falls in it (the response's `missing`), else 0;
	// MissingCovered is how many of those episodes the release has.
	MissingCovered, MissingTotal int
	// Have is how many of the covered episodes already have a file, a
	// torrent's or an HDRezka stream.
	Have int
	// SceneNote explains a scene numbering, as a note:
	// `Scene "Season 2" = TMDB S01E29–E38`. "" for none.
	SceneNote string
}

// Evaluation is what the profile makes of one release.
type Evaluation struct {
	// Hidden: the profile drops the release (counted in `hidden`).
	// HiddenReason says why.
	Hidden       bool
	HiddenReason string
	Score        int      // 0–100
	Reasons      []string // score_reasons: positive only, most important first
	Notes        []string
	Warnings     []string
	Flags        []string // in the contract's order
}

// HasFlag reports whether e carries flag.
func (e Evaluation) HasFlag(flag string) bool { return slices.Contains(e.Flags, flag) }

// Best is the index of the best match (best_id): the top score among the
// releases that aren't hidden or flagged cam or no_seeders, with a score of
// at least BestMinScore; the first on a tie. -1 when there's none.
func Best(evals []Evaluation) int {
	best := -1
	for i, e := range evals {
		if e.Hidden || e.Score < BestMinScore || e.HasFlag(FlagCAM) || e.HasFlag(FlagNoSeeders) {
			continue
		}
		if best < 0 || e.Score > evals[best].Score {
			best = i
		}
	}
	return best
}

// Score weights. A release that fits every part scores 96, and a preferred
// group adds the last 4, so it never beats much healthier seeding or the
// size limit's penalty.
const (
	weightProfile   = 36
	weightSeeders   = 26
	weightCoverage  = 20
	weightSize      = 10
	weightFreshness = 4
	boostGroup      = 4

	penaltyNoSeeders  = 20
	penaltyLowSeeders = 8
	penaltyOverLimit  = 12
	penaltyOutlier    = 20
	// camFactor scales a CAM's whole score: it stays under 10 however
	// healthy it is.
	camFactor = 0.08

	// fullSeeders is where the seeders' log scale tops out.
	fullSeeders = 500
	// lowSeeders: fewer seeders get the low_seeders warning.
	lowSeeders = 5
	// seedersReason: from this many, seeders are a reason.
	seedersReason = 20
)

// Evaluate scores r against p with what search knows about it.
func Evaluate(r release.Release, f Facts, p library.QualityProfile) Evaluation {
	e := Evaluation{Reasons: []string{}, Notes: []string{}, Warnings: []string{}, Flags: []string{}}
	ev := evaluator{r: r, f: f, p: p, tv: f.MediaType == library.JobTV, cam: r.Source == "CAM"}
	e.Hidden, e.HiddenReason = ev.hidden()

	size := ev.size()
	score := weightProfile*ev.profileFit() +
		weightSeeders*seedersFit(f.Seeders) +
		weightCoverage*ev.coverageFit() +
		weightSize*size.fit +
		weightFreshness*freshness(f.PublishedAt, f.Now)
	if ev.preferredGroup() {
		score += boostGroup
	}

	if ev.cam {
		e.Flags = append(e.Flags, FlagCAM)
		e.Warnings = append(e.Warnings, "CAM recording · filmed in a cinema")
	}
	switch {
	case f.Seeders <= 0:
		score -= penaltyNoSeeders
		e.Flags = append(e.Flags, FlagNoSeeders)
		e.Warnings = append(e.Warnings, "No seeders")
	case f.Seeders < lowSeeders:
		score -= penaltyLowSeeders
		e.Flags = append(e.Flags, FlagLowSeeders)
		e.Warnings = append(e.Warnings, fmt.Sprintf("Only %s · may be slow", plural(f.Seeders, "seeder", "seeders")))
	}
	if size.outlier != "" {
		score -= penaltyOutlier
		e.Flags = append(e.Flags, FlagSizeOutlier)
		e.Warnings = append(e.Warnings, size.outlier)
	}
	if size.over {
		score -= penaltyOverLimit
		e.Flags = append(e.Flags, FlagOverSizeLimit)
		e.Warnings = append(e.Warnings, fmt.Sprintf("Over your %s GB/hour limit", formatNumber(p.MaxGBPerHour)))
	}
	if ev.cam {
		score *= camFactor
	}
	e.Score = int(math.Round(min(max(score, 0), 100)))

	if !ev.cam { // nothing about a CAM recording is a reason to pick it
		e.Reasons = ev.reasons(size)
	}
	e.Notes = ev.notes()
	return e
}

// evaluator holds one release's evaluation inputs.
type evaluator struct {
	r       release.Release
	f       Facts
	p       library.QualityProfile
	tv, cam bool
}

// resolutions rank a release's resolutions, lowest first.
var resolutions = []string{"480p", "576p", "720p", "1080p", "2160p"}

func resolutionRank(res string) int { return slices.Index(resolutions, res) }

// preferredRank is the profile's preferred resolution's rank; 1080p when
// it names none.
func (ev evaluator) preferredRank() int {
	if r := resolutionRank(ev.p.PreferredResolution); r >= 0 {
		return r
	}
	return resolutionRank("1080p")
}

// limitRank is the profile's max_resolution's rank; -1 for no limit.
func (ev evaluator) limitRank() int {
	if ev.p.MaxResolution == nil {
		return -1
	}
	return resolutionRank(*ev.p.MaxResolution)
}

// hidden applies the profile's hide rules: an avoid term as a whole word,
// or a resolution above max_resolution.
func (ev evaluator) hidden() (bool, string) {
	if term := avoidedTerm(ev.r.DisplayTitle, ev.p.AvoidTerms); term != "" {
		return true, fmt.Sprintf("Has %q, a term you avoid", term)
	}
	if limit := ev.limitRank(); limit >= 0 && resolutionRank(ev.r.Resolution) > limit {
		return true, fmt.Sprintf("Above your %s limit", *ev.p.MaxResolution)
	}
	return false, ""
}

// avoidedTerm is the first term that name has as a whole word, case
// insensitive: "TS" is in "Movie.2023.TS.x264" but not in "Ghosts". Every
// run of characters other than letters and digits counts as one space, so
// "WEB-DL" also matches "WEB.DL".
func avoidedTerm(name string, terms []string) string {
	text := words(name)
	for _, t := range terms {
		if w := words(t); w != " " && strings.Contains(text, w) {
			return t
		}
	}
	return ""
}

// words lowercases s to its words, one space between them and one at each
// end: " perfect days 2023 hdcam x264 ".
func words(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte(' ')
	space := true
	for _, c := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			b.WriteRune(c)
			space = false
		case !space:
			b.WriteByte(' ')
			space = true
		}
	}
	if !space {
		b.WriteByte(' ')
	}
	return b.String()
}

// Profile fit points: the resolution, the source, and 3 for each thing the
// profile prefers (x265, HDR, Dolby Vision) that the release has.
const (
	pointsResolution = 20
	pointsSource     = 7
	pointsPreference = 3
)

var sourcePoints = map[string]float64{
	"REMUX": 7, "BluRay": 7, "WEB-DL": 6.5, "WEB": 6, "WEBRip": 5.5,
	release.Unknown: 5, "HDTV": 3, "DVD": 2, "CAM": 0,
}

// profileFit is how well the release fits the profile, 0–1: the
// preferred resolution is best, one above it (within the limit) nearly as
// good, and each step below costs more.
func (ev evaluator) profileFit() float64 {
	got := ev.resolutionPoints() + sourcePoints[ev.r.Source]
	most := float64(pointsResolution + pointsSource)
	for _, pref := range ev.preferences() {
		most += pointsPreference
		if pref.has {
			got += pointsPreference
		}
	}
	return got / most
}

func (ev evaluator) resolutionPoints() float64 {
	rank := resolutionRank(ev.r.Resolution)
	if rank < 0 {
		return 6
	}
	switch below := ev.preferredRank() - rank; {
	case below == 0:
		return pointsResolution
	case below < 0:
		return 18 // above preferred; a release above the limit is hidden anyway
	case below == 1:
		return 11
	case below == 2:
		return 5
	}
	return 2
}

// preference is one thing the profile asks for and whether r has it.
type preference struct {
	has    bool
	reason string // the reason when it has it; "" for none
}

// preferences are the profile's preferences that are on, in reason order.
func (ev evaluator) preferences() []preference {
	var out []preference
	dv := slices.Contains(ev.r.HDR, "DV")
	if ev.p.PreferDolbyVision {
		out = append(out, preference{dv, "Dolby Vision"})
	}
	if ev.p.PreferHDR {
		reason := ""
		switch {
		case dv && ev.p.PreferDolbyVision: // "Dolby Vision" already says it
		case dv:
			reason = "Dolby Vision"
		case len(ev.r.HDR) > 0:
			reason = ev.r.HDR[0]
		}
		out = append(out, preference{len(ev.r.HDR) > 0, reason})
	}
	if ev.p.PreferX265 {
		out = append(out, preference{ev.r.Codec == "x265", "x265"})
	}
	return out
}

func (ev evaluator) preferredGroup() bool {
	if ev.r.Group == "" {
		return false
	}
	for _, g := range ev.p.PreferredGroups {
		if strings.EqualFold(strings.TrimSpace(g), ev.r.Group) {
			return true
		}
	}
	return false
}

// seedersFit puts seeders on a log scale, 0–1: 10 seeders are 0.39, 100
// are 0.74, fullSeeders and more are 1.
func seedersFit(seeders int) float64 {
	if seeders <= 0 {
		return 0
	}
	return min(1, math.Log10(float64(seeders)+1)/math.Log10(fullSeeders+1))
}

// freshness is 0–1 by the release's age; 0.5 when it's unknown.
func freshness(published, now time.Time) float64 {
	if published.IsZero() || now.IsZero() {
		return 0.5
	}
	switch age := now.Sub(published); {
	case age <= 30*24*time.Hour:
		return 1
	case age <= 365*24*time.Hour:
		return 0.75
	case age <= 3*365*24*time.Hour:
		return 0.5
	}
	return 0.25
}

// Coverage kinds: what a TV release holds, by its pack and the TMDB
// episodes it covers.
type coverage int

const (
	coverUnknown coverage = iota
	coverEpisode          // one episode
	coverFew              // a few episodes (up to fewEpisodes)
	coverSeason           // a season's worth: a season pack, or a long run
	coverSeasons          // several seasons, or the whole show
)

const fewEpisodes = 6

func (ev evaluator) coverage() coverage {
	n := ev.f.Episodes
	pack := ev.r.Pack
	if n == 0 && pack != nil && pack.Episodes != nil {
		n = pack.Episodes.To - pack.Episodes.From + 1
	}
	switch {
	case ev.f.Seasons > 1:
		return coverSeasons
	case pack == nil:
	case pack.Type == release.PackMultiSeason || pack.Type == release.PackComplete:
		return coverSeasons
	case pack.Type == release.PackSeason:
		return coverSeason
	}
	switch {
	case n == 1:
		return coverEpisode
	case n > 1 && n <= fewEpisodes:
		return coverFew
	case n > fewEpisodes:
		return coverSeason
	}
	return coverUnknown
}

// coversMissingRun: the release has every episode of the missing run the
// request falls in (two or more).
func (ev evaluator) coversMissingRun() bool {
	return ev.f.MissingTotal >= 2 && ev.f.MissingCovered >= ev.f.MissingTotal
}

// coverageFit is how well what the release holds fits the request, 0–1.
// For an episode: the exact episode, then a few episodes, a season pack,
// several seasons; a pack of just the missing run beats them all. For a
// season, the reverse: the whole season, then several seasons, then
// episodes. Movies always fit.
func (ev evaluator) coverageFit() float64 {
	if !ev.tv {
		return 1
	}
	f, kind := ev.f, ev.coverage()
	// Exactly the episodes that are missing, and nothing you have.
	if ev.coversMissingRun() && f.Have == 0 {
		return 1
	}
	known := f.Episodes > 0
	switch f.Scope {
	case ScopeEpisode:
		switch {
		case known && !f.CoversRequested:
			return 0.2
		case kind == coverEpisode && f.MissingTotal >= 2:
			return 0.9 // a pack of the run would get the rest too
		}
		return episodeFit[kind]
	case ScopeSeason:
		switch {
		case f.CoversRequested && f.Seasons <= 1:
			return 1
		case f.CoversRequested:
			return 0.7
		case !known && kind == coverSeason:
			return 1
		}
		return seasonFit[kind]
	}
	if f.ShowSeasons > 0 && f.Seasons >= f.ShowSeasons {
		return 1
	}
	return anyFit[kind]
}

// Coverage fit by what a release holds, per scope.
var (
	episodeFit = [...]float64{coverEpisode: 1, coverFew: 0.75, coverSeason: 0.5, coverSeasons: 0.25, coverUnknown: 0.4}
	seasonFit  = [...]float64{coverSeasons: 0.7, coverSeason: 0.55, coverFew: 0.35, coverEpisode: 0.2, coverUnknown: 0.3}
	anyFit     = [...]float64{coverSeasons: 0.9, coverSeason: 0.75, coverFew: 0.55, coverEpisode: 0.5, coverUnknown: 0.5}
)

// sizeCheck is the size against the runtime and the profile.
type sizeCheck struct {
	fit       float64 // 0–1
	perItem   int64   // bytes per episode, or the movie's; 0 when unknown
	perHour   float64 // GB per hour; 0 without a runtime
	over      bool    // over max_gb_per_hour
	outlier   string  // the size_outlier warning; "" when plausible
	itemCount int
}

// Size plausibility: under minItemBytes an episode or film is implausible
// (unless it's very short), and so is a size per hour under its
// resolution's floor or over maxGBPerHourPlausible.
const (
	minItemBytes          = 60_000_000
	shortRuntimeMinutes   = 10
	maxGBPerHourPlausible = 60
)

var minGBPerHour = map[string]float64{"2160p": 0.3, "1080p": 0.15, "720p": 0.08}

const minGBPerHourOther = 0.04

func (ev evaluator) size() sizeCheck {
	s := sizeCheck{fit: 0.6} // unknown: neither good nor bad
	items := 1
	if ev.tv {
		items = ev.f.Episodes
		if items == 0 && ev.coverage() == coverEpisode {
			items = 1
		}
	}
	if items <= 0 || ev.f.SizeBytes <= 0 {
		return s
	}
	s.itemCount = items
	s.perItem = ev.f.SizeBytes / int64(items)
	runtime := ev.f.RuntimeMinutes

	floor, ok := minGBPerHour[ev.r.Resolution]
	if !ok {
		floor = minGBPerHourOther
	}
	if runtime > 0 {
		s.perHour = float64(s.perItem) / 1e9 / (float64(runtime) / 60)
	}
	switch {
	case s.perItem < minItemBytes && (runtime == 0 || runtime >= shortRuntimeMinutes),
		runtime > 0 && s.perHour < floor:
		s.outlier = "Unusually small · may be fake or poor quality"
		if resolutionRank(ev.r.Resolution) >= 0 {
			s.outlier = fmt.Sprintf("Unusually small for %s · may be fake or poor quality", ev.r.Resolution)
		}
	case runtime > 0 && s.perHour > maxGBPerHourPlausible:
		s.outlier = "Unusually large for its runtime"
	}
	if runtime == 0 {
		if s.outlier != "" {
			s.fit = 0
		}
		return s
	}
	s.over = ev.p.MaxGBPerHour > 0 && s.perHour > ev.p.MaxGBPerHour
	s.fit = 0
	if !s.over && s.outlier == "" {
		s.fit = 1
	}
	return s
}

// reasons are the positive phrases, most important first: what it covers,
// its quality (and group), what the profile prefers, seeders, size, and
// whether it matches the whole profile.
func (ev evaluator) reasons(size sizeCheck) []string {
	out := []string{}
	if c := ev.coverageReason(); c != "" {
		out = append(out, c)
	}

	rank := resolutionRank(ev.r.Resolution)
	fits := rank >= ev.preferredRank()
	label := qualityLabel(ev.r)
	switch {
	case ev.preferredGroup() && label != "":
		out = append(out, fmt.Sprintf("%s from %s, a group you prefer", label, ev.r.Group))
	case ev.preferredGroup():
		out = append(out, fmt.Sprintf("From %s, a group you prefer", ev.r.Group))
	case fits && label != "":
		out = append(out, label)
	}

	matches := fits && size.outlier == "" && !size.over
	for _, pref := range ev.preferences() {
		if pref.has && pref.reason != "" {
			out = append(out, pref.reason)
		}
		matches = matches && pref.has
	}

	if ev.f.Seeders >= seedersReason {
		out = append(out, plural(ev.f.Seeders, "seeder", "seeders"))
	}
	if size.perHour > 0 && size.outlier == "" && !size.over {
		switch {
		case !ev.tv:
			out = append(out, fmt.Sprintf("%s (%s GB/h)", formatBytes(size.perItem), strconv.FormatFloat(size.perHour, 'f', 1, 64)))
		case size.itemCount == 1:
			out = append(out, formatBytes(size.perItem))
		default:
			out = append(out, formatBytes(size.perItem)+" per episode")
		}
	}
	if matches {
		out = append(out, "matches your quality profile")
	}
	return out
}

// qualityLabel is "2160p WEB-DL", "1080p" or "" when the name gives no
// resolution.
func qualityLabel(r release.Release) string {
	if resolutionRank(r.Resolution) < 0 {
		return ""
	}
	if r.Source == release.Unknown {
		return r.Resolution
	}
	return r.Resolution + " " + r.Source
}

func (ev evaluator) coverageReason() string {
	f := ev.f
	if !ev.tv {
		return ""
	}
	switch {
	case f.MissingTotal == 2 && f.MissingCovered >= 2:
		return "Covers both missing episodes"
	case ev.coversMissingRun():
		return fmt.Sprintf("Covers all %d missing episodes", f.MissingTotal)
	case f.MissingTotal > 2 && f.MissingCovered >= 2:
		return fmt.Sprintf("Covers %d of %d missing episodes", f.MissingCovered, f.MissingTotal)
	case f.Scope == ScopeSeason && f.CoversRequested && f.Seasons <= 1:
		if f.Season == 0 {
			return "All specials"
		}
		return fmt.Sprintf("Whole season %d", f.Season)
	}
	return ""
}

// notes: the scene numbering, how much a big pack covers, and the
// episodes it would replace.
func (ev evaluator) notes() []string {
	out := []string{}
	if !ev.tv {
		return out
	}
	f := ev.f
	if f.SceneNote != "" {
		out = append(out, f.SceneNote)
	}
	if f.Seasons > 1 && f.Episodes > 0 {
		all := ""
		if f.ShowSeasons > 0 && f.Seasons >= f.ShowSeasons {
			all = "all "
		}
		out = append(out, fmt.Sprintf("Covers %s%d seasons, %s", all, f.Seasons, plural(f.Episodes, "episode", "episodes")))
	}
	// Auto-apply waits when a proposal replaces a file (PLAN §6
	// "Assignment"), so the review does ask.
	switch {
	case f.Have == 1:
		out = append(out, "Includes 1 episode you already have; the mapping review asks before replacing it")
	case f.Have > 1:
		out = append(out, fmt.Sprintf("Includes %d episodes you already have; the mapping review asks before replacing them", f.Have))
	}
	return out
}

// formatBytes writes decimal units, as the web does: "15.6 GB", "312 MB".
func formatBytes(n int64) string {
	if n >= 1e9 {
		return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + " GB"
	}
	return strconv.FormatInt(int64(math.Round(float64(n)/1e6)), 10) + " MB"
}

// formatNumber writes 8 as "8" and 7.5 as "7.5".
func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// plural writes "1 seeder", "312 seeders", "1,204 seeders".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return thousands(n) + " " + many
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
