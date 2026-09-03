package board

import (
	"testing"
	"time"
)

var tz = time.FixedZone("-03", -3*3600)
var now = time.Date(2026, 9, 3, 12, 0, 0, 0, tz)

func TestAge(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", now, "0h"},
		{"5h", now.Add(-5 * time.Hour), "5h"},
		{"23h59m", now.Add(-23*time.Hour - 59*time.Minute), "23h"},
		{"24h", now.Add(-24 * time.Hour), "1d"},
		{"47h", now.Add(-47 * time.Hour), "1d"},
		{"3d", now.Add(-3 * 24 * time.Hour), "3d"},
		{"12d", now.Add(-12 * 24 * time.Hour), "12d"},
		{"100d", now.Add(-100 * 24 * time.Hour), "100d"},
		{"future", now.Add(2 * time.Hour), "0h"},
	}
	for _, c := range cases {
		if got := Age(now, c.t); got != c.want {
			t.Errorf("%s: Age = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDaysHours(t *testing.T) {
	if got := Days(now, now.Add(-7*24*time.Hour-time.Hour)); got != 7 {
		t.Errorf("Days = %d, want 7", got)
	}
	if got := Hours(now, now.Add(-36*time.Hour-30*time.Minute)); got != 36 {
		t.Errorf("Hours = %d, want 36", got)
	}
}

func TestDayLabel(t *testing.T) {
	if got := DayLabel(now); got != "Thu 3 Sep" {
		t.Errorf("DayLabel = %q", got)
	}
	if got := DayLabel(time.Date(2026, 8, 31, 12, 0, 0, 0, tz)); got != "Mon 31 Aug" {
		t.Errorf("DayLabel = %q", got)
	}
}

func TestISOWeekKey(t *testing.T) {
	cases := map[string]time.Time{
		"2026-W36": time.Date(2026, 8, 31, 12, 0, 0, 0, tz),
		"2026-W35": time.Date(2026, 8, 30, 12, 0, 0, 0, tz),
		"2026-W33": time.Date(2026, 8, 11, 12, 0, 0, 0, tz),
		"2026-W01": time.Date(2026, 1, 1, 12, 0, 0, 0, tz),
	}
	for want, tm := range cases {
		if got := ISOWeekKey(tm); got != want {
			t.Errorf("ISOWeekKey(%s) = %q, want %q", tm, got, want)
		}
	}
	if got := ISOWeekKey(now); got != "2026-W36" {
		t.Errorf("ISOWeekKey(now) = %q", got)
	}
}

func TestGroupOf(t *testing.T) {
	cases := []struct {
		t    time.Time
		want WeekGroup
	}{
		{now, ThisWeek},
		{time.Date(2026, 9, 2, 9, 0, 0, 0, tz), ThisWeek},
		{time.Date(2026, 8, 31, 0, 0, 0, 0, tz), ThisWeek},
		{time.Date(2026, 8, 30, 23, 59, 0, 0, tz), LastWeek},
		{time.Date(2026, 8, 24, 0, 0, 0, 0, tz), LastWeek},
		{time.Date(2026, 8, 23, 23, 59, 0, 0, tz), Earlier},
		{time.Date(2026, 8, 11, 12, 0, 0, 0, tz), Earlier},
	}
	for _, c := range cases {
		if got := GroupOf(now, c.t); got != c.want {
			t.Errorf("GroupOf(%s) = %v, want %v", c.t, got, c.want)
		}
	}
	if ThisWeek.Label() != "THIS WEEK" || LastWeek.Label() != "LAST WEEK" || Earlier.Label() != "EARLIER" {
		t.Errorf("labels: %q %q %q", ThisWeek.Label(), LastWeek.Label(), Earlier.Label())
	}
}
