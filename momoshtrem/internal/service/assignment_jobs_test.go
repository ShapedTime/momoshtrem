package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

const gb = 1 << 30

// A season pack the rules place with high confidence: the job applies on
// its own, writing one row per episode (the multi-episode file twice) with
// its confidence and source, the subtitle on its video's episode, and the
// tree rebuilt once.
func TestJob_ShowAutoApplies(t *testing.T) {
	hash := strings.Repeat("a", 40)
	root := "Clean.Show.S01.1080p.WEB-DL-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Clean.Show.S01E01E02.1080p.WEB-DL-GRP.mkv", 2*gb),
		file("Clean.Show.S01E03.1080p.WEB-DL-GRP.mkv", gb),
		file("Clean.Show.S01E03.1080p.WEB-DL-GRP.en.srt", 40<<10),
		file("Sample/clean.show.s01e01.sample.mkv", 30<<20),
		file("Clean.Show.S01.nfo", 2<<10),
	))
	showID, eps := e.show("Clean Show", [][3]any{{1, 1, "Pilot"}, {1, 2, "Second"}, {1, 3, "Third"}})

	id, created, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(hash) + "&dn=" + root, Indexer: "Nyaa"})
	if err != nil || !created {
		t.Fatal(created, err)
	}
	queued := e.detail(id)
	if queued.Job.Status != library.JobQueued || len(queued.Proposals) != 0 || queued.Job.Torrent.Name != root ||
		queued.Job.Torrent.FileCount != nil || queued.Job.Progress.Seasons != nil {
		t.Errorf("queued job = %+v", queued.Job)
	}
	e.run(id)

	d := e.detail(id)
	j := d.Job
	if j.Status != library.JobApplied || j.AppliedAt == nil {
		t.Fatalf("status = %s (%+v)", j.Status, d.Proposals)
	}
	if *j.Torrent.FileCount != 5 || *j.Torrent.VideoCount != 2 || *j.Torrent.SizeBytes != 3*gb+30<<20+40<<10+2<<10 ||
		*j.Torrent.ReleaseGroup != "GRP" || *j.Torrent.Indexer != "Nyaa" || *j.Progress.Seasons != 1 || *j.Progress.Episodes != 3 ||
		*j.Progress.Files != 5 || *j.Progress.Videos != 2 || j.Progress.AIFiles != nil || j.AIAvailable {
		t.Errorf("job = %+v\ntorrent %+v progress %+v", j, j.Torrent, j.Progress)
	}
	want := JobCounts{Files: 4, ToApply: 2, Matched: 2, Ignored: 2}
	if j.Counts != want {
		t.Errorf("counts = %+v, want %+v", j.Counts, want)
	}
	multi := proposalByPath(d, "S01E01E02.1080p.WEB-DL-GRP.mkv")
	if multi == nil || len(multi.Targets) != 2 || multi.Targets[1].Name != "Second" || multi.Group != "matched" || multi.Replaces != nil {
		t.Errorf("multi-episode proposal = %+v", multi)
	}
	if p := proposalByPath(d, "S01E03.1080p.WEB-DL-GRP.mkv"); p == nil || p.SubtitleCount != 1 {
		t.Errorf("E03 = %+v", p)
	}
	if p := proposalByPath(d, ".srt"); p != nil {
		t.Errorf("a linked subtitle is listed: %+v", p)
	}

	for _, label := range []string{"S01E01", "S01E02"} {
		a := e.active(library.ItemTypeEpisode, eps[label])
		if a == nil || !strings.HasSuffix(a.FilePath, "S01E01E02.1080p.WEB-DL-GRP.mkv") || a.MatchSource != "rules" ||
			a.Confidence != "high" || a.Resolution != "1080p" || a.InfoHash != hash {
			t.Errorf("%s = %+v", label, a)
		}
	}
	if subs := e.subs.all(); len(subs) != 1 || subs[0].ItemID != eps["S01E03"] || subs[0].LanguageCode != "en" ||
		subs[0].ReleaseName != root || subs[0].InfoHash != hash {
		t.Errorf("subtitles = %+v", subs)
	}
	if e.tree.count() != 1 {
		t.Errorf("tree rebuilt %d times, want 1", e.tree.count())
	}

	// Assigning the same torrent again makes a new job: the first is closed.
	again, created, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(hash)})
	if err != nil || !created || again == id {
		t.Errorf("after apply = %d %v %v", again, created, err)
	}
}

