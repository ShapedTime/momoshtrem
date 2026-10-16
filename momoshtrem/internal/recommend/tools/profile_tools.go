package tools

import (
	"context"
	"encoding/json"
	"errors"
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
			p, err := inputs.Profile(ctx)
			if err != nil {
				return nil, err
			}
			if p == nil {
				return nil, &agent.ToolError{Message: "There's no taste profile for this run."}
			}
			return p, nil
		},
	}
}

var getCandidatesSchema = compact(`{
	"type": "object",
	"properties": {
		"media_type": {
			"description": "movie, tv, or any for both.",
			"type": "string",
			"enum": ["movie", "tv", "any"]
		},
		"genres": {
			"description": "Any of these genres, by name as the profile and candidates name them (Drama, Science Fiction, Animation); [] for any genre.",
			"type": "array",
			"items": {"type": "string"}
		},
		"min_runtime": {
			"description": "At least this many minutes (a movie's runtime, a show's episode runtime); 0 for no bound. A bound leaves out titles with no known runtime.",
			"type": "integer"
		},
		"max_runtime": {
			"description": "At most this many minutes, as min_runtime; 0 for no bound.",
			"type": "integer"
		},
		"year_from": {
			"description": "Released (or first aired) in this year or later; 0 for no bound.",
			"type": "integer"
		},
		"year_to": {
			"description": "Released (or first aired) in this year or earlier; 0 for no bound.",
			"type": "integer"
		},
		"language": {
			"description": "Original language, ISO 639-1 (en, ja, ko); empty for any.",
			"type": "string"
		},
		"anime": {
			"description": "only: anime only; exclude: no anime; either: both.",
			"type": "string",
			"enum": ["only", "exclude", "either"]
		},
		"limit": {
			"description": "At most this many, 1–60; 0 for 30.",
			"type": "integer"
		}
	},
	"required": ["media_type", "genres", "min_runtime", "max_runtime", "year_from", "year_to", "language", "anime", "limit"],
	"additionalProperties": false
}`)

// The sentinels that stand for "no filter": strict schemas get no
// nullable unions (PLAN §7 R1: at most 16 per request, across every
// strict tool).
const (
	anyMedia    = "any"
	animeOnly   = "only"
	animeNone   = "exclude"
	animeEither = "either"
)

type getCandidatesInput struct {
	MediaType  string   `json:"media_type"`
	Genres     []string `json:"genres"`
	MinRuntime int      `json:"min_runtime"`
	MaxRuntime int      `json:"max_runtime"`
	YearFrom   int      `json:"year_from"`
	YearTo     int      `json:"year_to"`
	Language   string   `json:"language"`
	Anime      string   `json:"anime"`
	Limit      int      `json:"limit"`
}

func (in getCandidatesInput) filter() (candidates.Filter, error) {
	f := candidates.Filter{Genres: in.Genres, MinRuntime: in.MinRuntime, MaxRuntime: in.MaxRuntime, YearFrom: in.YearFrom,
		YearTo: in.YearTo, Language: strings.TrimSpace(in.Language), Limit: in.Limit}
	switch in.MediaType {
	case anyMedia, "":
	case library.TitleMovie, library.TitleTV:
		f.MediaType = in.MediaType
	default:
		return f, &agent.ToolError{Message: "media_type is movie, tv or any."}
	}
	switch in.Anime {
	case animeEither, "":
	case animeOnly, animeNone:
		only := in.Anime == animeOnly
		f.Anime = &only
	default:
		return f, &agent.ToolError{Message: "anime is only, exclude or either."}
	}
	for _, n := range []int{in.MinRuntime, in.MaxRuntime, in.YearFrom, in.YearTo, in.Limit} {
		if n < 0 {
			return f, &agent.ToolError{Message: "Runtimes, years and the limit can't be negative (0 means no bound)."}
		}
	}
	return f, nil
}

// poolError is a pool that couldn't be built: TMDB answering none of its
// sources is the model's to work around (tmdb_search still may); a
// cancelled run or a database error ends the run.
func poolError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, candidates.ErrNoSources) {
		return &agent.ToolError{Message: "TMDB didn't answer for the candidate pool; use tmdb_search instead."}
	}
	return err
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
			"Anything watched, started, rated, dropped, hidden or turned down is already left out; library titles stay (in_library). " +
			"Every filter has a value for no filter: any, [], 0, empty or either. Each item: media_type, tmdb_id, title, date, genres, lang, anime, runtime (minutes; a show's per episode), " +
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
				return nil, poolError(ctx, err)
			}
			if pool == nil {
				return nil, &agent.ToolError{Message: "There's no candidate pool for this run; use tmdb_search instead."}
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
