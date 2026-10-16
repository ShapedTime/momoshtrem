package downloads

import (
	"cmp"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// The planner: pure rules over what a pass read (PLAN §6 "Downloads
// (#77)"). A pass plans in two phases with the piece cache read between
// them: planFiles decides releases, what each download covers and wants,
// and what the caps admit; planStates takes what's on disk and what the
// client says, and decides each torrent's and each download's state, the
// slots and the views. The manager does the reading and the applying.

// The manager's rules.
const (
	// MaxSlots is how many torrents download at a time.
	MaxSlots = 2
	// StallAfter is how long a torrent in a slot may go without progress
	// before it's stalled: its slot goes to the next one.
	StallAfter = 10 * time.Minute
	// StreamGrace is how long background downloads still wait after the
	// last stream ended.
	StreamGrace = time.Minute
	// SmartLifetime is a smart pick's maximum.
	SmartLifetime = 14 * 24 * time.Hour
	// stallBackoff is the wait before a stalled torrent's first retry; it
	// doubles with each stall, up to stallBackoffMax.
	stallBackoff    = 5 * time.Minute
	stallBackoffMax = time.Hour
)

// ---- Phase 1: releases, coverage and admission -----------------------------

// fileInput is what phase 1 reads.
type fileInput struct {
	now time.Time
	loc *time.Location
	// smart says whether smart picks count (smart.enabled): without it
	// they're neither planned nor listed, but still expire.
	smart bool
	// keptMax and smartMax are the effective caps.
	keptMax, smartMax int64
	downloads         []library.ResolvedDownload
	// pieces returns a torrent's piece map, path one of its files; nil
	// while it's unknown (never loaded and not stored).
	pieces func(hash, path string) *PieceMap
	// streaming are the episodes streaming now, per show, for the smart
	// rule "a later episode of the same show streams".
	streaming map[int64][]episodeNumber
}

type episodeNumber struct{ season, episode int }

func (a episodeNumber) after(b episodeNumber) bool {
	return a.season > b.season || (a.season == b.season && a.episode > b.episode)
}

// fileKind is how an item's file can be downloaded.
type fileKind int

const (
	kindNone        fileKind = iota // nothing assigned
	kindTorrent                     // a torrent file the cache can name
	kindHDRezka                     // streams from the CDN
	kindUnavailable                 // a pure v2 torrent's, or a path the torrent hasn't got
)

// coveredItem is an item a download covers now, with its file.
type coveredItem struct {
	ref     library.ItemRef
	number  episodeNumber
	kind    fileKind
	key     FileKey // kindTorrent and kindUnavailable torrent files
	watched bool    // since the download was created (until watched releases it)
}

// fileWant is a torrent file a download wants, before admission.
type fileWant struct {
	key    FileKey
	magnet string
	size   int64
	// joined is when the file joined the download: its assignment or the
	// download, the later. Files are admitted in that order, so one
	// assigned later is the one over the cap.
	joined time.Time
	// number is its first item's, for episode order.
	number episodeNumber
}

// coverage is what one download covers now.
type coverage struct {
	d *library.ResolvedDownload
	// items are its items, watched ones of an until-watched download
	// left out (released one by one).
	items []coveredItem
	// wants are its downloadable torrent files, each once.
	wants []fileWant
	// total counts every file it covers, each once; unavailable those
	// that can't be downloaded (HDRezka, pure v2).
	total, unavailable int
	// released are torrent files watched items released (until watched),
	// which no other item of it wants: their pieces are demoted.
	released []FileKey
}

// fileStage is phase 1's answer.
type fileStage struct {
	releases []library.DownloadRelease
	// released are the downloads released now, for their files' demotion.
	released []*coverage
	// covs are the downloads planned, kept by creation, then smart picks.
	covs []*coverage
	adm  admission
}

// planFiles decides releases, coverage and admission.
func planFiles(in fileInput) fileStage {
	var st fileStage
	var kept, smart []*coverage
	for i := range in.downloads {
		d := &in.downloads[i]
		cov := cover(d, in)
		if reason, ok := releaseFor(d, in.now, in.loc, in.streaming); ok {
			st.releases = append(st.releases, library.DownloadRelease{ID: d.ID, Reason: reason})
			if reason == library.ReleaseWatched {
				st.released = append(st.released, cov)
			}
			continue
		}
		switch {
		case d.Reason == library.DownloadKept:
			kept = append(kept, cov)
		case in.smart:
			smart = append(smart, cov)
		}
	}
	byCreation := func(a, b *coverage) int {
		return cmp.Or(a.d.CreatedAt.Compare(b.d.CreatedAt), cmp.Compare(a.d.ID, b.d.ID))
	}
	slices.SortFunc(kept, byCreation)
	slices.SortFunc(smart, byCreation)
	st.covs = append(kept, smart...)
	st.adm = admit(st.covs, in.keptMax, in.smartMax)
	return st
}

// watchedSince reports whether the item was watched after t.
func watchedSince(it library.CoveredItem, t time.Time) bool {
	return it.WatchedAt != nil && it.WatchedAt.After(t)
}

// watched reports whether a download counts the item as watched: a play
// after the download was created; for a smart pick, also a later episode
// of the show streaming.
func watched(d *library.ResolvedDownload, it library.CoveredItem, streaming map[int64][]episodeNumber) bool {
	if watchedSince(it, d.CreatedAt) {
		return true
	}
	if d.Reason != library.DownloadSmart || it.ItemType != library.ItemTypeEpisode {
		return false
	}
	n := episodeNumber{it.SeasonNumber, it.EpisodeNumber}
	return slices.ContainsFunc(streaming[d.Title.LibraryID], func(s episodeNumber) bool { return s.after(n) })
}

// cover works out what a download covers now. HDRezka files and pure v2
// torrents' can't be downloaded; with until watched, watched items leave
// it, and a file leaves once every item of it that wants it is watched.
func cover(d *library.ResolvedDownload, in fileInput) *coverage {
	cov := &coverage{d: d}
	byKey := map[FileKey]int{} // wants index
	unavailable := map[FileKey]bool{}
	releasedKeys := map[FileKey]bool{}
	hdrezka := 0
	for _, it := range d.Items {
		ci := coveredItem{
			ref:     library.ItemRef{Type: it.ItemType, ID: it.ItemID},
			number:  episodeNumber{it.SeasonNumber, it.EpisodeNumber},
			watched: watched(d, it, in.streaming),
		}
		f := it.File
		switch {
		case f == nil:
			ci.kind = kindNone
		case f.Kind == library.AssignmentKindHDRezka:
			ci.kind = kindHDRezka
		default:
			ci.key = FileKey{Hash: f.InfoHash, Path: f.FilePath}
			ci.kind = kindTorrent
			if pm := in.pieces(f.InfoHash, f.FilePath); pm != nil {
				if _, ok := pm.File(f.FilePath); !ok || !pm.V1() {
					ci.kind = kindUnavailable
				}
			}
		}
		if d.UntilWatched && ci.watched {
			if ci.kind == kindTorrent {
				releasedKeys[ci.key] = true
			}
			continue
		}
		cov.items = append(cov.items, ci)
		switch ci.kind {
		case kindHDRezka:
			hdrezka++
		case kindUnavailable:
			unavailable[ci.key] = true
		case kindTorrent:
			if _, ok := byKey[ci.key]; ok {
				continue
			}
			byKey[ci.key] = len(cov.wants)
			joined := d.CreatedAt
			if f.AssignedAt.After(joined) {
				joined = f.AssignedAt
			}
			cov.wants = append(cov.wants, fileWant{key: ci.key, magnet: f.MagnetURI, size: f.FileSize,
				joined: joined, number: ci.number})
		}
	}
	for key := range releasedKeys {
		if _, wanted := byKey[key]; !wanted {
			cov.released = append(cov.released, key)
		}
	}
	slices.SortFunc(cov.released, compareKeys)
	cov.unavailable = hdrezka + len(unavailable)
	cov.total = len(cov.wants) + cov.unavailable
	return cov
}

func compareKeys(a, b FileKey) int {
	return cmp.Or(cmp.Compare(a.Hash, b.Hash), cmp.Compare(a.Path, b.Path))
}

// releaseFor says whether an active download ends now, and why: its
// expires_at passed; or, until watched, its scope is watched (a movie or
// an episode; a season once it has also finished airing; a show once it
// has ended).
func releaseFor(d *library.ResolvedDownload, now time.Time, loc *time.Location, streaming map[int64][]episodeNumber) (library.ReleaseReason, bool) {
	if d.ExpiresAt != nil && !now.Before(*d.ExpiresAt) {
		return library.ReleaseExpired, true
	}
	if !d.UntilWatched || len(d.Items) == 0 {
		return "", false
	}
	allWatched := func(items []library.CoveredItem) bool {
		for _, it := range items {
			if !watched(d, it, streaming) {
				return false
			}
		}
		return len(items) > 0
	}
	switch d.Scope {
	case library.DownloadScopeMovie, library.DownloadScopeEpisode:
		if allWatched(d.Items) {
			return library.ReleaseWatched, true
		}
	case library.DownloadScopeSeason:
		if seasonFinished(d, now.In(loc).Format(time.DateOnly)) && allWatched(d.Items) {
			return library.ReleaseWatched, true
		}
	case library.DownloadScopeShow:
		var regular []library.CoveredItem
		for _, it := range d.Items {
			if it.SeasonNumber > 0 {
				regular = append(regular, it)
			}
		}
		if showEnded(d.Title.ShowStatus) && allWatched(regular) {
			return library.ReleaseWatched, true
		}
	}
	return "", false
}

// showEnded reads TMDB's status: the show won't air again.
func showEnded(status string) bool { return status == "Ended" || status == "Canceled" }

// seasonFinished says a season has finished airing: every episode it
// lists has aired by today, and it's over (the show has a later season or
// has ended, or its last episode is the finale).
func seasonFinished(d *library.ResolvedDownload, today string) bool {
	if len(d.Items) == 0 {
		return false
	}
	for _, it := range d.Items {
		if it.AirDate == "" || it.AirDate > today {
			return false
		}
	}
	last := d.Items[len(d.Items)-1]
	return d.Title.LaterSeason || showEnded(d.Title.ShowStatus) || last.EpisodeType == "finale"
}

// admission is what the caps admit.
type admission struct {
	// kept and smart are the admitted files: kept ones, and smart ones no
	// kept download has.
	kept, smart map[FileKey]bool
	// admitted and overCap are each download's files, by download id.
	admitted, overCap map[int64]map[FileKey]bool
	// committedKept is the size of the admitted kept files, each once
	// (committed_kept_bytes); committedSmart the smart ones'.
	committedKept, committedSmart int64
	sizes                         map[FileKey]int64
}

// admit admits files against the caps: kept downloads' first, then smart
// picks', each in the order they joined (a file assigned later comes
// last), then by download and episode order. A file another download has
// costs nothing. A file that doesn't fit is over the cap; a later, smaller
// one may still fit. Smart never takes kept room.
func admit(covs []*coverage, keptMax, smartMax int64) admission {
	a := admission{
		kept: map[FileKey]bool{}, smart: map[FileKey]bool{},
		admitted: map[int64]map[FileKey]bool{}, overCap: map[int64]map[FileKey]bool{},
		sizes: map[FileKey]int64{},
	}
	type candidate struct {
		cov  *coverage
		want fileWant
	}
	queue := func(reason library.DownloadReason) []candidate {
		var out []candidate
		for _, cov := range covs {
			if cov.d.Reason != reason {
				continue
			}
			for _, w := range cov.wants {
				out = append(out, candidate{cov, w})
			}
		}
		slices.SortStableFunc(out, func(x, y candidate) int {
			return cmp.Or(x.want.joined.Compare(y.want.joined),
				x.cov.d.CreatedAt.Compare(y.cov.d.CreatedAt), cmp.Compare(x.cov.d.ID, y.cov.d.ID),
				cmp.Compare(x.want.number.season, y.want.number.season),
				cmp.Compare(x.want.number.episode, y.want.number.episode),
				compareKeys(x.want.key, y.want.key))
		})
		return out
	}
	mark := func(m map[int64]map[FileKey]bool, id int64, key FileKey) {
		if m[id] == nil {
			m[id] = map[FileKey]bool{}
		}
		m[id][key] = true
	}
	for _, c := range queue(library.DownloadKept) {
		key, id := c.want.key, c.cov.d.ID
		a.sizes[key] = c.want.size
		switch {
		case a.kept[key]:
		case a.committedKept+c.want.size <= keptMax:
			a.kept[key] = true
			a.committedKept += c.want.size
		default:
			mark(a.overCap, id, key)
			continue
		}
		mark(a.admitted, id, key)
	}
	for _, c := range queue(library.DownloadSmart) {
		key, id := c.want.key, c.cov.d.ID
		a.sizes[key] = c.want.size
		switch {
		case a.kept[key], a.smart[key]:
		case a.committedSmart+c.want.size <= smartMax:
			a.smart[key] = true
			a.committedSmart += c.want.size
		default:
			mark(a.overCap, id, key)
			continue
		}
		mark(a.admitted, id, key)
	}
	return a
}

// ---- Windows ----------------------------------------------------------------

// windowNow says whether now is inside a download window (always, without
// any) and, outside, when the next one starts. Windows are local times in
// loc, the server's TZ, and may cross midnight.
func windowNow(windows []library.DownloadWindow, now time.Time, loc *time.Location) (inside bool, next time.Time) {
	if len(windows) == 0 {
		return true, time.Time{}
	}
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	for _, w := range windows {
		start, ok1 := library.ClockMinutes(w.Start)
		end, ok2 := library.ClockMinutes(w.End)
		if !ok1 || !ok2 || start == end {
			continue
		}
		if start < end && minute >= start && minute < end || start > end && (minute >= start || minute < end) {
			return true, time.Time{}
		}
		at := time.Date(local.Year(), local.Month(), local.Day(), start/60, start%60, 0, 0, loc)
		if !at.After(now) {
			at = time.Date(local.Year(), local.Month(), local.Day()+1, start/60, start%60, 0, 0, loc)
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	return next.IsZero(), next
}

// ---- Stalls -----------------------------------------------------------------

// stall is a torrent's slot history, kept across passes.
type stall struct {
	// slotSince is when it took its slot; zero while it hasn't one.
	slotSince time.Time
	// progress is the last progress measure seen (bytes on disk and
	// arrived), and progressAt when it last grew.
	progress   int64
	progressAt time.Time
	// attempts counts stalls since the last progress; retryAt is when the
	// last one's wait ends.
	attempts int
	retryAt  time.Time
}

// stepStall moves s to now with the progress measure seen now: growth is
// progress (the backoff starts over); a slot held StallAfter without any
// is a stall, which gives the slot up until the backoff has passed.
func stepStall(s stall, measure int64, now time.Time) stall {
	if measure > s.progress {
		s.progressAt, s.attempts, s.retryAt = now, 0, time.Time{}
	}
	s.progress = measure
	if s.slotSince.IsZero() {
		return s
	}
	since := s.slotSince
	if s.progressAt.After(since) {
		since = s.progressAt
	}
	if now.Sub(since) >= StallAfter {
		s.attempts++
		s.retryAt = now.Add(backoff(s.attempts))
		s.slotSince = time.Time{}
	}
	return s
}

// stalled reports whether the torrent waits for its retry.
func (s stall) stalled(now time.Time) bool { return now.Before(s.retryAt) }

// backoff is the wait after the nth stall in a row: 5, 10, 20, 40 minutes,
// then an hour.
func backoff(n int) time.Duration {
	d := stallBackoff
	for i := 1; i < n && d < stallBackoffMax; i++ {
		d *= 2
	}
	return min(d, stallBackoffMax)
}

// ---- Phase 2: states ----------------------------------------------------------

// holds are the conditions that hold every background download.
type holds struct {
	// disabled is background.enabled off.
	disabled bool
	// diskFull: the floor leaves no room to fetch more.
	diskFull bool
	// outside is outside every window; nextWindow the next one's start.
	outside    bool
	nextWindow time.Time
	// stream: pause while streaming applies now (something streams, or
	// did within StreamGrace).
	stream bool
}

// torrentIn is what phase 2 knows of a torrent with admitted files.
type torrentIn struct {
	hash string
	// done says every admitted file of it is on disk.
	done bool
	// paused is the user's sticky pause.
	paused bool
	// streamed: a session plays one of its files, or did within
	// StreamGrace.
	streamed bool
	// order is its first download's place in the plan (kept by
	// creation, then smart).
	order int
	// holder says it held a slot after the last pass.
	holder bool
	stall  stall
}

// torrentState is a torrent's state, the first that matches; inSlot says
// whether it got a slot.
func torrentState(t torrentIn, h holds, now time.Time, inSlot bool) State {
	switch {
	case t.done:
		return StateDone
	case h.disabled || t.paused:
		return StatePaused
	case h.diskFull:
		return StateDiskFull
	case h.outside:
		return StateWaitingForWindow
	case h.stream && !t.streamed:
		return StateWaitingForStream
	case t.stall.stalled(now):
		return StateStalled
	case inSlot:
		return StateDownloading
	}
	return StateQueued
}

// assignSlots gives MaxSlots torrents that can download a slot: streamed
// ones first, then those holding one (a slot isn't taken away), then in
// plan order.
func assignSlots(ts []torrentIn, h holds, now time.Time) map[string]bool {
	var ready []torrentIn
	for _, t := range ts {
		if s := torrentState(t, h, now, true); s == StateDownloading {
			ready = append(ready, t)
		}
	}
	rank := func(b bool) int {
		if b {
			return 0
		}
		return 1
	}
	slices.SortFunc(ready, func(a, b torrentIn) int {
		return cmp.Or(cmp.Compare(rank(a.streamed), rank(b.streamed)), cmp.Compare(rank(a.holder), rank(b.holder)),
			cmp.Compare(a.order, b.order), cmp.Compare(a.hash, b.hash))
	})
	slots := map[string]bool{}
	for _, t := range ready[:min(len(ready), MaxSlots)] {
		slots[t.hash] = true
	}
	return slots
}

// torrentOrder is the order a download over several torrents takes its
// state from, when none is downloading and some aren't done.
var torrentOrder = []State{StatePaused, StateDiskFull, StateWaitingForWindow, StateWaitingForStream, StateStalled, StateQueued}

// combineStates is the state of a download's admitted files over their
// torrents (done ones left out): done when all are, downloading when any
// is, else the first of torrentOrder one has.
func combineStates(states []State) State {
	var open []State
	for _, s := range states {
		if s != StateDone {
			open = append(open, s)
		}
	}
	if len(open) == 0 {
		return StateDone
	}
	if slices.Contains(open, StateDownloading) {
		return StateDownloading
	}
	for _, s := range torrentOrder {
		if slices.Contains(open, s) {
			return s
		}
	}
	return StateQueued
}

// downloadState is a download's state: unavailable, no_file, done, else
// its admitted files' state (admitted), which storage_full replaces while
// files are over the cap, unless they're paused or the disk is full.
func downloadState(total, downloadable, overCap int, admitted State) State {
	switch {
	case total > 0 && downloadable == 0:
		return StateUnavailable
	case total == 0:
		return StateNoFile
	case overCap > 0 && admitted != StatePaused && admitted != StateDiskFull:
		return StateStorageFull
	}
	return admitted
}

// pausedReason is why background downloads wait now, with when they
// resume: background downloads off, outside every window (the next
// start), or pause while streaming (a minute after the last stream ended;
// none while one streams).
func pausedReason(h holds, streamResumes *time.Time) (*PausedReason, *time.Time) {
	reason := func(r PausedReason) *PausedReason { return &r }
	switch {
	case h.disabled:
		return reason(PausedDisabled), nil
	case h.outside:
		next := h.nextWindow.UTC()
		return reason(PausedWindow), &next
	case h.stream:
		return reason(PausedStreaming), streamResumes
	}
	return nil, nil
}

// etaSeconds is the time left at bps, rounded up; nil without a speed.
func etaSeconds(left, bps int64) *int64 {
	if bps <= 0 || left <= 0 {
		return nil
	}
	s := (left + bps - 1) / bps
	return &s
}

// pct is done of wanted in whole percent, 100 only when done.
func pct(done, wanted int64, complete bool) int {
	switch {
	case complete:
		return 100
	case wanted <= 0:
		return 0
	}
	return int(min(99, done*100/wanted))
}