// Two files on one episode stop for review; Apply refuses until one is
// left out, then writes once. A second Apply (or one racing it) is
// job_closed.
func TestJob_ReviewApplyAndRace(t *testing.T) {
	hash := strings.Repeat("b", 40)
	root := "Race.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Race.Show.S01E01.1080p.WEB-GRP.mkv", gb),
		file("Race.Show.S01E02.Two.1080p.WEB-GRP.mkv", gb),
		file("Race.Show.S01E02.Extended.Cut.1080p.WEB-GRP.mkv", gb),
	))
	showID, eps := e.show("Race Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}})
	id := e.assignShow(showID, hash, nil)

	d := e.detail(id)
	if d.Job.Status != library.JobNeedsReview || d.Job.Counts.Conflicts != 2 || d.Job.Counts.NeedsReview != 2 {
		t.Fatalf("job = %s %+v", d.Job.Status, d.Job.Counts)
	}
	if d.Proposals[0].Group != "needs_review" || d.Proposals[1].Group != "needs_review" || d.Proposals[2].Group != "matched" {
		t.Errorf("order = %s, %s, %s", d.Proposals[0].Group, d.Proposals[1].Group, d.Proposals[2].Group)
	}
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodeConflictingProposals {
		t.Fatalf("apply with a conflict: %v", err)
	}
	if e.active(library.ItemTypeEpisode, eps["S01E01"]) != nil {
		t.Error("a refused apply wrote rows")
	}

	e.decide(id, map[string]library.Decision{"Extended.Cut.1080p.WEB-GRP.mkv": library.DecisionIgnored})
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodePendingProposals {
		t.Fatalf("apply with a pending file: %v", err)
	}
	e.decide(id, map[string]library.Decision{"Race.Show.S01E02.Two.1080p.WEB-GRP.mkv": library.DecisionAccepted})
	if got := e.job(id).Status; got != library.JobReady {
		t.Fatalf("status = %s, want ready", got)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = e.svc.Apply(e.ctx, id)
		}(i)
	}
	wg.Wait()
	ok, closed := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case conflictCode(err) == CodeJobClosed:
			closed++
		default:
			t.Errorf("apply: %v", err)
		}
	}
	if ok != 1 || closed != 1 {
		t.Errorf("racing applies: %d applied, %d job_closed", ok, closed)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E02"]); a == nil || !strings.Contains(a.FilePath, "E02.Two.") {
		t.Errorf("E02 = %+v", a)
	}
	if e.tree.count() != 1 {
		t.Errorf("tree rebuilt %d times, want 1", e.tree.count())
	}
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodeJobClosed {
		t.Errorf("apply again: %v", err)
	}
}

// Re-applying a torrent upserts its rows (a file an episode had before is
// reactivated, not a unique violation) and is authoritative for it: the
// rows the job no longer maps go, the episode's other torrent's row too.
func TestJob_ReapplyIsAuthoritative(t *testing.T) {
	hash, other := strings.Repeat("c", 40), strings.Repeat("d", 40)
	root := "Again.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t,
		info(hash, root, file("Again.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Again.Show.S01E02.1080p.WEB-GRP.mkv", gb)),
		info(other, "Again.Show.S01E01.720p.HDTV-LOL", file("Again.Show.S01E01.720p.HDTV-LOL.mkv", gb/2)),
	)
	showID, eps := e.show("Again Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}})

	first := e.assignShow(showID, hash, nil)
	if e.job(first).Status != library.JobApplied {
		t.Fatalf("first = %s", e.job(first).Status)
	}
	// The second torrent replaces E01's file, so it waits for the user.
	next := e.assignShow(showID, other, nil)
	if d := e.detail(next); d.Job.Status != library.JobReady || d.Proposals[0].Replaces == nil {
		t.Fatalf("second torrent = %s, replaces %+v", d.Job.Status, d.Proposals[0].Replaces)
	}
	if _, err := e.svc.Apply(e.ctx, next); err != nil {
		t.Fatal(err)
	}
	if e.active(library.ItemTypeEpisode, eps["S01E01"]).InfoHash != other {
		t.Fatal("the second torrent didn't take E01")
	}

	e.setAI(`{"auto_apply_high_confidence": false}`)
	second := e.assignShow(showID, hash, nil)
	d := e.detail(second)
	if d.Job.Status != library.JobReady {
		t.Fatalf("second = %s", d.Job.Status)
	}
	if p := proposalByPath(d, "S01E01.1080p.WEB-GRP.mkv"); p.Replaces == nil || p.Replaces.ReleaseName != "Again.Show.S01E01.720p.HDTV-LOL" {
		t.Errorf("E01 replaces = %+v", p.Replaces)
	}
	if p := proposalByPath(d, "S01E02.1080p.WEB-GRP.mkv"); p.Replaces != nil {
		t.Errorf("E02 has its own file, replaces = %+v", p.Replaces)
	}
	e.decide(second, map[string]library.Decision{"S01E02.1080p.WEB-GRP.mkv": library.DecisionIgnored})
	if _, err := e.svc.Apply(e.ctx, second); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a == nil || a.InfoHash != hash {
		t.Errorf("E01 = %+v, want the first torrent again", a)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E02"]); a != nil {
		t.Errorf("E02 = %+v, want none: the job no longer maps it", a)
	}
	var rows int
	if err := e.db.QueryRow(`SELECT count(*) FROM torrent_assignments WHERE info_hash = $1`, hash).Scan(&rows); err != nil || rows != 2 {
		t.Errorf("rows of the torrent = %d, %v (reactivated in place)", rows, err)
	}
	if d := e.detail(second); d.Proposals[0].Replaces != nil {
		t.Errorf("after apply, replaces = %+v", d.Proposals[0].Replaces)
	}
}

