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

	"github.com/shapedtime/momoshtrem/internal/downloads"
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

// fakeDownloads is a downloads snapshot (#77): what they want of each
// torrent, and what they pin on disk.
type fakeDownloads struct {
	torrents map[string]downloads.TorrentShare
	usage    downloads.Usage
}

func (f *fakeDownloads) Torrent(hash string) (downloads.TorrentShare, bool) {
	s, ok := f.torrents[hash]
	return s, ok
}

func (f *fakeDownloads) Usage() downloads.Usage { return f.usage }

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
//   - dune: a movie's, loaded and fetching: kept, and the download manager
//     downloads it;
//   - severance: a show's, loaded, with a streaming session (and an
//     HDRezka session elsewhere);
//   - past: a movie's, not loaded, paused;
//   - inspect: loaded, nothing references it (a dry run);
//   - legacy: a movie's, not loaded, no job (its magnet has a dn);
//   - done: a movie's, loaded, complete: a smart pick and a Keep have it
//     all on disk.
type env struct {
	svc       *Service
	torrents  *fakeTorrents
	lib       *fakeLibrary
	store     *fakePauseStore
	downloads *fakeDownloads
	statfs    error
	// pauseChanges counts OnPauseChange calls (the download manager's wakes).
	pauseChanges int
}

// The downloads of the env's torrents.
var (
	duneHolds = []downloads.TorrentHold{{DownloadID: 5, Reason: library.DownloadKept, Scope: library.DownloadScopeMovie, Title: "Dune: Part Two"}}
	doneHolds = []downloads.TorrentHold{
		{DownloadID: 2, Reason: library.DownloadKept, Scope: library.DownloadScopeMovie, Title: "Done"},
		{DownloadID: 1, Reason: library.DownloadSmart, Scope: library.DownloadScopeEpisode, Title: "Done"},
	}
)

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
	e.downloads = &fakeDownloads{
		torrents: map[string]downloads.TorrentShare{
			dune: {Wanted: downloads.TorrentWanted{Bytes: 900, DoneBytes: 360, EtaSeconds: ptr(int64(999)), State: downloads.StateDownloading},
				Holds: duneHolds},
			done: {Wanted: downloads.TorrentWanted{Bytes: 100, DoneBytes: 100, State: downloads.StateDone}, Holds: doneHolds},
		},
		usage: downloads.Usage{KeptBytes: 3 << 29, SmartBytes: 1 << 28, CommittedKeptBytes: 2 << 30},
	}
	e.svc = New(Deps{
		Torrents: e.torrents, Library: e.lib, Pauses: e.store,
		Downloads: func() DownloadsSnapshot { return e.downloads },
		Sessions:  fakeSessions{session("1", severance, at(49)), session("2", "", at(48))},
		Speeds:    fakeSpeeds{total: torrent.Rate{Down: 2_000_000, Up: 30_000}, history: []torrent.Rate{{Down: 1, Up: 2}, {Down: 3, Up: 4}}},
		Cache:     fakeCache{}, DiskPath: "/data/torrents",
		OnPauseChange: func() { e.pauseChanges++ },
		Now:           func() time.Time { return at(50) },
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

// wanted and holds come from the downloads (#77): what they want of the
// torrent, with the ETA from its speed now, and who holds it. Downloading
// is the manager downloading a loaded torrent with its network on, or bytes
// arriving; a torrent that's only cached (left on after a stream) isn't.
func TestList_Downloads(t *testing.T) {
	e := newEnv()
	byHash := e.list(t)

	d := byHash[dune]
	if d.State != StateDownloading || d.Wanted == nil || d.Wanted.Bytes != 900 || d.Wanted.DoneBytes != 360 ||
		d.Wanted.State != downloads.StateDownloading || !slices.Equal(d.Holds, duneHolds) {
		t.Errorf("dune = %+v (wanted %+v, holds %+v)", d, d.Wanted, d.Holds)
	}
	// 540 bytes left at 2 MB/s: the live speed's ETA, not the pass's.
	if d.Wanted.EtaSeconds == nil || *d.Wanted.EtaSeconds != 1 {
		t.Errorf("dune eta = %v, want 1 s", derefInt(d.Wanted.EtaSeconds))
	}
	// Kept and smart, all on disk: idle, kept first.
	if dn := byHash[done]; dn.State != StateIdle || dn.Wanted == nil || dn.Wanted.State != downloads.StateDone ||
		dn.Wanted.EtaSeconds != nil || !slices.Equal(dn.Holds, doneHolds) {
		t.Errorf("done = %+v (wanted %+v)", dn, dn.Wanted)
	}
	for _, h := range []string{frieren, severance, past, inspect, legacy} {
		if it := byHash[h]; it.Wanted != nil || it.Holds == nil || len(it.Holds) != 0 {
			t.Errorf("%s: wanted %+v, holds %#v; want null and []", h[:1], it.Wanted, it.Holds)
		}
	}

	tests := []struct {
		name  string
		bps   int64
		share *downloads.TorrentShare
		want  State
	}{
		{"cached after a stream, nothing wants it", 0, nil, StateIdle},
		{"bytes still arriving, nothing wants it", 40_000, nil, StateDownloading},
		{"queued at 0 B/s", 0, &downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, State: downloads.StateQueued}}, StateIdle},
		{"waiting for the window", 0, &downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, State: downloads.StateWaitingForWindow}}, StateIdle},
		{"stalled", 0, &downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, State: downloads.StateStalled}}, StateIdle},
		{"in a slot, no bytes yet", 0, &downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, State: downloads.StateDownloading}}, StateDownloading},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			st := e.torrents.loaded[dune]
			st.DownloadSpeed = tt.bps
			e.torrents.loaded[dune] = st
			delete(e.downloads.torrents, dune)
			if tt.share != nil {
				e.downloads.torrents[dune] = *tt.share
			}
			it := e.list(t)[dune]
			if it.State != tt.want || (tt.share == nil) != (it.Wanted == nil) {
				t.Errorf("state %s, wanted %+v; want %s", it.State, it.Wanted, tt.want)
			}
			if it.Wanted != nil && it.Wanted.EtaSeconds != nil {
				t.Errorf("eta = %d, want null at 0 B/s or while waiting", *it.Wanted.EtaSeconds)
			}
		})
	}
}

