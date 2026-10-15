package vfs

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// cacheFixture is a tree with a movie and an episode, each with a
// downloaded and a torrent subtitle.
func cacheFixture() *DirectoryTree {
	tree, moviesDir, tvDir := newEmptyTree()
	add := func(dir *VirtualDir, dirPath string, e Entry) {
		dir.children[e.Name()] = e
		tree.pathMap[dirPath+"/"+e.Name()] = e
	}
	addDir := func(parent *VirtualDir, parentPath, name string) (*VirtualDir, string) {
		d := NewVirtualDir(name)
		add(parent, parentPath, d)
		return d, parentPath + "/" + name
	}

	movie, moviePath := addDir(moviesDir, MoviesPath, "Dune - Part Two (2024)")
	add(movie, moviePath, NewPlaceholderFile("Dune - Part Two (2024).mkv", 5000, &library.TorrentAssignment{
		InfoHash: "aaaa", MagnetURI: "magnet:?xt=urn:btih:aaaa", FilePath: "Dune/Dune.mkv", FileSize: 5000}))
	add(movie, moviePath, NewSubtitleFile("Dune - Part Two (2024).en.srt", "/data/subtitles/movie/1/en.srt", 10))
	add(movie, moviePath, NewTorrentSubtitleFile("Dune - Part Two (2024).2.en.srt", "Dune/Subs/English.srt", 20, "aaaa"))

	show, showPath := addDir(tvDir, TVShowsPath, "Severance (2022)")
	season, seasonPath := addDir(show, showPath, "Season 02")
	add(season, seasonPath, NewPlaceholderFile("Severance - S02E05 - Trojan's Horse.mkv", 3000, &library.TorrentAssignment{
		InfoHash: "bbbb", MagnetURI: "magnet:?xt=urn:btih:bbbb", FilePath: "Sev/E05.mkv", FileSize: 3000}))
	add(season, seasonPath, NewTorrentSubtitleFile("Severance - S02E05 - Trojan's Horse.unknown.srt", "Sev/Subs/E05/2.srt", 30, "bbbb"))
	add(season, seasonPath, NewTorrentSubtitleFile("Severance - S02E05 - Trojan's Horse.2.unknown.srt", "Sev/Subs/E05/3.srt", 31, "bbbb"))
	add(season, seasonPath, NewSubtitleFile("Severance - S02E05 - Trojan's Horse.ru.srt", "/data/subtitles/episode/9/ru.srt", 40))
	return tree
}

// Before v2 the cache had no subtitles, so after a restart that loaded the
// tree from it they were gone from WebDAV.
func TestCache_RoundTripKeepsSubtitles(t *testing.T) {
	dir := t.TempDir()
	want := cacheFixture()

	saver := &LibraryFS{cacheDir: dir, tree: want}
	saver.saveTreeToCache()

	loader := &LibraryFS{cacheDir: dir}
	if err := loader.loadTreeFromCache(); err != nil {
		t.Fatalf("loadTreeFromCache: %v", err)
	}
	got := loader.tree

	if len(got.pathMap) != len(want.pathMap) {
		t.Errorf("entries = %d, want %d", len(got.pathMap), len(want.pathMap))
	}
	for path, w := range want.pathMap {
		g, ok := got.pathMap[path]
		if !ok {
			t.Errorf("%s missing after reload", path)
			continue
		}
		switch w := w.(type) {
		case *SubtitleFile:
			g, ok := g.(*SubtitleFile)
			if !ok || g.name != w.name || g.localPath != w.localPath || g.size != w.size {
				t.Errorf("%s = %#v, want %#v", path, g, w)
			}
		case *TorrentSubtitleFile:
			if g, ok := g.(*TorrentSubtitleFile); !ok || *g != *w {
				t.Errorf("%s = %#v, want %#v", path, g, w)
			}
		case *PlaceholderFile:
			g, ok := g.(*PlaceholderFile)
			if !ok || g.size != w.size || g.assignment.InfoHash != w.assignment.InfoHash || g.assignment.FilePath != w.assignment.FilePath {
				t.Errorf("%s = %#v, want %#v", path, g, w)
			}
		case *VirtualDir:
			g, ok := g.(*VirtualDir)
			if !ok || len(g.children) != len(w.children) {
				t.Errorf("%s: dir with %d children, want %d", path, len(g.children), len(w.children))
			}
		}
	}
}

