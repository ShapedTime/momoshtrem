package downloads

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// The planner's rules, table by table. Times are UTC unless a test says.

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time { t := t0.Add(d); return &t }

func ptrInt(n int) *int { return &n }

// movieDL is a kept movie download created at t0 with one torrent file.
func movieDL(id int64, hash, path string, size int64) library.ResolvedDownload {
	return library.ResolvedDownload{
		DownloadRecord: library.DownloadRecord{ID: id, Reason: library.DownloadKept, Scope: library.DownloadScopeMovie,
			MovieID: id, CreatedAt: t0},
		Title: library.DownloadTitle{MediaType: "movie", LibraryID: id, Title: "Film"},
		Items: []library.CoveredItem{{ItemType: library.ItemTypeMovie, ItemID: id, File: torrentFile(hash, path, size, t0.Add(-time.Hour))}},
	}
}

func torrentFile(hash, path string, size int64, assigned time.Time) *library.CoveredFile {
	return &library.CoveredFile{Kind: library.AssignmentKindTorrent, InfoHash: hash, MagnetURI: "magnet:?xt=urn:btih:" + hash,
		FilePath: path, FileSize: size, AssignedAt: assigned}
}

// episode is an aired episode of season s with a torrent file (hash "" for
// none, "hdrezka" for a stream).
func episode(id int64, s, e int, hash, path string, size int64) library.CoveredItem {
	it := library.CoveredItem{ItemType: library.ItemTypeEpisode, ItemID: id, SeasonNumber: s, EpisodeNumber: e, AirDate: "2026-01-01"}
	switch hash {
	case "":
	case "hdrezka":
		it.File = &library.CoveredFile{Kind: library.AssignmentKindHDRezka, FilePath: path}
	default:
		it.File = torrentFile(hash, path, size, t0.Add(-time.Hour))
	}
	return it
}

func showDL(id int64, scope library.DownloadScope, items ...library.CoveredItem) library.ResolvedDownload {
	d := library.ResolvedDownload{
		DownloadRecord: library.DownloadRecord{ID: id, Reason: library.DownloadKept, Scope: scope, ShowID: 7, CreatedAt: t0},
		Title:          library.DownloadTitle{MediaType: "tv", LibraryID: 7, Title: "Show", ShowStatus: "Returning Series"},
		Items:          items,
	}
	if scope == library.DownloadScopeSeason {
		d.ShowID, d.SeasonID, d.Title.SeasonNumber = 0, 70, ptrInt(1)
	}
	return d
}

func watchedAt(it library.CoveredItem, t time.Time) library.CoveredItem {
	it.WatchedAt = &t
	return it
}

func noPieces(string, string) *PieceMap { return nil }