// wanted and holds follow the web's TorrentListItem (contract/torrents.ts,
// the torrents mock): null and [] when nothing wants the torrent, and the
// fields of TorrentWanted, TorrentHold and the summary's disk.
func TestDownloadsJSON(t *testing.T) {
	e := newEnv()
	byHash := e.list(t)
	object := func(v any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	keys := func(m map[string]any) []string { return slices.Sorted(maps.Keys(m)) }
	sorted := func(s ...string) []string { return slices.Sorted(slices.Values(s)) }

	item := object(byHash[dune])
	if got, want := keys(item), sorted("info_hash", "name", "state", "loaded", "complete", "progress", "bytes_completed", "size_bytes",
		"download_bps", "upload_bps", "peers", "seeders", "added_at", "last_active_at", "titles", "open_job", "streaming",
		"wanted", "holds"); !slices.Equal(got, want) {
		t.Errorf("TorrentListItem fields =\n%v\nwant\n%v", got, want)
	}
	wanted := item["wanted"].(map[string]any)
	if got, want := keys(wanted), sorted("bytes", "done_bytes", "eta_seconds", "state", "resumes_at"); !slices.Equal(got, want) {
		t.Errorf("TorrentWanted fields = %v, want %v", got, want)
	}
	if wanted["resumes_at"] != nil || wanted["state"] != "downloading" {
		t.Errorf("wanted = %v", wanted)
	}
	hold := item["holds"].([]any)[0].(map[string]any)
	if got, want := keys(hold), sorted("download_id", "reason", "scope", "title"); !slices.Equal(got, want) {
		t.Errorf("TorrentHold fields = %v, want %v", got, want)
	}
	if hold["reason"] != "kept" || hold["scope"] != "movie" || hold["title"] != "Dune: Part Two" {
		t.Errorf("hold = %v", hold)
	}
	if w := object(byHash[done])["wanted"].(map[string]any); w["eta_seconds"] != nil || w["state"] != "done" {
		t.Errorf("done's wanted = %v", w)
	}
	nothing := object(byHash[frieren])
	if w, ok := nothing["wanted"]; !ok || w != nil {
		t.Errorf("wanted = %v (present %v), want null", w, ok)
	}
	if h, ok := nothing["holds"].([]any); !ok || len(h) != 0 {
		t.Errorf("holds = %v, want []", nothing["holds"])
	}

	// Each state the manager gives a torrent goes out as the web's
	// TorrentWantedState (DownloadState without no_file, unavailable and
	// storage_full), and only "downloading" puts the loaded torrent (at
	// 0 B/s) in Downloading; an ETA only comes with a speed then.
	webStates := []string{"queued", "downloading", "waiting_for_stream", "waiting_for_window", "paused", "stalled", "disk_full", "done"}
	idle := newEnv()
	st := idle.torrents.loaded[dune]
	st.DownloadSpeed = 0
	idle.torrents.loaded[dune] = st
	for _, s := range []downloads.State{downloads.StateDone, downloads.StatePaused, downloads.StateDiskFull, downloads.StateWaitingForWindow,
		downloads.StateWaitingForStream, downloads.StateStalled, downloads.StateDownloading, downloads.StateQueued} {
		idle.downloads.torrents[dune] = downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, DoneBytes: 360, State: s},
			Holds: duneHolds}
		it := idle.list(t)[dune]
		w := object(it)["wanted"].(map[string]any)
		if !slices.Contains(webStates, w["state"].(string)) || w["state"] != string(s) || w["eta_seconds"] != nil {
			t.Errorf("%s: wanted = %v", s, w)
		}
		if want := map[bool]State{true: StateDownloading, false: StateIdle}[s == downloads.StateDownloading]; it.State != want {
			t.Errorf("%s: torrent state %s, want %s", s, it.State, want)
		}
	}

	sum, err := e.svc.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(object(sum)["disk"].(map[string]any)), sorted("total_bytes", "free_bytes", "torrents_bytes", "cache_limit_bytes",
		"kept_bytes", "smart_bytes"); !slices.Equal(got, want) {
		t.Errorf("disk fields = %v, want %v", got, want)
	}
}

