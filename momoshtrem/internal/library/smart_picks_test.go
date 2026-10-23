package library

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// What smart downloads read: a show's next episodes with torrent files
// (regular, never watched, the newest active assignment a torrent's), the
// active picks with their watches and files, and releases that skip a pick
// kept meanwhile.
func TestDownloadsRepository_SmartReads(t *testing.T) {
	f := newDownloadsFixture(t)
	ctx := context.Background()
	e01, e11, e12, e21, e22 := f.episodes[[2]int{0, 1}], f.episodes[[2]int{1, 1}], f.episodes[[2]int{1, 2}],
		f.episodes[[2]int{2, 1}], f.episodes[[2]int{2, 2}]
	f.assign(ItemTypeEpisode, e01, "cc", "Specials/S00E01.mkv", 50)
	f.assign(ItemTypeEpisode, e11, "bb", "S1/E01.mkv", 100)
	f.assign(ItemTypeEpisode, e12, "bb", "S1/E02.mkv", 200)
	f.assign(ItemTypeEpisode, e21, "dd", "S2/E01.mkv", 300)
	f.assign(ItemTypeEpisode, e22, "dd", "S2/E02.mkv", 400)
	// S2E1 plays from HDRezka now: its newest assignment.
	f.exec(`UPDATE torrent_assignments SET created_at = NOW() - INTERVAL '1 day' WHERE item_id = $1`, e21)
	f.exec(`INSERT INTO torrent_assignments (item_type, item_id, kind, stream_ref, file_path, file_size, is_active)
		VALUES ('episode', $1, 'hdrezka', '{"provider":"hdrezka"}', 'hdrezka/1/t1/s02e01/1080p.mp4', 0, TRUE)`, e21)
	// S1E2 watched on Trakt (by TMDB id).
	watched := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	f.exec(`INSERT INTO trakt_watched_episodes (member_id, show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)
		VALUES (1, $1, 9, 9, 9012, 1, $2)`, f.show.TMDBID, watched)

	next := func(season, episode, k int) []int64 {
		t.Helper()
		eps, err := f.repo.NextEpisodesWithFiles(ctx, f.show.ID, season, episode, k)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int64
		for _, e := range eps {
			ids = append(ids, e.EpisodeID)
		}
		return ids
	}
	if got, want := next(1, 1, 10), []int64{e11, e22}; !reflect.DeepEqual(got, want) {
		t.Errorf("from S1E1 = %v, want %v (S1E2 watched, S2E1 HDRezka)", got, want)
	}
	if got, want := next(0, 1, 1), []int64{e11}; !reflect.DeepEqual(got, want) {
		t.Errorf("from S0E1 = %v, want %v (no specials)", got, want)
	}
	if got := next(1, 2, 0); got != nil {
		t.Errorf("k 0 = %v", got)
	}
	if eps, _ := f.repo.NextEpisodesWithFiles(ctx, f.show.ID, 2, 2, 1); len(eps) != 1 ||
		eps[0] != (NextEpisodeFile{EpisodeID: e22, SeasonNumber: 2, EpisodeNumber: 2, InfoHash: "dd", FilePath: "S2/E02.mkv", FileSize: 400}) {
		t.Errorf("S2E2 = %+v", eps)
	}
	local := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	f.exec(`INSERT INTO local_watches (episode_id, watched_at) VALUES ($1, $2)`, e22, local)
	if got := next(1, 2, 5); got != nil {
		t.Errorf("after S2E2's local watch = %v", got)
	}

	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	pick := func(episode int64, source SmartSource) int64 {
		t.Helper()
		id, created, err := f.repo.UpsertSmartDownload(ctx, episode, source, at, at.Add(14*24*time.Hour))
		if err != nil || !created {
			t.Fatal(created, err)
		}
		return id
	}
	p11, p12, p21, p22 := pick(e11, SmartUpNext), pick(e12, SmartStreaming), pick(e21, SmartUpNext), pick(e22, SmartUpNext)
	picks, err := f.repo.ActiveSmartPicks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(t time.Time) *time.Time { return &t }
	want := []SmartPick{
		{ID: p11, EpisodeID: e11, ShowID: f.show.ID, SeasonNumber: 1, EpisodeNumber: 1, Source: SmartUpNext, CreatedAt: at, HasFile: true,
			File: NextEpisodeFile{e11, 1, 1, "bb", "S1/E01.mkv", 100, ""}},
		{ID: p12, EpisodeID: e12, ShowID: f.show.ID, SeasonNumber: 1, EpisodeNumber: 2, Source: SmartStreaming, CreatedAt: at,
			WatchedAt: ptr(watched), HasFile: true, File: NextEpisodeFile{e12, 1, 2, "bb", "S1/E02.mkv", 200, ""}},
		{ID: p21, EpisodeID: e21, ShowID: f.show.ID, SeasonNumber: 2, EpisodeNumber: 1, Source: SmartUpNext, CreatedAt: at},
		{ID: p22, EpisodeID: e22, ShowID: f.show.ID, SeasonNumber: 2, EpisodeNumber: 2, Source: SmartUpNext, CreatedAt: at,
			WatchedAt: ptr(local), HasFile: true, File: NextEpisodeFile{e22, 2, 2, "dd", "S2/E02.mkv", 400, ""}},
	}
	for i := range picks { // compare times as instants
		picks[i].CreatedAt = picks[i].CreatedAt.UTC()
		if picks[i].WatchedAt != nil {
			picks[i].WatchedAt = ptr(picks[i].WatchedAt.UTC())
		}
	}
	if !reflect.DeepEqual(picks, want) {
		t.Errorf("picks = %+v\nwant    %+v", picks, want)
	}

	// Kept meanwhile, S2E1's pick isn't a smart pick to release any more.
	if _, err := f.repo.KeepSmartDownload(ctx, p21, true, nil, at); err != nil {
		t.Fatal(err)
	}
	ids, err := f.repo.ReleaseSmartPicks(ctx, []DownloadRelease{{ID: p11, Reason: ReleaseExpired}, {ID: p21, Reason: ReleaseExpired}}, at)
	if err != nil || !reflect.DeepEqual(ids, []int64{p11}) {
		t.Fatalf("released %v, %v; want only %d", ids, err, p11)
	}
	if rec, err := f.repo.GetDownload(ctx, p21); err != nil || rec.Reason != DownloadKept {
		t.Errorf("the kept one = %+v, %v", rec, err)
	}
	if picks, _ := f.repo.ActiveSmartPicks(ctx); len(picks) != 2 || picks[0].ID != p12 || picks[1].ID != p22 {
		t.Errorf("active picks after = %+v", picks)
	}

	// The re-pick rules: S1E1, released early, is a candidate again; once a
	// pick of it runs its lifetime, it's held back (until a play) but keeps
	// its place in the window; a dismissed episode is passed over.
	var e23 int64
	if err := f.db.QueryRow(`INSERT INTO episodes (season_id, episode_number, name, air_date)
		VALUES ($1, 3, 'Episode', '2025-01-01') RETURNING id`, f.seasons[2]).Scan(&e23); err != nil {
		t.Fatal(err)
	}
	f.assign(ItemTypeEpisode, e23, "dd", "S2/E03.mkv", 500)
	if got, want := next(1, 1, 1), []int64{e11}; !reflect.DeepEqual(got, want) {
		t.Errorf("after an early release = %v, want %v", got, want)
	}
	lifetime, _, err := f.repo.UpsertSmartDownload(ctx, e11, SmartUpNext, at.Add(-15*24*time.Hour), at.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.ReleaseSmartPicks(ctx, []DownloadRelease{{ID: lifetime, Reason: ReleaseExpired}}, at); err != nil {
		t.Fatal(err)
	}
	if eps, _ := f.repo.NextEpisodesWithFiles(ctx, f.show.ID, 1, 1, 2); len(eps) != 2 ||
		eps[0].EpisodeID != e11 || eps[0].Blocked != "expired" || eps[1].EpisodeID != e23 || eps[1].Blocked != "" {
		t.Errorf("after a lifetime expiry = %+v, want S1E1 held back, then S2E3", eps)
	}
	dismissed, _, err := f.repo.UpsertSmartDownload(ctx, e23, SmartUpNext, at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.ReleaseSmartPicks(ctx, []DownloadRelease{{ID: dismissed, Reason: ReleaseDeleted}}, at); err != nil {
		t.Fatal(err)
	}
	if got, want := next(1, 1, 5), []int64{e11}; !reflect.DeepEqual(got, want) {
		t.Errorf("after a dismissal = %v, want %v (S1E1, held back)", got, want)
	}
}
