package downloads

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
)

// A Keep pins the movie's pieces and, in a slot, loads its torrent and sets
// its file to Normal priority. A second Keep is the same download.
func TestKeep_PinsAndPriority(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Interstellar")
	tt := h.torrent("Interstellar", true, 3*testPiece+500, testPiece) // the film, and a sample
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.fill(tt, 0) // a piece streamed before the Keep
	other := h.torrent("Other", true, 2*testPiece)
	h.fill(other, 0, 1)

	d, created := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	if !created || d.Reason != library.DownloadKept || d.Scope != library.DownloadScopeMovie || d.Item.Title != "Interstellar" {
		t.Fatalf("Keep = %+v, created %v", d, created)
	}
	if d.State != StateDownloading || d.WantedBytes != 3*testPiece+500 || d.DoneBytes != testPiece || d.Files != (Files{Total: 1}) {
		t.Errorf("Keep answered %s, %d of %d, files %+v", d.State, d.DoneBytes, d.WantedBytes, d.Files)
	}
	// The defaults: until watched, 30 days at most.
	if !d.UntilWatched || d.ExpiresAt == nil || !d.ExpiresAt.Equal(h.clock.Now().AddDate(0, 0, 30)) {
		t.Errorf("duration = %v until %v", d.UntilWatched, d.ExpiresAt)
	}

	// In a slot and not loaded: it loads; then its file goes to Normal and
	// it's kept awake. The sample isn't wanted.
	h.awaitLoad(tt)
	h.refresh()
	if !h.client.isNormal(tt.hash, tt.path(0)) || h.client.isNormal(tt.hash, tt.path(1)) || h.client.markCount(tt.hash) == 0 {
		t.Errorf("priorities %v, marks %d", h.client.normal[tt.hash], h.client.markCount(tt.hash))
	}

	// The pins: a trim to nothing keeps the film's piece, not the other's.
	h.cache.SetCapacity(0)
	h.cache.TrimToCapacity()
	if have := h.have(tt, 0); !have[0] {
		t.Error("the kept film's piece was evicted")
	}
	if have := h.have(other, 0, 1); have[0] || have[1] {
		t.Error("an unkept torrent's pieces survived the trim")
	}
	// The boundary piece (3) it shares with the sample is pinned too.
	h.fill(tt, 3)
	h.cache.TrimToCapacity()
	if have := h.have(tt, 3); !have[0] {
		t.Error("the film's last (shared) piece was evicted")
	}

	again, created := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d,"until_watched":false}`, movie))
	if created || again.ID != d.ID || !again.UntilWatched {
		t.Errorf("a second Keep = %+v created %v, want the first unchanged", again, created)
	}
	// On disk: piece 0, and the film's 500 bytes of the shared piece 3.
	if u, _ := h.svc.DownloadUsage(h.ctx); u.CommittedKeptBytes != 3*testPiece+500 || u.KeptBytes != testPiece+500 {
		t.Errorf("usage = %+v", u)
	}
}

// After a restart the pins come back before the cache trims: DeferTrim
// holds every trim until RestorePins, which then lets it go (OnRestored).
func TestRestorePins_BeforeTheFirstTrim(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Kept")
	tt := h.torrent("Kept", true, 2*testPiece)
	other := h.torrent("Cache", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	h.awaitLoad(tt)

	// The restart: a fresh cache over the same folder, deferring trims; a
	// write over its capacity trims nothing yet.
	h.fill(tt, 0, 1)
	h.fill(other, 0, 1)
	h.cache.DeferTrim()
	h.cache.SetCapacity(testPiece)
	h.cache.TrimToCapacity()
	if have := h.have(other, 0, 1); !have[0] || !have[1] {
		t.Fatal("a trim ran before the pins were restored")
	}
	h.svc = h.newService()
	h.restored = 0
	h.svc.d.OnRestored = func() { h.restored++; h.cache.Ready() }
	if err := h.svc.RestorePins(h.ctx); err != nil {
		t.Fatal(err)
	}
	if h.restored != 1 {
		t.Fatalf("OnRestored ran %d times", h.restored)
	}
	h.cache.TrimToCapacity()
	if have := h.have(tt, 0, 1); !have[0] || !have[1] {
		t.Error("the kept pieces were evicted after the restart")
	}
	if have := h.have(other, 0, 1); have[0] || have[1] {
		t.Error("the trim spared unkept pieces")
	}
	// RestorePins loads nothing.
	select {
	case hash := <-h.client.loads:
		t.Errorf("RestorePins loaded %s", hash)
	default:
	}
	// Only the first pass tells.
	h.refresh()
	if h.restored != 1 {
		t.Errorf("OnRestored ran again: %d", h.restored)
	}
}

// The kept cap: a Keep that doesn't fit is 409 with what it needs and
// what's left; episodes assigned later that don't fit are over the cap
// (storage_full), and committed counts admitted files only. A storage PUT
// can't lower the cap below what's committed.
func TestCaps(t *testing.T) {
	h := newHarness(t)
	size := int64(4 * testPiece)
	h.storage(fmt.Sprintf(`{"kept":{"max_bytes":%d}}`, 3*size/2)) // room for 1.5 films
	a, b := h.movie("A"), h.movie("B")
	ta, tb := h.torrent("A", true, size), h.torrent("B", true, size)
	h.assign(library.ItemTypeMovie, a, ta, 0)
	h.assign(library.ItemTypeMovie, b, tb, 0)
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, a))

	_, _, err := h.svc.Keep(h.ctx, []byte(fmt.Sprintf(`{"scope":"movie","id":%d}`, b)))
	var full *StorageFullError
	if !errors.As(err, &full) || full.NeededBytes != size || full.AvailableBytes != size/2 {
		t.Fatalf("Keep B = %v, want 409 needing %d with %d left", err, size, size/2)
	}
	est, err := h.svc.Estimate(h.ctx, library.DownloadTarget{Scope: library.DownloadScopeMovie, ID: b})
	if err != nil || est != (Estimate{WantedBytes: size, NeededBytes: size, AvailableBytes: size / 2, Downloadable: true}) {
		t.Errorf("Estimate(B) = %+v, %v", est, err)
	}
	if est, _ := h.svc.Estimate(h.ctx, library.DownloadTarget{Scope: library.DownloadScopeMovie, ID: a}); est.NeededBytes != 0 {
		t.Errorf("Estimate(A, kept) needs %d", est.NeededBytes)
	}

	// A show keep with E1 fits the half left; E2, assigned later, doesn't.
	show, eps := h.show("Show", 2)
	ts := h.torrent("Show", true, size/2, size/2)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 1}], ts, 0)
	d, _ := h.keep(fmt.Sprintf(`{"scope":"show","id":%d}`, show))
	if d.State == StateStorageFull || d.Files.OverCap != 0 {
		t.Fatalf("show = %s, over %d", d.State, d.Files.OverCap)
	}
	h.clock.advance(time.Minute)
	h.exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active, created_at)
		VALUES ('episode', $1, $2, $3, $4, $5, TRUE, $6)`, eps[[2]int{1, 2}], ts.hash, ts.magnet(), ts.path(1), size/2, h.clock.Now())
	d = h.download(d.ID)
	if d.State != StateStorageFull || d.Files != (Files{Total: 2, OverCap: 1}) || d.WantedBytes != size/2 {
		t.Errorf("after E2: %s, files %+v, wanted %d", d.State, d.Files, d.WantedBytes)
	}
	if k, ok := h.svc.Snapshot().Episode(eps[[2]int{1, 2}]); !ok || k.State != StateStorageFull {
		t.Errorf("E2's keep = %+v %v", k, ok)
	}
	if u, _ := h.svc.DownloadUsage(h.ctx); u.CommittedKeptBytes != size+size/2 {
		t.Errorf("committed = %d, want the film and E1", u.CommittedKeptBytes)
	}

	// Lowering the kept cap below what's committed: 409, through the
	// governor (which holds the commit guard).
	lower := size
	_, err = h.gov.UpdateStorage(h.ctx, library.StorageSettingsPatch{Kept: library.KeptSettingsPatch{SetMaxBytes: true, MaxBytes: &lower}})
	var committed *storagecap.CommittedError
	if !errors.As(err, &committed) || committed.CommittedKeptBytes != size+size/2 {
		t.Errorf("PUT kept.max_bytes below committed = %v", err)
	}

	// More room: E2 is admitted.
	h.storage(fmt.Sprintf(`{"kept":{"max_bytes":%d}}`, 3*size))
	if d = h.download(d.ID); d.State == StateStorageFull || d.Files.OverCap != 0 || d.WantedBytes != size {
		t.Errorf("with room: %s, files %+v", d.State, d.Files)
	}
}

