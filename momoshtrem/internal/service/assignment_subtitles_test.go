package service

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// Torrent subtitles belong to the file they came with: when an item moves
// to another file (an override, or a re-apply, even within one torrent),
// the old file's go and the new file's come.

// realSubtitles makes the env record subtitles in its database.
func (e *jobEnv) realSubtitles() *subtitle.Repository {
	repo := subtitle.NewRepository(e.db.DB)
	e.svc.subtitles = subtitle.NewService(nil, repo, e.t.TempDir())
	return repo
}

// subtitlesOf lists an item's subtitles as "source:file name", sorted.
func subtitlesOf(t *testing.T, repo *subtitle.Repository, itemType subtitle.ItemType, id int64) string {
	t.Helper()
	subs, err := repo.GetByItem(context.Background(), itemType, id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range subs {
		out = append(out, string(s.Source)+":"+s.FilePath[strings.LastIndex(s.FilePath, "/")+1:])
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestSubtitles_FollowAnOverride(t *testing.T) {
	hash, root := strings.Repeat("a9", 20), "Cedar.Hill.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Cedar.Hill.S01E01.1080p.WEB-GRP.mkv", gb), file("Cedar.Hill.S01E01.1080p.WEB-GRP.en.srt", 1<<10),
		file("Cedar.Hill.S01E02.1080p.WEB-GRP.mkv", gb), file("Cedar.Hill.S01E02.1080p.WEB-GRP.en.srt", 1<<10),
		file("Cedar.Hill.S01E03.1080p.WEB-GRP.mkv", gb), file("Cedar.Hill.S01E03.1080p.WEB-GRP.en.srt", 1<<10)))
	showID, eps := e.show("Cedar Hill", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	repo := e.realSubtitles()
	if id := e.assignShow(showID, hash, nil); e.job(id).Status != library.JobApplied {
		for _, p := range e.detail(id).Proposals {
			t.Logf("%s %s %s %v %s", p.FilePath, p.Group, p.Decision, p.Confidence, p.Reason)
		}
		t.Fatalf("job = %s", e.job(id).Status)
	}
	// A downloaded subtitle is the user's: it stays whatever the file.
	if _, err := e.db.Exec(`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path, source)
		VALUES ('episode', $1, 'de', 'German', 'srt', '/subs/de.srt', 'opensubtitles')`, eps["S01E01"]); err != nil {
		t.Fatal(err)
	}
	if got := subtitlesOf(t, repo, subtitle.ItemTypeEpisode, eps["S01E01"]); got !=
		"opensubtitles:de.srt,torrent:Cedar.Hill.S01E01.1080p.WEB-GRP.en.srt" {
		t.Fatalf("E01 before = %v", got)
	}

	// E01 plays E03's file now: E03's subtitle comes, E01's goes.
	e3 := root + "/Cedar.Hill.S01E03.1080p.WEB-GRP.mkv"
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E01"], hash, e3); err != nil {
		t.Fatal(err)
	}
	if got := subtitlesOf(t, repo, subtitle.ItemTypeEpisode, eps["S01E01"]); got !=
		"opensubtitles:de.srt,torrent:Cedar.Hill.S01E03.1080p.WEB-GRP.en.srt" {
		t.Errorf("E01 after the override = %v", got)
	}
	if got := subtitlesOf(t, repo, subtitle.ItemTypeEpisode, eps["S01E03"]); got != "torrent:Cedar.Hill.S01E03.1080p.WEB-GRP.en.srt" {
		t.Errorf("E03 = %v", got)
	}

	// The same file again changes nothing.
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E01"], hash, e3); err != nil {
		t.Fatal(err)
	}
	if got := subtitlesOf(t, repo, subtitle.ItemTypeEpisode, eps["S01E01"]); got !=
		"opensubtitles:de.srt,torrent:Cedar.Hill.S01E03.1080p.WEB-GRP.en.srt" {
		t.Errorf("E01 after the same override = %v", got)
	}
}

func TestSubtitles_FollowAReapply(t *testing.T) {
	hash, root := strings.Repeat("b0", 20), "Maple.Road.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Maple.Road.S01E01.1080p.WEB-GRP.mkv", gb), file("Maple.Road.S01E01.1080p.WEB-GRP.en.srt", 1<<10),
		file("Maple.Road.S01E02.1080p.WEB-GRP.mkv", gb), file("Maple.Road.S01E02.1080p.WEB-GRP.en.srt", 1<<10)))
	showID, eps := e.show("Maple Road", [][3]any{{1, 1, "One"}, {1, 2, "Two"}})
	repo := e.realSubtitles()
	if id := e.assignShow(showID, hash, nil); e.job(id).Status != library.JobApplied {
		t.Fatalf("job = %s", e.job(id).Status)
	}
	if got := subtitlesOf(t, repo, subtitle.ItemTypeEpisode, eps["S01E01"]); got != "torrent:Maple.Road.S01E01.1080p.WEB-GRP.en.srt" {
		t.Fatalf("E01 before = %v", got)
	}

	// The same torrent again, with E01 moved to E02's file.
	e.setAI(`{"auto_apply_high_confidence": false}`)
	id := e.assignShow(showID, hash, nil)
	d := e.detail(id)
	e.patch(id,
		change(proposalByPath(d, "E01.1080p.WEB-GRP.mkv").ID, library.DecisionIgnored),
		change(proposalByPath(d, "E02.1080p.WEB-GRP.mkv").ID, library.DecisionAccepted, eps["S01E01"]))
	if _, err := e.svc.Apply(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a == nil || !strings.HasSuffix(a.FilePath, "E02.1080p.WEB-GRP.mkv") {
		t.Fatalf("E01 = %+v", a)
	}
	if got := subtitlesOf(t, repo, subtitle.ItemTypeEpisode, eps["S01E01"]); got != "torrent:Maple.Road.S01E02.1080p.WEB-GRP.en.srt" {
		t.Errorf("E01 after the re-apply = %v", got)
	}
}
