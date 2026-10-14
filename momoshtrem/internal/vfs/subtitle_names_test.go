package vfs

import (
	"context"
	"reflect"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

func TestSubtitleFileNames(t *testing.T) {
	const base = "Severance - S02E05 - Trojan's Horse"
	sub := func(id int64, lang string, source subtitle.Source, format string) *subtitle.Subtitle {
		return &subtitle.Subtitle{ID: id, LanguageCode: lang, Source: source, Format: format}
	}
	dl, tor := subtitle.SourceOpenSubtitles, subtitle.SourceTorrent

	tests := []struct {
		name string
		subs []*subtitle.Subtitle
		want map[int64]string
	}{
		{
			"one per language keeps the plain name",
			[]*subtitle.Subtitle{sub(1, "en", dl, "srt"), sub(2, "ru", tor, "ass")},
			map[int64]string{1: base + ".en.srt", 2: base + ".ru.ass"},
		},
		{
			// The downloaded one wins the plain name though the torrent's is older.
			"downloaded before torrent",
			[]*subtitle.Subtitle{sub(1, "en", tor, "srt"), sub(5, "en", dl, "srt")},
			map[int64]string{5: base + ".en.srt", 1: base + ".2.en.srt"},
		},
		{
			"several torrent files in a language: oldest first",
			[]*subtitle.Subtitle{sub(9, "en", tor, "srt"), sub(3, "en", tor, "srt"), sub(4, "en", tor, "ass")},
			map[int64]string{3: base + ".en.srt", 4: base + ".2.en.ass", 9: base + ".3.en.srt"},
		},
		{
			"unknown-language files don't overwrite each other",
			[]*subtitle.Subtitle{sub(1, "unknown", tor, "srt"), sub(2, "unknown", tor, "srt"), sub(3, "", tor, "")},
			map[int64]string{1: base + ".unknown.srt", 2: base + ".2.unknown.srt", 3: base + ".3.unknown.srt"},
		},
		{"none", nil, map[int64]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make(map[int64]string)
			for _, ns := range subtitleFileNames(base, tt.subs) {
				got[ns.Sub.ID] = ns.Name
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("names = %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeSubtitles is a subtitleLister over a fixed set.
type fakeSubtitles struct {
	subs  map[subtitle.Item][]*subtitle.Subtitle
	calls int
}

func (f *fakeSubtitles) GetByItems(_ context.Context, items []subtitle.Item) (map[subtitle.Item][]*subtitle.Subtitle, error) {
	f.calls++
	out := make(map[subtitle.Item][]*subtitle.Subtitle)
	for _, it := range items {
		if s, ok := f.subs[it]; ok {
			out[it] = s
		}
	}
	return out, nil
}

func TestAddSubtitleFiles_OnlyTheAssignedTorrents(t *testing.T) {
	const hashA, hashB = "aaaa", "bbbb"
	movie := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 1}
	fake := &fakeSubtitles{subs: map[subtitle.Item][]*subtitle.Subtitle{movie: {
		{ID: 1, LanguageCode: "en", Format: "srt", Source: subtitle.SourceOpenSubtitles, FilePath: "/subs/movie/1/en.srt", FileSize: 10},
		{ID: 2, LanguageCode: "en", Format: "srt", Source: subtitle.SourceTorrent, InfoHash: hashA, FilePath: "Dune/en.srt", FileSize: 20},
		{ID: 3, LanguageCode: "ru", Format: "srt", Source: subtitle.SourceTorrent, InfoHash: hashB, FilePath: "Old/ru.srt", FileSize: 30},
	}}}
	fs := &LibraryFS{subtitleRepo: fake}

	tree, moviesDir, _ := newEmptyTree()
	dir := NewVirtualDir("Dune (2024)")
	moviesDir.children[dir.name] = dir
	d := subtitleDir{dir, MoviesPath + "/Dune (2024)", "Dune (2024)", movie, hashA}

	subs := fs.loadSubtitles([]subtitleDir{d})
	addSubtitleFiles(tree, d, subs[movie])

	if fake.calls != 1 {
		t.Errorf("queries = %d, want 1", fake.calls)
	}
	local, ok := tree.pathMap["/Movies/Dune (2024)/Dune (2024).en.srt"].(*SubtitleFile)
	if !ok || local.localPath != "/subs/movie/1/en.srt" {
		t.Errorf("downloaded entry = %#v", tree.pathMap["/Movies/Dune (2024)/Dune (2024).en.srt"])
	}
	streamed, ok := tree.pathMap["/Movies/Dune (2024)/Dune (2024).2.en.srt"].(*TorrentSubtitleFile)
	if !ok || streamed.infoHash != hashA || streamed.torrentPath != "Dune/en.srt" {
		t.Errorf("torrent entry = %#v", tree.pathMap["/Movies/Dune (2024)/Dune (2024).2.en.srt"])
	}
	if len(dir.children) != 2 {
		t.Errorf("files = %d, want 2 (the old torrent's ru subtitle is left out)", len(dir.children))
	}

	t.Run("no repository", func(t *testing.T) {
		if got := (&LibraryFS{}).loadSubtitles([]subtitleDir{d}); got != nil {
			t.Errorf("loadSubtitles = %v, want nil", got)
		}
	})
}
