package subtitle

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/release"
)

// Release is what subtitle ranking needs to know about a release name such
// as "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.H.265-FLUX": whether two
// releases share a group, a source and a cut. internal/release reads the
// name; this keeps the parts ranking compares, in its own terms.
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

// sourceFamilies maps release sources to the family that decides timing:
// subtitles for a WEB-DL fit a WEBRip of the same stream, not a Blu-ray.
var sourceFamilies = map[string]string{
	"REMUX": "bluray", "BluRay": "bluray", "WEB-DL": "web", "WEBRip": "web", "WEB": "web",
	"HDTV": "hdtv", "DVD": "dvd", "CAM": "cam",
}

// sourceLabels are the display names of source families.
var sourceLabels = map[string]string{
	"bluray": "BluRay", "web": "WEB", "hdtv": "HDTV", "dvd": "DVD", "cam": "CAM",
}

// editionKeys are the cuts whose timing differs from the theatrical one,
// which is the unmarked default.
var editionKeys = map[string]string{
	"Extended": "extended", "Director's Cut": "directors", "IMAX": "imax", "Unrated": "unrated", "Uncut": "unrated",
	"Remastered": "remastered",
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

// ParseRelease reads a release name. An empty name gives an empty Release.
func ParseRelease(name string) Release {
	r := Release{Name: name}
	trimmed := strings.TrimSpace(name)
	if ext := strings.ToLower(path.Ext(trimmed)); knownExtensions[ext] {
		trimmed = strings.TrimSuffix(trimmed, trimmed[len(trimmed)-len(ext):])
	}

	words := wordRe.FindAllString(strings.ToLower(trimmed), -1)
	r.Normalized = strings.Join(words, " ")
	seen := make(map[string]bool, len(words))
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			r.Tokens = append(r.Tokens, w)
		}
	}

	p := release.Parse(trimmed)
	if p.Resolution != release.Unknown {
		r.Resolution = p.Resolution
	}
	r.Source = sourceFamilies[p.Source]
	r.GroupName = p.Group
	r.Group = strings.ToLower(p.Group)
	r.Edition = editionKeys[p.Edition]
	r.Repack = p.Repack
	return r
}

// groupLabel is the group for display.
func (r Release) groupLabel() string { return r.GroupName }

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
