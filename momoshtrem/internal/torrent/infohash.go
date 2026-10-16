package torrent

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrInvalidInfoHash is a hash that's neither 40 hex characters nor 32
// base32 ones.
var ErrInvalidInfoHash = errors.New("invalid info hash")

// NormalizeInfoHash returns a BitTorrent v1 info hash as lowercase hex, the
// form the service keys torrents by and the database stores. It accepts hex
// in either case, or base32 (32 characters, used by some magnets), and
// returns "" for anything else.
func NormalizeInfoHash(hash string) string {
	switch len(hash) {
	case 40:
		if _, err := hex.DecodeString(hash); err == nil {
			return strings.ToLower(hash)
		}
	case 32:
		if raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash)); err == nil && len(raw) == 20 {
			return hex.EncodeToString(raw)
		}
	}
	return ""
}
