package web

import (
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestIsExamEventType(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		want      bool
	}{
		{"bexam", "bexam", true},
		{"oexam", "oexam", true},
		{"rexam", "rexam", true},
		{"pexam", "pexam", true},
		{"sexam", "sexam", true},
		{"testing", "testing", true},
		{"testpridelenie", "testpridelenie", true},
		{"testvysledok", "testvysledok", true},
		{"case insensitive", "TestVysledok", true},
		{"unrelated type", "sprava", false},
		{"homework is not an exam", "homework", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isExamEventType(tc.eventType); got != tc.want {
				t.Errorf("isExamEventType(%q) = %v, want %v", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestBuildExamsPageViewFiltersNonExamEvents(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	events := []edupage.TimelineEvent{
		{EventID: 1, EventType: "sprava", Text: "Not an exam", Timestamp: now.AddDate(0, 0, 5)},
		{EventID: 2, EventType: "bexam", Text: "Big exam", Timestamp: now.AddDate(0, 0, 5)},
	}
	view := buildExamsPageView(events, now)

	total := len(view.Upcoming) + len(view.Past)
	if total != 1 {
		t.Fatalf("expected 1 exam event, got %d", total)
	}
	if view.Upcoming[0].Title != "Big exam" {
		t.Errorf("expected the exam event to survive filtering, got %+v", view.Upcoming[0])
	}
}

func TestBuildExamsPageViewSkipsZeroTimestamp(t *testing.T) {
	events := []edupage.TimelineEvent{
		{EventID: 1, EventType: "bexam", Text: "No timestamp"},
	}
	view := buildExamsPageView(events, time.Now())
	if len(view.Upcoming)+len(view.Past) != 0 {
		t.Fatalf("expected events with a zero timestamp to be skipped")
	}
}

func TestBuildExamsPageViewSplitsPastAndUpcoming(t *testing.T) {
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	events := []edupage.TimelineEvent{
		{EventID: 1, EventType: "testing", Text: "Yesterday's test", Timestamp: now.AddDate(0, 0, -1)},
		{EventID: 2, EventType: "bexam", Text: "Today's exam", Timestamp: now},
		{EventID: 3, EventType: "oexam", Text: "Next week's oral exam", Timestamp: now.AddDate(0, 0, 7)},
	}
	view := buildExamsPageView(events, now)

	if len(view.Past) != 1 || view.Past[0].Title != "Yesterday's test" {
		t.Fatalf("expected exactly the past event in Past, got %+v", view.Past)
	}
	if len(view.Upcoming) != 2 {
		t.Fatalf("expected today's and next week's exams to be upcoming, got %+v", view.Upcoming)
	}
	// Soonest-first: today's exam before next week's.
	if view.Upcoming[0].Title != "Today's exam" || view.Upcoming[1].Title != "Next week's oral exam" {
		t.Errorf("expected upcoming sorted soonest-first, got %+v", view.Upcoming)
	}
}

func TestBuildExamsPageViewPastSortedMostRecentFirst(t *testing.T) {
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	events := []edupage.TimelineEvent{
		{EventID: 1, EventType: "bexam", Text: "Three days ago", Timestamp: now.AddDate(0, 0, -3)},
		{EventID: 2, EventType: "bexam", Text: "Yesterday", Timestamp: now.AddDate(0, 0, -1)},
		{EventID: 3, EventType: "bexam", Text: "A week ago", Timestamp: now.AddDate(0, 0, -7)},
	}
	view := buildExamsPageView(events, now)

	if len(view.Past) != 3 {
		t.Fatalf("expected 3 past events, got %d", len(view.Past))
	}
	want := []string{"Yesterday", "Three days ago", "A week ago"}
	for i, w := range want {
		if view.Past[i].Title != w {
			t.Errorf("Past[%d] = %q, want %q", i, view.Past[i].Title, w)
		}
	}
}

func TestBuildExamsPageViewEmpty(t *testing.T) {
	view := buildExamsPageView(nil, time.Now())
	if len(view.Upcoming) != 0 || len(view.Past) != 0 {
		t.Fatal("expected no exams for an empty event list")
	}
}
