// Package token splits release and file names into typed tokens: episode
// markers, quality tags, release groups, numbers and words.
//
// It knows nothing about shows. The classifier (internal/identify) and the
// release parser read meaning into the token stream; this package only makes
// sure a resolution, a year, a codec or a CRC is never mistaken for an
// episode number.
package token

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Kind is what a token is.
type Kind uint8

const (
	Word    Kind = iota // anything not recognised below
	Num                 // a bare number: "05", "1071", "28.5" (Half)
	SxxEyy              // "S01E05", "S01E05E06", "S01E05-E07", "S01E05-07", "S01 E05", "Season 1 Episode 5"
	NxNN                // "1x05", "2x12-2x13"
	Season              // "S01", "Season 1", "Series 2", "2nd Season", "S01-S05", "Season 1-5"
	Episode             // "E05", "Ep05", "Episode 5", "Ep 01-12"
	Date                // "2024.03.15", "2024-03-15", "20240315"
	Res                 // "1080p", "1920x1080", "4K"
	Codec               // video, audio and HDR tags: "x264", "H.264", "HEVC", "10bit", "DDP5.1", "AAC 2.0"
	Source              // "BluRay", "WEB-DL", "HDTV", services such as "AMZN", "NF", "CR"
	CRC                 // "[ABCD1234]"
	Year                // "2019" (1900-2099)
	Group               // "[SubsPlease]" at the start, "-NTb" at the end
	Version             // "v2" on its own
	Range               // "01-12", "001~100", "01 ~ 28"
	Dash                // " - ": a separator, kept because anime names put the episode after it
)

var kindNames = [...]string{
	Word: "word", Num: "num", SxxEyy: "sxxeyy", NxNN: "nxnn", Season: "season", Episode: "episode",
	Date: "date", Res: "res", Codec: "codec", Source: "source", CRC: "crc", Year: "year",
	Group: "group", Version: "version", Range: "range", Dash: "dash",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "kind(" + strconv.Itoa(int(k)) + ")"
}

// Token is one piece of a name.
type Token struct {
	Text    string // As written ("S01E05", "H.264"); bracket characters excluded
	Norm    string // Lowercase; "&" becomes "and"
	Kind    Kind
	Pos     int  // Byte offset of the token in the input
	Bracket bool // Inside [], (), {} or 【】

	Season     int      // SxxEyy, NxNN, Season: the season (the first of a range)
	LastSeason int      // Season: the last season of a range; equals Season otherwise
	Eps        []int    // SxxEyy, NxNN, Episode: the episodes, ranges expanded
	N          int      // Num, Year: the value; Res: the line count (1080); Version: the version
	Lo, Hi     int      // Range: first and last number
	Date       string   // Date: YYYY-MM-DD; "" for a date that can't be read ("24.03.13")
	Dates      []string // Date: every reading ("03.04.2024" is 3 April or 4 March), Date first
	Ver        int      // A version suffix ("05v2", "S01E05v2"); 0 when none
	Half       bool     // Num: written "28.5" (a recap or special between episodes)

	sep    byte // the separator before the token: 0 at the start, ' ', '.', '_', '-' or '['
	hyphen bool // the token followed a '-' inside a word ("x264-GRP")
}

// IsTag reports whether the token describes the file rather than naming it:
// a resolution, codec, source, CRC, group or version.
func (t Token) IsTag() bool {
	switch t.Kind {
	case Res, Codec, Source, CRC, Group, Version:
		return true
	}
	return false
}

// IsMarker reports whether the token says which episode a file is: an
// SxxEyy, NxNN, season, episode, date or range token.
func (t Token) IsMarker() bool {
	switch t.Kind {
	case SxxEyy, NxNN, Season, Episode, Date, Range:
		return true
	}
	return false
}

// Tokenize splits a name (a file name without its extension, a folder name
// or a torrent name) into tokens.
func Tokenize(s string) []Token {
	chunks := split(s)
	toks := make([]Token, 0, len(chunks))
	for _, c := range chunks {
		toks = append(toks, classifyChunk(c)...)
	}
	toks = merge(toks)
	markTrailingGroup(toks)
	return toks
}

