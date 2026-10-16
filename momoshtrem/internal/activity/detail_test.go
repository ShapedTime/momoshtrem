package activity

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Sessions make a loaded torrent streaming and ride on its item; the
// summary has every one, HDRezka's too.
func TestList_StreamingSessions(t *testing.T) {
	e := newEnv()
	items := e.list(t)
	sev := items[severance]
	if sev.State != StateStreaming || len(sev.Streaming) != 1 || sev.Streaming[0].ID != "1" {
		t.Errorf("severance = %s with %+v, want streaming with session 1", sev.State, sev.Streaming)
	}
	for hash, it := range items {
		if hash != severance && len(it.Streaming) != 0 {
			t.Errorf("%s has sessions %+v", hash[:1], it.Streaming)
		}
	}
	sum, err := e.svc.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Streaming) != 2 || sum.Streaming[0].ID != "1" || sum.Streaming[1].Source != "hdrezka" || sum.Counts.Streaming != 1 {
		t.Errorf("summary streaming = %+v (counts %+v)", sum.Streaming, sum.Counts)
	}

	// Without a session the torrent isn't streaming, and nothing is null.
	e.svc.sessions = nil
	if it := e.list(t)[severance]; it.State != StateIdle || it.Streaming == nil {
		t.Errorf("without sessions: %s, %v", it.State, it.Streaming)
	}
	if sum, _ := e.svc.Summary(context.Background()); sum.Streaming == nil || len(sum.Streaming) != 0 {
		t.Errorf("summary streaming = %v, want []", sum.Streaming)
	}
}

// A torrent that isn't loaded and that no job names falls back to its
// stored metainfo for its name and size, read once.
func TestList_StoredMetainfoFallback(t *testing.T) {
	e := newEnv()
	reads := map[string]int{}
	e.svc.metainfo = countingMetainfo{fakeMetainfo{legacy: {Name: "Old.Movie.1999.1080p.BluRay", TotalSize: 8_000_000_000}}, reads}
	for range 3 {
		it := e.list(t)[legacy]
		if it.Name != "Old.Movie.1999.1080p.BluRay" || it.SizeBytes == nil || *it.SizeBytes != 8_000_000_000 {
			t.Fatalf("legacy = %q, %v; want the stored metainfo's", it.Name, it.SizeBytes)
		}
	}
	// Found once, then cached; past (no size) has nothing stored, which
	// isn't cached: its metadata may arrive later.
	if reads[legacy] != 1 || reads[past] != 3 {
		t.Errorf("metainfo reads = %v, want legacy's once", reads)
	}
	// A job's name and size win, and a loaded torrent's live ones: neither
	// reads it.
	if reads[frieren] != 0 || reads[dune] != 0 {
		t.Errorf("metainfo reads = %v, want none for frieren and dune", reads)
	}
	if it := e.list(t)[past]; it.Name != "Past.Lives.2023.1080p" || it.SizeBytes != nil {
		t.Errorf("past = %q, %v", it.Name, it.SizeBytes)
	}
}

type countingMetainfo struct {
	m     fakeMetainfo
	reads map[string]int
}

func (c countingMetainfo) Summary(hash string) (*torrent.StoredInfo, error) {
	c.reads[hash]++
	return c.m.Summary(hash)
}

