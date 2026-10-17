package downloads

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
)

// Smart picks over the integration harness, without Trakt: the streaming
// signal alone. smart_trakt_test.go has Up next, through the Trakt
// sync.

// smartRow is a smart pick's row: its episode, source and how it ended
// ("" while active).
type smartRow struct {
	Episode int
	Source  library.SmartSource
	Release library.ReleaseReason
}

// smartRows are the show's smart picks' rows, oldest first, with episodes
// named by number (season 1).
func (h *harness) smartRows() []smartRow {
	h.t.Helper()
	rows, err := h.db.Query(`
		SELECT e.episode_number, COALESCE(d.smart_source, ''), COALESCE(d.release_reason, '')
		FROM downloads d JOIN episodes e ON e.id = d.episode_id
		WHERE d.reason = 'smart' ORDER BY d.id`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []smartRow
	for rows.Next() {
		var r smartRow
		if err := rows.Scan(&r.Episode, &r.Source, &r.Release); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func (h *harness) wantSmartRows(when string, want ...smartRow) {
	h.t.Helper()
	if got := h.smartRows(); !reflect.DeepEqual(got, want) {
		h.t.Errorf("%s: smart rows = %v, want %v", when, got, want)
	}
}

// streamEpisode is a session on the show's S01E{e}, file e-1 of tt.
func streamEpisode(id string, tt testTorrent, show int64, e int) sessions.Session {
	s := session(id, tt, e-1, 0, testPiece)
	season, episode := 1, e
	s.Item = &sessions.Item{MediaType: "tv", LibraryID: show, Title: "Show", SeasonNumber: &season, EpisodeNumber: &episode}
	return s
}

// smartShow is a show of n episodes, each a file (a piece) of one
// torrent.
func (h *harness) smartShow(n int) (int64, testTorrent) { return h.smartShowNamed("Show", n) }

func (h *harness) smartShowNamed(title string, n int) (int64, testTorrent) {
	show, eps := h.show(title, n)
	sizes := make([]int64, n)
	for i := range sizes {
		sizes[i] = testPiece
	}
	tt := h.torrent(title, true, sizes...)
	for e := 1; e <= n; e++ {
		h.assign(library.ItemTypeEpisode, eps[[2]int{1, e}], tt, e-1)
	}
	return show, tt
}

func (h *harness) runSmart(p *SmartPicker) {
	h.t.Helper()
	if err := p.Run(h.ctx); err != nil {
		h.t.Fatal(err)
	}
	h.refresh()
}

func nudged(p *SmartPicker) bool {
	select {
	case <-p.nudge:
		return true
	default:
		return false
	}
}

// Without Trakt the episodes after the one streaming make the picks. A pick
// outlives its stream; a later episode streaming releases the earlier
// ones (watched); the episode streaming now stays picked. A new episode
// streaming nudges a pass; smart downloads off, nothing moves.
func TestSmart_StreamingWithoutTrakt(t *testing.T) {
	h := newHarness(t)
	h.storage(`{"smart":{"enabled":true,"episodes_ahead":2}}`)
	show, tt := h.smartShow(6)
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo})

	h.runSmart(picker)
	h.wantSmartRows("nothing streams")

	h.sessions.set(streamEpisode("s1", tt, show, 1))
	h.svc.poll(h.ctx)
	if !nudged(picker) {
		t.Error("an episode starting to stream didn't nudge the smart picks")
	}
	h.runSmart(picker)
	h.wantSmartRows("streaming E1",
		smartRow{2, library.SmartStreaming, ""}, smartRow{3, library.SmartStreaming, ""})
	list := h.svc.Snapshot().List()
	if len(list.Items) != 2 || list.Items[0].Reason != library.DownloadSmart || *list.Items[0].SmartSource != library.SmartStreaming ||
		*list.Items[0].Item.EpisodeNumber != 2 || !list.Items[0].UntilWatched {
		t.Errorf("listed = %+v", list.Items)
	}
	h.svc.poll(h.ctx)
	if nudged(picker) {
		t.Error("the same stream nudged again")
	}

	// The stream ends: the picks stay.
	h.clock.advance(time.Minute)
	h.sessions.set()
	h.svc.poll(h.ctx)
	h.runSmart(picker)
	h.wantSmartRows("after the stream",
		smartRow{2, library.SmartStreaming, ""}, smartRow{3, library.SmartStreaming, ""})

	// E2 streams: it stays picked, and E4 joins.
	h.clock.advance(time.Hour)
	h.sessions.set(streamEpisode("s2", tt, show, 2))
	h.svc.poll(h.ctx)
	h.runSmart(picker)
	h.wantSmartRows("streaming E2",
		smartRow{2, library.SmartStreaming, ""}, smartRow{3, library.SmartStreaming, ""}, smartRow{4, library.SmartStreaming, ""})

	// A jump to E4: the planner releases E2 and E3 (a later episode
	// streams), and E5 and E6 join.
	h.sessions.set(streamEpisode("s3", tt, show, 4))
	h.svc.poll(h.ctx)
	h.refresh()
	h.runSmart(picker)
	h.wantSmartRows("streaming E4",
		smartRow{2, library.SmartStreaming, library.ReleaseWatched}, smartRow{3, library.SmartStreaming, library.ReleaseWatched},
		smartRow{4, library.SmartStreaming, ""}, smartRow{5, library.SmartStreaming, ""}, smartRow{6, library.SmartStreaming, ""})

	// E4 paused (its session ends), with one episode ahead now: the show
	// keeps the paused episode and the one after it, and E6 goes.
	h.storage(`{"smart":{"episodes_ahead":1}}`)
	h.clock.advance(time.Minute)
	h.sessions.set()
	h.svc.poll(h.ctx)
	h.refresh()
	h.runSmart(picker)
	paused := []smartRow{
		{2, library.SmartStreaming, library.ReleaseWatched}, {3, library.SmartStreaming, library.ReleaseWatched},
		{4, library.SmartStreaming, ""}, {5, library.SmartStreaming, ""}, {6, library.SmartStreaming, library.ReleaseExpired},
	}
	h.wantSmartRows("E4 paused, one ahead", paused...)

	// Resumed, E4 finds E5 picked still.
	h.sessions.set(streamEpisode("s4", tt, show, 4))
	h.svc.poll(h.ctx)
	h.runSmart(picker)
	h.wantSmartRows("E4 resumed", paused...)

	// Smart downloads off: nothing's picked (E6, released early, could be
	// picked again with two ahead), and nothing's released.
	h.storage(`{"smart":{"enabled":false,"episodes_ahead":2}}`)
	h.runSmart(picker)
	h.wantSmartRows("smart downloads off", paused...)
	h.storage(`{"smart":{"enabled":true}}`)
	h.runSmart(picker)
	h.wantSmartRows("on again", append(paused, smartRow{6, library.SmartStreaming, ""})...)
}

// A pick the smart cap can't fit isn't made; one a Keep holds isn't
// either. A dismissed pick isn't picked again.
func TestSmart_RoomKeepsAndDismissals(t *testing.T) {
	h := newHarness(t)
	h.storage(fmt.Sprintf(`{"smart":{"enabled":true,"episodes_ahead":3,"max_bytes":%d}}`, 2*testPiece))
	show, tt := h.smartShow(6)
	h.keep(fmt.Sprintf(`{"scope":"season","id":%d,"season_number":1}`, show))
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo})
	h.sessions.set(streamEpisode("s1", tt, show, 1))
	h.svc.poll(h.ctx)
	h.runSmart(picker)
	h.wantSmartRows("the season is kept")

	// Without the Keep, the cap (two files) takes E2 and E3 of E2–E4.
	var keep int64
	if err := h.db.QueryRow(`SELECT id FROM downloads WHERE reason = 'kept'`).Scan(&keep); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Delete(h.ctx, keep); err != nil {
		t.Fatal(err)
	}
	h.runSmart(picker)
	h.wantSmartRows("two fit", smartRow{2, library.SmartStreaming, ""}, smartRow{3, library.SmartStreaming, ""})

	// Dismissed, E3 isn't picked again; its room goes to E4.
	var e3 int64
	if err := h.db.QueryRow(`SELECT d.id FROM downloads d JOIN episodes e ON e.id = d.episode_id
		WHERE d.released_at IS NULL AND e.episode_number = 3`).Scan(&e3); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Delete(h.ctx, e3); err != nil {
		t.Fatal(err)
	}
	h.runSmart(picker)
	h.wantSmartRows("E3 dismissed", smartRow{2, library.SmartStreaming, ""},
		smartRow{3, library.SmartStreaming, library.ReleaseDeleted}, smartRow{4, library.SmartStreaming, ""})
}

