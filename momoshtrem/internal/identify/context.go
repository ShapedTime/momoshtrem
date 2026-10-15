package identify

import (
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// PackType is what a torrent holds.
type PackType string

const (
	PackSingle      PackType = "single"       // one episode (or one video)
	PackSeason      PackType = "season"       // episodes of one season
	PackMultiSeason PackType = "multi_season" // several seasons
	PackComplete    PackType = "complete"     // a complete-series pack
	PackAbsRange    PackType = "abs_range"    // a run of absolute numbers ("One Piece 1071-1075")
)

// DirMode is how a folder's bare numbers read.
type DirMode string

const (
	DirSeasonal  DirMode = "seasonal"  // numbers within the folder's (or the only) season
	DirAbsolute  DirMode = "absolute"  // absolute numbers
	DirRemap     DirMode = "remap"     // a release season TMDB doesn't have
	DirSpecials  DirMode = "specials"  // a Specials folder
	DirAmbiguous DirMode = "ambiguous" // could be either
)

// PackContext is what the torrent as a whole says about its files.
type PackContext struct {
	Type  PackType `json:"type"`
	Group string   `json:"group,omitempty"` // release group
	// DirSeason maps folders to the season their names give (0 for a
	// Specials folder).
	DirSeason map[string]int `json:"dir_season,omitempty"`
	// NameRange is a range in the torrent's or a folder's name, such as
	// "(29-38)" or "1071 ~ 1075"; zero when there's none.
	NameRange [2]int `json:"name_range"`
	// DirMode maps folders holding bare-numbered videos to how the
	// numbers read.
	DirMode map[string]DirMode `json:"dir_mode,omitempty"`
}

type dirSeasonKey struct {
	dir    string
	season int
}

// packState is the PackContext plus what matching needs from it.
type packState struct {
	PackContext
	ix         *catalogIndex
	hintSeason int                    // -1 without a hint
	hintEp     int                    // 0 without a hint
	dirSegs    map[string]string      // folder → the segment that gave its season
	folderNums map[dirSeasonKey][]int // folder and season → the bare numbers read in it
	videosIn   map[string]int         // folder → its main videos
	remapMax   map[dirSeasonKey]int
	plans      map[dirSeasonKey]*remapPlan
	videos     int // main videos: not extras
}

// buildContext reads the torrent-level context: folder seasons, ranges in
// names, the release group, and per folder the highest bare number and the
// highest episode of each release season TMDB doesn't have.
func buildContext(in Input, files []*fileInfo, ix *catalogIndex) *packState {
	st := &packState{
		PackContext: PackContext{DirSeason: map[string]int{}, DirMode: map[string]DirMode{}},
		ix:          ix,
		hintSeason:  -1,
		dirSegs:     map[string]string{},
		folderNums:  map[dirSeasonKey][]int{},
		videosIn:    map[string]int{},
		remapMax:    map[dirSeasonKey]int{},
		plans:       map[dirSeasonKey]*remapPlan{},
	}
	if in.Hint != nil {
		if in.Hint.Season != nil && *in.Hint.Season >= 0 {
			st.hintSeason = *in.Hint.Season
		}
		if in.Hint.Episode != nil && *in.Hint.Episode > 0 {
			st.hintEp = *in.Hint.Episode
		}
	}

	seenDirs := map[string]bool{}
	for _, fi := range files {
		if seenDirs[fi.dir] {
			continue
		}
		seenDirs[fi.dir] = true
		if s, seg, ok := ix.seasonOfDirs(fi.dirs); ok {
			st.DirSeason[fi.dir] = s
			st.dirSegs[fi.dir] = seg
		}
		if st.NameRange == [2]int{} {
			st.NameRange = rangeOfNames(fi.dirs)
		}
	}
	if st.NameRange == [2]int{} && len(files) > 1 {
		// A single file's torrent name is the file's own name, whose range
		// is a double episode, not a batch.
		st.NameRange = rangeOfNames([]string{in.TorrentName})
	}
	st.Group = releaseGroup(in.TorrentName, files)

	for _, fi := range files {
		if fi.class != ClassVideo || fi.extra != "" {
			continue
		}
		st.videos++
		st.videosIn[fi.dir]++
		for _, c := range fi.cands {
			switch c.kind {
			case candNumber:
				s := st.seasonFor(fi)
				st.folderNums[dirSeasonKey{fi.dir, s}] = append(st.folderNums[dirSeasonKey{fi.dir, s}], c.eps...)
				if s >= 1 && !ix.seasonExists(s) {
					key := dirSeasonKey{fi.dir, s}
					for _, e := range c.eps {
						st.remapMax[key] = max(st.remapMax[key], e)
					}
				}
			case candExplicit:
				if c.season < 1 {
					continue
				}
				for _, e := range c.eps {
					if ix.episode(c.season, e) == nil {
						key := dirSeasonKey{fi.dir, c.season}
						st.remapMax[key] = max(st.remapMax[key], e)
					}
				}
			}
		}
	}
	return st
}

// seasonFor is the season a bare number in fi reads in: the file name's
// own ("Show.S02.05"), its folder's, the hint's, or -1.
func (st *packState) seasonFor(fi *fileInfo) int {
	if fi.fileSeason >= 0 {
		return fi.fileSeason
	}
	if s, ok := st.DirSeason[fi.dir]; ok {
		return s
	}
	return st.hintSeason
}

// seasonOfDirs finds the season the deepest folder naming one gives:
// "Season 2", "Show.S02.1080p", "Specials". A folder naming several seasons
// ("S01-S05") stops the search.
func (ix *catalogIndex) seasonOfDirs(dirs []string) (season int, seg string, ok bool) {
	for d := len(dirs) - 1; d >= 0; d-- {
		if specialDirs[ix.dirKey(dirs[d])] {
			return 0, dirs[d], true
		}
		for _, t := range token.Tokenize(dirs[d]) {
			switch t.Kind {
			case token.Season:
				if t.Season != t.LastSeason {
					return 0, "", false
				}
				return t.Season, dirs[d], true
			case token.SxxEyy, token.NxNN:
				return t.Season, dirs[d], true
			}
		}
	}
	return 0, "", false
}

// rangeOfNames returns the first number range in names, deepest first.
func rangeOfNames(names []string) [2]int {
	for d := len(names) - 1; d >= 0; d-- {
		for _, t := range token.Tokenize(names[d]) {
			if t.Kind == token.Range {
				return [2]int{t.Lo, t.Hi}
			}
		}
	}
	return [2]int{}
}

// releaseGroup is the torrent name's group, else the one most videos have.
func releaseGroup(torrentName string, files []*fileInfo) string {
	for _, t := range token.Tokenize(torrentName) {
		if t.Kind == token.Group && !isSiteGroup(t.Text) {
			return t.Text
		}
	}
	counts := map[string]int{}
	for _, fi := range files {
		if fi.class != ClassVideo {
			continue
		}
		for _, t := range fi.toks {
			if t.Kind == token.Group && !isSiteGroup(t.Text) {
				counts[t.Text]++
				break
			}
		}
	}
	best, n := "", 0
	for g, c := range counts {
		if c > n || c == n && g < best {
			best, n = g, c
		}
	}
	return best
}

// folderMode decides once per folder how its bare numbers read in season
// s (one TMDB has): seasonal ("Season 2/01..12" is S02E01..E12), unless more
// of them fall in the season's absolute range than in the season itself
// ("Season 2/26..37" for a show whose S02 starts at #26). A number past the
// season's end then has no episode; it never flips the folder's others.
func (st *packState) folderMode(dir string, s int) DirMode {
	ix := st.ix
	count := ix.seasonMax[s]
	start, hasAbs := ix.absStart[s]
	seasonal, absolute := 0, 0
	for _, n := range st.folderNums[dirSeasonKey{dir, s}] {
		if n <= count {
			seasonal++
		}
		if hasAbs && n >= start && n < start+count {
			absolute++
		}
	}
	if absolute > seasonal {
		return DirAbsolute
	}
	return DirSeasonal
}

// plan returns the remap for release season s in folder dir, deciding it
// once for every file there.
func (st *packState) plan(dir string, s int) *remapPlan {
	key := dirSeasonKey{dir, s}
	if p, ok := st.plans[key]; ok {
		return p
	}
	p := st.ix.planRemap(s, st.remapMax[key])
	st.plans[key] = p
	return p
}

// finishContext fills the pack type and the folder modes once files are
// matched.
func (st *packState) finishContext(files []*fileInfo, torrentName string) {
	seasons := map[int]bool{}
	for _, fi := range files {
		if fi.class != ClassVideo || fi.extra != "" {
			continue
		}
		for _, t := range fi.prop.Targets {
			if t.Season > 0 {
				seasons[t.Season] = true
			}
		}
		for _, c := range fi.cands {
			if c.kind == candNumber {
				st.DirMode[fi.dir] = st.dirMode(fi)
				break
			}
		}
	}
	absolute := false
	for _, m := range st.DirMode {
		absolute = absolute || m == DirAbsolute
	}
	name := " " + strings.ToLower(strings.Join(normWords(torrentName), " ")) + " "
	switch {
	case st.videos <= 1:
		st.Type = PackSingle
	case absolute && st.NameRange != [2]int{} && len(seasons) > 1:
		st.Type = PackAbsRange
	case len(seasons) > 1 && (strings.Contains(name, " complete ") || strings.Contains(name, " collection ")):
		st.Type = PackComplete
	case len(seasons) > 1:
		st.Type = PackMultiSeason
	case absolute && st.NameRange != [2]int{}:
		st.Type = PackAbsRange
	default:
		st.Type = PackSeason
	}
	if len(st.DirSeason) == 0 {
		st.DirSeason = nil
	}
	if len(st.DirMode) == 0 {
		st.DirMode = nil
	}
}

// dirMode says how fi's folder reads bare numbers.
func (st *packState) dirMode(fi *fileInfo) DirMode {
	s := st.seasonFor(fi)
	switch {
	case s == 0:
		return DirSpecials
	case s >= 1 && !st.ix.seasonExists(s):
		return DirRemap
	case s >= 1:
		return st.folderMode(fi.dir, s)
	case len(st.ix.regular) == 1:
		return DirSeasonal
	case st.ix.absPreferred() || st.NameRange != [2]int{}:
		return DirAbsolute
	}
	return DirAmbiguous
}
