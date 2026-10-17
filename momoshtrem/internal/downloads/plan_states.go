package downloads

import (
	"cmp"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Phase 2 of the planner: what the piece cache and the client say of each
// torrent with admitted files, then each torrent's state and slot and each
// download's view. Pure, like phase 1 (planner.go).

// torrentWant is a torrent with admitted files, from phase 1.
type torrentWant struct {
	hash string
	// magnet loads it: one of its assignments'.
	magnet string
	// files are its admitted files' sizes, by path.
	files map[string]int64
	// order is its first download's place in the plan.
	order int
}

// torrentWants groups the admitted files by torrent.
func torrentWants(st fileStage) map[string]*torrentWant {
	out := map[string]*torrentWant{}
	for i, cov := range st.covs {
		for _, w := range cov.wants {
			if !st.adm.admitted[cov.d.ID][w.key] {
				continue
			}
			t := out[w.key.Hash]
			if t == nil {
				t = &torrentWant{hash: w.key.Hash, files: map[string]int64{}, order: i}
				out[w.key.Hash] = t
			}
			if t.magnet == "" {
				t.magnet = w.magnet
			}
			t.files[w.key.Path] = w.size
		}
	}
	return out
}

// torrentFacts are what a pass read of a torrent with admitted files.
type torrentFacts struct {
	// done is each admitted file's bytes on disk, complete whether all of
	// it is (the piece cache's). Without a piece map yet, nothing is.
	done     map[string]int64
	complete map[string]bool
	paused   bool
	streamed bool
	// speed is its smoothed download speed; 0 when it isn't loaded.
	speed int64
	// measure is the stall's progress measure: on disk and arrived.
	measure int64
	// holder says it held a slot after the last pass; stall is its history.
	holder bool
	stall  stall
}

// torrentPlan is a torrent's part of the plan.
type torrentPlan struct {
	want  *torrentWant
	bytes int64
	done  int64
	// complete: every admitted file is on disk.
	complete bool
	state    State
	slot     bool
	stall    stall
	// speed is its download speed while downloading, else 0.
	speed int64
	// resumesAt is the next window's start or the next retry.
	resumesAt *time.Time
}

// stateInput is what phase 2 reads besides phase 1's answer.
type stateInput struct {
	now   time.Time
	holds holds
	facts map[string]*torrentFacts
}

// planTorrents decides each torrent's state and slot.
func planTorrents(wants map[string]*torrentWant, in stateInput) map[string]*torrentPlan {
	var ins []torrentIn
	out := map[string]*torrentPlan{}
	for hash, w := range wants {
		f := in.facts[hash]
		if f == nil {
			f = &torrentFacts{}
		}
		p := &torrentPlan{want: w, complete: true, speed: f.speed}
		for path, size := range w.files {
			p.bytes += size
			p.done += min(f.done[path], size)
			if !f.complete[path] {
				p.complete = false
			}
		}
		p.stall = stepStall(f.stall, f.measure, in.now)
		out[hash] = p
		ins = append(ins, torrentIn{hash: hash, done: p.complete, paused: f.paused, streamed: f.streamed,
			order: w.order, holder: f.holder && !p.stall.slotSince.IsZero(), stall: p.stall})
	}
	slots := assignSlots(ins, in.holds, in.now)
	for _, t := range ins {
		p := out[t.hash]
		p.slot = slots[t.hash]
		p.state = torrentState(t, in.holds, in.now, p.slot)
		if p.slot {
			if p.stall.slotSince.IsZero() {
				p.stall.slotSince = in.now
			}
		} else {
			p.stall.slotSince = time.Time{}
		}
		switch p.state {
		case StateWaitingForWindow:
			next := in.holds.nextWindow.UTC()
			p.resumesAt = &next
		case StateStalled:
			retry := p.stall.retryAt.UTC()
			p.resumesAt = &retry
		}
		if p.state != StateDownloading {
			p.speed = 0
		}
	}
	return out
}

// downloadView is a download's part of the plan: its wire shape, and its
// admitted files' state (before storage_full).
type downloadView struct {
	cov      *coverage
	download Download
	admitted State
}

// viewDownload builds a download's wire shape from its coverage, the
// admission, its torrents and the holds on every download.
func viewDownload(cov *coverage, adm admission, torrents map[string]*torrentPlan, facts map[string]*torrentFacts, h holds) downloadView {
	d := cov.d
	v := downloadView{cov: cov}
	dl := Download{
		ID: d.ID, Reason: d.Reason, Scope: d.Scope, Item: wireItem(d.Title),
		UntilWatched: d.UntilWatched, ExpiresAt: utcPtr(d.ExpiresAt), CreatedAt: d.CreatedAt.UTC(),
	}
	if d.SmartSource != "" {
		source := d.SmartSource
		dl.SmartSource = &source
	}
	admitted := adm.admitted[d.ID]
	// Per torrent, whether its own admitted files there are all on disk:
	// then that torrent counts as done for it, whatever others want.
	complete := map[string]bool{}
	for _, w := range cov.wants {
		if !admitted[w.key] {
			continue
		}
		var done int64
		var whole bool
		if f := facts[w.key.Hash]; f != nil {
			done, whole = min(f.done[w.key.Path], w.size), f.complete[w.key.Path]
		}
		dl.WantedBytes += w.size
		dl.DoneBytes += done
		if whole {
			dl.Files.Done++
		}
		if prev, seen := complete[w.key.Hash]; !seen || prev {
			complete[w.key.Hash] = whole
		}
	}
	var states []State
	var resumes *time.Time
	for hash, whole := range complete {
		t := torrents[hash]
		if whole || t == nil {
			states = append(states, StateDone)
			continue
		}
		states = append(states, t.state)
		dl.DownloadBps += t.speed
		if t.resumesAt != nil && (resumes == nil || t.resumesAt.Before(*resumes)) {
			resumes = t.resumesAt
		}
	}
	v.admitted = combineStates(states)
	if len(states) == 0 {
		// Nothing admitted (all over the cap): the holds on every download
		// still come first (paused, then disk_full, then storage_full).
		switch {
		case h.disabled:
			v.admitted = StatePaused
		case h.diskFull:
			v.admitted = StateDiskFull
		}
	}
	dl.Files.Total, dl.Files.Unavailable, dl.Files.OverCap = cov.total, cov.unavailable, len(adm.overCap[d.ID])
	dl.State = downloadState(cov.total, len(cov.wants), dl.Files.OverCap, v.admitted)
	switch dl.State {
	case StateDownloading:
		dl.EtaSeconds = etaSeconds(dl.WantedBytes-dl.DoneBytes, dl.DownloadBps)
	case StateStalled, StateWaitingForWindow:
		dl.ResumesAt = resumes
	}
	if dl.State != StateDownloading {
		dl.DownloadBps = 0
	}
	v.download = dl
	return v
}

func wireItem(t library.DownloadTitle) Item {
	it := Item{MediaType: t.MediaType, TMDBID: t.TMDBID, LibraryID: t.LibraryID, Title: t.Title,
		SeasonNumber: t.SeasonNumber, EpisodeNumber: t.EpisodeNumber}
	if t.PosterPath != "" {
		poster := t.PosterPath
		it.PosterPath = &poster
	}
	if t.EpisodeNumber != nil && t.EpisodeName != "" {
		name := t.EpisodeName
		it.EpisodeName = &name
	}
	return it
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// listOrder is GET /downloads' order: kept first, newest first; then smart
// picks by show and episode.
func listOrder(a, b Download) int {
	if a.Reason != b.Reason {
		if a.Reason == library.DownloadKept {
			return -1
		}
		return 1
	}
	if a.Reason == library.DownloadKept {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	}
	num := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	return cmp.Or(cmp.Compare(a.Item.LibraryID, b.Item.LibraryID),
		cmp.Compare(num(a.Item.SeasonNumber), num(b.Item.SeasonNumber)),
		cmp.Compare(num(a.Item.EpisodeNumber), num(b.Item.EpisodeNumber)), cmp.Compare(a.ID, b.ID))
}
