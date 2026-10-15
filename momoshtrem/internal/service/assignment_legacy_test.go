package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// waitFor polls cond for up to five seconds.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The legacy path (?wait=true) assigns everything the classifier places:
// a multi-episode file gets a row per episode, a special goes to season 0,
// the better of two copies is kept, extras are skipped, subtitles are
// created, and two different files on one episode are both left out (their
// proposals stay pending) while the job ends applied.
func TestAssignShowAndWait_LegacySemantics(t *testing.T) {
	hash := strings.Repeat("c", 40)
	root := "Legacy.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Legacy.Show.S01E01E02.1080p.WEB-GRP.mkv", 2*gb),
		file("1080p/Legacy.Show.S01E03.1080p.WEB-GRP.mkv", gb),
		file("720p/Legacy.Show.S01E03.720p.WEB-GRP.mkv", gb/2),
		file("Legacy.Show.S01E04.Fourth.1080p.WEB-GRP.mkv", gb),
		file("Legacy.Show.S01E04.Extended.Cut.1080p.WEB-GRP.mkv", gb),
		file("Legacy.Show.S00E01.Christmas.Special.1080p.WEB-GRP.mkv", gb),
		file("Legacy.Show.S01E05.1080p.WEB-GRP.en.srt", 40<<10),
		file("Sample/legacy.show.s01e01.1080p-sample.mkv", 30<<20),
		file("Legacy.Show.S01.1080p.WEB-GRP.nfo", 2<<10),
	))
	e.start()
	showID, ids := e.show("Legacy Show", [][3]any{
		{0, 1, "Christmas Special"},
		{1, 1, "Pilot"}, {1, 2, "Second"}, {1, 3, "Third"}, {1, 4, "Fourth"}, {1, 5, "Fifth"},
	})

	res, err := e.svc.AssignShowAndWait(e.ctx, showID, AssignRequest{MagnetURI: magnet(hash) + "&dn=" + root})
	if err != nil {
		t.Fatal(err)
	}
	want := AssignmentSummary{TotalFiles: 8, Matched: 4, Unmatched: 2, Skipped: 2, SubtitlesFound: 1}
	if res.Summary != want {
		t.Errorf("summary = %+v, want %+v\nunmatched %+v", res.Summary, want, res.Unmatched)
	}
	if res.JobID == 0 || e.job(res.JobID).Status != library.JobApplied {
		t.Errorf("job %d = %s, want applied", res.JobID, e.job(res.JobID).Status)
	}

	for _, label := range []string{"S01E01", "S01E02"} {
		a := e.active(library.ItemTypeEpisode, ids[label])
		if a == nil || a.FilePath != root+"/Legacy.Show.S01E01E02.1080p.WEB-GRP.mkv" || a.Resolution != "1080p" {
			t.Errorf("%s assignment = %+v", label, a)
		}
	}
	if a := e.active(library.ItemTypeEpisode, ids["S00E01"]); a == nil {
		t.Error("the special wasn't assigned")
	}
	if a := e.active(library.ItemTypeEpisode, ids["S01E03"]); a == nil || a.FilePath != root+"/1080p/Legacy.Show.S01E03.1080p.WEB-GRP.mkv" {
		t.Errorf("S01E03 assignment = %+v, want the 1080p copy", a)
	}
	if a := e.active(library.ItemTypeEpisode, ids["S01E04"]); a != nil {
		t.Errorf("a conflicting file was assigned: %+v", a)
	}
	for _, u := range res.Unmatched {
		if !strings.HasPrefix(u.Reason, "Conflicts with ") || u.Season != -1 || len([]rune(u.Reason)) > identify.MaxReasonRunes {
			t.Errorf("unmatched %+v", u)
		}
	}
	if subs := e.subs.all(); len(subs) != 1 || subs[0].ItemID != ids["S01E05"] || subs[0].LanguageCode != "en" {
		t.Errorf("subtitles = %+v", subs)
	}
	if e.tree.count() != 1 {
		t.Errorf("tree rebuilt %d times, want 1", e.tree.count())
	}
	d := e.detail(res.JobID)
	if d.Job.Counts.Pending != 2 || d.Job.Counts.Conflicts != 2 {
		t.Errorf("counts = %+v, want the two conflicting files left pending", d.Job.Counts)
	}
}