// Outside every window downloads wait, until the next window's start.
func TestWindows(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Night")
	tt := h.torrent("Night", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.client.set(func(c *fakeClient) { c.loaded[tt.hash] = 0 })
	h.storage(`{"background":{"windows":[{"start":"23:00","end":"06:00"}]}}`) // it's 12:00 UTC
	d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	want := time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC)
	if d.State != StateWaitingForWindow || d.ResumesAt == nil || !d.ResumesAt.Equal(want) {
		t.Errorf("state %s resumes %v, want waiting until %v", d.State, d.ResumesAt, want)
	}
	list := h.svc.Snapshot().List()
	if list.PausedReason == nil || *list.PausedReason != PausedWindow || list.ResumesAt == nil || !list.ResumesAt.Equal(want) {
		t.Errorf("list paused %v until %v", list.PausedReason, list.ResumesAt)
	}
	if h.client.isNormal(tt.hash, tt.path(0)) || h.client.markCount(tt.hash) != 0 {
		t.Error("a waiting download fetches")
	}
	h.clock.advance(11 * time.Hour) // 23:00
	if d = h.download(d.ID); d.State != StateDownloading || !h.client.isNormal(tt.hash, tt.path(0)) {
		t.Errorf("inside the window: %s", d.State)
	}
}

