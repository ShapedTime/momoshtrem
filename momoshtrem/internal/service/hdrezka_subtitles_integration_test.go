package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// HDRezka's tracks as its CDN serves them: the English one with a BOM and
// CRLF, the others numbered with LF.
const (
	cdnEnglish = "\xef\xbb\xbfWEBVTT\r\n\r\n00:01:14.908 --> 00:01:16.988\r\nMy father grew corn.\r\n\r\n"
	cdnRussian = "WEBVTT\n\n1\n00:01:15.122 --> 00:01:17.322\n<i>Мой отец выращивал кукурузу.</i>\n"
)

// fakeCDN serves subtitle tracks by path and counts the requests; a path
// it doesn't know answers 500.
type fakeCDN struct {
	*httptest.Server
	mu    sync.Mutex
	files map[string]string
	hits  map[string]int
}

func newFakeCDN(t *testing.T, files map[string]string) *fakeCDN {
	t.Helper()
	c := &fakeCDN{files: files, hits: map[string]int{}}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.hits[r.URL.Path]++
		body, ok := c.files[r.URL.Path]
		c.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *fakeCDN) hit(path string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits[path]
}

// dirNames lists a WebDAV folder, sorted.
func dirNames(t *testing.T, fs *vfs.LibraryFS, dir string) []string {
	t.Helper()
	entries, err := fs.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var names []string
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// An assign records its stream's subtitles end to end: the real CDN client
// downloads them (one track fails without failing the assign), they're
// stored as SRT and rows in Postgres, and the WebDAV tree lists them next
// to the .mp4. The same stream again refreshes them; another translator
// replaces them, its files too.
func TestIntegration_HDRezkaSubtitles(t *testing.T) {
	env := newJobEnv(t)
	movieID := env.movie("Interstellar", 2014)
	showID, eps := env.show("Breaking Bad", [][3]any{{1, 1, "Pilot"}})

	cdn := newFakeCDN(t, map[string]string{
		"/s/t238/en.vtt": cdnEnglish,
		"/s/t238/ru.vtt": cdnRussian,
		"/s/t56/en.vtt":  cdnEnglish,
		// t238's uk.vtt answers 500.
	})
	fakes := newHDRezkaEnv()
	fakes.streams.subtitles = func(key hdrezka.StreamKey) []hdrezka.Subtitle {
		base := fmt.Sprintf("%s/s/t%d/", cdn.URL, key.TranslatorID)
		if key.TranslatorID == 56 {
			return []hdrezka.Subtitle{{Language: "English", Code: "en", URL: base + "en.vtt"}}
		}
		return []hdrezka.Subtitle{
			{Language: "Русский", Code: "ru", URL: base + "ru.vtt"},
			{Language: "Українська", Code: "uk", URL: base + "uk.vtt"},
			{Language: "English", Code: "en", URL: base + "en.vtt"},
		}
	}
	fakes.site.subtitle = hdrezka.NewClient(hdrezka.WithRetries(1, 0)).Subtitle

	movies, shows, assignments := library.NewMovieRepository(env.db), library.NewShowRepository(env.db), library.NewAssignmentRepository(env.db)
	repo := subtitle.NewRepository(env.db.DB)
	root := t.TempDir()
	fs := vfs.NewLibraryFS(movies, shows, assignments, 0)
	fs.SetSubtitleRepository(repo)
	svc := NewHDRezkaService(HDRezkaDeps{
		Site: fakes.site, Resolver: fakes.streams, TMDB: fakes.tmdb, Movies: movies, Shows: shows,
		Assignments: assignments, Tree: fs, Subtitles: subtitle.NewService(nil, repo, root),
	})
	assign := func(translator int) {
		t.Helper()
		if _, err := svc.AssignMovie(env.ctx, movieID, HDRezkaAssign{URL: interstellarPath, TranslatorID: translator, Quality: "1080p"}); err != nil {
			t.Fatalf("assign translator %d: %v", translator, err)
		}
	}

	assign(238)
	if got := subtitlesOf(t, repo, subtitle.ItemTypeMovie, movieID); got != "hdrezka:en.srt,hdrezka:ru.srt" {
		t.Fatalf("subtitles = %s", got)
	}
	if cdn.hit("/s/t238/uk.vtt") != 1 {
		t.Errorf("uk.vtt asked %d times, want once", cdn.hit("/s/t238/uk.vtt"))
	}
	const folder = "/Movies/Interstellar (2014)"
	want := []string{"Interstellar (2014).en.srt", "Interstellar (2014).mp4", "Interstellar (2014).ru.srt"}
	if got := dirNames(t, fs, folder); !reflect.DeepEqual(got, want) {
		t.Errorf("WebDAV folder = %v, want %v", got, want)
	}
	f, err := fs.Open(folder + "/Interstellar (2014).ru.srt")
	if err != nil {
		t.Fatal(err)
	}
	srt, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(srt) != "1\n00:01:15,122 --> 00:01:17,322\n<i>Мой отец выращивал кукурузу.</i>\n\n" {
		t.Errorf("ru.srt over WebDAV = %q, %v", srt, err)
	}
	before, _ := repo.GetByItem(env.ctx, subtitle.ItemTypeMovie, movieID)

	// The same stream again: refreshed, not duplicated.
	assign(238)
	after, _ := repo.GetByItem(env.ctx, subtitle.ItemTypeMovie, movieID)
	if len(after) != 2 || after[0].ID != before[0].ID || after[1].ID != before[1].ID ||
		after[0].ReleaseName != "HDRezka · Оригинал (+субтитры) · 1080p" {
		t.Errorf("after assigning again = %+v, want the same rows", after)
	}

	// Another translator: its English replaces both, and ru.srt goes.
	assign(56)
	after, _ = repo.GetByItem(env.ctx, subtitle.ItemTypeMovie, movieID)
	if len(after) != 1 || after[0].LanguageCode != "en" || after[0].ReleaseName != "HDRezka · Дубляж · 1080p" {
		t.Errorf("after another translator = %+v", after)
	}
	if got := dirNames(t, fs, folder); !reflect.DeepEqual(got, want[:2]) {
		t.Errorf("WebDAV folder = %v, want %v", got, want[:2])
	}
	if _, err := os.Stat(filepath.Join(root, "hdrezka", "movie", fmt.Sprint(movieID), "ru.srt")); !os.IsNotExist(err) {
		t.Errorf("ru.srt is still on disk (err %v)", err)
	}

	// An episode's go next to it in its season folder.
	if _, err := svc.AssignShow(env.ctx, showID, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "720p"},
		Items:         []HDRezkaAssignItem{{EpisodeID: eps["S01E01"], Season: 1, Episode: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	season := "/TV Shows/Breaking Bad (2020)/Season 01"
	want = []string{"Breaking Bad - S01E01 - Pilot.en.srt", "Breaking Bad - S01E01 - Pilot.mp4", "Breaking Bad - S01E01 - Pilot.ru.srt"}
	if got := dirNames(t, fs, season); !reflect.DeepEqual(got, want) {
		t.Errorf("season folder = %v, want %v", got, want)
	}
}
