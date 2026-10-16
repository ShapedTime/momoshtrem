package release

import (
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify/token"
)

// groupAliases are names YTS publishes under; they all read "YTS".
var groupAliases = map[string]bool{
	"yts": true, "yify": true, "yts.mx": true, "yts.am": true, "yts.lt": true, "yts.ag": true,
}

// group is the release group as written: the scene group at the end
// ("x264-NTb", "x264-KILLERS[eztv]"), else the fansub group in front
// ("[SubsPlease] …"), else a trailing bracketed one ("Frieren - 29 [720p]
// [Judas]", "[YTS.MX]"), else a trailing "YIFY".
func (p *parser) group() string {
	g := p.findGroup()
	if groupAliases[strings.ToLower(g)] {
		return "YTS"
	}
	return g
}

func (p *parser) findGroup() string {
	toks := p.toks
	if len(toks) == 0 {
		return ""
	}
	if g := sceneGroup(toks); g != "" {
		return g
	}
	// An uploader's tag after the group hides it from the tokenizer:
	// "x264-KILLERS[eztv]".
	if name := strings.TrimRight(p.name, " "); strings.HasSuffix(name, "]") {
		if open := strings.LastIndexByte(name, '['); open > 0 {
			if g := sceneGroup(token.Tokenize(name[:open])); g != "" {
				return g
			}
		}
	}
	if lead := toks[0]; lead.Kind == token.Group && lead.Bracket && !isSite(lead.Text) {
		return lead.Text
	}
	if g := p.trailingBracket(); g != "" {
		return g
	}
	if last := toks[len(toks)-1]; groupAliases[last.Norm] {
		return last.Text
	}
	return ""
}

// sceneGroup is the group the tokenizer found after the tags ("x264-NTb").
func sceneGroup(toks []token.Token) string {
	for i := len(toks) - 1; i > 0; i-- {
		if toks[i].Kind == token.Group && !toks[i].Bracket {
			return toks[i].Text
		}
	}
	return ""
}

// trailingBracket is the content of a [...] that ends the name when it
// names a group: one word that isn't a tag ("[1080p]", "[AAC]", a CRC),
// a pack word ("[Batch]") or a language, and isn't a site ("[EZTVx.to]").
func (p *parser) trailingBracket() string {
	name := strings.TrimRight(p.name, " ")
	if !strings.HasSuffix(name, "]") {
		return ""
	}
	open := strings.LastIndexByte(name, '[')
	if open < 0 {
		return ""
	}
	g := name[open+1 : len(name)-1]
	if groupAliases[strings.ToLower(g)] {
		return g
	}
	if g == "" || strings.ContainsAny(g, " .") {
		return ""
	}
	for _, t := range p.toks {
		if t.Pos <= open || t.Pos >= len(name) {
			continue
		}
		if t.Kind != token.Word || releaseWord(t.Norm) {
			return ""
		}
	}
	return g
}

// releaseWord reports a word that describes a release rather than naming
// a group.
func releaseWord(n string) bool {
	_, lang := languageWords[n]
	_, src := sourceWords[n]
	_, ed := editions[n]
	return lang || src || ed || titleStops[n] || subWords[n] || hardsubWords[n] || n == "multi" || n == "dual"
}

// isSite reports a leading [Group] that is a site's address: "[
// www.Torrenting.com ]", "[EZTVx.to]".
func isSite(text string) bool {
	l := strings.ToLower(text)
	return !groupAliases[l] && strings.Contains(l, ".")
}
