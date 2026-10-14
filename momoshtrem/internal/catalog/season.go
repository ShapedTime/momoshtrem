package catalog

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// arcsBudget bounds the TMDB lookups for story arcs. Without them a long
// season still gets numbered blocks.
const arcsBudget = 3 * time.Second

// Season builds GET /titles/tv/{tmdb_id}/seasons/{n}: one season's episodes
// with their state and, for seasons over 20 episodes, jump ranges. Queries:
// the season (one, with the show), plus two TMDB cache reads for story arcs
// when the season is long. Returns ErrNotInLibrary when the show isn't in
// the library and ErrSeasonNotFound for an unknown season, or specials while
// they're switched off.
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

	today := s.today()
	tv := s.torrentView()
	episodes := make([]EpisodeSummary, 0, len(view.Season.Episodes))
	for _, ep := range view.Season.Episodes {
		downloading, done, length, _ := tv.fileProgress(ep.Assignment)
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
			State:          EpisodeStateOf(ep.Assignment != nil, downloading, ep.AirDate, today),
		}
		if downloading {
			summary.Pct = intPtr(pct(done, length))
		}
		if ep.Assignment != nil {
			summary.WebDAVPath = strPtr(vfs.EpisodeFilePath(view.ShowTitle, view.ShowYear, seasonNumber,
				ep.EpisodeNumber, ep.Name, ep.Assignment.FilePath))
		}
		episodes = append(episodes, summary)
	}

	var arcs []Arc
	if len(episodes) > rangeMinEpisodes {
		arcs = s.storyArcs(ctx, tmdbID, seasonNumber)
	}
	return &SeasonDetail{
		SeasonNumber: seasonNumber,
		Name:         SeasonName(seasonNumber, view.Season.Name),
		Episodes:     episodes,
		Ranges:       Ranges(seasonNumber, episodes, arcs),
	}, nil
}

// storyArcs returns the arcs of the show's largest story-arc episode group
// that overlap the season. Failures are logged: blocks stand in for arcs.
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

// episodeType keeps TMDB's known episode types; anything else is null.
func episodeType(t string) *string {
	switch t {
	case "standard", "mid_season", "finale":
		return &t
	}
	return nil
}
