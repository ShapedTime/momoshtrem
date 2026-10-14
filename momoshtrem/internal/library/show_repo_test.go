package library

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// queryCounter counts the queries pgx runs.
type queryCounter struct{ n atomic.Int32 }

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.n.Add(1)
	return ctx
}

func (q *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// openTracedDB opens the test database with a query counter.
func openTracedDB(t *testing.T) (*DB, *queryCounter) {
	t.Helper()
	openTestDB(t) // skips without a database, and migrates
	cfg, err := pgx.ParseConfig(os.Getenv("MOMOSHTREM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{}
	cfg.Tracer = counter
	db := NewDBFromSQL(stdlib.OpenDB(*cfg))
	t.Cleanup(func() { db.Close() })
	return db, counter
}

func TestShowRepository_GetWithSeasonsAndEpisodes(t *testing.T) {
	db, counter := openTracedDB(t)
	ctx := context.Background()
	meta := NewMetadataRepository(db)
	shows := NewShowRepository(db)
	show := seedShow(t, db, "Doctor Who", 2005)

	seasonIDs := map[int]int64{}
	for _, n := range []int{2, 1, 0} {
		s := &Season{ShowID: show.ID, SeasonNumber: n, Name: map[int]string{0: "Specials", 1: "Series 1", 2: "Series 2"}[n]}
		if _, err := meta.UpsertSeason(ctx, s); err != nil {
			t.Fatal(err)
		}
		seasonIDs[n] = s.ID
	}
	mustUpsert := func(season int, eps ...EpisodeMetadata) {
		if _, err := meta.UpsertEpisodes(ctx, seasonIDs[season], eps, false); err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert(0,
		EpisodeMetadata{EpisodeNumber: 1, Name: "The Christmas Invasion", AirDate: "2005-12-25", TMDBEpisodeID: 9001},
		EpisodeMetadata{EpisodeNumber: 2, Name: "The Runaway Bride", AirDate: "2006-12-25"},
	)
	mustUpsert(1,
		EpisodeMetadata{EpisodeNumber: 1, Name: "Rose", AirDate: "2005-03-26", TMDBEpisodeID: 1001, Runtime: 45, EpisodeType: "standard"},
		EpisodeMetadata{EpisodeNumber: 2, Name: "The End of the World", AirDate: "2005-04-02"},
		EpisodeMetadata{EpisodeNumber: 3, Name: "The Unquiet Dead"},
	)
	// Season 2 has no episodes yet.

	ep2, _ := shows.GetEpisode(seasonIDs[1], 2)
	ep3, _ := shows.GetEpisode(seasonIDs[1], 3)
	seedAssignment(t, db, ep2.ID, "old/DW.S01E02.mkv", false)
	activeID := seedAssignment(t, db, ep2.ID, "DW.S01E02.1080p.mkv", true)
	seedAssignment(t, db, ep3.ID, "gone/DW.S01E03.mkv", false)
	if _, err := meta.SetAbsoluteNumbers(ctx, show.ID, []AbsoluteNumber{{EpisodeID: ep2.ID, Number: 2}}); err != nil {
		t.Fatal(err)
	}

	before := counter.n.Load()
	got, err := shows.GetWithSeasonsAndEpisodesContext(ctx, show.ID)
	if err != nil {
		t.Fatalf("GetWithSeasonsAndEpisodes: %v", err)
	}
	if queries := counter.n.Load() - before; queries != 2 {
		t.Errorf("queries = %d, want 2", queries)
	}

	if len(got.Seasons) != 3 {
		t.Fatalf("seasons = %d, want 3", len(got.Seasons))
	}
	for i, want := range []int{0, 1, 2} {
		if got.Seasons[i].SeasonNumber != want || got.Seasons[i].ShowID != show.ID {
			t.Errorf("season %d = %d (show %d), want %d", i, got.Seasons[i].SeasonNumber, got.Seasons[i].ShowID, want)
		}
	}
	specials, s1, s2 := got.Seasons[0], got.Seasons[1], got.Seasons[2]
	if specials.Name != "Specials" || len(specials.Episodes) != 2 || specials.Episodes[0].AirDate != "2005-12-25" {
		t.Errorf("specials = %+v", specials)
	}
	if len(s2.Episodes) != 0 {
		t.Errorf("season 2 episodes = %d, want 0", len(s2.Episodes))
	}
	if len(s1.Episodes) != 3 {
		t.Fatalf("season 1 episodes = %d, want 3", len(s1.Episodes))
	}

	rose, end, unquiet := s1.Episodes[0], s1.Episodes[1], s1.Episodes[2]
	if rose.Name != "Rose" || rose.AirDate != "2005-03-26" || rose.TMDBEpisodeID != 1001 || rose.Runtime != 45 || rose.Assignment != nil {
		t.Errorf("rose = %+v", rose)
	}
	if end.Assignment == nil || end.Assignment.ID != activeID || end.Assignment.FilePath != "DW.S01E02.1080p.mkv" ||
		!end.Assignment.IsActive || end.Assignment.ItemID != end.ID || end.Assignment.Resolution != "1080p" {
		t.Errorf("active assignment = %+v", end.Assignment)
	}
	if end.AbsoluteNumber == nil || *end.AbsoluteNumber != 2 || rose.AbsoluteNumber != nil {
		t.Errorf("absolute numbers = %v, %v", rose.AbsoluteNumber, end.AbsoluteNumber)
	}
	if unquiet.Assignment != nil || unquiet.AirDate != "" {
		t.Errorf("unquiet = %+v (inactive assignments must not show)", unquiet)
	}

	missing, err := shows.GetWithSeasonsAndEpisodesContext(ctx, -1)
	if err != nil || missing != nil {
		t.Errorf("missing show = (%v, %v), want (nil, nil)", missing, err)
	}
}
