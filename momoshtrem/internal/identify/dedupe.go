package identify

import (
	"sort"
	"strconv"
	"strings"
)

// Size rule for samples without "sample" in their name: a video under 5%
// of another video with the same episodes, and under 300 MB.
const (
	sampleRatio    = 0.05
	sampleMaxBytes = 300 << 20
)

// dedupe settles files that point at the same episodes, in order:
//  1. a tiny copy of a video is its sample;
//  2. copies of one episode that differ only by version or quality keep the
//     best (version, then REPACK/PROPER, then resolution, then size); the
//     others are superseded extras;
//  3. what's left of two files on one episode is a conflict: neither may be
//     high, since at least one of them is wrong.
func dedupe(files []*fileInfo) {
	groups := map[string][]*fileInfo{}
	var keys []string
	for _, fi := range files {
		if !isMainVideo(fi) || fi.targetSet == "" {
			continue
		}
		if _, ok := groups[fi.targetSet]; !ok {
			keys = append(keys, fi.targetSet)
		}
		groups[fi.targetSet] = append(groups[fi.targetSet], fi)
	}
	for _, key := range keys {
		g := groups[key]
		if len(g) < 2 {
			continue
		}
		samplesBySize(g)
		keepBestCopies(g)
	}

	props := make([]Proposal, len(files))
	for i, fi := range files {
		props[i] = fi.prop
	}
	for i, others := range Conflicts(props) {
		fi := files[i]
		fi.prop.Confidence = capConfidence(fi.prop.Confidence, ConfidenceMedium)
		fi.prop.Reason = clipRunes(fi.prop.Reason, MaxReasonRunes-56) + "; " + clipRunes(baseName(files[others[0]].path), 40) + " maps here too"
	}
}

// keepBestCopies supersedes all but the best of each set of copies in g:
// files whose names are equal without tags, versions and REPACK.
func keepBestCopies(g []*fileInfo) {
	byKey := map[string][]*fileInfo{}
	var keys []string
	for _, fi := range g {
		if fi.targetSet == "" {
			continue
		}
		if _, ok := byKey[fi.contentKey]; !ok {
			keys = append(keys, fi.contentKey)
		}
		byKey[fi.contentKey] = append(byKey[fi.contentKey], fi)
	}
	for _, key := range keys {
		copies := byKey[key]
		if len(copies) < 2 {
			continue
		}
		best := copies[0]
		for _, fi := range copies[1:] {
			if betterCopy(fi, best) {
				best = fi
			}
		}
		for _, fi := range copies {
			if fi == best {
				continue
			}
			fi.prop = Proposal{Kind: KindExtra, ExtraType: ExtraSuperseded, SupersededBy: best.path,
				Reason: "Replaced by " + copyLabel(best, fi) + ": " + clipRunes(baseName(best.path), 60)}
			fi.extra, fi.targetSet = ExtraSuperseded, ""
		}
	}
}

// betterCopy reports whether a is a better copy than b: a higher version,
// then a REPACK or PROPER, then a higher resolution, then a larger file.
func betterCopy(a, b *fileInfo) bool {
	switch {
	case version(a) != version(b):
		return version(a) > version(b)
	case a.repack != b.repack:
		return a.repack > b.repack
	case a.lines != b.lines:
		return a.lines > b.lines
	}
	return a.size > b.size
}

// copyLabel says how best beats other: "v2", "the REPACK", "the 1080p copy".
func copyLabel(best, other *fileInfo) string {
	switch {
	case version(best) != version(other):
		return "v" + strconv.Itoa(version(best))
	case best.repack != other.repack:
		return "the " + best.repackWord
	case best.lines != other.lines:
		return "the " + resolutionLabel(best.lines) + " copy"
	}
	return "a larger copy"
}

// samplesBySize turns tiny copies in g into samples.
func samplesBySize(g []*fileInfo) {
	var largest int64
	for _, fi := range g {
		if fi.targetSet != "" {
			largest = max(largest, fi.size)
		}
	}
	for _, fi := range g {
		if fi.targetSet == "" || fi.size <= 0 || fi.size >= sampleMaxBytes || float64(fi.size) >= sampleRatio*float64(largest) {
			continue
		}
		fi.prop = Proposal{Kind: KindSample, ExtraType: ExtraSample, Reason: extraReason(ExtraSample, "")}
		fi.extra, fi.targetSet = ExtraSample, ""
	}
}

func version(fi *fileInfo) int { return max(fi.version, 1) }

func isMainVideo(fi *fileInfo) bool { return fi.class == ClassVideo && fi.extra == "" }

// Conflicts finds videos that share an episode: for each such proposal's
// index, the indexes of the others. A multi-episode file is not a conflict
// by itself, and extras, samples, subtitles and unmatched files never are.
func Conflicts(props []Proposal) map[int][]int {
	owners := map[int64][]int{}
	for i := range props {
		p := &props[i]
		if (p.Kind != KindEpisode && p.Kind != KindSpecial) || !p.HasTarget() {
			continue
		}
		seen := map[int64]bool{}
		for _, t := range p.Targets {
			if !seen[t.EpisodeID] {
				seen[t.EpisodeID] = true
				owners[t.EpisodeID] = append(owners[t.EpisodeID], i)
			}
		}
	}
	out := map[int][]int{}
	for _, idx := range owners {
		if len(idx) < 2 {
			continue
		}
		for _, i := range idx {
			for _, j := range idx {
				if i != j && !containsInt(out[i], j) {
					out[i] = append(out[i], j)
				}
			}
		}
	}
	for i := range out {
		sort.Ints(out[i])
	}
	return out
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
