package activity

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// Fakes for the Service's dependencies.

type fakeTorrents struct {
	loaded  map[string]torrent.TorrentStatus
	details map[string]*torrent.Detail
	paused  map[string]time.Time
}

func newFakeTorrents() *fakeTorrents {
	return &fakeTorrents{loaded: map[string]torrent.TorrentStatus{}, details: map[string]*torrent.Detail{}, paused: map[string]time.Time{}}
}

func (f *fakeTorrents) Detail(hash string) (*torrent.Detail, error) {
	if _, ok := f.loaded[hash]; !ok {
		return nil, torrent.ErrTorrentNotFound
	}
	if d, ok := f.details[hash]; ok {
		return d, nil
	}
	return &torrent.Detail{}, nil // metadata still arriving
}

func (f *fakeTorrents) ListTorrents() ([]torrent.TorrentStatus, error) {
	return slices.Collect(maps.Values(f.loaded)), nil
}

func (f *fakeTorrents) GetStatus(hash string) (*torrent.TorrentStatus, error) {
	st, ok := f.loaded[hash]
	if !ok {
		return nil, torrent.ErrTorrentNotFound
	}
	return &st, nil
}

func (f *fakeTorrents) Pause(hash string) error {
	if _, ok := f.paused[hash]; !ok {
		f.paused[hash] = time.Now()
	}
	if st, ok := f.loaded[hash]; ok {
		st.Paused, st.IsPaused = true, true
		f.loaded[hash] = st
	}
	return nil
}

func (f *fakeTorrents) Resume(hash string) error {
	if _, ok := f.paused[hash]; !ok {
		return nil
	}
	delete(f.paused, hash)
	if st, ok := f.loaded[hash]; ok {
		st.Paused, st.IsPaused = false, false
		f.loaded[hash] = st
	}
	return nil
}

func (f *fakeTorrents) Pauses() map[string]time.Time { return maps.Clone(f.paused) }

func (f *fakeTorrents) RestorePauses(p map[string]time.Time) { maps.Copy(f.paused, p) }

type fakeLibrary struct {
	known    []library.KnownTorrent
	magnets  map[string]string
	mappings map[string][]library.FileMapping
	jobFiles map[string][]library.JobFile
}

func (f *fakeLibrary) TorrentMagnet(_ context.Context, hash string) (string, error) {
	return f.magnets[hash], nil
}

func (f *fakeLibrary) ActiveMappings(_ context.Context, hash string) ([]library.FileMapping, error) {
	return f.mappings[hash], nil
}

func (f *fakeLibrary) JobFiles(_ context.Context, hash string) ([]library.JobFile, error) {
	return f.jobFiles[hash], nil
}

type fakeMetainfo map[string]*torrent.StoredInfo

func (f fakeMetainfo) Summary(hash string) (*torrent.StoredInfo, error) { return f[hash], nil }

func (f *fakeLibrary) KnownTorrents(_ context.Context, hash string) ([]library.KnownTorrent, error) {
	if hash == "" {
		return f.known, nil
	}
	for _, k := range f.known {
		if k.InfoHash == hash {
			return []library.KnownTorrent{k}, nil
		}
	}
	return nil, nil
}

type fakePauseStore struct {
	paused map[string]time.Time
	err    error
	writes int
}

func (f *fakePauseStore) TorrentPauses(context.Context) (map[string]time.Time, error) {
	return maps.Clone(f.paused), f.err
}

func (f *fakePauseStore) PauseTorrents(_ context.Context, p map[string]time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.writes++
	for h, at := range p {
		if _, ok := f.paused[h]; !ok {
			f.paused[h] = at
		}
	}
	return nil
}

func (f *fakePauseStore) ResumeTorrents(_ context.Context, hashes []string) error {
	if f.err != nil {
		return f.err
	}
	f.writes++
	for _, h := range hashes {
		delete(f.paused, h)
	}
	return nil
}

// fakeSessions are the sessions streaming now, the most recently read
// first.
type fakeSessions []sessions.Session

func (f fakeSessions) Sessions(context.Context) []sessions.Session { return slices.Clone(f) }

// session is Infuse on the Apple TV reading a torrent's file (hdrezka
// when hash is "").
func session(id, hash string, lastRead time.Time) sessions.Session {
	s := sessions.Session{ID: id, Source: sessions.SourceHDRezka, FileName: "f.mkv", Client: ptr("Infuse"), Device: ptr("Apple TV"),
		OffsetBytes: 100, SizeBytes: 1000, RateBps: 9_000_000, StartedAt: at(45), LastReadAt: lastRead}
	if hash != "" {
		s.Source, s.InfoHash, s.FilePath = sessions.SourceTorrent, &hash, ptr("Show/f.mkv")
	}
	return s
}

