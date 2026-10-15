package hdrezka

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Grade is how well a search hit matches the title it's for.
type Grade string

const (
	// GradeExact: the title or original title is the same, and neither the
	// year (±1) nor the kind disagrees.
	GradeExact Grade = "exact"
	// GradeLikely: the same title with the year or the kind off, or one
	// title contained in the other with nothing off.
	GradeLikely Grade = "likely"
	// GradeWeak: anything else.
	GradeWeak Grade = "weak"
)

// Want is the title a search is for, as TMDB has it.
type Want struct {
	Title         string
	OriginalTitle string
	Year          int  // 0 when unknown
	Kind          Kind // "" when unknown
}

// Match is a search hit with its grade.
type Match struct {
	SearchResult
	Grade Grade
}

// Rank grades candidates against want and orders them best first: by
// grade, then by title, year and kind agreement in that order of weight.
// Equal candidates keep the site's order.
func Rank(candidates []SearchResult, want Want) []Match {
	wants := titleForms(want.Title, want.OriginalTitle)
	type scored struct {
		Match
		score int
	}
	out := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		title := compareTitles(titleForms(c.Title, c.OriginalTitle), wants)
		year := compareYears(c.Year, want.Year)
		kind := compareKinds(c.Kind, want.Kind)

		conflicts := 0
		if year < 0 {
			conflicts++
		}
		if kind < 0 {
			conflicts++
		}
		grade := GradeWeak
		switch {
		case title == titleSame && conflicts == 0:
			grade = GradeExact
		case title == titleSame && conflicts == 1, title == titleContained && conflicts == 0:
			grade = GradeLikely
		}
		out = append(out, scored{
			Match: Match{SearchResult: c, Grade: grade},
			score: 6*title + 2*year + kind,
		})
	}
	slices.SortStableFunc(out, func(a, b scored) int {
		if c := cmp.Compare(gradeRank(a.Grade), gradeRank(b.Grade)); c != 0 {
			return c
		}
		return cmp.Compare(b.score, a.score)
	})
	matches := make([]Match, len(out))
	for i, s := range out {
		matches[i] = s.Match
	}
	return matches
}

func gradeRank(g Grade) int {
	switch g {
	case GradeExact:
		return 0
	case GradeLikely:
		return 1
	}
	return 2
}

const (
	titleDifferent = iota
	titleContained // one title holds the other as whole words
	titleSame
)

// compareTitles compares two sets of normalised titles.
func compareTitles(have, want []string) int {
	best := titleDifferent
	for _, h := range have {
		for _, w := range want {
			switch {
			case h == w:
				return titleSame
			case containsWords(h, w) || containsWords(w, h):
				best = titleContained
			}
		}
	}
	return best
}

// containsWords reports whether s holds sub as whole words. Very short
// titles don't count, or "It" would be in everything.
func containsWords(s, sub string) bool {
	return len(sub) >= 4 && strings.Contains(" "+s+" ", " "+sub+" ")
}

// compareYears is 2 for the same year, 1 for a year apart (release dates
// differ by country), 0 when either is unknown and -1 otherwise.
func compareYears(have, want int) int {
	switch d := have - want; {
	case have == 0 || want == 0:
		return 0
	case d == 0:
		return 2
	case d == 1 || d == -1:
		return 1
	}
	return -1
}

// compareKinds is 1 when the kinds agree, 0 when either is unknown (cartoons
// and anime can be either) and -1 when they disagree.
func compareKinds(have, want Kind) int {
	switch {
	case have == "" || want == "":
		return 0
	case have == want:
		return 1
	}
	return -1
}

// titleForms normalises titles and their " / " alternatives ("Xing ji yi
// you / Interstellar Wanderer"), dropping empty ones.
func titleForms(titles ...string) []string {
	var out []string
	for _, t := range titles {
		for _, alt := range strings.Split(t, " / ") {
			if n := normalizeTitle(alt); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// foldMarks strips combining marks after decomposition: "Amélie" reads as
// "Amelie" and "Ёлки" as "Елки".
var foldMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalizeTitle lowercases a title and keeps its words of letters and
// digits, so "Law & Order: SVU" and "law and order svu" agree.
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	if folded, _, err := transform.String(foldMarks, s); err == nil {
		s = folded
	}
	s = strings.NewReplacer("&", " and ", "'", "", "’", "", "`", "").Replace(s)
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}
