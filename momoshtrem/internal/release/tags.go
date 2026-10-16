package release

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// Tag vocabularies, keyed by token.Norm. The values are PLAN §6's allowed
// values.

var sourceWords = map[string]string{
	"remux": "REMUX", "bdremux": "REMUX", "dvdremux": "REMUX",
	"bluray": "BluRay", "blu-ray": "BluRay", "bdrip": "BluRay", "brrip": "BluRay", "bd": "BluRay", "bdmv": "BluRay",
	"bd25": "BluRay", "bd50": "BluRay", "uhdbd": "BluRay",
	"web-dl": "WEB-DL", "webdl": "WEB-DL", "webhd": "WEB-DL",
	"webrip": "WEBRip", "web-rip": "WEBRip", "web": "WEB",
	"hdtv": "HDTV", "pdtv": "HDTV", "sdtv": "HDTV", "dsr": "HDTV", "tvrip": "HDTV", "hdtvrip": "HDTV",
	"dvd": "DVD", "dvdrip": "DVD", "dvd5": "DVD", "dvd9": "DVD", "dvdr": "DVD", "r5": "DVD",
	// Filmed in a cinema, or a pre-release screener.
	"cam": "CAM", "camrip": "CAM", "hdcam": "CAM", "ts": "CAM", "hdts": "CAM", "tsrip": "CAM", "telesync": "CAM",
	"tc": "CAM", "hdtc": "CAM", "telecine": "CAM", "scr": "CAM", "screener": "CAM", "dvdscr": "CAM", "bdscr": "CAM",
}

// services are streaming services: a name that gives one and no source is
// a WEB release.
var services = map[string]bool{
	"amzn": true, "nf": true, "dsnp": true, "hmax": true, "hulu": true, "atvp": true, "pcok": true,
	"pmtp": true, "cr": true, "funi": true, "hidive": true, "adn": true, "bglobal": true, "b-global": true,
	"abema": true, "stan": true, "itunes": true,
}

var codecWords = map[string]string{
	"x265": "x265", "h265": "x265", "h.265": "x265", "x.265": "x265", "hevc": "x265",
	"x264": "x264", "h264": "x264", "h.264": "x264", "x.264": "x264", "avc": "x264", "hi10p": "x264", "hi10": "x264",
	"av1": "AV1", "xvid": "other", "divx": "other", "vp9": "other", "vc1": "other", "mpeg2": "other",
}

var hdrWords = map[string]string{
	"dv": "DV", "dovi": "DV", "hdr10+": "HDR10+", "hdr10plus": "HDR10+", "hdr10": "HDR10", "hdr": "HDR",
}

var hdrOrder = []string{"DV", "HDR10+", "HDR10", "HDR"}

var audioWords = map[string]string{
	"atmos": "Atmos", "truehd": "TrueHD",
	"dts-hd": "DTS-HD", "dtshd": "DTS-HD", "dts-x": "DTS-HD", "dts": "DTS", "dts-es": "DTS",
	"ddp": "DD+", "dd+": "DD+", "eac3": "DD+", "dd": "DD", "ac3": "DD", "dd-ex": "DD",
	"aac": "AAC", "flac": "FLAC", "opus": "Opus",
}

var audioOrder = []string{"Atmos", "TrueHD", "DTS-HD", "DTS", "DD+", "DD", "AAC", "FLAC", "Opus"}

// languageWords map audio-language tags to ISO 639-1. Two-letter tags
// ("IT", "DE") and words that are often names ("Dan", "Fin") aren't read.
var languageWords = map[string]string{
	"rus": "ru", "russian": "ru", "рус": "ru", "eng": "en", "english": "en",
	"jap": "ja", "jpn": "ja", "japanese": "ja", "ger": "de", "german": "de", "deu": "de",
	"fre": "fr", "fra": "fr", "french": "fr", "truefrench": "fr", "vff": "fr", "vfq": "fr",
	"ita": "it", "italian": "it", "spa": "es", "spanish": "es", "esp": "es", "castellano": "es", "latino": "es",
	"por": "pt", "portuguese": "pt", "kor": "ko", "korean": "ko",
	"chinese": "zh", "chs": "zh", "cht": "zh", "mandarin": "zh", "cantonese": "zh",
	"ukr": "uk", "ukrainian": "uk", "pol": "pl", "polish": "pl", "hindi": "hi", "tamil": "ta", "telugu": "te",
	"tur": "tr", "turkish": "tr", "ara": "ar", "arabic": "ar", "heb": "he", "hebrew": "he",
	"swe": "sv", "swedish": "sv", "nor": "no", "norwegian": "no", "danish": "da", "finnish": "fi",
	"dut": "nl", "dutch": "nl", "nld": "nl", "cze": "cs", "czech": "cs", "hun": "hu", "hungarian": "hu",
	"aze": "az", "azerbaijani": "az",
}

