package edupage

import (
	"sort"
	"strings"
	"time"
)

// RingingType distinguishes what a RingingTime announces.
type RingingType string

const (
	// RingingLesson marks the bell that starts a lesson.
	RingingLesson RingingType = "lesson"
	// RingingBreak marks the bell that starts a break (the end of a lesson).
	RingingBreak RingingType = "break"
)

// RingingTime is one bell of the school's daily ringing schedule
// (data["zvonenia"] in the login payload): a period boundary, with the
// time of day it rings and which period it belongs to.
//
// EduPage's zvonenia table carries no date — it's the same bell schedule
// every school day — so Time's date component is not meaningful on its own;
// see RingingTimes.
type RingingTime struct {
	Type   RingingType
	Time   time.Time
	Period string
}

// ringingReferenceDay is the arbitrary date RingingTime.Time values returned
// by RingingTimes are anchored to, since zvonenia carries no date of its
// own. Only the time-of-day component is meaningful for those values.
var ringingReferenceDay = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// ringingBell is a school period's parsed start/end bell, used internally by
// both RingingTimes and NextRingingTime.
type ringingBell struct {
	period  string
	start   time.Time
	startOK bool
	end     time.Time
	endOK   bool
}

// ringingBells resolves data["zvonenia"] into a slice of ringingBell,
// ordered chronologically by start (falling back to end) time — the raw
// data is not guaranteed to arrive in order, since EduPage sometimes sends
// an empty-array-as-object ({}) whose iteration order Go does not preserve.
// A nil slice (not an error) means the school has published no bell
// schedule at all.
func (c *Client) ringingBells() ([]ringingBell, error) {
	data := c.Data()
	if data == nil {
		return nil, ErrNotLoggedIn
	}

	raw := pickSlice(data["zvonenia"])
	if len(raw) == 0 {
		return nil, nil
	}

	bells := make([]ringingBell, 0, len(raw))
	for _, r := range raw {
		entry := pickMap(r)
		if entry == nil {
			continue
		}

		b := ringingBell{period: ringingPeriod(entry)}
		b.start, b.startOK = parseRingingTimeOfDay(pickString(entry["starttime"]))
		b.end, b.endOK = parseRingingTimeOfDay(pickString(entry["endtime"]))
		if !b.startOK && !b.endOK {
			continue
		}
		bells = append(bells, b)
	}
	if len(bells) == 0 {
		return nil, nil
	}

	sort.Slice(bells, func(i, j int) bool {
		return bellSortKey(bells[i]).Before(bellSortKey(bells[j]))
	})

	return bells, nil
}

// bellSortKey returns whichever of a bell's start/end times is available,
// preferring start, for ordering purposes.
func bellSortKey(b ringingBell) time.Time {
	if b.startOK {
		return b.start
	}
	return b.end
}

// ringingPeriod extracts the period identifier from a raw zvonenia entry.
// EduPage isn't consistent about the key across schools, so both observed
// spellings are tried before giving up and returning "".
// ringingPeriod returns the period label for a bell entry. Real zvonenia
// payloads label periods with "short"/"name"/"id" (e.g. "0", "1", "2"); the
// "period"/"uniperiod" keys are checked first only because other EduPage
// payloads use them.
func ringingPeriod(entry map[string]any) string {
	for _, key := range []string{"period", "uniperiod", "short", "name", "id"} {
		if p := strings.TrimSpace(pickString(entry[key])); p != "" {
			return p
		}
	}
	return ""
}

// parseRingingTimeOfDay parses an EduPage "HH:MM" time-of-day string (same
// format and "24:00" → "23:59" normalization as combineDayTime uses for
// lessons) onto ringingReferenceDay. It returns ok=false for an empty or
// malformed string.
func parseRingingTimeOfDay(hhmm string) (time.Time, bool) {
	t := combineDayTime(ringingReferenceDay, hhmm)
	return t, !t.IsZero()
}