// While anything streams, other torrents' downloads wait (their files back
// to no priority, no wakes); the streamed torrent's carry on. They resume a
// minute after the last stream ended.
func TestPauseWhileStreaming(t *testing.T) {
	h := newHarness(t)
	a, b := h.movie("A"), h.movie("B")
	ta, tb := h.torrent("A", true, 2*testPiece), h.torrent("B", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, a, ta, 0)
	h.assign(library.ItemTypeMovie, b, tb, 0)
	h.client.set(func(c *fakeClient) { c.loaded[ta.hash], c.loaded[tb.hash] = 0, 0 })
	da, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, a))
	db, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, b))
	if !h.client.isNormal(ta.hash, ta.path(0)) || !h.client.isNormal(tb.hash, tb.path(0)) {
		t.Fatal("both don't download before the stream")
	}

	h.sessions.set(session("s1", tb, 0, testPiece, 2*testPiece))
	if poll := h.svc.poll(h.ctx); !poll {
		t.Error("a stream starting didn't call for a pass")
	}
	marks := h.client.markCount(ta.hash)
	if da = h.download(da.ID); da.State != StateWaitingForStream {
		t.Errorf("A = %s, want waiting for the stream", da.State)
	}
	if db = h.download(db.ID); db.State != StateDownloading {
		t.Errorf("B (streaming) = %s, want downloading", db.State)
	}
	if h.client.isNormal(ta.hash, ta.path(0)) || !h.client.isNormal(tb.hash, tb.path(0)) || h.client.markCount(ta.hash) != marks {
		t.Error("A still fetches while B streams")
	}
	list := h.svc.Snapshot().List()
	if list.PausedReason == nil || *list.PausedReason != PausedStreaming || list.ResumesAt != nil {
		t.Errorf("paused %v resumes %v, want streaming with no time yet", list.PausedReason, list.ResumesAt)
	}

	// The stream ends: a minute's grace from the last poll that saw it.
	h.clock.advance(10 * time.Second)
	h.svc.poll(h.ctx)
	ended := h.clock.Now()
	h.clock.advance(PollInterval)
	h.sessions.set()
	h.svc.poll(h.ctx)
	if da = h.download(da.ID); da.State != StateWaitingForStream {
		t.Errorf("A just after the stream = %s", da.State)
	}
	if list := h.svc.Snapshot().List(); list.ResumesAt == nil || !list.ResumesAt.Equal(ended.Add(StreamGrace)) {
		t.Errorf("resumes at %v, want %v", list.ResumesAt, ended.Add(StreamGrace))
	}
	h.clock.advance(StreamGrace - PollInterval)
	if poll := h.svc.poll(h.ctx); !poll {
		t.Error("the grace ending didn't call for a pass")
	}
	if da = h.download(da.ID); da.State != StateDownloading || !h.client.isNormal(ta.hash, ta.path(0)) {
		t.Errorf("A a minute later = %s", da.State)
	}

	// Off, nothing waits.
	h.storage(`{"background":{"pause_while_streaming":false}}`)
	h.sessions.set(session("s2", tb, 0, testPiece, 2*testPiece))
	if da = h.download(da.ID); da.State != StateDownloading {
		t.Errorf("without pause while streaming, A = %s", da.State)
	}
}

