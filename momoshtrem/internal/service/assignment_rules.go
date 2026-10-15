package service

import (
	"sort"
	"unicode/utf8"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// The job contract's rules (PLAN §6 "Assignment"), as pure functions over
// proposals: how the classifier's answers become proposal rows, what
// counts as a conflict, the status a classified job settles on and when it
// applies on its own.

// maxReason is the longest reason the API returns, in characters.
const maxReason = identify.MaxReasonRunes

// maxSuggestedHints caps the "Re-check with AI" hint chips.
const maxSuggestedHints = 5

// showDrafts maps the classifier's proposals for a show's torrent onto
// proposal rows, one per file, in file order:
//   - episodes and specials with a target: matched (or specials, when every
//     target is in season 0) at high confidence, else needs_review; a file
//     in a conflict is needs_review whatever its confidence;
//   - videos without a target: unmatched;
//   - extras and samples: extras, with their extra_type; a superseded v1
//     points at its v2;
//   - subtitles: linked to their video (not listed; they follow it), or
//     extras when no video claims them.
//
// Each starts with its group's default decision.
func showDrafts(props []identify.Proposal) []library.ProposalDraft {
	conflicts := identify.Conflicts(props)
	paths := make(map[string]bool, len(props))
	for _, p := range props {
		paths[p.Path] = true
	}
	drafts := make([]library.ProposalDraft, len(props))
	for i, p := range props {
		d := library.ProposalDraft{
			FilePath:   p.Path,
			FileSize:   p.Size,
			EpisodeIDs: targetIDs(p.Targets),
			Confidence: string(p.Confidence),
			Source:     sourceOf(p.Source),
			Reason:     clipReason(p.Reason),
		}
		switch p.Kind {
		case identify.KindExtra, identify.KindSample:
			d.Kind, d.Group, d.ExtraType = library.KindExtra, library.GroupExtras, extraTypeOf(p)
			d.EpisodeIDs, d.Confidence = nil, ""
			if paths[p.SupersededBy] {
				d.SupersededByPath = p.SupersededBy
			}
		case identify.KindSubtitle:
			d.Kind, d.Group = library.KindSubtitle, library.GroupExtras
			if paths[p.LinkedVideo] {
				d.LinkedPath = p.LinkedVideo
			}
		default: // episode, special, unknown
			d.Kind = library.KindEpisode
			switch {
			case !p.HasTarget():
				d.Group = library.GroupUnmatched
			case len(conflicts[i]) > 0 || p.Confidence != identify.ConfidenceHigh:
				d.Group = library.GroupNeedsReview
			case allSpecials(p.Targets):
				d.Group = library.GroupSpecials
			default:
				d.Group = library.GroupMatched
			}
		}
		if len(d.EpisodeIDs) == 0 {
			d.Confidence = ""
		}
		d.Decision = d.Group.DefaultDecision()
		drafts[i] = d
	}
	return drafts
}

// movieDrafts maps a movie torrent's files onto proposal rows, in file
// order. The candidates RankMovieFiles found are kind movie: when one is
// plausible (high) it's matched; several plausible ones (medium) go to
// needs_review for the user to pick; the less likely ones (low) wait in
// extras. Every other file is an extra, read by the classifier (classified
// is Classify's answer for the same files); subtitles link to their video.
func movieDrafts(classified []identify.Proposal, candidates []identify.MovieCandidate) []library.ProposalDraft {
	cands := make(map[string]identify.MovieCandidate, len(candidates))
	plausible := 0
	for _, c := range candidates {
		cands[c.Path] = c
		if c.Confidence == identify.ConfidenceHigh || c.Confidence == identify.ConfidenceMedium {
			plausible++
		}
	}
	paths := make(map[string]bool, len(classified))
	for _, p := range classified {
		paths[p.Path] = true
	}

	drafts := make([]library.ProposalDraft, len(classified))
	for i, p := range classified {
		d := library.ProposalDraft{FilePath: p.Path, FileSize: p.Size, Source: library.SourceRules, Reason: clipReason(p.Reason)}
		c, isCandidate := cands[p.Path]
		switch {
		case isCandidate:
			d.Kind, d.Confidence, d.Reason = library.KindMovie, string(c.Confidence), clipReason(c.Reason)
			switch {
			case c.Confidence == identify.ConfidenceHigh && plausible == 1:
				d.Group = library.GroupMatched
			case c.Confidence == identify.ConfidenceHigh || c.Confidence == identify.ConfidenceMedium:
				d.Group = library.GroupNeedsReview
			default:
				d.Group = library.GroupExtras
			}
		case p.Kind == identify.KindSubtitle:
			d.Kind, d.Group = library.KindSubtitle, library.GroupExtras
			if paths[p.LinkedVideo] {
				d.LinkedPath = p.LinkedVideo
			}
		default:
			d.Kind, d.Group, d.ExtraType = library.KindExtra, library.GroupExtras, string(identify.ExtraOther)
			if p.Kind == identify.KindExtra || p.Kind == identify.KindSample {
				d.ExtraType = extraTypeOf(p)
			} else if identify.ClassOf(p.Path) == identify.ClassVideo {
				d.Reason = "Too small to be the film"
			}
		}
		d.Decision = d.Group.DefaultDecision()
		drafts[i] = d
	}
	return drafts
}

// videoCount counts the video files that aren't extras: episodes (matched
// or not) and movie candidates.
func videoCount(drafts []library.ProposalDraft) int {
	n := 0
	for _, d := range drafts {
		if d.Kind == library.KindEpisode || d.Kind == library.KindMovie {
			n++
		}
	}
	return n
}

func targetIDs(targets []identify.Target) []int64 {
	if len(targets) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(targets))
	seen := make(map[int64]bool, len(targets))
	for _, t := range targets {
		if !seen[t.EpisodeID] {
			seen[t.EpisodeID] = true
			ids = append(ids, t.EpisodeID)
		}
	}
	return ids
}

