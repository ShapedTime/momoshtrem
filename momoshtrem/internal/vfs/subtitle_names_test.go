package vfs

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

func TestSubtitleFileNames(t *testing.T) {
	const base = "Severance - S02E05 - Trojan's Horse"
	sub := func(id int64, lang string, source subtitle.Source, format string) *subtitle.Subtitle {
		return &subtitle.Subtitle{ID: id, LanguageCode: lang, Source: source, Format: format}
	}
	dl, tor, rezka := subtitle.SourceOpenSubtitles, subtitle.SourceTorrent, subtitle.SourceHDRezka

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
			"several torrent files in a language: oldest first, counted per format",
			[]*subtitle.Subtitle{sub(9, "en", tor, "srt"), sub(3, "en", tor, "srt"), sub(4, "en", tor, "ass")},
			map[int64]string{3: base + ".en.srt", 4: base + ".en.ass", 9: base + ".2.en.srt"},
		},
		{
			"VobSub .idx/.sub pairs keep one stem",
			[]*subtitle.Subtitle{sub(1, "en", tor, "idx"), sub(2, "en", tor, "sub"), sub(3, "en", tor, "idx"), sub(4, "en", tor, "sub")},
			map[int64]string{1: base + ".en.idx", 2: base + ".en.sub", 3: base + ".2.en.idx", 4: base + ".2.en.sub"},
		},
		{
			"unknown-language files don't overwrite each other",
			[]*subtitle.Subtitle{sub(1, "unknown", tor, "srt"), sub(2, "unknown", tor, "srt"), sub(3, "", tor, "")},
			map[int64]string{1: base + ".unknown.srt", 2: base + ".2.unknown.srt", 3: base + ".3.unknown.srt"},
		},
		{
			// HDRezka's "Русский (альтернативный)" has no code: a second Russian track.
			"an HDRezka track named after a language",
			[]*subtitle.Subtitle{sub(1, "ru", rezka, "srt"), sub(2, "русский-альтернативный", rezka, "srt"), sub(3, "en", rezka, "srt")},
			map[int64]string{1: base + ".ru.srt", 2: base + ".2.ru.srt", 3: base + ".en.srt"},
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
	d := subtitleDir{dir: dir, dirPath: MoviesPath + "/Dune (2024)", base: "Dune (2024)", item: movie, file: subtitle.TorrentFile(hashA)}

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

	t.Run("an HDRezka stream replaced the torrent", func(t *testing.T) {
		tree, moviesDir, _ := newEmptyTree()
		dir := NewVirtualDir("Dune (2024)")
		moviesDir.children[dir.name] = dir
		d := d
		d.dir, d.file = dir, currentFile(&library.TorrentAssignment{Kind: library.AssignmentKindHDRezka})
		addSubtitleFiles(tree, d, subs[movie])
		if len(dir.children) != 1 || dir.children["Dune (2024).en.srt"] == nil {
			t.Errorf("files = %v, want the downloaded one only", dir.children)
		}
	})

	t.Run("no repository", func(t *testing.T) {
		if got := (&LibraryFS{}).loadSubtitles([]subtitleDir{d}); got != nil {
			t.Errorf("loadSubtitles = %v, want nil", got)
		}
	})
}

// An HDRezka movie's subtitles sit next to its .mp4 as local files (an
// earlier torrent's don't), and the v5 cache restores them.
func TestAddSubtitleFiles_HDRezkaStream(t *testing.T) {
	ref := library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		TranslatorID: 238, TranslatorName: "Оригинал (+субтитры)", Quality: "1080p"}
	stream := &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka, Stream: &ref, FilePath: ref.FilePath(), FileSize: 3322740623}
	movie := subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 7}
	fs := NewTestFS([]TestMovie{{Title: "Interstellar", Year: 2014, Assignment: stream}}, nil)
	fs.subtitleRepo = &fakeSubtitles{subs: map[subtitle.Item][]*subtitle.Subtitle{movie: {
		{ID: 1, LanguageCode: "en", Format: "srt", Source: subtitle.SourceHDRezka, FilePath: "/subs/hdrezka/movie/7/en.srt", FileSize: 10},
		{ID: 2, LanguageCode: "ru", Format: "srt", Source: subtitle.SourceHDRezka, FilePath: "/subs/hdrezka/movie/7/ru.srt", FileSize: 20},
		{ID: 3, LanguageCode: "de", Format: "srt", Source: subtitle.SourceTorrent, InfoHash: "aaaa", FilePath: "Old/de.srt", FileSize: 30},
	}}}

	folder := MovieDir("Interstellar", 2014)
	d := subtitleDir{dir: fs.tree.pathMap[folder].(*VirtualDir), dirPath: folder, base: "Interstellar (2014)", item: movie, file: currentFile(stream)}
	addSubtitleFiles(fs.tree, d, fs.loadSubtitles([]subtitleDir{d})[movie])

	list := func(fs *LibraryFS) []string {
		t.Helper()
		entries, err := fs.ReadDir(folder)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for name := range entries {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}
	want := []string{"Interstellar (2014).en.srt", "Interstellar (2014).mp4", "Interstellar (2014).ru.srt"}
	if got := list(fs); !reflect.DeepEqual(got, want) {
		t.Fatalf("folder = %v, want %v", got, want)
	}

	cacheDir := t.TempDir()
	(&LibraryFS{cacheDir: cacheDir, tree: fs.tree}).saveTreeToCache()
	loader := &LibraryFS{cacheDir: cacheDir}
	if err := loader.loadTreeFromCache(); err != nil {
		t.Fatalf("loadTreeFromCache: %v", err)
	}
	if got := list(loader); !reflect.DeepEqual(got, want) {
		t.Errorf("folder from the cache = %v, want %v", got, want)
	}
	for lang, path := range map[string]string{"en": "/subs/hdrezka/movie/7/en.srt", "ru": "/subs/hdrezka/movie/7/ru.srt"} {
		f, ok := loader.tree.pathMap[folder+"/Interstellar (2014)."+lang+".srt"].(*SubtitleFile)
		if !ok || f.localPath != path {
			t.Errorf("%s from the cache = %#v, want a local file at %s", lang, f, path)
		}
	}
}
