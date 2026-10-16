package torrent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

func TestMetainfoStore(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	dir := filepath.Join(t.TempDir(), "metainfo")
	s := NewMetainfoStore(dir)

	// Nothing stored yet (and no folder).
	if mi, err := s.Load(f.hash); mi != nil || err != nil {
		t.Fatalf("Load before Save = %v, %v; want nil, nil", mi, err)
	}
	if err := s.Remove(f.hash); err != nil {
		t.Fatalf("Remove of a missing file: %v", err)
	}

	// Write, then load.
	mi := *f.mi
	mi.AnnounceList = metainfo.AnnounceList{{"udp://tracker.example:1337/announce"}}
	if err := s.Save(f.hash, &mi); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(f.hash)
	if err != nil || got == nil {
		t.Fatalf("Load = %v, %v", got, err)
	}
	if string(got.InfoBytes) != string(f.mi.InfoBytes) || len(got.AnnounceList) != 1 {
		t.Errorf("loaded %+v", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != f.hash+".torrent" {
		t.Errorf("folder holds %v, want only %s.torrent (no temp files)", entries, f.hash)
	}

	// Saving again replaces the file.
	if err := s.Save(f.hash, f.mi); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got, _ := s.Load(f.hash); got == nil || len(got.AnnounceList) != 0 {
		t.Errorf("after the second Save: %+v", got)
	}

	// Remove.
	if err := s.Remove(f.hash); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got, err := s.Load(f.hash); got != nil || err != nil {
		t.Errorf("Load after Remove = %v, %v; want nil, nil", got, err)
	}

	// A file holding another torrent, or garbage, is corrupt.
	if err := os.WriteFile(filepath.Join(dir, f.hash+".torrent"), mustEncode(t, other.mi), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(f.hash); !errors.Is(err, ErrCorruptMetainfo) {
		t.Errorf("Load of another torrent's file: err = %v, want ErrCorruptMetainfo", err)
	}
	if err := os.WriteFile(filepath.Join(dir, f.hash+".torrent"), []byte("not bencode"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(f.hash); !errors.Is(err, ErrCorruptMetainfo) {
		t.Errorf("Load of garbage: err = %v, want ErrCorruptMetainfo", err)
	}
}

func TestMetainfoStore_RejectsUnnormalisedHashes(t *testing.T) {
	f := newFixture(t)
	s := NewMetainfoStore(t.TempDir())
	for _, hash := range []string{"", "../../etc/passwd", strings.ToUpper(f.hash), f.hash[:39]} {
		if err := s.Save(hash, f.mi); !errors.Is(err, ErrInvalidInfoHash) {
			t.Errorf("Save(%q) err = %v, want ErrInvalidInfoHash", hash, err)
		}
		if _, err := s.Load(hash); !errors.Is(err, ErrInvalidInfoHash) {
			t.Errorf("Load(%q) err = %v, want ErrInvalidInfoHash", hash, err)
		}
		if err := s.Remove(hash); !errors.Is(err, ErrInvalidInfoHash) {
			t.Errorf("Remove(%q) err = %v, want ErrInvalidInfoHash", hash, err)
		}
	}
}

func mustEncode(t *testing.T, mi *metainfo.MetaInfo) []byte {
	t.Helper()
	var b strings.Builder
	if err := mi.Write(&b); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}
