package edupage

import (
	"testing"
	"time"
)

// clientWithZvonenia builds a minimal logged-in Client exposing the given
// raw "zvonenia" value via Data(), without any network access.
func clientWithZvonenia(zvonenia any) *Client {
	c := New(0)
	c.data = map[string]any{"zvonenia": zvonenia}
	return c
}

func TestRingingTimes_Empty(t *testing.T) {
	cases := []struct {
		name     string
		zvonenia any
	}{
		{"absent key", nil},
		{"empty array", []any{}},
		{"empty object", map[string]any{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(0)
			if tc.zvonenia != nil {
				c.data = map[string]any{"zvonenia": tc.zvonenia}
			} else {
				c.data = map[string]any{}
			}

			got, err := c.RingingTimes()
			if err != nil {
				t.Fatalf("RingingTimes() error = %v", err)
			}
			if got != nil {
				t.Errorf("RingingTimes() = %#v, want nil", got)
			}
		})
	}
}

func TestRingingTimes_NotLoggedIn(t *testing.T) {
	c := New(0)
	if _, err := c.RingingTimes(); err != ErrNotLoggedIn {
		t.Errorf("RingingTimes() error = %v, want ErrNotLoggedIn", err)
	}
	if _, err := c.NextRingingTime(time.Now()); err != ErrNotLoggedIn {
		t.Errorf("NextRingingTime() error = %v, want ErrNotLoggedIn", err)
	}
}

func TestRingingTimes_OrderedRegardlessOfInputOrder(t *testing.T) {
	// Deliberately out of order, and using the "empty object as array" quirk
	// is not applicable here since this is a non-empty list -- but the raw
	// slice order itself is scrambled to prove RingingTimes sorts.
	zvonenia := []any{
		map[string]any{"uniperiod": "3", "starttime": "09:50", "endtime": "10:35"},
		map[string]any{"uniperiod": "1", "starttime": "07:55", "endtime": "08:40"},
		map[string]any{"uniperiod": "2", "starttime": "08:50", "endtime": "09:35"},
	}
	c := clientWithZvonenia(zvonenia)

	got, err := c.RingingTimes()
	if err != nil {
		t.Fatalf("RingingTimes() error = %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("got %d entries, want 6: %#v", len(got), got)
	}

	wantPeriods := []string{"1", "1", "2", "2", "3", "3"}
	wantTypes := []RingingType{RingingLesson, RingingBreak, RingingLesson, RingingBreak, RingingLesson, RingingBreak}
	for i, rt := range got {
		if rt.Period != wantPeriods[i] {
			t.Errorf("got[%d].Period = %q, want %q", i, rt.Period, wantPeriods[i])
		}
		if rt.Type != wantTypes[i] {
			t.Errorf("got[%d].Type = %q, want %q", i, rt.Type, wantTypes[i])
		}
	}
	// Strictly increasing time-of-day.
	for i := 1; i < len(got); i++ {
		if !got[i-1].Time.Before(got[i].Time) {
			t.Errorf("entries not strictly increasing at index %d: %v then %v", i, got[i-1].Time, got[i].Time)
		}
	}
}

func TestRingingTimes_PeriodKeyFallback(t *testing.T) {
	zvonenia := []any{
		map[string]any{"period": "5", "starttime": "12:00", "endtime": "12:45"},
	}
	c := clientWithZvonenia(zvonenia)

	got, err := c.RingingTimes()
	if err != nil {
		t.Fatalf("RingingTimes() error = %v", err)
	}
	if len(got) != 2 || got[0].Period != "5" {
		t.Fatalf("got %#v, want period 5 lesson+break", got)
	}
}

func mustParseInLocal(t *testing.T, layout, value string) time.Time {
	t.Helper()
	tm, err := time.ParseInLocation(layout, value, time.UTC)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return tm
}

func TestNextRingingTime(t *testing.T) {
	zvonenia := []any{
		map[string]any{"uniperiod": "1", "starttime": "07:55", "endtime": "08:40"},
		map[string]any{"uniperiod": "2", "starttime": "08:50", "endtime": "09:35"},
	}

	cases := []struct {
		name       string
		after      time.Time
		wantType   RingingType
		wantPeriod string
		wantHHMM   string
		wantDate   string // 2006-01-02
	}{
		{
			name:       "before first bell same day",
			after:      mustParseInLocal(t, "2006-01-02 15:04", "2024-03-15 07:00"), // Friday
			wantType:   RingingLesson,
			wantPeriod: "1",
			wantHHMM:   "07:55",
			wantDate:   "2024-03-15",
		},
		{
			name:       "between start and end -> next is the break",
			after:      mustParseInLocal(t, "2006-01-02 15:04", "2024-03-15 08:00"), // Friday
			wantType:   RingingBreak,
			wantPeriod: "1",
			wantHHMM:   "08:40",
			wantDate:   "2024-03-15",
		},
		{
			name:       "past the last bell of the day rolls to next workday",
			after:      mustParseInLocal(t, "2006-01-02 15:04", "2024-03-15 23:00"), // Friday, after 09:35
			wantType:   RingingLesson,
			wantPeriod: "1",
			wantHHMM:   "07:55",
			wantDate:   "2024-03-18", // Monday
		},
		{
			name:       "starting on a Saturday rolls to Monday",
			after:      mustParseInLocal(t, "2006-01-02 15:04", "2024-03-16 09:00"), // Saturday
			wantType:   RingingLesson,
			wantPeriod: "1",
			wantHHMM:   "07:55",
			wantDate:   "2024-03-18", // Monday
		},
		{
			name:       "starting on a Sunday rolls to Monday",
			after:      mustParseInLocal(t, "2006-01-02 15:04", "2024-03-17 09:00"), // Sunday
			wantType:   RingingLesson,
			wantPeriod: "1",
			wantHHMM:   "07:55",
			wantDate:   "2024-03-18", // Monday
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := clientWithZvonenia(zvonenia)
			got, err := c.NextRingingTime(tc.after)
			if err != nil {
				t.Fatalf("NextRingingTime() error = %v", err)
			}
			if got == nil {
				t.Fatal("NextRingingTime() = nil, want a result")
			}
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tc.wantType)
			}
			if got.Period != tc.wantPeriod {
				t.Errorf("Period = %q, want %q", got.Period, tc.wantPeriod)
			}
			if gotHHMM := got.Time.Format("15:04"); gotHHMM != tc.wantHHMM {
				t.Errorf("Time = %s, want %s", gotHHMM, tc.wantHHMM)
			}
			if gotDate := got.Time.Format("2006-01-02"); gotDate != tc.wantDate {
				t.Errorf("Date = %s, want %s", gotDate, tc.wantDate)
			}
		})
	}
}

