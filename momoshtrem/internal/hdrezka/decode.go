package hdrezka

import (
	"encoding/base64"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Stream is what the CDN offers for a movie or an episode in one
// translation.
type Stream struct {
	Qualities []Quality // in site order, lowest first
	Subtitles []Subtitle
	// Legacy reports that the links came in the old "#h" base64 format
	// rather than as plain text.
	Legacy bool
}

// Quality is one rendition of a stream.
type Quality struct {
	Label      string // "1080p", "1080p Ultra", "4K"
	Resolution int    // lines: 1080; 2K is 1440 and 4K 2160; 0 when unknown
	Premium    bool   // the CDN serves a teaser for it without a Premium account
	MP4        string // the progressive file; "" when only HLS is offered
	HLS        string // the HLS manifest; "" when none is offered
}

// Subtitle is one subtitle track.
type Subtitle struct {
	Language string // the site's name for it, e.g. "English"
	Code     string // ISO 639-1, e.g. "en"; "" when the site gives none
	URL      string // a signed VTT link
}

// Best returns the highest-resolution quality that isn't premium and has an
// MP4.
func (s *Stream) Best() (Quality, bool) {
	var best Quality
	found := false
	for _, q := range s.Qualities {
		if q.Premium || q.MP4 == "" {
			continue
		}
		if !found || q.Resolution >= best.Resolution {
			best, found = q, true
		}
	}
	return best, found
}

// Find returns the quality with the given label, ignoring case.
func (s *Stream) Find(label string) (Quality, bool) {
	for _, q := range s.Qualities {
		if strings.EqualFold(q.Label, label) {
			return q, true
		}
	}
	return Quality{}, false
}

const (
	legacyPrefix    = "#h"
	legacySeparator = "//_//"
	hlsSuffix       = ":hls:manifest.m3u8"
	premiumMarker   = "pjs-prem-quality"
)

// legacyJunk holds the chunks the legacy format inserts after each
// separator: the base64 of every 2- and 3-character combination of
// @ # ! ^ $. One junk string is a run of these 4-character chunks.
var legacyJunk = func() map[string]bool {
	const chars = "@#!^$"
	m := map[string]bool{}
	for i := 0; i < len(chars); i++ {
		for j := 0; j < len(chars); j++ {
			m[base64.StdEncoding.EncodeToString([]byte{chars[i], chars[j]})] = true
			for k := 0; k < len(chars); k++ {
				m[base64.StdEncoding.EncodeToString([]byte{chars[i], chars[j], chars[k]})] = true
			}
		}
	}
	return m
}()

// decodeStreamURL returns the CDN's url field in plain form:
//
//	[360p]https://…/a.mp4:hls:manifest.m3u8 or https://…/a.mp4,[480p]…
//
// The site sends that as is today. legacy reports the older form: "#h", then
// the base64 of the plain form with junk inserted after "//_//" markers.
func decodeStreamURL(s string) (plain string, legacy bool, err error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "[") || strings.HasPrefix(s, "http"):
		return s, false, nil
	case strings.HasPrefix(s, legacyPrefix):
		plain, err := decodeLegacy(s)
		return plain, true, err
	}
	return "", false, &ParseError{What: "stream links"}
}

// decodeLegacy strips the junk after each separator (the separator may sit
// anywhere in the base64, so the junk is matched chunk by chunk rather than
// removed everywhere), then decodes.
func decodeLegacy(s string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(s, legacyPrefix), legacySeparator)
	for i := 1; i < len(parts); i++ {
		for len(parts[i]) >= 4 && legacyJunk[parts[i][:4]] {
			parts[i] = parts[i][4:]
		}
	}
	joined := strings.TrimRight(strings.Join(parts, ""), "=")
	decoded, err := base64.RawStdEncoding.DecodeString(joined)
	if err != nil {
		return "", &ParseError{What: "legacy stream links"}
	}
	return string(decoded), nil
}

// parseQualities reads a plain url field. Each entry is "[label]links";
// links are alternatives joined by " or ", of which the one not ending in
// .m3u8 is the MP4. A label wrapped in a pjs-prem-quality span is premium.
func parseQualities(plain string) []Quality {
	var out []Quality
	for _, entry := range splitEntries(plain) {
		rawLabel, links, ok := splitEntry(entry)
		if !ok {
			continue
		}
		label := stripTags(rawLabel)
		q := Quality{
			Label:      label,
			Resolution: resolution(label),
			Premium:    strings.Contains(rawLabel, premiumMarker),
		}
		for _, link := range strings.Split(links, " or ") {
			link = strings.TrimSpace(link)
			switch {
			case link == "":
			case strings.HasSuffix(link, ".m3u8"):
				if q.HLS == "" {
					q.HLS = link
				}
			case q.MP4 == "":
				q.MP4 = link
			}
		}
		if q.MP4 == "" && strings.HasSuffix(q.HLS, hlsSuffix) {
			q.MP4 = strings.TrimSuffix(q.HLS, hlsSuffix)
		}
		out = append(out, q)
	}
	return out
}

// parseSubtitles reads "[Русский]https://….vtt,[English]https://….vtt".
// codes maps the names to language codes (subtitle_lns; nil when the site
// sent false). The site writes Ukrainian as "ua"; it becomes ISO's "uk".
func parseSubtitles(raw string, codes map[string]string) []Subtitle {
	var out []Subtitle
	for _, entry := range splitEntries(raw) {
		name, link, ok := splitEntry(entry)
		if !ok {
			continue
		}
		// Like a quality, an entry may hold alternatives; the player takes
		// the last.
		if i := strings.LastIndex(link, " or "); i >= 0 {
			link = link[i+len(" or "):]
		}
		if link = strings.TrimSpace(link); link == "" {
			continue
		}
		name = strings.TrimSpace(name)
		code := strings.ToLower(strings.TrimSpace(codes[name]))
		if code == "ua" {
			code = "uk"
		}
		out = append(out, Subtitle{Language: name, Code: code, URL: link})
	}
	return out
}

// splitEntries splits "[a]x,[b]y" into "[a]x" and "[b]y". Links never
// contain ",[".
func splitEntries(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",[")
	for i := 1; i < len(parts); i++ {
		parts[i] = "[" + parts[i]
	}
	return parts
}

// splitEntry splits "[label]rest".
func splitEntry(entry string) (label, rest string, ok bool) {
	entry = strings.TrimSpace(entry)
	if !strings.HasPrefix(entry, "[") {
		return "", "", false
	}
	end := strings.Index(entry, "]")
	if end < 0 {
		return "", "", false
	}
	return entry[1:end], entry[end+1:], true
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// stripTags turns `<span class="pjs-prem-quality">1080p Ultra<img …></span>`
// into "1080p Ultra".
func stripTags(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(s, ""))), " ")
}

// resolution reads a label's vertical resolution: "1080p Ultra" is 1080,
// "2K" 1440 and "4K" 2160. Unknown labels give 0.
func resolution(label string) int {
	upper := strings.ToUpper(label)
	switch {
	case strings.HasPrefix(upper, "4K"):
		return 2160
	case strings.HasPrefix(upper, "2K"):
		return 1440
	}
	i := 0
	for i < len(upper) && upper[i] >= '0' && upper[i] <= '9' {
		i++
	}
	if i == 0 || i == len(upper) || upper[i] != 'P' {
		return 0
	}
	n, _ := strconv.Atoi(upper[:i])
	return n
}
