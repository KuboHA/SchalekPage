package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// TestRenderPreview is a manual helper: with SP_PREVIEW_DIR set it renders
// every page with sample data so the design can be eyeballed in a browser.
func TestRenderPreview(t *testing.T) {
	dir := os.Getenv("SP_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SP_PREVIEW_DIR to render preview pages")
	}

	tpl, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(tpl, slog.Default())

	day := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC)
	stu := &studentSummary{Name: "Jan Marek Schalek"}

	periods := []periodView{
		{Period: "1", HasPeriod: true, TimeDisplay: "08:00 - 08:45", StartTime: "08:00", Subject: "Mathematics", HasSubject: true, Classrooms: "B214", Teachers: "A. Nováková"},
		{Period: "2", HasPeriod: true, TimeDisplay: "08:55 - 09:40", StartTime: "08:55", Subject: "Physics", HasSubject: true, Classrooms: "A102", Teachers: "P. Horváth"},
		{Period: "3", HasPeriod: true, TimeDisplay: "Cancelled", StartTime: "09:50", Subject: "", HasSubject: false, Classrooms: "", Teachers: ""},
		{Period: "4", HasPeriod: true, TimeDisplay: "10:55 - 11:40", StartTime: "10:55", Subject: "Slovak Language and Literature", HasSubject: true, Classrooms: "C7", Teachers: "M. Kováčová"},
	}

	meals := []mealView{{
		Type:        "Lunch",
		OrderedMeal: "A",
		Menus: []edupage.Menu{
			{Number: "", Name: "Broth with vegetables and noodles"},
			{Number: "I.", Name: "Roast chicken, rice, cucumber salad", Allergens: "1, 3, 7"},
			{Number: "II.", Name: "Spinach gnocchi with feta", Allergens: "1, 7"},
		},
	}}

	notes := []*notificationView{
		{EventID: 1, EventType: "sprava", IconClass: "fa-envelope", FormattedTimestamp: "2 hours ago",
			Author: "A. Nováková", Recipient: "*", Text: "Tomorrow's maths test covers chapters 4 and 5. Bring a calculator.",
			ConfirmationCount: 12,
			Attachments:       []attachmentView{{Name: "chapter-4-notes.pdf", URL: "#"}, {Name: "diagram.png", URL: "#", IsImage: true}},
			Replies: []*notificationView{
				{EventID: 2, IconClass: "fa-reply", FormattedTimestamp: "1 hour ago", Author: "P. Horváth", Text: "Calculators will be provided."},
			}},
		{EventID: 3, EventType: "vyucujuci", IconClass: "fa-bullhorn", FormattedTimestamp: "yesterday",
			Author: "School office", Text: "The library closes at 14:00 on Friday."},
	}

	pages := map[string]struct {
		name string
		data any
	}{
		"login.html":           {"login.html", loginPageData{}},
		"login-error.html":     {"login.html", loginPageData{Error: "Wrong username or password."}},
		"two_factor.html":      {"two_factor.html", twoFactorPageData{}},
		"dashboard.html":       {"dashboard.html", dashboardPageData{Student: stu, Notifications: notes, Timetable: periods, Meals: meals}},
		"timetable.html":       {"timetable.html", timetablePageData{Student: stu, Timetable: periods, CurrentDate: day, PrevDate: day.AddDate(0, 0, -1), NextDate: day.AddDate(0, 0, 1)}},
		"timetable-empty.html": {"timetable.html", timetablePageData{Student: stu, CurrentDate: day, PrevDate: day, NextDate: day}},
		"lunches.html": {"lunches.html", lunchesPageData{Student: stu, Meals: meals, CurrentDate: day,
			PrevDate: day.AddDate(0, 0, -1), NextDate: day.AddDate(0, 0, 1),
			DateRange: []time.Time{day.AddDate(0, 0, -2), day.AddDate(0, 0, -1), day, day.AddDate(0, 0, 1), day.AddDate(0, 0, 2), day.AddDate(0, 0, 3), day.AddDate(0, 0, 4)}}},
		"grades.html": {"grades.html", gradesPageDataFull{Student: stu, gradesPageView: gradesPageView{
			HasOverall: true, Overall: 1.8, OverallInt: 2, OverallCls: averageBadgeClass(1.8),
			Subjects: []gradeSubjectView{
				{Subject: "Mathematics", HasAverage: true, Average: 1.6, AvgClass: averageBadgeClass(1.6), Grades: []gradeView{
					{GradeDisplay: "1", ColorClass: gradeColorClass(1), Title: "Quadratic equations", Teacher: "A. Nováková", Date: day, Comment: "Excellent working."},
					{GradeDisplay: "2", ColorClass: gradeColorClass(2), Title: "Term test", Teacher: "A. Nováková", Date: day.AddDate(0, 0, -20)},
				}},
				{Subject: "Physics", HasAverage: true, Average: 2.4, HasPercent: true, PercentAvg: 78, AvgClass: averageBadgeClass(2.4), Grades: []gradeView{
					{GradeDisplay: "78%", ColorClass: percentColorClass(78), Title: "Optics lab", Teacher: "P. Horváth", Date: day, IsPercent: true, Percent: 78, MaxPoints: 50},
					{GradeDisplay: "4", ColorClass: gradeColorClass(4), Title: "Mechanics quiz", Teacher: "P. Horváth", Date: day.AddDate(0, 0, -9)},
					{GradeDisplay: "5", ColorClass: gradeColorClass(5), Title: "Homework check", Teacher: "P. Horváth", Date: day.AddDate(0, 0, -30)},
				}},
			}}}},
		"substitutions.html": {"substitutions.html", substitutionsPageData{Student: stu, StudentClass: "4.B", CurrentDate: day,
			PrevDate: day.AddDate(0, 0, -1), NextDate: day.AddDate(0, 0, 1),
			Changes: []substitutionView{
				{LessonN: "3", Title: "Chemistry cancelled — teacher absent", Action: "remove"},
				{LessonN: "5", Title: "Biology moved to room A210", Action: "change"},
				{LessonN: "7", Title: "Extra consultation, Mathematics", Action: "add"},
			}}},
		"substitutions-empty.html": {"substitutions.html", substitutionsPageData{Student: stu, StudentClass: "4.B", CurrentDate: day, PrevDate: day, NextDate: day}},
		"grades-empty.html":        {"grades.html", gradesPageDataFull{Student: stu}},
	}

	for file, p := range pages {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		s.render(rec, req, p.name, p.data)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", file, rec.Code, rec.Body.String())
		}
		if err := os.WriteFile(dir+"/"+file, rec.Body.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