func TestDetail(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	e.lib.magnets = map[string]string{
		dune:    "magnet:?xt=urn:btih:" + dune + "&dn=Dune",
		frieren: "magnet:?xt=urn:btih:" + frieren + "&tr=udp%3A%2F%2Fnyaa.tracker.wf%3A7777%2Fannounce",
		legacy:  "magnet:?xt=urn:btih:" + legacy + "&tr=udp%3A%2F%2Fa%3A1&tr=udp%3A%2F%2Fb%3A2&tr=udp%3A%2F%2Fa%3A1",
	}
	e.lib.mappings = map[string][]library.FileMapping{
		dune: {{FilePath: "Dune/Dune.mkv", MovieID: ptr(int64(8))}},
		frieren: {
			{FilePath: "Frieren/Frieren - 29.mkv", Episode: &library.EpisodeRef{ID: 129, ShowID: 3, Season: 1, Number: 29, Name: "A Selfish Wish"}},
			{FilePath: "Frieren/Frieren - 30.mkv", Episode: &library.EpisodeRef{ID: 130, ShowID: 3, Season: 1, Number: 30}},
		},
	}
	e.lib.jobFiles = map[string][]library.JobFile{
		dune: {{Path: "Dune/Dune.mkv", Size: 900, Kind: library.KindMovie, Decision: library.DecisionAccepted},
			{Path: "Dune/Sample/dune.sample.mkv", Size: 90, Kind: library.KindExtra, ExtraType: "sample", Decision: library.DecisionIgnored}},
		frieren: {
			{Path: "Frieren/Frieren - 29.mkv", Size: 1300, Kind: library.KindEpisode, Decision: library.DecisionAccepted},
			{Path: "Frieren/Frieren - 30.mkv", Size: 1280, Kind: library.KindEpisode, Decision: library.DecisionAccepted},
			{Path: "Frieren/Frieren - NCOP2.mkv", Size: 140, Kind: library.KindExtra, ExtraType: "opening", Decision: library.DecisionIgnored},
			{Path: "Frieren/Frieren - 29.ass", Size: 40, Kind: library.KindSubtitle, Decision: library.DecisionAccepted, Linked: true},
			{Path: "Frieren/Fonts/OpenSans.ttf", Size: 2, Kind: library.KindExtra, ExtraType: "font", Decision: library.DecisionIgnored},
			{Path: "Frieren/Frieren - 31 (raw).mkv", Size: 1300, Kind: library.KindEpisode, Decision: library.DecisionIgnored},
		},
	}
	e.torrents.details[dune] = &torrent.Detail{
		HasInfo: true, Name: "Dune.Part.Two.2160p", TotalSize: 1000, PieceLength: 100, PiecesTotal: 10, PiecesHave: 4,
		Pieces: []torrent.PieceRun{{State: torrent.PieceHave, Count: 4}, {State: torrent.PiecePriority, Count: 2}, {State: torrent.PieceMissing, Count: 4}},
		Files: []torrent.FileEntry{{Path: "Dune/Dune.mkv", Size: 900, BytesCompleted: 360}, {Path: "Dune/Sample/dune.sample.mkv", Size: 90},
			{Path: "Dune/Dune.nfo", Size: 10, BytesCompleted: 10}},
		Trackers:        []string{"udp://tracker.opentrackr.org:1337/announce"},
		Magnet:          "magnet:?xt=urn:btih:" + dune + "&dn=live",
		DownloadedBytes: 400, UploadedBytes: 100, Peers: 10, Seeders: 4,
	}
	e.svc.metainfo = fakeMetainfo{frieren: {Name: "Frieren", TotalSize: 4062, PieceLength: 1 << 20, PiecesTotal: 4,
		Files: []torrent.FileEntry{{Path: "Frieren/Frieren - 29.mkv", Size: 1300}, {Path: "Frieren/Frieren - 30.mkv", Size: 1280},
			{Path: "Frieren/Frieren - NCOP2.mkv", Size: 140}, {Path: "Frieren/Frieren - 29.ass", Size: 40},
			{Path: "Frieren/Fonts/OpenSans.ttf", Size: 2}, {Path: "Frieren/Frieren - 31 (raw).mkv", Size: 1300}},
		Trackers: []string{"http://nyaa.tracker.wf:7777/announce"}}}

	if _, err := e.svc.Detail(ctx, "summary"); !errors.Is(err, torrent.ErrInvalidInfoHash) {
		t.Errorf("Detail(summary) = %v, want ErrInvalidInfoHash", err)
	}
	if _, err := e.svc.Detail(ctx, hash('9')); !errors.Is(err, torrent.ErrTorrentNotFound) {
		t.Errorf("Detail(unknown) = %v, want ErrTorrentNotFound", err)
	}

	t.Run("loaded", func(t *testing.T) {
		d, err := e.svc.Detail(ctx, dune)
		if err != nil {
			t.Fatal(err)
		}
		if d.InfoHash != dune || d.State != StateDownloading || d.Name != "Dune.Part.Two.2160p" || val(d.MagnetURI) != e.lib.magnets[dune] {
			t.Errorf("item = %+v, magnet %v", d.Item, d.MagnetURI)
		}
		if val(d.PieceLength) != 100 || val(d.PiecesTotal) != 10 || val(d.PiecesHave) != 4 || len(d.Pieces) != 3 {
			t.Errorf("pieces: %v %v %v %v", d.PieceLength, d.PiecesTotal, d.PiecesHave, d.Pieces)
		}
		if len(d.Files) != 3 {
			t.Fatalf("files = %+v", d.Files)
		}
		movie, sample, nfo := d.Files[0], d.Files[1], d.Files[2]
		if val(movie.Progress) != 0.4 || len(movie.PlaysAs) != 1 || movie.PlaysAs[0] != (PlaysAs{MediaType: "movie", ItemID: 8, Label: "Movie"}) || movie.Ignored != nil {
			t.Errorf("the movie's file = %+v", movie)
		}
		if sample.Ignored == nil || *sample.Ignored != (Ignored{Reason: "sample", Label: "Ignored (sample)"}) || val(sample.Progress) != 0 {
			t.Errorf("the sample = %+v", sample)
		}
		if nfo.Ignored != nil || len(nfo.PlaysAs) != 0 || val(nfo.Progress) != 1 {
			t.Errorf("a file no job lists = %+v", nfo)
		}
		if len(d.Trackers) != 1 || d.Trackers[0] != (Tracker{URL: "udp://tracker.opentrackr.org:1337/announce", Status: "unknown"}) {
			t.Errorf("trackers = %+v", d.Trackers)
		}
		if val(d.DownloadedBytes) != 400 || val(d.UploadedBytes) != 100 || val(d.Ratio) != 0.25 ||
			d.Swarm == nil || *d.Swarm != (Swarm{ConnectedPeers: 10, ConnectedSeeders: 4}) {
			t.Errorf("totals: %v %v %v %+v", d.DownloadedBytes, d.UploadedBytes, d.Ratio, d.Swarm)
		}
	})

	t.Run("not loaded, from the stored metainfo", func(t *testing.T) {
		d, err := e.svc.Detail(ctx, frieren)
		if err != nil {
			t.Fatal(err)
		}
		if d.Loaded || d.Pieces != nil || d.PiecesHave != nil || val(d.PiecesTotal) != 4 || val(d.PieceLength) != 1<<20 {
			t.Errorf("pieces: loaded %v, %v %v %v %v", d.Loaded, d.Pieces, d.PiecesHave, d.PiecesTotal, d.PieceLength)
		}
		want := []struct {
			label, ignored string
		}{{"S01E29", ""}, {"S01E30", ""}, {"", "Ignored (NCOP)"}, {"", ""}, {"", "Ignored (font)"}, {"", "Ignored"}}
		if len(d.Files) != len(want) {
			t.Fatalf("files = %+v", d.Files)
		}
		for i, w := range want {
			f := d.Files[i]
			label := ""
			if len(f.PlaysAs) > 0 {
				label = f.PlaysAs[0].Label
			}
			ignored := ""
			if f.Ignored != nil {
				ignored = f.Ignored.Label
			}
			if f.Progress != nil || label != w.label || ignored != w.ignored {
				t.Errorf("file %d %s: progress %v, plays as %q, ignored %q; want %q, %q", i, f.Path, f.Progress, label, ignored, w.label, w.ignored)
			}
		}
		ep := d.Files[0].PlaysAs[0]
		if ep.MediaType != "tv" || ep.ItemID != 129 || val(ep.SeasonNumber) != 1 || val(ep.EpisodeNumber) != 29 ||
			val(ep.EpisodeName) != "A Selfish Wish" || d.Files[1].PlaysAs[0].EpisodeName != nil {
			t.Errorf("plays as = %+v / %+v", ep, d.Files[1].PlaysAs[0])
		}
		if d.Files[2].Ignored.Reason != "opening" || d.Files[5].Ignored.Reason != "user" {
			t.Errorf("reasons = %q, %q", d.Files[2].Ignored.Reason, d.Files[5].Ignored.Reason)
		}
		if len(d.Trackers) != 1 || d.Trackers[0].URL != "http://nyaa.tracker.wf:7777/announce" {
			t.Errorf("trackers = %+v, want the stored ones", d.Trackers)
		}
		if d.DownloadedBytes != nil || d.UploadedBytes != nil || d.Ratio != nil || d.Swarm != nil {
			t.Errorf("totals of a torrent that isn't loaded: %v %v %v %v", d.DownloadedBytes, d.UploadedBytes, d.Ratio, d.Swarm)
		}
	})

	t.Run("not loaded, nothing stored: the job's list and the magnet's trackers", func(t *testing.T) {
		e.svc.metainfo = nil
		d, err := e.svc.Detail(ctx, frieren)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Files) != 6 || d.Files[0].SizeBytes != 1300 || d.PieceLength != nil || d.PiecesTotal != nil {
			t.Errorf("files %d, pieces %v %v", len(d.Files), d.PieceLength, d.PiecesTotal)
		}
		if len(d.Trackers) != 1 || d.Trackers[0].URL != "udp://nyaa.tracker.wf:7777/announce" {
			t.Errorf("trackers = %+v, want the magnet's", d.Trackers)
		}
		d, _ = e.svc.Detail(ctx, legacy)
		if len(d.Files) != 0 || d.Files == nil {
			t.Errorf("no file list anywhere: files %v, want []", d.Files)
		}
		if got := []string{d.Trackers[0].URL, d.Trackers[1].URL}; len(d.Trackers) != 2 || !slices.Equal(got, []string{"udp://a:1", "udp://b:2"}) {
			t.Errorf("trackers = %+v, want each once", d.Trackers)
		}
	})

	t.Run("loaded, nothing references it: the live magnet", func(t *testing.T) {
		d, err := e.svc.Detail(ctx, inspect)
		if err != nil {
			t.Fatal(err)
		}
		// Its metadata is still arriving: no pieces, no files.
		if d.MagnetURI != nil || d.Pieces != nil || len(d.Files) != 0 || d.Swarm == nil {
			t.Errorf("detail = %+v", d)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		d, _ := e.svc.Detail(ctx, frieren)
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		_ = json.Unmarshal(raw, &got)
		for _, field := range []string{"pieces", "pieces_have", "downloaded_bytes", "uploaded_bytes", "ratio", "swarm", "progress"} {
			if field == "progress" {
				if v := got["files"].([]any)[0].(map[string]any)["progress"]; v != nil {
					t.Errorf("files[0].progress = %v, want null", v)
				}
				continue
			}
			if v, ok := got[field]; !ok || v != nil {
				t.Errorf("%s = %v (present %v), want null", field, v, ok)
			}
		}
		for _, field := range []string{"info_hash", "state", "titles", "streaming", "magnet_uri", "files", "trackers"} {
			if _, ok := got[field]; !ok {
				t.Errorf("%s missing from %s", field, raw)
			}
		}
	})
}