func allSpecials(targets []identify.Target) bool {
	for _, t := range targets {
		if t.Season != 0 {
			return false
		}
	}
	return len(targets) > 0
}

// extraTypes are the extra types the contract (and the column's CHECK)
// knows.
var extraTypes = map[identify.ExtraType]bool{
	identify.ExtraOpening: true, identify.ExtraEnding: true, identify.ExtraSample: true,
	identify.ExtraTrailer: true, identify.ExtraFeaturette: true, identify.ExtraFont: true,
	identify.ExtraInfo: true, identify.ExtraImage: true, identify.ExtraSuperseded: true,
	identify.ExtraOther: true,
}

// extraTypeOf is an extra's type; one the contract doesn't know is other.
func extraTypeOf(p identify.Proposal) string {
	if p.Kind == identify.KindSample {
		return string(identify.ExtraSample)
	}
	if !extraTypes[p.ExtraType] {
		return string(identify.ExtraOther)
	}
	return string(p.ExtraType)
}

func sourceOf(s identify.Source) string {
	if s == identify.SourceAI {
		return library.SourceAI
	}
	return library.SourceRules
}

// clipReason keeps a reason within maxReason characters.
func clipReason(s string) string {
	if utf8.RuneCountInString(s) <= maxReason {
		return s
	}
	r := []rune(s)
	return string(r[:maxReason-1]) + "…"
}

// listed returns the proposals the API lists: subtitles that belong to a
// video are left out (they're in its subtitle_count).
func listed(ps []*library.AssignmentProposal) []*library.AssignmentProposal {
	out := make([]*library.AssignmentProposal, 0, len(ps))
	for _, p := range ps {
		if p.Listed() {
			out = append(out, p)
		}
	}
	return out
}

// conflictIDs returns the proposals in a conflict: non-ignored proposals
// whose targets share an episode. A multi-episode file isn't one by
// itself; movie candidates and subtitles never are.
func conflictIDs(ps []*library.AssignmentProposal) map[int64]bool {
	owners := map[int64][]int64{}
	for _, p := range ps {
		if p.Decision == library.DecisionIgnored || p.Kind == library.KindMovie || p.Kind == library.KindSubtitle {
			continue
		}
		seen := map[int64]bool{}
		for _, id := range p.EpisodeIDs {
			if !seen[id] {
				seen[id] = true
				owners[id] = append(owners[id], p.ID)
			}
		}
	}
	out := map[int64]bool{}
	for _, ids := range owners {
		if len(ids) > 1 {
			for _, id := range ids {
				out[id] = true
			}
		}
	}
	return out
}

