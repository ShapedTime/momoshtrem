package torrent

import (
	"fmt"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/types"
)

// PieceState is a piece on Activity's piece map (GET /torrents/{hash}).
type PieceState string

const (
	PieceHave    PieceState = "have"    // complete
	PiecePartial PieceState = "partial" // some of it arrived
	// PiecePriority is a piece the streaming windows want (priority High
	// or more) that hasn't started arriving.
	PiecePriority PieceState = "priority"
	PieceMissing  PieceState = "missing"
)

// PieceRun is consecutive pieces in one state.
type PieceRun struct {
	State PieceState `json:"state"`
	Count int        `json:"count"`
}

// PieceRuns maps anacrolix's piece-state runs to Activity's states, in
// piece order, merging runs that end up in the same state.
func PieceRuns(runs torrent.PieceStateRuns) []PieceRun {
	out := []PieceRun{}
	for _, r := range runs {
		if r.Length <= 0 {
			continue
		}
		st := pieceStateOf(r.PieceState)
		if n := len(out); n > 0 && out[n-1].State == st {
			out[n-1].Count += r.Length
			continue
		}
		out = append(out, PieceRun{State: st, Count: r.Length})
	}
	return out
}

func pieceStateOf(ps torrent.PieceState) PieceState {
	switch {
	case ps.Complete:
		return PieceHave
	case ps.Partial:
		return PiecePartial
	case ps.Priority >= types.PiecePriorityHigh:
		return PiecePriority
	default:
		return PieceMissing
	}
}

// FileEntry is one file of a torrent: its path as the client names it
// ("{name}/…" in a multi-file torrent), its size and, loaded, how much of
// it is on disk.
type FileEntry struct {
	Path           string
	Size           int64
	BytesCompleted int64
}

// StoredInfo is what a torrent's stored metainfo says, read without
// loading the torrent.
type StoredInfo struct {
	Name        string
	TotalSize   int64
	PieceLength int64
	PiecesTotal int
	Files       []FileEntry
	Trackers    []string // the announce list's URLs, tier by tier
}

// Detail is what the client knows of a loaded torrent.
type Detail struct {
	// HasInfo is false while the metadata is still arriving: the info's
	// fields are then empty.
	HasInfo     bool
	Name        string
	TotalSize   int64
	PieceLength int64
	PiecesTotal int
	PiecesHave  int
	Pieces      []PieceRun
	Files       []FileEntry
	Trackers    []string
	// Magnet is one built from the live torrent: its hash, name and
	// trackers.
	Magnet string
	// DownloadedBytes and UploadedBytes are the useful data received and
	// the data sent since it loaded.
	DownloadedBytes int64
	UploadedBytes   int64
	Peers           int // connected
	Seeders         int // connected
}

// Summary reads the torrent's stored metainfo: nil, nil when there is
// none.
func (s *MetainfoStore) Summary(hash string) (*StoredInfo, error) {
	mi, err := s.Load(hash)
	if err != nil || mi == nil {
		return nil, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorruptMetainfo, err)
	}
	return &StoredInfo{
		Name:        info.BestName(),
		TotalSize:   info.TotalLength(),
		PieceLength: info.PieceLength,
		PiecesTotal: info.NumPieces(),
		Files:       filesOf(&info),
		Trackers:    trackersOf(mi),
	}, nil
}

// filesOf lists an info dict's files with the paths anacrolix's File.Path
// gives them.
func filesOf(info *metainfo.Info) []FileEntry {
	files := info.UpvertedFiles()
	out := make([]FileEntry, 0, len(files))
	for _, fi := range files {
		out = append(out, FileEntry{
			Path: strings.Join(append([]string{info.BestName()}, fi.BestPath()...), "/"),
			Size: fi.Length,
		})
	}
	return out
}

func trackersOf(mi *metainfo.MetaInfo) []string {
	urls := mi.UpvertedAnnounceList().DistinctValues()
	if urls == nil {
		return []string{}
	}
	return urls
}

// Detail returns what the client knows of a loaded torrent: its piece
// map, files, trackers and totals. ErrTorrentNotFound when it isn't
// loaded. It never loads one.
func (s *service) Detail(infoHash string) (*Detail, error) {
	hash := NormalizeInfoHash(infoHash)
	t := s.loaded(hash)
	if t == nil {
		return nil, ErrTorrentNotFound
	}
	stats := t.Stats()
	mi := t.Metainfo()
	d := &Detail{
		Trackers:        trackersOf(&mi),
		DownloadedBytes: stats.BytesReadUsefulData.Int64(),
		UploadedBytes:   stats.BytesWrittenData.Int64(),
		Peers:           stats.ActivePeers,
		Seeders:         stats.ConnectedSeeders,
	}
	ih := t.InfoHash()
	info := t.Info()
	d.Magnet = mi.Magnet(&ih, info).String()
	if info == nil {
		return d, nil
	}
	d.HasInfo = true
	d.Name = info.BestName()
	d.TotalSize = info.TotalLength()
	d.PieceLength = info.PieceLength
	d.PiecesTotal = t.NumPieces()
	runs := t.PieceStateRuns()
	d.Pieces = PieceRuns(runs)
	for _, r := range runs {
		if r.Complete {
			d.PiecesHave += r.Length
		}
	}
	for _, f := range t.Files() {
		d.Files = append(d.Files, FileEntry{Path: f.Path(), Size: f.Length(), BytesCompleted: f.BytesCompleted()})
	}
	return d, nil
}
