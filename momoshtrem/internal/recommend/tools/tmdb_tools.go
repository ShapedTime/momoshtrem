package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Result sizes.
const (
	searchResults   = 10
	searchOverview  = 140 // characters
	detailsOverview = 600
	detailsKeywords = 12
	detailsCast     = 6
	detailsCreators = 3
	minVotes        = 50 // fewer votes: no vote shown
)

var tmdbSearchSchema = compact(`{
	"type": "object",
	"properties": {
		"query": {
			"description": "A title, as it's known in English or its original language.",
			"type": "string"
		},
		"media_type": {
			"description": "movie or tv; null searches both.",
			"anyOf": [{"type": "string", "enum": ["movie", "tv"]}, {"type": "null"}]
		},
		"year": {
			"description": "The release (or first air) year, to tell remakes apart; null for any.",
			"anyOf": [{"type": "integer"}, {"type": "null"}]
		}
	},
	"required": ["query", "media_type", "year"],
	"additionalProperties": false
}`)

type tmdbSearchInput struct {
	Query     string  `json:"query"`
	MediaType *string `json:"media_type"`
	Year      *int    `json:"year"`
}

// searchHit is one tmdb_search result.
type searchHit struct {
	MediaType     string   `json:"media_type"`
	TMDBID        int      `json:"tmdb_id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title,omitempty"` // when it differs
	Date          string   `json:"date,omitempty"`
	Language      string   `json:"lang,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Vote          float64  `json:"vote,omitempty"`
	Overview      string   `json:"overview,omitempty"`
	Mark
	popularity float64
}

// NewTMDBSearch is tmdb_search: find a title's TMDB id by name.
func NewTMDBSearch(t TMDB, titles Titles) agent.Tool {
	return &tool{
		name: NameTMDBSearch,
		description: "Find a movie or show on TMDB by name, for a title that isn't in the candidates (one you know of, or found on the web). " +
			"Answers up to 10 matches, most popular first, each with its tmdb_id, and excluded when it can't be picked (watched, started, " +
			"dropped or hidden on Trakt, disliked, not interested, marked seen).",
		schema: tmdbSearchSchema,
		run: func(ctx context.Context, input json.RawMessage) (any, error) {
			var in tmdbSearchInput
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			query := strings.TrimSpace(in.Query)
			if query == "" {
				return nil, &agent.ToolError{Message: "query is empty."}
			}
			types := []string{tmdb.MediaMovie, tmdb.MediaTV}
			if in.MediaType != nil {
				if !validMediaType(*in.MediaType) {
					return nil, &agent.ToolError{Message: "media_type is movie, tv or null."}
				}
				types = []string{*in.MediaType}
			}
			year := 0
			if in.Year != nil {
				year = *in.Year
			}
			return search(ctx, t, titles, query, types, year)
		},
	}
}

func search(ctx context.Context, t TMDB, titles Titles, query string, types []string, year int) (any, error) {
	var hits []searchHit
	var lastErr error
	answered := 0
	for _, mt := range types {
		page, err := t.Search(ctx, mt, query, year)
		if err != nil {
			lastErr = err
			continue
		}
		answered++
		genres := genreNames(ctx, t, mt)
		for _, item := range page.Results {
			hits = append(hits, hitOf(mt, item, genres))
		}
	}
	if answered == 0 {
		return nil, tmdbError(lastErr, fmt.Sprintf("%q", query))
	}
	slices.SortStableFunc(hits, func(a, b searchHit) int { return cmp.Compare(b.popularity, a.popularity) })
	hits = hits[:min(len(hits), searchResults)]

	refs := make([]library.TitleRef, len(hits))
	for i, h := range hits {
		refs[i] = library.TitleRef{MediaType: h.MediaType, TMDBID: h.TMDBID}
	}
	known, err := states(ctx, titles, refs)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		if s, ok := known[refs[i]]; ok {
			hits[i].Mark = markOf(s)
		}
	}
	return struct {
		Results []searchHit `json:"results"`
	}{append([]searchHit{}, hits...)}, nil
}

func hitOf(mediaType string, item tmdb.ListItem, genres map[int]string) searchHit {
	h := searchHit{MediaType: mediaType, TMDBID: item.ID, Title: item.DisplayTitle(), Language: item.OriginalLanguage,
		Overview: clip(strings.TrimSpace(item.Overview), searchOverview), popularity: item.Popularity}
	h.Date = item.ReleaseDate
	if mediaType == tmdb.MediaTV {
		h.Date = item.FirstAirDate
	}
	for _, id := range item.GenreIDs {
		if name := genres[id]; name != "" {
			h.Genres = append(h.Genres, name)
		}
	}
	if item.VoteCount >= minVotes {
		h.Vote = round1(item.VoteAverage)
	}
	return h
}

// genreNames maps a media type's genre ids to names; empty when TMDB
// doesn't say.
func genreNames(ctx context.Context, t TMDB, mediaType string) map[int]string {
	names := map[int]string{}
	genres, err := t.GenreList(ctx, mediaType)
	if err != nil {
		return names
	}
	for _, g := range genres {
		names[g.ID] = g.Name
	}
	return names
}

var tmdbDetailsSchema = compact(`{
	"type": "object",
	"properties": {
		"media_type": {
			"description": "movie or tv.",
			"type": "string",
			"enum": ["movie", "tv"]
		},
		"tmdb_id": {
			"description": "The title's TMDB id.",
			"type": "integer"
		}
	},
	"required": ["media_type", "tmdb_id"],
	"additionalProperties": false
}`)

type tmdbDetailsInput struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
}

// details is tmdb_details' answer.
type details struct {
	MediaType     string   `json:"media_type"`
	TMDBID        int      `json:"tmdb_id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title,omitempty"` // when it differs
	Date          string   `json:"date,omitempty"`           // release, or first air date
	YearEnd       int      `json:"year_end,omitempty"`       // a show that ended: its last year
	TMDBStatus    string   `json:"tmdb_status,omitempty"`    // Released, Returning Series, Ended…
	NextEpisode   string   `json:"next_episode,omitempty"`   // a show's next air date
	Runtime       int      `json:"runtime,omitempty"`        // minutes: a movie's, a show's episode
	Seasons       int      `json:"seasons,omitempty"`        // Specials left out
	Episodes      int      `json:"episodes,omitempty"`
	Genres        []string `json:"genres"`
	Keywords      []string `json:"keywords"`
	Language      string   `json:"lang,omitempty"`
	Countries     []string `json:"countries,omitempty"`
	Anime         bool     `json:"anime,omitempty"`
	Creators      []string `json:"creators,omitempty"` // directors, or a show's creators
	Cast          []string `json:"cast,omitempty"`
	Vote          float64  `json:"vote,omitempty"`
	Votes         int      `json:"votes,omitempty"`
	Tagline       string   `json:"tagline,omitempty"`
	Overview      string   `json:"overview,omitempty"`
	Mark
}

// NewTMDBDetails is tmdb_details: one title, compactly.
func NewTMDBDetails(t TMDB, titles Titles) agent.Tool {
	return &tool{
		name: NameTMDBDetails,
		description: "One movie or show from TMDB, compactly: dates, status, runtime, seasons, genres, keywords, language, creators, cast, " +
			"rating and overview, and whether it can be picked (excluded) and what the user did with it. " +
			"Use it to check a title before recommending it, or to ground its reason.",
		schema: tmdbDetailsSchema,
		run: func(ctx context.Context, input json.RawMessage) (any, error) {
			var in tmdbDetailsInput
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			if !validMediaType(in.MediaType) || in.TMDBID <= 0 {
				return nil, &agent.ToolError{Message: "Send media_type (movie or tv) and a positive tmdb_id."}
			}
			var d *details
			if in.MediaType == tmdb.MediaMovie {
				m, err := t.MovieDetails(ctx, in.TMDBID)
				if err != nil {
					return nil, tmdbError(err, "movie "+strconv.Itoa(in.TMDBID))
				}
				d = movieDetails(m)
			} else {
				s, err := t.ShowDetails(ctx, in.TMDBID)
				if err != nil {
					return nil, tmdbError(err, "show "+strconv.Itoa(in.TMDBID))
				}
				d = showDetails(s)
			}
			known, err := states(ctx, titles, []library.TitleRef{{MediaType: in.MediaType, TMDBID: in.TMDBID}})
			if err != nil {
				return nil, err
			}
			for _, s := range known {
				d.Mark = markOf(s)
			}
			return d, nil
		},
	}
}

func movieDetails(m *tmdb.MovieDetails) *details {
	d := &details{MediaType: tmdb.MediaMovie, TMDBID: m.ID, Title: m.Title, Date: m.ReleaseDate, TMDBStatus: m.Status,
		Runtime: m.Runtime, Genres: names(m.Genres), Keywords: keywordNames(m.Keywords.Keywords), Language: m.OriginalLanguage,
		Countries: m.OriginCountry, Anime: metadata.IsAnime(m.Genres, m.OriginalLanguage, m.OriginCountry, m.Keywords.Keywords),
		Tagline: m.Tagline, Overview: clip(strings.TrimSpace(m.Overview), detailsOverview)}
	if m.OriginalTitle != m.Title {
		d.OriginalTitle = m.OriginalTitle
	}
	d.Creators = m.Credits.Directors()
	if len(d.Creators) > detailsCreators {
		d.Creators = d.Creators[:detailsCreators]
	}
	for _, c := range m.Credits.Cast {
		if len(d.Cast) == detailsCast {
			break
		}
		d.Cast = append(d.Cast, c.Name)
	}
	d.Vote, d.Votes = vote(m.VoteAverage, m.VoteCount)
	return d
}

func showDetails(s *tmdb.ShowDetails) *details {
	d := &details{MediaType: tmdb.MediaTV, TMDBID: s.ID, Title: s.Name, Date: s.FirstAirDate, TMDBStatus: s.Status,
		Episodes: s.NumberOfEpisodes, Genres: names(s.Genres), Keywords: keywordNames(s.Keywords.Keywords), Language: s.OriginalLanguage,
		Countries: s.OriginCountry, Anime: metadata.IsAnime(s.Genres, s.OriginalLanguage, s.OriginCountry, s.Keywords.Keywords),
		Tagline: s.Tagline, Overview: clip(strings.TrimSpace(s.Overview), detailsOverview)}
	if s.OriginalName != s.Name {
		d.OriginalTitle = s.OriginalName
	}
	if len(s.EpisodeRunTime) > 0 {
		d.Runtime = s.EpisodeRunTime[0]
	}
	for _, season := range s.Seasons {
		if season.SeasonNumber > 0 {
			d.Seasons++
		}
	}
	if (s.Status == "Ended" || s.Status == "Canceled") && len(s.LastAirDate) >= 4 {
		d.YearEnd, _ = strconv.Atoi(s.LastAirDate[:4])
	}
	if s.NextEpisodeToAir != nil {
		d.NextEpisode = s.NextEpisodeToAir.AirDate
	}
	for _, c := range s.CreatedBy {
		if len(d.Creators) < detailsCreators {
			d.Creators = append(d.Creators, c.Name)
		}
	}
	for _, c := range s.AggregateCredits.Cast {
		if len(d.Cast) == detailsCast {
			break
		}
		d.Cast = append(d.Cast, c.Name)
	}
	d.Vote, d.Votes = vote(s.VoteAverage, s.VoteCount)
	return d
}

func names(genres []tmdb.Genre) []string {
	out := []string{}
	for _, g := range genres {
		out = append(out, g.Name)
	}
	return out
}

func keywordNames(keywords []tmdb.Keyword) []string {
	out := []string{}
	for _, k := range keywords {
		if len(out) == detailsKeywords {
			break
		}
		out = append(out, k.Name)
	}
	return out
}

func vote(avg float64, count int) (float64, int) {
	if count < minVotes {
		return 0, count
	}
	return round1(avg), count
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

var checkSeenSchema = compact(`{
	"type": "object",
	"properties": {
		"titles": {
			"description": "The titles to check, at most 50.",
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"media_type": {"type": "string", "enum": ["movie", "tv"]},
					"tmdb_id": {"type": "integer"}
				},
				"required": ["media_type", "tmdb_id"],
				"additionalProperties": false
			}
		}
	},
	"required": ["titles"],
	"additionalProperties": false
}`)

// MaxCheckSeen is how many titles check_seen takes at once.
const MaxCheckSeen = 50

type checkSeenInput struct {
	Titles []struct {
		MediaType string `json:"media_type"`
		TMDBID    int    `json:"tmdb_id"`
	} `json:"titles"`
}

// seenItem is one check_seen answer.
type seenItem struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	CanPick   bool   `json:"can_pick"`
	Plays     int    `json:"plays,omitempty"`
	Progress  string `json:"progress,omitempty"` // a show's "26/38" (regular episodes, Trakt's)
	Mark
}

// NewCheckSeen is check_seen: whether titles can still be picked.
func NewCheckSeen(titles Titles) agent.Tool {
	return &tool{
		name: NameCheckSeen,
		description: "Check whether titles can be recommended: can_pick is false for anything watched, started, dropped or hidden on Trakt, " +
			"disliked, not interested or marked seen (excluded says which). Also answers plays, a show's progress, the user's rating and " +
			"whether it's liked, in the library or on the watchlist. Check every title that didn't come from get_candidates.",
		schema: checkSeenSchema,
		run: func(ctx context.Context, input json.RawMessage) (any, error) {
			var in checkSeenInput
			if err := decode(input, &in); err != nil {
				return nil, err
			}
			if len(in.Titles) > MaxCheckSeen {
				return nil, &agent.ToolError{Message: fmt.Sprintf("Check at most %d titles at once.", MaxCheckSeen)}
			}
			refs := make([]library.TitleRef, len(in.Titles))
			for i, t := range in.Titles {
				if !validMediaType(t.MediaType) || t.TMDBID <= 0 {
					return nil, &agent.ToolError{Message: "Each title needs media_type (movie or tv) and a positive tmdb_id."}
				}
				refs[i] = library.TitleRef{MediaType: t.MediaType, TMDBID: t.TMDBID}
			}
			list, err := titles.TitleStates(ctx, refs)
			if err != nil {
				return nil, err
			}
			if len(list) != len(refs) {
				return nil, errors.New("check_seen: the store answered another number of titles")
			}
			res := struct {
				Items    []seenItem `json:"items"`
				CanPick  int        `json:"can_pick"`
				Excluded int        `json:"excluded"`
			}{Items: []seenItem{}}
			for _, s := range list {
				item := seenItem{MediaType: s.MediaType, TMDBID: s.TMDBID, Plays: s.Plays, Mark: markOf(s)}
				item.CanPick = item.Excluded == ""
				if s.Aired > 0 {
					item.Progress = fmt.Sprintf("%d/%d", s.Completed, s.Aired)
				}
				if item.CanPick {
					res.CanPick++
				} else {
					res.Excluded++
				}
				res.Items = append(res.Items, item)
			}
			return res, nil
		},
	}
}
