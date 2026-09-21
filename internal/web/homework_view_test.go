package web

import (
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestIsHomeworkEventType(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		want      bool
	}{
		{"homework", "homework", true},
		{"homeworkstudentstav", "homeworkstudentstav", true},
		{"etesthw", "etesthw", true},
		{"case insensitive", "HomeWork", true},
		{"unrelated type", "sprava", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHomeworkEventType(tc.eventType); got != tc.want {
				t.Errorf("isHomeworkEventType(%q) = %v, want %v", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestExtractOldVals(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		want map[string]any
	}{
		{"nil data", nil, nil},
		{"missing key", map[string]any{"other": "x"}, nil},
		{"nil value", map[string]any{"oldVals": nil}, nil},
		{
			"already decoded object",
			map[string]any{"oldVals": map[string]any{"date": "2026-01-01", "title": "Read ch.1"}},
			map[string]any{"date": "2026-01-01", "title": "Read ch.1"},
		},
		{
			"JSON-encoded string",
			map[string]any{"oldVals": `{"date":"2026-01-02","title":"Do exercises"}`},
			map[string]any{"date": "2026-01-02", "title": "Do exercises"},
		},
		{"malformed JSON string", map[string]any{"oldVals": `{not json`}, nil},
		{"empty string", map[string]any{"oldVals": "   "}, nil},
		{"unexpected shape (number)", map[string]any{"oldVals": float64(42)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractOldVals(tc.data)
			if len(got) != len(tc.want) {
				t.Fatalf("extractOldVals(%v) = %#v, want %#v", tc.data, got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("extractOldVals(%v)[%q] = %v, want %v", tc.data, k, got[k], v)
				}
			}
		})
	}
}

func TestStringFromAny(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"float", float64(3), "3"},
		{"bool", true, "true"},
		{"nested map", map[string]any{"a": 1}, ""},
		{"nested slice", []any{1, 2}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringFromAny(tc.v); got != tc.want {
				t.Errorf("stringFromAny(%v) = %q, want %q", tc.v, got, tc.want)
			}
		})
	}
}

func TestHomeworkDueDate(t *testing.T) {
	cases := []struct {
		name    string
		oldVals map[string]any
		wantOK  bool
		want    time.Time
	}{
		{"nil oldVals", nil, false, time.Time{}},
		{"missing date", map[string]any{"title": "x"}, false, time.Time{}},
		{"empty date", map[string]any{"date": ""}, false, time.Time{}},
		{"plain date", map[string]any{"date": "2026-03-04"}, true, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)},
		{"full timestamp", map[string]any{"date": "2026-03-04 10:00:00"}, true, time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)},
		{"garbage", map[string]any{"date": "not-a-date"}, false, time.Time{}},
		{"numeric date value", map[string]any{"date": float64(20260304)}, false, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := homeworkDueDate(tc.oldVals)
			if ok != tc.wantOK {
				t.Fatalf("homeworkDueDate(%v) ok = %v, want %v", tc.oldVals, ok, tc.wantOK)
			}
			if ok && !got.Equal(tc.want) {
				t.Errorf("homeworkDueDate(%v) = %v, want %v", tc.oldVals, got, tc.want)
			}
		})
	}
}

