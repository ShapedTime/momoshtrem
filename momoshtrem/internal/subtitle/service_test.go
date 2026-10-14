package subtitle_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/subtitle/subtitletest"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	flux  = "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX"
)

var dune = subtitle.Item{Type: subtitle.ItemTypeMovie, ID: 1}

func duneContext(release, hash string) subtitle.ItemContext {
	return subtitle.ItemContext{
		Item:     dune,
		Search:   opensubtitles.SearchParams{Type: "movie", TMDBID: 693134},
		InfoHash: hash,
		Release:  release,
	}
}

func TestCurrent(t *testing.T) {
	downloaded := &subtitle.Subtitle{ID: 1, Source: subtitle.SourceOpenSubtitles}
	fromA := &subtitle.Subtitle{ID: 2, Source: subtitle.SourceTorrent, InfoHash: hashA}
	fromB := &subtitle.Subtitle{ID: 3, Source: subtitle.SourceTorrent, InfoHash: hashB}
	all := []*subtitle.Subtitle{downloaded, fromA, fromB}

	tests := []struct {
		name string
		hash string
		want []int64
	}{
		{"assigned torrent A", hashA, []int64{1, 2}},
		{"reassigned to B", hashB, []int64{1, 3}},
		{"nothing assigned", "", []int64{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int64
			for _, s := range subtitle.Current(all, tt.hash) {
				got = append(got, s.ID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Current = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseLanguages(t *testing.T) {
	tests := []struct {
		raw     string
		want    []string
		wantErr bool
	}{
		{"", []string{"en"}, false},
		{"en", []string{"en"}, false},
		{"ru, EN,az,en,", []string{"az", "en", "ru"}, false},
		{"pt-BR,zh-cn", []string{"pt-br", "zh-cn"}, false},
		{"en,../etc", nil, true},
		{"english", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := subtitle.ParseLanguages(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, subtitle.ErrInvalidLanguage) {
					t.Fatalf("err = %v, want ErrInvalidLanguage", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLanguages = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestService_List(t *testing.T) {
	repo := subtitletest.NewRepo()
	repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", Source: subtitle.SourceTorrent, InfoHash: hashA, FilePath: "Dune/en.srt"})
	repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", Source: subtitle.SourceOpenSubtitles, ReleaseName: "Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS"})
	repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "ru", Source: subtitle.SourceTorrent, InfoHash: hashB, FilePath: "Old/ru.srt"})
	repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 2, LanguageCode: "en", Source: subtitle.SourceOpenSubtitles})
	svc := subtitle.NewService(nil, repo, t.TempDir())

	t.Run("assigned", func(t *testing.T) {
		got, err := svc.List(context.Background(), duneContext(flux, hashA))
		if err != nil {
			t.Fatal(err)
		}
		type row struct {
			lang    string
			source  subtitle.Source
			release string
			match   subtitle.Match
		}
		var rows []row
		for _, s := range got {
			rows = append(rows, row{s.LanguageCode, s.Source, s.ReleaseName, s.Match})
		}
		want := []row{
			// Downloaded first; its match is against the current release.
			{"en", subtitle.SourceOpenSubtitles, "Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS", subtitle.MatchDifferentTiming},
			// The torrent's own, named after the release it came with.
			{"en", subtitle.SourceTorrent, flux, subtitle.MatchExactRelease},
			// ru from torrent B is stale: left out.
		}
		if !reflect.DeepEqual(rows, want) {
			t.Errorf("List =\n%v\nwant\n%v", rows, want)
		}
	})

	t.Run("nothing assigned", func(t *testing.T) {
		got, err := svc.List(context.Background(), duneContext("", ""))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Source != subtitle.SourceOpenSubtitles || got[0].Match != subtitle.MatchUnknown {
			t.Errorf("List = %+v, want only the downloaded one, unknown", got)
		}
	})
}

func TestService_Download(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repo := subtitletest.NewRepo()
	torrent := repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", Source: subtitle.SourceTorrent, InfoHash: hashA, FilePath: "Dune/en.srt"})
	fetcher := &subtitletest.Fetcher{Files: map[int]subtitletest.File{
		11: {Name: "Dune.Part.Two.FLUX.en.srt", Content: []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n")},
		12: {Name: "Dune.Part.Two.NTb.en.ass", Content: []byte("[Script Info]\n")},
	}}
	svc := subtitle.NewService(fetcher, repo, dir)

	first, err := svc.Download(ctx, duneContext(flux, hashA), 11, "EN", flux)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	srt := filepath.Join(dir, "movie", "1", "en.srt")
	if first.FilePath != srt || first.Format != "srt" || first.LanguageCode != "en" || first.LanguageName != "English" {
		t.Errorf("first = %+v", first.Subtitle)
	}
	if first.Source != subtitle.SourceOpenSubtitles || first.ReleaseName != flux || first.Match != subtitle.MatchExactRelease {
		t.Errorf("first = source %s release %q match %s", first.Source, first.ReleaseName, first.Match)
	}
	if _, err := os.Stat(srt); err != nil {
		t.Fatalf("stored file: %v", err)
	}

	// The same language again, in another format: it replaces the first,
	// file included; the torrent's English subtitle stays.
	second, err := svc.Download(ctx, duneContext(flux, hashA), 12, "en", "")
	if err != nil {
		t.Fatalf("second Download: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("second ID = %d, want the first's (%d): a replacement", second.ID, first.ID)
	}
	if second.Format != "ass" || second.ReleaseName != "" || second.Match != subtitle.MatchUnknown {
		t.Errorf("second = format %s release %q match %s", second.Format, second.ReleaseName, second.Match)
	}
	if _, err := os.Stat(srt); !os.IsNotExist(err) {
		t.Errorf("replaced en.srt still on disk (err %v)", err)
	}
	rows := repo.All()
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the torrent subtitle and one download", rows)
	}
	if got, _ := repo.GetByID(ctx, torrent.ID); got == nil {
		t.Error("torrent subtitle was replaced by the download")
	}

	t.Run("nothing assigned still downloads", func(t *testing.T) {
		got, err := svc.Download(ctx, duneContext("", ""), 11, "ru", "Dune.Part.Two.FLUX")
		if err != nil {
			t.Fatal(err)
		}
		if got.Match != subtitle.MatchUnknown {
			t.Errorf("match = %s, want unknown", got.Match)
		}
	})

	t.Run("language codes name files", func(t *testing.T) {
		_, err := svc.Download(ctx, duneContext(flux, hashA), 11, "../../etc/passwd", "")
		if !errors.Is(err, subtitle.ErrInvalidLanguage) {
			t.Errorf("err = %v, want ErrInvalidLanguage", err)
		}
	})

	t.Run("OpenSubtitles failure", func(t *testing.T) {
		_, err := svc.Download(ctx, duneContext(flux, hashA), 99, "de", "")
		if !errors.Is(err, subtitle.ErrFetch) {
			t.Errorf("err = %v, want ErrFetch", err)
		}
	})
}

func TestService_DeleteFromItem(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	local := filepath.Join(dir, "en.srt")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := subtitletest.NewRepo()
	downloaded := repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", Source: subtitle.SourceOpenSubtitles, FilePath: local})
	torrent := repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", Source: subtitle.SourceTorrent, InfoHash: hashA, FilePath: "Dune/en.srt"})
	stale := repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "ru", Source: subtitle.SourceTorrent, InfoHash: hashB, FilePath: "Old/ru.srt"})
	svc := subtitle.NewService(nil, repo, dir)
	assigned := duneContext(flux, hashA)

	if err := svc.DeleteFromItem(ctx, assigned, torrent.ID); !errors.Is(err, subtitle.ErrTorrentSubtitle) {
		t.Errorf("torrent subtitle: err = %v, want ErrTorrentSubtitle", err)
	}
	// A subtitle the list hides isn't found either.
	if err := svc.DeleteFromItem(ctx, assigned, stale.ID); !errors.Is(err, subtitle.ErrNotFound) {
		t.Errorf("an earlier torrent's subtitle: err = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteFromItem(ctx, duneContext("", ""), torrent.ID); !errors.Is(err, subtitle.ErrNotFound) {
		t.Errorf("torrent subtitle with nothing assigned: err = %v, want ErrNotFound", err)
	}
	other := subtitle.ItemContext{Item: subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: 1}}
	if err := svc.DeleteFromItem(ctx, other, downloaded.ID); !errors.Is(err, subtitle.ErrNotFound) {
		t.Errorf("another item's subtitle: err = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteFromItem(ctx, assigned, 999); !errors.Is(err, subtitle.ErrNotFound) {
		t.Errorf("missing subtitle: err = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteFromItem(ctx, assigned, downloaded.ID); err != nil {
		t.Fatalf("delete downloaded: %v", err)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Errorf("file still on disk (err %v)", err)
	}
	if rows := repo.All(); len(rows) != 2 || rows[0].ID != torrent.ID || rows[1].ID != stale.ID {
		t.Errorf("rows = %+v, want the two torrent subtitles", rows)
	}
}

// failingUpsert is a repository whose downloaded upserts fail.
type failingUpsert struct{ *subtitletest.Repo }

func (failingUpsert) UpsertDownloaded(context.Context, *subtitle.Subtitle) error {
	return errors.New("database is down")
}

// A failed upsert leaves the stored subtitle file as it was, and no temp file.
func TestService_DownloadKeepsFileWhenTheRecordFails(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	movieDir := filepath.Join(dir, "movie", "1")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(movieDir, "en.srt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := failingUpsert{subtitletest.NewRepo()}
	repo.Add(subtitle.Subtitle{ItemType: "movie", ItemID: 1, LanguageCode: "en", Source: subtitle.SourceOpenSubtitles, FilePath: existing})
	fetcher := &subtitletest.Fetcher{Files: map[int]subtitletest.File{11: {Name: "new.en.srt", Content: []byte("new")}}}
	svc := subtitle.NewService(fetcher, repo, dir)

	if _, err := svc.Download(ctx, duneContext(flux, hashA), 11, "en", ""); err == nil {
		t.Fatal("Download succeeded with a failing repository")
	}
	if got, _ := os.ReadFile(existing); string(got) != "old" {
		t.Errorf("en.srt = %q, want the old content", got)
	}
	entries, _ := os.ReadDir(movieDir)
	if len(entries) != 1 {
		t.Errorf("files = %v, want only en.srt", entries)
	}
}

func TestService_SearchRanked(t *testing.T) {
	fetcher := &subtitletest.Fetcher{Response: &opensubtitles.SearchResponse{Data: []opensubtitles.SubtitleResult{
		{Attributes: opensubtitles.SubtitleAttributes{Language: "en", DownloadCount: 900, Release: "Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS",
			Files: []opensubtitles.SubtitleFile{{FileID: 1, FileName: "a.srt"}}}},
		{Attributes: opensubtitles.SubtitleAttributes{Language: "PT-BR", DownloadCount: 10, Release: flux, AITranslated: true,
			Files: []opensubtitles.SubtitleFile{{FileID: 2, FileName: "b.srt"}}}},
	}}}
	svc := subtitle.NewService(fetcher, subtitletest.NewRepo(), t.TempDir())

	got, err := svc.SearchRanked(context.Background(), duneContext(flux, hashA), []string{"en", "pt-br"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.Searches) != 1 {
		t.Fatalf("searches = %d", len(fetcher.Searches))
	}
	wantParams := opensubtitles.SearchParams{Type: "movie", TMDBID: 693134, Languages: []string{"en", "pt-br"}}
	if !reflect.DeepEqual(fetcher.Searches[0], wantParams) {
		t.Errorf("params = %+v, want %+v", fetcher.Searches[0], wantParams)
	}
	if got[0].FileID != 2 || got[0].LanguageCode != "pt-br" || got[0].LanguageName != "Portuguese (Brazil)" || !got[0].MachineTranslated {
		t.Errorf("first = %+v, want the exact release, flagged as AI translated", got[0])
	}

	t.Run("not configured", func(t *testing.T) {
		svc := subtitle.NewService(nil, subtitletest.NewRepo(), t.TempDir())
		if _, err := svc.SearchRanked(context.Background(), duneContext(flux, hashA), []string{"en"}); !errors.Is(err, subtitle.ErrNotConfigured) {
			t.Errorf("err = %v, want ErrNotConfigured", err)
		}
	})
}
