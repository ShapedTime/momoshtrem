package ai

import (
	"github.com/shapedtime/momoshtrem/internal/identify"
)

// UnsureFiles lists the files the AI looks at in classifier_mode auto:
// the videos the rules left without a target, below high confidence, or in
// a conflict (sharing an episode with another file). Nothing unsure means
// no call. MergeAI keeps high answers, so a conflict between two high
// answers is left to the user.
//
// Extras, samples and subtitles aren't asked about: the rules find extras
// by name and markers, and subtitles follow their video.
func UnsureFiles(rules []identify.Proposal) []string {
	conflicts := identify.Conflicts(rules)
	var out []string
	for i, p := range rules {
		if isEpisodeVideo(p) && (!p.HasTarget() || p.Confidence != identify.ConfidenceHigh || len(conflicts[i]) > 0) {
			out = append(out, p.Path)
		}
	}
	return out
}

// Videos lists every video file, extras and samples included: the scope of
// classifier_mode ai_always.
func Videos(rules []identify.Proposal) []string {
	var out []string
	for _, p := range rules {
		if identify.ClassOf(p.Path) == identify.ClassVideo {
			out = append(out, p.Path)
		}
	}
	return out
}

// isEpisodeVideo reports whether a proposal is a video the rules read as
// an episode (or tried to).
func isEpisodeVideo(p identify.Proposal) bool {
	if identify.ClassOf(p.Path) != identify.ClassVideo {
		return false
	}
	switch p.Kind {
	case identify.KindEpisode, identify.KindSpecial, identify.KindUnknown:
		return true
	}
	return false
}

// effectiveScope is the videos to ask about: in.Scope restricted to the
// torrent's video files, or UnsureFiles when it's empty.
func effectiveScope(in identify.ResolveInput) []string {
	if len(in.Scope) == 0 {
		return UnsureFiles(in.Rules)
	}
	want := make(map[string]bool, len(in.Scope))
	for _, p := range in.Scope {
		want[p] = true
	}
	var out []string
	for _, p := range filesOf(in) {
		if want[p.Path] && identify.ClassOf(p.Path) == identify.ClassVideo {
			out = append(out, p.Path)
			delete(want, p.Path)
		}
	}
	return out
}

// filesOf is the torrent's files with the rules' answers, or without
// answers (kind unknown) when the caller passed no rules.
func filesOf(in identify.ResolveInput) []identify.Proposal {
	if len(in.Rules) > 0 {
		return in.Rules
	}
	files := make([]identify.Proposal, len(in.Files))
	for i, f := range in.Files {
		files[i] = identify.Proposal{Path: f.Path, Size: f.Size, Kind: identify.KindUnknown}
	}
	return files
}
