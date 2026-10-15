package identify

import (
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// fileInfo is what Classify learns about one file, before and after
// matching.
type fileInfo struct {
	path  string
	size  int64
	class FileClass
	dirs  []string
	dir   string // folder path; "" at the torrent's root
	name  string // file name without its extension
	toks  []token.Token
	body  []token.Token // toks without the leading [Group]

	titleLen int // tokens of body that spell a known show title

	extra    ExtraType
	extraWhy string // the marker that made it an extra

	hasMarker   bool // an explicit episode marker: SxxEyy, NxNN, Episode, Date, "S2 - 05"
	cands       []candidate
	titleText   []string    // the episode's name as the file gives it
	region      titleRegion // the show title before the episode number
	special     bool        // S00, E00, OVA, "28.5", a Specials folder
	specialText string      // as written: "S04E00", "OVA 2", "28.5"
	half        bool
	version     int
	fileSeason  int // a season the file name gives without an episode ("Show.S02.NCOP"); -1 none

	// Matching
	prop      Proposal
	targetSet string // the target episode IDs, for dedupe
	suffixWhy string // why bare-number readings were refused
}

type candKind uint8

const (
	candExplicit candKind = iota // season and episodes: "S01E05", "1x05", "S2 - 05"
	candNumber                   // episodes without a season: "- 05", "E1088", "01 - Pilot"
	candDate                     // "2024.03.14"
	candSpecial                  // "OVA 2", "Special"
	candConcat                   // "101" as S01E01
)

type candidate struct {
	kind   candKind
	season int
	eps    []int
	date   string
	text   string // as written, for reasons
	method Method
	word   string // candSpecial: "ova", "oad", "sp", "special"
}

type regionMatch uint8

const (
	regionEmpty       regionMatch = iota // no title before the episode
	regionExact                          // the show's title (or the start of it)
	regionPrefixExtra                    // the show's title and more words: "Dr. Stone - New World"
	regionPartial                        // shares a word with the show's title
	regionNone                           // shares nothing
)

type titleRegion struct {
	match regionMatch
	words []string
	extra []string // regionPrefixExtra: the words after the show's title
}

// analyze reads one file: its class, its tokens and its candidates.
func analyze(f TorrentFile, ix *catalogIndex) *fileInfo {
	dirs, name, _ := splitPath(f.Path)
	fi := &fileInfo{
		path: f.Path, size: f.Size, class: ClassOf(f.Path),
		dirs: dirs, dir: strings.Join(dirs, "/"), name: name, fileSeason: -1,
	}
	if fi.class != ClassVideo && fi.class != ClassSubtitle {
		fi.extra = extraOfClass(fi.class)
		return fi
	}
	fi.toks = token.Tokenize(name)
	fi.body = fi.toks
	for len(fi.body) > 0 && fi.body[0].Kind == token.Group {
		fi.body = fi.body[1:]
	}
	fi.titleLen = ix.titlePrefix(fi.body)
	fi.extract(ix)
	if fi.class == ClassVideo {
		fi.findExtra(ix)
	}
	return fi
}

func isEpisodeMarker(t token.Token) bool {
	switch t.Kind {
	case token.SxxEyy, token.NxNN, token.Episode, token.Date:
		return true
	case token.Season:
		return t.Season == t.LastSeason
	}
	return false
}

// extract finds the file's candidates, the episode title it gives and the
// show title before them.
func (fi *fileInfo) extract(ix *catalogIndex) {
	body := fi.body
	k := fi.titleLen
	from := k
	for m := k; m < len(body); m++ {
		t := body[m]
		if !isEpisodeMarker(t) {
			continue
		}
		switch t.Kind {
		case token.SxxEyy, token.NxNN:
			method := MethodSxxEyy
			if t.Kind == token.NxNN {
				method = MethodNxNN
			}
			fi.hasMarker = true
			fi.version = t.Ver
			eps := nonZero(t.Eps)
			if len(eps) < len(t.Eps) || t.Season == 0 {
				fi.special, fi.specialText = true, t.Text
			}
			if t.Season == 0 {
				method = MethodSpecialNumber
			}
			if len(eps) > 0 {
				fi.cands = append(fi.cands, candidate{kind: candExplicit, season: t.Season, eps: eps, text: t.Text, method: method})
			}
			fi.finish(ix, m, m+1)
			return
		case token.Episode:
			fi.hasMarker = true
			fi.version = t.Ver
			fi.cands = append(fi.cands, candidate{kind: candNumber, eps: t.Eps, text: t.Text})
			fi.finish(ix, m, m+1)
			return
		case token.Date:
			fi.hasMarker = true
			fi.cands = append(fi.cands, candidate{kind: candDate, date: t.Date, text: t.Text})
			fi.finish(ix, m, m+1)
			return
		case token.Season:
			j := m + 1
			for j < len(body) && body[j].Kind == token.Dash {
				j++
			}
			if j < len(body) && !body[j].Bracket {
				n := body[j]
				switch {
				case n.Kind == token.Num && n.Half:
					fi.special, fi.half, fi.specialText = true, true, n.Text
					fi.finish(ix, m, j+1)
					return
				case n.Kind == token.Num && n.N > 0:
					fi.hasMarker = true
					fi.version = n.Ver
					fi.cands = append(fi.cands, candidate{kind: candExplicit, season: t.Season, eps: []int{n.N},
						text: t.Text + " - " + n.Text, method: MethodSeasonNumber})
					fi.finish(ix, m, j+1)
					return
				case n.Kind == token.Range:
					if eps := expandRange(n.Lo, n.Hi); eps != nil {
						fi.hasMarker = true
						fi.cands = append(fi.cands, candidate{kind: candExplicit, season: t.Season, eps: eps,
							text: t.Text + " - " + n.Text, method: MethodSeasonNumber})
						fi.finish(ix, m, j+1)
						return
					}
				}
			}
			// A season without an episode: "Show.S02.NCOP", "Show S2 [OVA]".
			fi.fileSeason = t.Season
			from = m + 1
		}
		break
	}
	fi.noMarker(ix, from)
}

// finish records the show title before the marker at m and the episode
// title after it.
func (fi *fileInfo) finish(ix *catalogIndex, m, after int) {
	fi.region = ix.regionOf(regionWords(fi.body[:m]))
	fi.titleText = titleWordsFrom(fi.body, after)
}

// noMarker reads a name without an explicit episode marker: a special
// word, an in-between "28.5", a bare number, or only a title.
func (fi *fileInfo) noMarker(ix *catalogIndex, from int) {
	body := fi.body
	k := fi.titleLen
	j := fi.numberIndex(from)

	// A special word before the episode number (or anywhere, without one):
	// "OVA 2", "OAD - 01", "SP1", "Christmas Special". After the number it's
	// the episode's name: "Show - 05 - The Special One".
	specialEnd := len(body)
	if j >= 0 {
		specialEnd = j
	}
	for i := from; i < specialEnd; i++ {
		t := body[i]
		if t.Kind != token.Word || t.Bracket {
			continue
		}
		m := reSpecialWord.FindStringSubmatch(t.Norm)
		if m == nil {
			continue
		}
		fi.special = true
		c := candidate{kind: candSpecial, word: m[1], text: t.Text, method: MethodSpecialNumber}
		after := i + 1
		if m[2] != "" {
			c.eps = []int{atoiSafe(m[2])}
		} else {
			j := i + 1
			for j < len(body) && body[j].Kind == token.Dash {
				j++
			}
			if j < len(body) && body[j].Kind == token.Num && !body[j].Bracket && body[j].N > 0 && !body[j].Half {
				c.eps = []int{body[j].N}
				c.text += " " + body[j].Text
				after = j + 1
			}
		}
		fi.specialText = c.text
		fi.cands = append(fi.cands, c)
		end := i
		if k > 0 && k < i {
			end = k // "Extras.Christmas.Special": the title is "Extras"
		}
		fi.region = ix.regionOf(regionWords(body[:end]))
		if len(c.eps) > 0 {
			fi.titleText = titleWordsFrom(body, after)
		} else {
			// "Christmas Special": the whole name after the show title.
			fi.titleText = titleWordsFrom(body, max(k, from))
		}
		return
	}

	for i := from; i < specialEnd; i++ {
		if t := body[i]; t.Kind == token.Num && t.Half && !t.Bracket {
			fi.special, fi.half, fi.specialText = true, true, t.Text
			fi.region = ix.regionOf(regionWords(body[:i]))
			return
		}
	}

	if j < 0 {
		end := k
		if end == 0 {
			// An unknown title runs to the first dash: "Bluey - The Sign".
			for i, t := range body {
				if t.Kind == token.Dash {
					end = i
					break
				}
			}
		}
		end = max(end, from)
		fi.region = ix.regionOf(regionWords(body[:end]))
		fi.titleText = titleWordsFrom(body, end)
		return
	}

	n := body[j]
	var eps []int
	if n.Kind == token.Range {
		eps = expandRange(n.Lo, n.Hi)
	} else if n.N > 0 {
		eps = []int{n.N}
	}
	if eps == nil {
		return
	}
	fi.version = n.Ver
	fi.cands = append(fi.cands, candidate{kind: candNumber, eps: eps, text: n.Text})
	if n.Kind == token.Num && len(n.Text) >= 3 && len(n.Text) <= 4 && n.N%100 > 0 && n.N >= 100 && !ix.absPreferred() {
		fi.cands = append(fi.cands, candidate{kind: candConcat, season: n.N / 100, eps: []int{n.N % 100}, text: n.Text})
	}
	fi.region = ix.regionOf(regionWords(body[:j]))
	fi.titleText = titleWordsFrom(body, j+1)
}

// numberIndex finds the token that numbers the episode in a name without a
// marker, or -1:
//   - a number after a dash: "Show - 05", "Show - The Final Season - 01";
//   - a number opening the name: "01 - Pilot", "101 Pilot";
//   - a number right after the show's title: "One Piece 001";
//   - a number alone in brackets: "Spy x Family [12][1080p]".
func (fi *fileInfo) numberIndex(from int) int {
	body := fi.body
	isNum := func(t token.Token) bool {
		return (t.Kind == token.Num && t.N > 0 && !t.Half || t.Kind == token.Range) && !t.Bracket
	}
	for i := max(from, 1); i < len(body); i++ {
		if body[i-1].Kind == token.Dash && isNum(body[i]) {
			return i
		}
	}
	if from == 0 && fi.titleLen == 0 && len(body) > 0 && isNum(body[0]) {
		return 0
	}
	if k := max(fi.titleLen, from); k > 0 && k < len(body) && isNum(body[k]) {
		return k
	}
	for i := from; i < len(body); i++ {
		t := body[i]
		if t.Kind != token.Num || !t.Bracket || len(t.Text) > 3 || t.N == 0 {
			continue
		}
		alone := (i == 0 || !body[i-1].Bracket) && (i+1 == len(body) || !body[i+1].Bracket || body[i+1].IsTag())
		if alone {
			return i
		}
	}
	return -1
}

// findExtra decides whether a video is an extra: a marker in its name (or,
// after an episode marker, in its tags), or an extras folder.
func (fi *fileInfo) findExtra(ix *catalogIndex) {
	var dirType ExtraType
	var dirWhy string
	for d := len(fi.dirs) - 1; d >= 0; d-- {
		if t, why := ix.dirExtra(fi.dirs[d]); t != "" {
			dirType, dirWhy = t, why
			break
		}
	}

	var t ExtraType
	var why string
	if fi.hasMarker {
		// After "S01E05" the words are the episode's name; only the tags
		// at the end can say "sample".
		tail := -1
		for i, tok := range fi.body {
			if tok.IsTag() {
				tail = i
				break
			}
		}
		if tail >= 0 {
			t, why = textExtra(wordsOf(fi.body[tail:]), true)
		}
		if t == "" && len(fi.toks) > 0 {
			if last := fi.toks[len(fi.toks)-1]; last.Kind == token.Word && (last.Norm == "sample" || reNCWord.MatchString(last.Norm)) {
				t, why = textExtra([]string{last.Norm}, true)
			}
		}
	} else {
		start := fi.titleLen
		var words []string
		for _, tok := range fi.body[start:] {
			if tok.IsTag() || tok.Kind == token.Dash {
				continue
			}
			words = append(words, normWords(tok.Text)...)
		}
		t, why = textExtra(words, false)
		if t == "" {
			// "Show - OP1", "Show - ED": a short marker right after a dash.
			for i := start; i+1 < len(fi.body); i++ {
				if fi.body[i].Kind == token.Dash && fi.body[i+1].Kind == token.Word {
					if st := shortExtra(fi.body[i+1].Norm); st != "" {
						t, why = st, fi.body[i+1].Text
					}
					break
				}
			}
		}
	}
	if t == "" && dirType != "" {
		t, why = dirType, dirWhy
		if t == ExtraFont || t == ExtraImage {
			t = ExtraOther
		}
	}
	if t != "" {
		fi.extra, fi.extraWhy = t, why
	}
}

// dirExtra reports a folder of extras ("Extras", "Featurettes", "Sample",
// "NC"), ignoring the show's title in it.
func (ix *catalogIndex) dirExtra(seg string) (ExtraType, string) {
	key := ix.dirKey(seg)
	if t, ok := markerDirs[key]; ok {
		return t, seg
	}
	return "", ""
}

// dirKey is a folder name's words without the show's title, years and
// quality tags: "Trailer.Park.Boys.S01-S12.COMPLETE.1080p" → "s01 s12
// complete".
func (ix *catalogIndex) dirKey(seg string) string {
	toks := token.Tokenize(seg)
	for len(toks) > 0 && toks[0].Kind == token.Group {
		toks = toks[1:]
	}
	toks = toks[ix.titlePrefix(toks):]
	var words []string
	for _, t := range toks {
		if t.IsTag() || t.Kind == token.Dash || t.Kind == token.Year {
			continue
		}
		words = append(words, normWords(t.Text)...)
	}
	return strings.Join(words, " ")
}

// titlePrefix returns how many of toks spell a known title of the show:
// all of one ("Law.and.Order.SVU"), or the start of one up to a dash or an
// episode marker ("Frieren - 05").
func (ix *catalogIndex) titlePrefix(toks []token.Token) int {
	best := 0
	for _, title := range ix.titles {
		w, i := 0, 0
		for i < len(toks) && w < len(title) {
			t := toks[i]
			if t.Kind == token.Dash && w > 0 {
				i++
				continue
			}
			if t.Bracket || t.Kind == token.Dash {
				break
			}
			tw := normWords(t.Text)
			if len(tw) == 0 {
				i++
				continue
			}
			if w+len(tw) > len(title) || !equalWords(tw, title[w:w+len(tw)]) {
				break
			}
			w += len(tw)
			i++
		}
		if w == len(title) && i > best {
			best = i
		}
	}
	if best > 0 {
		return best
	}

	// The start of a title: "Frieren - 05" for "Frieren: Beyond Journey's
	// End". It must end where the title region does.
	for _, title := range ix.titles {
		w, i := 0, 0
		for i < len(toks) && w < len(title) {
			tw := normWords(toks[i].Text)
			if toks[i].Bracket || toks[i].Kind == token.Dash || len(tw) == 0 ||
				w+len(tw) > len(title) || !equalWords(tw, title[w:w+len(tw)]) {
				break
			}
			w += len(tw)
			i++
		}
		if i == 0 || !hasSignificant(title[:w]) {
			continue
		}
		if i == len(toks) || toks[i].Kind == token.Dash || toks[i].Bracket || isEpisodeMarker(toks[i]) || toks[i].IsTag() {
			if i > best {
				best = i
			}
		}
	}
	return best
}

// regionOf compares the words before a file's episode number with the
// show's titles.
func (ix *catalogIndex) regionOf(words []string) titleRegion {
	r := titleRegion{words: words}
	if len(words) == 0 {
		r.match = regionEmpty
		return r
	}
	best := regionNone
	for _, title := range ix.titles {
		switch {
		case equalWords(words, title):
			r.match, r.extra = regionExact, nil
			return r
		case len(words) < len(title) && equalWords(words, title[:len(words)]) && hasSignificant(words):
			r.match, r.extra = regionExact, nil
			return r
		case len(words) > len(title) && equalWords(words[:len(title)], title):
			var extra []string
			for _, w := range words[len(title):] {
				if !ignorableTitleWords[w] {
					extra = append(extra, w)
				}
			}
			if len(extra) == 0 {
				r.match, r.extra = regionExact, nil
				return r
			}
			if best != regionPrefixExtra || len(extra) < len(r.extra) {
				best, r.extra = regionPrefixExtra, extra
			}
		}
	}
	if best == regionPrefixExtra {
		r.match = best
		return r
	}
	for _, w := range words {
		if !significant(w) {
			continue
		}
		for _, title := range ix.titles {
			for _, tw := range title {
				if tw == w {
					r.match = regionPartial
					return r
				}
			}
		}
	}
	r.match = regionNone
	return r
}

// regionWords are the words of the show-title part of a name: no group,
// tags, bracketed parts or years.
func regionWords(toks []token.Token) []string {
	var out []string
	for _, t := range toks {
		if t.Bracket || t.IsTag() || t.Kind == token.Dash || t.Kind == token.Year || t.IsMarker() {
			continue
		}
		out = append(out, normWords(t.Text)...)
	}
	return out
}

// titleWordsFrom returns the episode name a file gives from body[i] on:
// words up to the first tag or bracket, dashes skipped.
func titleWordsFrom(body []token.Token, i int) []string {
	var parts []string
	for ; i < len(body); i++ {
		t := body[i]
		if t.Kind == token.Dash {
			continue
		}
		if t.Bracket || t.IsTag() || t.IsMarker() {
			break
		}
		parts = append(parts, t.Text)
	}
	return nameWords(strings.Join(parts, " "))
}

func nonZero(eps []int) []int {
	out := make([]int, 0, len(eps))
	for _, e := range eps {
		if e > 0 {
			out = append(out, e)
		}
	}
	return out
}

func expandRange(lo, hi int) []int {
	if lo < 1 || hi <= lo || hi-lo > 200 {
		return nil
	}
	eps := make([]int, 0, hi-lo+1)
	for e := lo; e <= hi; e++ {
		eps = append(eps, e)
	}
	return eps
}

func equalWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasSignificant(words []string) bool {
	for _, w := range words {
		if significant(w) {
			return true
		}
	}
	return false
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