type fakeSpeeds struct {
	total   torrent.Rate
	history []torrent.Rate
}

func (f fakeSpeeds) Total() torrent.Rate     { return f.total }
func (f fakeSpeeds) History() []torrent.Rate { return f.history }
func (f fakeSpeeds) Interval() time.Duration { return 2 * time.Second }

// fakeCache holds 1 GB of a 4 GB cache.
type fakeCache struct{}

func (fakeCache) Info() piececache.CacheInfo {
	return piececache.CacheInfo{Capacity: 4 << 30, Filled: 1 << 30}
}

// hash is 40 of c: a well-formed info hash.
func hash(c byte) string { return string(slices.Repeat([]byte{c}, 40)) }

func at(minutes int) time.Time { return time.Date(2026, 10, 1, 12, minutes, 0, 0, time.UTC) }

func ptr[T any](v T) *T { return &v }

// episodes are season's episodes from–to.
func episodes(season, from, to int) (out []library.EpisodeNumber) {
	for e := from; e <= to; e++ {
		out = append(out, library.EpisodeNumber{Season: season, Episode: e})
	}
	return out
}

// env is a Service over fakes holding one torrent of each kind:
//   - frieren: a show's, not loaded, with an open job;
//   - dune: a movie's, loaded and fetching;
//   - severance: a show's, loaded, with a streaming session (and an
//     HDRezka session elsewhere);
//   - past: a movie's, not loaded, paused;
//   - inspect: loaded, nothing references it (a dry run);
//   - legacy: a movie's, not loaded, no job (its magnet has a dn);
//   - done: a movie's, loaded, complete.
type env struct {
	svc      *Service
	torrents *fakeTorrents
	lib      *fakeLibrary
	store    *fakePauseStore
	statfs   error
}

var (
	frieren, dune, severance, past = hash('a'), hash('b'), hash('c'), hash('d')
	inspect, legacy, done          = hash('e'), hash('f'), hash('0')
)

func newEnv() *env {
	e := &env{torrents: newFakeTorrents(), store: &fakePauseStore{paused: map[string]time.Time{}}}
	e.lib = &fakeLibrary{known: []library.KnownTorrent{
		{InfoHash: frieren, JobName: "Frieren.S01.1080p.WEB", SizeBytes: ptr(int64(12_600_000_000)),
			MagnetURI: "magnet:?xt=urn:btih:" + frieren + "&dn=Frieren.dn", AddedAt: at(1),
			OpenJob: &library.OpenJobRef{ID: 7, Status: library.JobNeedsReview},
			Titles: []library.KnownTitle{{MediaType: library.JobTV, LibraryID: 3, TMDBID: 209867, Title: "Frieren", Year: 2023,
				PosterPath: "/frieren.jpg", Episodes: episodes(1, 29, 38), SeasonSizes: map[int]int{1: 52}}}},
		{InfoHash: dune, JobName: dune, AddedAt: at(2),
			Titles: []library.KnownTitle{{MediaType: library.JobMovie, LibraryID: 8, TMDBID: 693134, Title: "Dune: Part Two", Year: 2024}}},
		{InfoHash: severance, AddedAt: at(3),
			Titles: []library.KnownTitle{{MediaType: library.JobTV, LibraryID: 4, TMDBID: 95396, Title: "Severance", Year: 2022,
				Episodes: episodes(2, 1, 10), SeasonSizes: map[int]int{2: 10}}}},
		{InfoHash: past, JobName: "Past.Lives.2023.1080p", AddedAt: at(4),
			Titles: []library.KnownTitle{{MediaType: library.JobMovie, LibraryID: 9, TMDBID: 666277, Title: "Past Lives", Year: 2023}}},
		{InfoHash: legacy, MagnetURI: "magnet:?xt=urn:btih:" + legacy + "&dn=Old.Movie.1999", AddedAt: at(5),
			Titles: []library.KnownTitle{{MediaType: library.JobMovie, LibraryID: 10, TMDBID: 1, Title: "Old Movie", Year: 1999}}},
		{InfoHash: done, AddedAt: at(6),
			Titles: []library.KnownTitle{{MediaType: library.JobMovie, LibraryID: 11, TMDBID: 2, Title: "Done", Year: 2020}}},
	}}
	e.torrents.loaded[dune] = torrent.TorrentStatus{InfoHash: dune, Name: "Dune.Part.Two.2160p", TotalSize: 1000, Downloaded: 420,
		Progress: 0.42, Seeders: 4, Peers: 10, DownloadSpeed: 2_000_000, UploadSpeed: 30_000, LoadedAt: at(20), LastActive: at(30)}
	e.torrents.loaded[severance] = torrent.TorrentStatus{InfoHash: severance, Name: "Severance.S02", TotalSize: 500, Downloaded: 500,
		Progress: 1, Complete: true, Peers: 3, Seeders: 3, IsPaused: true, LoadedAt: at(21), LastActive: at(25)}
	e.torrents.loaded[inspect] = torrent.TorrentStatus{InfoHash: inspect, Name: "Some.Release", TotalSize: 900, IsPaused: true,
		LoadedAt: at(22), LastActive: at(22)}
	e.torrents.loaded[done] = torrent.TorrentStatus{InfoHash: done, Name: "Done.2020", TotalSize: 100, Downloaded: 100,
		Progress: 1, Complete: true, LoadedAt: at(23), LastActive: at(40)}
	e.torrents.paused[past] = at(10)
	e.svc = New(Deps{
		Torrents: e.torrents, Library: e.lib, Pauses: e.store,
		Sessions: fakeSessions{session("1", severance, at(49)), session("2", "", at(48))},
		Speeds:   fakeSpeeds{total: torrent.Rate{Down: 2_000_000, Up: 30_000}, history: []torrent.Rate{{Down: 1, Up: 2}, {Down: 3, Up: 4}}},
		Cache:    fakeCache{}, DiskPath: "/data/torrents",
		Now: func() time.Time { return at(50) },
		Statfs: func(path string) (int64, int64, error) {
			if e.statfs != nil {
				return 0, 0, e.statfs
			}
			return 100 << 30, 40 << 30, nil
		},
	})
	return e
}

