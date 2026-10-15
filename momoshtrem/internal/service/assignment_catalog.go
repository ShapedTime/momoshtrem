package service

import (
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// ShowCatalog builds the classifier's view of a library show loaded with
// ShowRepository.GetWithSeasonsAndEpisodesContext: its names, its origin
// countries (for "The.Office.US"-style tags), its numbering and every
// episode (ID, season, number, absolute number, name, air date and type).
//
// Season 0 is left out when the show's specials are off, as the title page
// hides it: specials then come out unmatched ("Specials are off for this
// show") instead of being assigned. Episode-group orderings (AltOrderings)
// aren't stored in the library, so none are set here.
func ShowCatalog(show *library.Show) *identify.ShowCatalog {
	cat := &identify.ShowCatalog{
		Title:         show.Title,
		OriginalTitle: show.OriginalName,
		Year:          show.Year,
		IsAnime:       show.IsAnime,
		Ordering:      identify.Ordering(show.EpisodeOrdering),
		Countries:     show.OriginCountry,
	}
	if cat.OriginalTitle == cat.Title {
		cat.OriginalTitle = ""
	}
	for _, season := range show.Seasons {
		if season.SeasonNumber == 0 && !show.IncludeSpecials {
			continue
		}
		for _, ep := range season.Episodes {
			e := identify.Episode{
				ID:      ep.ID,
				Season:  season.SeasonNumber,
				Number:  ep.EpisodeNumber,
				Name:    ep.Name,
				AirDate: ep.AirDate,
				Type:    ep.EpisodeType,
			}
			if ep.AbsoluteNumber != nil {
				e.Absolute = *ep.AbsoluteNumber
			}
			cat.Episodes = append(cat.Episodes, e)
		}
	}
	return cat
}
