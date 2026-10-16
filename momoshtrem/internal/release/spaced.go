package release

import (
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// Some indexers (through Jackett) send names with "-", "+" and "." turned
// into spaces: "[SubsPlease] Sousou no Frieren (01 28) (1080p) [Batch]",
// "S01 S04", "DDP5 1". These read them back only where the spaces can
// mean one thing, so a name written that way on purpose reads the same.

// spacedRange reads "(01 28)", the "(01-28)" of a fansub batch, right
// after the title or after its season ("S2 (01 04)"): a bracket that holds
// just two numbers, the second the greater.
func (p *parser) spacedRange(i int) (Span, bool) {
	afterTitle := i == p.end || i == p.end+1 && p.toks[p.end].Kind == token.Season
	if !afterTitle || !p.bracketPair(i) {
		return Span{}, false
	}
	a, b := p.toks[i], p.toks[i+1]
	if a.Half || b.Half || a.N >= b.N {
		return Span{}, false
	}
	return Span{a.N, b.N}, true
}

// spacedSeasonRange reads "S01 S04", two season markers one space apart,
// as the "S01-S04" it was, and returns the last season. Longer runs ("S01
// S03 S05") and wider gaps ("S01   S02" was "S01 + S02") stay lists.
func (p *parser) spacedSeasonRange(i int) (int, bool) {
	if i+1 >= len(p.toks) || p.gap(i, i+1) != " " {
		return 0, false
	}
	a, b := p.toks[i], p.toks[i+1]
	if !sxx(a) || !sxx(b) || a.Season >= b.Season {
		return 0, false
	}
	if i > 0 && p.toks[i-1].Kind == token.Season || i+2 < len(p.toks) && p.toks[i+2].Kind == token.Season {
		return 0, false
	}
	return b.Season, true
}

// sxx reports a one-season marker written "S01" or "S1".
func sxx(t token.Token) bool {
	return t.Kind == token.Season && t.Season == t.LastSeason && len(t.Norm) <= 3 && t.Norm[0] == 's'
}

// spacedChannels reads channels split by a space: "DDP5 1", "AAC2 0",
// "TrueHD 7 1", "[5 1]". "" when token i doesn't start them.
func (p *parser) spacedChannels(i int) string {
	if i+1 >= len(p.toks) || p.gap(i, i+1) != " " {
		return ""
	}
	t, next := p.toks[i], p.toks[i+1]
	if next.Kind != token.Num || len(next.Text) != 1 {
		return ""
	}
	switch {
	case t.Kind == token.Codec && audioOf(t.Norm) != "": // "DDP5 1"
		return channelsOf(t.Norm + "." + next.Text)
	case t.Kind == token.Num && len(t.Text) == 1 && (i > 0 && p.toks[i-1].Kind == token.Codec || p.bracketPair(i)): // "TrueHD 7 1", "[5 1]"
		return channelsOf(t.Text + "." + next.Text)
	}
	return ""
}

// bracketPair reports two numbers at i and i+1 that are all one bracket
// holds, spaces apart: "(01 28)", "[5 1]".
func (p *parser) bracketPair(i int) bool {
	if i+1 >= len(p.toks) {
		return false
	}
	a, b := p.toks[i], p.toks[i+1]
	if a.Kind != token.Num || b.Kind != token.Num || !a.Bracket || !b.Bracket {
		return false
	}
	end := b.Pos + len(b.Text)
	gap := p.gap(i, i+1)
	return a.Pos > 0 && end < len(p.name) && strings.IndexByte("([{", p.name[a.Pos-1]) >= 0 &&
		strings.IndexByte(")]}", p.name[end]) >= 0 && gap != "" && strings.TrimLeft(gap, " ") == ""
}

// gap is the name's text between tokens i and j.
func (p *parser) gap(i, j int) string {
	from, to := p.toks[i].Pos+len(p.toks[i].Text), p.toks[j].Pos
	if from > to || to > len(p.name) {
		return ""
	}
	return p.name[from:to]
}
