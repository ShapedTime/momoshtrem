package metadata

import (
	"context"
	"errors"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// BackfillOptions configures Backfill.
type BackfillOptions struct {
	// DryRun lists what would be processed without calling TMDB or writing.
	DryRun bool
	// Force reprocesses items that already have metadata.
	Force bool
	// Only restricts the run to movies or shows; "" does both.
	Only library.MediaKind
	// Limit caps the items processed per kind; 0 means no limit.
	Limit int
}

// BackfillEvent reports one processed item.
type BackfillEvent struct {
	Kind   library.MediaKind
	Index  int // 1-based position within Kind
	Total  int // Items of Kind in this run
	Item   library.BackfillItem
	DryRun bool
	// EpisodesAdded counts episodes the show didn't have yet (specials,
	// newly aired); 0 for movies.
	EpisodesAdded int
	Err           error // nil on success
}

// BackfillSummary totals a run.
type BackfillSummary struct {
	Movies        int // Movies processed (or listed, in a dry run)
	Shows         int
	Failed        int
	EpisodesAdded int
}

// Backfill writes TMDB metadata onto movies and shows that predate it. It is
// resumable: an item is stamped (metadata_updated_at) only once it fully
// synced, and a rerun picks up the rest. It never renames anything WebDAV
// shows: titles, years and existing episode names are kept, and only new
// episodes get TMDB's names. Shows also get their specials, absolute numbers
// and, on their first sync, anime's default ordering.
//
// A failing item is reported through progress and counted; the run goes on.
// Backfill returns an error only when it can't list the items or ctx ends.
func (s *Syncer) Backfill(ctx context.Context, opts BackfillOptions, progress func(BackfillEvent)) (BackfillSummary, error) {
	if progress == nil {
		progress = func(BackfillEvent) {}
	}
	var summary BackfillSummary

	kinds := []library.MediaKind{library.MediaMovie, library.MediaShow}
	if opts.Only != "" {
		kinds = []library.MediaKind{opts.Only}
	}

	for _, kind := range kinds {
		items, err := s.meta.ListForBackfill(ctx, kind, opts.Force, opts.Limit)
		if err != nil {
			return summary, err
		}

		for i, item := range items {
			if err := ctx.Err(); err != nil {
				return summary, err
			}

			ev := BackfillEvent{Kind: kind, Index: i + 1, Total: len(items), Item: item, DryRun: opts.DryRun}
			if !opts.DryRun {
				switch kind {
				case library.MediaMovie:
					ev.Err = s.backfillMovie(ctx, item)
				case library.MediaShow:
					ev.EpisodesAdded, ev.Err = s.backfillShow(ctx, item)
				}
			}
			if ev.Err != nil && ctx.Err() != nil {
				return summary, ctx.Err()
			}

			switch {
			case ev.Err != nil:
				summary.Failed++
			case kind == library.MediaMovie:
				summary.Movies++
			default:
				summary.Shows++
				summary.EpisodesAdded += ev.EpisodesAdded
			}
			progress(ev)
		}
	}
	return summary, nil
}

func (s *Syncer) backfillMovie(ctx context.Context, item library.BackfillItem) error {
	details, err := s.tmdb.MovieDetails(ctx, item.TMDBID)
	if err != nil {
		return tmdbError(err)
	}
	return s.meta.UpdateMovieMetadata(ctx, item.ID, movieMetadata(details))
}

func (s *Syncer) backfillShow(ctx context.Context, item library.BackfillItem) (episodesAdded int, err error) {
	show, err := s.shows.GetByID(item.ID)
	if err != nil {
		return 0, err
	}
	if show == nil {
		return 0, library.ErrShowNotFound // Deleted since it was listed
	}

	details, err := s.tmdb.ShowDetails(ctx, show.TMDBID)
	if err != nil {
		return 0, tmdbError(err)
	}
	if err := s.updateShowMetadata(ctx, show, details); err != nil {
		return 0, err
	}

	result, err := s.syncShow(ctx, show, details.Status, seasonsToSync(details, show.IncludeSpecials), true)
	if err != nil {
		return 0, err
	}
	if !result.Complete() {
		errs := make([]error, 0, len(result.SeasonErrors)+1)
		for _, se := range result.SeasonErrors {
			errs = append(errs, se)
		}
		if result.NumberingError != nil {
			errs = append(errs, fmt.Errorf("absolute numbers: %w", result.NumberingError))
		}
		return result.EpisodesAdded, errors.Join(errs...)
	}
	return result.EpisodesAdded, nil
}