// The loop runs a pass at Start and on Nudge, until Close.
func TestSmart_Loop(t *testing.T) {
	h := newHarness(t)
	h.storage(`{"smart":{"enabled":true}}`)
	show, tt := h.smartShow(3)
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo})
	picker.Start()
	defer picker.Close()
	h.sessions.set(streamEpisode("s1", tt, show, 1))
	h.svc.poll(h.ctx)
	deadline := time.Now().Add(5 * time.Second)
	for len(h.smartRows()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the nudged pass never picked: %v", h.smartRows())
		}
		time.Sleep(10 * time.Millisecond)
	}
	picker.Close()
	picker.Close() // twice is fine
}

// A play that syncs after the pick was made, of a time before it, never
// counts for the manager (watched means after created_at): the smart pass
// releases the pick watched, and its pieces go first when the cache trims.
func TestSmart_LatePlayReleasedWatched(t *testing.T) {
	h := newHarness(t)
	h.storage(`{"smart":{"enabled":true,"episodes_ahead":2}}`)
	other := h.torrent("Other", true, 2*testPiece)
	h.fill(other, 0, 1)
	show, tt := h.smartShow(3)
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo})
	h.sessions.set(streamEpisode("s1", tt, show, 1))
	h.svc.poll(h.ctx)
	h.runSmart(picker)
	h.fill(tt, 1) // E2, after the other torrent's pieces
	h.sessions.set()
	h.svc.poll(h.ctx)

	h.exec(`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, plays, last_watched_at)
		SELECT tmdb_id, 1, 2, 1, $2 FROM shows WHERE id = $1`, show, h.clock.Now().Add(-time.Hour))
	h.clock.advance(time.Minute)
	h.runSmart(picker)
	h.wantSmartRows("E2 played before it was picked",
		smartRow{2, library.SmartStreaming, library.ReleaseWatched}, smartRow{3, library.SmartStreaming, ""})

	h.cache.SetCapacity(2 * testPiece)
	h.cache.TrimToCapacity()
	if have, others := h.have(tt, 1), h.have(other, 0, 1); have[0] || !others[0] || !others[1] {
		t.Errorf("after the trim E2 %v, the other torrent %v; want E2's piece evicted first", have, others)
	}
}