func (e *env) list(t *testing.T) map[string]Item {
	t.Helper()
	items, err := e.svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Item{}
	for _, it := range items {
		out[it.InfoHash] = it
	}
	return out
}

func TestList_MergesTheLibraryAndTheClient(t *testing.T) {
	e := newEnv()
	items, err := e.svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Streaming, downloading, paused, then idle: loaded ones by last
	// activity, then the ones not loaded, newest first.
	var order []string
	for _, it := range items {
		order = append(order, it.InfoHash)
	}
	want := []string{severance, dune, past, done, inspect, legacy, frieren}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v\nwant    %v", order, want)
	}

	byHash := e.list(t)
	tests := []struct {
		hash      string
		state     State
		name      string
		loaded    bool
		size      *int64
		addedAt   time.Time
		titles    int
		wantScope string
	}{
		{frieren, StateIdle, "Frieren.S01.1080p.WEB", false, ptr(int64(12_600_000_000)), at(1), 1, "S01E29–E38"},
		{dune, StateDownloading, "Dune.Part.Two.2160p", true, ptr(int64(1000)), at(2), 1, ""},
		{severance, StateStreaming, "Severance.S02", true, ptr(int64(500)), at(3), 1, "Season 2"},
		{past, StatePaused, "Past.Lives.2023.1080p", false, nil, at(4), 1, ""},
		{inspect, StateIdle, "Some.Release", true, ptr(int64(900)), at(22), 0, ""},
		{legacy, StateIdle, "Old.Movie.1999", false, nil, at(5), 1, ""},
		{done, StateIdle, "Done.2020", true, ptr(int64(100)), at(6), 1, ""},
	}
	for _, tt := range tests {
		it := byHash[tt.hash]
		if it.State != tt.state || it.Name != tt.name || it.Loaded != tt.loaded || !it.AddedAt.Equal(tt.addedAt) || len(it.Titles) != tt.titles {
			t.Errorf("%s: %+v", tt.hash[:1], it)
		}
		if (it.SizeBytes == nil) != (tt.size == nil) || (it.SizeBytes != nil && *it.SizeBytes != *tt.size) {
			t.Errorf("%s: size %v, want %v", tt.hash[:1], it.SizeBytes, tt.size)
		}
		if tt.wantScope != "" && (it.Titles[0].Scope == nil || *it.Titles[0].Scope != tt.wantScope) {
			t.Errorf("%s: scope %v, want %q", tt.hash[:1], deref(it.Titles[0].Scope), tt.wantScope)
		}
		if tt.titles > 0 && it.Titles[0].MediaType == "movie" && it.Titles[0].Scope != nil {
			t.Errorf("%s: a movie has scope %q", tt.hash[:1], *it.Titles[0].Scope)
		}
	}

	if d := byHash[dune]; d.DownloadBps != 2_000_000 || d.UploadBps != 30_000 || *d.Peers != 10 || *d.Seeders != 4 ||
		*d.Progress != 0.42 || *d.BytesCompleted != 420 || *d.Complete || !d.LastActiveAt.Equal(at(30)) {
		t.Errorf("dune = %+v", d)
	}
	if f := byHash[frieren]; f.OpenJob == nil || f.OpenJob.ID != 7 || f.OpenJob.Status != "needs_review" ||
		f.Titles[0].PosterPath == nil || *f.Titles[0].PosterPath != "/frieren.jpg" || f.Titles[0].LibraryID != 3 {
		t.Errorf("frieren = %+v", f)
	}
	if name := byHash[dune].Name; name == dune {
		t.Error("the info dict's name should beat a job name that's only the hash")
	}
}

