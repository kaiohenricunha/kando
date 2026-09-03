package board

import (
	"fmt"
	"time"
)

// Age renders now−t as "<N>h" under 24 hours, otherwise "<N>d" (whole days, floor).
func Age(now, t time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Days is the whole number of days between t and now (floor, never negative).
func Days(now, t time.Time) int {
	d := now.Sub(t)
	if d < 0 {
		return 0
	}
	return int(d.Hours() / 24)
}

// Hours is the whole number of hours between t and now (floor, never negative).
func Hours(now, t time.Time) int {
	d := now.Sub(t)
	if d < 0 {
		return 0
	}
	return int(d.Hours())
}

// DayLabel formats t as "Mon 2 Jan" (no zero padding).
func DayLabel(t time.Time) string {
	return t.Format("Mon 2 Jan")
}

// ISOWeekKey formats t's ISO week as "2026-W36".
func ISOWeekKey(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// WeekGroup buckets a past instant relative to now's week (weeks start Monday).
type WeekGroup int

// Week groups in display order.
const (
	ThisWeek WeekGroup = iota
	LastWeek
	Earlier
)

// Label is the uppercase group heading.
func (g WeekGroup) Label() string {
	switch g {
	case ThisWeek:
		return "THIS WEEK"
	case LastWeek:
		return "LAST WEEK"
	default:
		return "EARLIER"
	}
}

// weekStart returns Monday 00:00 of the week containing t, in t's location.
func weekStart(t time.Time) time.Time {
	wd := int(t.Weekday()) // Sunday=0
	back := (wd + 6) % 7   // days since Monday
	y, m, d := t.Date()
	return time.Date(y, m, d-back, 0, 0, 0, 0, t.Location())
}

// GroupOf classifies t against now's week, using now's location for the boundaries.
func GroupOf(now, t time.Time) WeekGroup {
	start := weekStart(now)
	t = t.In(now.Location())
	if !t.Before(start) {
		return ThisWeek
	}
	if !t.Before(start.AddDate(0, 0, -7)) {
		return LastWeek
	}
	return Earlier
}
