package torrent

import (
	"crypto/rand"
	"os"
)

var emptyPeerID [20]byte

// GetOrCreatePeerID reads an existing peer ID from file or creates a new one.
// The peer ID is a 20-byte identifier used by the BitTorrent protocol.
// Persisting it ensures stable identity across restarts.
func GetOrCreatePeerID(path string) ([20]byte, error) {
	// Try to read existing ID
	idb, err := os.ReadFile(path)
	if err == nil && len(idb) >= 20 {
		var out [20]byte
		copy(out[:], idb)
		return out, nil
	}

	if err != nil && !os.IsNotExist(err) {
		return emptyPeerID, err
	}

	// Generate new random ID
	var out [20]byte
	if _, err := rand.Read(out[:]); err != nil {
		return emptyPeerID, err
	}

	// Persist to file
	if err := os.WriteFile(path, out[:], 0644); err != nil {
		return emptyPeerID, err
	}

	return out, nil
}