// RingingTimes returns the school's full bell schedule as an ordered list
// of lesson/break bells, one pair per zvonenia period entry (lesson-start,
// then break-start i.e. lesson-end). This makes zero HTTP requests — the
// data is already in the login payload.
//
// Because zvonenia carries no date, the returned RingingTime.Time values
// are anchored to an arbitrary reference day; callers that only care about
// time-of-day (e.g. laying out a timetable gutter) should read
// Time.Hour()/Time.Minute() or format with "15:04" rather than the date.
//
// It returns (nil, nil) when the school has published no bell schedule.
func (c *Client) RingingTimes() ([]RingingTime, error) {
	bells, err := c.ringingBells()
	if err != nil {
		return nil, err
	}
	if len(bells) == 0 {
		return nil, nil
	}

	out := make([]RingingTime, 0, len(bells)*2)
	for _, b := range bells {
		if b.startOK {
			out = append(out, RingingTime{Type: RingingLesson, Time: b.start, Period: b.period})
		}
		if b.endOK {
			out = append(out, RingingTime{Type: RingingBreak, Time: b.end, Period: b.period})
		}
	}

	return out, nil
}

// nextWorkday rolls t forward past a weekend, mirroring the Python
// reference's RingingTimes.__get_next_workday: a Saturday rolls to the
// following Monday at midnight, a Sunday to the following Monday at
// midnight, and any other day is returned unchanged (time-of-day intact).
func nextWorkday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return startOfDay(t).AddDate(0, 0, 2)
	case time.Sunday:
		return startOfDay(t).AddDate(0, 0, 1)
	default:
		return t
	}
}

// startOfDay returns t truncated to midnight, in t's own location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// timeOfDay reprojects t's hour/minute/second/nanosecond onto
// ringingReferenceDay, so it can be compared against bells parsed by
// parseRingingTimeOfDay regardless of t's actual calendar date.
func timeOfDay(t time.Time) time.Time {
	return time.Date(
		ringingReferenceDay.Year(), ringingReferenceDay.Month(), ringingReferenceDay.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		time.UTC,
	)
}

// atTimeOfDay combines day's date with tod's time-of-day, in day's location.
func atTimeOfDay(day, tod time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), tod.Hour(), tod.Minute(), tod.Second(), tod.Nanosecond(), day.Location())
}

// maxRingingDaysAhead bounds how far NextRingingTime will roll forward
// looking for a bell before giving up. A school week plus slack is far more
// than the schedule should ever need — real data resolves within one or two
// iterations — this only guards against a pathological/empty schedule.
const maxRingingDaysAhead = 8

// NextRingingTime returns the next bell — the start of a lesson, or the
// start of a break — strictly after the given time, skipping weekends (a
// Saturday or Sunday rolls forward to the next Monday at midnight before
// bells are even considered). If after is already past the last bell for
// its day, it rolls into the next workday at midnight and tries again.
//
// It returns (nil, nil) when the school has published no bell schedule at
// all (an empty or absent "zvonenia" table) — this is not an error.
func (c *Client) NextRingingTime(after time.Time) (*RingingTime, error) {
	bells, err := c.ringingBells()
	if err != nil {
		return nil, err
	}
	if len(bells) == 0 {
		return nil, nil
	}

	cursor := after
	for i := 0; i < maxRingingDaysAhead; i++ {
		cursor = nextWorkday(cursor)
		cursorTOD := timeOfDay(cursor)

		for _, b := range bells {
			if b.startOK && cursorTOD.Before(b.start) {
				return &RingingTime{Type: RingingLesson, Time: atTimeOfDay(cursor, b.start), Period: b.period}, nil
			}
			if b.endOK && cursorTOD.Before(b.end) {
				return &RingingTime{Type: RingingBreak, Time: atTimeOfDay(cursor, b.end), Period: b.period}, nil
			}
		}

		// Past the last bell for cursor's day: move to the next calendar
		// day at midnight and let the top of the next iteration re-check
		// for a weekend.
		cursor = startOfDay(cursor).AddDate(0, 0, 1)
	}

	return nil, nil
}
