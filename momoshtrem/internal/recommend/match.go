package recommend

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// The agent names each title it picks by TMDB id and also by name and
// year, so an id it remembered wrong is caught: R2's shelves and hero
// picks, and Ask's show_picks (#160's live run showed The Mandalorian's id
// sent for Upload).

// SameTitle reports whether the agent's name for a title is TMDB's (its
// title or original title), loosely: case, accents and punctuation aside,
// or one holding the other as whole words of four characters or more. It
// catches an id that names another title altogether, not a sequel ("Dune"
// passes for "Dune: Part Two"); SameYear catches more. The snapshot is
// TMDB's either way.
func SameTitle(given string, names ...string) bool {
	g := NormalizeTitle(given)
	if g == "" {
		return false
	}
	for _, n := range names {
		n = NormalizeTitle(n)
		switch {
		case n == "":
		case n == g, containsWords(n, g), containsWords(g, n):
			return true
		}
	}
	return false
}

// SameYear reports whether the agent's year fits TMDB's: a year apart is
// fine (release dates differ by country); either one unknown (0) passes.
// With the title it tells a remake from its original ("Dark" from "Dark
// Matter" too, which the loose title check lets through).
func SameYear(given, actual int) bool {
	if given == 0 || actual == 0 {
		return true
	}
	return given-actual <= 1 && actual-given <= 1
}

// Mismatch is why a pick whose id names another title was dropped, for
// the tool result: what the id is on TMDB, what the model called it, and
// to look it up. given and actual are titles, with their years when known
// (0 leaves the year out).
func Mismatch(given string, givenYear int, actual string, year int) string {
	name, want := actual, given
	if year > 0 {
		name = fmt.Sprintf("%s (%d)", actual, year)
	}
	if givenYear > 0 {
		want = fmt.Sprintf("%s (%d)", given, givenYear)
	}
	return fmt.Sprintf("that tmdb_id is “%s” on TMDB, not “%s”: look the title up with tmdb_search", name, want)
}

// containsWords reports whether s holds sub as whole words; very short
// titles don't count ("It" is in everything).
func containsWords(s, sub string) bool {
	return len(sub) >= 4 && strings.Contains(" "+s+" ", " "+sub+" ")
}

// foldMarks strips combining marks after decomposition: "Amélie" reads as
// "Amelie". A transformer keeps state, so each call makes its own: titles
// are checked concurrently.
func foldMarks() transform.Transformer {
	return transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
}

// NormalizeTitle lowercases a title and keeps its words of letters and
// digits, dropping a leading "the": "The Office (US)" and "office us"
// agree.
func NormalizeTitle(s string) string {
	s = strings.ToLower(s)
	if folded, _, err := transform.String(foldMarks(), s); err == nil {
		s = folded
	}
	s = strings.NewReplacer("&", " and ", "'", "", "’", "", "`", "").Replace(s)
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	return strings.Join(words, " ")
}
