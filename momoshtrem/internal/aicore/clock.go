package aicore

import "time"

// The AI features' time boundaries are server-local (PLAN §6 "Time"): the
// service's TZ, which Go reads into time.Local at startup. They're computed
// here and passed to SQL as parameters, never date_trunc(…, NOW()):
// Postgres's session zone isn't TZ.

// MonthBounds is the calendar month around now, in now's location: from
// the 1st, 00:00, up to the next month's 1st (the budget's month).
func MonthBounds(now time.Time) (start, next time.Time) {
	y, m, _ := now.Date()
	start = time.Date(y, m, 1, 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 1, 0)
}

// DayBounds is the day around now, in now's location: from midnight up to
// the next midnight (Ask's daily limit, the hero's day).
func DayBounds(now time.Time) (start, next time.Time) {
	y, m, d := now.Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	return start, time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}
