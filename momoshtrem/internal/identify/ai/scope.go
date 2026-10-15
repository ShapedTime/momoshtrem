package ai

import (
	"github.com/shapedtime/momoshtrem/internal/identify"
)

// Scope lists the files the AI looks at after classification: the main
// videos (see IsMainVideo) the rules left without a target, below high
// confidence or in a conflict (classifier_mode auto); with all, every main
// video (classifier_mode ai_always). Nothing in scope means no call.
// MergeAI keeps high answers, so a conflict between two high answers is
// left to the user.
func Scope(rules []identify.Proposal, all bool) []string {
	conflicts := identify.Conflicts(rules)
	var out []string
	for i, p := range rules {
		if IsMainVideo(p) && (all || !p.HasTarget() || p.Confidence != identify.ConfidenceHigh || len(conflicts[i]) > 0) {
			out = append(out, p.Path)
		}
	}
	return out
}

// IsMainVideo reports whether a proposal is a video the rules read as an
// episode (or tried to): not an extra, a sample or a subtitle. The rules
// find extras by name and markers, and subtitles follow their video, so
// the AI is never asked about those.
func IsMainVideo(p identify.Proposal) bool {
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
// torrent's video files, or Scope(in.Rules, false) when it's empty.
func effectiveScope(in identify.ResolveInput) []string {
	if len(in.Scope) == 0 {
		return Scope(in.Rules, false)
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
