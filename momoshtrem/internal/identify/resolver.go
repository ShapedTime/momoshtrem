package identify

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Resolver is asked about the files the rules couldn't place with high
// confidence. The AI resolver (internal/identify/ai) implements it;
// NoopResolver is the default. Its answers go through MergeAI, which
// validates them against the catalog.
type Resolver interface {
	Resolve(ctx context.Context, in ResolveInput) (ResolveOutput, error)
}

// ResolveInput is a torrent, the show and what the rules made of it.
type ResolveInput struct {
	TorrentName string
	Files       []TorrentFile
	Show        *ShowCatalog
	// Rules are the rules' proposals, one per file in Files order.
	Rules []Proposal
	// Scope lists the paths to resolve. Empty means every file the rules
	// didn't place with high confidence.
	Scope []string
	// Hint is the user's hint: "absolute numbering", "these are the OVAs".
	Hint string

	// InfoHash, ShowID and MetadataUpdatedAt key the resolver's cache,
	// with Hint and Scope; without an InfoHash and a ShowID nothing is
	// cached.
	InfoHash          string
	ShowID            int64
	MetadataUpdatedAt time.Time
	// JobID is the assignment job the call is for: usage is recorded
	// against it. Zero when there's no job (the eval).
	JobID int64
	// Model is the model to ask (the classifier_model setting); empty means
	// the resolver's default.
	Model string
}

// ResolveOutput is the resolver's answer and what it cost.
type ResolveOutput struct {
	Files []AIFile
	Usage Usage
}

// AIFile is the resolver's answer for one file (PLAN B3's schema). A target
// is a season and episodes, or an absolute number.
type AIFile struct {
	Path           string     `json:"path"`
	Kind           Kind       `json:"kind"`
	Season         *int       `json:"season"`
	Episodes       []int      `json:"episodes"`
	AbsoluteNumber *int       `json:"absolute_number"`
	Confidence     Confidence `json:"confidence"`
	Reason         string     `json:"reason"`
}

// Usage is what a Resolve call cost. The prompt is InputTokens (uncached)
// plus CacheWriteTokens plus CacheReadTokens; OutputTokens include thinking.
type Usage struct {
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
}

// Add sums two usages; the model is the first non-empty one.
func (u Usage) Add(o Usage) Usage {
	if u.Model == "" {
		u.Model = o.Model
	}
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
	u.CostUSD += o.CostUSD
	return u
}

// NoopResolver answers nothing: the rules' proposals stand.
type NoopResolver struct{}

var _ Resolver = NoopResolver{}

// Resolve returns an empty answer.
func (NoopResolver) Resolve(context.Context, ResolveInput) (ResolveOutput, error) {
	return ResolveOutput{}, nil
}

// MergeAI applies the resolver's answers to the rules' proposals and
// returns the merged proposals; rules is not modified. An answer is
// rejected, and the rules' proposal kept, when:
//   - its path isn't one of the torrent's files;
//   - the rules' proposal is high confidence (the AI never overrides it);
//   - its target isn't an episode in the catalog;
//   - its target is an episode a high-confidence rules proposal has;
//   - it's for a subtitle (subtitles follow their video) or says
//     "unknown" (no answer);
//   - it maps a file that isn't a video (a font, an .nfo) to an episode.
//
// Accepted answers keep the resolver's confidence, with the source "ai".
// Subtitles linked to a changed video follow it.
func MergeAI(rules []Proposal, ai []AIFile, cat *ShowCatalog) []Proposal {
	out := make([]Proposal, len(rules))
	index := make(map[string]int, len(rules))
	highOwner := map[int64]string{}
	for i, p := range rules {
		p.Targets = append([]Target(nil), p.Targets...)
		out[i] = p
		index[p.Path] = i
		if p.Confidence == ConfidenceHigh && (p.Kind == KindEpisode || p.Kind == KindSpecial) {
			for _, t := range p.Targets {
				highOwner[t.EpisodeID] = p.Path
			}
		}
	}
	ix := newIndex(cat, time.Time{})

	changed := map[string]bool{}
	for _, a := range ai {
		i, ok := index[a.Path]
		if !ok || changed[a.Path] {
			continue
		}
		r := out[i]
		if r.Confidence == ConfidenceHigh || r.Kind == KindSubtitle {
			continue
		}
		conf := a.Confidence
		if !conf.Valid() {
			conf = ConfidenceLow
		}
		reason := clipReason(aiReason(a.Reason))
		if reason == "" {
			reason = "The AI gave no reason"
		}

		switch a.Kind {
		case KindExtra, KindSample:
			if ClassOf(r.Path) != ClassVideo {
				continue
			}
			p := Proposal{Path: r.Path, Size: r.Size, Kind: a.Kind, ExtraType: r.ExtraType, Source: SourceAI, Method: MethodAI, Reason: reason}
			if a.Kind == KindSample {
				p.ExtraType = ExtraSample
			} else if p.ExtraType == "" {
				p.ExtraType = ExtraOther
			}
			out[i] = p
			changed[r.Path] = true
		case KindEpisode, KindSpecial:
			if ClassOf(r.Path) != ClassVideo {
				continue
			}
			eps := ix.resolveAI(a)
			if eps == nil {
				continue
			}
			taken := false
			for _, ep := range eps {
				if owner, ok := highOwner[ep.ID]; ok && owner != r.Path {
					taken = true
				}
			}
			if taken {
				continue
			}
			kind := KindSpecial
			for _, ep := range eps {
				if ep.Season != 0 {
					kind = KindEpisode
				}
			}
			out[i] = Proposal{Path: r.Path, Size: r.Size, Kind: kind, Targets: targetsOf(eps), Confidence: conf,
				Method: MethodAI, Source: SourceAI, Reason: reason}
			changed[r.Path] = true
		}
	}

	for i, p := range out {
		if p.Kind != KindSubtitle || !changed[p.LinkedVideo] {
			continue
		}
		v := out[index[p.LinkedVideo]]
		p.Targets, p.Confidence, p.Source = nil, ConfidenceNone, SourceAI
		p.Reason = "Subtitle of an extra: " + baseName(v.Path)
		if v.HasTarget() {
			p.Targets = append([]Target(nil), v.Targets...)
			p.Confidence = v.Confidence
			p.Reason = "Subtitle of " + baseName(v.Path)
		}
		p.Reason = clipReason(p.Reason)
		out[i] = p
	}
	return out
}

