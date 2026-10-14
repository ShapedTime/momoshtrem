package catalog

import (
	"context"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// maxSimilar is how many "More like this" titles a page shows.
const maxSimilar = 12

// similar lists "More like this": TMDB's recommendations, else its similar
// titles, at most 12, each with its place in the library (one query for all
// of them). mediaType is the page's; lists hold titles of the same type.
func (s *Service) similar(ctx context.Context, mediaType string, recommendations, similar []tmdb.ListItem) ([]SimilarTitle, error) {
	items := recommendations
	if len(items) == 0 {
		items = similar
	}
	if len(items) > maxSimilar {
		items = items[:maxSimilar]
	}
	if len(items) == 0 {
		return []SimilarTitle{}, nil
	}

	titles := make([]SimilarTitle, 0, len(items))
	var ids library.TMDBIDs
	for _, item := range items {
		t := item.MediaType
		if t != "movie" && t != "tv" {
			t = mediaType
		}
		if t == "movie" {
			ids.Movies = append(ids.Movies, item.ID)
		} else {
			ids.Shows = append(ids.Shows, item.ID)
		}
		titles = append(titles, SimilarTitle{
			MediaType:  t,
			TMDBID:     item.ID,
			Title:      item.DisplayTitle(),
			Year:       yearPtr(item.Year()),
			PosterPath: strPtr(item.PosterPath),
		})
	}

	rows, _, err := s.store.LibraryItems(ctx, library.LibraryQuery{
		Today:       s.today(),
		Downloading: s.torrentView().downloadingFiles(),
		OnlyTMDB:    &ids,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to annotate similar titles: %w", err)
	}
	type key struct {
		mediaType string
		tmdbID    int
	}
	inLibrary := make(map[key]library.LibraryItem, len(rows))
	for _, row := range rows {
		inLibrary[key{row.MediaType, row.TMDBID}] = row
	}
	for i := range titles {
		if row, ok := inLibrary[key{titles[i].MediaType, titles[i].TMDBID}]; ok {
			titles[i].Library = &SimilarLibrary{
				LibraryID:         row.ID,
				AvailabilityStage: Stage(row.Stage),
				EpisodesTotal:     copyInt(row.EpisodesTotal),
				EpisodesReady:     copyInt(row.EpisodesReady),
			}
		}
	}
	return titles, nil
}