// subtitleOnly are language tags that name subtitles: "VOSTFR" is the
// original audio with French subtitles.
var subtitleOnly = map[string]string{"vostfr": "fr", "subfrench": "fr"}

var subWords = map[string]bool{
	"sub": true, "subs": true, "subbed": true, "subtitle": true, "subtitles": true, "субтитры": true,
}

var hardsubWords = map[string]bool{
	"hc": true, "hardsub": true, "hardsubs": true, "hardsubbed": true, "hardcoded": true, "korsub": true, "korsubs": true,
}

// editions map a cut's first word to its name; the ones in editionPairs
// need the next word too ("Director's Cut", not "Director's").
var editions = map[string]string{
	"extended": "Extended", "imax": "IMAX", "unrated": "Unrated", "uncut": "Uncut",
	"remastered": "Remastered", "remaster": "Remastered", "theatrical": "Theatrical", "criterion": "Criterion",
}

var editionPairs = map[[2]string]string{
	{"directors", "cut"}: "Director's Cut", {"director's", "cut"}: "Director's Cut",
	{"special", "edition"}: "Special Edition", {"final", "cut"}: "Final Cut",
}

var (
	reChannels = regexp.MustCompile(`(\d\.\d)$`)
	reBitDepth = regexp.MustCompile(`^(\d{1,2})-?bits?$`)
)

// readTags reads the tag zone, the tokens after the title, into r.
func (p *parser) readTags(r *Release) {
	var remux, service bool
	var subLangs []string
	for i := p.end; i < len(p.toks); i++ {
		t := p.toks[i]
		n, next := t.Norm, p.norm(i+1)
		// An episode title's words ("S02E03.The.Final.Cut") aren't tags;
		// tags there are written in capitals ("S01E01.FRENCH.720p").
		if i >= p.epTitleFrom && i < p.epTitleTo && titleCase(t.Text) {
			continue
		}

		if t.Kind == token.Res && r.Resolution == Unknown {
			r.Resolution = resolution(t)
		}
		if src, ok := sourceWords[n]; ok {
			switch {
			case src == "WEB" && next == "dl":
				src = "WEB-DL"
			case src == "WEB" && (next == "rip" || next == "dlrip"):
				src = "WEBRip"
			}
			if src == "REMUX" {
				remux = true
			} else if r.Source == Unknown {
				r.Source = src
			}
		}
		if t.Kind == token.Source && services[n] {
			service = true
		}
		if c, ok := codecWords[n]; ok && r.Codec == Unknown {
			r.Codec = c
		}
		if r.BitDepth == 0 {
			r.BitDepth = bitDepth(n, next)
		}
		if h, ok := hdrWords[n]; ok {
			r.HDR = appendOnce(r.HDR, h)
		}
		if n == "dolby" && next == "vision" {
			r.HDR = appendOnce(r.HDR, "DV")
		}
		if a := audioOf(n); a != "" {
			if a == "DTS" && (next == "hd" || next == "ma") {
				a = "DTS-HD"
			}
			r.Audio = appendOnce(r.Audio, a)
		}
		if r.Channels == "" && (t.Kind == token.Codec || t.Kind == token.Word) {
			r.Channels = channelsOf(n)
		}

		switch {
		case isMultiSub(n):
			r.Subs = "multi"
		case hardsubWords[n]:
			r.Subs = "hardcoded"
		case n == "dual audio" || n == "dual-audio" || n == "dual":
			r.Languages = appendOnce(r.Languages, "dual")
		case n == "multi audio" || n == "multi" && next != "quality":
			r.Languages = appendOnce(r.Languages, "multi")
		}
		for _, part := range strings.Split(n, "+") {
			if code, ok := subtitleOnly[part]; ok {
				subLangs = appendOnce(subLangs, code)
			} else if code, ok := languageWords[part]; ok {
				if p.subtitleLanguage(i) {
					subLangs = appendOnce(subLangs, code)
				} else {
					r.Languages = appendOnce(r.Languages, code)
				}
			}
		}

		if r.Edition == "" {
			if e, ok := editionPairs[[2]string{n, next}]; ok {
				r.Edition = e
			} else if e, ok := editions[n]; ok {
				r.Edition = e
			}
		}
		if strings.HasPrefix(n, "repack") || n == "proper" || n == "rerip" {
			r.Repack = true
		}
	}

	switch {
	case remux:
		r.Source = "REMUX"
	case r.Source == Unknown && service:
		r.Source = "WEB"
	}
	if r.Subs == "" && len(subLangs) > 1 {
		r.Subs = "multi"
	}
	r.HDR = ordered(r.HDR, hdrOrder)
	r.Audio = ordered(r.Audio, audioOrder)
}

