package schedule

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// SlotHour is when scheduled runs start: 03:00 server-local (the
// design's), nightly or, weekly, on Sunday.
const SlotHour = 3

// NextSlot is the schedule's first slot strictly after now, in now's
// location (server-local: time.Local); ok is false for manual (or an
// unknown schedule).
func NextSlot(schedule string, now time.Time) (time.Time, bool) {
	y, m, d := now.Date()
	slot := time.Date(y, m, d, SlotHour, 0, 0, 0, now.Location())
	switch schedule {
	case recommend.ScheduleNightly:
		if !slot.After(now) {
			slot = time.Date(y, m, d+1, SlotHour, 0, 0, 0, now.Location())
		}
		return slot, true
	case recommend.ScheduleWeekly:
		ahead := (int(time.Sunday) - int(now.Weekday()) + 7) % 7
		slot = time.Date(y, m, d+ahead, SlotHour, 0, 0, 0, now.Location())
		if !slot.After(now) {
			slot = time.Date(y, m, d+ahead+7, SlotHour, 0, 0, 0, now.Location())
		}
		return slot, true
	}
	return time.Time{}, false
}

// LastSlot is the schedule's latest slot at or before now; ok is false for
// manual.
func LastSlot(schedule string, now time.Time) (time.Time, bool) {
	y, m, d := now.Date()
	slot := time.Date(y, m, d, SlotHour, 0, 0, 0, now.Location())
	switch schedule {
	case recommend.ScheduleNightly:
		if slot.After(now) {
			slot = time.Date(y, m, d-1, SlotHour, 0, 0, 0, now.Location())
		}
		return slot, true
	case recommend.ScheduleWeekly:
		back := (int(now.Weekday()) - int(time.Sunday) + 7) % 7
		slot = time.Date(y, m, d-back, SlotHour, 0, 0, 0, now.Location())
		if slot.After(now) {
			slot = time.Date(y, m, d-back-7, SlotHour, 0, 0, 0, now.Location())
		}
		return slot, true
	}
	return time.Time{}, false
}

// slotIndex numbers a slot so that each next slot counts one up: the days
// from the Unix epoch to the slot's server-local date, nightly, or the
// weeks, weekly. Calendar days, not 24-hour spans: a DST change doesn't
// skip or repeat a number.
func slotIndex(schedule string, slot time.Time) int {
	y, m, d := slot.Date()
	days := int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / (24 * 60 * 60))
	if schedule == recommend.ScheduleWeekly {
		return days / 7
	}
	return days
}

// rotate is ids from the k-th on, then the ones before it, k being index
// mod len(ids): the order a slot queues the members in, so each next slot
// starts one member further on.
func rotate(ids []int64, index int) []int64 {
	if len(ids) == 0 {
		return nil
	}
	k := index % len(ids)
	if k < 0 {
		k += len(ids)
	}
	return append(append(make([]int64, 0, len(ids)), ids[k:]...), ids[:k]...)
}
