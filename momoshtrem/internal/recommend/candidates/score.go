package candidates

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// minVotes is how many votes make TMDB's average worth showing and
// weighing.
const minVotes = 50

// What each source is worth. A loved title's recommendation counts more
// the more you love it (seed rank), and every source a title comes from
// adds up, to maxSourceScore.
var sourceScore = map[string]float64{
	"kw":       0.5,
	"lang":     0.35,
	"new":      0.3,
	"gems":     0.3,
	"trending": 0.25,
	"upcoming": 0.2,
	"on_air":   0.2,
}

// TMDB's "similar" lists match on genres and keywords alone, so they
// count for less than its recommendations (what people who liked the
// title also liked).
const (
	recScore       = 1.0
	similarScore   = 0.35
	maxSourceScore = 2.0
	maxKeywordFit  = 1.0
	// A title's TMDB rating moves it by up to qualityWeight: +all at
	// qualityTop above qualityMid, −all as far below.
	qualityWeight = 0.6
	qualityMid    = 6.8
	qualityTop    = 1.5
)

// scorer is the profile as lookups.
type scorer struct {
	genres     map[string]float64 // by lower-case name
	keywords   map[int]float64
	languages  map[string]float64
	movieShare float64
	animeShare float64
	seedRank   map[string]int // a loved title's clipped name → its rank
}

func newScorer(p *profile.Profile) *scorer {
	s := &scorer{genres: map[string]float64{}, keywords: map[int]float64{}, languages: map[string]float64{},
		movieShare: p.Mix.MovieShare, animeShare: p.Mix.AnimeShare, seedRank: map[string]int{}}
	if p.Mix.Movies+p.Mix.Shows == 0 {
		s.movieShare = 0.5
	}
	for _, g := range p.Genres {
		s.genres[strings.ToLower(g.Name)] = g.W
	}
	for _, k := range p.Keywords {
		s.keywords[k.ID] = k.W
	}
	for _, l := range p.Languages {
		s.languages[l.Name] = l.W
	}
	for i, t := range p.Loved {
		if i == seeds {
			break
		}
		s.seedRank[clipName(t.Title)] = i
	}
	return s
}

// score is how well c overlaps the profile: its sources, genres, keywords
// (once TMDB described it), language, media type, anime and rating.
func (s *scorer) score(c *Candidate) float64 {
	var src float64
	for _, w := range c.Why {
		kind, seed, _ := strings.Cut(w, ":")
		switch kind {
		case "rec":
			src += recScore * s.seedWeight(seed)
		case "similar":
			src += similarScore * s.seedWeight(seed)
		default:
			src += sourceScore[kind]
		}
	}
	score := min(src, maxSourceScore)

	if len(c.Genres) > 0 {
		var g float64
		for _, name := range c.Genres {
			g += s.genres[strings.ToLower(name)]
		}
		score += g / math.Sqrt(float64(len(c.Genres)))
	}
	var kw float64
	for _, id := range c.keywordIDs {
		kw += s.keywords[id]
	}
	score += min(kw, maxKeywordFit)
	score += 0.5 * s.languages[c.Language]
	if c.MediaType == tmdb.MediaMovie {
		score += 0.3 * s.movieShare
	} else {
		score += 0.3 * (1 - s.movieShare)
	}
	if c.Anime {
		score += s.animeShare - 0.15 // a lift for anime fans, a dip for everyone else
	}
	if c.Vote > 0 {
		score += qualityWeight * max(-1, min(1, (c.Vote-qualityMid)/qualityTop))
	}
	return math.Round(score*1000) / 1000
}

// seedWeight is a loved title's weight as a seed: the first counts fully,
// each next one 5% less.
func (s *scorer) seedWeight(name string) float64 {
	rank, ok := s.seedRank[name]
	if !ok {
		return 0.5
	}
	return 1 - 0.05*float64(rank)
}

// rank scores the candidates and sorts them best first (ties: the more
// sources, then TMDB id, so a pool is stable).
func rank(cands []*Candidate, s *scorer) {
	for _, c := range cands {
		c.score = s.score(c)
	}
	slices.SortStableFunc(cands, func(a, b *Candidate) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(len(b.Why), len(a.Why)),
			cmp.Compare(a.MediaType, b.MediaType), cmp.Compare(a.TMDBID, b.TMDBID))
	})
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }
