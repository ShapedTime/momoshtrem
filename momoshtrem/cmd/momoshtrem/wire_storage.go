package main

import (
	"github.com/anacrolix/missinggo/v2/filecache"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
)

// newStorageGovernor sizes the piece cache (#200): the storage setting,
// config.yaml's global_cache_size when the file sets it, else 75% of the
// metadata folder's disk. It also serves /settings/storage.
func newStorageGovernor(cfg *config.Config, settings *library.SettingsRepository) *storagecap.Governor {
	var configBytes *int64
	if bytes, ok := cfg.Torrent.GlobalCacheBytes(); ok {
		configBytes = &bytes
	}
	return storagecap.New(storagecap.Deps{
		Settings:    settings,
		DiskPath:    cfg.Torrent.MetadataFolder,
		ConfigBytes: configBytes,
	})
}

// storageCache adapts the piece cache to the Governor. When #201 swaps in
// internal/torrent/piececache, only the type and Info's Pinned change here.
func storageCache(fc *filecache.Cache) storagecap.Cache {
	return storagecap.CacheFuncs{
		InfoFunc: func() storagecap.CacheInfo {
			info := fc.Info()
			return storagecap.CacheInfo{Capacity: info.Capacity, Filled: info.Filled}
		},
		SetCapacityFunc:    fc.SetCapacity,
		TrimToCapacityFunc: fc.TrimToCapacity,
	}
}