// Moving an episode to another torrent rebuilds the tree, so the earlier
// torrent's files leave it.
func TestAssignShowAndWait_Reassignment(t *testing.T) {
	hashA, hashB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	e := newJobEnv(t,
		info(hashA, "Reassigned.Show.S01.1080p.WEB-DL-GRP", file("Reassigned.Show.S01E01.1080p.WEB-DL-GRP.mkv", gb)),
		&torrent.TorrentInfo{InfoHash: hashB, Name: "Reassigned.Show.S01E01.720p.HDTV.x264-LOL.mkv",
			Files: []identify.TorrentFile{file("Reassigned.Show.S01E01.720p.HDTV.x264-LOL.mkv", gb/2)}},
	)
	e.start()
	showID, ids := e.show("Reassigned Show", [][3]any{{1, 1, "Pilot"}})

	for i, hash := range []string{hashA, hashB} {
		res, err := e.svc.AssignShowAndWait(e.ctx, showID, AssignRequest{MagnetURI: magnet(hash)})
		if err != nil || res.Summary.Matched != 1 {
			t.Fatalf("assignment %d = %+v, %v", i, res, err)
		}
		if e.tree.count() != i+1 {
			t.Errorf("after assignment %d the tree was rebuilt %d times", i, e.tree.count())
		}
	}
	if a := e.active(library.ItemTypeEpisode, ids["S01E01"]); a == nil || a.InfoHash != hashB {
		t.Errorf("S01E01 = %+v", a)
	}
}

// With specials off, a special is reported unmatched instead of assigned.
func TestAssignShowAndWait_SpecialsOff(t *testing.T) {
	hash := strings.Repeat("d", 40)
	e := newJobEnv(t, &torrent.TorrentInfo{InfoHash: hash, Name: "No.Specials.Show.S00E01.mkv",
		Files: []identify.TorrentFile{file("No.Specials.Show.S00E01.Christmas.Special.mkv", gb)}})
	e.start()
	showID, ids := e.show("No Specials Show", [][3]any{{0, 1, "Christmas Special"}, {1, 1, "Pilot"}})
	if _, err := e.db.Exec(`UPDATE shows SET include_specials = FALSE WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.AssignShowAndWait(e.ctx, showID, AssignRequest{MagnetURI: magnet(hash)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Matched != 0 || len(res.Unmatched) != 1 || res.Unmatched[0].Reason != "Specials are off for this show" {
		t.Errorf("result = %+v", res)
	}
	if a := e.active(library.ItemTypeEpisode, ids["S00E01"]); a != nil {
		t.Errorf("the special was assigned: %+v", a)
	}
}

// The legacy movie assign takes the classifier's best candidate: the film
// by title and year, not just the largest video.
func TestAssignMovieAndWait_PicksTheFilm(t *testing.T) {
	hash := strings.Repeat("e", 40)
	e := newJobEnv(t, info(hash, "Nolan Pack",
		file("Inception.2010.1080p.BluRay.x264.mkv", 14*gb),
		file("Interstellar.2014.1080p.BluRay.x264.mkv", 12*gb),
		file("Sample/interstellar.sample.mkv", 50<<20),
	))
	e.start()
	movieID := e.movie("Interstellar", 2014)

	res, err := e.svc.AssignMovieAndWait(e.ctx, movieID, AssignRequest{MagnetURI: magnet(hash)})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Assignment
	if a == nil || a.FilePath != "Nolan Pack/Interstellar.2014.1080p.BluRay.x264.mkv" || a.Resolution != "1080p" || a.Source != "BluRay" {
		t.Errorf("assignment = %+v", a)
	}
	if e.job(res.JobID).Status != library.JobApplied {
		t.Errorf("job = %s", e.job(res.JobID).Status)
	}
}

// A job that fails answers the legacy request with its error.
func TestAssignAndWait_Failure(t *testing.T) {
	e := newJobEnv(t)
	e.start()
	showID, _ := e.show("Lost Show", [][3]any{{1, 1, "Pilot"}})
	_, err := e.svc.AssignShowAndWait(e.ctx, showID, AssignRequest{MagnetURI: magnet(strings.Repeat("f", 40))})
	var failed *JobFailedError
	if !errors.As(err, &failed) || !failed.Timeout || failed.Message != msgMetadataTimeout {
		t.Errorf("err = %v", err)
	}
	if _, err := e.svc.AssignShowAndWait(e.ctx, showID+1000, AssignRequest{MagnetURI: magnet(strings.Repeat("f", 40))}); !errors.Is(err, library.ErrShowNotFound) {
		t.Errorf("unknown show: %v", err)
	}
	if _, err := e.svc.AssignShowAndWait(e.ctx, showID, AssignRequest{MagnetURI: "magnet:?dn=x"}); !errors.Is(err, library.ErrInvalidMagnet) {
		t.Errorf("no hash: %v", err)
	}
}