func TestReleaseFor(t *testing.T) {
	now := t0.Add(24 * time.Hour)
	later := t0.Add(time.Hour)
	before := t0.Add(-time.Hour)
	untilWatched := func(d library.ResolvedDownload) library.ResolvedDownload { d.UntilWatched = true; return d }
	smart := func(d library.ResolvedDownload) library.ResolvedDownload {
		d.Reason, d.SmartSource, d.UntilWatched = library.DownloadSmart, library.SmartUpNext, true
		return d
	}
	finale := func(it library.CoveredItem) library.CoveredItem { it.EpisodeType = "finale"; return it }
	ended := func(d library.ResolvedDownload) library.ResolvedDownload { d.Title.ShowStatus = "Ended"; return d }
	movie := movieDL(1, "aa", "Film.mkv", 100)
	expired := movie
	expired.ExpiresAt = at(time.Hour)

	tests := []struct {
		name      string
		d         library.ResolvedDownload
		streaming map[int64][]episodeNumber
		want      library.ReleaseReason
	}{
		{"active, no maximum", movie, nil, ""},
		{"expires_at passed", expired, nil, library.ReleaseExpired},
		{"expires_at passed, until watched too", untilWatched(expired), nil, library.ReleaseExpired},
		{"until watched, not watched", untilWatched(movie), nil, ""},
		{"until watched, watched before the Keep", untilWatched(func() library.ResolvedDownload {
			d := movie
			d.Items = []library.CoveredItem{watchedAt(d.Items[0], before)}
			return d
		}()), nil, ""},
		{"until watched, watched since", untilWatched(func() library.ResolvedDownload {
			d := movie
			d.Items = []library.CoveredItem{watchedAt(d.Items[0], later)}
			return d
		}()), nil, library.ReleaseWatched},
		{"watched, but kept until removed", func() library.ResolvedDownload {
			d := movie
			d.Items = []library.CoveredItem{watchedAt(d.Items[0], later)}
			return d
		}(), nil, ""},
		{"an episode, watched since", untilWatched(showDL(2, library.DownloadScopeEpisode,
			watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later))), nil, library.ReleaseWatched},
		{"a season, all watched but still airing", untilWatched(showDL(3, library.DownloadScopeSeason,
			watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later), watchedAt(episode(11, 1, 2, "bb", "e2.mkv", 10), later))), nil, ""},
		{"a season, all watched, ended with its finale", untilWatched(showDL(3, library.DownloadScopeSeason,
			watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later), finale(watchedAt(episode(11, 1, 2, "bb", "e2.mkv", 10), later)))), nil, library.ReleaseWatched},
		{"a season with a later one, one episode unwatched", untilWatched(func() library.ResolvedDownload {
			d := showDL(3, library.DownloadScopeSeason, watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later), episode(11, 1, 2, "bb", "e2.mkv", 10))
			d.Title.LaterSeason = true
			return d
		}()), nil, ""},
		{"a season whose last episode hasn't aired", untilWatched(func() library.ResolvedDownload {
			e2 := watchedAt(episode(11, 1, 2, "", "", 0), later)
			e2.AirDate = "2026-12-01"
			d := showDL(3, library.DownloadScopeSeason, watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later), e2)
			d.Title.LaterSeason = true
			return d
		}()), nil, ""},
		{"a show, all watched but returning", untilWatched(showDL(4, library.DownloadScopeShow,
			watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later))), nil, ""},
		{"a show that ended, all watched; specials don't count", untilWatched(ended(showDL(4, library.DownloadScopeShow,
			episode(9, 0, 1, "bb", "sp.mkv", 10), watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later)))), nil, library.ReleaseWatched},
		{"a show that ended, one regular unwatched", untilWatched(ended(showDL(4, library.DownloadScopeShow,
			watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later), episode(11, 1, 2, "bb", "e2.mkv", 10)))), nil, ""},
		{"a smart pick, a later episode streams", smart(showDL(5, library.DownloadScopeEpisode, episode(10, 1, 3, "bb", "e3.mkv", 10))),
			map[int64][]episodeNumber{7: {{1, 4}}}, library.ReleaseWatched},
		{"a smart pick, an earlier episode streams", smart(showDL(5, library.DownloadScopeEpisode, episode(10, 1, 3, "bb", "e3.mkv", 10))),
			map[int64][]episodeNumber{7: {{1, 2}}}, ""},
		{"a kept episode, a later one streams", untilWatched(showDL(5, library.DownloadScopeEpisode, episode(10, 1, 3, "bb", "e3.mkv", 10))),
			map[int64][]episodeNumber{7: {{2, 1}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := releaseFor(&tt.d, now, time.UTC, tt.streaming)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("releaseFor = %q %v, want %q", got, ok, tt.want)
			}
		})
	}
}

