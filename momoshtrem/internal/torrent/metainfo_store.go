package torrent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent/metainfo"
)

// ErrCorruptMetainfo is a stored .torrent that doesn't decode, or whose info
// dict isn't the hash it's stored under.
var ErrCorruptMetainfo = errors.New("stored metainfo is corrupt")

// MetainfoStore keeps each torrent's metainfo (its .torrent file) on disk,
// so a torrent loads again without waiting for peers to send the info dict,
// and its data can be deleted while it isn't loaded: the piece hashes name
// its pieces in the cache. Files live at {dir}/<hash>.torrent.
type MetainfoStore struct {
	dir string
}

// NewMetainfoStore returns a store in dir ({metadata_folder}/metainfo). The
// folder is created on the first Save.
func NewMetainfoStore(dir string) *MetainfoStore {
	return &MetainfoStore{dir: dir}
}

// path is the hash's file. The hash must be normalised first: anything but
// 40 lowercase hex characters is rejected, so it can't leave the folder.
func (s *MetainfoStore) path(hash string) (string, error) {
	if NormalizeInfoHash(hash) != hash || len(hash) != 40 {
		return "", fmt.Errorf("%w: %q", ErrInvalidInfoHash, hash)
	}
	return filepath.Join(s.dir, hash+".torrent"), nil
}

// Save writes the torrent's metainfo, replacing any earlier file. The write
// is atomic: a temp file in the folder, synced, then renamed.
func (s *MetainfoStore) Save(hash string, mi *metainfo.MetaInfo) error {
	final, err := s.path(hash)
	if err != nil {
		return err
	}
	if len(mi.InfoBytes) == 0 {
		return errors.New("metainfo has no info dict")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("failed to create the metainfo folder: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".metainfo-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create a temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // a no-op once renamed

	if err := mi.Write(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write metainfo: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to sync metainfo: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close metainfo: %w", err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		return fmt.Errorf("failed to store metainfo: %w", err)
	}
	return nil
}

// Load reads the torrent's stored metainfo; nil, nil when there is none.
// ErrCorruptMetainfo when the file doesn't decode or holds another torrent.
func (s *MetainfoStore) Load(hash string) (*metainfo.MetaInfo, error) {
	path, err := s.path(hash)
	if err != nil {
		return nil, err
	}
	mi, err := metainfo.LoadFromFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorruptMetainfo, err)
	}
	if len(mi.InfoBytes) == 0 {
		return nil, fmt.Errorf("%w: no info dict", ErrCorruptMetainfo)
	}
	if got := mi.HashInfoBytes().HexString(); got != hash {
		return nil, fmt.Errorf("%w: the info dict hashes to %s", ErrCorruptMetainfo, got)
	}
	return mi, nil
}

// Remove deletes the torrent's stored metainfo. A missing file is fine.
func (s *MetainfoStore) Remove(hash string) error {
	path, err := s.path(hash)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove metainfo: %w", err)
	}
	return nil
}