// A torrent's pause or resume runs a download pass before it answers, so
// its wanted.state is the new one; when the pass fails the manager is woken
// instead, and the answer is the last pass's.
func TestPause_RefreshesTheDownloads(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	refreshes := 0
	var refreshErr error
	var passCtxErr error
	e.svc.refresh = func(ctx context.Context) error {
		refreshes++
		passCtxErr = ctx.Err()
		if refreshErr != nil {
			return refreshErr
		}
		share := e.downloads.torrents[dune]
		share.Wanted.State = downloads.StateDownloading
		if _, paused := e.torrents.paused[dune]; paused {
			share.Wanted.State = downloads.StatePaused
		}
		e.downloads.torrents[dune] = share
		return nil
	}

	item, err := e.svc.Pause(ctx, dune)
	if err != nil {
		t.Fatal(err)
	}
	if item.State != StatePaused || item.Wanted == nil || item.Wanted.State != downloads.StatePaused || item.Wanted.EtaSeconds != nil ||
		refreshes != 1 || e.pauseChanges != 0 {
		t.Errorf("paused = %+v (wanted %+v), refreshes %d, wakes %d", item, item.Wanted, refreshes, e.pauseChanges)
	}
	if item, err = e.svc.Resume(ctx, dune); err != nil || item.Wanted.State != downloads.StateDownloading || refreshes != 2 {
		t.Errorf("resumed = %+v (wanted %+v), %v, refreshes %d", item, item.Wanted, err, refreshes)
	}

	refreshErr = errors.New("db down")
	if item, err = e.svc.Pause(ctx, dune); err != nil || item.State != StatePaused || item.Wanted.State != downloads.StateDownloading ||
		e.pauseChanges != 1 {
		t.Errorf("pass failed: %+v (wanted %+v), %v, wakes %d; want the pause, the last pass's state and a wake", item, item.Wanted, err,
			e.pauseChanges)
	}

	// Pause-all and resume-all answer counts: they only wake the manager.
	if _, err := e.svc.ResumeAll(ctx); err != nil || refreshes != 3 || e.pauseChanges != 2 {
		t.Errorf("resume-all: %v, refreshes %d, wakes %d", err, refreshes, e.pauseChanges)
	}

	// The client went away: the pause is stored, and the pass still runs
	// to its end (it writes local watches).
	refreshErr = nil
	gone, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.svc.Pause(gone, dune); err != nil || refreshes != 4 || passCtxErr != nil {
		t.Errorf("pause, client gone: %v, refreshes %d, the pass's context %v; want it not cancelled", err, refreshes, passCtxErr)
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
	// What downloads pin is the snapshot's usage (#77), within the cache's
	// fill.
	if d := sum.Disk; d == nil || *d != (Disk{TotalBytes: 100 << 30, FreeBytes: 40 << 30, TorrentsBytes: 1 << 30, CacheLimitBytes: 4 << 30,
		KeptBytes: 3 << 29, SmartBytes: 1 << 28}) {
		t.Errorf("disk = %+v", d)
	}
	if sum.Streaming == nil {
		t.Error("streaming is null")
	}

	// Without downloads nothing is pinned, and only bytes arriving make a
	// torrent downloading.
	e.svc.dl = nil
	if sum, _ = e.svc.Summary(context.Background()); sum.Disk == nil || sum.Disk.KeptBytes != 0 || sum.Disk.SmartBytes != 0 ||
		sum.Counts.Downloading != 1 {
		t.Errorf("without downloads: disk %+v, counts %+v", sum.Disk, sum.Counts)
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
	// Each pause and resume woke the download manager (#77).
	if e.pauseChanges != 5 {
		t.Errorf("pause changes = %d, want 5", e.pauseChanges)
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

// Pause all stops the download queue too (#77): the torrents a download
// still wants bytes of, queued or waiting, loaded or not. Otherwise the
// paused ones give up their slots and the manager loads the next. One
// whose downloads are done, or that nothing wants, isn't touched. Resume
// all brings them all back.
func TestPauseAll_StopsTheDownloadQueue(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	e.downloads.torrents[frieren] = downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, State: downloads.StateQueued}}
	e.downloads.torrents[legacy] = downloads.TorrentShare{Wanted: downloads.TorrentWanted{Bytes: 900, State: downloads.StateWaitingForWindow}}

	n, err := e.svc.PauseAll(ctx)
	if err != nil || n != 4 {
		t.Fatalf("PauseAll = %d, %v; want streaming, downloading, queued and waiting", n, err)
	}
	for _, h := range []string{severance, dune, frieren, legacy} {
		if _, ok := e.store.paused[h]; !ok {
			t.Errorf("%s: pause not stored", h[:1])
		}
		if _, ok := e.torrents.paused[h]; !ok {
			t.Errorf("%s: not paused in the client", h[:1])
		}
	}
	for _, h := range []string{done, inspect} {
		if _, ok := e.torrents.paused[h]; ok {
			t.Errorf("%s: paused, but no download wants bytes of it", h[:1])
		}
	}
	if e.pauseChanges != 1 {
		t.Errorf("wakes = %d, want 1", e.pauseChanges)
	}

	if n, err = e.svc.ResumeAll(ctx); err != nil || n != 5 {
		t.Fatalf("ResumeAll = %d, %v; want the four and the one paused before", n, err)
	}
	if len(e.torrents.paused) != 0 || len(e.store.paused) != 0 {
		t.Errorf("left paused: client %v, store %v", e.torrents.paused, e.store.paused)
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