// A paused torrent's downloads are paused: no priority, no wakes; resumed,
// they go on. Background downloads off pause everything.
func TestStickyPauseAndKillSwitch(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Paused")
	tt := h.torrent("Paused", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.client.set(func(c *fakeClient) { c.loaded[tt.hash] = 0 })
	d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	h.client.set(func(c *fakeClient) { c.paused[tt.hash] = true })
	marks := h.client.markCount(tt.hash)
	if d = h.download(d.ID); d.State != StatePaused || h.client.isNormal(tt.hash, tt.path(0)) || h.client.markCount(tt.hash) != marks {
		t.Errorf("paused: %s, normal %v, marks %d → %d", d.State, h.client.isNormal(tt.hash, tt.path(0)), marks, h.client.markCount(tt.hash))
	}
	if share, ok := h.svc.Snapshot().Torrent(tt.hash); !ok || share.Wanted.State != StatePaused || len(share.Holds) != 1 {
		t.Errorf("torrent share = %+v %v", share, ok)
	}
	// A paused download's file isn't downloading (/library's set, P6).
	if got := h.svc.Snapshot().DownloadingFiles(); len(got) != 0 {
		t.Errorf("paused: downloading files = %v, want none", got)
	}
	h.client.set(func(c *fakeClient) { delete(c.paused, tt.hash) })
	if d = h.download(d.ID); d.State != StateDownloading {
		t.Errorf("resumed: %s", d.State)
	}
	if got := h.svc.Snapshot().DownloadingFiles(); !slices.Equal(got, []FileKey{{tt.hash, tt.path(0)}}) {
		t.Errorf("resumed: downloading files = %v, want the movie's file", got)
	}

	h.storage(`{"background":{"enabled":false}}`)
	if d = h.download(d.ID); d.State != StatePaused || h.client.isNormal(tt.hash, tt.path(0)) {
		t.Errorf("background off: %s", d.State)
	}
	if list := h.svc.Snapshot().List(); list.PausedReason == nil || *list.PausedReason != PausedDisabled || list.ResumesAt != nil {
		t.Errorf("list paused %v", list.PausedReason)
	}
}

// Two slots; one stalled for 10 minutes gives its slot up and is retried
// after 5; progress starts the backoff over.
func TestStalledRotation(t *testing.T) {
	h := newHarness(t)
	var torrents []testTorrent
	var ids []int64
	for i, name := range []string{"A", "B", "C"} {
		movie := h.movie(name)
		tt := h.torrent(name, true, 2*testPiece)
		h.assign(library.ItemTypeMovie, movie, tt, 0)
		h.client.set(func(c *fakeClient) { c.loaded[tt.hash] = 0 })
		d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
		torrents, ids = append(torrents, tt), append(ids, d.ID)
		h.clock.advance(time.Duration(i+1) * time.Second)
	}
	states := func() []State {
		h.refresh()
		var out []State
		for _, id := range ids {
			d, _ := h.svc.Snapshot().Download(id)
			out = append(out, d.State)
		}
		return out
	}
	want := func(got []State, want ...State) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("states = %v, want %v", got, want)
		}
	}
	want(states(), StateDownloading, StateDownloading, StateQueued)

	// A makes progress; B doesn't.
	h.clock.advance(5 * time.Minute)
	h.client.set(func(c *fakeClient) { c.progress[torrents[0].hash] = 100 })
	want(states(), StateDownloading, StateDownloading, StateQueued)
	h.clock.advance(5 * time.Minute)
	got := states()
	want(got, StateDownloading, StateStalled, StateDownloading)
	b, _ := h.svc.Snapshot().Download(ids[1])
	if b.ResumesAt == nil || !b.ResumesAt.Equal(h.clock.Now().Add(5*time.Minute)) {
		t.Errorf("B retries at %v", b.ResumesAt)
	}
	if h.client.isNormal(torrents[1].hash, torrents[1].path(0)) || !h.client.isNormal(torrents[2].hash, torrents[2].path(0)) {
		t.Error("the stalled torrent kept its priority, or C didn't get it")
	}

	// B's retry: C and A hold their slots, so B queues.
	h.clock.advance(5 * time.Minute)
	h.client.set(func(c *fakeClient) { c.progress[torrents[0].hash], c.progress[torrents[2].hash] = 200, 50 })
	want(states(), StateDownloading, StateQueued, StateDownloading)
}

// Removing a title removes its downloads (the FK) and their pins; a torrent
// whose assignments are gone leaves its downloads with no file.
func TestCascades(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Gone")
	tm := h.torrent("Gone", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tm, 0)
	h.fill(tm, 0, 1)
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	show, eps := h.show("Show", 2)
	ts := h.torrent("Show", true, testPiece, testPiece)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 1}], ts, 0)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 2}], ts, 1)
	h.fill(ts, 0, 1)
	sd, _ := h.keep(fmt.Sprintf(`{"scope":"show","id":%d}`, show))
	if info := h.cache.Info(); info.Pinned != 4*testPiece {
		t.Fatalf("pinned = %d", info.Pinned)
	}

	h.exec(`DELETE FROM movies WHERE id = $1`, movie)
	h.refresh()
	if info := h.cache.Info(); info.Pinned != 2*testPiece {
		t.Errorf("after the movie went, pinned = %d", info.Pinned)
	}
	if len(h.svc.Snapshot().List().Items) != 1 {
		t.Errorf("listed %d downloads", len(h.svc.Snapshot().List().Items))
	}

	// The torrent's DELETE deactivates its assignments: the show keep
	// stays, with no file, and pins nothing.
	h.exec(`UPDATE torrent_assignments SET is_active = FALSE WHERE info_hash = $1`, ts.hash)
	if d := h.download(sd.ID); d.State != StateNoFile || d.Files.Total != 0 {
		t.Errorf("show = %s, files %+v", d.State, d.Files)
	}
	if info := h.cache.Info(); info.Pinned != 0 {
		t.Errorf("pinned = %d, want nothing", info.Pinned)
	}
}