// Dispute is a high-confidence rules answer the AI disagrees with.
type Dispute struct {
	Path string
	// Rules are the rules' targets, which the proposal keeps.
	Rules []Target
	// AI is the AI's answer: its targets (none for an extra or a sample),
	// kind and confidence.
	AI           []Target
	AIKind       Kind
	AIConfidence Confidence
}

// FlagDisagreements is for classifier_mode ai_always, where the AI also
// sees the files the rules matched with high confidence. MergeAI never
// overrides those; FlagDisagreements finds the ones the AI answers
// differently, at medium or high confidence (another episode that exists,
// or an extra or a sample), and sends them to review instead: each keeps
// its targets and source, drops to medium confidence (so auto-apply waits
// for the user), and its reason names the AI's alternative ("AI suggests
// S00E03: a recap special"). Subtitles linked to a flagged video drop with
// it. Agreement, a low-confidence or "unknown" answer, or a target that
// doesn't exist changes nothing.
//
// Run it on MergeAI's output with the same answers. It returns the
// proposals and the disputes; merged is not modified.
func FlagDisagreements(merged []Proposal, ai []AIFile, cat *ShowCatalog) ([]Proposal, []Dispute) {
	out := make([]Proposal, len(merged))
	index := make(map[string]int, len(merged))
	for i, p := range merged {
		out[i] = p
		index[p.Path] = i
	}
	ix := newIndex(cat, time.Time{})

	var disputes []Dispute
	flagged := map[string]bool{}
	seen := map[string]bool{}
	for _, a := range ai {
		i, ok := index[a.Path]
		if !ok || seen[a.Path] {
			continue
		}
		seen[a.Path] = true
		p := out[i]
		if p.Source != SourceRules || p.Confidence != ConfidenceHigh || ClassOf(p.Path) != ClassVideo ||
			(p.Kind != KindEpisode && p.Kind != KindSpecial) {
			continue
		}
		if a.Confidence != ConfidenceHigh && a.Confidence != ConfidenceMedium {
			continue
		}
		var alt []Target
		var suggestion string
		switch a.Kind {
		case KindExtra:
			suggestion = "an extra"
		case KindSample:
			suggestion = "a sample clip"
		case KindEpisode, KindSpecial:
			eps := ix.resolveAI(a)
			if eps == nil {
				continue
			}
			alt = targetsOf(eps)
			if sameTargets(alt, p.Targets) {
				continue
			}
			labels := make([]string, len(alt))
			for k, t := range alt {
				labels[k] = fmt.Sprintf("S%02dE%02d", t.Season, t.Episode)
			}
			suggestion = strings.Join(labels, "+")
		default:
			continue
		}
		reason := "AI suggests " + suggestion
		if why := aiReason(a.Reason); why != "" {
			reason += ": " + why
		}
		disputes = append(disputes, Dispute{Path: p.Path, Rules: append([]Target(nil), p.Targets...),
			AI: alt, AIKind: a.Kind, AIConfidence: a.Confidence})
		p.Targets = append([]Target(nil), p.Targets...)
		p.Confidence, p.Reason = ConfidenceMedium, clipReason(reason)
		out[i] = p
		flagged[p.Path] = true
	}

	for i, p := range out {
		if p.Kind == KindSubtitle && flagged[p.LinkedVideo] && p.Confidence == ConfidenceHigh {
			out[i].Confidence = ConfidenceMedium
		}
	}
	return out, disputes
}

// sameTargets reports whether two target lists name the same episodes.
func sameTargets(a, b []Target) bool {
	if len(a) != len(b) {
		return false
	}
	ids := make(map[int64]bool, len(a))
	for _, t := range a {
		ids[t.EpisodeID] = true
	}
	for _, t := range b {
		if !ids[t.EpisodeID] {
			return false
		}
	}
	return true
}

// aiReason trims an AI reason. The UI labels AI reasons from the source, so
// one never starts with "AI:".
func aiReason(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"AI:", "AI -", "AI –"} {
		if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
			s = strings.TrimSpace(s[len(prefix):])
		}
	}
	return s
}

// resolveAI finds the episodes an AI answer names: a season and episodes,
// else an absolute number. It returns nil when any is missing.
func (ix *catalogIndex) resolveAI(a AIFile) []*Episode {
	switch {
	case a.Season != nil && len(a.Episodes) > 0:
		return ix.lookupAll(*a.Season, a.Episodes)
	case a.AbsoluteNumber != nil:
		return ix.lookupAbs([]int{*a.AbsoluteNumber})
	}
	return nil
}
