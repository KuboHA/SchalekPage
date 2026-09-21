package web

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestParseDateOrToday(t *testing.T) {
	now := mustDate(t, "2026-09-21").Add(10 * time.Hour)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"valid date", "2026-01-15", "2026-01-15"},
		{"empty falls back to today", "", "2026-09-21"},
		{"garbage falls back to today", "not-a-date", "2026-09-21"},
		{"wrong format falls back to today", "21/09/2026", "2026-09-21"},
		{"partial date falls back to today", "2026-09", "2026-09-21"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDateOrToday(tc.in, now)
			if got.Format(dateLayout) != tc.want {
				t.Errorf("parseDateOrToday(%q) = %s, want %s", tc.in, got.Format(dateLayout), tc.want)
			}
		})
	}
}

func TestShowTomorrowRollover(t *testing.T) {
	day := mustDate(t, "2026-09-21")

	cases := []struct {
		name string
		hm   [2]int
		want bool
	}{
		{"well before cutoff", [2]int{8, 0}, false},
		{"just before cutoff", [2]int{14, 29}, false},
		{"exactly at cutoff", [2]int{14, 30}, true},
		{"just after cutoff", [2]int{14, 31}, true},
		{"well after cutoff", [2]int{23, 59}, true},
		{"midnight", [2]int{0, 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := day.Add(time.Duration(tc.hm[0])*time.Hour + time.Duration(tc.hm[1])*time.Minute)
			if got := showTomorrow(now); got != tc.want {
				t.Errorf("showTomorrow(%v) = %v, want %v", now, got, tc.want)
			}
		})
	}
}

func TestDashboardTargetDate(t *testing.T) {
	day := mustDate(t, "2026-09-21")

	before := day.Add(9 * time.Hour) // 09:00, before cutoff
	if got := dashboardTargetDate(before); !got.Equal(day) {
		t.Errorf("before cutoff: got %v, want %v", got, day)
	}

	after := day.Add(15 * time.Hour) // 15:00, after cutoff
	want := day.AddDate(0, 0, 1)
	if got := dashboardTargetDate(after); !got.Equal(want) {
		t.Errorf("after cutoff: got %v, want %v", got, want)
	}
}

func TestDashboardTargetDateMonthRollover(t *testing.T) {
	day := mustDate(t, "2026-09-30").Add(15 * time.Hour)
	want := mustDate(t, "2026-10-01")
	if got := dashboardTargetDate(day); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
