package identify

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// Title awareness: where a name spells the show's title, whether that title
// is this show's, and which words of a name belong to it.

// siteTLDs end the "www.Site.tld" prefix some sites put before a release's
// name.
var siteTLDs = map[string]bool{
	"com": true, "org": true, "net": true, "to": true, "cc": true, "io": true, "me": true, "info": true,
	"xyz": true, "ws": true, "tv": true, "se": true, "nl": true, "ru": true, "co": true, "uk": true,
	"in": true, "li": true, "sx": true, "is": true, "eu": true, "club": true, "site": true, "lol": true,
	"biz": true, "pw": true, "am": true, "mx": true, "ag": true, "lt": true, "st": true, "unblocked": true,
}

// stripLead drops what comes before a release's own name: [Group] tags, a
// site prefix ("www.UIndex.org -", "www.Torrent9.pl -", "[ www.Torrenting.com
// ] -") and dashes.
func stripLead(toks []token.Token) []token.Token {
	for len(toks) > 0 {
		switch t := toks[0]; {
		case t.Kind == token.Group || t.Kind == token.Dash:
			toks = toks[1:]
		case t.Norm == "www" && len(toks) > 3:
			// www.<site>.<tld>, whatever the TLD, plus a second-level
			// one ("co.uk").
			toks = toks[3:]
			if len(toks) > 1 && siteTLDs[toks[0].Norm] && toks[1].Kind == token.Dash {
				toks = toks[1:]
			}
		default:
			return toks
		}
	}
	return toks
}

