package metadata

import (
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// AnimeKeywordID is TMDB's "anime" keyword.
const AnimeKeywordID = 210024

// IsAnime reports whether a title is anime: tagged with the anime keyword,
// or animated and Japanese (original language ja or origin country JP).
func IsAnime(genres []tmdb.Genre, originalLanguage string, originCountry []string, keywords []tmdb.Keyword) bool {
	for _, k := range keywords {
		if k.ID == AnimeKeywordID {
			return true
		}
	}

	animated := false
	for _, g := range genres {
		if g.ID == tmdb.GenreAnimation {
			animated = true
			break
		}
	}
	if !animated {
		return false
	}

	if originalLanguage == "ja" {
		return true
	}
	for _, c := range originCountry {
		if c == "JP" {
			return true
		}
	}
	return false
}

// DefaultOrdering is the episode ordering a newly added show gets: absolute
// for anime, aired for everything else.
func DefaultOrdering(isAnime bool) library.EpisodeOrdering {
	if isAnime {
		return library.OrderingAbsolute
	}
	return library.OrderingAired
}