// fixedUpNext is an Up next that doesn't change.
type fixedUpNext []UpNextShow

func (f fixedUpNext) UpNextShows(context.Context) ([]UpNextShow, error) { return f, nil }

// picksByShow are the smart picks' rows as "Show E1 source[ release]", by
// show and episode.
func (h *harness) picksByShow() string {
	h.t.Helper()
	rows, err := h.db.Query(`
		SELECT s.title, e.episode_number, d.smart_source, COALESCE(d.release_reason, '')
		FROM downloads d JOIN episodes e ON e.id = d.episode_id JOIN seasons sn ON sn.id = e.season_id
		JOIN shows s ON s.id = sn.show_id
		WHERE d.reason = 'smart' ORDER BY s.title, e.episode_number, d.id`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var title, source, release string
		var episode int
		if err := rows.Scan(&title, &episode, &source, &release); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, strings.TrimSpace(fmt.Sprintf("%s E%d %s %s", title, episode, source, release)))
	}
	return strings.Join(out, ", ")
}

// A stream's pick that doesn't fit the smart cap takes the room of Up
// next's picks of other shows, the least recently watched first, but never
// one all on disk of a show watched within 7 days. Up next's released
// picks then wait (they don't fit) while the stream's hold the room.
func TestSmart_StreamTakesUpNextRoom(t *testing.T) {
	h := newHarness(t)
	h.storage(fmt.Sprintf(`{"smart":{"enabled":true,"episodes_ahead":2,"max_bytes":%d}}`, 3*testPiece))
	recent, older := h.clock.Now().Add(-3*24*time.Hour), h.clock.Now().Add(-10*24*time.Hour)
	alpha, ta := h.smartShowNamed("Alpha", 3)
	bravo, _ := h.smartShowNamed("Bravo", 3)
	charlie, tc := h.smartShowNamed("Charlie", 3)
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo, UpNext: fixedUpNext{
		{ShowID: alpha, SeasonNumber: 1, EpisodeNumber: 1, LastWatchedAt: &recent},
		{ShowID: bravo, SeasonNumber: 1, EpisodeNumber: 1, LastWatchedAt: &older},
	}})

	h.runSmart(picker)
	upNext := "Alpha E1 up_next, Alpha E2 up_next, Bravo E1 up_next"
	if got := h.picksByShow(); got != upNext {
		t.Fatalf("Up next's picks = %s, want %s (the cap fits three)", got, upNext)
	}
	h.fill(ta, 0) // Alpha E1 is on disk
	h.refresh()

	h.sessions.set(streamEpisode("s1", tc, charlie, 1))
	h.svc.poll(h.ctx)
	h.runSmart(picker)
	want := "Alpha E1 up_next, Alpha E2 up_next expired, Bravo E1 up_next expired, " +
		"Charlie E2 streaming, Charlie E3 streaming"
	if got := h.picksByShow(); got != want {
		t.Errorf("with Charlie streaming = %s\nwant                  %s", got, want)
	}
	h.runSmart(picker)
	if got := h.picksByShow(); got != want {
		t.Errorf("a pass later = %s\nwant          %s", got, want)
	}
}

