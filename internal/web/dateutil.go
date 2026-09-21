package web

import "time"

// dateLayout is the "YYYY-MM-DD" layout used throughout EduPage and this app's
// URLs, matching Python's '%Y-%m-%d'.
const dateLayout = "2006-01-02"

// parseDateOrToday parses s as a "YYYY-MM-DD" date. An empty or unparseable
// string falls back to today (as determined by now), matching app.py's
// try/except ValueError -> date.today() behaviour. The returned time is
// truncated to a date (midnight) in the same location as now.
func parseDateOrToday(s string, now time.Time) time.Time {
	today := truncateToDate(now)
	if s == "" {
		return today
	}
	t, err := time.ParseInLocation(dateLayout, s, now.Location())
	if err != nil {
		return today
	}
	return t
}

// truncateToDate zeroes out the time-of-day component of t, keeping its
// location.
func truncateToDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// rolloverCutoffHour and rolloverCutoffMinute are the time of day (14:30, as
// in app.py's `should_show_tomorrow`) after which the dashboard and its mini
// timetable/lunch cards switch to showing tomorrow instead of today.
const (
	rolloverCutoffHour   = 14
	rolloverCutoffMinute = 30
)

// showTomorrow reports whether, at the given instant, the dashboard should
// roll over to tomorrow's schedule. It mirrors app.py's should_show_tomorrow:
// now >= today at 14:30.
func showTomorrow(now time.Time) bool {
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), rolloverCutoffHour, rolloverCutoffMinute, 0, 0, now.Location())
	return !now.Before(cutoff)
}

// dashboardTargetDate returns the date the dashboard should display: today,
// or tomorrow after the 14:30 rollover.
func dashboardTargetDate(now time.Time) time.Time {
	d := truncateToDate(now)
	if showTomorrow(now) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