func TestCover(t *testing.T) {
	later := t0.Add(time.Hour)
	v2 := NewPieceMap(v1Info("V2", 100))
	v2.hashes = nil // a pure v2 torrent: no v1 hashes
	pieces := func(hash, _ string) *PieceMap {
		if hash == "v2" {
			return v2
		}
		return nil
	}
	tests := []struct {
		name         string
		d            library.ResolvedDownload
		wants        []FileKey
		total, unav  int
		released     []FileKey
		itemsCovered int
	}{
		{"a movie", movieDL(1, "aa", "Film.mkv", 100), []FileKey{{"aa", "Film.mkv"}}, 1, 0, nil, 1},
		{"a shared file counts once; no file and HDRezka", showDL(2, library.DownloadScopeShow,
			episode(10, 1, 1, "bb", "e1-e2.mkv", 10), episode(11, 1, 2, "bb", "e1-e2.mkv", 10),
			episode(12, 1, 3, "", "", 0), episode(13, 1, 4, "hdrezka", "hdrezka/1/t1/s01e04/1080p.mp4", 0)),
			[]FileKey{{"bb", "e1-e2.mkv"}}, 2, 1, nil, 4},
		{"a pure v2 torrent's file is unavailable", showDL(3, library.DownloadScopeSeason,
			episode(10, 1, 1, "v2", "V2/a.mkv", 10), episode(11, 1, 2, "cc", "e2.mkv", 10)),
			[]FileKey{{"cc", "e2.mkv"}}, 2, 1, nil, 2},
		{"until watched: watched episodes leave, a file another still wants stays", func() library.ResolvedDownload {
			d := showDL(4, library.DownloadScopeShow,
				watchedAt(episode(10, 1, 1, "bb", "e1-e2.mkv", 10), later), episode(11, 1, 2, "bb", "e1-e2.mkv", 10),
				watchedAt(episode(12, 1, 3, "bb", "e3.mkv", 10), later), episode(13, 1, 4, "bb", "e4.mkv", 10))
			d.UntilWatched = true
			return d
		}(), []FileKey{{"bb", "e1-e2.mkv"}, {"bb", "e4.mkv"}}, 2, 0, []FileKey{{"bb", "e3.mkv"}}, 2},
		{"watched episodes stay when kept until removed", showDL(5, library.DownloadScopeShow,
			watchedAt(episode(10, 1, 1, "bb", "e1.mkv", 10), later)), []FileKey{{"bb", "e1.mkv"}}, 1, 0, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cov := cover(&tt.d, fileInput{pieces: pieces})
			var wants []FileKey
			for _, w := range cov.wants {
				wants = append(wants, w.key)
			}
			if !reflect.DeepEqual(wants, tt.wants) || cov.total != tt.total || cov.unavailable != tt.unav ||
				!reflect.DeepEqual(cov.released, tt.released) || len(cov.items) != tt.itemsCovered {
				t.Errorf("wants %v total %d unavailable %d released %v items %d; want %v %d %d %v %d",
					wants, cov.total, cov.unavailable, cov.released, len(cov.items), tt.wants, tt.total, tt.unav, tt.released, tt.itemsCovered)
			}
		})
	}
}

// admitted lists a download's admitted and over-cap files, sorted.
func admitted(a admission, id int64) (in, over []FileKey) {
	for k := range a.admitted[id] {
		in = append(in, k)
	}
	for k := range a.overCap[id] {
		over = append(over, k)
	}
	slices.SortFunc(in, compareKeys)
	slices.SortFunc(over, compareKeys)
	return in, over
}

