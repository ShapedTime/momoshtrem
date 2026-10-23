package schedule

import (
	"reflect"
	"testing"
	"time"
)

// A slot's index counts one up a slot: nightly by the server-local date,
// weekly by the week, whatever the hour or a DST change.
func TestSlotIndex(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	at := func(m time.Month, d, h int, loc *time.Location) time.Time {
		return time.Date(2026, m, d, h, 0, 0, 0, loc)
	}
	for _, tt := range []struct {
		schedule string
		slot     time.Time
		want     int
	}{
		{"nightly", at(10, 16, 3, time.UTC), 20742}, // days from 1 January 1970
		{"nightly", at(10, 17, 3, time.UTC), 20743},
		{"nightly", at(10, 16, 3, berlin), 20742}, // the local date, not UTC's
		{"nightly", at(10, 16, 0, berlin), 20742}, // 15 October in UTC
		// Clocks go back on 25 October: the 25th and 26th still count one
		// apart.
		{"nightly", at(10, 25, 3, berlin), 20751},
		{"nightly", at(10, 26, 3, berlin), 20752},
		{"weekly", at(10, 18, 3, berlin), 2963}, // Sundays
		{"weekly", at(10, 25, 3, berlin), 2964},
		{"weekly", at(11, 1, 3, berlin), 2965},
	} {
		if got := slotIndex(tt.schedule, tt.slot); got != tt.want {
			t.Errorf("slotIndex(%s, %v) = %d, want %d", tt.schedule, tt.slot, got, tt.want)
		}
	}
}

// Each slot starts one member further on, wrapping; the ids aren't
// changed.
func TestRotate(t *testing.T) {
	ids := []int64{1, 3, 4}
	for _, tt := range []struct {
		ids   []int64
		index int
		want  []int64
	}{
		{ids, 0, []int64{1, 3, 4}},
		{ids, 1, []int64{3, 4, 1}},
		{ids, 2, []int64{4, 1, 3}},
		{ids, 3, []int64{1, 3, 4}},
		{ids, 20744, []int64{4, 1, 3}}, // 20744 mod 3 = 2
		{ids, -1, []int64{4, 1, 3}},    // before the epoch
		{[]int64{7}, 20742, []int64{7}},
		{[]int64{1, 2}, 20742, []int64{1, 2}},
		{[]int64{1, 2}, 20743, []int64{2, 1}},
		{nil, 5, nil},
	} {
		if got := rotate(tt.ids, tt.index); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("rotate(%v, %d) = %v, want %v", tt.ids, tt.index, got, tt.want)
		}
	}
	if !reflect.DeepEqual(ids, []int64{1, 3, 4}) {
		t.Errorf("rotate changed its input: %v", ids)
	}
}
