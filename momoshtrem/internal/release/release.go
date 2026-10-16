// Package release reads a torrent's release name, such as
// "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", into
// the attributes torrent search filters, scores and shows (PLAN §6
// "Search", Release.attrs), plus its title, year and pack coverage.
// Subtitle ranking uses it too.
//
// It builds on identify/token, which keeps years, resolutions and codecs
// from being read as episode numbers.
package release

import (
	"encoding/json"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// Unknown is the resolution, source or codec of a name that doesn't say.
const Unknown = "unknown"

// Pack types: PLAN §6 pack.type.
const (
	PackEpisode      = "episode"       // S01E05, or one absolute number: "Frieren - 29"
	PackMultiEpisode = "multi_episode" // S01E01-E03, or an absolute range: "(29-38)"
	PackSeason       = "season"        // S01, "Season 1"
	PackMultiSeason  = "multi_season"  // S01-S04, "Seasons 1-4"
	PackComplete     = "complete"      // the whole show: "Complete Series", "Complete" without seasons
	PackUnknown      = "unknown"       // markers that don't add up: "S01E05.S02E03", "- 28.5", a Batch with no numbers
)

// Release is what a release name says. It marshals to PLAN §6
// Release.attrs; Title, Year and DisplayTitle aren't part of that.
type Release struct {
	Resolution string   // "2160p", "1080p", "720p", "576p", "480p" or Unknown
	Source     string   // "REMUX", "BluRay", "WEB-DL", "WEBRip", "WEB", "HDTV", "DVD", "CAM" (TS, TELESYNC, SCR…) or Unknown
	Codec      string   // "x265", "x264", "AV1", "other" or Unknown
	BitDepth   int      // 10 or 12; 0 when the name doesn't say
	HDR        []string // "DV", "HDR10+", "HDR10", "HDR", in that order; empty for SDR
	Audio      []string // "Atmos", "TrueHD", "DTS-HD", "DTS", "DD+", "DD", "AAC", "FLAC", "Opus", in that order
	Channels   string   // "7.1", "5.1", "2.0" or ""
	Languages  []string // audio languages: "multi", "dual" or ISO 639-1 codes, in name order
	Subs       string   // "multi", "hardcoded" or ""
	Group      string   // as written ("NTb", "SubsPlease"); YIFY and YTS.MX are "YTS"; "" when unknown
	Edition    string   // "Extended", "Director's Cut", "IMAX", "Unrated", "Uncut", "Remastered", "Theatrical", …; "" for none
	Repack     bool     // REPACK, PROPER or RERIP
	Pack       *Pack    // nil when the name gives no season or episode (a movie)

	Title        string // the name before its first tag or marker, cleaned: "Dune Part Two", "Sousou no Frieren"
	Year         int    // 0 when none
	DisplayTitle string // the name with dots and underscores as spaces where safe: "DDP5.1" and "H.265" stay
}

// Pack is what a release covers, in the release's own numbering, not
// TMDB's: "Frieren S02E01" may be TMDB's S01E29.
type Pack struct {
	Type     string `json:"type"`     // PackEpisode, PackMultiEpisode, …
	Label    string `json:"label"`    // "S01E29", "S01E01–E03", "Episode 29", "Episodes 29–38", "Season 2", "Specials", "Seasons 1–4", "Complete", "Batch"
	Seasons  []int  `json:"seasons"`  // the seasons named, ascending; empty for absolute numbers and "Complete"
	Episodes *Span  `json:"episodes"` // nil for whole seasons
	// Absolute: Episodes are absolute numbers, since the name gives no
	// season ("Frieren - 29", "One Piece (001-061)"). Search maps them to
	// TMDB episodes through identify.
	Absolute bool   `json:"-"`
	Date     string `json:"-"` // a dated episode ("2024-03-15"); its Episodes is nil
}

// Span is an episode range, both ends included.
type Span struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// attrs is Release's JSON: PLAN §6 Release.attrs, with nulls and empty
// arrays where the contract has them.
type attrs struct {
	Resolution string   `json:"resolution"`
	Source     string   `json:"source"`
	Codec      string   `json:"codec"`
	BitDepth   *int     `json:"bit_depth"`
	HDR        []string `json:"hdr"`
	Audio      []string `json:"audio"`
	Channels   *string  `json:"channels"`
	Languages  []string `json:"languages"`
	Subs       *string  `json:"subs"`
	Group      *string  `json:"group"`
	Edition    *string  `json:"edition"`
	Repack     bool     `json:"repack"`
	Pack       *Pack    `json:"pack"`
}

// MarshalJSON writes PLAN §6 Release.attrs.
func (r Release) MarshalJSON() ([]byte, error) {
	a := attrs{
		Resolution: r.Resolution, Source: r.Source, Codec: r.Codec,
		HDR: nonNil(r.HDR), Audio: nonNil(r.Audio), Languages: nonNil(r.Languages),
		Channels: nullable(r.Channels), Subs: nullable(r.Subs), Group: nullable(r.Group), Edition: nullable(r.Edition),
		Repack: r.Repack,
	}
	if r.BitDepth != 0 {
		a.BitDepth = &r.BitDepth
	}
	if r.Pack != nil {
		p := *r.Pack
		if p.Seasons == nil {
			p.Seasons = []int{}
		}
		a.Pack = &p
	}
	return json.Marshal(a)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// videoExtensions are dropped from the end of a name. ".ts" isn't one: in a
// release name it's more likely a telesync.
var videoExtensions = []string{".mkv", ".mp4", ".avi", ".m4v", ".mov", ".wmv"}

// Parse reads a release name. An empty name gives Unknown attributes and a
// nil Pack.
func Parse(name string) Release {
	name = strings.TrimSpace(name)
	for _, ext := range videoExtensions {
		if len(name) > len(ext) && strings.EqualFold(name[len(name)-len(ext):], ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	p := newParser(name)
	r := Release{Resolution: Unknown, Source: Unknown, Codec: Unknown}
	p.readTags(&r)
	r.Pack = p.pack()
	r.Group = p.group()
	r.Year = p.year()
	r.Title = p.title()
	r.DisplayTitle = p.display
	return r
}

// parser holds a name, its tokens and where its title ends.
type parser struct {
	name    string
	display string // name with safe dots and underscores as spaces; same byte offsets
	toks    []token.Token
	start   int // the first token after a leading [Group] or site prefix
	end     int // the first token after the title: the tag zone starts here
	absNum  int // index of a fansub-style trailing absolute number ("[Group] Show 1071 [1080p]"), or -1
	// epTitleFrom and epTitleTo bound the episode title after a
	// single-episode marker ("S02E03.Bit.by.a.Dead.Bee.720p"): its Title
	// Case words aren't tags.
	epTitleFrom, epTitleTo int
}

func newParser(name string) *parser {
	toks := token.Tokenize(name)
	p := &parser{name: name, toks: toks, absNum: -1}
	p.display = displayTitle(name, toks)
	p.start = leadEnd(toks)
	p.end = titleEnd(toks, p.start)

	// "[Group] Detective Conan 1120 [1080p]": with a fansub group in front,
	// a number right before the tags is the episode, not part of the title.
	if i := p.end - 1; i > p.start && toks[0].Kind == token.Group && toks[0].Bracket {
		t := toks[i]
		last := p.end == len(toks) || toks[p.end].Kind != token.Year && (toks[p.end].Bracket || toks[p.end].IsTag())
		if t.Kind == token.Num && t.N > 0 && !t.Half && !t.Bracket && last {
			p.absNum, p.end = i, i
		}
	}

	p.epTitleFrom, p.epTitleTo = p.end, p.end
	if p.end < len(toks) {
		switch toks[p.end].Kind {
		case token.SxxEyy, token.NxNN, token.Episode, token.Date:
			j := p.end + 1
			for j < len(toks) && !toks[j].Bracket && (toks[j].Kind == token.Word || toks[j].Kind == token.Num || toks[j].Kind == token.Dash) {
				j++
			}
			p.epTitleFrom, p.epTitleTo = p.end+1, j
		}
	}
	return p
}

// leadEnd skips what comes before a release's own name: a leading [Group],
// a site prefix ("www.Site.org -") and dashes.
func leadEnd(toks []token.Token) int {
	i := 0
	for i < len(toks) {
		switch t := toks[i]; {
		case i == 0 && t.Kind == token.Group, t.Kind == token.Dash:
			i++
		case t.Norm == "www" && i+3 < len(toks):
			i += 3
		default:
			return i
		}
	}
	return i
}

// titleStops are words that end a title when they follow it: pack, cut and
// repack words.
var titleStops = map[string]bool{
	"complete": true, "batch": true, "specials": true,
	"сезон": true, "сезоны": true, "сезона": true, "серии": true, "серия": true, "серий": true,
	"extended": true, "directors": true, "director's": true, "unrated": true, "uncut": true,
	"remastered": true, "imax": true, "theatrical": true, "repack": true, "proper": true, "rerip": true,
}

// titleEnd is the index of the first token after the title. The title ends
// at a marker, a resolution, a year (unless it starts the title, "1917", or
// another year follows, "Blade Runner 2049 2017"), a dash before anything
// but a word, a bracket or a pack word. A codec or source ends it only
// when nothing stronger follows: "Charlotte's.Web.2006" ends at the year.
func titleEnd(toks []token.Token, start int) int {
	weak := -1
	for i := start; i < len(toks); i++ {
		t := toks[i]
		later := i > start
		switch {
		case t.IsMarker():
			if !(t.Kind == token.Range && later && rangeWords[toks[i-1].Norm]) { // "Kill.Bill.Vol.1-2"
				return i
			}
		case t.Kind == token.Res || t.Kind == token.CRC || t.Kind == token.Group:
			if later {
				return i
			}
		case t.Kind == token.Year:
			if later && !(i+1 < len(toks) && toks[i+1].Kind == token.Year) {
				return i
			}
		case t.Kind == token.Dash:
			if i+1 == len(toks) || toks[i+1].Kind != token.Word || toks[i+1].Bracket {
				return i
			}
		case t.Bracket || t.Kind == token.Word && titleStops[t.Norm]:
			if later && seasonWordsRU[t.Norm] && i-1 > start && toks[i-1].Kind == token.Num { // "1 сезон"
				return i - 1
			}
			if later {
				return i
			}
		case t.Kind == token.Codec || t.Kind == token.Source:
			if later && weak < 0 {
				weak = i
			}
		}
	}
	if weak >= 0 {
		return weak
	}
	return len(toks)
}

// title is the cleaned text before the title's end.
func (p *parser) title() string {
	if p.start >= len(p.toks) || p.end <= p.start {
		return ""
	}
	to := len(p.name)
	if p.end < len(p.toks) {
		to = p.toks[p.end].Pos
	}
	s := p.display[p.toks[p.start].Pos:to]
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimRight(s, " -–—/.,:;([{")
}

// year is the release year: the first year after the title, else the
// start of a year range ("[2011-2019, США, …]").
func (p *parser) year() int {
	for _, t := range p.toks[p.end:] {
		if t.Kind == token.Year {
			return t.N
		}
	}
	for _, t := range p.toks[p.end:] {
		if t.Kind == token.Range && isYearRange(t) {
			return t.Lo
		}
	}
	return 0
}

func isYearRange(t token.Token) bool {
	return t.Lo >= 1900 && t.Lo <= 2099 && t.Hi >= 1900 && t.Hi <= 2099
}

// displayTitle turns a dotted scene name into words: "Dune.Part.Two.2024.
// 2160p.WEB-DL.DDP5.1.H.265-FLUX" → "Dune Part Two 2024 2160p WEB-DL
// DDP5.1 H.265-FLUX". A dot stays inside a token that needs it ("DDP5.1",
// "H.265", "2024.03.15"), inside brackets ("[YTS.MX]") and between single
// letters ("S.W.A.T"). A name with more spaces than dots and underscores
// is already readable and left alone. Each replaced byte is one byte, so
// offsets into the name still hold.
func displayTitle(name string, toks []token.Token) string {
	if strings.Count(name, " ") >= strings.Count(name, ".")+strings.Count(name, "_") {
		return name
	}
	keep := make([]bool, len(name))
	for _, t := range toks {
		if !strings.Contains(t.Text, ".") {
			continue
		}
		for i := t.Pos; i < t.Pos+len(t.Text) && i < len(name); i++ {
			keep[i] = name[i] == '.'
		}
	}
	b := []byte(name)
	depth := 0
	for i, c := range b {
		switch c {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}
		case '.', '_':
			if depth == 0 && !keep[i] && !(c == '.' && betweenInitials(name, i)) {
				b[i] = ' '
			}
		}
	}
	return string(b)
}

// betweenInitials reports a dot between two single letters: "S.W.A.T".
func betweenInitials(s string, i int) bool {
	single := func(j int, edge int) bool {
		return j >= 0 && j < len(s) && isLetter(s[j]) && (edge < 0 || edge >= len(s) || s[edge] == '.')
	}
	return single(i-1, i-2) && single(i+1, i+2)
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
