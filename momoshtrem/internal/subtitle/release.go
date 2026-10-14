package subtitle

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Release is what subtitle ranking needs to know about a release name such
// as "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.H.265-FLUX".
//
// This is a deliberately small tokenizer. #6's release parser (torrent
// search) will replace it; until then it only has to tell whether two
// releases share a group, a source and a cut.
type Release struct {
	Name       string   // As given
	Normalized string   // Lowercase alphanumeric words, extension dropped
	Tokens     []string // The words of Normalized, deduplicated
	Resolution string   // "2160p", "1080p", "720p", "576p", "480p"; "" when unknown
	Source     string   // Source family: "bluray", "web", "hdtv", "dvd", "cam"; "" when unknown
	Group      string   // Release group, lowercase, aliases folded ("yify" → "yts"); "" when unknown
	GroupName  string   // The group as written, for display
	Edition    string   // "extended", "directors", "imax", "unrated", "remastered"; "" for the theatrical cut
	Repack     bool     // REPACK, PROPER or RERIP
}

// knownExtensions are dropped from the end of a name before parsing.
var knownExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true, ".mov": true, ".wmv": true, ".ts": true,
	".srt": true, ".ass": true, ".ssa": true, ".sub": true, ".vtt": true, ".idx": true, ".smi": true,
}

// sourceFamilies maps source tokens to the family that decides timing:
// subtitles for a WEB-DL fit a WEBRip of the same stream, not a Blu-ray.
var sourceFamilies = map[string]string{
	"bluray": "bluray", "bdrip": "bluray", "brrip": "bluray", "bdremux": "bluray", "remux": "bluray",
	"bd": "bluray", "bd25": "bluray", "bd50": "bluray", "uhdbd": "bluray",
	"webdl": "web", "webrip": "web", "web": "web", "webhd": "web",
	"hdtv": "hdtv", "pdtv": "hdtv", "dsr": "hdtv", "tvrip": "hdtv", "hdtvrip": "hdtv",
	"dvdrip": "dvd", "dvd": "dvd", "dvd5": "dvd", "dvd9": "dvd", "dvdscr": "dvd", "r5": "dvd",
	"cam": "cam", "hdcam": "cam", "ts": "cam", "telesync": "cam", "hdts": "cam", "tc": "cam",
}

// sourceLabels are the display names of source families.
var sourceLabels = map[string]string{
	"bluray": "BluRay", "web": "WEB", "hdtv": "HDTV", "dvd": "DVD", "cam": "CAM",
}

// groupAliases folds names one group publishes under.
var groupAliases = map[string]string{
	"yts": "yts", "yify": "yts", "yts.mx": "yts", "yts.am": "yts", "yts.lt": "yts", "yts.ag": "yts",
}

// notGroups are words that follow a hyphen at the end of a name without
// being a group: "WEB-DL", "DTS-HD", "Blu-ray".
var notGroups = map[string]bool{
	"dl": true, "hd": true, "rip": true, "ray": true, "ma": true, "x": true, "es": true,
}

// editionWords mark a cut whose timing differs from the theatrical one, which
// is the unmarked default. "Director's Cut" is matched as a phrase in
// ParseRelease.
var editionWords = map[string]string{
	"extended": "extended", "imax": "imax", "unrated": "unrated", "uncut": "unrated",
	"remastered": "remastered",
}

var (
	wordRe       = regexp.MustCompile(`[a-z0-9]+`)
	resolutionRe = regexp.MustCompile(`^(2160|1080|720|576|480)[pi]$`)
	// trailingGroupRe finds "-FLUX" or "-NTb" at the end; bracketed, the
	// group may have dots: "-[YTS.MX]".
	trailingGroupRe   = regexp.MustCompile(`-([A-Za-z0-9]+)$`)
	trailingBracketRe = regexp.MustCompile(`-\[([A-Za-z0-9.]+)\]$`)
	// leadingGroupRe finds "[SubsPlease] Show - 01" (fansub style).
	leadingGroupRe = regexp.MustCompile(`^\[([^\]]+)\]`)
	bracketRe      = regexp.MustCompile(`\[([^\]]+)\]`)
)

