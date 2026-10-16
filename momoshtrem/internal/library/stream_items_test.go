package library

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// A session's items: a movie with its runtime; an episode with its show;
// a file two episodes share plays for both runtimes, and for none when one
// is unknown.
func TestStreamItems(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	show, eps := jobShow(t, db, 4)
	movie := seedMovie(t, db, "Past Lives", 2023)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`UPDATE movies SET runtime = 106 WHERE id = $1`, movie.ID)
	exec(`UPDATE episodes SET runtime = 24 WHERE id = ANY($1::bigint[])`, []int64{eps[1], eps[2], eps[3]})
	hash := strings.Repeat("5", 40)
	assign := func(ep int64, path string, active bool) {
		exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
			VALUES ('episode', $1, $2, 'magnet:?xt=urn:btih:'||$2, $3, 1, $4)`, ep, hash, path, active)
	}
	assign(eps[1], "Pack/E01-E02.mkv", true) // E01 and E02 share a file
	assign(eps[2], "Pack/E01-E02.mkv", true)
	assign(eps[3], "Pack/E03-E04.mkv", true) // E04 has no runtime
	assign(eps[4], "Pack/E03-E04.mkv", true)
	assign(eps[3], "Pack/E01-E02.mkv", false) // inactive: doesn't count

	if got, err := repo.StreamItems(ctx, nil, nil); got != nil || err != nil {
		t.Errorf("nothing asked = %v, %v", got, err)
	}
	got, err := repo.StreamItems(ctx, []int64{movie.ID, 999_999}, []int64{eps[1], eps[3], 888_888})
	if err != nil {
		t.Fatal(err)
	}
	type ref struct {
		typ ItemType
		id  int64
	}
	byItem := map[ref]StreamItem{}
	for _, it := range got {
		byItem[ref{it.ItemType, it.ItemID}] = it
	}
	if len(got) != 3 {
		t.Fatalf("items = %+v, want the movie and two episodes (the rest are gone)", got)
	}
	one := 1
	if m := byItem[ref{ItemTypeMovie, movie.ID}]; m.ItemType != ItemTypeMovie || m.MediaType != JobMovie || m.LibraryID != movie.ID ||
		m.TMDBID != movie.TMDBID || m.Title != "Past Lives" || m.RuntimeMinutes != 106 || m.SeasonNumber != nil {
		t.Errorf("movie = %+v", m)
	}
	want := StreamItem{ItemType: ItemTypeEpisode, ItemID: eps[1], MediaType: JobTV, TMDBID: show.TMDBID, LibraryID: show.ID,
		Title: "Jobs Show", SeasonNumber: &one, EpisodeNumber: &one, EpisodeName: "Episode 1", RuntimeMinutes: 48}
	if e := byItem[ref{ItemTypeEpisode, eps[1]}]; !reflect.DeepEqual(e, want) {
		t.Errorf("E01 = %+v\nwant  %+v", e, want)
	}
	if e := byItem[ref{ItemTypeEpisode, eps[3]}]; e.RuntimeMinutes != 0 || *e.EpisodeNumber != 3 {
		t.Errorf("E03 = %+v, want an unknown runtime (E04 has none)", e)
	}

	// An episode without a torrent file plays for its own runtime.
	exec(`DELETE FROM torrent_assignments`)
	if got, _ := repo.StreamItems(ctx, nil, []int64{eps[2]}); len(got) != 1 || got[0].RuntimeMinutes != 24 {
		t.Errorf("E02 alone = %+v", got)
	}
}

// The detail's job files come from the newest job that listed them,
// preferring one that isn't a dry run.
func TestJobFiles(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 2)
	hash := strings.Repeat("6", 40)

	if got, err := repo.JobFiles(ctx, hash); got != nil || err != nil {
		t.Errorf("no job = %v, %v", got, err)
	}
	ncop := draft("Pack/NCOP.mkv", GroupExtras, DecisionIgnored)
	ncop.ExtraType = "opening"
	sub := draft("Pack/E01.ass", GroupExtras, DecisionAccepted)
	sub.Kind, sub.LinkedPath = KindSubtitle, "Pack/E01.mkv"
	seedJob(t, db, newShowJob(show, hash), JobApplied, []ProposalDraft{
		draft("Pack/E01.mkv", GroupMatched, DecisionAccepted, eps[1]), ncop, sub,
	})
	dry := newShowJob(show, hash)
	dry.DryRun = true
	seedJob(t, db, dry, JobReady, []ProposalDraft{draft("Pack/E01.mkv", GroupMatched, DecisionIgnored, eps[1])})

	got, err := repo.JobFiles(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	want := []JobFile{
		{Path: "Pack/E01.mkv", Size: 1, Kind: KindEpisode, Decision: DecisionAccepted},
		{Path: "Pack/NCOP.mkv", Size: 1, Kind: KindExtra, ExtraType: "opening", Decision: DecisionIgnored},
		{Path: "Pack/E01.ass", Size: 1, Kind: KindSubtitle, Decision: DecisionAccepted, Linked: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files =\n%+v\nwant (the applied job's, not the newer dry run's)\n%+v", got, want)
	}
}