func TestAdmit(t *testing.T) {
	plan := func(keptMax, smartMax int64, ds ...library.ResolvedDownload) admission {
		var covs []*coverage
		for i := range ds {
			covs = append(covs, cover(&ds[i], fileInput{pieces: noPieces}))
		}
		return admit(covs, keptMax, smartMax)
	}
	t.Run("kept by creation; a shared file counts once", func(t *testing.T) {
		a := movieDL(1, "aa", "A.mkv", 40)
		b := movieDL(2, "aa", "A.mkv", 40) // the same file
		c := movieDL(3, "cc", "C.mkv", 50)
		c.CreatedAt = t0.Add(time.Minute)
		adm := plan(100, 0, a, b, c)
		if adm.committedKept != 90 || !adm.kept[FileKey{"aa", "A.mkv"}] || !adm.kept[FileKey{"cc", "C.mkv"}] {
			t.Errorf("committed %d, kept %v", adm.committedKept, adm.kept)
		}
		if in, _ := admitted(adm, 2); len(in) != 1 {
			t.Errorf("the second keep of A doesn't hold it: %v", in)
		}
	})
	t.Run("a file assigned later is the one over the cap; a smaller one still fits", func(t *testing.T) {
		show := showDL(1, library.DownloadScopeShow, episode(10, 1, 1, "bb", "e1.mkv", 30), episode(11, 1, 2, "bb", "e2.mkv", 30))
		show.Items[1].File.AssignedAt = t0.Add(time.Hour) // E02 after the movie's Keep
		show.Items = append(show.Items, episode(12, 1, 3, "bb", "e3.mkv", 5))
		show.Items[2].File.AssignedAt = t0.Add(2 * time.Hour)
		movie := movieDL(2, "aa", "A.mkv", 60)
		movie.CreatedAt = t0.Add(time.Minute)
		adm := plan(100, 0, show, movie)
		in, over := admitted(adm, 1)
		if !reflect.DeepEqual(in, []FileKey{{"bb", "e1.mkv"}, {"bb", "e3.mkv"}}) || !reflect.DeepEqual(over, []FileKey{{"bb", "e2.mkv"}}) {
			t.Errorf("show: admitted %v, over %v", in, over)
		}
		if in, over := admitted(adm, 2); len(in) != 1 || len(over) != 0 {
			t.Errorf("the movie, kept before E02 came: admitted %v, over %v", in, over)
		}
		if adm.committedKept != 95 {
			t.Errorf("committed = %d, want 95 (over-cap files don't count)", adm.committedKept)
		}
	})
	t.Run("episodes in episode order within a Keep", func(t *testing.T) {
		show := showDL(1, library.DownloadScopeShow, episode(11, 1, 2, "bb", "e2.mkv", 60), episode(10, 1, 1, "bb", "e1.mkv", 60))
		in, over := admitted(plan(100, 0, show), 1)
		if !reflect.DeepEqual(in, []FileKey{{"bb", "e1.mkv"}}) || !reflect.DeepEqual(over, []FileKey{{"bb", "e2.mkv"}}) {
			t.Errorf("admitted %v, over %v", in, over)
		}
	})
	t.Run("smart has its own cap and never takes kept room; a kept file costs smart nothing", func(t *testing.T) {
		kept := movieDL(1, "aa", "A.mkv", 90)
		s1 := showDL(2, library.DownloadScopeEpisode, episode(10, 1, 1, "aa", "A.mkv", 90))
		s1.Reason = library.DownloadSmart
		s2 := showDL(3, library.DownloadScopeEpisode, episode(11, 1, 2, "bb", "e2.mkv", 15))
		s2.Reason = library.DownloadSmart
		s3 := showDL(4, library.DownloadScopeEpisode, episode(12, 1, 3, "bb", "e3.mkv", 15))
		s3.Reason, s3.CreatedAt = library.DownloadSmart, t0.Add(time.Minute)
		adm := plan(100, 20, kept, s1, s2, s3)
		if adm.committedKept != 90 || adm.committedSmart != 15 || !adm.smart[FileKey{"bb", "e2.mkv"}] {
			t.Errorf("committed kept %d smart %d, smart %v", adm.committedKept, adm.committedSmart, adm.smart)
		}
		if in, _ := admitted(adm, 2); len(in) != 1 {
			t.Errorf("the smart pick of a kept file isn't admitted: %v", in)
		}
		if _, over := admitted(adm, 4); len(over) != 1 {
			t.Errorf("the second smart pick fits a full smart cap: %v", over)
		}
	})
}

