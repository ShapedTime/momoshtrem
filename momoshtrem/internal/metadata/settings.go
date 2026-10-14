package metadata

import (
	"context"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// SettingsPatch changes some of a show's settings; nil fields are kept.
// An EpisodeGroupID of "" clears the group.
type SettingsPatch struct {
	EpisodeOrdering *library.EpisodeOrdering
	EpisodeGroupID  *string
	IncludeSpecials *bool
	ClassifierMode  *library.ClassifierMode
}

// ApplySettings validates and stores a show's settings, then brings the
// library in line with them:
//   - a new ordering or group renumbers the episodes;
//   - turning specials on fetches season 0 (turning them off keeps the rows;
//     readers hide them).
//
// Invalid values return library.ErrInvalidShowSettings: an unknown ordering
// or classifier mode, episode_group ordering without a group, or a group that
// isn't one of the show's TMDB episode groups. It returns the show reloaded
// with its seasons and episodes.
func (s *Syncer) ApplySettings(ctx context.Context, showID int64, patch SettingsPatch) (*library.Show, error) {
	show, err := s.shows.GetByID(showID)
	if err != nil {
		return nil, err
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}

	before := show.Settings()
	next := before
	if patch.EpisodeOrdering != nil {
		next.EpisodeOrdering = *patch.EpisodeOrdering
	}
	if patch.EpisodeGroupID != nil {
		next.EpisodeGroupID = *patch.EpisodeGroupID
	}
	if patch.IncludeSpecials != nil {
		next.IncludeSpecials = *patch.IncludeSpecials
	}
	if patch.ClassifierMode != nil {
		next.ClassifierMode = *patch.ClassifierMode
	}

	if err := s.validateSettings(ctx, show, before, next); err != nil {
		return nil, err
	}
	if err := s.meta.UpdateShowSettings(ctx, show.ID, next); err != nil {
		return nil, err
	}

	specialsOn := next.IncludeSpecials && !before.IncludeSpecials
	if specialsOn {
		s.fetchSpecials(ctx, show)
	}

	full, err := s.shows.GetWithSeasonsAndEpisodesContext(ctx, show.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload show: %w", err)
	}
	if full == nil {
		return nil, library.ErrShowNotFound
	}

	if specialsOn || next.EpisodeOrdering != before.EpisodeOrdering || next.EpisodeGroupID != before.EpisodeGroupID {
		if err := s.renumber(ctx, full); err != nil {
			// The settings are saved; the next refresh renumbers.
			s.log.Warn("Failed to renumber after a settings change", "show_id", show.ID, "error", err)
		}
	}
	return full, nil
}

func (s *Syncer) validateSettings(ctx context.Context, show *library.Show, before, next library.ShowSettings) error {
	if !next.EpisodeOrdering.Valid() {
		return fmt.Errorf("%w: unknown episode ordering %q", library.ErrInvalidShowSettings, next.EpisodeOrdering)
	}
	if !next.ClassifierMode.Valid() {
		return fmt.Errorf("%w: unknown classifier mode %q", library.ErrInvalidShowSettings, next.ClassifierMode)
	}
	if next.EpisodeOrdering == library.OrderingEpisodeGroup && next.EpisodeGroupID == "" {
		return fmt.Errorf("%w: episode_group ordering needs an episode_group_id", library.ErrInvalidShowSettings)
	}
	if next.EpisodeGroupID == "" || next.EpisodeGroupID == before.EpisodeGroupID {
		return nil
	}

	groups, err := s.tmdb.EpisodeGroups(ctx, show.TMDBID)
	if err != nil {
		return tmdbError(err)
	}
	for _, g := range groups {
		if g.ID == next.EpisodeGroupID {
			return nil
		}
	}
	return fmt.Errorf("%w: episode group %q is not one of this show's", library.ErrInvalidShowSettings, next.EpisodeGroupID)
}

// fetchSpecials stores season 0 if TMDB has one. Failures are logged: the
// settings are already saved and the next refresh retries.
func (s *Syncer) fetchSpecials(ctx context.Context, show *library.Show) {
	details, err := s.tmdb.ShowDetails(ctx, show.TMDBID)
	if err != nil {
		s.log.Warn("Failed to fetch show for specials", "show_id", show.ID, "error", err)
		return
	}
	for _, n := range details.SeasonNumbers() {
		if n != 0 {
			continue
		}
		res := s.syncSeasons(ctx, show, details.Status, []int{0}, false)
		for _, se := range res.errors {
			s.log.Warn("Failed to fetch specials", "show_id", show.ID, "error", se.Err)
		}
		return
	}
}