// norm is token i's Norm, or "" past the end.
func (p *parser) norm(i int) string {
	if i < len(p.toks) {
		return p.toks[i].Norm
	}
	return ""
}

// subtitleLanguage reports a language tag that names subtitles: after a
// subtitle word, maybe with other languages between ("Sub (Rus, Eng)"), or
// right before one ("ENG SUB").
func (p *parser) subtitleLanguage(i int) bool {
	if subWords[p.norm(i+1)] {
		return true
	}
	for j := i - 1; j >= p.end; j-- {
		n := p.toks[j].Norm
		if subWords[n] {
			return true
		}
		if _, ok := languageWords[n]; !ok {
			return false
		}
	}
	return false
}

// resolution buckets a Res token into the allowed values. "1920x800" is
// 1080p by its width.
func resolution(t token.Token) string {
	lines := t.N
	if w, _, ok := strings.Cut(t.Norm, "x"); ok {
		if width, err := strconv.Atoi(w); err == nil {
			lines = max(lines, width*9/16)
		}
	}
	switch {
	case lines >= 2160:
		return "2160p"
	case lines >= 1080:
		return "1080p"
	case lines >= 720:
		return "720p"
	case lines >= 576:
		return "576p"
	case lines >= 480:
		return "480p"
	}
	return Unknown
}

// bitDepth reads "10bit", "10-bit", "Hi10P" or "10 bit"; 0 for anything
// else, 8-bit included.
func bitDepth(n, next string) int {
	if n == "hi10p" || n == "hi10" {
		return 10
	}
	if next == "bit" || next == "bits" {
		n += "bit"
	}
	if m := reBitDepth.FindStringSubmatch(n); m != nil && (m[1] == "10" || m[1] == "12") {
		d, _ := strconv.Atoi(m[1])
		return d
	}
	return 0
}

// audioOf reads an audio tag, with or without channels: "DDP5.1",
// "AAC2.0", "TrueHD".
func audioOf(n string) string {
	if a, ok := audioWords[n]; ok {
		return a
	}
	if i := strings.IndexAny(n, "0123456789"); i > 0 {
		if rest := n[i:]; len(rest) == 1 || len(rest) == 3 && rest[1] == '.' {
			return audioWords[n[:i]]
		}
	}
	return ""
}

// channelsOf reads "5.1" from "DDP5.1" or "5.1", or "6ch".
func channelsOf(n string) string {
	switch n {
	case "8ch":
		return "7.1"
	case "6ch":
		return "5.1"
	case "2ch":
		return "2.0"
	}
	if m := reChannels.FindString(n); m == "7.1" || m == "5.1" || m == "2.0" {
		return m
	}
	return ""
}

// isMultiSub reports "Multi-Subs", "Multiple Subtitle", "MultiSub".
func isMultiSub(n string) bool {
	return (strings.HasPrefix(n, "multi") || strings.HasPrefix(n, "multiple")) && strings.Contains(n, "sub")
}

// titleCase reports a word written like a title word: "Final", not
// "FINAL", "final" or "MULTi".
func titleCase(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	if size == len(s) || !unicode.IsUpper(r) {
		return false
	}
	return strings.IndexFunc(s[size:], unicode.IsUpper) < 0
}

func appendOnce(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}

// ordered sorts s into order's order.
func ordered(s []string, order []string) []string {
	slices.SortFunc(s, func(a, b string) int { return slices.Index(order, a) - slices.Index(order, b) })
	return s
}
