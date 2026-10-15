package identify

import (
	"fmt"
	"strings"
	"time"
)

// Input is a torrent's files and the show they're for.
type Input struct {
	TorrentName string
	Files       []TorrentFile
	Show        *ShowCatalog
	Hint        *Hint
	// Now is "today" for air-date checks; zero means time.Now().
	Now time.Time
}

// Hint is what the user says about a torrent when assigning it.
type Hint struct {
	Season  *int   // the season bare numbers belong to
	Episode *int   // the episode of a single-file torrent
	Text    string // free text, for the AI resolver only
}

// Result is the classifier's answer for a torrent.
type Result struct {
	// Proposals has one entry per input file, in input order.
	Proposals []Proposal
	Pack      PackContext
	// SuggestedHints are hints worth offering the user (and the AI) when
	// the rules couldn't place every file.
	SuggestedHints []string
}

// Classify maps a torrent's files to the show's episodes. It's pure: same
// input, same result.
//
// It works in two passes. First every file is read on its own: its class,
// whether it's an extra, and its candidates (an SxxEyy tag, a bare number,
// an air date, a special marker, an episode name). Then the torrent's
// context (folder seasons, ranges in names, the highest number per
// folder) decides how bare numbers read and how release seasons TMDB
// doesn't have remap, the same way for every file of a folder. Matching
// tries aired S/E, absolute numbers, air dates, the season remap, special
// numbers and episode names, and weighs them; see decide.
func Classify(in Input) Result {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	ix := newIndex(in.Show, now)

	files := make([]*fileInfo, len(in.Files))
	for i, f := range in.Files {
		files[i] = analyze(f, ix)
	}
	st := buildContext(in, files, ix)

	for _, fi := range files {
		switch {
		case fi.extra != "":
			fi.prop = Proposal{Kind: KindExtra, ExtraType: fi.extra, Reason: extraReason(fi.extra, fi.extraWhy)}
			if fi.extra == ExtraSample {
				fi.prop.Kind = KindSample
			}
		case fi.class == ClassVideo:
			st.matchVideo(fi)
		}
	}
	dedupe(files)
	st.linkSubtitles(files)
	st.finishContext(files, in.TorrentName)

	props := make([]Proposal, len(files))
	for i, fi := range files {
		p := fi.prop
		p.Path, p.Size, p.Source = in.Files[i].Path, in.Files[i].Size, SourceRules
		if !p.HasTarget() {
			p.Targets, p.Confidence, p.Method = nil, ConfidenceNone, ""
		}
		p.Reason = clipReason(p.Reason)
		props[i] = p
	}
	return Result{Proposals: props, Pack: st.PackContext, SuggestedHints: st.suggestHints(files)}
}

// matchVideo weighs a video's readings and sets its proposal.
func (st *packState) matchVideo(fi *fileInfo) {
	if st.seasonFor(fi) == 0 && !fi.special {
		fi.special, fi.specialText = true, ""
	}
	d := decide(st.options(fi))
	if d.best == nil || d.score <= 0 {
		kind := KindUnknown
		if fi.special {
			kind = KindSpecial
		}
		fi.prop = Proposal{Kind: kind, Reason: st.noMatchReason(fi)}
		return
	}

	conf := confidenceOf(d.score, d.margin)
	reason := d.best.reason
	if d.note != "" {
		reason += "; " + d.note
	}
	if conf == ConfidenceHigh {
		if note := st.titleNote(fi); note != "" {
			conf = ConfidenceMedium
			reason += "; " + note
		}
	}
	for _, ep := range d.best.eps {
		if !st.ix.aired(ep) {
			conf = capConfidence(conf, ConfidenceMedium)
			reason += fmt.Sprintf("; TMDB says it airs %s", ep.AirDate)
			break
		}
	}

	kind := KindSpecial
	for _, ep := range d.best.eps {
		if ep.Season != 0 {
			kind = KindEpisode
		}
	}
	fi.prop = Proposal{Kind: kind, Targets: targetsOf(d.best.eps), Confidence: conf, Method: d.best.method, Reason: reason}
	fi.targetSet = d.best.key()
}

