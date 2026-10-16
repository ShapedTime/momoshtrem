// Package storagecap decides how big the piece cache may grow, and keeps
// it there (#77 P2, PLAN §6 "Storage (#77)").
//
// The limit comes, in order, from the storage setting (once a PUT has sent
// limit_bytes; null there means auto), config.yaml's
// torrent.global_cache_size (only when the file sets it), and auto: 75% of
// the disk holding the metadata folder, so it follows the disk when it
// grows. It's held between 4 GiB and 90% of the disk.
//
// The cap applied to the cache, the effective size, is the limit unless
// the disk runs short: it never leaves less than 5 GiB of the disk free,
// and never goes below 4 GiB. Kept and smart downloads (#77 P4, P5) each
// get a cap inside the limit, leaving the streaming cache a reserve.
//
// The Governor applies the effective size: at startup, every minute, and
// after a PUT, lowering it in steps of at most 2 GiB. It also serves GET
// and PUT /settings/storage.
package storagecap

import (
	"github.com/shapedtime/momoshtrem/internal/library"
)

// GiB is 2^30 bytes.
const GiB int64 = 1 << 30

// The limits' constants (PLAN §6 "Storage (#77)").
const (
	// MinBytes is the least the cache is ever capped at.
	MinBytes = 4 * GiB
	// FloorFreeBytes is what the cache always leaves free on the disk.
	FloorFreeBytes = 5 * GiB
	// MaxReserveBytes bounds the streaming reserve, 25% of the limit
	// otherwise.
	MaxReserveBytes = 20 * GiB
	// TrimStepBytes is the most one trim lowers the cache by.
	TrimStepBytes = 2 * GiB
)

// The fractions, in percent: of the disk for auto and the maximum, of the
// limit for the reserve and the default caps.
const (
	autoPercent    = 75
	maxPercent     = 90
	reservePercent = 25
	keptPercent    = 50
	smartPercent   = 15
)

// Source is where the limit comes from.
type Source string

const (
	SourceSetting Source = "setting"
	SourceConfig  Source = "config"
	SourceAuto    Source = "auto"
)

// Disk is the file system holding the cache.
type Disk struct {
	TotalBytes int64 `json:"total_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
}

// Inputs are what Resolve decides from.
type Inputs struct {
	Settings library.StorageSettings
	// ConfigBytes is config.yaml's global_cache_size; nil when the file
	// doesn't set it.
	ConfigBytes *int64
	// Disk is the metadata folder's file system; zero when it can't be
	// read, which leaves out auto (4 GiB then), the maximum and the floor.
	Disk Disk
	// FilledBytes is the cache's fill now.
	FilledBytes int64
}

// Limits are Resolve's answer.
type Limits struct {
	Source Source
	// AutoBytes is 75% of the disk, MaxBytes 90%.
	AutoBytes int64
	MaxBytes  int64
	// ConfigBytes is config.yaml's limit, when the file sets one.
	ConfigBytes *int64
	// LimitBytes is the limit after precedence, between MinBytes and
	// MaxBytes: limit_effective_bytes.
	LimitBytes int64
	// EffectiveBytes is the cap for the cache now: the limit, or less
	// while the free-space floor holds it (Floored).
	EffectiveBytes int64
	Floored        bool
	// ReserveBytes is the streaming reserve, min(20 GiB, 25% of the
	// limit): kept and smart downloads never take it.
	ReserveBytes int64
	// KeptMaxBytes and SmartMaxBytes are the caps of kept and smart
	// downloads: their settings, or 50% and 15% of the limit, shrunk
	// (smart first) to fit the limit less the reserve.
	KeptMaxBytes  int64
	SmartMaxBytes int64
}

// Resolve works out the limits.
func Resolve(in Inputs) Limits {
	l := Limits{
		AutoBytes:   in.Disk.TotalBytes * autoPercent / 100,
		MaxBytes:    in.Disk.TotalBytes * maxPercent / 100,
		ConfigBytes: in.ConfigBytes,
	}
	var want int64
	switch s := in.Settings; {
	case s.LimitSet && s.LimitBytes != nil:
		l.Source, want = SourceSetting, *s.LimitBytes
	case s.LimitSet: // null: auto, whatever config.yaml says
		l.Source, want = SourceAuto, l.AutoBytes
	case in.ConfigBytes != nil:
		l.Source, want = SourceConfig, *in.ConfigBytes
	default:
		l.Source, want = SourceAuto, l.AutoBytes
	}
	if l.MaxBytes > 0 {
		want = min(want, l.MaxBytes)
	}
	l.LimitBytes = max(want, MinBytes)

	l.EffectiveBytes = l.LimitBytes
	if in.Disk.TotalBytes > 0 {
		// The cache may grow into the free space, less the floor.
		room := in.FilledBytes + in.Disk.FreeBytes - FloorFreeBytes
		l.EffectiveBytes = max(min(l.LimitBytes, room), MinBytes)
	}
	l.Floored = l.EffectiveBytes < l.LimitBytes

	l.ReserveBytes = min(MaxReserveBytes, l.LimitBytes*reservePercent/100)
	l.KeptMaxBytes, l.SmartMaxBytes = downloadMaxima(in.Settings, l.LimitBytes, l.ReserveBytes)
	return l
}

// downloadMaxima are the kept and smart caps: kept + smart never exceeds
// the limit less the reserve, smart giving way first.
func downloadMaxima(s library.StorageSettings, limit, reserve int64) (kept, smart int64) {
	room := limit - reserve
	kept, smart = limit*keptPercent/100, limit*smartPercent/100
	if s.Kept.MaxBytes != nil {
		kept = *s.Kept.MaxBytes
	}
	if s.Smart.MaxBytes != nil {
		smart = *s.Smart.MaxBytes
	}
	kept = max(min(kept, room), 0)
	smart = max(min(smart, room-kept), 0)
	return kept, smart
}