func TestWindowNow(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database:", err)
	}
	w := func(start, end string) library.DownloadWindow { return library.DownloadWindow{Start: start, End: end} }
	local := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, berlin) }
	tests := []struct {
		name    string
		windows []library.DownloadWindow
		now     time.Time
		inside  bool
		next    time.Time
	}{
		{"no windows: any time", nil, local(17, 12, 0), true, time.Time{}},
		{"inside a day window", []library.DownloadWindow{w("09:00", "17:00")}, local(17, 12, 0), true, time.Time{}},
		{"at its end: outside, tomorrow's start", []library.DownloadWindow{w("09:00", "17:00")}, local(17, 17, 0), false, local(18, 9, 0)},
		{"before its start: today's", []library.DownloadWindow{w("09:00", "17:00")}, local(17, 8, 59), false, local(17, 9, 0)},
		{"across midnight, before it", []library.DownloadWindow{w("23:00", "06:00")}, local(17, 23, 30), true, time.Time{}},
		{"across midnight, after it", []library.DownloadWindow{w("23:00", "06:00")}, local(18, 5, 59), true, time.Time{}},
		{"across midnight, outside", []library.DownloadWindow{w("23:00", "06:00")}, local(18, 6, 0), false, local(18, 23, 0)},
		{"the soonest of several", []library.DownloadWindow{w("23:00", "06:00"), w("13:00", "14:00")}, local(17, 12, 0), false, local(17, 13, 0)},
		// 25 October 2026: Berlin leaves summer time at 03:00.
		{"over the clock change", []library.DownloadWindow{w("02:30", "04:00")}, local(24, 23, 0), false, local(25, 2, 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The server's TZ is Berlin; now is given in UTC, as the clock has it.
			inside, next := windowNow(tt.windows, tt.now.UTC(), berlin)
			if inside != tt.inside || !next.Equal(tt.next) {
				t.Errorf("windowNow = %v %v, want %v %v", inside, next, tt.inside, tt.next)
			}
		})
	}
}