// linkSubtitles gives each subtitle the episodes of the video it belongs to:
// the video whose name starts its own, the video its folder is named after
// ("Subs/<video>/2_English.srt"), or the torrent's only video. Otherwise
// it reads its own name and links to a video with the same episodes.
func (st *packState) linkSubtitles(files []*fileInfo) {
	var videos []*fileInfo
	main := 0
	for _, fi := range files {
		if fi.class == ClassVideo {
			videos = append(videos, fi)
			if isMainVideo(fi) {
				main++
			}
		}
	}
	for _, fi := range files {
		if fi.class != ClassSubtitle {
			continue
		}
		v := linkedVideo(fi, videos, main)
		if v != nil {
			p := Proposal{Kind: KindSubtitle, LinkedVideo: v.path, Method: MethodLinked}
			switch {
			case !isMainVideo(v):
				p.Reason = "Subtitle of an extra: " + baseName(v.path)
			case !v.prop.HasTarget():
				p.Reason = "Subtitle of an unmatched video: " + baseName(v.path)
			default:
				p.Targets, p.Confidence = v.prop.Targets, v.prop.Confidence
				p.Reason = "Subtitle of " + baseName(v.path)
			}
			fi.prop = p
			continue
		}

		d := decide(st.options(fi))
		if d.best == nil || d.score <= 0 {
			fi.prop = Proposal{Kind: KindSubtitle, Reason: "No video or episode for this subtitle"}
			continue
		}
		p := Proposal{Kind: KindSubtitle, Targets: targetsOf(d.best.eps), Method: d.best.method,
			Confidence: confidenceOf(d.score, d.margin), Reason: d.best.reason}
		if p.Confidence == ConfidenceHigh && st.titleNote(fi) != "" {
			p.Confidence = ConfidenceMedium
		}
		key := d.best.key()
		for _, v := range videos {
			if v.targetSet == key {
				p.LinkedVideo = v.path
				break
			}
		}
		fi.prop = p
	}
}

func linkedVideo(sub *fileInfo, videos []*fileInfo, main int) *fileInfo {
	var best *fileInfo
	for _, v := range videos {
		if v.dir == sub.dir && strings.HasPrefix(sub.name, v.name) &&
			(len(sub.name) == len(v.name) || strings.ContainsRune(". _-[(", rune(sub.name[len(v.name)]))) {
			if best == nil || len(v.name) > len(best.name) {
				best = v
			}
		}
	}
	if best != nil {
		return best
	}
	for d := len(sub.dirs) - 1; d >= 0; d-- {
		for _, v := range videos {
			if sub.dirs[d] == v.name {
				return v
			}
		}
	}
	if main == 1 {
		for _, v := range videos {
			if isMainVideo(v) {
				return v
			}
		}
	}
	return nil
}

// suggestHints offers hints when files are left without a confident
// answer.
func (st *packState) suggestHints(files []*fileInfo) []string {
	var hints []string
	add := func(h string) {
		for _, x := range hints {
			if x == h {
				return
			}
		}
		hints = append(hints, h)
	}
	for _, fi := range files {
		if !isMainVideo(fi) || fi.prop.Confidence == ConfidenceHigh {
			continue
		}
		switch {
		case fi.special:
			add("These are specials")
		case fi.suffixWhy != "":
			add(fmt.Sprintf("%q is a later season", strings.Join(fi.region.extra, " ")))
		default:
			for _, c := range fi.cands {
				if c.kind == candNumber {
					if st.ix.absPreferred() {
						add("Numbers are per season")
					} else {
						add("Numbers are absolute")
					}
				}
			}
		}
	}
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hints
}

func targetsOf(eps []*Episode) []Target {
	out := make([]Target, len(eps))
	for i, ep := range eps {
		out[i] = Target{EpisodeID: ep.ID, Season: ep.Season, Episode: ep.Number}
	}
	return out
}

// shiftDate moves a YYYY-MM-DD date by days.
func shiftDate(date string, days int) (string, bool) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", false
	}
	return t.AddDate(0, 0, days).Format("2006-01-02"), true
}
