package web

import (
	"testing"
	"time"
)

func TestParseSinceOrDefault(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	wantDefault := now.Add(defaultNotificationHistoryLookback)

	cases := []struct {
		name string
		raw  string
		want time.Time
	}{
		{"empty falls back to default lookback", "", wantDefault},
		{"garbage falls back to default lookback", "not-a-date", wantDefault},
		{"valid date parsed", "2026-01-15", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSinceOrDefault(tc.raw, now); !got.Equal(tc.want) {
				t.Errorf("parseSinceOrDefault(%q, now) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
