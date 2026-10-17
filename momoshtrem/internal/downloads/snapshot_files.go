package downloads

import (
	"cmp"
	"slices"
)

// DownloadingFiles are the files that are downloading now (FileStatus.
// Downloading: a download wants them and they aren't all on disk, unless
// their download is paused), by hash and then path. /library's one query
// reads them as its downloading set (P6 #204). nil for a nil Snapshot.
func (s *Snapshot) DownloadingFiles() []FileKey {
	if s == nil {
		return nil
	}
	var out []FileKey
	for key, f := range s.files {
		if f.Downloading() {
			out = append(out, key)
		}
	}
	slices.SortFunc(out, func(a, b FileKey) int {
		return cmp.Or(cmp.Compare(a.Hash, b.Hash), cmp.Compare(a.Path, b.Path))
	})
	return out
}
