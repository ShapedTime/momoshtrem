package identify

import (
	"fmt"
	"strconv"
)

// remapPlan maps a release season TMDB doesn't have (or episodes past its
// end) onto TMDB's episodes. It's decided once per folder and release
// season, so every file there moves by the same offset.
type remapPlan struct {
	method Method
	strong bool   // the evidence is good enough for high confidence
	label  string // for reasons: "TMDB's 2nd cour starts at E29"
	season int    // the release season
	lookup func(e int) *Episode
}

// Weights of remapped readings.
const (
	wRemapStrong   = 0.85
	wRemapWeak     = 0.7
	wRemapOverflow = 0.65
)

func (p *remapPlan) weight() float64 {
	switch {
	case p.method == MethodRemapOverflow:
		return wRemapOverflow
	case p.strong:
		return wRemapStrong
	}
	return wRemapWeak
}

// planRemap decides how release season s maps onto TMDB, given the highest
// episode maxE the pack has of it. It runs only for a season TMDB doesn't
// have or episodes past a season's end; an S/E that exists is never
// remapped. In order:
//  1. an alternative ordering (TMDB episode group) with a season s of at
//     least maxE episodes;
//  2. the cours of TMDB's last season: release seasons after TMDB's last
//     are its later cours (Frieren S02 is TMDB S01's 2nd cour). Strong when
//     the cour follows a break of 90 days or more and the pack fits in it;
//  3. counting on past the season's end through absolute numbers
//     (S01E25 of a 24-episode season is S02E01).
//
// It returns nil when none applies.
func (ix *catalogIndex) planRemap(s, maxE int) *remapPlan {
	if s < 1 || maxE < 1 {
		return nil
	}
	for _, alt := range ix.cat.AltOrderings {
		for _, g := range alt.Groups {
			if g.Number != s || len(g.EpisodeIDs) == 0 {
				continue
			}
			ids := g.EpisodeIDs
			return &remapPlan{
				method: MethodRemapGroup,
				strong: len(ids) >= maxE,
				label:  fmt.Sprintf("episode group %q", alt.Name),
				season: s,
				lookup: func(e int) *Episode {
					if e < 1 || e > len(ids) {
						return nil
					}
					return ix.byID[ids[e-1]]
				},
			}
		}
	}

	last := ix.lastRegular()
	if last > 0 && s > last {
		k := s - last + 1 // release S(last+1) is the 2nd cour of TMDB's last season
		cours := ix.coursOf(last)
		if k <= len(cours) {
			c := cours[k-1]
			return &remapPlan{
				method: MethodRemapCour,
				strong: c.gapDays >= 90 && maxE <= c.count(),
				label:  fmt.Sprintf("TMDB's %s cour starts at E%02d", ordinal(k), c.first),
				season: s,
				lookup: func(e int) *Episode {
					if e < 1 || c.first+e-1 > c.last {
						return nil
					}
					return ix.episode(c.season, c.first+e-1)
				},
			}
		}
		return nil
	}

	if ix.seasonExists(s) && maxE > ix.seasonMax[s] {
		start, ok := ix.absStart[s]
		if !ok {
			return nil
		}
		count := ix.seasonMax[s]
		return &remapPlan{
			method: MethodRemapOverflow,
			label:  fmt.Sprintf("S%02d has %d episodes", s, count),
			season: s,
			lookup: func(e int) *Episode {
				if e <= count {
					return nil
				}
				return ix.byAbs[start+e-1]
			},
		}
	}
	return nil
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}
