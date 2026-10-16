package identify

import (
	"sort"
	"time"
)

// Labels of the numberings ReleaseNumbers finds besides TMDB's own. Torrent
// search labels its extra queries with them (PLAN §6 "Search").
const (
	LabelAbsolute = "absolute number"
	LabelScene    = "scene numbering"
)

// ReleaseNumber is one way a release may number a TMDB episode.
type ReleaseNumber struct {
	// Method is MethodSxxEyy for TMDB's own numbering, MethodAbsolute, or
	// MethodRemapCour / MethodRemapGroup for a release season TMDB doesn't
	// have.
	Method Method
	// Label is "" for TMDB's own numbering, else LabelAbsolute or
	// LabelScene.
	Label   string
	Season  int // the release season; 0 for an absolute number
	Episode int // the episode in that season, or the absolute number
}

// Range is an inclusive run of episode numbers.
type Range struct{ From, To int }

// Span is what a release's name says it covers, in the release's own
// numbering. Search builds it from the release parser's pack; it lives here
// because the parser imports identify/token, so identify can't import it.
type Span struct {
	// Seasons are the release seasons: one with Episodes ("S02E01-E05"), or
	// whole seasons without ("S02", "S01-S03").
	Seasons []int
	// Episodes is an in-season run of Seasons[0]. Without exactly one
	// season, the numbers read as absolute.
	Episodes *Range
	// Absolute is a run of absolute numbers ("Frieren (29-38)",
	// "One Piece 1124"). It wins over Seasons and Episodes.
	Absolute *Range
}

// Numbering indexes a catalog for ReleaseNumbers and Cover. Build it once
// per catalog: it costs about what a Classify call's index does.
type Numbering struct {
	ix    *catalogIndex
	absOf map[int64]int // the absolute number releases use, by episode id
	cours []cour        // the cours of TMDB's last regular season
	// scene are the episodes a release season TMDB doesn't have maps to
	// (a later cour, an episode group's season).
	scene map[int64]bool
}

// NewNumbering indexes cat.
func NewNumbering(cat *ShowCatalog) *Numbering {
	ix := newIndex(cat, time.Now())
	n := &Numbering{
		ix:    ix,
		absOf: make(map[int64]int, len(ix.byAbs)),
		scene: map[int64]bool{},
	}
	for a, ep := range ix.byAbs {
		n.absOf[ep.ID] = a
	}
	last := ix.lastRegular()
	if last > 0 {
		n.cours = ix.coursOf(last)
	}

	seasons := map[int]bool{}
	for k := 2; k <= len(n.cours); k++ {
		seasons[last+k-1] = true // release S(last+1) is the 2nd cour
	}
	for _, alt := range ix.cat.AltOrderings {
		for _, g := range alt.Groups {
			seasons[g.Number] = true
		}
	}
	for s := range seasons {
		if s < 1 || ix.seasonExists(s) {
			continue
		}
		for _, ep := range n.remapSeason(s) {
			n.scene[ep.ID] = true
		}
	}
	return n
}

// ReleaseNumbers lists the numberings releases may use for TMDB episode
// (season, episode): TMDB's own first, then its absolute number, then scene
// numberings (Frieren S01E29 is release S02E01, the 2nd cour). Each is
// read back the way the classifier reads a release, so a release numbered
// any of these ways maps to this episode. Specials have only their own
// number. Nil when the catalog has no such episode.
func ReleaseNumbers(cat *ShowCatalog, season, episode int) []ReleaseNumber {
	return NewNumbering(cat).ReleaseNumbers(season, episode)
}

// Cover maps what a release covers to catalog episodes, in season and
// episode order. See Numbering.Cover.
func Cover(cat *ShowCatalog, span Span) []Target {
	return NewNumbering(cat).Cover(span)
}

// ReleaseNumbers is the package's ReleaseNumbers on n's catalog.
func (n *Numbering) ReleaseNumbers(season, episode int) []ReleaseNumber {
	ix := n.ix
	ep := ix.episode(season, episode)
	if ep == nil {
		return nil
	}
	out := []ReleaseNumber{{Method: MethodSxxEyy, Season: season, Episode: episode}}
	if season == 0 {
		return out
	}
	if a, ok := n.absOf[ep.ID]; ok {
		out = append(out, ReleaseNumber{Method: MethodAbsolute, Label: LabelAbsolute, Episode: a})
	}

	// Candidates: the season of an episode group that lists it, and the
	// later cour of TMDB's last season it's in. planRemap decides which
	// one a release season really means, so each is read back.
	type se struct{ season, episode int }
	var candidates []se
	for _, alt := range ix.cat.AltOrderings {
		for _, g := range alt.Groups {
			for i, id := range g.EpisodeIDs {
				if id == ep.ID && g.Number >= 1 {
					candidates = append(candidates, se{g.Number, i + 1})
				}
			}
		}
	}
	if last := ix.lastRegular(); season == last {
		for k := 2; k <= len(n.cours); k++ {
			if c := n.cours[k-1]; episode >= c.first && episode <= c.last {
				candidates = append(candidates, se{last + k - 1, episode - c.first + 1})
			}
		}
	}
	seen := map[se]bool{{season, episode}: true}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		if got, method := n.read(c.season, c.episode); got == ep {
			out = append(out, ReleaseNumber{Method: method, Label: LabelScene, Season: c.season, Episode: c.episode})
		}
	}
	return out
}

