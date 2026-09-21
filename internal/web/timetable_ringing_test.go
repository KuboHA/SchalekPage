package web

import (
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestBuildRingingView(t *testing.T) {
	at := time.Date(2026, 9, 21, 8, 45, 0, 0, time.UTC)

	t.Run("nil yields nil so the strip is simply omitted", func(t *testing.T) {
		if got := buildRingingView(nil); got != nil {
			t.Errorf("buildRingingView(nil) = %v, want nil", got)
		}
	})

	t.Run("lesson bell", func(t *testing.T) {
		got := buildRingingView(&edupage.RingingTime{
			Type:   edupage.RingingLesson,
			Time:   at,
			Period: "2",
		})
		if got == nil {
			t.Fatal("expected a view")
		}
		if got.Time != "08:45" {
			t.Errorf("Time = %q, want \"08:45\"", got.Time)
		}
		if got.Period != "2" {
			t.Errorf("Period = %q, want \"2\"", got.Period)
		}
		if got.Label != "Next lesson starts" {
			t.Errorf("Label = %q", got.Label)
		}
	})

	t.Run("break bell is labelled differently", func(t *testing.T) {
		got := buildRingingView(&edupage.RingingTime{Type: edupage.RingingBreak, Time: at})
		if got == nil {
			t.Fatal("expected a view")
		}
		if got.Label != "Next break starts" {
			t.Errorf("Label = %q, want the break label", got.Label)
		}
	})
}

func TestBuildPeriodViewLessonFlags(t *testing.T) {
	start := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)
	end := start.Add(45 * time.Minute)

	cases := []struct {
		name          string
		lesson        edupage.Lesson
		wantCancelled bool
		wantEvent     bool
		wantGroups    string
	}{
		{
			name:   "ordinary lesson carries no flags",
			lesson: edupage.Lesson{StartTime: start, EndTime: end},
		},
		{
			name:          "cancelled lesson is flagged",
			lesson:        edupage.Lesson{StartTime: start, EndTime: end, IsCancelled: true},
			wantCancelled: true,
		},
		{
			name:      "event is flagged",
			lesson:    edupage.Lesson{StartTime: start, EndTime: end, IsEvent: true},
			wantEvent: true,
		},
		{
			name:       "groups are joined for display",
			lesson:     edupage.Lesson{StartTime: start, EndTime: end, Groups: []string{"A", "B"}},
			wantGroups: "A, B",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPeriodView(tc.lesson, true)
			if got.IsCancelled != tc.wantCancelled {
				t.Errorf("IsCancelled = %v, want %v", got.IsCancelled, tc.wantCancelled)
			}
			if got.IsEvent != tc.wantEvent {
				t.Errorf("IsEvent = %v, want %v", got.IsEvent, tc.wantEvent)
			}
			if got.Groups != tc.wantGroups {
				t.Errorf("Groups = %q, want %q", got.Groups, tc.wantGroups)
			}
		})
	}
}

// TestBuildPeriodView_CancelledLessonKeepsItsTime guards against conflating
// "no time data" (the pre-existing "Cancelled" TimeDisplay fallback for a
// lesson EduPage sent no start/end for) with IsCancelled (an explicitly
// removed/absent lesson). A cancelled lesson that still carries real
// start/end times must show them — the template strikes them through via
// IsCancelled, it must not fall into the missing-time branch instead.
func TestBuildPeriodView_CancelledLessonKeepsItsTime(t *testing.T) {
	start := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)
	end := start.Add(45 * time.Minute)

	l := edupage.Lesson{StartTime: start, EndTime: end, IsCancelled: true}
	pv := buildPeriodView(l, true)

	if pv.TimeDisplay != "08:00 - 08:45" {
		t.Errorf("TimeDisplay = %q, want the actual formatted time range", pv.TimeDisplay)
	}
	if !pv.IsCancelled {
		t.Error("expected IsCancelled = true")
	}
}
