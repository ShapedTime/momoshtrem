package catalog

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// arcsBudget bounds the TMDB lookups for story arcs. Without them a long
// season still gets numbered blocks.
const arcsBudget = 3 * time.Second

// Season builds GET /titles/tv/{tmdb_id}/seasons/{n}: one season's episodes
// with their state and, for seasons over 20 episodes, jump ranges. Queries:
// the season (one, with the show and Trakt's plays), the show's mapping
// review, plus two TMDB cache reads for story arcs when a regular season is
// long. Returns
// ErrNotInLibrary when the show isn't in the library and ErrSeasonNotFound
// for an unknown season, or specials while they're switched off.
func (s *Service) Season(ctx context.Context, tmdbID, seasonNumber int) (*SeasonDetail, error) {
	view, err := s.store.SeasonByTMDBID(ctx, tmdbID, seasonNumber)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, ErrNotInLibrary
	}
	if view.Season == nil || (seasonNumber == 0 && !view.IncludeSpecials) {
		return nil, ErrSeasonNotFound
	}
	rev, err := s.reviewOf(ctx, string(library.JobTV), view.ShowID)
	if err != nil {
		return nil, err
	}

	today := s.today()
	tv := s.torrentView()
	paths := EpisodePaths(view.ShowTitle, view.ShowYear, seasonNumber, view.Season.Episodes)
	episodes := make([]EpisodeSummary, 0, len(view.Season.Episodes))
	for _, ep := range view.Season.Episodes {
		f, downloading := tv.fileDownload(ep.Assignment)
		summary := EpisodeSummary{
			ID:             ep.ID,
			SeasonNumber:   seasonNumber,
			EpisodeNumber:  ep.EpisodeNumber,
			AbsoluteNumber: copyInt(ep.AbsoluteNumber),
			Name:           ep.Name,
			Overview:       ep.Overview,
			AirDate:        strPtr(ep.AirDate),
			StillPath:      strPtr(ep.StillPath),
			Runtime:        positive(ep.Runtime),
			EpisodeType:    episodeType(ep.EpisodeType),
			State:          EpisodeStateOf(ep.Watched, ep.Assignment != nil, downloading, rev.underReview(ep.ID), ep.AirDate, today),
			Keep:           keepOf(tv.downloads.Episode(ep.ID)),
		}
		if downloading {
			summary.Pct = intPtr(f.Pct())
		}
		if ep.Assignment != nil {
			summary.WebDAVPath = strPtr(paths[ep.ID])
			summary.Source, summary.Stream = sourceOf(ep.Assignment), episodeStream(ep.Assignment)
		}
		episodes = append(episodes, summary)
	}

	var arcs []Arc
	if seasonNumber > 0 && len(episodes) > rangeMinEpisodes { // specials always get blocks
		arcs = s.storyArcs(ctx, tmdbID, seasonNumber)
	}
	return &SeasonDetail{
		SeasonNumber: seasonNumber,
		Name:         SeasonName(seasonNumber, view.Season.Name),
		Episodes:     episodes,
		Ranges:       Ranges(seasonNumber, episodes, arcs),
		Keep:         keepOf(tv.downloads.Season(view.ShowID, seasonNumber)),
	}, nil
}

// storyArcs returns the arcs of the show's largest story-arc episode group
// that list episodes of the season. Failures are logged: blocks stand in for
// arcs.
func (s *Service) storyArcs(ctx context.Context, tmdbID, seasonNumber int) []Arc {
	ctx, cancel := context.WithTimeout(ctx, arcsBudget)
	defer cancel()

	groups, err := s.tmdb.EpisodeGroups(ctx, tmdbID)
	if err != nil {
		s.log.Warn("Episode groups unavailable; using blocks", "tmdb_id", tmdbID, "error", err)
		return nil
	}
	best, bestCount := "", 0
	for _, g := range groups {
		if g.Type == tmdb.GroupStoryArc && g.EpisodeCount > bestCount {
			best, bestCount = g.ID, g.EpisodeCount
		}
	}
	if best == "" {
		return nil
	}
	group, err := s.tmdb.EpisodeGroup(ctx, best)
	if err != nil {
		s.log.Warn("Story arcs unavailable; using blocks", "tmdb_id", tmdbID, "group", best, "error", err)
		return nil
	}
	return ArcsForSeason(group, seasonNumber)
}

// EpisodePaths is each assigned episode's WebDAV path, by episode id, as
// the tree names it: a file a job mapped to several episodes is one
// "S01E01-E02" entry they share (vfs.SeasonEntries).
func EpisodePaths(showTitle string, showYear, season int, episodes []library.Episode) map[int64]string {
	var ids []int64
	var eps []vfs.SeasonEpisode
	for _, ep := range episodes {
		if ep.Assignment == nil {
			continue
		}
		ids = append(ids, ep.ID)
		eps = append(eps, vfs.SeasonEpisode{
			Number: ep.EpisodeNumber, Name: ep.Name,
			InfoHash: ep.Assignment.InfoHash, FilePath: ep.Assignment.FilePath,
			Grouped: ep.Assignment.Groupable(),
		})
	}
	out := make(map[int64]string, len(ids))
	for i, p := range vfs.SeasonEpisodePaths(showTitle, showYear, season, eps) {
		out[ids[i]] = p
	}
	return out
}

// episodeType keeps TMDB's known episode types; anything else is null.
func episodeType(t string) *string {
	switch t {
	case "standard", "mid_season", "finale":
		return &t
	}
	return nil
}