// Every entry ReadDir returns can be statted: WebDAV PROPFIND stats them
// all, and a nil torrent subtitle entry used to panic the listing.
func TestReadDir_ListsTorrentSubtitles(t *testing.T) {
	fs := &LibraryFS{tree: cacheFixture()}
	for _, dir := range []string{"/Movies/Dune - Part Two (2024)", "/TV Shows/Severance (2022)/Season 02"} {
		entries, err := fs.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%s): %v", dir, err)
		}
		for name, f := range entries {
			if f == nil {
				t.Errorf("%s/%s: nil entry", dir, name)
				continue
			}
			if info, err := f.Stat(); err != nil || info.Name() != name {
				t.Errorf("%s/%s: Stat = %v, %v", dir, name, info, err)
			}
		}
	}
}

// A v5 cache keeps each file's source: an HDRezka stream restored from it
// still opens as a remote file, not as a torrent with an empty hash.
func TestCache_RoundTripKeepsStreams(t *testing.T) {
	movie := &library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		Title: "Интерстеллар", TranslatorID: 238, TranslatorName: "Оригинал (+субтитры)", Quality: "1080p"}
	episode := &library.StreamRef{Provider: "hdrezka", Path: "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html", PostID: 646,
		TranslatorID: 238, Ads: true, Season: 1, Episode: 2, Quality: "720p"}
	want := NewTestFS(
		[]TestMovie{{Title: "Interstellar", Year: 2014, Assignment: &library.TorrentAssignment{
			Kind: library.AssignmentKindHDRezka, Stream: movie, FilePath: movie.FilePath(), FileSize: 3322740623}}},
		[]TestEpisode{{Show: "Breaking Bad", Year: 2008, Season: 1, Episode: 2, Name: "Cat's in the Bag...",
			Assignment: &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka, Stream: episode,
				FilePath: episode.FilePath(), FileSize: 1145043718}}},
	).tree

	dir := t.TempDir()
	(&LibraryFS{cacheDir: dir, tree: want}).saveTreeToCache()
	loader := &LibraryFS{cacheDir: dir}
	if err := loader.loadTreeFromCache(); err != nil {
		t.Fatalf("loadTreeFromCache: %v", err)
	}

	files := 0
	for path, w := range want.pathMap {
		w, ok := w.(*PlaceholderFile)
		if !ok {
			continue
		}
		files++
		g, ok := loader.tree.pathMap[path].(*PlaceholderFile)
		if !ok {
			t.Fatalf("%s missing after reload", path)
		}
		a := g.assignment
		if a.Kind != library.AssignmentKindHDRezka || a.IsTorrent() || a.Stream == nil || *a.Stream != *w.assignment.Stream ||
			a.FilePath != w.assignment.FilePath || a.FileSize != w.assignment.FileSize || g.size != w.size {
			t.Errorf("%s = %+v (stream %+v), want %+v", path, a, a.Stream, w.assignment)
		}
		f, err := loader.Open(path)
		if err != nil {
			t.Fatalf("Open(%s): %v", path, err)
		}
		if _, ok := f.(*RemoteFile); !ok {
			t.Errorf("Open(%s) = %T, want a RemoteFile", path, f)
		}
	}
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
}

// An older cache (v1 has no subtitles, v2 no grouped multi-episode files,
// v4 no sources) is ignored, so the tree is rebuilt from the database.
func TestCache_IgnoresOlderVersions(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		dir := t.TempDir()
		var buf bytes.Buffer
		old := treeCache{Version: version, Movies: []cachedMovie{{FolderName: "Old (2000)", FileName: "Old (2000).mkv"}}}
		if err := gob.NewEncoder(&buf).Encode(old); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, cacheFile), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}

		fs := &LibraryFS{cacheDir: dir}
		if err := fs.loadTreeFromCache(); err == nil {
			t.Fatalf("a v%d cache loaded", version)
		}
		if fs.tree != nil {
			t.Errorf("tree set from a v%d cache", version)
		}
	}
}