// Day 14: Up next's picks run their lifetime while the show is still in
// Up next's 21 days. Nobody played it since, so they're held back and keep
// their slots: nothing new is picked for the show until a play.
func TestSmart_LifetimeHoldsTheWindow(t *testing.T) {
	h := newHarness(t)
	h.storage(`{"smart":{"enabled":true,"episodes_ahead":2}}`)
	show, _ := h.smartShowNamed("Alpha", 5)
	watched := h.clock.Now().Add(-24 * time.Hour)
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo, UpNext: fixedUpNext{
		{ShowID: show, SeasonNumber: 1, EpisodeNumber: 1, LastWatchedAt: &watched},
	}})
	h.runSmart(picker)
	if got, want := h.picksByShow(), "Alpha E1 up_next, Alpha E2 up_next"; got != want {
		t.Fatalf("picks = %s, want %s", got, want)
	}

	h.clock.advance(SmartLifetime)
	h.refresh() // the manager releases them: their lifetime ran out
	h.runSmart(picker)
	expired := "Alpha E1 up_next expired, Alpha E2 up_next expired"
	if got := h.picksByShow(); got != expired {
		t.Errorf("day 14 = %s, want %s (nothing new)", got, expired)
	}

	// A play of the show after (E1 watched here): E1 isn't a candidate, E2
	// can be picked again, and E3 joins.
	h.clock.advance(time.Minute)
	h.exec(`INSERT INTO local_watches (episode_id, watched_at)
		SELECT e.id, $2 FROM episodes e JOIN seasons sn ON sn.id = e.season_id WHERE sn.show_id = $1 AND e.episode_number = 1`,
		show, h.clock.Now())
	h.runSmart(picker)
	if got, want := h.picksByShow(), expired+", Alpha E2 up_next, Alpha E3 up_next"; got != want {
		t.Errorf("after a play = %s\nwant          %s", got, want)
	}
}

// A storage change makes the last pass's room stale: the next smart pass
// refreshes first, so the picks fit the new cap.
func TestSmart_StorageChangeRefreshesTheRoom(t *testing.T) {
	h := newHarness(t)
	h.storage(`{"smart":{"enabled":true,"episodes_ahead":2}}`)
	show, tt := h.smartShow(3)
	picker := NewSmartPicker(h.svc, SmartDeps{Store: h.repo})
	h.refresh() // the room as the old cap leaves it

	h.storage(fmt.Sprintf(`{"smart":{"max_bytes":%d}}`, testPiece))
	h.svc.StorageChanged()
	if !nudged(picker) {
		t.Error("a storage change didn't nudge the smart picks")
	}
	h.sessions.set(streamEpisode("s1", tt, show, 1))
	h.svc.poll(h.ctx) // reads the sessions; no pass
	if err := picker.Run(h.ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := h.picksByShow(), "Show E2 streaming"; got != want {
		t.Errorf("picks = %s, want %s (the new cap fits one)", got, want)
	}
}
