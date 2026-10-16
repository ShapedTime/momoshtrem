package subtitle_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/subtitle/subtitletest"
)

// cleanupFixture is an episode and a movie with subtitles from every
// source, files on disk, and a stray file no row names.
type cleanupFixture struct {
	dir       string
	repo      *subtitletest.Repo
	svc       *subtitle.Service
	episode   subtitle.Item
	movie     subtitle.Item
	files     map[string]string // "episode/opensubtitles" → path
	strayFile string
}

func newCleanupFixture(t *testing.T) *cleanupFixture {
	t.Helper()
	f := &cleanupFixture{
		dir:     t.TempDir(),
		repo:    subtitletest.NewRepo(),
		episode: subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: 11},
		movie:   subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 22},
		files:   map[string]string{},
	}
	f.svc = subtitle.NewService(nil, f.repo, f.dir)
	for _, item := range []subtitle.Item{f.episode, f.movie} {
		id := strconv.FormatInt(item.ID, 10)
		for _, src := range []subtitle.Source{subtitle.SourceOpenSubtitles, subtitle.SourceHDRezka, subtitle.SourceTorrent} {
			path := "Pack/Subs/en.srt"
			switch src {
			case subtitle.SourceOpenSubtitles:
				path = filepath.Join(f.dir, string(item.Type), id, "en.srt")
			case subtitle.SourceHDRezka:
				path = filepath.Join(f.dir, "hdrezka", string(item.Type), id, "en.srt")
			}
			if src != subtitle.SourceTorrent {
				write(t, path)
			}
			hash := ""
			if src == subtitle.SourceTorrent {
				hash = hashA
			}
			f.repo.Add(subtitle.Subtitle{ItemType: item.Type, ItemID: item.ID, LanguageCode: "en", Format: "srt",
				FilePath: path, Source: src, InfoHash: hash})
			f.files[string(item.Type)+"/"+string(src)] = path
		}
	}
	f.strayFile = filepath.Join(f.dir, "episode", "11", "old.srt")
	write(t, f.strayFile)
	return f
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func onDisk(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sourcesOf lists the item's remaining subtitles' sources, sorted.
func (f *cleanupFixture) sourcesOf(item subtitle.Item) []string {
	var out []string
	for _, s := range f.repo.All() {
		if s.Item() == item {
			out = append(out, string(s.Source))
		}
	}
	slices.Sort(out)
	return out
}

func TestService_DeleteForItems(t *testing.T) {
	f := newCleanupFixture(t)

	n, err := f.svc.DeleteForItems(context.Background(), []subtitle.Item{f.episode})
	if err != nil || n != 3 {
		t.Fatalf("DeleteForItems = %d, %v; want the episode's 3", n, err)
	}
	if got := f.sourcesOf(f.episode); len(got) != 0 {
		t.Errorf("episode subtitles left: %v", got)
	}
	for _, path := range []string{
		f.files["episode/opensubtitles"], f.files["episode/hdrezka"], f.strayFile,
		filepath.Join(f.dir, "episode", "11"), filepath.Join(f.dir, "hdrezka", "episode", "11"),
	} {
		if onDisk(path) {
			t.Errorf("%s is still on disk", path)
		}
	}
	// The movie is untouched.
	if got := f.sourcesOf(f.movie); !slices.Equal(got, []string{"hdrezka", "opensubtitles", "torrent"}) {
		t.Errorf("movie subtitles = %v", got)
	}
	if !onDisk(f.files["movie/opensubtitles"]) || !onDisk(f.files["movie/hdrezka"]) {
		t.Error("the movie's files were removed")
	}
}

func TestService_DeleteFileSubtitles(t *testing.T) {
	f := newCleanupFixture(t)

	n, err := f.svc.DeleteFileSubtitles(context.Background(), []subtitle.Item{f.movie})
	if err != nil || n != 2 {
		t.Fatalf("DeleteFileSubtitles = %d, %v; want the torrent and HDRezka ones", n, err)
	}
	if got := f.sourcesOf(f.movie); !slices.Equal(got, []string{"opensubtitles"}) {
		t.Errorf("movie subtitles = %v, want the downloaded one only", got)
	}
	if onDisk(f.files["movie/hdrezka"]) || onDisk(filepath.Dir(f.files["movie/hdrezka"])) {
		t.Error("the HDRezka file (or its empty folder) is still on disk")
	}
	if !onDisk(f.files["movie/opensubtitles"]) {
		t.Error("the downloaded file was removed")
	}
	if got := f.sourcesOf(f.episode); len(got) != 3 {
		t.Errorf("episode subtitles = %v, want all 3 kept", got)
	}
}

// Without a download path no folder is named, so nothing outside the
// subtitles can be removed.
func TestService_DeleteForItemsWithoutDownloadPath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	write(t, filepath.Join(cwd, "movie", "22", "keep.srt"))
	svc := subtitle.NewService(nil, subtitletest.NewRepo(), "")

	if _, err := svc.DeleteForItems(context.Background(), []subtitle.Item{{Type: subtitle.ItemTypeMovie, ID: 22}}); err != nil {
		t.Fatal(err)
	}
	if !onDisk(filepath.Join(cwd, "movie", "22", "keep.srt")) {
		t.Error("a folder relative to the working directory was removed")
	}
}

func TestRepository_DeleteByItems(t *testing.T) {
	repo, _, id := openRepo(t)
	ctx := context.Background()
	ep := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id}
	other := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: id + 1}
	cleanupItems(t, repo, ep, other)

	for _, item := range []subtitle.Item{ep, other} {
		if err := repo.UpsertDownloaded(ctx, downloaded(item, "en", "/subs/en.srt", "")); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpsertTorrent(ctx, fromTorrent(item, "en", hashA, "Pack/en.srt")); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := repo.DeleteByItems(ctx, []subtitle.Item{ep}, subtitle.SourceTorrent, subtitle.SourceHDRezka)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Source != subtitle.SourceTorrent || deleted[0].FilePath != "Pack/en.srt" || deleted[0].Item() != ep {
		t.Errorf("deleted = %+v, want the episode's torrent subtitle", deleted)
	}

	deleted, err = repo.DeleteByItems(ctx, []subtitle.Item{ep, other})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 3 {
		t.Errorf("deleted %d, want every source of both items (3 left)", len(deleted))
	}
	for _, item := range []subtitle.Item{ep, other} {
		if subs, _ := repo.GetByItem(ctx, item.Type, item.ID); len(subs) != 0 {
			t.Errorf("%v: %d subtitles left", item, len(subs))
		}
	}
	if deleted, err := repo.DeleteByItems(ctx, nil); err != nil || deleted != nil {
		t.Errorf("no items = %v, %v", deleted, err)
	}
}
