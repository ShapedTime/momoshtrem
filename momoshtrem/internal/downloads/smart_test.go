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
			name: "a streaming pick stays after its stream ends, the first k of its show",
			k:    2,
			picks: []library.SmartPick{pick(7, 1, 6, library.SmartStreaming), pick(8, 1, 4, library.SmartStreaming),
				pick(9, 1, 5, library.SmartStreaming)},
			releases: []library.DownloadRelease{{ID: 7, Reason: library.ReleaseExpired}},
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
	tests := []struct {
		name    string
		creates []smartCreate
		room    smartRoom
		refused []int64 // the re-pick rules refuse them
		made    []int64
		skipped int
	}{
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
			var made []int64
			skipped, err := fitSmart(tt.creates, tt.room, func(c smartCreate) (bool, error) {
				if slices.Contains(tt.refused, c.episodeID) {
					return false, nil
				}
				made = append(made, c.episodeID)
				return true, nil
			})
			if err != nil || !reflect.DeepEqual(made, tt.made) || skipped != tt.skipped {
				t.Errorf("made %v, skipped %d (%v); want %v, %d", made, skipped, err, tt.made, tt.skipped)
			}
		})
	}
}