// chunk is a run of characters between separators.
type chunk struct {
	text    string
	pos     int
	bracket bool
	sep     byte
	dash    bool // a standalone " - "
	tilde   bool // a standalone " ~ "
	group   bool // the leading [Group]
}

func isOpen(r rune) bool  { return r == '[' || r == '(' || r == '{' || r == '【' || r == '「' }
func isClose(r rune) bool { return r == ']' || r == ')' || r == '}' || r == '】' || r == '」' }
func isDash(r rune) bool  { return r == '-' || r == '–' || r == '—' }

// split cuts s into chunks. Dots, spaces, underscores, brackets and most
// punctuation separate; hyphens and tildes inside a word are kept ("WEB-DL",
// "S01E01-E02", "01~12") and standalone ones become separator chunks.
func split(s string) []chunk {
	var out []chunk
	depth := 0
	start := -1
	var sep byte
	flush := func(end int) {
		if start >= 0 && end > start {
			out = append(out, chunk{text: s[start:end], pos: start, bracket: depth > 0, sep: sep})
		}
		start = -1
	}

	// The leading [Group] stays whole: "[Erai-raws]", "[Anime Time]".
	i := 0
	if trimmed := strings.TrimLeft(s, " "); len(trimmed) > 0 {
		lead := len(s) - len(trimmed)
		r, size := utf8.DecodeRuneInString(trimmed)
		if r == '[' || r == '【' {
			closer := "]"
			if r == '【' {
				closer = "】"
			}
			if end := strings.Index(trimmed[size:], closer); end > 0 {
				name := strings.TrimSpace(trimmed[size : size+end])
				if name != "" {
					out = append(out, chunk{text: name, pos: lead + size, bracket: true, sep: '[', group: true})
				}
				i = lead + size + end + len(closer)
				sep = ' '
			}
		}
	}

	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case isOpen(r):
			flush(i)
			depth++
			sep = '['
		case isClose(r):
			flush(i)
			if depth > 0 {
				depth--
			}
			sep = ' '
		case isDash(r) || r == '~':
			prevSpace := i == 0 || s[i-1] == ' ' || s[i-1] == '_'
			next, _ := utf8.DecodeRuneInString(s[i+size:])
			nextSpace := i+size >= len(s) || next == ' ' || next == '_'
			switch {
			case start >= 0 && !nextSpace && (unicode.IsLetter(next) || unicode.IsDigit(next)):
				// Inside a word: "WEB-DL", "x264-GRP", "01~12".
			case prevSpace && nextSpace:
				flush(i)
				out = append(out, chunk{text: string(r), pos: i, bracket: depth > 0, sep: ' ', dash: r != '~', tilde: r == '~'})
				sep = ' '
			default:
				flush(i)
				sep = '-'
			}
		case r == '&':
			flush(i)
			out = append(out, chunk{text: "&", pos: i, bracket: depth > 0, sep: sep})
			sep = ' '
		case r == '\'' || r == '’' || r == '+' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
			if start < 0 {
				start = i
			}
		default: // space, '.', '_', ',', ':', '!', '?', ...
			flush(i)
			switch r {
			case '.', '_', ' ':
				sep = byte(r)
			default:
				sep = ' '
			}
		}
		i += size
	}
	flush(len(s))
	return out
}