func TestJob_AutoApplySettingOff(t *testing.T) {
	hash := strings.Repeat("e", 40)
	e := newJobEnv(t, info(hash, "Off.Show.S01E01.1080p.WEB-GRP", file("Off.Show.S01E01.1080p.WEB-GRP.mkv", gb)))
	showID, _ := e.show("Off Show", [][3]any{{1, 1, "One"}})
	e.setAI(`{"classifier_enabled": true, "auto_apply_high_confidence": false}`)
	id := e.assignShow(showID, hash, nil)
	if got := e.job(id).Status; got != library.JobReady {
		t.Errorf("status = %s, want ready", got)
	}
}

func TestJob_Discard(t *testing.T) {
	hash, shared := strings.Repeat("f", 40), strings.Repeat("1", 40)
	e := newJobEnv(t,
		info(hash, "Gone.Show.S01", file("Gone.Show.S01E01.Pilot.mkv", gb), file("Gone.Show.S01E01.Extended.Cut.mkv", gb)),
		info(shared, "Shared.Show.S01", file("Gone.Show.S01E01.Pilot.mkv", gb), file("Gone.Show.S01E01.Extended.Cut.mkv", gb)),
	)
	showID, eps := e.show("Gone Show", [][3]any{{1, 1, "One"}})

	id := e.assignShow(showID, hash, nil)
	if e.job(id).Status != library.JobNeedsReview {
		t.Fatalf("status = %s", e.job(id).Status)
	}
	d, err := e.svc.Discard(e.ctx, id)
	if err != nil || d.Job.Status != library.JobCancelled || len(d.Proposals) != 2 {
		t.Fatalf("discard = %+v, %v", d, err)
	}
	if got := e.torrents.removals(); !reflect.DeepEqual(got, []string{hash}) {
		t.Errorf("removed = %v, want the unused torrent", got)
	}
	if d, err := e.svc.Discard(e.ctx, id); err != nil || d.Job.Status != library.JobCancelled {
		t.Errorf("discard again = %v", err)
	}
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodeJobClosed {
		t.Errorf("apply a cancelled job: %v", err)
	}

	// A torrent another title's file uses stays.
	if _, err := e.db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size)
		VALUES ('movie', 1, $1, 'm', 'x.mkv', 1)`, shared); err != nil {
		t.Fatal(err)
	}
	id = e.assignShow(showID, shared, nil)
	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if got := e.torrents.removals(); len(got) != 1 {
		t.Errorf("removed = %v, want the shared torrent kept", got)
	}

	// Applied and failed jobs are closed.
	e.decide(e.assignShow(showID, hash, nil), map[string]library.Decision{"Extended.Cut.mkv": library.DecisionIgnored, "Pilot.mkv": library.DecisionAccepted})
	applied, err := e.svc.List(e.ctx, []library.JobStatus{library.JobReady})
	if err != nil || len(applied) != 1 {
		t.Fatalf("ready jobs = %+v, %v", applied, err)
	}
	if _, err := e.svc.Apply(e.ctx, applied[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Discard(e.ctx, applied[0].ID); conflictCode(err) != CodeJobClosed {
		t.Errorf("discard an applied job: %v", err)
	}
	if e.active(library.ItemTypeEpisode, eps["S01E01"]) == nil {
		t.Error("nothing applied")
	}
	if _, err := e.svc.Discard(e.ctx, 999999); !errors.Is(err, library.ErrJobNotFound) {
		t.Errorf("unknown job: %v", err)
	}
}

// A job discarded while its file list is on the way stops there and
// drops the torrent.
func TestJob_DiscardWhileFetching(t *testing.T) {
	hash := strings.Repeat("2", 40)
	e := newJobEnv(t, info(hash, "Late.Show.S01", file("Late.Show.S01E01.mkv", gb)))
	showID, _ := e.show("Late Show", [][3]any{{1, 1, "One"}})
	id, _, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(hash)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'fetching_metadata' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	job := e.job(id)
	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	e.svc.process(e.ctx, job)
	if got := e.job(id); got.Status != library.JobCancelled || got.FileCount != nil {
		t.Errorf("job = %s, file count %v", got.Status, got.FileCount)
	}
	if got := e.torrents.removals(); len(got) != 2 {
		t.Errorf("removed = %v, want twice (on discard and after the add)", got)
	}
}

func TestJob_Fails(t *testing.T) {
	missing, noVideo := strings.Repeat("3", 40), strings.Repeat("4", 40)
	e := newJobEnv(t, info(noVideo, "Docs", file("readme.txt", 10), file("cover.jpg", 10)))
	showID, _ := e.show("Fail Show", [][3]any{{1, 1, "One"}})

	id := e.assignShow(showID, missing, nil)
	d := e.detail(id)
	if d.Job.Status != library.JobFailed || d.Job.Error == nil || *d.Job.Error != failMetadataTimeout.message || len(d.Proposals) != 0 {
		t.Errorf("timeout job = %+v", d.Job)
	}
	id = e.assignShow(showID, noVideo, nil)
	if j := e.job(id); j.Status != library.JobFailed || j.Error != failNoVideos.message || j.ErrorCode != FailureNoVideos {
		t.Errorf("no-video job = %s %q", j.Status, j.Error)
	}
	if _, err := e.svc.Apply(e.ctx, id); conflictCode(err) != CodeJobClosed {
		t.Errorf("apply a failed job: %v", err)
	}
	if got := e.torrents.removals(); len(got) != 2 {
		t.Errorf("removed = %v, want each failed job's torrent once", got)
	}

	// No torrent client: the job fails as such.
	noClient := e.service(JobDeps{})
	noClient.torrents = nil
	id, _, err := noClient.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(strings.Repeat("5", 40))})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = noClient
	e.run(id)
	if j := e.job(id); j.Status != library.JobFailed || j.ErrorCode != FailureNoTorrents {
		t.Errorf("no client = %s %q", j.Status, j.ErrorCode)
	}
}

// fail only touches a job that's still working: a classified one stays as
// it is, and its torrent isn't released.
func TestJob_FailLeavesClassifiedJobs(t *testing.T) {
	hash := strings.Repeat("6", 40)
	e := newJobEnv(t, info(hash, "Kept.Show.S01", file("Kept.Show.S01E01.Pilot.mkv", gb), file("Kept.Show.S01E01.Extended.Cut.mkv", gb)))
	showID, _ := e.show("Kept Show", [][3]any{{1, 1, "Pilot"}})
	id := e.assignShow(showID, hash, nil)
	job := e.job(id)
	if job.Status != library.JobNeedsReview {
		t.Fatalf("status = %s", job.Status)
	}
	if err := e.svc.fail(e.ctx, job, failInternal); err != nil {
		t.Fatal(err)
	}
	if got := e.job(id); got.Status != library.JobNeedsReview || got.ErrorCode != "" {
		t.Errorf("job = %s %q", got.Status, got.ErrorCode)
	}
	if got := e.torrents.removals(); len(got) != 0 {
		t.Errorf("removed = %v, want none", got)
	}
}

// A torrent of extras alone (openings, endings) waits for the user rather
// than ending applied with nothing written.
func TestJob_ExtrasOnlyWaits(t *testing.T) {
	hash := strings.Repeat("7", 40)
	e := newJobEnv(t, info(hash, "[Group] Extras Show - Creditless",
		file("[Group] Extras Show - NCOP1 (1080p).mkv", 100<<20), file("[Group] Extras Show - NCED1 (1080p).mkv", 100<<20)))
	showID, _ := e.show("Extras Show", [][3]any{{1, 1, "One"}})
	id := e.assignShow(showID, hash, nil)
	d := e.detail(id)
	if d.Job.Status != library.JobReady || d.Job.Counts.ToApply != 0 || d.Job.Counts.Ignored != 2 {
		t.Errorf("job = %s %+v", d.Job.Status, d.Job.Counts)
	}
	if e.tree.count() != 0 {
		t.Error("the tree was rebuilt for nothing")
	}
}

// Jobs a stopped worker left: before resolving they start again from
// queued; an interrupted AI run keeps the rules' answers, with ai_error.
func TestJob_RestartRecovery(t *testing.T) {
	hash := strings.Repeat("5", 40)
	e := newJobEnv(t, info(hash, "Back.Show.S01", file("Back.Show.S01E01.Pilot.mkv", gb), file("Back.Show.S01E01.Extended.Cut.mkv", gb)))
	showID, _ := e.show("Back Show", [][3]any{{1, 1, "One"}})

	resolving := e.assignShow(showID, hash, nil) // needs_review: two files on E01
	if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = 'resolving' WHERE id = $1`, resolving); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i, status := range []string{"queued", "fetching_metadata", "classifying"} {
		id, _, err := e.svc.Assign(e.ctx, library.JobTV, showID, AssignRequest{MagnetURI: magnet(strings.Repeat(string(rune('6'+i)), 40))})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.Exec(`UPDATE assignment_jobs SET status = $2 WHERE id = $1`, id, status); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	e.svc.recoverInterrupted(e.ctx)
	for _, id := range ids {
		if got := e.job(id).Status; got != library.JobQueued {
			t.Errorf("job %d = %s, want queued", id, got)
		}
	}
	d := e.detail(resolving)
	if d.Job.Status != library.JobNeedsReview || d.Job.AIError == nil || *d.Job.AIError != msgAIInterrupted || len(d.Proposals) != 2 {
		t.Errorf("interrupted AI run = %s %v, %d proposals", d.Job.Status, d.Job.AIError, len(d.Proposals))
	}

	// The workers pick the requeued jobs up (their torrents time out here).
	e.start()
	waitFor(t, func() bool {
		for _, id := range ids {
			if e.job(id).Status.Working() {
				return false
			}
		}
		return true
	})
}