func TestIgnoredLabels(t *testing.T) {
	tests := []struct {
		file library.JobFile
		want *Ignored
	}{
		{library.JobFile{Kind: library.KindExtra, ExtraType: "opening", Decision: library.DecisionIgnored}, &Ignored{"opening", "Ignored (NCOP)"}},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "ending", Decision: library.DecisionIgnored}, &Ignored{"ending", "Ignored (NCED)"}},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "info", Decision: library.DecisionIgnored}, &Ignored{"info", "Ignored (not video)"}},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "image", Decision: library.DecisionIgnored}, &Ignored{"image", "Ignored (not video)"}},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "superseded", Decision: library.DecisionIgnored}, &Ignored{"superseded", "Ignored (older version)"}},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "trailer", Decision: library.DecisionIgnored}, &Ignored{"trailer", "Ignored (trailer)"}},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "other", Decision: library.DecisionIgnored}, &Ignored{"other", "Ignored (extra)"}},
		{library.JobFile{Kind: library.KindSubtitle, Decision: library.DecisionIgnored}, &Ignored{"subtitle", "Ignored (subtitle)"}},
		{library.JobFile{Kind: library.KindMovie, Decision: library.DecisionIgnored}, &Ignored{"user", "Ignored"}},
		{library.JobFile{Kind: library.KindSubtitle, Decision: library.DecisionIgnored, Linked: true}, nil},
		{library.JobFile{Kind: library.KindEpisode, Decision: library.DecisionPending}, nil},
		{library.JobFile{Kind: library.KindExtra, ExtraType: "opening", Decision: library.DecisionAccepted}, nil},
	}
	for _, tt := range tests {
		got := ignoredOf(tt.file)
		if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
			t.Errorf("ignoredOf(%+v) = %+v, want %+v", tt.file, got, tt.want)
		}
	}
}

func val[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
