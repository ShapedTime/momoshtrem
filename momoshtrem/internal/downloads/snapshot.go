package downloads

import (
	"cmp"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Snapshot is what the downloads look like after the last pass: what the
// read paths show (title pages, /library, Up next and Activity, P6 #204).
// It's built once per pass, never changed after, and read without any
// lock: Service.Snapshot returns the latest. Every method is safe on a nil
// Snapshot (before the first pass), answering nothing.
type Snapshot struct {
	at       time.Time
	list     List
	byID     map[int64]int // index into list.Items
	movies   map[int64]Keep
	shows    map[int64]Keep
	seasons  map[seasonKey]Keep
	episodes map[int64]Keep
	files    map[FileKey]FileStatus
	// downloading are the keys of the files that are downloading, by hash
	// and path (DownloadingFiles).
	downloading []FileKey
	torrents    map[string]TorrentShare
	usage       Usage
}

type seasonKey struct {
	show   int64
	season int
}

// FileStatus is what downloads want of one torrent file.
type FileStatus struct {
	// Bytes is the file's size, DoneBytes what's on disk of it (the piece
	// cache's), Complete whether all of it is.
	Bytes     int64
	DoneBytes int64
	Complete  bool
	// State is its torrent's (TorrentWanted.state): done once the file is
	// complete.
	State State
}

// Downloading is PLAN §6's "a file is downloading": a download wants it
// (every FileStatus is one that's wanted: admitted, not over the cap) and
// it isn't all on disk, unless its download is paused.
func (f FileStatus) Downloading() bool { return !f.Complete && f.State.Wants() }

// Pct is what's on disk of the file, 0–100.
func (f FileStatus) Pct() int { return pct(f.DoneBytes, f.Bytes, f.Complete) }

// TorrentShare is what downloads want of a torrent: B6's
// TorrentListItem.wanted and holds.
type TorrentShare struct {
	Wanted TorrentWanted
	// Holds are the downloads that want it, kept first, then by id.
	Holds []TorrentHold
}

// At is when the pass that built it ran; zero for a nil Snapshot.
func (s *Snapshot) At() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.at
}

// List is GET /downloads. Its slices are shared: don't change them.
func (s *Snapshot) List() List {
	if s == nil {
		return List{Items: []Download{}}
	}
	return s.list
}

// Download is the active download with the id, as GET /downloads lists
// it; false when it isn't listed (unknown, released, or a smart pick while
// smart downloads are off).
func (s *Snapshot) Download(id int64) (Download, bool) {
	if s == nil {
		return Download{}, false
	}
	i, ok := s.byID[id]
	if !ok {
		return Download{}, false
	}
	return s.list.Items[i], true
}

// Movie is the movie's download (TitleLibraryState.keep for a movie).
func (s *Snapshot) Movie(movieID int64) (Keep, bool) {
	if s == nil {
		return Keep{}, false
	}
	k, ok := s.movies[movieID]
	return k, ok
}

// Show is the show's show-scope download (TitleLibraryState.keep for a
// show).
func (s *Snapshot) Show(showID int64) (Keep, bool) {
	if s == nil {
		return Keep{}, false
	}
	k, ok := s.shows[showID]
	return k, ok
}

// Season is the season-scope download of the show's season (the season
// page's keep).
func (s *Snapshot) Season(showID int64, season int) (Keep, bool) {
	if s == nil {
		return Keep{}, false
	}
	k, ok := s.seasons[seasonKey{showID, season}]
	return k, ok
}

// Episode is what holds the episode (EpisodeSummary.keep): its own
// download, its season's, its show's, or a smart pick; kept wins over
// smart, then the narrowest scope. State and Pct are the episode's own
// file's; the rest is the download's. An episode an until-watched show or
// season keep released (watched) isn't held by it.
func (s *Snapshot) Episode(episodeID int64) (Keep, bool) {
	if s == nil {
		return Keep{}, false
	}
	k, ok := s.episodes[episodeID]
	return k, ok
}

// File is what downloads want of a torrent file; false when no download
// wants it (none covers it, or it's over the cap). The hash is lowercase
// hex.
func (s *Snapshot) File(infoHash, path string) (FileStatus, bool) {
	if s == nil {
		return FileStatus{}, false
	}
	f, ok := s.files[FileKey{infoHash, path}]
	return f, ok
}

// Torrent is what downloads want of a torrent; false when none does. The
// hash is lowercase hex.
func (s *Snapshot) Torrent(infoHash string) (TorrentShare, bool) {
	if s == nil {
		return TorrentShare{}, false
	}
	t, ok := s.torrents[infoHash]
	return t, ok
}

// Usage is what downloads hold now: on disk, committed and the caps.
func (s *Snapshot) Usage() Usage {
	if s == nil {
		return Usage{}
	}
	return s.usage
}

// snapshotInput is a pass's plan, for building its snapshot.
type snapshotInput struct {
	at       time.Time
	stage    fileStage
	torrents map[string]*torrentPlan
	facts    map[string]*torrentFacts
	views    []downloadView
	usage    Usage
	reason   *PausedReason
	resumes  *time.Time
}

