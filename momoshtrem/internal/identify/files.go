package identify

import (
	"regexp"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// FileClass is what a file is by its extension.
type FileClass string

const (
	ClassVideo    FileClass = "video"
	ClassSubtitle FileClass = "subtitle"
	ClassFont     FileClass = "font"
	ClassImage    FileClass = "image"
	ClassInfo     FileClass = "info"
	ClassArchive  FileClass = "archive"
	ClassOther    FileClass = "other"
)

var extClasses = map[string]FileClass{
	".mkv": ClassVideo, ".mp4": ClassVideo, ".avi": ClassVideo, ".wmv": ClassVideo, ".mov": ClassVideo,
	".m4v": ClassVideo, ".webm": ClassVideo, ".ts": ClassVideo, ".m2ts": ClassVideo, ".vob": ClassVideo,
	".flv": ClassVideo, ".divx": ClassVideo, ".mpg": ClassVideo, ".mpeg": ClassVideo, ".ogm": ClassVideo,
	".srt": ClassSubtitle, ".ass": ClassSubtitle, ".ssa": ClassSubtitle, ".sub": ClassSubtitle,
	".idx": ClassSubtitle, ".vtt": ClassSubtitle, ".smi": ClassSubtitle, ".sup": ClassSubtitle,
	".ttf": ClassFont, ".otf": ClassFont, ".ttc": ClassFont, ".woff": ClassFont, ".woff2": ClassFont,
	".jpg": ClassImage, ".jpeg": ClassImage, ".png": ClassImage, ".gif": ClassImage, ".webp": ClassImage,
	".bmp": ClassImage, ".tbn": ClassImage,
	".nfo": ClassInfo, ".txt": ClassInfo, ".md": ClassInfo, ".url": ClassInfo, ".html": ClassInfo,
	".htm": ClassInfo, ".sfv": ClassInfo, ".md5": ClassInfo, ".sha1": ClassInfo, ".srr": ClassInfo,
	".zip": ClassArchive, ".rar": ClassArchive, ".7z": ClassArchive, ".tar": ClassArchive, ".gz": ClassArchive,
}

var reRarPart = regexp.MustCompile(`^\.r\d\d$`)

// ClassOf returns a file's class by its extension.
func ClassOf(path string) FileClass {
	_, _, ext := splitPath(path)
	if c, ok := extClasses[ext]; ok {
		return c
	}
	if reRarPart.MatchString(ext) {
		return ClassArchive
	}
	return ClassOther
}

// splitPath returns a path's folders, its file name without the extension,
// and the lowercase extension.
func splitPath(p string) (dirs []string, name, ext string) {
	p = strings.ReplaceAll(p, "\\", "/")
	segs := strings.Split(strings.Trim(p, "/"), "/")
	base := segs[len(segs)-1]
	dirs = segs[:len(segs)-1]
	if i := strings.LastIndexByte(base, '.'); i > 0 && len(base)-i <= 6 && !strings.ContainsAny(base[i:], " ") {
		ext = strings.ToLower(base[i:])
		if _, known := extClasses[ext]; known || reRarPart.MatchString(ext) {
			return dirs, base[:i], ext
		}
	}
	return dirs, base, ""
}

// extraOfClass is the extra type of files that are neither videos nor
// subtitles.
func extraOfClass(c FileClass) ExtraType {
	switch c {
	case ClassFont:
		return ExtraFont
	case ClassImage:
		return ExtraImage
	case ClassInfo:
		return ExtraInfo
	}
	return ExtraOther
}

var (
	reOpening = regexp.MustCompile(`^(ncop|nc op|creditless op|textless op)\d*$|^(nc|creditless|textless|clean|non credit)? ?opening( \d+)?$`)
	reEnding  = regexp.MustCompile(`^(nced|ncend|nc ed|creditless ed|textless ed)\d*$|^(nc|creditless|textless|clean|non credit)? ?ending( \d+)?$`)
	reOpWord  = regexp.MustCompile(`^(nc)?op\d*$`)
	reEdWord  = regexp.MustCompile(`^(nc)?ed\d*$`)
	reNCWord  = regexp.MustCompile(`^nc(op|ed|end)\d*$`)
)

// markerWords are words that make a video an extra wherever they appear
// outside the show's title (and, after an episode marker, only in the tags).
var markerWords = map[string]ExtraType{
	"sample": ExtraSample, "samples": ExtraSample,
	"trailer": ExtraTrailer, "trailers": ExtraTrailer, "teaser": ExtraTrailer, "teasers": ExtraTrailer,
	"preview": ExtraTrailer, "previews": ExtraTrailer, "promo": ExtraTrailer,
	"featurette": ExtraFeaturette, "featurettes": ExtraFeaturette, "interview": ExtraFeaturette,
	"interviews": ExtraFeaturette, "bloopers": ExtraFeaturette, "outtakes": ExtraFeaturette,
	"extras": ExtraFeaturette, "bonus": ExtraFeaturette,
	"menu": ExtraOther, "menus": ExtraOther,
}

// markerPhrases are two-word markers.
var markerPhrases = map[string]ExtraType{
	"behind the": ExtraFeaturette, "deleted scene": ExtraFeaturette, "deleted scenes": ExtraFeaturette,
	"gag reel": ExtraFeaturette, "making of": ExtraFeaturette,
}

// markerDirs are folders whose videos are extras.
var markerDirs = map[string]ExtraType{
	"extras": ExtraFeaturette, "extra": ExtraFeaturette, "featurettes": ExtraFeaturette,
	"featurette": ExtraFeaturette, "bonus": ExtraFeaturette, "behind the scenes": ExtraFeaturette,
	"deleted scenes": ExtraFeaturette, "interviews": ExtraFeaturette, "sample": ExtraSample,
	"samples": ExtraSample, "trailers": ExtraTrailer, "trailer": ExtraTrailer, "nc": ExtraOther,
	"ncop": ExtraOpening, "nced": ExtraEnding, "creditless": ExtraOther, "menus": ExtraOther,
	"menu": ExtraOther, "scans": ExtraImage, "fonts": ExtraFont, "attachments": ExtraFont,
	"cd": ExtraOther, "artwork": ExtraImage,
	// Blu-ray and DVD structures: streams there aren't episode files.
	"bdmv": ExtraOther, "stream": ExtraOther, "clipinf": ExtraOther, "playlist": ExtraOther,
	"certificate": ExtraOther, "video ts": ExtraOther, "audio ts": ExtraOther,
}

// specialDirs are folders that hold specials (season 0).
var specialDirs = map[string]bool{
	"specials": true, "special": true, "sp": true, "sps": true, "ova": true, "ovas": true,
	"oad": true, "oads": true, "ona": true, "season 0": true, "season 00": true, "s00": true, "s0": true,
}

// specialWord matches words that mark a special: "OVA", "OAD 2", "SP1",
// "Special", "Recap".
var reSpecialWord = regexp.MustCompile(`^(ova|oad|ona|oav|sp|special|specials|recap)(\d{1,3})?$`)

// wordsOf returns the normalised words of tokens.
func wordsOf(toks []token.Token) []string {
	var out []string
	for _, t := range toks {
		if t.Kind == token.Dash || t.Kind == token.Group {
			continue
		}
		out = append(out, normWords(t.Text)...)
	}
	return out
}

// textExtra finds an extra marker in words. strict limits it to markers
// that can't be an episode's name: "sample", "ncop", "nced".
func textExtra(words []string, strict bool) (ExtraType, string) {
	for i, w := range words {
		if i+1 < len(words) {
			pair := w + " " + words[i+1]
			if !strict {
				if t, ok := markerPhrases[pair]; ok {
					return t, pair
				}
			}
			if reOpening.MatchString(pair) {
				return ExtraOpening, pair
			}
			if reEnding.MatchString(pair) {
				return ExtraEnding, pair
			}
		}
		switch {
		case reNCWord.MatchString(w) && strings.Contains(w, "op"):
			return ExtraOpening, w
		case reNCWord.MatchString(w):
			return ExtraEnding, w
		case w == "sample" || w == "samples":
			return ExtraSample, w
		case strict:
			continue
		case reOpening.MatchString(w):
			return ExtraOpening, w
		case reEnding.MatchString(w):
			return ExtraEnding, w
		}
		if t, ok := markerWords[w]; ok {
			return t, w
		}
	}
	return "", ""
}

// shortExtra reads a bare "OP", "ED1", "PV" or "CM" that follows a dash in
// an anime name: "Show - OP1 [1080p]".
func shortExtra(w string) ExtraType {
	switch {
	case reOpWord.MatchString(w):
		return ExtraOpening
	case reEdWord.MatchString(w):
		return ExtraEnding
	case w == "pv" || w == "cm" || strings.HasPrefix(w, "pv") && containsDigit(w) || strings.HasPrefix(w, "cm") && containsDigit(w):
		return ExtraTrailer
	}
	return ""
}

// Quality is what a file name says about its video.
type Quality struct {
	Resolution string // "2160p", "1080p", "720p", "576p", "480p"; "" when unknown
	Source     string // "BluRay", "WEB-DL", "WEBRip", "HDTV", "DVDRip", "DVD"; "" when unknown
}

// ParseQuality reads the resolution and source from a file's path, the
// file name first, then its folders.
func ParseQuality(path string) Quality {
	dirs, name, _ := splitPath(path)
	var q Quality
	for _, s := range append([]string{name}, reverse(dirs)...) {
		for _, t := range token.Tokenize(s) {
			if q.Resolution == "" && t.Kind == token.Res {
				q.Resolution = resolutionLabel(t.N)
			}
			if q.Source == "" && t.Kind == token.Source {
				q.Source = sourceLabels[strings.ReplaceAll(t.Norm, "-", "")]
			}
		}
	}
	return q
}

func resolutionLabel(lines int) string {
	switch {
	case lines >= 2000:
		return "2160p"
	case lines >= 1000:
		return "1080p"
	case lines >= 700:
		return "720p"
	case lines >= 560:
		return "576p"
	case lines > 0:
		return "480p"
	}
	return ""
}

var sourceLabels = map[string]string{
	"bluray": "BluRay", "bdrip": "BluRay", "brrip": "BluRay", "bdremux": "BluRay", "bd": "BluRay",
	"webdl": "WEB-DL", "web": "WEB-DL", "webhd": "WEB-DL", "webrip": "WEBRip",
	"hdtv": "HDTV", "pdtv": "HDTV", "sdtv": "HDTV", "hdtvrip": "HDTV", "tvrip": "HDTV", "dsr": "HDTV",
	"dvdrip": "DVDRip", "dvd": "DVD", "dvd5": "DVD", "dvd9": "DVD", "hdrip": "HDRip",
}

func reverse(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}
