package identify

import (
	"fmt"
	"sort"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// MovieInfo is what RankMovieFiles knows about the film.
type MovieInfo struct {
	Title      string
	Year       int
	RuntimeMin int // 0 when unknown
}

// MovieCandidate is a video that may be the film. Plausible candidates are
// medium or high: a torrent with one plausible candidate has it high, so a
// movie job can match it; several plausible ones go to review.
type MovieCandidate struct {
	Path       string
	Size       int64
	Score      float64 // 0..1: size, title and year together
	Confidence Confidence
	Reason     string
}

// Weights of a movie candidate's score.
const (
	movieSizeWeight  = 0.5
	movieTitleWeight = 0.3
	movieYearWeight  = 0.2
	// bigShare is how large another video must be, against the largest,
	// to make the torrent a collection (or a multi-version pack).
	bigShare = 0.5
	// minMBPerMinute is the smallest plausible size of a whole film.
	minMBPerMinute = 1
)

// RankMovieFiles ranks a torrent's videos as candidates for a film, best
// first. Samples, trailers and other extras are left out, but never the
// largest video when its size suits the runtime, and never every video:
// when nothing else is left, every video is ranked and none is high. When
// the top video is the only large one it's high confidence; a collection
// pack (several large videos) gives several candidates and none is high,
// so the choice goes to review.
func RankMovieFiles(files []TorrentFile, m MovieInfo) []MovieCandidate {
	ix := newIndex(&ShowCatalog{Title: m.Title, Year: m.Year}, time.Time{})
	var videos []TorrentFile
	var largest TorrentFile
	for _, f := range files {
		if ClassOf(f.Path) == ClassVideo {
			videos = append(videos, f)
			if f.Size > largest.Size {
				largest = f
			}
		}
	}
	var kept []TorrentFile
	for _, f := range videos {
		if f == largest && plausibleFilm(f, m) && !movieExtraFolder(f, ix) || !movieExtra(f, ix, m) {
			kept = append(kept, f)
		}
	}
	fallback := len(kept) == 0
	if fallback {
		kept = videos
	}

	type scored struct {
		MovieCandidate
		title, year float64
	}
	var cands []scored
	var top int64
	for _, f := range kept {
		dirs, name, _ := splitPath(f.Path)
		title, year := 0.0, 0.5
		for _, s := range append([]string{name}, reverse(dirs)...) {
			t, y := movieNameMatch(s, ix, m.Year)
			title, year = max(title, t), max(year, y)
			if t > 0 {
				break // the nearest name that gives the title decides the year too
			}
		}
		cands = append(cands, scored{MovieCandidate: MovieCandidate{Path: f.Path, Size: f.Size}, title: title, year: year})
		top = max(top, f.Size)
	}
	for i := range cands {
		c := &cands[i]
		size := 0.0
		if top > 0 {
			size = float64(c.Size) / float64(top)
		}
		c.Score = movieSizeWeight*size + movieTitleWeight*c.title + movieYearWeight*c.year
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		return cands[i].Size > cands[j].Size
	})
	if len(cands) == 0 {
		return nil
	}

	big := 0
	for _, c := range cands[1:] {
		if float64(c.Size) >= bigShare*float64(cands[0].Size) {
			big++
		}
	}
	out := make([]MovieCandidate, len(cands))
	for i, c := range cands {
		mc := c.MovieCandidate
		match := movieMatchNote(c.title, c.year)
		switch {
		case i == 0 && big == 0:
			mc.Confidence = ConfidenceHigh
			mc.Reason = fmt.Sprintf("Largest video (%s)%s", humanSize(c.Size), match)
		case i == 0:
			mc.Confidence = ConfidenceMedium
			mc.Reason = fmt.Sprintf("Best of %d large videos (%s)%s", big+1, humanSize(c.Size), match)
		case float64(c.Size) >= bigShare*float64(cands[0].Size):
			mc.Confidence = ConfidenceMedium
			mc.Reason = fmt.Sprintf("Another large video (%s)%s", humanSize(c.Size), match)
		default:
			mc.Confidence = ConfidenceLow
			mc.Reason = fmt.Sprintf("Smaller video (%s)%s", humanSize(c.Size), match)
		}
		if fallback {
			mc.Confidence = capConfidence(mc.Confidence, ConfidenceMedium)
			mc.Reason = "Only extras-like videos; " + mc.Reason
		}
		mc.Reason = clipReason(mc.Reason)
		out[i] = mc
	}
	return out
}

// plausibleFilm reports whether a video is large enough for the film's
// runtime (any size when the runtime is unknown).
func plausibleFilm(f TorrentFile, m MovieInfo) bool {
	return m.RuntimeMin <= 0 || f.Size >= int64(m.RuntimeMin)*minMBPerMinute<<20
}

// movieExtraFolder reports a video in an extras folder ("Featurettes",
// "Sample").
func movieExtraFolder(f TorrentFile, ix *catalogIndex) bool {
	dirs, _, _ := splitPath(f.Path)
	for _, d := range dirs {
		if t, _ := ix.dirExtra(d); t != "" {
			return true
		}
	}
	return false
}

// movieExtra reports samples, trailers and other extras: by folder, by a
// marker in the name (the film's own title never counts: "Behind the
// Candelabra", "The Interview"), or by a size too small for the runtime.
func movieExtra(f TorrentFile, ix *catalogIndex, m MovieInfo) bool {
	if movieExtraFolder(f, ix) {
		return true
	}
	_, name, _ := splitPath(f.Path)
	var words []string
	for _, t := range stripLead(token.Tokenize(name)) {
		if !t.IsTag() && t.Kind != token.Dash {
			words = append(words, normWords(t.Text)...)
		}
	}
	if t, _ := textExtra(ix.maskTitles(words), false); t != "" {
		return true
	}
	return !plausibleFilm(f, m) && f.Size > 0
}

// movieNameMatch scores a name against the film: title 1 when it starts
// with the title, 0.5 when it shares a word; year 1 when it gives the
// film's year, 0 when it gives another, 0.5 when it gives none.
func movieNameMatch(name string, ix *catalogIndex, year int) (title, yr float64) {
	toks := stripLead(token.Tokenize(name))
	if ix.titlePrefix(toks) > 0 {
		title = 1
	} else {
		for _, w := range normWords(name) {
			if significant(w) && ix.titleWords[w] {
				title = 0.5
				break
			}
		}
	}
	yr = 0.5
	for _, t := range toks {
		if t.Kind == token.Year {
			if year > 0 && t.N == year {
				return title, 1
			}
			yr = 0
		}
	}
	return title, yr
}

func movieMatchNote(title, year float64) string {
	switch {
	case title == 1 && year == 1:
		return "; title and year match"
	case title == 1:
		return "; title matches"
	case year == 0:
		return "; another year in the name"
	}
	return ""
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", n>>10)
}
