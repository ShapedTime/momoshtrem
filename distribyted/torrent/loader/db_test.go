package loader

import (
	"os"
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/require"
)

const m1 = "magnet:?xt=urn:btih:c9e15763f722f23e98a29decdfae341b98d53056"
const testHash = "c9e15763f722f23e98a29decdfae341b98d53056"

func TestDB(t *testing.T) {
	require := require.New(t)

	tmpService, err := os.MkdirTemp("", "service")
	require.NoError(err)
	defer os.RemoveAll(tmpService)

	s, err := NewDB(tmpService)
	require.NoError(err)
	defer s.Close()

	// Test invalid magnet
	err = s.AddMagnet("route1", "WRONG MAGNET", nil)
	require.Error(err)

	// Test adding magnet without metadata
	err = s.AddMagnet("route1", m1, nil)
	require.NoError(err)

	// Test adding same magnet to different route
	err = s.AddMagnet("route2", m1, nil)
	require.NoError(err)

	// List and verify
	l, err := s.ListMagnets()
	require.NoError(err)
	require.Len(l, 2)
	require.Len(l["route1"], 1)
	require.Equal(l["route1"][0].MagnetURI, m1)
	require.Nil(l["route1"][0].Metadata)
	require.Len(l["route2"], 1)
	require.Equal(l["route2"][0].MagnetURI, m1)
	require.Nil(l["route2"][0].Metadata)

	// Test removal from non-existent route
	removed, err := s.RemoveFromHash("other", testHash)
	require.NoError(err)
	require.False(removed)

	// Test removal from existing route
	removed, err = s.RemoveFromHash("route1", testHash)
	require.NoError(err)
	require.True(removed)

	// Verify removal
	l, err = s.ListMagnets()
	require.NoError(err)
	require.Len(l, 1)
	require.Len(l["route2"], 1)
	require.Equal(l["route2"][0].MagnetURI, m1)
}

func TestDBWithMetadata(t *testing.T) {
	require := require.New(t)

	tmpService, err := os.MkdirTemp("", "service")
	require.NoError(err)
	defer os.RemoveAll(tmpService)

	s, err := NewDB(tmpService)
	require.NoError(err)
	defer s.Close()

	// Add magnet with metadata
	season := 1
	metadata := &TMDBMetadata{
		Type:   MediaTypeTV,
		TMDBID: 12345,
		Title:  "Test Show",
		Year:   2024,
		Season: &season,
	}
	err = s.AddMagnet("route1", m1, metadata)
	require.NoError(err)

	// List and verify metadata is returned
	l, err := s.ListMagnets()
	require.NoError(err)
	require.Len(l, 1)
	require.Len(l["route1"], 1)
	require.Equal(l["route1"][0].MagnetURI, m1)
	require.NotNil(l["route1"][0].Metadata)
	require.Equal(l["route1"][0].Metadata.Title, "Test Show")
	require.Equal(l["route1"][0].Metadata.TMDBID, 12345)
	require.Equal(l["route1"][0].Metadata.Type, MediaTypeTV)
	require.Equal(*l["route1"][0].Metadata.Season, 1)

	// Test GetTorrentInfo
	info, err := s.GetTorrentInfo("route1", testHash)
	require.NoError(err)
	require.NotNil(info)
	require.Equal(info.MagnetURI, m1)
	require.NotNil(info.Metadata)
	require.Equal(info.Metadata.Title, "Test Show")

	// Test GetTorrentInfo for non-existent
	info, err = s.GetTorrentInfo("route1", "nonexistenthash")
	require.NoError(err)
	require.Nil(info)
}

func TestUpdateMetadata(t *testing.T) {
	require := require.New(t)

	tmpService, err := os.MkdirTemp("", "service")
	require.NoError(err)
	defer os.RemoveAll(tmpService)

	s, err := NewDB(tmpService)
	require.NoError(err)
	defer s.Close()

	// Add magnet without metadata first
	err = s.AddMagnet("route1", m1, nil)
	require.NoError(err)

	// Verify no metadata
	info, err := s.GetTorrentInfo("route1", testHash)
	require.NoError(err)
	require.NotNil(info)
	require.Nil(info.Metadata)

	// Update with metadata
	newMetadata := &TMDBMetadata{
		Type:   MediaTypeMovie,
		TMDBID: 67890,
		Title:  "Test Movie",
		Year:   2023,
	}
	err = s.UpdateMetadata("route1", testHash, newMetadata)
	require.NoError(err)

	// Verify metadata was updated
	info, err = s.GetTorrentInfo("route1", testHash)
	require.NoError(err)
	require.NotNil(info)
	require.NotNil(info.Metadata)
	require.Equal(info.Metadata.Title, "Test Movie")
	require.Equal(info.Metadata.Type, MediaTypeMovie)

	// Update metadata again
	updatedMetadata := &TMDBMetadata{
		Type:   MediaTypeMovie,
		TMDBID: 67890,
		Title:  "Updated Title",
		Year:   2023,
	}
	err = s.UpdateMetadata("route1", testHash, updatedMetadata)
	require.NoError(err)

	// Verify update
	info, err = s.GetTorrentInfo("route1", testHash)
	require.NoError(err)
	require.Equal(info.Metadata.Title, "Updated Title")
}

func TestBackwardCompatibility(t *testing.T) {
	require := require.New(t)

	tmpService, err := os.MkdirTemp("", "service")
	require.NoError(err)
	defer os.RemoveAll(tmpService)

	// Manually insert old-format entry (plain string)
	opts := badger.DefaultOptions(tmpService)
	opts.Logger = nil
	db, err := badger.Open(opts)
	require.NoError(err)

	err = db.Update(func(txn *badger.Txn) error {
		key := "/route/" + testHash + "/legacy"
		return txn.Set([]byte(key), []byte(m1)) // Plain string, not JSON
	})
	require.NoError(err)
	require.NoError(db.Close())

	// Open with our loader
	s, err := NewDB(tmpService)
	require.NoError(err)
	defer s.Close()

	// Should still read old entry
	l, err := s.ListMagnets()
	require.NoError(err)
	require.Len(l["legacy"], 1)
	require.Equal(l["legacy"][0].MagnetURI, m1)
	require.Nil(l["legacy"][0].Metadata)

	// Test GetTorrentInfo with old format
	info, err := s.GetTorrentInfo("legacy", testHash)
	require.NoError(err)
	require.NotNil(info)
	require.Equal(info.MagnetURI, m1)
	require.Nil(info.Metadata)

	// Update old entry with metadata
	newMetadata := &TMDBMetadata{
		Type:   MediaTypeTV,
		TMDBID: 11111,
		Title:  "Migrated Show",
		Year:   2022,
	}
	err = s.UpdateMetadata("legacy", testHash, newMetadata)
	require.NoError(err)

	// Verify it's now JSON format with metadata
	info, err = s.GetTorrentInfo("legacy", testHash)
	require.NoError(err)
	require.NotNil(info)
	require.NotNil(info.Metadata)
	require.Equal(info.Metadata.Title, "Migrated Show")
}