func TestStepStall(t *testing.T) {
	inSlot := stall{slotSince: t0}
	tests := []struct {
		name     string
		s        stall
		measure  int64
		now      time.Time
		stalled  bool
		attempts int
		retryAt  time.Time
	}{
		{"progress in a slot", inSlot, 10, t0.Add(20 * time.Minute), false, 0, time.Time{}},
		{"no slot, no stall", stall{}, 0, t0.Add(time.Hour), false, 0, time.Time{}},
		{"9 minutes without progress", inSlot, 0, t0.Add(9 * time.Minute), false, 0, time.Time{}},
		{"10 minutes without progress: stalled 5 minutes", inSlot, 0, t0.Add(10 * time.Minute), true, 1, t0.Add(15 * time.Minute)},
		{"the third stall in a row waits 20", stall{slotSince: t0, attempts: 2}, 0, t0.Add(10 * time.Minute), true, 3, t0.Add(30 * time.Minute)},
		{"progress after a stall starts the backoff over", stall{attempts: 3, retryAt: t0.Add(time.Hour), progress: 5}, 6, t0, false, 0, time.Time{}},
		{"progress counts from when it last grew", stall{slotSince: t0, progress: 5, progressAt: t0.Add(5 * time.Minute)}, 5, t0.Add(14 * time.Minute), false, 0, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stepStall(tt.s, tt.measure, tt.now)
			if got.stalled(tt.now) != tt.stalled || got.attempts != tt.attempts || !got.retryAt.Equal(tt.retryAt) {
				t.Errorf("stepStall = %+v (stalled %v)", got, got.stalled(tt.now))
			}
			if tt.stalled && !got.slotSince.IsZero() {
				t.Error("a stall keeps its slot")
			}
		})
	}
	for n, want := range map[int]time.Duration{1: 5 * time.Minute, 2: 10 * time.Minute, 4: 40 * time.Minute, 5: time.Hour, 9: time.Hour} {
		if got := backoff(n); got != want {
			t.Errorf("backoff(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestTorrentState(t *testing.T) {
	now := t0
	stalled := stall{retryAt: now.Add(time.Minute)}
	tests := []struct {
		name   string
		t      torrentIn
		h      holds
		inSlot bool
		want   State
	}{
		{"done wins over everything", torrentIn{done: true, paused: true}, holds{disabled: true, diskFull: true}, true, StateDone},
		{"the user's pause", torrentIn{paused: true}, holds{diskFull: true}, true, StatePaused},
		{"background downloads off", torrentIn{}, holds{disabled: true}, true, StatePaused},
		{"disk full", torrentIn{}, holds{diskFull: true, outside: true}, true, StateDiskFull},
		{"outside every window", torrentIn{}, holds{outside: true, stream: true}, true, StateWaitingForWindow},
		{"another torrent streams", torrentIn{}, holds{stream: true}, true, StateWaitingForStream},
		{"its own stream: it carries on", torrentIn{streamed: true}, holds{stream: true}, true, StateDownloading},
		{"stalled", torrentIn{stall: stalled}, holds{}, true, StateStalled},
		{"in a slot", torrentIn{}, holds{}, true, StateDownloading},
		{"waiting for a slot", torrentIn{}, holds{}, false, StateQueued},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := torrentState(tt.t, tt.h, now, tt.inSlot); got != tt.want {
				t.Errorf("torrentState = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAssignSlots(t *testing.T) {
	now := t0
	ts := []torrentIn{
		{hash: "a", order: 0},
		{hash: "b", order: 1},
		{hash: "c", order: 2, holder: true},
		{hash: "d", order: 3, streamed: true},
		{hash: "e", order: 4, done: true},
		{hash: "f", order: 5, stall: stall{retryAt: now.Add(time.Minute)}},
	}
	slots := assignSlots(ts, holds{}, now)
	if !reflect.DeepEqual(slots, map[string]bool{"d": true, "c": true}) {
		t.Errorf("slots = %v, want the streamed d and the holder c", slots)
	}
	if got := assignSlots(ts[:2], holds{}, now); !reflect.DeepEqual(got, map[string]bool{"a": true, "b": true}) {
		t.Errorf("slots = %v, want a and b in plan order", got)
	}
	if got := assignSlots(ts, holds{stream: true}, now); !reflect.DeepEqual(got, map[string]bool{"d": true}) {
		t.Errorf("while d streams, slots = %v, want d alone", got)
	}
	if got := assignSlots(ts, holds{outside: true}, now); len(got) != 0 {
		t.Errorf("outside the windows, slots = %v", got)
	}
}

func TestCombineAndDownloadState(t *testing.T) {
	combine := []struct {
		states []State
		want   State
	}{
		{nil, StateDone},
		{[]State{StateDone, StateDone}, StateDone},
		{[]State{StateDone, StateQueued, StateDownloading}, StateDownloading},
		{[]State{StateQueued, StateStalled, StateDone}, StateStalled},
		{[]State{StateQueued, StateWaitingForStream, StatePaused}, StatePaused},
		{[]State{StateWaitingForWindow, StateDiskFull}, StateDiskFull},
		{[]State{StateQueued}, StateQueued},
	}
	for _, tt := range combine {
		if got := combineStates(tt.states); got != tt.want {
			t.Errorf("combineStates(%v) = %s, want %s", tt.states, got, tt.want)
		}
	}
	download := []struct {
		name                         string
		total, downloadable, overCap int
		admitted, want               State
	}{
		{"nothing assigned", 0, 0, 0, StateDone, StateNoFile},
		{"HDRezka only", 2, 0, 0, StateDone, StateUnavailable},
		{"all on disk", 3, 3, 0, StateDone, StateDone},
		{"over the cap, the rest on disk", 3, 3, 1, StateDone, StateStorageFull},
		{"over the cap, the rest downloading", 3, 3, 1, StateDownloading, StateStorageFull},
		{"over the cap, paused wins", 3, 3, 1, StatePaused, StatePaused},
		{"over the cap, disk full wins", 3, 3, 1, StateDiskFull, StateDiskFull},
		{"downloading", 1, 1, 0, StateDownloading, StateDownloading},
	}
	for _, tt := range download {
		if got := downloadState(tt.total, tt.downloadable, tt.overCap, tt.admitted); got != tt.want {
			t.Errorf("%s: downloadState = %s, want %s", tt.name, got, tt.want)
		}
	}
}

// A download over several torrents: its admitted files' state per torrent,
// done where its own files are on disk whatever others want of it.
func TestViewDownload_MultiTorrent(t *testing.T) {
	d := showDL(1, library.DownloadScopeShow,
		episode(10, 1, 1, "aa", "e1.mkv", 100), episode(11, 1, 2, "bb", "e2.mkv", 100), episode(12, 1, 3, "cc", "e3.mkv", 100))
	cov := cover(&d, fileInput{pieces: noPieces})
	adm := admit([]*coverage{cov}, 1000, 0)
	facts := map[string]*torrentFacts{
		"aa": {done: map[string]int64{"e1.mkv": 100}, complete: map[string]bool{"e1.mkv": true}},
		"bb": {done: map[string]int64{"e2.mkv": 40}, complete: map[string]bool{}},
		"cc": {done: map[string]int64{}, complete: map[string]bool{}},
	}
	retry := t0.Add(5 * time.Minute)
	torrents := map[string]*torrentPlan{
		// aa is downloading for another download: e1 is on disk, so for this one it's done.
		"aa": {state: StateDownloading, speed: 9},
		"bb": {state: StateStalled, resumesAt: &retry},
		"cc": {state: StateQueued},
	}
	v := viewDownload(cov, adm, torrents, facts)
	dl := v.download
	if dl.State != StateStalled || dl.ResumesAt == nil || !dl.ResumesAt.Equal(retry) {
		t.Errorf("state %s resumes %v, want stalled until %v", dl.State, dl.ResumesAt, retry)
	}
	if dl.WantedBytes != 300 || dl.DoneBytes != 140 || dl.Files != (Files{Total: 3, Done: 1}) || dl.DownloadBps != 0 {
		t.Errorf("view = %+v", dl)
	}

	torrents["cc"] = &torrentPlan{state: StateDownloading, speed: 50}
	dl = viewDownload(cov, adm, torrents, facts).download
	if dl.State != StateDownloading || dl.DownloadBps != 50 || dl.EtaSeconds == nil || *dl.EtaSeconds != 4 {
		t.Errorf("one torrent downloading: %s at %d, eta %v", dl.State, dl.DownloadBps, dl.EtaSeconds)
	}
}

func TestListOrder(t *testing.T) {
	kept := func(id int64, created time.Duration) Download {
		return Download{ID: id, Reason: library.DownloadKept, CreatedAt: t0.Add(created)}
	}
	smart := func(id, show int64, s, e int) Download {
		return Download{ID: id, Reason: library.DownloadSmart, Item: Item{LibraryID: show, SeasonNumber: &s, EpisodeNumber: &e}}
	}
	items := []Download{smart(5, 2, 1, 3), kept(1, 0), smart(6, 1, 2, 1), kept(2, time.Hour), smart(7, 2, 1, 2)}
	slices.SortFunc(items, listOrder)
	var ids []int64
	for _, d := range items {
		ids = append(ids, d.ID)
	}
	if want := []int64{2, 1, 6, 7, 5}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order = %v, want %v (kept newest first, then smart by show and episode)", ids, want)
	}
}