func TestHomeworkTitle(t *testing.T) {
	cases := []struct {
		name     string
		oldVals  map[string]any
		fallback string
		want     string
	}{
		{"title present", map[string]any{"title": "Read chapter 3"}, "fallback text", "Read chapter 3"},
		{"title missing uses fallback", map[string]any{}, "fallback text", "fallback text"},
		{"nil oldVals uses fallback", nil, "fallback text", "fallback text"},
		{"blank title uses fallback", map[string]any{"title": "   "}, "fallback text", "fallback text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := homeworkTitle(tc.oldVals, tc.fallback); got != tc.want {
				t.Errorf("homeworkTitle(%v, %q) = %q, want %q", tc.oldVals, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestBuildHomeworkPageViewFiltersNonHomeworkEvents(t *testing.T) {
	events := []edupage.TimelineEvent{
		{EventID: 1, EventType: "sprava", Text: "Not homework"},
		{EventID: 2, EventType: "homework", Text: "Read chapter 3"},
	}
	view := buildHomeworkPageView(events, time.Now())

	total := 0
	for _, g := range view.Groups {
		total += len(g.Items)
	}
	if total != 1 {
		t.Fatalf("expected 1 homework item, got %d", total)
	}
}

func TestBuildHomeworkPageViewGroupsByDueDateSortedAscending(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	events := []edupage.TimelineEvent{
		{
			EventID:        1,
			EventType:      "homework",
			Text:           "Later task",
			AdditionalData: map[string]any{"oldVals": map[string]any{"date": "2026-02-01", "title": "Later task"}},
		},
		{
			EventID:        2,
			EventType:      "etesthw",
			Text:           "Sooner task",
			AdditionalData: map[string]any{"oldVals": map[string]any{"date": "2026-01-15", "title": "Sooner task"}},
		},
		{
			EventID:   3,
			EventType: "homeworkstudentstav",
			Text:      "No due date task",
		},
	}
	view := buildHomeworkPageView(events, now)

	if len(view.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(view.Groups))
	}
	if !view.Groups[0].HasDueDate || view.Groups[0].Items[0].Title != "Sooner task" {
		t.Errorf("expected first group to be the soonest due date, got %+v", view.Groups[0])
	}
	if !view.Groups[1].HasDueDate || view.Groups[1].Items[0].Title != "Later task" {
		t.Errorf("expected second group to be the later due date, got %+v", view.Groups[1])
	}
	if view.Groups[2].HasDueDate {
		t.Errorf("expected the undated bucket to trail, got %+v", view.Groups[2])
	}
}

func TestBuildHomeworkPageViewDoneVsOutstanding(t *testing.T) {
	doneAt := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	events := []edupage.TimelineEvent{
		{EventID: 1, EventType: "homework", Text: "Done item", IsDone: true, DoneAt: &doneAt},
		{EventID: 2, EventType: "homework", Text: "Outstanding item"},
	}
	view := buildHomeworkPageView(events, time.Now())

	if view.DoneTotal != 1 || view.OutstandingTotal != 1 {
		t.Fatalf("expected 1 done and 1 outstanding, got done=%d outstanding=%d", view.DoneTotal, view.OutstandingTotal)
	}
	if len(view.Groups) != 1 {
		t.Fatalf("expected both undated items in a single group, got %d groups", len(view.Groups))
	}
	g := view.Groups[0]
	// Outstanding items sort before done items within a group.
	if g.Items[0].IsDone {
		t.Errorf("expected outstanding item first, got %+v", g.Items[0])
	}
	if !g.Items[1].IsDone || !g.Items[1].HasDoneAt || !g.Items[1].DoneAt.Equal(doneAt) {
		t.Errorf("expected done item second with DoneAt set, got %+v", g.Items[1])
	}
}

func TestBuildHomeworkPageViewPastDueFlag(t *testing.T) {
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	events := []edupage.TimelineEvent{
		{
			EventID:        1,
			EventType:      "homework",
			AdditionalData: map[string]any{"oldVals": map[string]any{"date": "2026-06-01"}},
		},
		{
			EventID:        2,
			EventType:      "homework",
			AdditionalData: map[string]any{"oldVals": map[string]any{"date": "2026-07-01"}},
		},
	}
	view := buildHomeworkPageView(events, now)
	if len(view.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(view.Groups))
	}
	if !view.Groups[0].IsPastDue {
		t.Errorf("expected the June 1 group to be past due")
	}
	if view.Groups[1].IsPastDue {
		t.Errorf("expected the July 1 group not to be past due")
	}
}

func TestBuildHomeworkPageViewEmpty(t *testing.T) {
	view := buildHomeworkPageView(nil, time.Now())
	if len(view.Groups) != 0 {
		t.Fatalf("expected no groups, got %d", len(view.Groups))
	}
	if view.DoneTotal != 0 || view.OutstandingTotal != 0 {
		t.Fatal("expected zero totals for an empty event list")
	}
}
