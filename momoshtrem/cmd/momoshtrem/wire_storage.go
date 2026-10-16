package main

import (
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
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

// storageCache adapts the piece cache (#201's piececache) to the Governor:
// its cap, its fill with the pinned part, the streaming reserve (set on
// every pass, #77 P4) and its trims.
func storageCache(fc *piececache.Cache) storagecap.Cache {
	return storagecap.CacheFuncs{
		InfoFunc: func() storagecap.CacheInfo {
			info := fc.Info()
			return storagecap.CacheInfo{Capacity: info.Capacity, Filled: info.Filled, Pinned: info.Pinned}
		},
		SetCapacityFunc:    fc.SetCapacity,
		SetReserveFunc:     fc.SetReserve,
		TrimToCapacityFunc: fc.TrimToCapacity,
	}
}
