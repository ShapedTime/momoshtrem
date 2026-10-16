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