// isSiteGroup reports a [Group] that is a site's address.
func isSiteGroup(text string) bool {
	l := strings.ToLower(text)
	if strings.HasPrefix(l, "www.") {
		return true
	}
	if i := strings.LastIndexByte(l, '.'); i > 0 && siteTLDs[l[i+1:]] {
		return true
	}
	return false
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

type regionMatch uint8

const (
	regionEmpty       regionMatch = iota // no title before the episode
	regionExact                          // a title of the show, maybe with its country tag
	regionShort                          // the start of a longer title: "Frieren", "Law and Order"
	regionPrefixExtra                    // a title and more words: "Dr. Stone - New World"
	regionPartial                        // shares a word with a title
	regionNone                           // shares nothing
)

// titleRegion is the show-title part of a name: the words before the
// episode marker or number.
type titleRegion struct {
	match  regionMatch
	words  []string
	years  []int    // years in it ("Doctor.Who.2005"), unless a title is a year ("1923")
	extra  []string // regionPrefixExtra: the words after the title
	sequel bool     // it names a later season the title doesn't: "Season", "Part", "Final", "2nd"
}

// releaseWords describe a release, not a title; title regions skip them
// even in brackets ("[Group] Show (Uncensored) - 05").
var releaseWords = map[string]bool{
	"eng": true, "english": true, "sub": true, "subs": true, "subbed": true, "subtitle": true, "subtitles": true,
	"dub": true, "dubbed": true, "dual": true, "audio": true, "multi": true, "multiple": true, "raw": true,
	"raws": true, "uncensored": true, "uncut": true, "censored": true, "batch": true, "tv": true,
	"jpn": true, "jap": true, "japanese": true, "chs": true, "cht": true, "big5": true, "gb": true,
	"vostfr": true, "hardsub": true, "softsub": true, "remastered": true, "repack": true, "proper": true,
}

// sequelWords name a later season or part.
var sequelWords = map[string]bool{
	"season": true, "seasons": true, "part": true, "pt": true, "cour": true, "final": true, "ii": true,
	"iii": true, "iv": true, "2nd": true, "3rd": true, "4th": true, "5th": true, "6th": true,
}

// region reads a name's title part (regionWords, then regionOf). A year one
// of the show's titles uses stays a title word ("1923", "Space: 1999"),
// unless the name is a title without it: with TMDB's alias "Doctor Who
// (2005-2022)", "Doctor.Who.2005" is still "Doctor Who" and the year 2005.
func (ix *catalogIndex) region(toks []token.Token) titleRegion {
	words, years, titleYears := ix.regionWords(toks)
	r := ix.regionOf(words, years)
	if r.match == regionExact || len(titleYears) == 0 {
		return r
	}
	rest := make([]string, 0, len(words))
	moved := append([]int(nil), years...)
	for i, w := range words {
		if n, ok := titleYears[i]; ok {
			moved = append(moved, n)
			continue
		}
		rest = append(rest, w)
	}
	if alt := ix.regionOf(rest, moved); alt.match == regionExact {
		return alt
	}
	return r
}

// regionWords returns the words and years of a name's title part. Tags,
// markers and dashes are skipped; bracketed words count ("(The Final
// Season)") unless they describe the release. A year a title uses is a
// title word; titleYears maps those words' indexes to their years.
func (ix *catalogIndex) regionWords(toks []token.Token) (words []string, years []int, titleYears map[int]int) {
	for _, t := range toks {
		if t.IsTag() || t.IsMarker() || t.Kind == token.Dash {
			continue
		}
		if t.Kind == token.Year {
			if !ix.titleWords[t.Norm] {
				years = append(years, t.N)
				continue
			}
			if titleYears == nil {
				titleYears = map[int]int{}
			}
			titleYears[len(words)] = t.N
		}
		for _, w := range normWords(t.Text) {
			if !releaseWords[w] {
				words = append(words, w)
			}
		}
	}
	return words, years, titleYears
}

// regionOf compares the words of a name's title part with the show's
// titles.
func (ix *catalogIndex) regionOf(words []string, years []int) titleRegion {
	r := titleRegion{match: regionNone, words: words, years: years}
	if len(words) == 0 {
		r.match = regionEmpty
		return r
	}
	for _, w := range words {
		if !ix.titleWords[w] && (sequelWords[w] || isSmallNumber(w)) {
			r.sequel = true
		}
	}
	for _, title := range ix.titles {
		switch {
		case equalWords(words, title):
			r.match, r.extra = regionExact, nil
			return r
		case len(words) > len(title) && equalWords(words[:len(title)], title):
			var extra []string
			for _, w := range words[len(title):] {
				if !ix.countryTags[w] {
					extra = append(extra, w)
				}
			}
			if len(extra) == 0 {
				r.match, r.extra = regionExact, nil
				return r
			}
			if r.match != regionPrefixExtra || len(extra) < len(r.extra) {
				r.match, r.extra = regionPrefixExtra, extra
			}
		case len(words) < len(title) && equalWords(words, title[:len(words)]) && hasSignificant(words):
			if r.match == regionNone {
				r.match = regionShort
			}
		}
	}
	if r.match != regionNone {
		return r
	}
	for _, w := range words {
		if significant(w) && ix.titleWords[w] {
			r.match = regionPartial
			return r
		}
	}
	return r
}

// refusesNumbers reports a title part that names a season the bare number
// doesn't tell: "Dr. Stone - New World - 01", "Shingeki no Kyojin (The Final
// Season) - 17", "Sousou no Frieren 2 - 05".
func (r titleRegion) refusesNumbers() bool {
	return r.match == regionPrefixExtra || r.sequel && r.match != regionExact
}

func isSmallNumber(w string) bool {
	n, err := strconv.Atoi(w)
	return err == nil && n > 0 && n < 100
}

// yearsOK reports whether a name's years fit the show's first-air year
// (one off is fine).
func (ix *catalogIndex) yearsOK(years []int) bool {
	for _, y := range years {
		if ix.cat.Year > 0 && (y < ix.cat.Year-1 || y > ix.cat.Year+1) {
			return false
		}
	}
	return true
}

// genericFolderWords may follow the show's title in a folder name.
var genericFolderWords = map[string]bool{
	"complete": true, "series": true, "collection": true, "specials": true, "special": true, "extras": true,
	"extra": true, "featurettes": true, "featurette": true, "bonus": true, "subs": true, "subtitles": true,
	"sub": true, "sample": true, "samples": true, "fonts": true, "nc": true, "ova": true, "ovas": true,
	"oad": true, "oads": true, "batch": true, "pack": true, "week": true, "of": true, "multi": true,
	"quality": true, "and": true, "boxset": true, "box": true, "set": true, "remastered": true,
	"disc": true, "disk": true, "dvd": true, "cd": true, "vol": true, "volume": true, "book": true,
}

// genericFolderWord reports a folder word that names no show: "Disc 1",
// "Complete Series", "Specials".
func genericFolderWord(w string) bool {
	if genericFolderWords[w] {
		return true
	}
	for _, r := range w {
		if r < '0' || r > '9' {
			return false
		}
	}
	return w != ""
}

// titleNote says why a file's title part isn't this show's, or "" when it
// is (or the file and its folders name no title). A high answer needs it to
// be: "Law.and.Order.S20E05" on SVU, "Gen.V.S01E05" on The Boys and
// "Doctor.Who.2023.S01E01" on Doctor Who (2005) can't be sure.
//
// The title part is the file's own, else its nearest folder's that names
// one. It must be a title or alias of the show, with the show's country tag
// at most; the start of a longer title doesn't count. Years must be within
// one of the show's.
func (st *packState) titleNote(fi *fileInfo) string {
	ix := st.ix
	r := fi.region
	if r.match != regionEmpty || len(r.years) > 0 {
		switch {
		case r.match != regionExact && r.match != regionEmpty:
			return fmt.Sprintf("%q isn't this show's title", clipRunes(strings.Join(r.words, " "), 40))
		case r.match == regionEmpty:
			return "the name gives no title"
		case !ix.yearsOK(r.years):
			return fmt.Sprintf("the name's year isn't the show's (%d)", ix.cat.Year)
		}
		return ""
	}
	for d := len(fi.dirs) - 1; d >= 0; d-- {
		if note, present := ix.folderTitleNote(fi.dirs[d]); present {
			return note
		}
	}
	return ""
}

// folderTitleNote checks a folder name's title part like titleNote does,
// allowing words such as "Complete Series" or "Specials" after the title.
// present is false for a folder that names no title ("Season 1", "Subs").
func (ix *catalogIndex) folderTitleNote(seg string) (note string, present bool) {
	toks := stripLead(token.Tokenize(seg))
	end := len(toks)
	for i, t := range toks {
		if t.IsMarker() || t.IsTag() {
			end = i
			break
		}
	}
	words, years, _ := ix.regionWords(toks[:end])
	k := 0
	for _, title := range ix.titles {
		if len(title) <= len(words) && len(title) > k && equalWords(words[:len(title)], title) {
			k = len(title)
		}
	}
	if k == 0 {
		for _, w := range words {
			if !genericFolderWord(w) {
				return fmt.Sprintf("folder %q isn't this show's", clipRunes(seg, 40)), true
			}
		}
		if len(years) > 0 {
			return "the folder's year isn't the show's", true
		}
		return "", false
	}
	for _, w := range words[k:] {
		if !genericFolderWord(w) && !ix.countryTags[w] {
			return fmt.Sprintf("folder %q isn't this show's", clipRunes(seg, 40)), true
		}
	}
	if !ix.yearsOK(years) {
		return fmt.Sprintf("the folder's year isn't the show's (%d)", ix.cat.Year), true
	}
	return "", true
}

// maskTitles removes every occurrence of the show's titles from words, so
// "Trailer" in "Trailer Park Boys" or "Extras" in "Extras" never marks an
// extra wherever the title sits.
func (ix *catalogIndex) maskTitles(words []string) []string {
	out := append([]string(nil), words...)
	for _, title := range ix.titles {
		for i := 0; i+len(title) <= len(out); {
			if equalWords(out[i:i+len(title)], title) {
				out = append(out[:i], out[i+len(title):]...)
				continue
			}
			i++
		}
	}
	return out
}

// countryCodes maps TMDB origin countries to the tags releases use.
var countryCodes = map[string][]string{
	"US": {"us", "usa"}, "GB": {"uk", "gb"}, "AU": {"au"}, "NZ": {"nz"}, "CA": {"ca"}, "IE": {"ie"},
}

// countryWords are the country tags releases put after a title.
var countryWords = map[string]bool{"us": true, "usa": true, "uk": true, "gb": true, "au": true, "nz": true, "ca": true, "ie": true}