// ParseRelease tokenizes a release name. An empty name gives an empty Release.
func ParseRelease(name string) Release {
	r := Release{Name: name}
	trimmed := strings.TrimSpace(name)
	if ext := strings.ToLower(path.Ext(trimmed)); knownExtensions[ext] {
		trimmed = strings.TrimSuffix(trimmed, trimmed[len(trimmed)-len(ext):])
	}
	lower := strings.ToLower(trimmed)

	words := wordRe.FindAllString(lower, -1)
	r.Normalized = strings.Join(words, " ")

	seen := make(map[string]bool, len(words))
	for i, w := range words {
		if !seen[w] {
			seen[w] = true
			r.Tokens = append(r.Tokens, w)
		}
		switch {
		case r.Resolution == "" && resolutionRe.MatchString(w):
			r.Resolution = w[:len(w)-1] + "p"
		case r.Resolution == "" && (w == "4k" || w == "uhd"):
			r.Resolution = "2160p"
		}
		if r.Source == "" {
			// "WEB-DL" and "Blu-ray" split into two words.
			if i+1 < len(words) {
				if fam, ok := sourceFamilies[w+words[i+1]]; ok {
					r.Source = fam
				}
			}
			if r.Source == "" {
				if fam, ok := sourceFamilies[w]; ok {
					r.Source = fam
				}
			}
		}
		if r.Edition == "" {
			if ed, ok := editionWords[w]; ok {
				r.Edition = ed
			}
		}
		if w == "repack" || w == "proper" || w == "rerip" {
			r.Repack = true
		}
	}

	if r.Edition == "" {
		padded := " " + r.Normalized + " "
		if strings.Contains(padded, " directors cut ") || strings.Contains(padded, " director s cut ") {
			r.Edition = "directors"
		}
	}

	r.GroupName = findGroup(trimmed)
	r.Group = normalizeGroup(r.GroupName)
	return r
}

// findGroup returns the release group as written, or "".
func findGroup(name string) string {
	if m := trailingGroupRe.FindStringSubmatch(name); m != nil && !notGroups[strings.ToLower(m[1])] {
		return m[1]
	}
	if m := trailingBracketRe.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	// YTS writes its name in brackets: "Interstellar (2014) [1080p] [YTS.MX]".
	for _, m := range bracketRe.FindAllStringSubmatch(name, -1) {
		if _, ok := groupAliases[strings.ToLower(m[1])]; ok {
			return m[1]
		}
	}
	if m := leadingGroupRe.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	// "Interstellar.2014.1080p.BluRay.x264.YIFY"
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == ' ' || r == '_' })
	if len(words) > 0 {
		last := words[len(words)-1]
		if _, ok := groupAliases[strings.ToLower(last)]; ok {
			return last
		}
	}
	return ""
}

func normalizeGroup(group string) string {
	g := strings.ToLower(group)
	if alias, ok := groupAliases[g]; ok {
		return alias
	}
	return g
}

// groupLabel is the group for display: aliases by their common name.
func (r Release) groupLabel() string {
	if r.Group == "yts" {
		return "YTS"
	}
	return r.GroupName
}

// sourceLabel is the source family for display.
func (r Release) sourceLabel() string { return sourceLabels[r.Source] }

// AssignedRelease names the release a library item's file comes from: the
// magnet's display name (dn), else the torrent's top folder, else the file
// name without its extension.
func AssignedRelease(magnetURI, filePath string) string {
	if u, err := url.Parse(magnetURI); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			return dn
		}
	}
	filePath = strings.Trim(filePath, "/")
	if folder, _, ok := strings.Cut(filePath, "/"); ok && folder != "" {
		return folder
	}
	base := path.Base(filePath)
	if base == "." || base == "" {
		return ""
	}
	return strings.TrimSuffix(base, path.Ext(base))
}