// A local session at 90% for 20 s is a watch here: an until-watched movie
// keep ends (released watched); a show keep lets go of the episode's file
// alone, and demotes its pieces.
func TestLocalWatches(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Watched")
	tm := h.torrent("Watched", true, 4*testPiece)
	h.assign(library.ItemTypeMovie, movie, tm, 0)
	md, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	show, eps := h.show("Show", 2)
	ts := h.torrent("Show", true, 2*testPiece, 2*testPiece)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 1}], ts, 0)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 2}], ts, 1)
	h.fill(ts, 0, 1, 2, 3)
	sd, _ := h.keep(fmt.Sprintf(`{"scope":"show","id":%d}`, show))
	h.clock.advance(time.Hour) // the watches come after the Keeps

	// Reading the index at the end, then nothing more, isn't a watch: the
	// session sits at 100% until it expires.
	read := func(id string, tt testTorrent, file int, offset, size int64) sessions.Session {
		s := session(id, tt, file, offset, size)
		s.LastReadAt = h.clock.Now()
		return s
	}
	index := read("m", tm, 0, 4*testPiece, 4*testPiece)
	for range 6 {
		h.sessions.set(index)
		h.svc.poll(h.ctx)
		h.clock.advance(PollInterval)
	}
	h.refresh()
	if _, ok := h.svc.Snapshot().Download(md.ID); !ok {
		t.Fatal("an index read released the movie")
	}

	// Reading on from 90% through the end for 20 s: watched.
	h.clock.advance(time.Minute)
	h.sessions.set(read("m2", tm, 0, 4*testPiece*91/100, 4*testPiece), read("e", ts, 0, 2*testPiece*91/100, 2*testPiece))
	h.svc.poll(h.ctx)
	h.clock.advance(watchConfirm)
	h.sessions.set(read("m2", tm, 0, 4*testPiece, 4*testPiece), read("e", ts, 0, 2*testPiece, 2*testPiece))
	if poll := h.svc.poll(h.ctx); !poll {
		t.Error("a watch didn't call for a pass")
	}
	var n int
	if err := h.db.QueryRow(`SELECT count(*) FROM local_watches`).Scan(&n); err != nil || n != 2 {
		t.Errorf("local watches = %d, %v", n, err)
	}
	h.sessions.set()
	h.refresh()
	if _, ok := h.svc.Snapshot().Download(md.ID); ok {
		t.Error("the watched movie is still kept")
	}
	var reason string
	if err := h.db.QueryRow(`SELECT release_reason FROM downloads WHERE id = $1`, md.ID).Scan(&reason); err != nil || reason != "watched" {
		t.Errorf("release = %q, %v", reason, err)
	}
	d, ok := h.svc.Snapshot().Download(sd.ID)
	if !ok || d.Files.Total != 1 || d.WantedBytes != 2*testPiece {
		t.Errorf("show after E1 = %+v %v", d, ok)
	}
	if _, ok := h.svc.Snapshot().Episode(eps[[2]int{1, 1}]); ok {
		t.Error("the watched episode is still held")
	}
	// E1's pieces are unpinned and go first; E2's stay.
	h.cache.SetCapacity(2 * testPiece)
	h.cache.TrimToCapacity()
	if have := h.have(ts, 0, 1, 2, 3); have[0] || have[1] || !have[2] || !have[3] {
		t.Errorf("after the trim, have %v; want E2's alone", have)
	}
}

