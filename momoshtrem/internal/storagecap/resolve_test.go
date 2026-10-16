package storagecap

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

func gib(n float64) int64           { return int64(n * float64(GiB)) }
func ptr[T any](v T) *T             { return &v }
func disk(total, free float64) Disk { return Disk{TotalBytes: gib(total), FreeBytes: gib(free)} }

// withLimit is the settings after a PUT of limit_bytes (nil: auto).
func withLimit(limit *int64) library.StorageSettings {
	s := library.DefaultStorageSettings()
	s.LimitBytes, s.LimitSet = limit, true
	return s
}

// The VM's disk: 200 GiB, 160 free, 37 in the cache.
var vm = Inputs{Settings: library.DefaultStorageSettings(), Disk: disk(200, 160), FilledBytes: gib(37)}

func TestResolve_Precedence(t *testing.T) {
	config := ptr(int64(53760) << 20) // global_cache_size: 53760 (52.5 GiB)
	keptOnly := library.DefaultStorageSettings()
	keptOnly.Kept.DefaultDays = ptr(90) // a document without limit_bytes
	tests := []struct {
		name       string
		settings   library.StorageSettings
		config     *int64
		disk       Disk
		wantSource Source
		wantLimit  int64
	}{
		{"nothing set: auto, 75% of the disk", library.DefaultStorageSettings(), nil, vm.Disk, SourceAuto, gib(150)},
		{"config.yaml sets it", library.DefaultStorageSettings(), config, vm.Disk, SourceConfig, 53760 << 20},
		{"the setting wins over config.yaml", withLimit(ptr(gib(100))), config, vm.Disk, SourceSetting, gib(100)},
		{"a null setting is auto, whatever config.yaml says", withLimit(nil), config, vm.Disk, SourceAuto, gib(150)},
		{"a document without limit_bytes falls through to config.yaml", keptOnly, config, vm.Disk, SourceConfig, 53760 << 20},
		{"auto follows the disk", library.DefaultStorageSettings(), nil, disk(1000, 900), SourceAuto, gib(750)},
		{"a setting above 90% of the disk is held there", withLimit(ptr(gib(500))), nil, vm.Disk, SourceSetting, gib(180)},
		{"a config below 4 GiB is held there", library.DefaultStorageSettings(), ptr(gib(1)), vm.Disk, SourceConfig, MinBytes},
		{"auto on a small disk is at least 4 GiB", library.DefaultStorageSettings(), nil, disk(4, 4), SourceAuto, MinBytes},
		{"an unreadable disk: auto is 4 GiB", library.DefaultStorageSettings(), nil, Disk{}, SourceAuto, MinBytes},
		{"an unreadable disk keeps the setting, unbounded", withLimit(ptr(gib(500))), nil, Disk{}, SourceSetting, gib(500)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Resolve(Inputs{Settings: tt.settings, ConfigBytes: tt.config, Disk: tt.disk})
			if l.Source != tt.wantSource || l.LimitBytes != tt.wantLimit {
				t.Errorf("Resolve = %s %d, want %s %d", l.Source, l.LimitBytes, tt.wantSource, tt.wantLimit)
			}
			if l.ConfigBytes != tt.config {
				t.Errorf("ConfigBytes = %v, want %v", l.ConfigBytes, tt.config)
			}
		})
	}
}

func TestResolve_VM(t *testing.T) {
	l := Resolve(vm)
	want := Limits{
		Source: SourceAuto, AutoBytes: gib(150), MaxBytes: gib(180), LimitBytes: gib(150),
		EffectiveBytes: gib(150), ReserveBytes: gib(20), KeptMaxBytes: gib(75), SmartMaxBytes: gib(22.5),
	}
	if l != want {
		t.Errorf("Resolve(vm) = %+v\nwant %+v", l, want)
	}
}

