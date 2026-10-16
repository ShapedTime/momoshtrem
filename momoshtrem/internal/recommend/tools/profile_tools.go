package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
)

// Tool names.
const (
	NameGetTasteProfile = "get_taste_profile"
	NameGetCandidates   = "get_candidates"
	NameTMDBSearch      = "tmdb_search"
	NameTMDBDetails     = "tmdb_details"
	NameCheckSeen       = "check_seen"
)

var getTasteProfileSchema = compact(`{
	"type": "object",
	"properties": {},
	"required": [],
	"additionalProperties": false
}`)

// NewGetTasteProfile is get_taste_profile: the profile, as R1 built it.
func NewGetTasteProfile(inputs Inputs) agent.Tool {
	return &tool{
		name: NameGetTasteProfile,
		description: "The user's taste profile, built from their Trakt history, ratings and feedback. " +
			"Shares (w, 0–1) are the part of their weighted history a genre, keyword, person, decade, language or country covers. " +
			"loved: rated 8+, liked or finished; dropped: shows abandoned; recent: the last 30 days; aversions: rated 4 or less, disliked or not interested. " +
			"Ground every reason in it, and never recommend what it lists.",
		schema: getTasteProfileSchema,
		run: func(ctx context.Context, input json.RawMessage) (any, error) {
			var in struct{}
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			return inputs.Profile(ctx)
		},
	}
}

var getCandidatesSchema = compact(`{
	"type": "object",
	"properties": {
		"media_type": {
			"description": "movie or tv; null for both.",
			"anyOf": [{"type": "string", "enum": ["movie", "tv"]}, {"type": "null"}]
		},
		"genres": {
			"description": "Any of these genres, by name as the profile and candidates name them (Drama, Science Fiction, Animation); null for any.",
			"anyOf": [{"type": "array", "items": {"type": "string"}}, {"type": "null"}]
		},
		"min_runtime": {
			"description": "At least this many minutes: a movie's runtime, a show's episode runtime. Titles with no known runtime are left out.",
			"anyOf": [{"type": "integer"}, {"type": "null"}]
		},
		"max_runtime": {
			"description": "At most this many minutes, as min_runtime.",
			"anyOf": [{"type": "integer"}, {"type": "null"}]
		},
		"year_from": {
			"description": "Released (or first aired) in this year or later.",
			"anyOf": [{"type": "integer"}, {"type": "null"}]
		},
		"year_to": {
			"description": "Released (or first aired) in this year or earlier.",
			"anyOf": [{"type": "integer"}, {"type": "null"}]
		},
		"language": {
			"description": "Original language, ISO 639-1 (en, ja, ko); null for any.",
			"anyOf": [{"type": "string"}, {"type": "null"}]
		},
		"anime": {
			"description": "true: anime only; false: no anime; null: either.",
			"anyOf": [{"type": "boolean"}, {"type": "null"}]
		},
		"limit": {
			"description": "At most this many, 1–60; null for 30.",
			"anyOf": [{"type": "integer"}, {"type": "null"}]
		}
	},
	"required": ["media_type", "genres", "min_runtime", "max_runtime", "year_from", "year_to", "language", "anime", "limit"],
	"additionalProperties": false
}`)

type getCandidatesInput struct {
	MediaType  *string  `json:"media_type"`
	Genres     []string `json:"genres"`
	MinRuntime *int     `json:"min_runtime"`
	MaxRuntime *int     `json:"max_runtime"`
	YearFrom   *int     `json:"year_from"`
	YearTo     *int     `json:"year_to"`
	Language   *string  `json:"language"`
	Anime      *bool    `json:"anime"`
	Limit      *int     `json:"limit"`
}

func (in getCandidatesInput) filter() (candidates.Filter, error) {
	f := candidates.Filter{Genres: in.Genres, Anime: in.Anime}
	if in.MediaType != nil {
		if !validMediaType(*in.MediaType) {
			return f, &agent.ToolError{Message: "media_type is movie, tv or null."}
		}
		f.MediaType = *in.MediaType
	}
	for _, v := range []struct {
		src *int
		dst *int
	}{{in.MinRuntime, &f.MinRuntime}, {in.MaxRuntime, &f.MaxRuntime}, {in.YearFrom, &f.YearFrom}, {in.YearTo, &f.YearTo}, {in.Limit, &f.Limit}} {
		if v.src != nil {
			if *v.src < 0 {
				return f, &agent.ToolError{Message: "Runtimes, years and the limit can't be negative."}
			}
			*v.dst = *v.src
		}
	}
	if in.Language != nil {
		f.Language = strings.TrimSpace(*in.Language)
	}
	return f, nil
}

// candidatesResult is get_candidates' answer.
type candidatesResult struct {
	Matched int                    `json:"matched"` // in the pool, through the filters
	Pool    candidates.Stats       `json:"pool"`
	Items   []candidates.Candidate `json:"items"`
}

// NewGetCandidates is get_candidates: the pool, filtered. Titles turned
// down since the pool was built are left out too.
func NewGetCandidates(inputs Inputs, titles Titles) agent.Tool {
	return &tool{
		name: NameGetCandidates,
		description: "The candidate pool: up to 150 titles TMDB suggests for this taste profile, best first (pre-ranked by overlap with the profile). " +
			"Anything watched, started, dropped, hidden or turned down is already left out; library titles stay (in_library). " +
			"Every filter is optional (null). Each item: media_type, tmdb_id, title, date, genres, lang, anime, runtime (minutes; a show's per episode), " +
			"seasons, ended, vote (TMDB), why (its sources: rec:<loved title> and similar:<loved title> are TMDB's picks for a title they loved; " +
			"kw their keywords; new, gems and lang discover; trending, upcoming, on_air), in_library, watchlist.",
		schema: getCandidatesSchema,
		run: func(ctx context.Context, input json.RawMessage) (any, error) {
			var in getCandidatesInput
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			f, err := in.filter()
			if err != nil {
				return nil, err
			}
			pool, err := inputs.Pool(ctx)
			if err != nil {
				return nil, err
			}
			all := pool.Match(f)
			refs := make([]library.TitleRef, len(all))
			for i := range all {
				refs[i] = all[i].Ref()
			}
			known, err := states(ctx, titles, refs)
			if err != nil {
				return nil, err
			}
			res := candidatesResult{Pool: pool.Stats, Items: []candidates.Candidate{}}
			for _, c := range all {
				if s, ok := known[c.Ref()]; ok && candidates.Exclusion(s) != "" {
					continue
				}
				res.Matched++
				if len(res.Items) < f.EffectiveLimit() {
					res.Items = append(res.Items, c)
				}
			}
			return res, nil
		},
	}
}