func TestJob_Movies(t *testing.T) {
	pack, single := strings.Repeat("7", 40), strings.Repeat("8", 40)
	e := newJobEnv(t,
		info(pack, "Nolan Pack",
			file("Inception.2010.1080p.BluRay.x264.mkv", 14*gb),
			file("Interstellar.2014.1080p.BluRay.x264.mkv", 12*gb),
			file("Sample/interstellar.sample.mkv", 50<<20)),
		info(single, "Interstellar.2014.2160p.WEB-DL-GRP",
			file("Interstellar.2014.2160p.WEB-DL-GRP.mkv", 20*gb),
			file("Interstellar.2014.2160p.WEB-DL-GRP.en.srt", 80<<10)),
	)
	movieID := e.movie("Interstellar", 2014)
	e.svc = e.service(JobDeps{Resolver: &fakeResolver{}}) // a real resolver: still no AI for movies

	// A collection pack waits for a pick.
	id, _, err := e.svc.Assign(e.ctx, library.JobMovie, movieID, AssignRequest{MagnetURI: magnet(pack)})
	if err != nil {
		t.Fatal(err)
	}
	e.run(id)
	d := e.detail(id)
	if d.Job.Status != library.JobNeedsReview || d.Job.Counts.Pending != 2 || d.Job.Progress.Seasons != nil || *d.Job.Torrent.VideoCount != 2 || d.Job.AIAvailable {
		t.Fatalf("pack = %s %+v", d.Job.Status, d.Job.Counts)
	}
	for _, p := range d.Proposals[:2] {
		if p.Kind != "movie" || p.Group != "needs_review" || len(p.Targets) != 0 {
			t.Errorf("candidate = %+v", p)
		}
	}
	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}

	// file_path picks the film up front: the job applies once classified.
	id, _, err = e.svc.Assign(e.ctx, library.JobMovie, movieID, AssignRequest{MagnetURI: magnet(pack), FilePath: "Interstellar.2014.1080p.BluRay.x264.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	e.run(id)
	if got := e.job(id).Status; got != library.JobApplied {
		t.Fatalf("picked = %s", got)
	}
	if a := e.active(library.ItemTypeMovie, movieID); a == nil || a.FilePath != "Nolan Pack/Interstellar.2014.1080p.BluRay.x264.mkv" || a.MatchSource != "rules" {
		t.Errorf("movie = %+v", a)
	}

	// One plausible film applies on its own, with its subtitle.
	id, _, _ = e.svc.Assign(e.ctx, library.JobMovie, movieID, AssignRequest{MagnetURI: magnet(single)})
	e.run(id)
	d = e.detail(id)
	if d.Job.Status != library.JobApplied || d.Proposals[0].SubtitleCount != 1 {
		t.Fatalf("single = %s %+v", d.Job.Status, d.Proposals)
	}
	if a := e.active(library.ItemTypeMovie, movieID); a == nil || a.InfoHash != single || a.Confidence != "high" {
		t.Errorf("movie = %+v", a)
	}
	if subs := e.subs.all(); len(subs) != 1 || subs[0].ItemType != "movie" || subs[0].ItemID != movieID {
		t.Errorf("subtitles = %+v", subs)
	}
}

// The first AI run looks at the unsure files; its answers set their
// groups, and a job that's then all high applies on its own. A failed run
// leaves the rules' proposals and sets ai_error.
func TestJob_FirstAIRun(t *testing.T) {
	hash := strings.Repeat("9", 40)
	root := "AI.Show.S01.1080p.WEB-GRP"
	torrentInfo := info(hash, root, file("AI.Show.S01E01.1080p.WEB-GRP.mkv", gb), file("Something Strange.mkv", gb))
	e := newJobEnv(t, torrentInfo)
	showID, eps := e.show("AI Show", [][3]any{{0, 1, "The Mystery Special"}, {1, 1, "One"}})

	season, episode := 0, 1
	resolver := &fakeResolver{out: identify.ResolveOutput{Files: []identify.AIFile{{
		Path: root + "/Something Strange.mkv", Kind: identify.KindSpecial, Season: &season, Episodes: []int{episode},
		Confidence: identify.ConfidenceHigh, Reason: "The special by its title",
	}}}}
	e.svc = e.service(JobDeps{Resolver: resolver})

	id := e.assignShow(showID, hash, nil)
	d := e.detail(id)
	if d.Job.Status != library.JobApplied || !d.Job.AIAvailable || d.Job.Progress.AIFiles == nil || *d.Job.Progress.AIFiles != 1 {
		t.Fatalf("job = %s ai %v files %v (%+v)", d.Job.Status, d.Job.AIAvailable, d.Job.Progress.AIFiles, d.Proposals)
	}
	if len(resolver.calls) != 1 || !reflect.DeepEqual(resolver.calls[0].Scope, []string{root + "/Something Strange.mkv"}) || len(resolver.calls[0].Rules) != 2 {
		t.Errorf("resolver input = %+v", resolver.calls)
	}
	bonus := proposalByPath(d, "Something Strange.mkv")
	if bonus.Group != "specials" || bonus.Source != "ai" || bonus.Decision != "accepted" || bonus.Reason != "The special by its title" ||
		len(bonus.SuggestedTargets) != 1 || bonus.Updated {
		t.Errorf("bonus = %+v", bonus)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S00E01"]); a == nil || a.MatchSource != "ai" {
		t.Errorf("special = %+v", a)
	}

	// The run fails: the rules' answer stands, with ai_error.
	failing := &fakeResolver{err: errors.New("overloaded")}
	e.svc = e.service(JobDeps{Resolver: failing})
	if _, err := e.db.Exec(`DELETE FROM torrent_assignments`); err != nil {
		t.Fatal(err)
	}
	id = e.assignShow(showID, hash, nil)
	d = e.detail(id)
	if d.Job.Status != library.JobNeedsReview && d.Job.Status != library.JobReady {
		t.Fatalf("after a failed run = %s", d.Job.Status)
	}
	if d.Job.AIError == nil || *d.Job.AIError != msgAIFailed || proposalByPath(d, "Something Strange.mkv").Group != "unmatched" {
		t.Errorf("failed run: ai_error %v, bonus %+v", d.Job.AIError, proposalByPath(d, "Something Strange.mkv"))
	}

	// rules_only: no AI at all.
	if _, err := e.db.Exec(`UPDATE shows SET classifier_mode = 'rules_only' WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Discard(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	calls := len(failing.calls)
	id = e.assignShow(showID, hash, nil)
	if d := e.detail(id); len(failing.calls) != calls || d.Job.AIAvailable || d.Job.Progress.AIFiles != nil {
		t.Errorf("rules_only: calls %d → %d, ai_available %v", calls, len(failing.calls), d.Job.AIAvailable)
	}

	// classifier_enabled off: no AI either.
	if _, err := e.db.Exec(`UPDATE shows SET classifier_mode = 'auto' WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	e.setAI(`{"classifier_enabled": false}`)
	if d := e.detail(id); d.Job.AIAvailable {
		t.Error("ai_available with the classifier off")
	}
}

// Stale metadata is refreshed while the file list loads; fresh metadata
// is refreshed only when a video doesn't match.
func TestJob_RefreshesMetadata(t *testing.T) {
	hash, unmatched := strings.Repeat("ab", 20), strings.Repeat("cd", 20)
	e := newJobEnv(t,
		info(hash, "Fresh.Show.S01", file("Fresh.Show.S01E01.mkv", gb)),
		info(unmatched, "Fresh.Show.S01E09", file("Fresh.Show.S01E09.mkv", gb)),
	)
	showID, _ := e.show("Fresh Show", [][3]any{{1, 1, "One"}})
	refresher := &fakeRefresher{db: e.db}
	e.svc = e.service(JobDeps{Metadata: refresher})

	e.assignShow(showID, hash, nil) // fresh, everything matches
	if refresher.calls != 0 {
		t.Errorf("refreshed %d times, want 0", refresher.calls)
	}
	if _, err := e.db.Exec(`UPDATE shows SET metadata_updated_at = NULL WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	e.svc.Discard(e.ctx, e.assignShow(showID, unmatched, nil)) // never synced
	if refresher.calls != 1 {
		t.Errorf("refreshed %d times, want 1 (stale, and not again for the unmatched file)", refresher.calls)
	}
	if _, err := e.db.Exec(`UPDATE shows SET metadata_updated_at = NOW() - interval '2 hours' WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	e.svc.Discard(e.ctx, e.assignShow(showID, unmatched, nil)) // E09 isn't known: maybe it just aired
	if refresher.calls != 2 {
		t.Errorf("refreshed %d times, want 2", refresher.calls)
	}
	e.svc.Discard(e.ctx, e.assignShow(showID, unmatched, nil)) // refreshed just now
	if refresher.calls != 2 {
		t.Errorf("refreshed %d times, want 2 (cooldown)", refresher.calls)
	}

	// A stale refresh that fails isn't tried again for the unmatched file.
	refresher.err = errors.New("TMDB is down")
	if _, err := e.db.Exec(`UPDATE shows SET metadata_updated_at = NULL WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	e.svc.Discard(e.ctx, e.assignShow(showID, unmatched, nil))
	if refresher.calls != 3 {
		t.Errorf("refreshed %d times, want 3 (one try per job)", refresher.calls)
	}
}

// The catalog gets TMDB's alternative titles as aliases and its
// season-like episode groups as alternative orderings.
func TestJobService_CatalogExtras(t *testing.T) {
	g := &tmdb.EpisodeGroup{ID: "bd", Name: "Blu-ray", Type: tmdb.GroupDVD, Groups: []tmdb.EpisodeGroupGroup{
		{Name: "Season 2", Order: 1, Episodes: []tmdb.GroupEpisode{{SeasonNumber: 1, EpisodeNumber: 3, Order: 0}}},
		{Name: "Season 1", Order: 0, Episodes: []tmdb.GroupEpisode{
			{SeasonNumber: 1, EpisodeNumber: 2, Order: 1}, {SeasonNumber: 1, EpisodeNumber: 1, Order: 0}}},
	}}
	src := &fakeCatalogSource{
		titles: []tmdb.AlternativeTitle{{Title: "Kusuriya no Hitorigoto", Type: "Romaji"}, {Title: "薬屋のひとりごと"}, {Title: "The Apothecary Diaries"}},
		summaries: []tmdb.EpisodeGroupSummary{
			{ID: "abs", Type: tmdb.GroupAbsolute}, {ID: "arcs", Type: tmdb.GroupStoryArc}, {ID: "bd", Type: tmdb.GroupDVD},
		},
		groups: map[string]*tmdb.EpisodeGroup{"bd": g, "abs": {ID: "abs"}},
	}
	s := NewJobService(JobDeps{TMDB: &fakeTitleTMDB{fakeCatalogSource: *src}})
	show := &library.Show{ID: 1, TMDBID: 5, Title: "The Apothecary Diaries", IncludeSpecials: true, Seasons: []library.Season{
		{SeasonNumber: 1, Episodes: []library.Episode{{ID: 11, EpisodeNumber: 1}, {ID: 12, EpisodeNumber: 2}, {ID: 13, EpisodeNumber: 3}}},
	}}
	extras := s.catalogExtras(t.Context(), show)
	if len(extras.EpisodeGroups) != 1 || extras.EpisodeGroups[0].ID != "bd" {
		t.Errorf("groups = %+v, want the season-like one", extras.EpisodeGroups)
	}
	cat := ShowCatalog(show, extras)
	if !reflect.DeepEqual(cat.Aliases, []string{"Kusuriya no Hitorigoto"}) {
		t.Errorf("aliases = %q", cat.Aliases)
	}
	want := []identify.AltOrdering{{Name: "Blu-ray", Groups: []identify.AltGroup{
		{Number: 1, EpisodeIDs: []int64{11, 12}}, {Number: 2, EpisodeIDs: []int64{13}},
	}}}
	if !reflect.DeepEqual(cat.AltOrderings, want) {
		t.Errorf("orderings = %+v", cat.AltOrderings)
	}
}

func TestGroupSeason(t *testing.T) {
	tests := []struct {
		name    string
		number  int
		special bool
	}{
		{"Season 2", 2, false}, {"Series 03", 3, false}, {"Staffel 1", 1, false}, {"4", 4, false},
		{"Specials", 0, true}, {"Season 0", 0, true}, {"OVA", 0, true}, {"Volume One", -1, false},
	}
	for _, tt := range tests {
		if n, sp := groupSeason(tt.name); n != tt.number || sp != tt.special {
			t.Errorf("groupSeason(%q) = %d, %v; want %d, %v", tt.name, n, sp, tt.number, tt.special)
		}
	}
	// A part without a season in its name counts by position, specials aside.
	groups := []*tmdb.EpisodeGroup{{Name: "DVD", Groups: []tmdb.EpisodeGroupGroup{
		{Name: "Specials", Order: 0, Episodes: []tmdb.GroupEpisode{{SeasonNumber: 0, EpisodeNumber: 1}}},
		{Name: "Volume One", Order: 1, Episodes: []tmdb.GroupEpisode{{SeasonNumber: 1, EpisodeNumber: 1}, {SeasonNumber: 9, EpisodeNumber: 9}, {SeasonNumber: 1, EpisodeNumber: 2}}},
		{Name: "Volume Two", Order: 2, Episodes: []tmdb.GroupEpisode{{SeasonNumber: 7, EpisodeNumber: 7}}},
	}}}
	got := altOrderings(groups, []identify.Episode{{ID: 1, Season: 0, Number: 1}, {ID: 11, Season: 1, Number: 1}, {ID: 12, Season: 1, Number: 2}})
	want := []identify.AltOrdering{{Name: "DVD", Groups: []identify.AltGroup{
		{Number: 0, EpisodeIDs: []int64{1}},
		{Number: 1, EpisodeIDs: []int64{11}}, // stops at the episode the catalog doesn't have
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("orderings = %+v", got)
	}
}

// A season split into parts is one release season, so S02E14 of a
// 13 + 12 split finds the second part's first episode.
func TestAltOrderings_MergesSplitSeasons(t *testing.T) {
	part := func(name string, order int, eps ...int) tmdb.EpisodeGroupGroup {
		g := tmdb.EpisodeGroupGroup{Name: name, Order: order}
		for i, e := range eps {
			g.Episodes = append(g.Episodes, tmdb.GroupEpisode{SeasonNumber: 1, EpisodeNumber: e, Order: i})
		}
		return g
	}
	var catalog []identify.Episode
	for e := 1; e <= 6; e++ {
		catalog = append(catalog, identify.Episode{ID: int64(100 + e), Season: 1, Number: e})
	}
	tests := []struct {
		name  string
		parts []tmdb.EpisodeGroupGroup
		want  []identify.AltGroup
	}{
		{"two parts of season 2", []tmdb.EpisodeGroupGroup{
			part("Season 1", 0, 1, 2), part("Season 2 Part 1", 1, 3, 4), part("Season 2 Part 2", 2, 5, 6),
		}, []identify.AltGroup{{Number: 1, EpisodeIDs: []int64{101, 102}}, {Number: 2, EpisodeIDs: []int64{103, 104, 105, 106}}}},
		{"a cut-short part isn't continued", []tmdb.EpisodeGroupGroup{
			part("Season 2 Part 1", 0, 3, 9), part("Season 2 Part 2", 1, 5, 6),
		}, []identify.AltGroup{{Number: 2, EpisodeIDs: []int64{103}}, {Number: 2, EpisodeIDs: []int64{105, 106}}}},
		{"only consecutive parts merge", []tmdb.EpisodeGroupGroup{
			part("Season 2", 0, 1), part("Season 3", 1, 2), part("Season 2", 2, 3),
		}, []identify.AltGroup{{Number: 2, EpisodeIDs: []int64{101}}, {Number: 3, EpisodeIDs: []int64{102}}, {Number: 2, EpisodeIDs: []int64{103}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := altOrderings([]*tmdb.EpisodeGroup{{Name: "Blu-ray", Groups: tt.parts}}, catalog)
			if len(got) != 1 || !reflect.DeepEqual(got[0].Groups, tt.want) {
				t.Errorf("groups = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The job's JSON has exactly the fields of the web contract
// (web/src/lib/api/contract/assignment.ts).
func TestJobDetail_JSONMatchesContract(t *testing.T) {
	conf, extra := "high", "opening"
	id := int64(3)
	d := JobDetail{
		Job: jobView(&library.AssignmentJob{ID: 1, MediaType: library.JobTV, ShowID: &id, Status: library.JobNeedsReview}, nil, false),
		Proposals: []ProposalView{{ID: 2, Confidence: &conf, ExtraType: &extra,
			Targets: []ProposalTarget{{EpisodeID: 4}}, SuggestedTargets: []ProposalTarget{},
			Replaces: &ProposalReplaces{ReleaseName: "r", FilePath: "f"}}},
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	keys := func(v any) []string {
		var out []string
		for k := range v.(map[string]any) {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	job := doc["job"].(map[string]any)
	proposal := doc["proposals"].([]any)[0].(map[string]any)
	tests := []struct {
		name string
		got  any
		want string
	}{
		{"detail", doc, "job proposals"},
		{"job", job, "ai_available ai_error applied_at counts created_at dry_run error hint id item progress status suggested_hints torrent updated_at"},
		{"item", job["item"], "id media_type poster_path title tmdb_id year"},
		{"torrent", job["torrent"], "file_count indexer info_hash name release_group size_bytes video_count"},
		{"progress", job["progress"], "ai_files episodes files seasons videos"},
		{"counts", job["counts"], "conflicts files ignored matched needs_review pending specials to_apply unmatched"},
		{"proposal", proposal, "confidence decision extra_type file_path file_size group hinted id kind reason replaces source subtitle_count suggested_targets superseded_by targets updated user_decided"},
		{"target", proposal["targets"].([]any)[0], "absolute_number episode episode_id name season"},
		{"replaces", proposal["replaces"], "file_path release_name"},
	}
	for _, tt := range tests {
		if got := strings.Join(keys(tt.got), " "); got != tt.want {
			t.Errorf("%s keys:\n got  %s\n want %s", tt.name, got, tt.want)
		}
	}
	if job["hint"] != nil || job["suggested_hints"] == nil {
		t.Errorf("hint %v, suggested_hints %v: want null and []", job["hint"], job["suggested_hints"])
	}
}
