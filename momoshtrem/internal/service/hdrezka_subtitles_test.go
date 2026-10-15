package service

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/subtitle/subtitletest"
)

// withSubtitles gives the env's service a subtitle store over an in-memory
// repository, keeping files under the returned directory.
func (e *hdrezkaEnv) withSubtitles(t *testing.T) (*subtitletest.Repo, string) {
	t.Helper()
	repo := subtitletest.NewRepo()
	dir := t.TempDir()
	e.svc.subtitles = subtitle.NewService(nil, repo, dir)
	return repo, dir
}

// subtitleRows lists the stored subtitles as "source item_id lang name
// release", sorted.
func subtitleRows(repo *subtitletest.Repo) []string {
	var out []string
	for _, row := range repo.All() {
		out = append(out, strings.Join([]string{
			string(row.Source), strconv.FormatInt(row.ItemID, 10), row.LanguageCode, row.LanguageName, row.ReleaseName}, " "))
	}
	sort.Strings(out)
	return out
}

// A movie's assign downloads its stream's tracks and stores them as SRT
// before the one tree rebuild; a track that fails doesn't fail the assign.
func TestHDRezkaAssign_RecordsSubtitles(t *testing.T) {
	e := newHDRezkaEnv()
	repo, dir := e.withSubtitles(t)
	e.streams.subtitles = func(hdrezka.StreamKey) []hdrezka.Subtitle {
		return []hdrezka.Subtitle{
			{Language: "English", Code: "en", URL: "https://cdn.example/sub/en.vtt"},
			{Language: "Русский", URL: "https://cdn.example/sub/ru.vtt"}, // no code: the name tells
			{Language: "Українська", Code: "uk", URL: "https://cdn.example/sub/uk.vtt"},
		}
	}
	e.site.subtitle = func(_ context.Context, link string) ([]byte, error) {
		if strings.HasSuffix(link, "/uk.vtt") {
			return nil, hdrezka.ErrLinkExpired
		}
		return []byte("WEBVTT\r\n\r\n1\r\n00:00:01.000 --> 00:00:02.500 align:start\r\n<i>" + path.Base(link) + "</i>\r\n"), nil
	}

	got, err := e.svc.AssignMovie(context.Background(), 7, HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"})
	if err != nil || got.Quality != "1080p" {
		t.Fatalf("AssignMovie = %+v, %v", got, err)
	}
	const label = "HDRezka · Оригинал (+субтитры) · 1080p"
	want := []string{"hdrezka 7 en English " + label, "hdrezka 7 ru Russian " + label}
	if rows := subtitleRows(repo); !reflect.DeepEqual(rows, want) {
		t.Errorf("subtitles = %v, want %v", rows, want)
	}
	srt, err := os.ReadFile(filepath.Join(dir, "hdrezka", "movie", "7", "en.srt"))
	if err != nil || string(srt) != "1\n00:00:01,000 --> 00:00:02,500\n<i>en.vtt</i>\n\n" {
		t.Errorf("en.srt = %q, %v", srt, err)
	}
	if n := e.tree.count(); n != 1 {
		t.Errorf("rebuilds = %d, want 1, with the subtitles in", n)
	}
	if n := e.site.called("subtitle:"); n != 3 {
		t.Errorf("downloads = %d, want 3", n)
	}

	// The same stream again refreshes the subtitles in place.
	before := repo.All()
	if _, err := e.svc.AssignMovie(context.Background(), 7, HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"}); err != nil {
		t.Fatal(err)
	}
	after := repo.All()
	if len(after) != 2 || after[0].ID != before[0].ID || after[1].ID != before[1].ID {
		t.Errorf("subtitles after assigning again = %+v, want the same two rows", after)
	}
}

// A show's assign stores each assigned episode's tracks under its own
// directory; an episode that failed gets none.
func TestHDRezkaShowAssign_RecordsSubtitlesPerEpisode(t *testing.T) {
	e := newHDRezkaEnv()
	repo, dir := e.withSubtitles(t)
	e.streams.errs = map[[2]int]error{{1, 2}: hdrezka.ErrPremium}

	res, err := e.svc.AssignShow(context.Background(), 5, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "720p"},
		Items: []HDRezkaAssignItem{
			{EpisodeID: 501, Season: 1, Episode: 1},
			{EpisodeID: 502, Season: 1, Episode: 2},
			{EpisodeID: 503, Season: 1, Episode: 3},
		},
	})
	if err != nil || len(res.Assigned) != 2 || len(res.Failed) != 1 {
		t.Fatalf("AssignShow = %+v, %v", res, err)
	}
	// testStream offers English and a nameless "Ukrainian?" track.
	const label = "HDRezka · Оригинал (+субтитры) · 720p"
	want := []string{
		"hdrezka 501 en English " + label, "hdrezka 501 uk Ukrainian " + label,
		"hdrezka 503 en English " + label, "hdrezka 503 uk Ukrainian " + label,
	}
	if rows := subtitleRows(repo); !reflect.DeepEqual(rows, want) {
		t.Errorf("subtitles = %v, want %v", rows, want)
	}
	for _, id := range []string{"501", "503"} {
		if _, err := os.Stat(filepath.Join(dir, "hdrezka", "episode", id, "uk.srt")); err != nil {
			t.Errorf("episode %s: %v", id, err)
		}
	}
	if n := e.tree.count(); n != 1 {
		t.Errorf("rebuilds = %d, want 1", n)
	}
}

// Subtitles that aren't in within the assign's budget don't hold up its
// answer: the tree is rebuilt at once, and again when they're in.
func TestHDRezkaAssign_SubtitlesAfterTheBudget(t *testing.T) {
	e := newHDRezkaEnv()
	repo, _ := e.withSubtitles(t)
	e.svc.assignBudget = 200 * time.Millisecond
	release := make(chan struct{})
	e.site.subtitle = func(_ context.Context, link string) ([]byte, error) {
		<-release
		return []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nLate.\n"), nil
	}

	start := time.Now()
	if _, err := e.svc.AssignMovie(context.Background(), 7, HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the assign took %v, waiting for subtitles", took)
	}
	if n, rows := e.tree.count(), repo.All(); n != 1 || len(rows) != 0 {
		t.Fatalf("after the answer: %d rebuilds, %d subtitles; want 1 and none yet", n, len(rows))
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for e.tree.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n, rows := e.tree.count(), repo.All(); n != 2 || len(rows) != 2 {
		t.Errorf("once they're in: %d rebuilds, %d subtitles; want 2 and 2", n, len(rows))
	}
}

// Without a subtitle store nothing is downloaded, and the tree is rebuilt
// once.
func TestHDRezkaAssign_WithoutSubtitleStore(t *testing.T) {
	e := newHDRezkaEnv()
	if _, err := e.svc.AssignMovie(context.Background(), 7, HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"}); err != nil {
		t.Fatal(err)
	}
	if n := e.site.called("subtitle:"); n != 0 {
		t.Errorf("downloads = %d, want none", n)
	}
	if n := e.tree.count(); n != 1 {
		t.Errorf("rebuilds = %d, want 1", n)
	}
}