// Smart picks: listed while smart downloads are on, no duration of their
// own (PATCH 422), a Keep turns one kept (created now); a DELETE dismisses
// it for good.
func TestSmartPicks(t *testing.T) {
	h := newHarness(t)
	h.storage(`{"smart":{"enabled":true}}`)
	show, eps := h.show("Show", 3)
	ts := h.torrent("Show", true, testPiece, testPiece, testPiece)
	for e := 1; e <= 3; e++ {
		h.assign(library.ItemTypeEpisode, eps[[2]int{1, e}], ts, e-1)
	}
	id, created, err := h.svc.UpsertSmart(h.ctx, eps[[2]int{1, 2}], library.SmartUpNext)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	d := h.download(id)
	if d.Reason != library.DownloadSmart || d.SmartSource == nil || *d.SmartSource != library.SmartUpNext ||
		!d.UntilWatched || d.ExpiresAt == nil || !d.ExpiresAt.Equal(h.clock.Now().Add(SmartLifetime)) {
		t.Errorf("smart pick = %+v", d)
	}
	if _, err := h.svc.Update(h.ctx, id, []byte(`{"until_watched":false}`)); !errors.Is(err, ErrSmartPick) {
		t.Errorf("PATCH a smart pick: %v", err)
	}

	// Kept: the same row, created now.
	h.clock.advance(time.Hour)
	k, created := h.keep(fmt.Sprintf(`{"scope":"episode","id":%d,"expires_at":null}`, eps[[2]int{1, 2}]))
	if created || k.ID != id || k.Reason != library.DownloadKept || k.SmartSource != nil || !k.CreatedAt.Equal(h.clock.Now()) || k.ExpiresAt != nil {
		t.Errorf("Keep on the pick = %+v (created %v)", k, created)
	}

	// Another pick, dismissed: never picked again.
	other, _, err := h.svc.UpsertSmart(h.ctx, eps[[2]int{1, 3}], library.SmartStreaming)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Delete(h.ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.svc.UpsertSmart(h.ctx, eps[[2]int{1, 3}], library.SmartUpNext); !errors.Is(err, ErrRepickBlocked) {
		t.Errorf("re-pick of a dismissed episode: %v", err)
	}
	if err := h.svc.Delete(h.ctx, other); !errors.Is(err, ErrNotFound) {
		t.Errorf("DELETE a released pick: %v", err)
	}

	// With smart downloads off, picks aren't listed, planned or found.
	third, _, _ := h.svc.UpsertSmart(h.ctx, eps[[2]int{1, 1}], library.SmartUpNext)
	h.storage(`{"smart":{"enabled":false}}`)
	h.refresh()
	if _, ok := h.svc.Snapshot().Download(third); ok {
		t.Error("a smart pick is listed with smart downloads off")
	}
	if err := h.svc.Delete(h.ctx, third); !errors.Is(err, ErrNotFound) {
		t.Errorf("DELETE a hidden pick: %v", err)
	}
	_ = show
}

// POST's and PATCH's checks, in the contract's order.
func TestKeepAndUpdate_Errors(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Film")
	show, eps := h.show("Rezka", 1)
	h.exec(`INSERT INTO torrent_assignments (item_type, item_id, kind, stream_ref, file_path, file_size, is_active)
		VALUES ('episode', $1, 'hdrezka', '{"provider":"hdrezka"}', 'hdrezka/1/t1/s01e01/1080p.mp4', 0, TRUE)`, eps[[2]int{1, 1}])
	past := h.clock.Now().Add(-time.Hour).Format(time.RFC3339)
	for _, tt := range []struct {
		body  string
		field string
		err   error
	}{
		{`[1]`, "", ErrNotAnObject},
		{`{"scope":"film","id":1}`, "scope", nil},
		{`{"scope":"movie","id":"1"}`, "id", nil},
		{`{"scope":"movie","id":0}`, "id", nil},
		{fmt.Sprintf(`{"scope":"season","id":%d}`, show), "season_number", nil},
		{fmt.Sprintf(`{"scope":"movie","id":%d,"until_watched":"yes"}`, movie), "until_watched", nil},
		{fmt.Sprintf(`{"scope":"movie","id":%d,"expires_at":"tomorrow"}`, movie), "expires_at", nil},
		{fmt.Sprintf(`{"scope":"movie","id":%d,"expires_at":%q}`, movie, past), "expires_at", nil},
		{`{"scope":"movie","id":999999}`, "", ErrTargetNotFound},
		{fmt.Sprintf(`{"scope":"season","id":%d,"season_number":4}`, show), "", ErrTargetNotFound},
		{fmt.Sprintf(`{"scope":"show","id":%d}`, show), "", ErrNotDownloadable},
	} {
		_, _, err := h.svc.Keep(h.ctx, []byte(tt.body))
		var field *FieldError
		switch {
		case tt.field != "":
			if !errors.As(err, &field) || field.Field != tt.field {
				t.Errorf("Keep(%s) = %v, want a 422 on %s", tt.body, err, tt.field)
			}
		case !errors.Is(err, tt.err):
			t.Errorf("Keep(%s) = %v, want %v", tt.body, err, tt.err)
		}
	}

	// A Keep with nothing assigned is fine: no file yet.
	d, created := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d,"until_watched":false,"expires_at":null}`, movie))
	if !created || d.State != StateNoFile || d.UntilWatched || d.ExpiresAt != nil {
		t.Errorf("a Keep with nothing assigned = %+v", d)
	}
	future := h.clock.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	for _, tt := range []struct {
		id    int64
		body  string
		field string
		err   error
	}{
		{999999, `{}`, "", ErrNotFound},
		{d.ID, `"x"`, "", ErrNotAnObject},
		{d.ID, `{"until_watched":null}`, "until_watched", nil},
		{d.ID, `{"expires_at":5}`, "expires_at", nil},
	} {
		_, err := h.svc.Update(h.ctx, tt.id, []byte(tt.body))
		var field *FieldError
		switch {
		case tt.field != "":
			if !errors.As(err, &field) || field.Field != tt.field {
				t.Errorf("Update(%s) = %v, want a 422 on %s", tt.body, err, tt.field)
			}
		case !errors.Is(err, tt.err):
			t.Errorf("Update(%s) = %v, want %v", tt.body, err, tt.err)
		}
	}
	u, err := h.svc.Update(h.ctx, d.ID, []byte(fmt.Sprintf(`{"until_watched":true,"expires_at":%q}`, future.Format(time.RFC3339))))
	if err != nil || !u.UntilWatched || u.ExpiresAt == nil || !u.ExpiresAt.Equal(future) {
		t.Errorf("Update = %+v, %v", u, err)
	}
	if err := h.svc.Delete(h.ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Update(h.ctx, d.ID, []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Errorf("PATCH a released download: %v", err)
	}
}

// A change that ends a download: a Keep on an until-watched movie just
// watched is a new Keep (201); a PATCH that makes a watched movie until
// watched answers its final view (200), and the next GET doesn't list it.
func TestKeepAndUpdate_Releases(t *testing.T) {
	h := newHarness(t)
	seen := func(movie int64) {
		h.t.Helper()
		h.clock.advance(time.Hour)
		if err := h.repo.UpsertLocalWatch(h.ctx, library.ItemRef{Type: library.ItemTypeMovie, ID: movie}, h.clock.Now()); err != nil {
			t.Fatal(err)
		}
		h.clock.advance(time.Minute)
	}
	reason := func(id int64) string {
		var r *string
		if err := h.db.QueryRow(`SELECT release_reason FROM downloads WHERE id = $1`, id).Scan(&r); err != nil {
			t.Fatal(err)
		}
		if r == nil {
			return ""
		}
		return *r
	}

	a := h.movie("Seen")
	ta := h.torrent("Seen", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, a, ta, 0)
	first, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, a)) // until watched
	seen(a)
	again, created := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, a))
	if !created || again.ID == first.ID || reason(first.ID) != "watched" {
		t.Errorf("Keep after the watch = %d (created %v); the first is released %q", again.ID, created, reason(first.ID))
	}

	b := h.movie("Rewatch")
	tb := h.torrent("Rewatch", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, b, tb, 0)
	h.fill(tb, 0, 1)
	kept, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d,"until_watched":false}`, b))
	seen(b)
	if d := h.download(kept.ID); d.UntilWatched {
		t.Fatal("kept until removed reads until watched")
	}
	got, err := h.svc.Update(h.ctx, kept.ID, []byte(`{"until_watched":true}`))
	if err != nil {
		t.Fatalf("PATCH that ends the download: %v", err)
	}
	if got.ID != kept.ID || !got.UntilWatched || got.State != StateDone || got.WantedBytes != 2*testPiece || got.Files.Done != 1 {
		t.Errorf("final view = %+v", got)
	}
	if reason(kept.ID) != "watched" {
		t.Errorf("released %q, want watched", reason(kept.ID))
	}
	if _, ok := h.svc.Snapshot().Download(kept.ID); ok {
		t.Error("the ended download is still listed")
	}
}

