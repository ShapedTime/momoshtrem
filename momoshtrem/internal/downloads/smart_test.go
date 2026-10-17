package downloads

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// The smart picks' rules, table by table.

// nextEp is show 1's episode S{s}E{e} (id s*100+e) with a 100-byte file of
// its own.
func nextEp(s, e int) library.NextEpisodeFile {
	return library.NextEpisodeFile{EpisodeID: int64(s*100 + e), SeasonNumber: s, EpisodeNumber: e,
		InfoHash: "aa", FilePath: fmt.Sprintf("S%02dE%02d.mkv", s, e), FileSize: 100}
}

// pick is an active smart pick of show 1's S{s}E{e}, made at t0, with its
// torrent file.
func pick(id int64, s, e int, source library.SmartSource) library.SmartPick {
	return library.SmartPick{ID: id, EpisodeID: int64(s*100 + e), ShowID: 1, SeasonNumber: s, EpisodeNumber: e,
		Source: source, CreatedAt: t0, HasFile: true}
}

func TestSmartAnchors(t *testing.T) {
	recent, old := t0.Add(-20*24*time.Hour), t0.Add(-22*24*time.Hour)
	newer := t0.Add(-time.Hour)
	tests := []struct {
		name      string
		upNext    []UpNextShow
		streaming map[int64][]episodeNumber
		want      []smartAnchor
	}{
		{name: "nothing watched"},
		{
			name:      "streaming: from the episode after, specials anchor nothing, one anchor per episode",
			streaming: map[int64][]episodeNumber{2: {{1, 3}, {1, 3}}, 1: {{0, 4}, {2, 5}}},
			want: []smartAnchor{
				{1, episodeNumber{2, 6}, library.SmartStreaming},
				{2, episodeNumber{1, 4}, library.SmartStreaming},
			},
		},
		{
			name: "Up next: watched in the last 21 days, from the episode it names, newest first",
			upNext: []UpNextShow{
				{ShowID: 3, SeasonNumber: 1, EpisodeNumber: 2, LastWatchedAt: &recent},
				{ShowID: 4, SeasonNumber: 2, EpisodeNumber: 1, LastWatchedAt: &old},
				{ShowID: 5, SeasonNumber: 1, EpisodeNumber: 1}, // a watchlist start
				{ShowID: 6, SeasonNumber: 3, EpisodeNumber: 7, LastWatchedAt: &newer},
			},
			want: []smartAnchor{
				{6, episodeNumber{3, 7}, library.SmartUpNext},
				{3, episodeNumber{1, 2}, library.SmartUpNext},
			},
		},
		{
			name:      "streaming comes first",
			upNext:    []UpNextShow{{ShowID: 1, SeasonNumber: 1, EpisodeNumber: 3, LastWatchedAt: &newer}},
			streaming: map[int64][]episodeNumber{1: {{1, 3}}},
			want: []smartAnchor{
				{1, episodeNumber{1, 4}, library.SmartStreaming},
				{1, episodeNumber{1, 3}, library.SmartUpNext},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smartAnchors(tt.upNext, tt.streaming, t0); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("anchors = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanSmart(t *testing.T) {
	upNext := func(s, e int) smartAnchor { return smartAnchor{1, episodeNumber{s, e}, library.SmartUpNext} }
	streamFrom := func(s, e int) smartAnchor { return smartAnchor{1, episodeNumber{s, e}, library.SmartStreaming} }
	win := func(eps ...library.NextEpisodeFile) []library.NextEpisodeFile { return eps }
	heldBack := func(ep library.NextEpisodeFile) library.NextEpisodeFile { ep.Blocked = "expired"; return ep }
	watched := func(p library.SmartPick, at time.Time) library.SmartPick { p.WatchedAt = &at; return p }
	noFile := func(p library.SmartPick) library.SmartPick { p.HasFile = false; return p }
	type create struct {
		episode int64
		source  library.SmartSource
	}
	tests := []struct {
		name      string
		k         int
		anchors   []smartAnchor
		windows   [][]library.NextEpisodeFile
		picks     []library.SmartPick
		streaming map[int64][]episodeNumber
		creates   []create
		releases  []library.DownloadRelease
	}{
		{
			// Day 14: Up next's E3 and E4 ran their lifetime, and nobody
			// played the show since: their slots stay theirs.
			name: "an episode held back after its lifetime takes its slot, unpicked",
			k:    2, anchors: []smartAnchor{upNext(1, 3)},
			windows: [][]library.NextEpisodeFile{win(heldBack(nextEp(1, 3)), heldBack(nextEp(1, 4)))},
		},
		{
			name: "the slots after a held-back one are picked",
			k:    2, anchors: []smartAnchor{upNext(1, 3)},
			windows: [][]library.NextEpisodeFile{win(heldBack(nextEp(1, 3)), nextEp(1, 4))},
			creates: []create{{104, library.SmartUpNext}},
		},
		{
			name: "Up next's window, at most k",
			k:    2, anchors: []smartAnchor{upNext(1, 3)}, windows: [][]library.NextEpisodeFile{win(nextEp(1, 3), nextEp(1, 4), nextEp(1, 5))},
			creates: []create{{103, library.SmartUpNext}, {104, library.SmartUpNext}},
		},
		{
			name:    "an episode two anchors name is picked once, the first's",
			k:       2,
			anchors: []smartAnchor{streamFrom(1, 4), upNext(1, 3)},
			windows: [][]library.NextEpisodeFile{win(nextEp(1, 4), nextEp(1, 5)), win(nextEp(1, 3), nextEp(1, 4))},
			creates: []create{{104, library.SmartStreaming}, {105, library.SmartStreaming}, {103, library.SmartUpNext}},
		},
		{
			name: "a candidate picked already stays",
			k:    2, anchors: []smartAnchor{upNext(1, 3)}, windows: [][]library.NextEpisodeFile{win(nextEp(1, 3), nextEp(1, 4))},
			picks:   []library.SmartPick{pick(9, 1, 3, library.SmartStreaming)},
			creates: []create{{104, library.SmartUpNext}},
		},
		{
			name:     "Up next's picks go once the show isn't in it (or Trakt is off)",
			k:        2,
			picks:    []library.SmartPick{pick(9, 1, 3, library.SmartUpNext)},
			releases: []library.DownloadRelease{{ID: 9, Reason: library.ReleaseExpired}},
		},
		{
			name: "a pick out of the window goes (k lowered)",
			k:    1, anchors: []smartAnchor{upNext(1, 3)}, windows: [][]library.NextEpisodeFile{win(nextEp(1, 3))},
			picks:    []library.SmartPick{pick(8, 1, 3, library.SmartUpNext), pick(9, 1, 4, library.SmartUpNext)},
			releases: []library.DownloadRelease{{ID: 9, Reason: library.ReleaseExpired}},
		},
		{
			name: "a streaming pick stays after its stream ends (or pauses), the first k+1 of its show",
			k:    2,
			picks: []library.SmartPick{pick(6, 1, 7, library.SmartStreaming), pick(7, 1, 6, library.SmartStreaming),
				pick(8, 1, 4, library.SmartStreaming), pick(9, 1, 5, library.SmartStreaming)},
			releases: []library.DownloadRelease{{ID: 6, Reason: library.ReleaseExpired}},
		},
		{
			// Stream E1, autoplay E2 (E4 joins), pause E2: E2 and the two
			// after it stay, so resuming finds them picked.
			name: "a paused episode and the k after it stay",
			k:    2,
			picks: []library.SmartPick{pick(7, 1, 2, library.SmartStreaming), pick(8, 1, 3, library.SmartStreaming),
				pick(9, 1, 4, library.SmartStreaming)},
		},
		{
			// Up next names E1 (skipped, or stopped at 85%) while E3
			// streams: the planner would release E1 and E2 as watched.
			name:      "Up next's episodes before the one streaming aren't picked",
			k:         2,
			anchors:   []smartAnchor{streamFrom(1, 4), upNext(1, 1)},
			windows:   [][]library.NextEpisodeFile{win(nextEp(1, 4), nextEp(1, 5)), win(nextEp(1, 1), nextEp(1, 2))},
			streaming: map[int64][]episodeNumber{1: {{1, 3}}},
			creates:   []create{{104, library.SmartStreaming}, {105, library.SmartStreaming}},
		},
		{
			name: "a streaming pick stays while Up next names earlier episodes",
			k:    2, anchors: []smartAnchor{upNext(1, 3)}, windows: [][]library.NextEpisodeFile{win(nextEp(1, 3), nextEp(1, 4))},
			picks:   []library.SmartPick{pick(9, 1, 5, library.SmartStreaming)},
			creates: []create{{103, library.SmartUpNext}, {104, library.SmartUpNext}},
		},
		{
			name: "a streaming pick out of a stream's window goes",
			k:    2, anchors: []smartAnchor{streamFrom(1, 2)}, windows: [][]library.NextEpisodeFile{win(nextEp(1, 2), nextEp(1, 3))},
			streaming: map[int64][]episodeNumber{1: {{1, 1}}},
			picks:     []library.SmartPick{pick(9, 1, 7, library.SmartStreaming)},
			creates:   []create{{102, library.SmartStreaming}, {103, library.SmartStreaming}},
			releases:  []library.DownloadRelease{{ID: 9, Reason: library.ReleaseExpired}},
		},
		{
			name:     "a held pick without a torrent file goes",
			k:        2,
			picks:    []library.SmartPick{noFile(pick(9, 1, 4, library.SmartStreaming))},
			releases: []library.DownloadRelease{{ID: 9, Reason: library.ReleaseExpired}},
		},
		{
			name: "the episode streaming now stays, and is the planner's once a later one streams",
			k:    1, anchors: []smartAnchor{streamFrom(1, 5)}, windows: [][]library.NextEpisodeFile{win(nextEp(1, 5))},
			streaming: map[int64][]episodeNumber{1: {{1, 4}}},
			picks:     []library.SmartPick{pick(8, 1, 3, library.SmartUpNext), pick(9, 1, 4, library.SmartStreaming)},
			creates:   []create{{105, library.SmartStreaming}},
		},
		{
			name: "watched since it was picked: the planner's; watched before (a late play): released watched",
			k:    2,
			picks: []library.SmartPick{watched(pick(8, 1, 3, library.SmartUpNext), t0.Add(time.Hour)),
				watched(pick(9, 1, 4, library.SmartUpNext), t0.Add(-time.Hour))},
			releases: []library.DownloadRelease{{ID: 9, Reason: library.ReleaseWatched}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := planSmart(smartInput{k: tt.k, anchors: tt.anchors, windows: tt.windows, picks: tt.picks, streaming: tt.streaming})
			var creates []create
			for _, c := range plan.creates {
				creates = append(creates, create{c.episodeID, c.source})
			}
			if !reflect.DeepEqual(creates, tt.creates) {
				t.Errorf("creates = %v, want %v", creates, tt.creates)
			}
			if !reflect.DeepEqual(plan.releases, tt.releases) {
				t.Errorf("releases = %v, want %v", plan.releases, tt.releases)
			}
		})
	}
}

func TestFitSmart(t *testing.T) {
	c := func(episode int64, path string, size int64) smartCreate {
		return smartCreate{episodeID: episode, source: library.SmartUpNext, file: FileKey{"aa", path}, size: size}
	}
	room := func(left int64, has []string, kept ...int64) smartRoom {
		return smartRoom{known: true, left: left,
			has:  func(k FileKey) bool { return k.Hash == "aa" && slices.Contains(has, k.Path) },
			kept: func(id int64) bool { return slices.Contains(kept, id) }}
	}
	stream := func(c smartCreate) smartCreate { c.source = library.SmartStreaming; return c }
	tests := []struct {
		name    string
		creates []smartCreate
		room    smartRoom
		refused []int64 // the re-pick rules refuse them
		// frees is what evict frees for a stream's pick (0: nothing).
		frees   int64
		made    []int64
		evicted []int64 // the picks evict was asked to make room for
		skipped int
	}{
		{
			name:    "a stream's pick that doesn't fit has room made; Up next's doesn't",
			creates: []smartCreate{c(1, "a", 80), c(2, "b", 50), stream(c(3, "c", 50))},
			room:    room(100, nil),
			frees:   40,
			made:    []int64{1, 3}, evicted: []int64{3}, skipped: 1,
		},
		{
			name:    "when nothing can be freed, it's skipped",
			creates: []smartCreate{stream(c(1, "a", 150))},
			room:    room(100, nil),
			evicted: []int64{1}, skipped: 1,
		},
		{
			name:    "before the first pass, the planner decides",
			creates: []smartCreate{c(1, "a", 500), c(2, "b", 500)},
			room:    roomOf(nil),
			made:    []int64{1, 2},
		},
		{
			name:    "what doesn't fit is skipped; a later, smaller one may fit",
			creates: []smartCreate{c(1, "a", 60), c(2, "b", 60), c(3, "c", 40)},
			room:    room(100, nil),
			made:    []int64{1, 3}, skipped: 1,
		},
		{
			name:    "a file a download has, or an earlier pick's, costs nothing",
			creates: []smartCreate{c(1, "kept", 500), c(2, "pair", 80), c(3, "pair", 80), c(4, "d", 20)},
			room:    room(100, []string{"kept"}),
			made:    []int64{1, 2, 3, 4},
		},
		{
			name:    "an episode a Keep holds isn't picked",
			creates: []smartCreate{c(1, "a", 10), c(2, "b", 10)},
			room:    room(100, nil, 1),
			made:    []int64{2},
		},
		{
			name:    "a pick the re-pick rules refuse takes no room",
			creates: []smartCreate{c(1, "a", 100), c(2, "b", 100), c(3, "b", 100)},
			room:    room(100, nil),
			refused: []int64{1},
			made:    []int64{2, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var made, evicted []int64
			skipped, err := fitSmart(tt.creates, tt.room, func(c smartCreate) (bool, error) {
				if slices.Contains(tt.refused, c.episodeID) {
					return false, nil
				}
				made = append(made, c.episodeID)
				return true, nil
			}, func(c smartCreate, need int64) (smartRoom, bool, error) {
				evicted = append(evicted, c.episodeID)
				if tt.frees < need {
					return smartRoom{}, false, nil
				}
				after := tt.room
				after.left = c.size - need + tt.frees // what was left, and what was freed
				return after, true, nil
			})
			if err != nil || !reflect.DeepEqual(made, tt.made) || !reflect.DeepEqual(evicted, tt.evicted) || skipped != tt.skipped {
				t.Errorf("made %v, evicted for %v, skipped %d (%v); want %v, %v, %d",
					made, evicted, skipped, err, tt.made, tt.evicted, tt.skipped)
			}
		})
	}
}

func TestVictimsFor(t *testing.T) {
	days := func(n int) *time.Time { at := t0.Add(-time.Duration(n) * 24 * time.Hour); return &at }
	v := func(id, show, bytes int64, done bool, watched *time.Time) smartVictim {
		return smartVictim{id: id, show: show, bytes: bytes, done: done, watched: watched}
	}
	tests := []struct {
		name string
		need int64
		pool []smartVictim
		want []int64
	}{
		{
			name: "the least recently watched shows' first, a show's newest pick first",
			need: 150,
			pool: []smartVictim{v(1, 2, 100, false, days(1)), v(2, 3, 100, false, days(10)), v(3, 3, 100, false, days(10)),
				v(4, 4, 100, false, nil)},
			want: []int64{4, 3},
		},
		{
			name: "never the streamed show's own, nor one all on disk of a show watched within 7 days",
			need: 100,
			pool: []smartVictim{v(1, 1, 500, false, days(30)), v(2, 2, 500, true, days(6)), v(3, 3, 100, true, days(8))},
			want: []int64{3},
		},
		{
			name: "not enough to free: none",
			need: 300,
			pool: []smartVictim{v(1, 2, 100, false, days(9)), v(2, 3, 100, false, days(9)), v(3, 4, 0, false, days(9))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int64
			for _, v := range victimsFor(tt.need, 1, tt.pool, t0) {
				got = append(got, v.id)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("victims = %v, want %v", got, tt.want)
			}
		})
	}
}

// The victims: Up next's picks of shows not streaming, whose file smart
// pays for and no other pick wants, so releasing one frees its bytes.
func TestVictimPool(t *testing.T) {
	file := func(p library.SmartPick, path string) library.SmartPick {
		p.File = library.NextEpisodeFile{EpisodeID: p.EpisodeID, InfoHash: "aa", FilePath: path, FileSize: 100}
		return p
	}
	ofShow := func(p library.SmartPick, show int64) library.SmartPick { p.ShowID = show; return p }
	noFile := func(p library.SmartPick) library.SmartPick { p.HasFile = false; return p }
	watched := t0.Add(-48 * time.Hour)
	snap := &Snapshot{
		smartFiles: map[FileKey]bool{{"aa", "a"}: true, {"aa", "pair"}: true, {"aa", "done"}: true, {"aa", "s"}: true},
		files:      map[FileKey]FileStatus{{"aa", "done"}: {Bytes: 100, DoneBytes: 100, Complete: true}},
	}
	picks := []library.SmartPick{
		file(pick(1, 1, 1, library.SmartUpNext), "a"),
		file(pick(2, 1, 2, library.SmartUpNext), "kept"), // a Keep has its file: smart doesn't pay for it
		file(pick(3, 1, 3, library.SmartUpNext), "pair"), // another pick wants its file
		file(pick(4, 1, 4, library.SmartStreaming), "pair"),
		file(pick(5, 1, 5, library.SmartUpNext), "done"),
		file(ofShow(pick(6, 1, 6, library.SmartUpNext), 2), "s"), // its show streams
		file(pick(7, 1, 7, library.SmartUpNext), "released"),     // released this pass
		noFile(pick(8, 1, 8, library.SmartUpNext)),
		file(pick(9, 1, 9, library.SmartStreaming), "stream"), // a stream's own pick
	}
	pool := victimPool(picks, map[int64]bool{7: true}, []UpNextShow{{ShowID: 1, LastWatchedAt: &watched}},
		map[int64][]episodeNumber{2: {{1, 1}}}, snap)
	want := []smartVictim{{id: 1, show: 1, bytes: 100, watched: &watched}, {id: 5, show: 1, bytes: 100, done: true, watched: &watched}}
	if !reflect.DeepEqual(pool, want) {
		t.Errorf("pool = %+v\nwant   %+v", pool, want)
	}
	if victimPool(picks, nil, nil, nil, nil) != nil {
		t.Error("a pool before the first pass")
	}
}