// JobCounts are a job's proposal counts (PLAN §6 AssignmentJob.counts).
type JobCounts struct {
	Files       int `json:"files"`        // listed proposals
	ToApply     int `json:"to_apply"`     // not ignored and maps
	Pending     int `json:"pending"`      // maps and pending: they block Apply
	Conflicts   int `json:"conflicts"`    // in a conflict: they block Apply
	NeedsReview int `json:"needs_review"` // pending and maps, or in a conflict
	Matched     int `json:"matched"`      // accepted, not Season 0 (or the movie file)
	Specials    int `json:"specials"`     // accepted, Season 0
	Ignored     int `json:"ignored"`
	Unmatched   int `json:"unmatched"` // not ignored and doesn't map
}

// countsOf counts the listed proposals ps.
func countsOf(ps []*library.AssignmentProposal) JobCounts {
	conflicts := conflictIDs(ps)
	c := JobCounts{Files: len(ps), Conflicts: len(conflicts)}
	for _, p := range ps {
		maps := p.Maps()
		pending := maps && p.Decision == library.DecisionPending
		switch p.Decision {
		case library.DecisionIgnored:
			c.Ignored++
		case library.DecisionAccepted:
			if p.FirstSeason != nil && *p.FirstSeason == 0 {
				c.Specials++
			} else {
				c.Matched++
			}
		}
		if p.Decision != library.DecisionIgnored {
			if maps {
				c.ToApply++
			} else {
				c.Unmatched++
			}
		}
		if pending {
			c.Pending++
		}
		if pending || conflicts[p.ID] {
			c.NeedsReview++
		}
	}
	return c
}

// settledStatus is the status of a classified job: needs_review while a
// proposal that maps is pending or a conflict exists, else ready.
func settledStatus(listedProposals []*library.AssignmentProposal) library.JobStatus {
	c := countsOf(listedProposals)
	if c.Pending > 0 || c.Conflicts > 0 {
		return library.JobNeedsReview
	}
	return library.JobReady
}

// autoApplies reports whether a freshly classified job applies on its own
// (with auto_apply_high_confidence on): not a dry run, no conflict,
// nothing that maps pending and all of it high, and no unmatched video.
func autoApplies(job *library.AssignmentJob, listedProposals []*library.AssignmentProposal) bool {
	if job.DryRun {
		return false
	}
	c := countsOf(listedProposals)
	if c.Conflicts > 0 || c.Pending > 0 || c.Unmatched > 0 {
		return false
	}
	for _, p := range listedProposals {
		if p.Decision != library.DecisionIgnored && p.Maps() && p.Confidence != string(identify.ConfidenceHigh) {
			return false
		}
	}
	return true
}

// groupOrder is the order groups are listed in.
var groupOrder = map[library.ProposalGroup]int{
	library.GroupNeedsReview: 0,
	library.GroupUnmatched:   1,
	library.GroupSpecials:    2,
	library.GroupMatched:     3,
	library.GroupExtras:      4,
}

// confidenceOrder puts high first and no confidence last.
var confidenceOrder = map[string]int{"high": 0, "medium": 1, "low": 2, "": 3}

// sortProposals orders proposals for the API: by group (needs_review,
// unmatched, specials, matched, extras), then first target (regular
// seasons before Specials, files without a target last), then confidence,
// then path. targetOf gives a proposal's first target.
func sortProposals(ps []ProposalView) {
	key := func(p ProposalView) [4]int {
		season, episode := 20_000, 0
		if len(p.Targets) > 0 {
			season, episode = p.Targets[0].Season, p.Targets[0].Episode
			if season == 0 {
				season = 10_000
			}
		}
		conf := ""
		if p.Confidence != nil {
			conf = *p.Confidence
		}
		return [4]int{groupOrder[library.ProposalGroup(p.Group)], season, episode, confidenceOrder[conf]}
	}
	sort.SliceStable(ps, func(i, j int) bool {
		ki, kj := key(ps[i]), key(ps[j])
		for n := range ki {
			if ki[n] != kj[n] {
				return ki[n] < kj[n]
			}
		}
		return ps[i].FilePath < ps[j].FilePath
	})
}
