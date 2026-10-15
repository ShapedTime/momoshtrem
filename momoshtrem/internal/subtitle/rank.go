package subtitle

import (
	"math"
	"sort"
)

// Match says how well a subtitle's timing should fit the assigned release.
type Match string

const (
	// MatchExactRelease: made for this release (score ≥ 90).
	MatchExactRelease Match = "exact_release"
	// MatchSameSource: same source family, so the timing usually fits (≥ 60).
	MatchSameSource Match = "same_source"
	// MatchDifferentTiming: another source or cut; the timing may drift.
	MatchDifferentTiming Match = "different_timing"
	// MatchUnknown: the assigned release or the subtitle's is unknown.
	MatchUnknown Match = "unknown"
)

// Score weights. A release with the same group and source family scores in
// the 90s; the same source alone in the 60s; anything else lower.
const (
	exactScore          = 100
	exactThreshold      = 90
	sameSourceThreshold = 60
	maxInexactScore     = 99

	groupWeight         = 30
	sourceWeight        = 55
	sourceUnknownWeight = 20 // One side doesn't say: neither a match nor a mismatch
	resolutionWeight    = 3
	editionSameWeight   = 2
	editionPenalty      = 45 // Another cut adds or removes scenes: it stays below same_source
	jaccardWeight       = 10
	translationPenalty  = 20 // AI or machine translated
	downloadsWeight     = 0.6
	downloadsMaxDecades = 5
)

// Candidate is one OpenSubtitles file, ranked against the assigned release.
type Candidate struct {
	FileID            int
	LanguageCode      string
	LanguageName      string
	ReleaseName       string
	FileName          string
	Downloads         int
	HearingImpaired   bool
	MachineTranslated bool // AI or machine translated

	// Set by Rank.
	Match  Match
	Score  int    // 0–100
	Reason string // One line, e.g. "Same group (FLUX) and source (WEB)"

	rank float64 // Unrounded score, for ordering
}

// Rank scores each candidate against the assigned release name and returns
// them best first: by score, then downloads. With no assigned release every
// candidate is unknown and they're ordered by downloads.
func Rank(assigned string, candidates []Candidate) []Candidate {
	out := make([]Candidate, len(candidates))
	copy(out, candidates)

	a := ParseRelease(assigned)
	for i := range out {
		scoreCandidate(a, &out[i])
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank > out[j].rank
		}
		if out[i].Downloads != out[j].Downloads {
			return out[i].Downloads > out[j].Downloads
		}
		return out[i].FileID < out[j].FileID
	})
	return out
}

// MatchStored is the match of a stored subtitle against the assigned
// release. Subtitles that came with the torrent or the HDRezka stream are
// made for it.
func MatchStored(assigned string, sub *Subtitle) Match {
	if sub.Source == SourceTorrent || sub.Source == SourceHDRezka {
		return MatchExactRelease
	}
	a, b := ParseRelease(assigned), ParseRelease(sub.ReleaseName)
	if a.Normalized == "" || b.Normalized == "" {
		return MatchUnknown
	}
	score, _ := compare(a, b)
	return bucket(int(math.Round(score)))
}

func scoreCandidate(a Release, c *Candidate) {
	downloads := math.Min(math.Log10(1+float64(max(c.Downloads, 0))), downloadsMaxDecades) * downloadsWeight
	penalty := 0.0
	if c.MachineTranslated {
		penalty = translationPenalty
	}

	b := ParseRelease(c.ReleaseName)
	if a.Normalized == "" || b.Normalized == "" {
		c.Match, c.Score = MatchUnknown, 0
		c.rank = downloads - penalty
		if a.Normalized == "" {
			c.Reason = "Nothing assigned to compare with"
		} else {
			c.Reason = "The subtitle doesn't name its release"
		}
		return
	}

	score, reason := compare(a, b)
	if score < exactScore {
		score = math.Min(score+downloads, maxInexactScore)
	}
	score -= penalty
	c.rank = score
	c.Score = int(math.Round(math.Max(score, 0)))
	c.Match = bucket(c.Score)
	if c.MachineTranslated {
		reason += "; machine translated"
	}
	c.Reason = reason
}

// compare scores release b against the assigned release a, 0–100, and says
// why in one line.
func compare(a, b Release) (float64, string) {
	if a.Normalized == b.Normalized {
		return exactScore, "Made for this exact release"
	}

	sameGroup := a.Group != "" && a.Group == b.Group
	sameSource := a.Source != "" && a.Source == b.Source
	otherSource := a.Source != "" && b.Source != "" && a.Source != b.Source
	otherCut := a.Edition != b.Edition

	score := jaccardWeight * jaccard(a.Tokens, b.Tokens)
	if sameGroup {
		score += groupWeight
	}
	switch {
	case sameSource:
		score += sourceWeight
	case !otherSource:
		score += sourceUnknownWeight
	}
	if a.Resolution != "" && a.Resolution == b.Resolution {
		score += resolutionWeight
	}
	if otherCut {
		score -= editionPenalty
	} else {
		score += editionSameWeight
	}
	score = math.Max(0, math.Min(score, maxInexactScore))

	var reason string
	switch {
	case otherCut:
		reason = "Different cut (" + editionLabel(b.Edition) + " vs " + editionLabel(a.Edition) + "): the timing may drift"
	case sameGroup && sameSource:
		reason = "Same group (" + b.groupLabel() + ") and source (" + b.sourceLabel() + ")"
	case sameGroup && otherSource:
		reason = "Same group (" + b.groupLabel() + "), different source (" + b.sourceLabel() + " vs " + a.sourceLabel() + "): the timing may drift"
	case sameGroup:
		reason = "Same group (" + b.groupLabel() + ")"
	case sameSource && b.Group != "":
		reason = "Same source (" + b.sourceLabel() + "), different group (" + b.groupLabel() + ")"
	case sameSource:
		reason = "Same source (" + b.sourceLabel() + ")"
	case otherSource:
		reason = "Different source (" + b.sourceLabel() + " vs " + a.sourceLabel() + "): the timing may drift"
	default:
		reason = "No shared group or source: the timing may drift"
	}
	return score, reason
}

func bucket(score int) Match {
	switch {
	case score >= exactThreshold:
		return MatchExactRelease
	case score >= sameSourceThreshold:
		return MatchSameSource
	default:
		return MatchDifferentTiming
	}
}

// jaccard is |a ∩ b| / |a ∪ b| over two token sets.
func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[t] = true
	}
	inter := 0
	for _, t := range b {
		if set[t] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

var editionLabels = map[string]string{
	"extended": "Extended", "directors": "Director's Cut", "imax": "IMAX",
	"unrated": "Unrated", "remastered": "Remastered",
}

func editionLabel(edition string) string {
	if l, ok := editionLabels[edition]; ok {
		return l
	}
	return "theatrical"
}