// narrowness ranks scopes for an episode's keep: its own first.
var narrowness = map[library.DownloadScope]int{
	library.DownloadScopeEpisode: 0, library.DownloadScopeSeason: 1, library.DownloadScopeShow: 2,
}

func buildSnapshot(in snapshotInput) *Snapshot {
	s := &Snapshot{
		at: in.at, byID: map[int64]int{},
		movies: map[int64]Keep{}, shows: map[int64]Keep{}, seasons: map[seasonKey]Keep{}, episodes: map[int64]Keep{},
		files: map[FileKey]FileStatus{}, torrents: map[string]TorrentShare{}, usage: in.usage,
	}
	items := make([]Download, 0, len(in.views))
	for _, v := range in.views {
		items = append(items, v.download)
	}
	slices.SortFunc(items, listOrder)
	for i, d := range items {
		s.byID[d.ID] = i
	}
	s.list = List{Items: items, Usage: in.usage, PausedReason: in.reason, ResumesAt: in.resumes}

	// Every admitted file, with its torrent's state.
	for hash, t := range in.torrents {
		f := in.facts[hash]
		for path, size := range t.want.files {
			st := FileStatus{Bytes: size, State: t.state}
			if f != nil {
				st.DoneBytes, st.Complete = min(f.done[path], size), f.complete[path]
			}
			if st.Complete {
				st.State = StateDone
			}
			s.files[FileKey{hash, path}] = st
		}
	}
	s.downloading = downloadingOf(s.files)

	// Titles, seasons and episodes.
	holds := map[int64]episodeHold{}
	for _, v := range in.views {
		d, dl := v.cov.d, v.download
		keep := Keep{DownloadID: dl.ID, Reason: dl.Reason, Scope: dl.Scope, State: dl.State,
			Pct: pct(dl.DoneBytes, dl.WantedBytes, dl.State == StateDone), UntilWatched: dl.UntilWatched, ExpiresAt: dl.ExpiresAt}
		switch d.Scope {
		case library.DownloadScopeMovie:
			s.movies[d.MovieID] = keep
		case library.DownloadScopeShow:
			s.shows[d.ShowID] = keep
		case library.DownloadScopeSeason:
			if d.Title.SeasonNumber != nil {
				s.seasons[seasonKey{d.Title.LibraryID, *d.Title.SeasonNumber}] = keep
			}
		}
		if d.Scope == library.DownloadScopeMovie {
			continue
		}
		for _, it := range v.cov.items {
			ek := keep
			ek.State, ek.Pct = s.itemFile(in.stage.adm.overCap[d.ID], it)
			cand := episodeHold{ek, d.Reason, d.Scope}
			if cur, ok := holds[it.ref.ID]; ok && !cand.beats(cur) {
				continue
			}
			holds[it.ref.ID] = cand
		}
	}
	for id, h := range holds {
		s.episodes[id] = h.keep
	}

	// Torrents: wanted and holds (the downloads with admitted files there).
	holders := map[string][]TorrentHold{}
	for _, v := range in.views {
		seen := map[string]bool{}
		for key := range in.stage.adm.admitted[v.download.ID] {
			if seen[key.Hash] {
				continue
			}
			seen[key.Hash] = true
			holders[key.Hash] = append(holders[key.Hash], TorrentHold{DownloadID: v.download.ID, Reason: v.download.Reason,
				Scope: v.download.Scope, Title: v.download.Item.Title})
		}
	}
	for hash, t := range in.torrents {
		w := TorrentWanted{Bytes: t.bytes, DoneBytes: t.done, State: t.state, ResumesAt: t.resumesAt}
		if t.state == StateDownloading {
			w.EtaSeconds = etaSeconds(t.bytes-t.done, t.speed)
		}
		share := TorrentShare{Wanted: w, Holds: holders[hash]}
		slices.SortFunc(share.Holds, func(a, b TorrentHold) int {
			if a.Reason != b.Reason {
				if a.Reason == library.DownloadKept {
					return -1
				}
				return 1
			}
			return cmp.Compare(a.DownloadID, b.DownloadID)
		})
		s.torrents[hash] = share
	}
	return s
}

// episodeHold is a download holding an episode, with the episode's keep.
type episodeHold struct {
	keep   Keep
	reason library.DownloadReason
	scope  library.DownloadScope
}

// beats reports whether h holds an episode before cur: kept over smart,
// then the narrowest scope.
func (h episodeHold) beats(cur episodeHold) bool {
	if h.reason != cur.reason {
		return h.reason == library.DownloadKept
	}
	return narrowness[h.scope] < narrowness[cur.scope]
}

// itemFile is an episode's own file's state and pct under a download
// (overCap its files over the cap): no_file without one, unavailable from
// HDRezka or a pure v2 torrent, storage_full over the cap, done once it's
// on disk, else its torrent's.
func (s *Snapshot) itemFile(overCap map[FileKey]bool, it coveredItem) (State, int) {
	switch it.kind {
	case kindNone:
		return StateNoFile, 0
	case kindHDRezka, kindUnavailable:
		return StateUnavailable, 0
	}
	f, ok := s.files[it.key]
	if overCap[it.key] || !ok {
		return StateStorageFull, 0
	}
	return f.State, f.Pct()
}
