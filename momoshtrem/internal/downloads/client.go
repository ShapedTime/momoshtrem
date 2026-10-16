package downloads

import (
	"errors"
	"fmt"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Client is the torrent client as the manager drives it: through
// torrent.Service's public API only (ServiceClient), so the streaming path
// stays untouched. Hashes are lowercase hex.
type Client interface {
	// Loaded returns the loaded torrents with their smoothed download
	// speeds.
	Loaded() (map[string]int64, error)
	// Info is a loaded torrent's info dict, found through one of its
	// files; nil when it isn't loaded.
	Info(hash, path string) *metainfo.Info
	// Load adds the torrent from its magnet (GetOrAddTorrent): at once from
	// its stored metainfo, else it waits for peers.
	Load(magnet string) error
	// PausedAt says whether the user paused the torrent (sticky).
	PausedAt(hash string) (time.Time, bool)
	// MarkActive keeps a downloading torrent's network on: the activity
	// manager idles torrents nobody reads.
	MarkActive(hash string)
	// SetPriorities sets the loaded torrent's files in normal to Normal
	// priority and those in none to no priority, leaving any already
	// there. Pieces keep their own priorities: anacrolix takes the most of
	// file, reader and piece, so streaming still comes first.
	SetPriorities(hash string, normal, none []string) error
	// Completion is what the client believes is complete of the loaded
	// torrent's pieces, by index, found through one of its files.
	Completion(hash, path string) ([]bool, error)
	// UpdateCompletion makes the client read those pieces' completion
	// from the storage again (streamlab F11: the cache lost them).
	UpdateCompletion(hash, path string, pieces []int) error
	// Progress is how much of the files the client has, dirty chunks
	// included: the stall's measure of data arriving.
	Progress(hash string, paths []string) int64
}

// ServiceClient is Client over torrent.Service. MarkActive is the activity
// manager's; nil without idle mode.
type ServiceClient struct {
	Torrents torrent.Service
	Mark     func(hash string)
}

var _ Client = ServiceClient{}

func (c ServiceClient) Loaded() (map[string]int64, error) {
	list, err := c.Torrents.ListTorrents()
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(list))
	for _, st := range list {
		out[st.InfoHash] = st.DownloadSpeed
	}
	return out, nil
}

func (c ServiceClient) torrentOf(hash, path string) (*atorrent.Torrent, error) {
	h, err := c.Torrents.GetFile(hash, path)
	if err != nil {
		return nil, err
	}
	t := h.Torrent()
	if t == nil || t.Info() == nil {
		return nil, torrent.ErrTorrentNotFound
	}
	return t, nil
}

func (c ServiceClient) Info(hash, path string) *metainfo.Info {
	t, err := c.torrentOf(hash, path)
	if err != nil {
		return nil
	}
	return t.Info()
}

func (c ServiceClient) Load(magnet string) error {
	_, err := c.Torrents.GetOrAddTorrent(magnet)
	return err
}

func (c ServiceClient) PausedAt(hash string) (time.Time, bool) { return c.Torrents.PausedAt(hash) }

func (c ServiceClient) MarkActive(hash string) {
	if c.Mark != nil {
		c.Mark(hash)
	}
}

func (c ServiceClient) SetPriorities(hash string, normal, none []string) error {
	var errs []error
	set := func(paths []string, p types.PiecePriority) {
		for _, path := range paths {
			h, err := c.Torrents.GetFile(hash, path)
			if errors.Is(err, torrent.ErrFileNotFound) {
				continue
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if f := h.File(); f != nil && f.Priority() != p {
				f.SetPriority(p)
			}
		}
	}
	set(none, types.PiecePriorityNone)
	set(normal, types.PiecePriorityNormal)
	return errors.Join(errs...)
}

func (c ServiceClient) Completion(hash, path string) ([]bool, error) {
	t, err := c.torrentOf(hash, path)
	if err != nil {
		return nil, err
	}
	out := make([]bool, 0, t.NumPieces())
	for _, run := range t.PieceStateRuns() {
		for range run.Length {
			out = append(out, run.Complete)
		}
	}
	return out, nil
}

func (c ServiceClient) UpdateCompletion(hash, path string, pieces []int) error {
	t, err := c.torrentOf(hash, path)
	if err != nil {
		return err
	}
	n := t.NumPieces()
	for _, i := range pieces {
		if i < 0 || i >= n {
			return fmt.Errorf("piece %d out of range (%d pieces)", i, n)
		}
		t.Piece(i).UpdateCompletion()
	}
	return nil
}

func (c ServiceClient) Progress(hash string, paths []string) int64 {
	progress, err := c.Torrents.FilesProgress(hash)
	if err != nil {
		return 0
	}
	var n int64
	for _, path := range paths {
		n += progress[path].BytesCompleted
	}
	return n
}