// classifyChunk turns a chunk into one token, or several when it holds
// hyphenated parts that aren't one token ("x264-GRP", "AM-1").
func classifyChunk(c chunk) []Token {
	base := Token{Text: c.text, Norm: strings.ToLower(c.text), Pos: c.pos, Bracket: c.bracket, sep: c.sep}
	switch {
	case c.group:
		base.Kind = Group
		return []Token{base}
	case c.dash:
		base.Kind = Dash
		return []Token{base}
	case c.tilde:
		base.Kind = Dash
		base.Norm = "~"
		return []Token{base}
	case c.text == "&":
		base.Norm = "and"
		return []Token{base}
	}
	base.hyphen = c.sep == '-'
	if t, ok := classify(base); ok {
		return []Token{t}
	}
	if !strings.ContainsAny(c.text, "-–—~") {
		return []Token{base} // a word
	}

	// Hyphenated: classify the parts, joining known compounds ("WEB-DL",
	// "DTS-HD", "10-bit", "Blu-ray").
	type part struct {
		text string
		off  int
	}
	var parts []part
	start := -1
	for j, r := range c.text {
		if isDash(r) || r == '~' {
			if start >= 0 {
				parts = append(parts, part{c.text[start:j], start})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = j
		}
	}
	if start >= 0 {
		parts = append(parts, part{c.text[start:], start})
	}

	out := make([]Token, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		hyphen := i > 0 || base.hyphen
		sep := c.sep
		if i > 0 {
			sep = '-'
		}
		if i+1 < len(parts) {
			if kind, ok := compounds[strings.ToLower(p.text+"-"+parts[i+1].text)]; ok {
				joined := c.text[p.off : parts[i+1].off+len(parts[i+1].text)]
				out = append(out, Token{Text: joined, Norm: strings.ToLower(joined), Kind: kind, Pos: c.pos + p.off, Bracket: c.bracket, sep: sep, hyphen: hyphen})
				i++
				continue
			}
		}
		t := Token{Text: p.text, Norm: strings.ToLower(p.text), Pos: c.pos + p.off, Bracket: c.bracket, sep: sep, hyphen: hyphen}
		if ct, ok := classify(t); ok {
			t = ct
		}
		out = append(out, t)
	}
	return out
}

// compounds are hyphenated tags that are one token.
var compounds = map[string]Kind{
	"web-dl": Source, "web-rip": Source, "blu-ray": Source, "b-global": Source, "hd-dvd": Source,
	"dts-hd": Codec, "dts-x": Codec, "dts-es": Codec, "10-bit": Codec, "8-bit": Codec, "dd-ex": Codec,
	"multi-subs": Codec, "multi-sub": Codec, "dual-audio": Codec,
}

var (
	reSeason      = regexp.MustCompile(`^s(\d{1,2})$`)
	reSeasonWord  = regexp.MustCompile(`^(?:season|series)(\d{1,2})$`)
	reSeasonRange = regexp.MustCompile(`^s(\d{1,2})-s?(\d{1,2})$`)
	reEpisode     = regexp.MustCompile(`^(?:e|ep)(\d{1,4})(?:v(\d))?$`)
	reEpRange     = regexp.MustCompile(`^(?:e|ep)(\d{1,4})-(?:e|ep)?(\d{1,4})$`)
	reDateISO     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reDateCompact = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`)
	reResP        = regexp.MustCompile(`^(\d{3,4})[pi]$`)
	reResWxH      = regexp.MustCompile(`^(\d{3,4})x(\d{3,4})$`)
	reRange       = regexp.MustCompile(`^(\d{1,4})[-~](\d{1,4})$`)
	reNumVer      = regexp.MustCompile(`^(\d{1,4})v(\d)$`)
	reVersion     = regexp.MustCompile(`^v(\d)$`)
	reCRC         = regexp.MustCompile(`^[0-9a-f]{8}$`)
	reYear        = regexp.MustCompile(`^(19\d\d|20\d\d)$`)
	reNum         = regexp.MustCompile(`^\d+$`)
	reVideoCodec  = regexp.MustCompile(`^(?:[xh]\.?26[45]|hevc|avc|av1|vp9|xvid|divx|hi10p?|\d{1,2}bits?|hdr|hdr10|hdr10\+|hdr10plus|dv|dovi|sdr|10bit|8bit|remux)$`)
	reAudioCodec  = regexp.MustCompile(`^(?:dd|ddp|dd\+|eac3|ac3|aac|dts|truehd|atmos|flac|opus|lpcm|pcm|mp3|ma)(\d(?:\.\d)?)?$`)
)

// sources are release sources and streaming services.
var sources = map[string]bool{
	"bluray": true, "bdrip": true, "brrip": true, "bdremux": true, "bd": true, "bdmv": true,
	"web": true, "webdl": true, "webrip": true, "webhd": true, "hdtv": true, "pdtv": true, "sdtv": true,
	"dsr": true, "tvrip": true, "hdtvrip": true, "dvd": true, "dvdrip": true, "dvd5": true, "dvd9": true,
	"dvdscr": true, "hdrip": true, "vhsrip": true, "laserdisc": true, "tc": true,
	"hdcam": true, "camrip": true, "hdts": true, "telesync": true, "telecine": true, "hdtc": true,
	"amzn": true, "nf": true, "dsnp": true, "hmax": true, "hulu": true, "atvp": true, "pcok": true,
	"pmtp": true, "cr": true, "funi": true, "hidive": true, "adn": true, "bglobal": true, "abema": true,
	"stan": true, "itunes": true,
}

// classify recognises a single chunk. ok is false for a plain word.
func classify(t Token) (Token, bool) {
	l := t.Norm
	switch {
	case l == "":
		return t, false
	case parseSE(&t):
		return t, true
	case parseNxNN(&t):
		return t, true
	}
	if m := reResWxH.FindStringSubmatch(l); m != nil {
		t.Kind, t.N = Res, atoi(m[2])
		return t, true
	}
	if m := reResP.FindStringSubmatch(l); m != nil {
		if n := atoi(m[1]); n >= 240 && n <= 4320 {
			t.Kind, t.N = Res, n
			return t, true
		}
	}
	switch l {
	case "4k", "uhd":
		t.Kind, t.N = Res, 2160
		return t, true
	case "8k":
		t.Kind, t.N = Res, 4320
		return t, true
	}
	if m := reSeason.FindStringSubmatch(l); m != nil {
		t.Kind, t.Season = Season, atoi(m[1])
		t.LastSeason = t.Season
		return t, true
	}
	if m := reSeasonWord.FindStringSubmatch(l); m != nil {
		t.Kind, t.Season = Season, atoi(m[1])
		t.LastSeason = t.Season
		return t, true
	}
	if m := reSeasonRange.FindStringSubmatch(l); m != nil {
		if a, b := atoi(m[1]), atoi(m[2]); a < b {
			t.Kind, t.Season, t.LastSeason = Season, a, b
			return t, true
		}
	}
	if m := reEpisode.FindStringSubmatch(l); m != nil {
		t.Kind, t.Eps = Episode, []int{atoi(m[1])}
		t.Ver = atoi(m[2])
		return t, true
	}
	if m := reEpRange.FindStringSubmatch(l); m != nil {
		if eps := expand(atoi(m[1]), atoi(m[2])); eps != nil {
			t.Kind, t.Eps = Episode, eps
			return t, true
		}
	}
	if m := reDateISO.FindStringSubmatch(l); m != nil {
		if d, ok := makeDate(m[1], m[2], m[3]); ok {
			t.Kind, t.Date, t.Dates = Date, d, []string{d}
			return t, true
		}
	}
	if m := reDateCompact.FindStringSubmatch(l); m != nil && !t.Bracket {
		if d, ok := makeDate(m[1], m[2], m[3]); ok {
			t.Kind, t.Date, t.Dates = Date, d, []string{d}
			return t, true
		}
	}
	if t.Bracket && reCRC.MatchString(l) {
		t.Kind = CRC
		return t, true
	}
	if m := reRange.FindStringSubmatch(l); m != nil {
		if lo, hi := atoi(m[1]), atoi(m[2]); lo < hi {
			t.Kind, t.Lo, t.Hi = Range, lo, hi
			return t, true
		}
	}
	if m := reNumVer.FindStringSubmatch(l); m != nil {
		t.Kind, t.N, t.Ver = Num, atoi(m[1]), atoi(m[2])
		return t, true
	}
	if m := reVersion.FindStringSubmatch(l); m != nil {
		t.Kind, t.N = Version, atoi(m[1])
		return t, true
	}
	if reYear.MatchString(l) {
		t.Kind, t.N = Year, atoi(l)
		return t, true
	}
	if reNum.MatchString(l) {
		t.Kind, t.N = Num, atoi(l)
		return t, true
	}
	if reVideoCodec.MatchString(l) || reAudioCodec.MatchString(l) {
		t.Kind = Codec
		return t, true
	}
	if sources[l] {
		t.Kind = Source
		return t, true
	}
	return t, false
}

// parseSE reads "s01e05", "s01e05e06", "s01e05-e07", "s01e05-07",
// "s01e05-s01e06", "s01e12+e13", "s01e12+s01e13" and "s01e05v2". A "-"
// joins a range, a "+" a list.
func parseSE(t *Token) bool {
	l := t.Norm
	season, rest, ok := leadingNumber(l, "s", 2)
	if !ok || !strings.HasPrefix(rest, "e") {
		return false
	}
	first, rest, ok := leadingNumber(rest, "e", 4)
	if !ok {
		return false
	}
	eps := []int{first}
	ver := 0
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, "v") && len(rest) == 2 && rest[1] >= '0' && rest[1] <= '9':
			ver = int(rest[1] - '0')
			rest = ""
		case strings.HasPrefix(rest, "e"):
			n, r, ok := leadingNumber(rest, "e", 4)
			if !ok {
				return false
			}
			if eps = appendEp(eps, n, false); eps == nil {
				return false
			}
			rest = r
		case strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, "+"):
			isRange := rest[0] == '-'
			r := rest[1:]
			if s2, r2, ok := leadingNumber(r, "s", 2); ok && strings.HasPrefix(r2, "e") {
				if s2 != season {
					return false
				}
				r = r2
			}
			r = strings.TrimPrefix(r, "e")
			n, r, ok := leadingNumber(r, "", 4)
			if !ok {
				return false
			}
			next := appendEp(eps, n, isRange)
			if next == nil {
				return false
			}
			eps, rest = next, r
		default:
			return false
		}
	}
	t.Kind, t.Season, t.LastSeason, t.Eps, t.Ver = SxxEyy, season, season, eps, ver
	return true
}

// parseNxNN reads "1x05", "1x05-1x06", "1x05-06", "1x05x06" and
// "1x05+1x06". A first part longer than two digits is a resolution, not a
// season.
func parseNxNN(t *Token) bool {
	l := t.Norm
	i := 0
	for i < len(l) && l[i] >= '0' && l[i] <= '9' {
		i++
	}
	if i == 0 || i > 2 || i >= len(l) || l[i] != 'x' {
		return false
	}
	season := atoi(l[:i])
	rest := l[i:]
	first, rest, ok := leadingNumber(rest, "x", 3)
	if !ok || first < 1 || len(l[i+1:])-len(rest) < 2 {
		return false
	}
	eps := []int{first}
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, "x"):
			n, r, ok := leadingNumber(rest, "x", 3)
			if !ok {
				return false
			}
			if eps = appendEp(eps, n, false); eps == nil {
				return false
			}
			rest = r
		case strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, "+"):
			isRange := rest[0] == '-'
			r := rest[1:]
			if j := strings.IndexByte(r, 'x'); j > 0 && j <= 2 {
				if atoi(r[:j]) != season {
					return false
				}
				r = r[j+1:]
			}
			n, r, ok := leadingNumber(r, "", 3)
			if !ok {
				return false
			}
			next := appendEp(eps, n, isRange)
			if next == nil {
				return false
			}
			eps, rest = next, r
		default:
			return false
		}
	}
	t.Kind, t.Season, t.LastSeason, t.Eps = NxNN, season, season, eps
	return true
}

// leadingNumber strips prefix and reads up to max digits.
func leadingNumber(s, prefix string, max int) (n int, rest string, ok bool) {
	if !strings.HasPrefix(s, prefix) {
		return 0, s, false
	}
	s = s[len(prefix):]
	i := 0
	for i < len(s) && i < max && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || (i < len(s) && s[i] >= '0' && s[i] <= '9') {
		return 0, s, false
	}
	return atoi(s[:i]), s[i:], true
}

// appendEp adds n to eps; as a range end it adds every episode up to n. It
// returns nil for an episode that doesn't come after the last one, or a
// range that spans more than 50.
func appendEp(eps []int, n int, isRange bool) []int {
	last := eps[len(eps)-1]
	if n <= last {
		return nil
	}
	if !isRange {
		return append(eps, n)
	}
	if n-last > 50 {
		return nil
	}
	for e := last + 1; e <= n; e++ {
		eps = append(eps, e)
	}
	return eps
}

func expand(lo, hi int) []int {
	if hi <= lo || hi-lo > 200 {
		return nil
	}
	eps := make([]int, 0, hi-lo+1)
	for e := lo; e <= hi; e++ {
		eps = append(eps, e)
	}
	return eps
}

func makeDate(y, m, d string) (string, bool) {
	s := y + "-" + m + "-" + d
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", false
	}
	year := atoi(y)
	return s, year >= 1930 && year <= 2099
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// merge joins tokens that only mean something together: dates written with
// separators, "Season 1", "Episode 5", "S01 E05", "H.264", "DDP5.1",
// "AAC 2.0", "28.5", "01 ~ 28".
func merge(in []Token) []Token {
	out := make([]Token, 0, len(in))
	for i := 0; i < len(in); i++ {
		t, skip := joinNext(in, i, out)
		i += skip
		out = emit(out, t)
	}
	return out
}

// joinNext returns the token starting at in[i], joined with the tokens after
// it when they belong together, and how many of those it took.
func joinNext(in []Token, i int, out []Token) (Token, int) {
	t := in[i]
	var n1, n2 Token
	has1, has2 := i+1 < len(in), i+2 < len(in)
	if has1 {
		n1 = in[i+1]
	}
	if has2 {
		n2 = in[i+2]
	}
	joined := func(t Token, text string, kind Kind) Token {
		t.Text, t.Norm, t.Kind = text, strings.ToLower(text), kind
		return t
	}

	switch {
	// 2024.03.15, 2024 03 15
	case t.Kind == Year && has2 && n1.Kind == Num && n2.Kind == Num &&
		len(n1.Text) == 2 && len(n2.Text) == 2 && n1.sep == n2.sep:
		if d, ok := makeDate(t.Text, n1.Text, n2.Text); ok {
			t = joined(t, t.Text+string(n1.sep)+n1.Text+string(n2.sep)+n2.Text, Date)
			t.Date, t.Dates = d, []string{d}
			return t, 2
		}

	// 13.03.2024 (day first) or 03.13.2024 (month first): both readings
	// when both are dates.
	case t.Kind == Num && len(t.Text) == 2 && has2 && n1.Kind == Num && len(n1.Text) == 2 &&
		n2.Kind == Year && n1.sep == n2.sep:
		var dates []string
		if d, ok := makeDate(n2.Text, n1.Text, t.Text); ok {
			dates = append(dates, d)
		}
		if d, ok := makeDate(n2.Text, t.Text, n1.Text); ok && (len(dates) == 0 || dates[0] != d) {
			dates = append(dates, d)
		}
		if len(dates) > 0 {
			t = joined(t, t.Text+string(n1.sep)+n1.Text+string(n2.sep)+n2.Text, Date)
			t.Date, t.Dates = dates[0], dates
			return t, 2
		}

	// 24.03.13: a date with a two-digit year can't be read reliably. It
	// stays a date, without a reading, so it isn't taken for an episode.
	case t.Kind == Num && len(t.Text) == 2 && has2 && n1.Kind == Num && len(n1.Text) == 2 &&
		n2.Kind == Num && len(n2.Text) == 2 && n1.sep == n2.sep && n1.sep != ' ' && twoDigitDate(t.Text, n1.Text, n2.Text):
		return joined(t, t.Text+string(n1.sep)+n1.Text+string(n2.sep)+n2.Text, Date), 2

	// "28.5" after a space or a dash: an in-between episode.
	case t.Kind == Num && !t.Bracket && has1 && n1.Kind == Num && n1.Text == "5" && n1.sep == '.' &&
		(t.sep == ' ' || prevKind(out) == Dash):
		t = joined(t, t.Text+".5", Num)
		t.Half = true
		return t, 1

	// H.264, x 265
	case t.Kind == Word && (t.Norm == "h" || t.Norm == "x") && has1 && n1.Kind == Num && (n1.N == 264 || n1.N == 265):
		return joined(t, t.Text+"."+n1.Text, Codec), 1

	// Season 1, Series 2, Season 1-5
	case t.Kind == Word && isSeasonWord(t.Norm) && has1 && (n1.Kind == Num || n1.Kind == Range):
		t = joined(t, t.Text+" "+n1.Text, Season)
		if n1.Kind == Range {
			t.Season, t.LastSeason = n1.Lo, n1.Hi
		} else {
			t.Season, t.LastSeason = n1.N, n1.N
		}
		return t, 1

	// 2nd Season
	case t.Kind == Word && isOrdinal(t.Norm) && has1 && n1.Kind == Word && isSeasonWord(n1.Norm):
		t = joined(t, t.Text+" "+n1.Text, Season)
		t.Season = atoi(strings.TrimRight(t.Norm[:len(t.Norm)-len(n1.Norm)-1], "stndrh"))
		t.LastSeason = t.Season
		return t, 1

	// Episode 5, Ep 05, Episodes 1-12
	case t.Kind == Word && isEpisodeWord(t.Norm) && has1 && n1.Kind == Num:
		t = joined(t, t.Text+" "+n1.Text, Episode)
		t.Eps, t.Ver = []int{n1.N}, n1.Ver
		return t, 1
	case t.Kind == Word && isEpisodeWord(t.Norm) && has1 && n1.Kind == Range && expand(n1.Lo, n1.Hi) != nil:
		t = joined(t, t.Text+" "+n1.Text, Episode)
		t.Eps = expand(n1.Lo, n1.Hi)
		return t, 1

	// 01 ~ 28
	case t.Kind == Num && has2 && n1.Kind == Dash && n1.Norm == "~" && n2.Kind == Num && t.N < n2.N:
		t = joined(t, t.Text+" ~ "+n2.Text, Range)
		t.Lo, t.Hi = t.N, n2.N
		return t, 2

	// Audio channels: DDP5.1, AAC2.0
	case t.Kind == Codec && has1 && n1.Kind == Num && len(n1.Text) == 1 && n1.sep == '.' && endsWithDigit(t.Norm):
		return joined(t, t.Text+"."+n1.Text, Codec), 1

	// Audio channels after a tag: AAC 2.0, DTS-HD MA 5.1
	case t.Kind == Num && len(t.Text) == 1 && has1 && n1.Kind == Num && len(n1.Text) == 1 && n1.sep == '.' &&
		isTagKind(prevKind(out)):
		return joined(t, t.Text+"."+n1.Text, Codec), 1

	// DTS-HD MA
	case t.Kind == Word && t.Norm == "ma" && prevKind(out) == Codec:
		t.Kind = Codec
		return t, 0

	// Dual Audio, Multi Subs, Multiple Subtitle
	case t.Kind == Word && (t.Norm == "dual" || t.Norm == "multi" || t.Norm == "multiple") && has1 && n1.Kind == Word &&
		(n1.Norm == "audio" || strings.HasPrefix(n1.Norm, "sub")):
		return joined(t, t.Text+" "+n1.Text, Codec), 1
	}

	if t.Kind == Dash && t.Norm == "~" {
		t.Norm = "-"
	}
	return t, 0
}

// emit appends t, folding an episode marker into the one before it:
//   - an episode after a season: "S01 E05", "Season 1 Episode 5";
//   - later episodes of the same season, side by side or joined by "&",
//     "and", "+" or ",": "S01E01.E02", "S01E01 & E02", "S02E12.S02E13",
//     "2x12 & 2x13", "6x04 + 6x05", "2x12, 2x13".
func emit(out []Token, t Token) []Token {
	if len(out) == 0 || (t.Kind != Episode && t.Kind != SxxEyy && t.Kind != NxNN) {
		return append(out, t)
	}
	at := len(out) - 1
	joiner := ""
	if out[at].Kind == Word && (out[at].Norm == "and" || out[at].Norm == "+") && at > 0 {
		joiner = out[at].Text
		at--
	}
	prev := &out[at]
	isSE := func(k Kind) bool { return k == SxxEyy || k == NxNN }
	switch {
	case t.Kind == Episode && joiner == "" && prev.Kind == Season && prev.Season == prev.LastSeason:
		prev.Kind = SxxEyy
		prev.Eps, prev.Ver = t.Eps, t.Ver
	case isSE(prev.Kind) && (t.Kind == Episode || isSE(t.Kind) && t.Season == prev.Season) &&
		len(t.Eps) > 0 && t.Eps[0] > prev.Eps[len(prev.Eps)-1]:
		prev.Eps = append(prev.Eps, t.Eps...)
	default:
		return append(out, t)
	}
	if joiner != "" {
		prev.Text += " " + joiner
	}
	prev.Text += " " + t.Text
	prev.Norm = strings.ToLower(prev.Text)
	return out[:at+1]
}

// twoDigitDate reports whether three two-digit numbers read as a date in
// any order a release uses: yy.mm.dd, dd.mm.yy or mm.dd.yy.
func twoDigitDate(a, b, c string) bool {
	for _, ymd := range [][3]string{{a, b, c}, {c, b, a}, {c, a, b}} {
		if _, ok := makeDate("20"+ymd[0], ymd[1], ymd[2]); ok {
			return true
		}
	}
	return false
}

func isTagKind(k Kind) bool { return k == Codec || k == Source || k == Res }

func prevKind(out []Token) Kind {
	if len(out) == 0 {
		return Word
	}
	return out[len(out)-1].Kind
}

func endsWithDigit(s string) bool {
	return s != "" && s[len(s)-1] >= '0' && s[len(s)-1] <= '9'
}

func isSeasonWord(s string) bool {
	switch s {
	case "season", "seasons", "series", "saison", "staffel", "temporada", "stagione":
		return true
	}
	return false
}

func isEpisodeWord(s string) bool {
	switch s {
	case "episode", "episodes", "ep", "eps", "episodio", "folge":
		return true
	}
	return false
}

var reOrdinal = regexp.MustCompile(`^\d{1,2}(st|nd|rd|th)$`)

func isOrdinal(s string) bool { return reOrdinal.MatchString(s) }

// notGroups are words that end a name after a hyphen without being a group.
var notGroups = map[string]bool{
	"sample": true, "trailer": true, "proper": true, "repack": true, "rerip": true, "internal": true,
	"real": true, "extended": true, "dubbed": true, "subbed": true, "dl": true, "hd": true,
}

// markTrailingGroup marks the release group at the end of a scene name:
// the last hyphenated word ("x264-GRP", "WEB-DL-NTb-sample") after a quality
// tag.
func markTrailingGroup(toks []Token) {
	tagSeen := -1
	for i, t := range toks {
		if t.Kind == Res || t.Kind == Codec || t.Kind == Source {
			tagSeen = i
			break
		}
	}
	if tagSeen < 0 {
		return
	}
	for i := len(toks) - 1; i > tagSeen; i-- {
		t := &toks[i]
		if t.Kind == Word && notGroups[t.Norm] {
			continue
		}
		if t.hyphen && (t.Kind == Word || t.Kind == Num) {
			t.Kind = Group
		}
		return
	}
}
