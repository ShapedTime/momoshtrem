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
//  1. a newer version supersedes an older one ("14v2" over "14");
//  2. a tiny copy of a video is its sample;
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
		supersede(g)
		samplesBySize(g)
	}

	props := make([]Proposal, len(files))
	for i, fi := range files {
		props[i] = fi.prop
	}
	for i, others := range Conflicts(props) {
		fi := files[i]
		fi.prop.Confidence = capConfidence(fi.prop.Confidence, ConfidenceMedium)
		fi.prop.Reason += "; " + baseName(files[others[0]].path) + " maps here too"
	}
}

// supersede turns every file of g but the newest version into an extra.
func supersede(g []*fileInfo) {
	newest := g[0]
	for _, fi := range g[1:] {
		if version(fi) > version(newest) {
			newest = fi
		}
	}
	for _, fi := range g {
		if fi == newest || version(fi) >= version(newest) {
			continue
		}
		fi.prop = Proposal{Kind: KindExtra, ExtraType: ExtraSuperseded, SupersededBy: newest.path,
			Reason: "Replaced by the newer " + versionLabel(newest) + ": " + baseName(newest.path)}
		fi.extra, fi.targetSet = ExtraSuperseded, ""
	}
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

func versionLabel(fi *fileInfo) string { return "v" + strconv.Itoa(version(fi)) }

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
