package identify

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxReasonRunes is the longest a proposal's reason may be.
const MaxReasonRunes = 120

// clipReason shortens a reason to MaxReasonRunes, ending it with "…".
func clipReason(s string) string { return clipRunes(s, MaxReasonRunes) }

// clipRunes shortens s to at most n runes.
func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return strings.TrimRight(string(runes[:n-1]), " ;,") + "…"
}

// extraReason explains why a file is ignored by default.
func extraReason(t ExtraType, why string) string {
	quoted := ""
	if why != "" {
		if t == ExtraOpening || t == ExtraEnding {
			why = strings.ToUpper(why) // "NCOP01"
		}
		quoted = fmt.Sprintf(" (%q)", clipRunes(why, 40))
	}
	switch t {
	case ExtraOpening:
		return "Creditless opening" + quoted + "; ignored by default"
	case ExtraEnding:
		return "Creditless ending" + quoted + "; ignored by default"
	case ExtraSample:
		return "Sample clip; ignored by default"
	case ExtraTrailer:
		return "Trailer or preview" + quoted + "; ignored by default"
	case ExtraFeaturette:
		return "Bonus material" + quoted + "; ignored by default"
	case ExtraFont:
		return "Font for the subtitles; ignored"
	case ExtraInfo:
		return "Info file; ignored"
	case ExtraImage:
		return "Image; ignored"
	}
	if why != "" {
		return "Extra" + quoted + "; ignored by default"
	}
	return "Not a video or subtitle; ignored"
}

// noMatchReason explains why a video has no target.
func (st *packState) noMatchReason(fi *fileInfo) string {
	switch {
	case fi.special && !st.ix.hasSpecials:
		return "Specials are off for this show"
	case fi.suffixWhy != "":
		return fi.suffixWhy
	case fi.half:
		return fmt.Sprintf("Episode %s falls between episodes: a recap or special?", fi.specialText)
	case fi.special && fi.specialText != "":
		return fmt.Sprintf("%q: no matching special in TMDB", fi.specialText)
	case fi.special:
		return "Looks like a special; no match in TMDB"
	}
	for _, c := range fi.cands {
		switch c.kind {
		case candExplicit:
			return fmt.Sprintf("%q isn't in TMDB", c.text)
		case candDate:
			return fmt.Sprintf("Nothing aired on %s in TMDB", c.date)
		case candNumber:
			return fmt.Sprintf("No episode %s in TMDB", numText(c))
		}
	}
	if len(st.ix.cat.Episodes) == 0 {
		return "The show has no episodes yet"
	}
	return "No episode number or title in the name"
}
