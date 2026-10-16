package storagecap

import "github.com/shapedtime/momoshtrem/internal/library"

// Report is GET /settings/storage (PLAN §6 "Storage (#77)").
type Report struct {
	// LimitBytes is the stored limit: null is auto, or not set (config.yaml
	// or auto then; Source says which).
	LimitBytes          *int64 `json:"limit_bytes"`
	Source              Source `json:"source"`
	AutoBytes           int64  `json:"auto_bytes"`
	ConfigBytes         *int64 `json:"config_bytes"`
	LimitEffectiveBytes int64  `json:"limit_effective_bytes"`
	// EffectiveBytes is the cap the Governor applies to the cache.
	EffectiveBytes     int64                      `json:"effective_bytes"`
	Floored            bool                       `json:"floored"`
	FloorFreeBytes     int64                      `json:"floor_free_bytes"`
	MinBytes           int64                      `json:"min_bytes"`
	MaxBytes           int64                      `json:"max_bytes"`
	ReserveBytes       int64                      `json:"reserve_bytes"`
	Disk               Disk                       `json:"disk"`
	Usage              Usage                      `json:"usage"`
	CommittedKeptBytes int64                      `json:"committed_kept_bytes"`
	Kept               KeptReport                 `json:"kept"`
	Smart              SmartReport                `json:"smart"`
	Background         library.BackgroundSettings `json:"background"`
}

// Usage is what's on disk now: CacheBytes is the cache's fill less what
// downloads pin.
type Usage struct {
	KeptBytes  int64 `json:"kept_bytes"`
	SmartBytes int64 `json:"smart_bytes"`
	CacheBytes int64 `json:"cache_bytes"`
}

// KeptReport is the kept settings with their effective cap.
type KeptReport struct {
	MaxBytes            *int64 `json:"max_bytes"`
	EffectiveMaxBytes   int64  `json:"effective_max_bytes"`
	DefaultUntilWatched bool   `json:"default_until_watched"`
	DefaultDays         *int   `json:"default_days"`
}

// SmartReport is the smart settings with their effective cap.
type SmartReport struct {
	Enabled           bool   `json:"enabled"`
	EpisodesAhead     int    `json:"episodes_ahead"`
	MaxBytes          *int64 `json:"max_bytes"`
	EffectiveMaxBytes int64  `json:"effective_max_bytes"`
}

func newReport(in Inputs, l Limits, cache CacheInfo, usage DownloadUsage) *Report {
	s := in.Settings
	var limit *int64
	if s.LimitSet {
		limit = s.LimitBytes
	}
	windows := s.Background.Windows
	if windows == nil {
		windows = []library.DownloadWindow{}
	}
	return &Report{
		LimitBytes:          limit,
		Source:              l.Source,
		AutoBytes:           l.AutoBytes,
		ConfigBytes:         l.ConfigBytes,
		LimitEffectiveBytes: l.LimitBytes,
		EffectiveBytes:      l.EffectiveBytes,
		Floored:             l.Floored,
		FloorFreeBytes:      FloorFreeBytes,
		MinBytes:            MinBytes,
		MaxBytes:            l.MaxBytes,
		ReserveBytes:        l.ReserveBytes,
		Disk:                in.Disk,
		Usage: Usage{
			KeptBytes:  usage.KeptBytes,
			SmartBytes: usage.SmartBytes,
			CacheBytes: max(cache.Filled-cache.Pinned, 0),
		},
		CommittedKeptBytes: usage.CommittedKeptBytes,
		Kept: KeptReport{
			MaxBytes:            s.Kept.MaxBytes,
			EffectiveMaxBytes:   l.KeptMaxBytes,
			DefaultUntilWatched: s.Kept.DefaultUntilWatched,
			DefaultDays:         s.Kept.DefaultDays,
		},
		Smart: SmartReport{
			Enabled:           s.Smart.Enabled,
			EpisodesAhead:     s.Smart.EpisodesAhead,
			MaxBytes:          s.Smart.MaxBytes,
			EffectiveMaxBytes: l.SmartMaxBytes,
		},
		Background: library.BackgroundSettings{
			Enabled:             s.Background.Enabled,
			PauseWhileStreaming: s.Background.PauseWhileStreaming,
			Windows:             windows,
		},
	}
}
