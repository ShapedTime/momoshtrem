package service

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// maxAliases caps the alternative titles a catalog carries.
const maxAliases = 20

// CatalogExtras is what TMDB knows about a show beyond the library: its
// alternative titles (romaji fansub names such as "Sousou no Frieren" are
// TMDB alternative titles, not its original name) and episode groups
// (Blu-ray and other orderings). Both are optional.
type CatalogExtras struct {
	AlternativeTitles []tmdb.AlternativeTitle
	EpisodeGroups     []*tmdb.EpisodeGroup
}

// ShowCatalog builds the classifier's view of a library show loaded with
// ShowRepository.GetWithSeasonsAndEpisodesContext: its names, its origin
// countries (for "The.Office.US"-style tags), its numbering and every
// episode (ID, season, number, absolute number, name, air date and type),
// plus the aliases and alternative orderings extras gives.
//
// Season 0 is left out when the show's specials are off, as the title page
// hides it: specials then come out unmatched ("Specials are off for this
// show") instead of being assigned.
func ShowCatalog(show *library.Show, extras CatalogExtras) *identify.ShowCatalog {
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
	cat.Aliases = aliases(extras.AlternativeTitles, cat.Title, cat.OriginalTitle)
	cat.AltOrderings = altOrderings(extras.EpisodeGroups, cat.Episodes)
	return cat
}

// aliases keeps the alternative titles a file name could use: written in
// Latin letters (file names are), not the title again, each once.
func aliases(titles []tmdb.AlternativeTitle, known ...string) []string {
	seen := map[string]bool{}
	for _, k := range known {
		seen[strings.ToLower(strings.TrimSpace(k))] = true
	}
	var out []string
	for _, t := range titles {
		title := strings.TrimSpace(t.Title)
		key := strings.ToLower(title)
		if title == "" || seen[key] || !hasLatinLetter(title) {
			continue
		}
		seen[key] = true
		out = append(out, title)
		if len(out) == maxAliases {
			break
		}
	}
	return out
}

func hasLatinLetter(s string) bool {
	for _, r := range s {
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// altOrderings turns TMDB episode groups into the classifier's alternative
// orderings: each group of a group (a season of a Blu-ray order, say) is a
// release season, numbered from its name ("Season 2", "Specials" is 0) or,
// failing that, by its position. Its episodes are the catalog's, in the
// group's order; the list stops at the first episode the catalog doesn't
// have, so positions stay right.
func altOrderings(groups []*tmdb.EpisodeGroup, episodes []identify.Episode) []identify.AltOrdering {
	type seKey struct{ season, number int }
	ids := make(map[seKey]int64, len(episodes))
	for _, ep := range episodes {
		ids[seKey{ep.Season, ep.Number}] = ep.ID
	}

	var out []identify.AltOrdering
	for _, g := range groups {
		if g == nil {
			continue
		}
		parts := append([]tmdb.EpisodeGroupGroup(nil), g.Groups...)
		sort.SliceStable(parts, func(i, j int) bool { return parts[i].Order < parts[j].Order })
		ordering := identify.AltOrdering{Name: g.Name}
		position := 0
		for _, part := range parts {
			number, special := groupSeason(part.Name)
			if !special {
				position++
				if number < 0 {
					number = position
				}
			}
			eps := append([]tmdb.GroupEpisode(nil), part.Episodes...)
			sort.SliceStable(eps, func(i, j int) bool { return eps[i].Order < eps[j].Order })
			var episodeIDs []int64
			for _, ep := range eps {
				id, ok := ids[seKey{ep.SeasonNumber, ep.EpisodeNumber}]
				if !ok {
					break
				}
				episodeIDs = append(episodeIDs, id)
			}
			if len(episodeIDs) > 0 {
				ordering.Groups = append(ordering.Groups, identify.AltGroup{Number: number, EpisodeIDs: episodeIDs})
			}
		}
		if len(ordering.Groups) > 0 {
			out = append(out, ordering)
		}
	}
	return out
}

var (
	reGroupSeason  = regexp.MustCompile(`(?i)\b(?:season|series|part|cour|staffel|saison|temporada|stagione)\s*0*(\d{1,3})\b`)
	reGroupNumber  = regexp.MustCompile(`^\s*0*(\d{1,3})\s*$`)
	reGroupSpecial = regexp.MustCompile(`(?i)\b(?:specials?|extras?|ova)\b`)
)

// groupSeason reads a release season from an episode group's part name:
// the number in "Season 2" (or a bare "2"), 0 for "Specials". It returns
// -1 when the name gives none; special reports a specials part.
func groupSeason(name string) (number int, special bool) {
	if m := reGroupSeason.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, n == 0
	}
	if m := reGroupNumber.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, n == 0
	}
	if reGroupSpecial.MatchString(name) {
		return 0, true
	}
	return -1, false
}