// The floor: the cache grows only into the free space above 5 GiB, and is
// never capped below 4 GiB.
func TestResolve_Floor(t *testing.T) {
	tests := []struct {
		name        string
		in          Inputs
		wantEff     int64
		wantFloored bool
	}{
		{"room to spare", vm, gib(150), false},
		{"the disk is short: filled + free − 5 GiB", Inputs{Settings: vm.Settings, Disk: disk(200, 10), FilledBytes: gib(140)}, gib(145), true},
		{"exactly at the limit", Inputs{Settings: vm.Settings, Disk: disk(200, 15), FilledBytes: gib(140)}, gib(150), false},
		{"never below 4 GiB", Inputs{Settings: vm.Settings, Disk: disk(200, 2), FilledBytes: gib(1)}, MinBytes, true},
		{"a 4 GiB limit isn't floored below itself", Inputs{Settings: vm.Settings, ConfigBytes: ptr(MinBytes), Disk: disk(200, 0)}, MinBytes, false},
		{"an unreadable disk has no floor", Inputs{Settings: withLimit(ptr(gib(50))), FilledBytes: gib(10)}, gib(50), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Resolve(tt.in)
			if l.EffectiveBytes != tt.wantEff || l.Floored != tt.wantFloored {
				t.Errorf("effective = %d (floored %v), want %d (floored %v)", l.EffectiveBytes, l.Floored, tt.wantEff, tt.wantFloored)
			}
		})
	}
}

// The reserve and the download caps: kept + smart fits the limit less the
// reserve, smart giving way first. The floor doesn't change them.
func TestResolve_DownloadMaxima(t *testing.T) {
	settings := func(kept, smart *int64) library.StorageSettings {
		s := library.DefaultStorageSettings()
		s.Kept.MaxBytes, s.Smart.MaxBytes = kept, smart
		return s
	}
	tests := []struct {
		name                         string
		in                           Inputs
		wantReserve, wantKept, wantS int64
	}{
		{"defaults: 50% and 15%; the reserve is 20 GiB at most", vm, gib(20), gib(75), gib(22.5)},
		{"a small limit: the reserve is 25%", Inputs{Settings: withLimit(ptr(gib(40))), Disk: vm.Disk}, gib(10), gib(20), gib(6)},
		{"explicit caps that fit", Inputs{Settings: settings(ptr(gib(100)), ptr(gib(30))), Disk: vm.Disk}, gib(20), gib(100), gib(30)},
		{"too much: smart shrinks first", Inputs{Settings: settings(ptr(gib(120)), ptr(gib(30))), Disk: vm.Disk}, gib(20), gib(120), gib(10)},
		{"kept alone over the room: kept shrinks, smart gets nothing", Inputs{Settings: settings(ptr(gib(200)), nil), Disk: vm.Disk}, gib(20), gib(130), 0},
		{"an auto limit that shrank under an explicit kept cap", Inputs{Settings: settings(ptr(gib(60)), nil), Disk: disk(100, 90)}, gib(18.75), gib(56.25), 0},
		{"zero caps", Inputs{Settings: settings(ptr(int64(0)), ptr(int64(0))), Disk: vm.Disk}, gib(20), 0, 0},
		{"floored: the caps follow the limit, not the floor", Inputs{Settings: vm.Settings, Disk: disk(200, 1), FilledBytes: gib(50)}, gib(20), gib(75), gib(22.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Resolve(tt.in)
			if l.ReserveBytes != tt.wantReserve || l.KeptMaxBytes != tt.wantKept || l.SmartMaxBytes != tt.wantS {
				t.Errorf("reserve, kept, smart = %d, %d, %d; want %d, %d, %d",
					l.ReserveBytes, l.KeptMaxBytes, l.SmartMaxBytes, tt.wantReserve, tt.wantKept, tt.wantS)
			}
			if l.KeptMaxBytes+l.SmartMaxBytes > l.LimitBytes-l.ReserveBytes {
				t.Error("kept + smart takes the reserve")
			}
		})
	}
}
