package candidates

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Discover's windows and thresholds.
const (
	keywordYears  = 5   // "kw": your keywords, released in the last 5 years
	newDays       = 120 // "new": your genres, released in the last 120 days
	gemsAfterYear = 2   // "gems": your genres, highly rated, at least 2 years old
	langYears     = 3   // "lang": your second language, the last 3 years
	topKeywords   = 6
	topGenres     = 3
	minTypeShare  = 0.15 // a media type under this share of the profile gets no discover
	minLangShare  = 0.15
)

// source is one TMDB list a pool reads.
type source struct {
	name  string
	fetch func(ctx context.Context, t Lists) (*tmdb.ResultsPage, error)
}

func discover(name, mediaType string, q tmdb.DiscoverQuery) source {
	return source{name: name, fetch: func(ctx context.Context, t Lists) (*tmdb.ResultsPage, error) {
		return t.Discover(ctx, mediaType, q)
	}}
}

// sources are the lists a pool reads for p (the seeds' recommendations
// and similar titles aside), with every window computed from now
// (server-local).
func (b *Builder) sources(p *profile.Profile, now time.Time, genreNames map[string]map[int]string) []source {
	date := func(t time.Time) string { return t.Format(time.DateOnly) }
	today := date(now)

	var keywords []int
	for _, k := range p.Keywords {
		if k.ID > 0 && len(keywords) < topKeywords {
			keywords = append(keywords, k.ID)
		}
	}
	var lang string
	for _, l := range p.Languages {
		if l.Name != "en" && l.W >= minLangShare {
			lang = l.Name
			break
		}
	}

	var out []source
	for _, mt := range []string{tmdb.MediaMovie, tmdb.MediaTV} {
		share := p.Mix.MovieShare
		if mt == tmdb.MediaTV {
			share = 1 - share
		}
		if p.Mix.Movies+p.Mix.Shows > 0 && share < minTypeShare {
			continue
		}
		// The profile's genres that are this media type's (TMDB numbers
		// TV genres apart: Sci-Fi & Fantasy is 10765, not 878)
		var genres []int
		for _, g := range p.Genres {
			if _, ok := genreNames[mt][g.ID]; ok && g.ID > 0 && len(genres) < topGenres {
				genres = append(genres, g.ID)
			}
		}
		if len(keywords) > 0 {
			out = append(out, discover("kw", mt, tmdb.DiscoverQuery{Keywords: keywords, From: date(now.AddDate(-keywordYears, 0, 0)),
				MinVoteCount: 50}))
		}
		if len(genres) > 0 {
			out = append(out,
				discover("new", mt, tmdb.DiscoverQuery{Genres: genres, From: date(now.AddDate(0, 0, -newDays)), To: today, MinVoteCount: 10}),
				discover("gems", mt, tmdb.DiscoverQuery{Genres: genres, To: date(now.AddDate(-gemsAfterYear, 0, 0)), MinVoteAverage: 7.5,
					MinVoteCount: 300, SortBy: "vote_average.desc"}))
		}
		if lang != "" {
			out = append(out, discover("lang", mt, tmdb.DiscoverQuery{Language: lang, Genres: genres,
				From: date(now.AddDate(-langYears, 0, 0)), MinVoteCount: 20}))
		}
	}
	out = append(out,
		source{"trending", func(ctx context.Context, t Lists) (*tmdb.ResultsPage, error) {
			return t.Trending(ctx, tmdb.MediaMovie, "week")
		}},
		source{"trending", func(ctx context.Context, t Lists) (*tmdb.ResultsPage, error) {
			return t.Trending(ctx, tmdb.MediaTV, "week")
		}},
		source{"upcoming", func(ctx context.Context, t Lists) (*tmdb.ResultsPage, error) { return t.UpcomingMovies(ctx, 1) }},
		source{"on_air", func(ctx context.Context, t Lists) (*tmdb.ResultsPage, error) { return t.OnTheAir(ctx, 1) }},
	)
	return out
}

func isAnime(genres []tmdb.Genre, lang string, countries []string, keywords []tmdb.Keyword) bool {
	return metadata.IsAnime(genres, lang, countries, keywords)
}