// read is the episode the classifier reads a release's SxxEyy as, and how:
// TMDB's own when it exists (an S/E that exists is never remapped), else
// through planRemap.
func (n *Numbering) read(season, episode int) (*Episode, Method) {
	if ep := n.ix.episode(season, episode); ep != nil {
		return ep, MethodSxxEyy
	}
	if p := n.ix.planRemap(season, episode); p != nil {
		return p.lookup(episode), p.method
	}
	return nil, ""
}

// Cover maps what a release covers to catalog episodes, in season and
// episode order, with the classifier's remap rules:
//   - an absolute run maps through the absolute numbers;
//   - an in-season run reads each episode as the classifier reads the files
//     of a release season: TMDB's own S/E when it exists, else through the
//     season's remap (Frieren "S02E01-E03" is TMDB S01E29-E31);
//   - a whole season TMDB doesn't have is all its remap reaches (Frieren
//     "S02" is TMDB S01E29-E38). One TMDB has is all its episodes, except
//     that for anime (or a show numbered absolutely) the episodes a later
//     release season maps to are left out: releases that call the 2nd cour
//     "S02" call the 1st "S01" (Frieren "S01" is E01-E28).
//
// Episodes the catalog doesn't have are left out, so a pack that runs past
// TMDB covers what TMDB has.
func (n *Numbering) Cover(span Span) []Target {
	ix := n.ix
	var eps []*Episode
	switch {
	case span.Absolute != nil:
		eps = n.absolute(*span.Absolute)
	case span.Episodes != nil && len(span.Seasons) == 1:
		eps = n.inSeason(span.Seasons[0], *span.Episodes)
	case span.Episodes != nil:
		eps = n.absolute(*span.Episodes)
	default:
		for _, s := range span.Seasons {
			if !ix.seasonExists(s) {
				eps = append(eps, n.remapSeason(s)...)
				continue
			}
			split := s > 0 && ix.absPreferred()
			for e := 1; e <= ix.seasonMax[s]; e++ {
				if ep := ix.episode(s, e); ep != nil && !(split && n.scene[ep.ID]) {
					eps = append(eps, ep)
				}
			}
		}
	}

	seen := make(map[int64]bool, len(eps))
	unique := eps[:0]
	for _, ep := range eps {
		if !seen[ep.ID] {
			seen[ep.ID] = true
			unique = append(unique, ep)
		}
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if unique[i].Season != unique[j].Season {
			return unique[i].Season < unique[j].Season
		}
		return unique[i].Number < unique[j].Number
	})
	return targetsOf(unique)
}

func (n *Numbering) absolute(r Range) []*Episode {
	var eps []*Episode
	for a := r.From; a <= r.To; a++ {
		if ep := n.ix.byAbs[a]; ep != nil {
			eps = append(eps, ep)
		}
	}
	return eps
}

// inSeason reads release episodes r of season s: TMDB's own where they
// exist, the others through the season's remap, decided once for the run
// (a pack folder's files share one plan).
func (n *Numbering) inSeason(s int, r Range) []*Episode {
	ix := n.ix
	var plan *remapPlan
	if s >= 1 {
		plan = ix.planRemap(s, r.To)
	}
	var eps []*Episode
	for e := r.From; e <= r.To; e++ {
		if ep := ix.episode(s, e); ep != nil {
			eps = append(eps, ep)
		} else if plan != nil {
			if ep := plan.lookup(e); ep != nil {
				eps = append(eps, ep)
			}
		}
	}
	return eps
}

// remapSeason is every episode release season s (one TMDB doesn't have)
// maps to, in order; nil when nothing maps it.
func (n *Numbering) remapSeason(s int) []*Episode {
	p := n.ix.planRemap(s, 1)
	if p == nil {
		return nil
	}
	var eps []*Episode
	for e := 1; ; e++ {
		ep := p.lookup(e)
		if ep == nil {
			return eps
		}
		eps = append(eps, ep)
	}
}
