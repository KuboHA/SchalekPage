package web

import (
	"net/http"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// studentSummary is the minimal student info every protected page's navbar
// needs. A nil *studentSummary means "unknown student", which templates
// must guard with {{ if .Student }}.
type studentSummary struct {
	Name string
}

func studentSummaryOrNil(name string, ok bool) *studentSummary {
	if !ok {
		return nil
	}
	return &studentSummary{Name: name}
}

// baseURL builds the EduPage origin used to resolve relative attachment
// URLs, matching app.py's `f"https://{subdomain}.edupage.org"`.
func baseURL(subdomain string) string {
	if subdomain == "" {
		return ""
	}
	return "https://" + subdomain + ".edupage.org"
}

// dashboardPageData is the view model for dashboard.html.
type dashboardPageData struct {
	Student       *studentSummary
	Notifications []*notificationView
	Timetable     []periodView
	Meals         []mealView
	ShowTomorrow  bool
}

// handleDashboard serves GET /dashboard, porting app.py's dashboard(): the
// 14:30 rollover, notification enrichment/threading, and the day's
// timetable/meals mini-cards.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()

	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("dashboard: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	events, err := sess.Client.Notifications()
	if err != nil {
		s.logger.Warn("dashboard: fetch notifications failed", "error", err)
	}
	notifications := buildNotificationViews(events, baseURL(sess.Subdomain), now)

	target := dashboardTargetDate(now)
	tt, err := sess.Client.MyTimetable(target)
	if err != nil {
		s.logger.Warn("dashboard: fetch timetable failed", "error", err)
	}
	var periods []periodView
	if tt != nil {
		periods = buildPeriodViews(tt.Lessons, false)
	}

	meals, err := sess.Client.MealsFor(target)
	if err != nil {
		s.logger.Warn("dashboard: fetch meals failed", "error", err)
	}

	s.render(w, r, "dashboard.html", dashboardPageData{
		Student:       studentSummaryOrNil(student.Name, studentOK),
		Notifications: notifications,
		Timetable:     periods,
		Meals:         buildMealViews(meals),
		ShowTomorrow:  showTomorrow(now),
	})
}

// timetablePageData is the view model for timetable.html.
type timetablePageData struct {
	Student     *studentSummary
	Timetable   []periodView
	CurrentDate time.Time
	PrevDate    time.Time
	NextDate    time.Time
}

// handleTimetable serves GET /timetable/ and /timetable/{date}.
func (s *Server) handleTimetable(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()
	current := parseDateOrToday(r.PathValue("date"), now)

	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("timetable: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)
	if !studentOK {
		// app.py redirects to the login page when the student record can't
		// be resolved at all.
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tt, err := sess.Client.MyTimetable(current)
	if err != nil {
		s.logger.Warn("timetable: fetch timetable failed", "error", err)
	}
	var periods []periodView
	if tt != nil {
		periods = buildPeriodViews(tt.Lessons, true)
	}

	s.render(w, r, "timetable.html", timetablePageData{
		Student:     studentSummaryOrNil(student.Name, true),
		Timetable:   periods,
		CurrentDate: current,
		PrevDate:    current.AddDate(0, 0, -1),
		NextDate:    current.AddDate(0, 0, 1),
	})
}

// lunchesPageData is the view model for lunches.html.
type lunchesPageData struct {
	Student     *studentSummary
	Meals       []mealView
	CurrentDate time.Time
	PrevDate    time.Time
	NextDate    time.Time
	DateRange   []time.Time
}

// handleLunches serves GET /lunches/ and /lunches/{date}.
//
// Unlike app.py's lunches.html, which computed its 7-day date strip with
// `current_date.replace(day=current_date.day + i)` — broken across month
// boundaries (e.g. day 30 + 3 is not a valid `day=` value) — the date
// range here is computed with time.AddDate, which rolls over correctly.
func (s *Server) handleLunches(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()
	current := parseDateOrToday(r.PathValue("date"), now)

	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("lunches: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	meals, err := sess.Client.MealsFor(current)
	if err != nil {
		s.logger.Warn("lunches: fetch meals failed", "error", err)
	}

	dateRange := make([]time.Time, 0, 7)
	for i := -3; i <= 3; i++ {
		dateRange = append(dateRange, current.AddDate(0, 0, i))
	}

	s.render(w, r, "lunches.html", lunchesPageData{
		Student:     studentSummaryOrNil(student.Name, studentOK),
		Meals:       buildMealViews(meals),
		CurrentDate: current,
		PrevDate:    current.AddDate(0, 0, -1),
		NextDate:    current.AddDate(0, 0, 1),
		DateRange:   dateRange,
	})
}

// gradesPageDataFull is the view model for grades.html.
type gradesPageDataFull struct {
	Student *studentSummary
	gradesPageView
}

// handleGrades serves GET /grades.
func (s *Server) handleGrades(w http.ResponseWriter, r *http.Request, sess *Session) {
	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("grades: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	year, err := sess.Client.SchoolYear()
	if err != nil {
		s.logger.Warn("grades: fetch school year failed", "error", err)
	}
	grades, err := sess.Client.GradesForTerm(year, edupage.TermSecond)
	if err != nil {
		s.logger.Warn("grades: fetch grades failed", "error", err)
	}

	s.render(w, r, "grades.html", gradesPageDataFull{
		Student:        studentSummaryOrNil(student.Name, studentOK),
		gradesPageView: buildGradesPageView(grades),
	})
}

// substitutionsPageData is the view model for substitutions.html.
type substitutionsPageData struct {
	Student      *studentSummary
	StudentClass string
	Changes      []substitutionView
	CurrentDate  time.Time
	PrevDate     time.Time
	NextDate     time.Time
}

// handleSubstitutions serves GET /substitutions/ and /substitutions/{date}.
func (s *Server) handleSubstitutions(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()
	current := parseDateOrToday(r.PathValue("date"), now)

	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("substitutions: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	var className string
	if studentOK {
		if classes, err := sess.Client.Classes(); err != nil {
			s.logger.Warn("substitutions: fetch classes failed", "error", err)
		} else {
			for _, c := range classes {
				if c.ClassID == student.ClassID {
					className = c.Short
					if className == "" {
						className = c.Name
					}
					break
				}
			}
		}
	}

	changes, err := sess.Client.TimetableChanges(current)
	if err != nil {
		s.logger.Warn("substitutions: fetch changes failed", "error", err)
	}

	s.render(w, r, "substitutions.html", substitutionsPageData{
		Student:      studentSummaryOrNil(student.Name, studentOK),
		StudentClass: className,
		Changes:      buildSubstitutionViews(changes, className),
		CurrentDate:  current,
		PrevDate:     current.AddDate(0, 0, -1),
		NextDate:     current.AddDate(0, 0, 1),
	})
}
