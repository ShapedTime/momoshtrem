package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// backfillFlags are the parsed `backfill-metadata` flags.
type backfillFlags struct {
	opts metadata.BackfillOptions
	rps  float64
}

// parseBackfillFlags parses the subcommand's flags before anything touches
// the database. ok is false when the process should exit with code (0 after
// -h, 2 for bad flags).
func parseBackfillFlags(args []string) (f backfillFlags, code int, ok bool) {
	fs := flag.NewFlagSet(backfillCommand, flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "list what would be backfilled, without calling TMDB or writing")
	force := fs.Bool("force", false, "also reprocess movies and shows that already have metadata")
	only := fs.String("only", "", "only backfill `movies` or shows")
	limit := fs.Int("limit", 0, "process at most `N` movies and N shows (0 means all)")
	rps := fs.Float64("rps", 10, "TMDB requests per second")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return f, 0, false
		}
		return f, 2, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", fs.Args())
		return f, 2, false
	}

	switch *only {
	case "":
	case "movies":
		f.opts.Only = library.MediaMovie
	case "shows":
		f.opts.Only = library.MediaShow
	default:
		fmt.Fprintf(os.Stderr, "invalid --only %q: want movies or shows\n", *only)
		return f, 2, false
	}
	if *limit < 0 || *rps <= 0 {
		fmt.Fprintln(os.Stderr, "--limit must be 0 or more and --rps more than 0")
		return f, 2, false
	}

	f.opts.DryRun, f.opts.Force, f.opts.Limit = *dryRun, *force, *limit
	f.rps = *rps
	return f, 0, true
}

// runBackfill runs `momoshtrem backfill-metadata` and returns the exit code:
// 0 when everything was backfilled, 1 when something failed (a rerun
// resumes).
func runBackfill(cfg *config.Config, db *library.DB, f backfillFlags) int {
	if cfg.TMDB.APIKey == "" && !f.opts.DryRun {
		slog.Error("TMDB_API_KEY is required to backfill metadata")
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := tmdb.NewClient(cfg.TMDB.APIKey, tmdb.WithLanguage(cfg.TMDB.Language), tmdb.WithRateLimit(f.rps))
	syncer := metadata.NewSyncer(
		tmdb.NewCached(client, library.NewTMDBCacheRepository(db)),
		library.NewMovieRepository(db),
		library.NewShowRepository(db),
		library.NewMetadataRepository(db),
	)

	slog.Info("Backfilling metadata",
		"dry_run", f.opts.DryRun, "force", f.opts.Force, "only", f.opts.Only, "limit", f.opts.Limit,
		"rps", f.rps, "language", cfg.TMDB.Language)

	summary, err := syncer.Backfill(ctx, f.opts, logBackfillEvent)

	slog.Info("Backfill finished",
		"dry_run", f.opts.DryRun,
		"movies", summary.Movies,
		"shows", summary.Shows,
		"episodes_added", summary.EpisodesAdded,
		"failed", summary.Failed,
	)
	switch {
	case err != nil:
		slog.Error("Backfill stopped; run it again to resume", "error", err)
		return 1
	case summary.Failed > 0:
		slog.Warn("Some items failed; run it again to retry them", "failed", summary.Failed)
		return 1
	}
	return 0
}

func logBackfillEvent(ev metadata.BackfillEvent) {
	attrs := []any{
		"kind", ev.Kind,
		"progress", fmt.Sprintf("%d/%d", ev.Index, ev.Total),
		"id", ev.Item.ID,
		"tmdb_id", ev.Item.TMDBID,
		"title", ev.Item.Title,
		"year", ev.Item.Year,
	}
	switch {
	case ev.Err != nil:
		slog.Warn("Backfill failed", append(attrs, "error", ev.Err)...)
	case ev.DryRun:
		slog.Info("Would backfill", attrs...)
	case ev.Kind == library.MediaShow:
		slog.Info("Backfilled", append(attrs, "episodes_added", ev.EpisodesAdded)...)
	default:
		slog.Info("Backfilled", attrs...)
	}
}
