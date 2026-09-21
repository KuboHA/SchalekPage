package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// TestNewPagesRender executes every page template added for the new features
// against its real view model. Parsing (TestTemplatesParse) only proves the
// syntax is valid; it does not catch a field that a template references but
// the data struct does not have, which fails at execution time. This walks
// both a populated and an empty case, because the empty case exercises the
// "nothing here yet" branches that a logged-in smoke test rarely hits.
func TestNewPagesRender(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatalf("ParseTemplates() failed: %v", err)
	}

	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	student := &studentSummary{Name: "Test Student"}

	events := []edupage.TimelineEvent{
		{
			EventID:   1,
			EventType: "homework",
			Text:      "Read chapter 4",
			Timestamp: now.Add(-24 * time.Hour),
			AdditionalData: map[string]any{
				"oldVals": map[string]any{"date": "2026-09-25", "title": "Chapter 4"},
			},
		},
		{
			EventID:   2,
			EventType: "bexam",
			Text:      "Maths test",
			Timestamp: now.Add(-48 * time.Hour),
			AdditionalData: map[string]any{
				"oldVals": map[string]any{"date": "2026-09-30", "title": "Maths test"},
			},
		},
	}

	meals := &edupage.Meals{
		Lunch: &edupage.Meal{
			Date:              day,
			OrderedMeal:       "A",
			CanBeChangedUntil: ptrTime(now.Add(4 * time.Hour)),
			Menus:             []edupage.Menu{{Number: "I.", Name: "Soup"}, {Number: "II.", Name: "Pasta"}},
		},
		AfternoonSnack: &edupage.Meal{
			Date:              day,
			CanBeChangedUntil: ptrTime(now.Add(-4 * time.Hour)),
			Menus:             []edupage.Menu{{Number: "I.", Name: "Fruit"}},
		},
	}

	period := 1
	lessons := []edupage.Lesson{
		{Period: &period, StartTime: now, EndTime: now.Add(45 * time.Minute), IsCancelled: true, Groups: []string{"A"}},
		{Period: &period, StartTime: now, EndTime: now.Add(45 * time.Minute), IsEvent: true},
	}

	cases := []struct {
		name     string
		template string
		data     any
	}{
		{
			"homework populated", "homework.html",
			homeworkPageData{Student: student, homeworkPageView: buildHomeworkPageView(events, now)},
		},
		{
			"homework empty", "homework.html",
			homeworkPageData{Student: student, homeworkPageView: buildHomeworkPageView(nil, now)},
		},
		{
			"exams populated", "exams.html",
			examsPageData{Student: student, examsPageView: buildExamsPageView(events, now)},
		},
		{
			"exams empty", "exams.html",
			examsPageData{Student: student, examsPageView: buildExamsPageView(nil, now)},
		},
		{
			"lunches with ordering controls", "lunches.html",
			lunchesPageData{
				Student: student, Meals: buildMealViews(meals, now),
				CurrentDate: day, PrevDate: day.AddDate(0, 0, -1), NextDate: day.AddDate(0, 0, 1),
				DateRange: []time.Time{day},
			},
		},
		{
			"lunches with a write outcome banner", "lunches.html",
			lunchesPageData{
				Student: student, Meals: buildMealViews(meals, now),
				CurrentDate: day, PrevDate: day, NextDate: day,
				DateRange: []time.Time{day},
				OK:        "Order placed.", Error: "",
			},
		},
		{
			"timetable with cancelled lesson and bell strip", "timetable.html",
			timetablePageData{
				Student: student, Timetable: buildPeriodViews(lessons, true),
				CurrentDate: day, PrevDate: day, NextDate: day,
				NextBell: buildRingingView(&edupage.RingingTime{
					Type: edupage.RingingLesson, Time: now, Period: "2",
				}),
			},
		},
		{
			"timetable without a bell schedule", "timetable.html",
			timetablePageData{
				Student: student, Timetable: nil,
				CurrentDate: day, PrevDate: day, NextDate: day,
				NextBell: nil,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := templates.ExecuteTemplate(&buf, tc.template, tc.data); err != nil {
				t.Fatalf("executing %s: %v", tc.template, err)
			}
			if !strings.Contains(buf.String(), "</html>") {
				t.Errorf("%s rendered no closing </html>; output truncated?", tc.template)
			}
		})
	}
}
