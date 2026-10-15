package subtitle_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/subtitle/subtitletest"
)

const (
	interstellarFile    = "hdrezka/2259/t238/movie/1080p.mp4"
	interstellarRelease = "HDRezka · Оригинал (+субтитры) · 1080p"
)

var interstellar = subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 7}

func vttCue(text string) []byte {
	return []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n" + text + "\n")
}

func srtCue(text string) string {
	return "1\n00:00:01,000 --> 00:00:02,000\n" + text + "\n\n"
}

// filesIn lists a directory's file names, sorted.
func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// HDRezka tracks become SRT files in the item's own directory, one per
// language; saving the stream again refreshes them in place.
func TestService_SaveHDRezka(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := subtitletest.NewRepo()
	repo.Streams = map[subtitle.Item]string{interstellar: interstellarFile}
	svc := subtitle.NewService(nil, repo, root)
	stream := subtitle.HDRezkaStream{File: interstellarFile, Release: interstellarRelease}

	saved, err := svc.SaveHDRezka(ctx, interstellar, stream, []subtitle.HDRezkaTrack{
		{Code: "ru", Name: "Русский", VTT: vttCue("<i>Привет.</i>")},
		{Code: "en", Name: "English", VTT: vttCue("Hello.")},
		{Code: "", Name: "English", VTT: vttCue("Hello again.")}, // en is taken
		{Code: "uk", Name: "Українська", VTT: []byte("<html>Forbidden</html>")},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "hdrezka", "movie", "7")
	if len(saved) != 2 || !reflect.DeepEqual(filesIn(t, dir), []string{"en.srt", "ru.srt"}) {
		t.Fatalf("saved %d, files %v", len(saved), filesIn(t, dir))
	}
	rows := repo.All()
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	for _, row := range rows {
		want := map[string]string{"en": "English", "ru": "Russian"}[row.LanguageCode]
		if row.Source != subtitle.SourceHDRezka || row.LanguageName != want || row.Format != "srt" ||
			row.ReleaseName != interstellarRelease || row.FilePath != filepath.Join(dir, row.LanguageCode+".srt") || row.InfoHash != "" {
			t.Errorf("row = %+v", row)
		}
		content, _ := os.ReadFile(row.FilePath)
		if row.FileSize != int64(len(content)) {
			t.Errorf("%s: size %d, file %d bytes", row.LanguageCode, row.FileSize, len(content))
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "ru.srt")); string(got) != srtCue("<i>Привет.</i>") {
		t.Errorf("ru.srt = %q", got)
	}

	// The same stream again: the rows are refreshed, not duplicated.
	ids := map[string]int64{rows[0].LanguageCode: rows[0].ID, rows[1].LanguageCode: rows[1].ID}
	if _, err := svc.SaveHDRezka(ctx, interstellar, stream, []subtitle.HDRezkaTrack{
		{Code: "en", Name: "English", VTT: vttCue("Hello, fresh.")},
	}); err != nil {
		t.Fatal(err)
	}
	rows = repo.All()
	if len(rows) != 2 || ids[rows[0].LanguageCode] != rows[0].ID || ids[rows[1].LanguageCode] != rows[1].ID {
		t.Errorf("rows after a refresh = %+v, want the same two", rows)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "en.srt")); string(got) != srtCue("Hello, fresh.") {
		t.Errorf("en.srt = %q", got)
	}

	// Nothing usable: nothing stored, no error.
	if saved, err := svc.SaveHDRezka(ctx, interstellar, stream, []subtitle.HDRezkaTrack{{Code: "de", VTT: []byte("nope")}}); err != nil || saved != nil {
		t.Errorf("SaveHDRezka = %v, %v", saved, err)
	}
}

// Another stream on the item: nothing is stored or left on disk.
func TestService_SaveHDRezkaStaleStream(t *testing.T) {
	root := t.TempDir()
	repo := subtitletest.NewRepo()
	repo.Streams = map[subtitle.Item]string{interstellar: "hdrezka/2259/t56/movie/1080p.mp4"}
	svc := subtitle.NewService(nil, repo, root)

	_, err := svc.SaveHDRezka(context.Background(), interstellar, subtitle.HDRezkaStream{File: interstellarFile},
		[]subtitle.HDRezkaTrack{{Code: "en", VTT: vttCue("Hi.")}})
	if !errors.Is(err, subtitle.ErrStaleStream) {
		t.Fatalf("err = %v, want ErrStaleStream", err)
	}
	if rows := repo.All(); len(rows) != 0 {
		t.Errorf("rows = %+v", rows)
	}
	if files := filesIn(t, filepath.Join(root, "hdrezka", "movie", "7")); len(files) != 0 {
		t.Errorf("files left behind: %v", files)
	}
}

// Another translator's subtitles replace the first's: languages it hasn't
// lose their files too (their rows went when the file changed).
func TestService_SaveHDRezkaRemovesStaleFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := subtitletest.NewRepo()
	svc := subtitle.NewService(nil, repo, root)
	dir := filepath.Join(root, "hdrezka", "movie", "7")

	if _, err := svc.SaveHDRezka(ctx, interstellar, subtitle.HDRezkaStream{File: "a.mp4"}, []subtitle.HDRezkaTrack{
		{Code: "en", VTT: vttCue("A")}, {Code: "ru", VTT: vttCue("A")}, {Code: "uk", VTT: vttCue("A")},
	}); err != nil {
		t.Fatal(err)
	}
	// What the assignment writer does when the item's file changes.
	if err := repo.DeleteByItem(ctx, interstellar.Type, interstellar.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveHDRezka(ctx, interstellar, subtitle.HDRezkaStream{File: "b.mp4"}, []subtitle.HDRezkaTrack{
		{Code: "en", VTT: vttCue("B")},
	}); err != nil {
		t.Fatal(err)
	}
	if got := filesIn(t, dir); !reflect.DeepEqual(got, []string{"en.srt"}) {
		t.Errorf("files = %v, want en.srt only", got)
	}
}

// HDRezka subtitles are local files: deleting one removes its file.
func TestService_DeleteHDRezkaSubtitle(t *testing.T) {
	ctx := context.Background()
	repo := subtitletest.NewRepo()
	svc := subtitle.NewService(nil, repo, t.TempDir())
	saved, err := svc.SaveHDRezka(ctx, interstellar, subtitle.HDRezkaStream{File: interstellarFile}, []subtitle.HDRezkaTrack{{Code: "en", VTT: vttCue("Hi.")}})
	if err != nil || len(saved) != 1 {
		t.Fatalf("SaveHDRezka = %v, %v", saved, err)
	}
	streamed := subtitle.ItemContext{Item: interstellar, File: subtitle.HDRezkaFile()}

	torrent := subtitle.ItemContext{Item: interstellar, File: subtitle.TorrentFile(hashA)}
	if err := svc.DeleteFromItem(ctx, torrent, saved[0].ID); !errors.Is(err, subtitle.ErrNotFound) {
		t.Errorf("with a torrent assigned: err = %v, want ErrNotFound", err)
	}
	list, err := svc.List(ctx, streamed)
	if err != nil || len(list) != 1 || list[0].Match != subtitle.MatchExactRelease {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if err := svc.DeleteFromItem(ctx, streamed, saved[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(saved[0].FilePath); !os.IsNotExist(err) {
		t.Errorf("file still on disk (err %v)", err)
	}
	if rows := repo.All(); len(rows) != 0 {
		t.Errorf("rows = %+v", rows)
	}
}
