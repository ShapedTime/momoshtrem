package activity_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/activity"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// The detail of a torrent the client hasn't loaded, over Postgres: its
// files from the newest job's list, then from its stored metainfo, with
// what each plays as and why the job left the rest out. Nothing loads it.
func TestDetail_NotLoaded(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	f := &fixture{t: t, ctx: ctx, db: db, jobs: library.NewAssignmentJobRepository(db), settings: library.NewSettingsRepository(db)}

	info := metainfo.Info{Name: "Frieren.S01", PieceLength: 1 << 18, Pieces: make([]byte, 20*4), Files: []metainfo.FileInfo{
		{Path: []string{"Frieren - 01.mkv"}, Length: 400_000},
		{Path: []string{"Frieren - 02.mkv"}, Length: 410_000},
		{Path: []string{"NCOP.mkv"}, Length: 90_000},
	}}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes, AnnounceList: [][]string{{"http://nyaa.tracker.wf:7777/announce"}}}
	hash := mi.HashInfoBytes().HexString()

	show := f.id(`INSERT INTO shows (tmdb_id, title, year) VALUES (209867, 'Frieren', 2023) RETURNING id`)
	season := f.id(`INSERT INTO seasons (show_id, season_number) VALUES ($1, 1) RETURNING id`, show)
	e1 := f.id(`INSERT INTO episodes (season_id, episode_number, name) VALUES ($1, 1, 'The Journey''s End') RETURNING id`, season)
	e2 := f.id(`INSERT INTO episodes (season_id, episode_number) VALUES ($1, 2) RETURNING id`, season)
	f.assign("episode", e1, hash, "Frieren.S01", "Frieren.S01/Frieren - 01.mkv")
	f.assign("episode", e2, hash, "Frieren.S01", "Frieren.S01/Frieren - 02.mkv")

	job := &library.AssignmentJob{MediaType: library.JobTV, TMDBID: 209867, ShowID: &show, InfoHash: hash,
		MagnetURI: "magnet:?xt=urn:btih:" + hash, TorrentName: "Frieren.S01"}
	if _, err := f.jobs.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	drafts := []library.ProposalDraft{
		{FilePath: "Frieren.S01/Frieren - 01.mkv", FileSize: 400_000, Kind: library.KindEpisode, Group: library.GroupMatched,
			EpisodeIDs: []int64{e1}, Source: library.SourceRules, Decision: library.DecisionAccepted},
		{FilePath: "Frieren.S01/Frieren - 02.mkv", FileSize: 410_000, Kind: library.KindEpisode, Group: library.GroupMatched,
			EpisodeIDs: []int64{e2}, Source: library.SourceRules, Decision: library.DecisionAccepted},
		{FilePath: "Frieren.S01/NCOP.mkv", FileSize: 90_000, Kind: library.KindExtra, Group: library.GroupExtras, ExtraType: "opening",
			Source: library.SourceRules, Decision: library.DecisionIgnored},
	}
	if err := f.jobs.Locked(ctx, job.ID, func(tx *library.JobTx) error {
		if err := tx.ReplaceProposals(drafts); err != nil {
			return err
		}
		tx.Job.Status = library.JobApplied
		return tx.SaveJob()
	}); err != nil {
		t.Fatal(err)
	}

	client := torrent.NewService(nil, nil, time.Second, time.Second)
	check := func(t *testing.T, d *activity.Detail, wantSizes []int64) {
		t.Helper()
		if d.InfoHash != hash || d.Loaded || d.State != activity.StateIdle || d.Pieces != nil || d.PiecesHave != nil ||
			d.Swarm != nil || d.DownloadedBytes != nil || d.MagnetURI == nil || !strings.HasPrefix(*d.MagnetURI, "magnet:?xt=urn:btih:"+hash) {
			t.Errorf("detail = %+v", d)
		}
		if len(d.Files) != 3 {
			t.Fatalf("files = %+v", d.Files)
		}
		for i, df := range d.Files {
			if df.SizeBytes != wantSizes[i] || df.Progress != nil {
				t.Errorf("file %d = %+v", i, df)
			}
		}
		if p := d.Files[0].PlaysAs; len(p) != 1 || p[0].Label != "S01E01" || p[0].ItemID != e1 || p[0].EpisodeName == nil ||
			*p[0].EpisodeName != "The Journey's End" {
			t.Errorf("E01 plays as %+v", p)
		}
		if p := d.Files[1].PlaysAs; len(p) != 1 || p[0].Label != "S01E02" || p[0].EpisodeName != nil {
			t.Errorf("E02 plays as %+v", p)
		}
		if ig := d.Files[2].Ignored; ig == nil || *ig != (activity.Ignored{Reason: "opening", Label: "Ignored (NCOP)"}) || len(d.Files[2].PlaysAs) != 0 {
			t.Errorf("NCOP = %+v", d.Files[2])
		}
	}

	t.Run("from the job's file list", func(t *testing.T) {
		svc := activity.New(activity.Deps{Torrents: client, Library: f.jobs, Pauses: f.settings})
		d, err := svc.Detail(ctx, strings.ToUpper(hash))
		if err != nil {
			t.Fatal(err)
		}
		check(t, d, []int64{400_000, 410_000, 90_000})
		if d.PieceLength != nil || d.PiecesTotal != nil || len(d.Trackers) != 0 {
			t.Errorf("without metainfo: pieces %v/%v, trackers %+v", d.PieceLength, d.PiecesTotal, d.Trackers)
		}
	})

	t.Run("from the stored metainfo", func(t *testing.T) {
		store := torrent.NewMetainfoStore(filepath.Join(t.TempDir(), "metainfo"))
		if err := store.Save(hash, mi); err != nil {
			t.Fatal(err)
		}
		svc := activity.New(activity.Deps{Torrents: client, Library: f.jobs, Pauses: f.settings, Metainfo: store})
		d, err := svc.Detail(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		check(t, d, []int64{400_000, 410_000, 90_000})
		if d.PieceLength == nil || *d.PieceLength != 1<<18 || d.PiecesTotal == nil || *d.PiecesTotal != 4 {
			t.Errorf("pieces: %v of %v", d.PiecesTotal, d.PieceLength)
		}
		if len(d.Trackers) != 1 || d.Trackers[0] != (activity.Tracker{URL: "http://nyaa.tracker.wf:7777/announce", Status: "unknown"}) {
			t.Errorf("trackers = %+v", d.Trackers)
		}
	})

	if _, err := activity.New(activity.Deps{Torrents: client, Library: f.jobs, Pauses: f.settings}).Detail(ctx, strings.Repeat("9", 40)); !errors.Is(err, torrent.ErrTorrentNotFound) {
		t.Errorf("an unknown torrent = %v, want ErrTorrentNotFound", err)
	}
	if st, _ := client.ListTorrents(); len(st) != 0 {
		t.Errorf("the detail loaded %d torrents", len(st))
	}
}
