package torrent

import (
	"bytes"
	"encoding/gob"
	"log/slog"
	"time"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/dgraph-io/badger/v3"
)

var _ bep44.Store = &ItemStore{}

// ItemStore implements bep44.Store using Badger for DHT item persistence.
type ItemStore struct {
	ttl time.Duration
	db  *badger.DB
}

// badgerLogger adapts slog for Badger's logger interface.
type badgerLogger struct {
	log *slog.Logger
}

func (l *badgerLogger) Errorf(f string, v ...interface{}) {
	l.log.Error(f, "args", v)
}

func (l *badgerLogger) Warningf(f string, v ...interface{}) {
	l.log.Warn(f, "args", v)
}

func (l *badgerLogger) Infof(f string, v ...interface{}) {
	l.log.Info(f, "args", v)
}

func (l *badgerLogger) Debugf(f string, v ...interface{}) {
	l.log.Debug(f, "args", v)
}

// NewItemStore creates a new DHT item store backed by Badger.
func NewItemStore(path string, itemsTTL time.Duration) (*ItemStore, error) {
	log := slog.With("component", "item-store")

	opts := badger.DefaultOptions(path).
		WithLogger(&badgerLogger{log: log}).
		WithValueLogFileSize(1<<26 - 1)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	// Run garbage collection
	err = db.RunValueLogGC(0.5)
	if err != nil && err != badger.ErrNoRewrite {
		db.Close()
		return nil, err
	}

	return &ItemStore{
		db:  db,
		ttl: itemsTTL,
	}, nil
}

// Put stores a DHT item with TTL.
func (s *ItemStore) Put(i *bep44.Item) error {
	tx := s.db.NewTransaction(true)
	defer tx.Discard()

	key := i.Target()
	var value bytes.Buffer

	enc := gob.NewEncoder(&value)
	if err := enc.Encode(i); err != nil {
		return err
	}

	e := badger.NewEntry(key[:], value.Bytes()).WithTTL(s.ttl)
	if err := tx.SetEntry(e); err != nil {
		return err
	}

	return tx.Commit()
}

// Get retrieves a DHT item by target.
func (s *ItemStore) Get(t bep44.Target) (*bep44.Item, error) {
	tx := s.db.NewTransaction(false)
	defer tx.Discard()

	dbi, err := tx.Get(t[:])
	if err == badger.ErrKeyNotFound {
		return nil, bep44.ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}

	valb, err := dbi.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(valb)
	dec := gob.NewDecoder(buf)
	var i *bep44.Item
	if err := dec.Decode(&i); err != nil {
		return nil, err
	}

	return i, nil
}

// Del removes a DHT item (no-op, TTL handles expiration).
func (s *ItemStore) Del(t bep44.Target) error {
	return nil
}

// Close shuts down the Badger database.
func (s *ItemStore) Close() error {
	return s.db.Close()
}