// A Keep's own release of a watched download demotes its pieces: they go
// before older ones when the cache trims.
func TestKeep_DemotesWhatItReleases(t *testing.T) {
	h := newHarness(t)
	older := h.torrent("Older", true, 2*testPiece)
	h.fill(older, 0, 1)
	time.Sleep(10 * time.Millisecond)
	movie := h.movie("Seen")
	tt := h.torrent("Seen", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.fill(tt, 0, 1) // newer than Older's
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	h.clock.advance(time.Hour)
	if err := h.repo.UpsertLocalWatch(h.ctx, library.ItemRef{Type: library.ItemTypeMovie, ID: movie}, h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	// Another Keep: its pre-pass releases the watched movie.
	other := h.movie("Other")
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, other))
	h.cache.SetCapacity(2 * testPiece)
	h.cache.TrimToCapacity()
	if have := h.have(tt, 0, 1); have[0] || have[1] {
		t.Errorf("the released, watched movie's pieces survived the trim: %v", have)
	}
	if have := h.have(older, 0, 1); !have[0] || !have[1] {
		t.Errorf("older pieces went first: %v", have)
	}
}

// The estimate: what a show's Keep wants, a file another kept download has
// counts nothing, HDRezka can't be kept.
func TestEstimate(t *testing.T) {
	h := newHarness(t)
	show, eps := h.show("Show", 2)
	ts := h.torrent("Show", true, testPiece, 2*testPiece)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 1}], ts, 0)
	h.assign(library.ItemTypeEpisode, eps[[2]int{1, 2}], ts, 1)
	h.keep(fmt.Sprintf(`{"scope":"episode","id":%d}`, eps[[2]int{1, 2}]))
	est, err := h.svc.Estimate(h.ctx, library.DownloadTarget{Scope: library.DownloadScopeSeason, ID: show, SeasonNumber: 1})
	_, limits, _ := h.gov.Limits(h.ctx)
	if err != nil || est.WantedBytes != 3*testPiece || est.NeededBytes != testPiece || !est.Downloadable ||
		est.AvailableBytes != limits.KeptMaxBytes-2*testPiece {
		t.Errorf("Estimate(season) = %+v, %v", est, err)
	}
	if _, err := h.svc.Estimate(h.ctx, library.DownloadTarget{Scope: library.DownloadScopeShow, ID: 999999}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("Estimate(unknown) = %v", err)
	}
}

