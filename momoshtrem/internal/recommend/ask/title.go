package ask

import (
	"strings"
	"unicode"
)

// Thread titles (PLAN §6): counted in code points (runes; the web counts
// with Array.from), from the trimmed message.
const (
	titleMax    = 60
	titleMinCut = 30 // cut at a space only past this index
)

// ThreadTitle is a new thread's title: the trimmed message with its first
// character upper-cased (unicode.ToUpper on the first rune). Over 60
// characters it takes the first 60, cut back to their last space when that
// space's index (0-based) is over 30 (otherwise all 60: a hard cut), trims
// trailing whitespace and appends "…".
func ThreadTitle(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	if len(runes) <= titleMax {
		return string(runes)
	}
	cut := runes[:titleMax]
	space := -1
	for i, r := range cut {
		if r == ' ' {
			space = i
		}
	}
	if space > titleMinCut {
		cut = cut[:space]
	}
	return strings.TrimRightFunc(string(cut), unicode.IsSpace) + "…"
}