// A torrent the client hasn't loaded has nulls for what only the client
// knows, and lists are never null.
func TestList_NotLoadedJSON(t *testing.T) {
	e := newEnv()
	raw, err := json.Marshal(e.list(t)[frieren])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"complete", "progress", "bytes_completed", "peers", "seeders", "last_active_at"} {
		if v, ok := got[field]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want null", field, v, ok)
		}
	}
	if got["download_bps"] != 0.0 || got["upload_bps"] != 0.0 || got["loaded"] != false {
		t.Errorf("speeds/loaded = %v %v %v", got["download_bps"], got["upload_bps"], got["loaded"])
	}
	if s, ok := got["streaming"].([]any); !ok || len(s) != 0 {
		t.Errorf("streaming = %v, want []", got["streaming"])
	}
	raw, _ = json.Marshal(e.list(t)[inspect])
	if string(raw) == "" || !json.Valid(raw) {
		t.Fatal(string(raw))
	}
	_ = json.Unmarshal(raw, &got)
	if titles, ok := got["titles"].([]any); !ok || len(titles) != 0 || got["open_job"] != nil {
		t.Errorf("an unreferenced torrent: titles %v, open_job %v", got["titles"], got["open_job"])
	}
}

func TestSummary(t *testing.T) {
	e := newEnv()
	sum, err := e.svc.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Streaming: 1, Downloading: 1, Paused: 1, Idle: 4}
	if sum.Counts != want || sum.Peers != 13 || sum.DownloadBps != 2_000_000 || sum.UploadBps != 30_000 {
		t.Errorf("summary = %+v", sum)
	}
	if h := sum.History; h.IntervalSeconds != 2 || !slices.Equal(h.DownloadBps, []int64{1, 3}) || !slices.Equal(h.UploadBps, []int64{2, 4}) {
		t.Errorf("history = %+v", h)
	}
	if d := sum.Disk; d == nil || *d != (Disk{TotalBytes: 100 << 30, FreeBytes: 40 << 30, TorrentsBytes: 1 << 30, CacheLimitBytes: 4 << 30}) {
		t.Errorf("disk = %+v", d)
	}
	if sum.Streaming == nil {
		t.Error("streaming is null")
	}

	e.statfs = errors.New("no such file or directory")
	if sum, _ = e.svc.Summary(context.Background()); sum.Disk != nil {
		t.Errorf("disk = %+v, want null when statfs fails", sum.Disk)
	}
	e.svc.speeds = nil
	if sum, _ = e.svc.Summary(context.Background()); sum.History.DownloadBps == nil || sum.History.IntervalSeconds != 2 || sum.DownloadBps != 0 {
		t.Errorf("without a sampler: %+v", sum)
	}
}

