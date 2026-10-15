package identify

import (
	"context"
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

// Usage is what a Resolve call cost.
type Usage struct {
	Model           string
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	CostUSD         float64
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
//     "unknown" (no answer).
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