func TestNextRingingTime_NoSchedule(t *testing.T) {
	c := clientWithZvonenia([]any{})
	got, err := c.NextRingingTime(time.Now())
	if err != nil {
		t.Fatalf("NextRingingTime() error = %v", err)
	}
	if got != nil {
		t.Errorf("NextRingingTime() = %#v, want nil", got)
	}
}

func TestParseRingingTimeOfDay(t *testing.T) {
	cases := []struct {
		hhmm   string
		wantOK bool
		want   string
	}{
		{"07:55", true, "07:55"},
		{"24:00", true, "23:59"},
		{"", false, ""},
		{"garbage", false, ""},
	}
	for _, tc := range cases {
		got, ok := parseRingingTimeOfDay(tc.hhmm)
		if ok != tc.wantOK {
			t.Errorf("parseRingingTimeOfDay(%q) ok = %v, want %v", tc.hhmm, ok, tc.wantOK)
		}
		if ok && got.Format("15:04") != tc.want {
			t.Errorf("parseRingingTimeOfDay(%q) = %s, want %s", tc.hhmm, got.Format("15:04"), tc.want)
		}
	}
}

// TestRingingPeriodUsesRealKeys guards the key names real zvonenia payloads
// actually use. They label periods with short/name/id; reading only
// period/uniperiod leaves every bell with an empty period label.
func TestRingingPeriodUsesRealKeys(t *testing.T) {
	cases := []struct {
		name  string
		entry map[string]any
		want  string
	}{
		{"real payload shape", map[string]any{"id": "2", "name": "2", "short": "2", "starttime": "08:50"}, "2"},
		{"explicit period wins", map[string]any{"period": "7", "short": "2"}, "7"},
		{"uniperiod fallback", map[string]any{"uniperiod": "4"}, "4"},
		{"short before name", map[string]any{"short": "A", "name": "B"}, "A"},
		{"nothing usable", map[string]any{"starttime": "08:00"}, ""},
		{"blank values are skipped", map[string]any{"period": "  ", "id": "5"}, "5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ringingPeriod(tc.entry); got != tc.want {
				t.Errorf("ringingPeriod() = %q, want %q", got, tc.want)
			}
		})
	}
}