func TestPauseAndResume(t *testing.T) {
	ctx := context.Background()
	e := newEnv()

	if _, err := e.svc.Pause(ctx, "summary"); !errors.Is(err, torrent.ErrInvalidInfoHash) {
		t.Errorf("Pause(summary) = %v, want ErrInvalidInfoHash", err)
	}
	if _, err := e.svc.Pause(ctx, hash('9')); !errors.Is(err, torrent.ErrTorrentNotFound) || e.store.writes != 0 {
		t.Errorf("Pause(unknown) = %v (writes %d), want ErrTorrentNotFound", err, e.store.writes)
	}

	// Not loaded: it's paused (and stays unloaded); by an uppercase hash.
	item, err := e.svc.Pause(ctx, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	if item.InfoHash != frieren || item.State != StatePaused || item.Loaded || item.Titles[0].Title != "Frieren" {
		t.Errorf("paused item = %+v", item)
	}
	if _, ok := e.store.paused[frieren]; !ok {
		t.Error("the pause isn't stored")
	}
	if _, ok := e.torrents.paused[frieren]; !ok {
		t.Error("the client didn't pause it")
	}
	stored := e.store.paused[frieren]
	if _, err := e.svc.Pause(ctx, frieren); err != nil || !e.store.paused[frieren].Equal(stored) {
		t.Errorf("pausing again: %v, stored %v → %v", err, stored, e.store.paused[frieren])
	}

	// Loaded and nothing references it: still known.
	if item, err = e.svc.Pause(ctx, inspect); err != nil || item.State != StatePaused || !item.Loaded {
		t.Errorf("pause a loaded dry run = %+v, %v", item, err)
	}

	item, err = e.svc.Resume(ctx, frieren)
	if err != nil || item.State != StateIdle {
		t.Fatalf("resume = %+v, %v", item, err)
	}
	if _, ok := e.store.paused[frieren]; ok {
		t.Error("the stored pause is still there")
	}
	if item, err = e.svc.Resume(ctx, frieren); err != nil || item.State != StateIdle {
		t.Errorf("resume again = %+v, %v", item, err)
	}

	// The store fails: the client isn't touched.
	e.store.err = errors.New("connection reset")
	if _, err := e.svc.Pause(ctx, dune); err == nil {
		t.Fatal("Pause with the store down: want an error")
	}
	if _, ok := e.torrents.paused[dune]; ok {
		t.Error("the client paused a torrent whose pause wasn't stored")
	}
}

func TestPauseAllAndResumeAll(t *testing.T) {
	ctx := context.Background()
	e := newEnv()

	n, err := e.svc.PauseAll(ctx)
	if err != nil || n != 2 {
		t.Fatalf("PauseAll = %d, %v; want the streaming and the downloading torrent", n, err)
	}
	for _, h := range []string{severance, dune} {
		if _, ok := e.store.paused[h]; !ok {
			t.Errorf("%s: pause not stored", h[:1])
		}
		if !e.torrents.loaded[h].Paused {
			t.Errorf("%s: not paused in the client", h[:1])
		}
	}
	if len(e.store.paused) != 2 {
		t.Errorf("stored pauses = %v", e.store.paused)
	}
	if n, _ := e.svc.PauseAll(ctx); n != 0 {
		t.Errorf("PauseAll again = %d, want 0", n)
	}

	// Resume all: every paused torrent, the one paused before (not loaded) too.
	if n, err = e.svc.ResumeAll(ctx); err != nil || n != 3 {
		t.Fatalf("ResumeAll = %d, %v; want 3", n, err)
	}
	if len(e.torrents.paused) != 0 || len(e.store.paused) != 0 {
		t.Errorf("left paused: client %v, store %v", e.torrents.paused, e.store.paused)
	}
	if items := e.list(t); items[dune].State != StateDownloading || items[severance].State != StateStreaming {
		t.Errorf("after resume all: dune %s, severance %s", items[dune].State, items[severance].State)
	}
}

func TestRestorePauses(t *testing.T) {
	e := newEnv()
	e.torrents.paused = map[string]time.Time{}
	upperFrieren := strings.ToUpper(frieren) // stored by hand
	e.store.paused = map[string]time.Time{dune: at(1), upperFrieren: at(2), hash('9'): at(3), inspect: at(4), "junk": at(5)}
	if err := e.svc.RestorePauses(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The library knows dune and frieren; inspect is only loaded (a dry run,
	// gone after a restart), 999… and junk nothing knows: pruned.
	if got := slices.Sorted(maps.Keys(e.torrents.paused)); !slices.Equal(got, []string{upperFrieren, dune}) {
		t.Errorf("client pauses = %v, want dune's and frieren's", got)
	}
	if got := slices.Sorted(maps.Keys(e.store.paused)); !slices.Equal(got, []string{upperFrieren, dune}) {
		t.Errorf("stored pauses = %v, want the unknown ones pruned", got)
	}

	// Nothing stored: nothing to read from the library.
	e.store.paused = map[string]time.Time{}
	e.lib.known = nil
	if err := e.svc.RestorePauses(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.store.err = errors.New("connection reset")
	if err := e.svc.RestorePauses(context.Background()); err == nil {
		t.Error("want the store's error")
	}
}