// The disk's floor: no room to fetch more is disk_full.
func TestDiskFull(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Big")
	tt := h.torrent("Big", true, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.disk.mu.Lock()
	h.disk.free = 6 * gib // the cache is held at its 4 GiB minimum, all of it the reserve
	h.disk.mu.Unlock()
	if d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie)); d.State != StateDiskFull {
		t.Errorf("state = %s, want disk_full", d.State)
	}
}

// The F11 re-sync: an admitted piece the cache lost while the client still
// believes it complete is read again.
func TestResync(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Lost")
	tt := h.torrent("Lost", true, 3*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.fill(tt, 0, 2)
	h.client.set(func(c *fakeClient) {
		c.loaded[tt.hash] = 0
		c.complete[tt.hash] = []bool{true, true, true} // piece 1 was evicted before the Keep
	})
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	if got := h.client.updated[tt.hash]; fmt.Sprint(got) != "[1]" {
		t.Errorf("re-read %v, want piece 1", got)
	}
	h.refresh()
	if got := h.client.updated[tt.hash]; fmt.Sprint(got) != "[1]" {
		t.Errorf("re-read again: %v", got)
	}
}

// A Keep released and made again while its torrent stays loaded (torrents
// don't unload until a DELETE or a restart): the pieces the cache lost while
// nothing pinned them are re-read on the new Keep's first pass.
func TestResync_ReKeep(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Again")
	tt := h.torrent("Again", true, 3*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.fill(tt, 0, 1, 2)
	h.client.set(func(c *fakeClient) {
		c.loaded[tt.hash] = 0
		c.complete[tt.hash] = []bool{true, true, true}
	})
	d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	if err := h.svc.Delete(h.ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	// Unpinned, the LRU evicts piece 1; the client still believes it complete.
	if err := h.cache.Remove("completed/" + tt.pm.Ref(1).Hex); err != nil {
		t.Fatal(err)
	}
	h.refresh()
	h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	h.refresh()
	if got := h.client.updated[tt.hash]; fmt.Sprint(got) != "[1]" {
		t.Errorf("after a Keep made again, re-read %v, want piece 1", got)
	}
}

// A torrent nothing stored loads first, then its pieces are known (from
// the client) and pinned.
func TestUnstoredMetainfo(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Legacy")
	tt := h.torrent("Legacy", false, 2*testPiece)
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.fill(tt, 0)
	d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	if d.State != StateDownloading || d.DoneBytes != 0 {
		t.Errorf("before the load: %s, %d done", d.State, d.DoneBytes)
	}
	h.awaitLoad(tt)
	if d = h.download(d.ID); d.DoneBytes != testPiece {
		t.Errorf("after the load: %d done", d.DoneBytes)
	}
	if info := h.cache.Info(); info.Pinned != testPiece {
		t.Errorf("pinned = %d", info.Pinned)
	}
}

// The loop: a Wake runs a pass.
func TestLoop(t *testing.T) {
	h := newHarness(t)
	movie := h.movie("Looped")
	tt := h.torrent("Looped", true, 2*testPiece)
	h.svc.Start()
	t.Cleanup(h.svc.Close)
	d, _ := h.keep(fmt.Sprintf(`{"scope":"movie","id":%d}`, movie))
	if d.State != StateNoFile {
		t.Fatalf("state = %s", d.State)
	}
	h.assign(library.ItemTypeMovie, movie, tt, 0)
	h.svc.Wake()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if got, _ := h.svc.Snapshot().Download(d.ID); got.State == StateDownloading {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a Wake never ran a pass")
		}
	}
	var _ sessions.Session
}
