package loader

import (
	"encoding/json"
	"path"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/dgraph-io/badger/v3"
	dlog "github.com/distribyted/distribyted/log"
	"github.com/rs/zerolog/log"
)

var _ LoaderAdder = &DB{}

const routeRootKey = "/route/"

type DB struct {
	db *badger.DB
}

func NewDB(path string) (*DB, error) {
	l := log.Logger.With().Str("component", "torrent-store").Logger()

	opts := badger.DefaultOptions(path).
		WithLogger(&dlog.Badger{L: l}).
		WithValueLogFileSize(1<<26 - 1)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	err = db.RunValueLogGC(0.5)
	if err != nil && err != badger.ErrNoRewrite {
		return nil, err
	}

	return &DB{
		db: db,
	}, nil
}

func (l *DB) AddMagnet(r, m string, metadata *TMDBMetadata) error {
	err := l.db.Update(func(txn *badger.Txn) error {
		spec, err := metainfo.ParseMagnetUri(m)
		if err != nil {
			return err
		}

		ih := spec.InfoHash.HexString()

		rp := path.Join(routeRootKey, ih, r)

		// Store as JSON with optional metadata
		data := TorrentWithMetadata{
			MagnetURI: m,
			Metadata:  metadata,
		}
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return err
		}

		return txn.Set([]byte(rp), jsonBytes)
	})

	if err != nil {
		return err
	}

	return l.db.Sync()
}

func (l *DB) RemoveFromHash(r, h string) (bool, error) {
	tx := l.db.NewTransaction(true)
	defer tx.Discard()

	var mh metainfo.Hash
	if err := mh.FromHexString(h); err != nil {
		return false, err
	}

	rp := path.Join(routeRootKey, h, r)
	if _, err := tx.Get([]byte(rp)); err != nil {
		return false, nil
	}

	if err := tx.Delete([]byte(rp)); err != nil {
		return false, err
	}

	return true, tx.Commit()
}

func (l *DB) ListMagnets() (map[string][]TorrentWithMetadata, error) {
	tx := l.db.NewTransaction(false)
	defer tx.Discard()

	it := tx.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	prefix := []byte(routeRootKey)
	out := make(map[string][]TorrentWithMetadata)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		_, r := path.Split(string(it.Item().Key()))
		i := it.Item()
		if err := i.Value(func(v []byte) error {
			var twm TorrentWithMetadata

			// Try JSON unmarshal first (new format)
			if err := json.Unmarshal(v, &twm); err != nil {
				// Fallback: treat as plain magnet string (backward compat)
				twm = TorrentWithMetadata{
					MagnetURI: string(v),
					Metadata:  nil,
				}
			}

			out[r] = append(out[r], twm)
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func (l *DB) GetTorrentInfo(route, hash string) (*TorrentWithMetadata, error) {
	tx := l.db.NewTransaction(false)
	defer tx.Discard()

	rp := path.Join(routeRootKey, hash, route)
	item, err := tx.Get([]byte(rp))
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	var twm TorrentWithMetadata
	err = item.Value(func(v []byte) error {
		if err := json.Unmarshal(v, &twm); err != nil {
			// Backward compat: plain string
			twm = TorrentWithMetadata{
				MagnetURI: string(v),
				Metadata:  nil,
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &twm, nil
}

func (l *DB) UpdateMetadata(route, hash string, metadata *TMDBMetadata) error {
	return l.db.Update(func(txn *badger.Txn) error {
		rp := path.Join(routeRootKey, hash, route)

		// Get existing entry
		item, err := txn.Get([]byte(rp))
		if err != nil {
			return err
		}

		var twm TorrentWithMetadata
		err = item.Value(func(v []byte) error {
			if err := json.Unmarshal(v, &twm); err != nil {
				// Backward compat: plain string
				twm = TorrentWithMetadata{
					MagnetURI: string(v),
					Metadata:  nil,
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Update metadata
		twm.Metadata = metadata

		// Save back
		jsonBytes, err := json.Marshal(twm)
		if err != nil {
			return err
		}

		return txn.Set([]byte(rp), jsonBytes)
	})
}

func (l *DB) ListTorrentPaths() (map[string][]string, error) {
	return nil, nil
}

func (l *DB) Close() error {
	return l.db.Close()
}
