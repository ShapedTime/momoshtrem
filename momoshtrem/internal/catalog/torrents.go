package catalog

import (
	"net/url"
	"path"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// torrentView answers torrent questions for one request, asking the client
// about each torrent once. All in memory: no database.
type torrentView struct {
	client Torrents
	states map[string]*torrentSnapshot
}

// torrentSnapshot is one torrent as the client sees it now.
type torrentSnapshot struct {
	loaded   bool
	active   bool
	paused   bool  // the user paused it, loaded or not; never active
	seeders  int
	speedBps int64 // download speed, smoothed
	files    map[string]torrent.FileProgress
}

func (s *Service) torrentView() *torrentView {
	return &torrentView{client: s.torrents, states: map[string]*torrentSnapshot{}}
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
	files, err := v.client.FilesProgress(hash)
	if err != nil {
		return st // not loaded (it loads on first read)
	}
	st.loaded, st.files = true, files
	st.active = v.client.IsActive(hash)
	if status, err := v.client.GetStatus(hash); err == nil {
		st.seeders = status.Seeders
		st.speedBps = status.DownloadSpeed
	}
	return st
}

// fileProgress is how far along an assigned file is. downloading is true
// while its torrent is loaded and active and the file isn't all on disk.
// An HDRezka stream is never downloading: it plays from the CDN.
func (v *torrentView) fileProgress(a *library.TorrentAssignment) (downloading bool, done, length int64, seeders int) {
	if a == nil || !a.IsTorrent() {
		return false, 0, 0, 0
	}
	st := v.state(a.InfoHash)
	if !st.loaded || !st.active {
		return false, 0, 0, st.seeders
	}
	p, ok := st.files[a.FilePath]
	if !ok || p.Complete() {
		return false, p.BytesCompleted, p.Length, st.seeders
	}
	return true, p.BytesCompleted, p.Length, st.seeders
}

// downloadingFiles lists the files of every loaded, active torrent that
// aren't all on disk yet, for the /library query.
func (v *torrentView) downloadingFiles() []library.FileRef {
	if v.client == nil {
		return nil
	}
	statuses, err := v.client.ListTorrents()
	if err != nil {
		return nil
	}
	var refs []library.FileRef
	for _, status := range statuses {
		st := v.state(status.InfoHash)
		if !st.loaded || !st.active {
			continue
		}
		for p, progress := range st.files {
			if !progress.Complete() {
				refs = append(refs, library.FileRef{InfoHash: status.InfoHash, Path: p})
			}
		}
	}
	return refs
}

// filesProgress is how far along these files are together, as the client
// sees them now; a file it doesn't know counts as nothing.
func (v *torrentView) filesProgress(refs []library.FileRef) (done, length int64) {
	for _, ref := range refs {
		p := v.state(ref.InfoHash).files[ref.Path]
		done += p.BytesCompleted
		length += p.Length
	}
	return done, length
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
