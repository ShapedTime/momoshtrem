package catalog

import (
	"net/url"
	"path"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// torrentView answers torrent questions for one request: what downloads
// want, from one snapshot of the download manager's last pass (#77), and
// what the client says of each torrent, asked once. All in memory: no
// database, and nothing is loaded.
type torrentView struct {
	client    Torrents
	downloads DownloadsSnapshot // never nil
	states    map[string]*torrentSnapshot
}

// torrentSnapshot is one torrent as the client sees it now.
type torrentSnapshot struct {
	loaded   bool
	active   bool
	paused   bool // the user paused it, loaded or not; never active
	seeders  int
	speedBps int64 // download speed, smoothed
}

func (s *Service) torrentView() *torrentView {
	return &torrentView{client: s.torrents, downloads: s.snapshot(), states: map[string]*torrentSnapshot{}}
}

// snapshot is the download manager's latest pass; without downloads, one
// that answers nothing.
func (s *Service) snapshot() DownloadsSnapshot {
	if s.downloads == nil {
		return (*downloads.Snapshot)(nil)
	}
	if snap := s.downloads(); snap != nil {
		return snap
	}
	return (*downloads.Snapshot)(nil)
}

func (v *torrentView) state(hash string) *torrentSnapshot {
	if st, ok := v.states[hash]; ok {
		return st
	}
	st := &torrentSnapshot{}
	v.states[hash] = st
	if v.client == nil || hash == "" {
		return st
	}
	_, st.paused = v.client.PausedAt(hash)
	status, err := v.client.GetStatus(hash)
	if err != nil {
		return st // not loaded (it loads on first read)
	}
	st.loaded = true
	st.active = v.client.IsActive(hash)
	st.seeders = status.Seeders
	st.speedBps = status.DownloadSpeed
	return st
}

// fileDownload is what downloads want of an assigned file (PLAN §6
// "Availability rules"): it's downloading only while a download wants it
// and it isn't all on disk, and not while that download is paused. A file
// nothing keeps streams on demand, and an HDRezka stream plays from the
// CDN: neither is ever downloading.
func (v *torrentView) fileDownload(a *library.TorrentAssignment) (f downloads.FileStatus, downloading bool) {
	if a == nil || !a.IsTorrent() {
		return downloads.FileStatus{}, false
	}
	f, ok := v.downloads.File(a.InfoHash, a.FilePath)
	return f, ok && f.Downloading()
}

// downloadingFiles are the files downloading now, for the /library query.
func (v *torrentView) downloadingFiles() []library.FileRef {
	keys := v.downloads.DownloadingFiles()
	if len(keys) == 0 {
		return nil
	}
	refs := make([]library.FileRef, len(keys))
	for i, k := range keys {
		refs[i] = library.FileRef{InfoHash: k.Hash, Path: k.Path}
	}
	return refs
}

// filesPct is what's on disk of these files together, in whole percent,
// from the piece cache (the snapshot's); a file no download wants counts
// as nothing.
func (v *torrentView) filesPct(refs []library.FileRef) int {
	var done, bytes int64
	for _, ref := range refs {
		f, _ := v.downloads.File(ref.InfoHash, ref.Path)
		done += f.DoneBytes
		bytes += f.Bytes
	}
	return pct(done, bytes)
}

// downloadTotals combines a title's downloading files (DownloadProgress):
// what's on disk of them, their torrents' speeds (each torrent once), the
// most seeders, and when they'll be done.
type downloadTotals struct {
	files    map[library.FileRef]bool
	torrents map[string]*torrentTotal
	done     int64
	bytes    int64
	seeders  int
}

// torrentTotal is one torrent of a title's download.
type torrentTotal struct {
	speedBps int64
	left     int64 // bytes the title's files there still want
	// fetching: the manager is downloading the torrent (it's in a slot), so
	// its speed fetches these files; not while it waits or is queued.
	fetching bool
}

func (d *downloadTotals) add(a *library.TorrentAssignment, f downloads.FileStatus, st *torrentSnapshot) {
	ref := library.FileRef{InfoHash: a.InfoHash, Path: a.FilePath}
	if d.files[ref] {
		return // a multi-episode file counts once
	}
	if d.files == nil {
		d.files = map[library.FileRef]bool{}
		d.torrents = map[string]*torrentTotal{}
	}
	d.files[ref] = true
	t := d.torrents[a.InfoHash]
	if t == nil {
		t = &torrentTotal{speedBps: st.speedBps, fetching: true}
		d.torrents[a.InfoHash] = t
	}
	t.left += max(0, f.Bytes-f.DoneBytes)
	t.fetching = t.fetching && f.State == downloads.StateDownloading
	d.done += f.DoneBytes
	d.bytes += f.Bytes
	d.seeders = max(d.seeders, st.seeders)
}

// progress is the title's download, nil when nothing is downloading. The
// ETA is set while every torrent of it downloads with a speed: the slowest
// one's (torrents download side by side). A torrent that waits or is
// queued has none to give.
func (d *downloadTotals) progress() *DownloadProgress {
	if len(d.files) == 0 {
		return nil
	}
	p := &DownloadProgress{Pct: pct(d.done, d.bytes), Seeders: d.seeders}
	var eta int64
	known := true
	for _, t := range d.torrents {
		p.SpeedBps += t.speedBps
		if !t.fetching || t.speedBps <= 0 {
			known = false
			continue
		}
		eta = max(eta, (t.left+t.speedBps-1)/t.speedBps)
	}
	if known {
		p.ETASeconds = intPtr(int(eta))
	}
	return p
}

// keepOf is the Keep of a snapshot read, nil when nothing holds it.
func keepOf(k downloads.Keep, ok bool) *downloads.Keep {
	if !ok {
		return nil
	}
	return &k
}

// Downloads tells, for one request, which assigned files are downloading,
// from one snapshot of the downloads, as the title pages do. All in
// memory.
type Downloads struct{ tv *torrentView }

// Downloads starts one request's view of the downloads.
func (s *Service) Downloads() *Downloads {
	return &Downloads{tv: s.torrentView()}
}

// Progress reports whether an assigned file is downloading (a download
// wants it and it isn't all on disk) and what's on disk of it, in whole
// percent. An HDRezka stream is never downloading.
func (d *Downloads) Progress(a *library.TorrentAssignment) (downloading bool, percent int) {
	f, downloading := d.tv.fileDownload(a)
	return downloading, f.Pct()
}

// pct is done/length in whole percent, rounded down so a file is only ever
// 100 once it's complete.
func pct(done, length int64) int {
	if length <= 0 {
		return 0
	}
	p := int(done * 100 / length)
	return min(max(p, 0), 100)
}

// releaseName names an assigned release: the magnet's display name, else the
// torrent's top folder, else the file name without its extension.
func releaseName(a *library.TorrentAssignment) string {
	if u, err := url.Parse(a.MagnetURI); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			return dn
		}
	}
	if dir, _, ok := strings.Cut(a.FilePath, "/"); ok && dir != "" {
		return dir
	}
	base := path.Base(a.FilePath)
	return strings.TrimSuffix(base, path.Ext(base))
}
