package web

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// runConcurrently runs each fn in its own goroutine and blocks until all
// have returned. It exists because several page handlers need multiple
// independent EduPage fetches per request (Students/Classes are answered
// straight from the cached login payload, but MyTimetable, MealsFor and
// TimetableChanges each make their own live HTTP round trip) — running them
// one after another was adding their latencies together for no reason.
func runConcurrently(fns ...func()) {
	var wg sync.WaitGroup
	wg.Add(len(fns))
	for _, fn := range fns {
		go func(fn func()) {
			defer wg.Done()
			fn()
		}(fn)
	}
	wg.Wait()
}

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
	target := dashboardTargetDate(now)

	// Students() and Notifications() are answered from the login payload
	// already sitting in memory, so they cost nothing. MyTimetable and
	// MealsFor each make their own live request to EduPage; they don't
	// depend on each other, so they run concurrently instead of back to
	// back (sess.Client is safe for this — see edupage.Client's doc comment
	// and its read-only-after-login field usage).
	var (
		students []edupage.Student
		events   []edupage.TimelineEvent
		tt       *edupage.Timetable
		meals    *edupage.Meals
	)
	runConcurrently(
		func() {
			var err error
			students, err = sess.Client.Students()
			if err != nil {
				s.logger.Warn("dashboard: fetch students failed", "error", err)
			}
		},
		func() {
			var err error
			events, err = sess.Client.Notifications()
			if err != nil {
				s.logger.Warn("dashboard: fetch notifications failed", "error", err)
			}
		},
		func() {
			var err error
			tt, err = sess.Client.MyTimetable(target)
			if err != nil {
				s.logger.Warn("dashboard: fetch timetable failed", "error", err)
			}
		},
		func() {
			var err error
			meals, err = sess.Client.MealsFor(target)
			if err != nil {
				s.logger.Warn("dashboard: fetch meals failed", "error", err)
			}
		},
	)

	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)
	notifications := buildNotificationViews(events, baseURL(sess.Subdomain), now)
	var periods []periodView
	if tt != nil {
		periods = buildPeriodViews(tt.Lessons, false)
	}

	s.render(w, r, "dashboard.html", dashboardPageData{
		Student:       studentSummaryOrNil(student.Name, studentOK),
		Notifications: notifications,
		Timetable:     periods,
		Meals:         buildMealViews(meals, now),
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
	// NextBell is the school's next ringing time. It is answered from the
	// cached login payload, so it adds no request to the page.
	NextBell *ringingView
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

	tt, err := sess.Client.MyTimetable(current)
	if err != nil {
		s.logger.Warn("timetable: fetch timetable failed", "error", err)
	}
	var periods []periodView
	if tt != nil {
		periods = buildPeriodViews(tt.Lessons, true)
	}

	// The bell schedule comes from the cached login payload, so this costs
	// no extra request; a school that publishes none simply yields nil.
	nextBell, err := sess.Client.NextRingingTime(now)
	if err != nil {
		s.logger.Warn("timetable: next ringing time failed", "error", err)
	}

	s.render(w, r, "timetable.html", timetablePageData{
		Student:     studentSummaryOrNil(student.Name, studentOK),
		Timetable:   periods,
		CurrentDate: current,
		PrevDate:    current.AddDate(0, 0, -1),
		NextDate:    current.AddDate(0, 0, 1),
		NextBell:    buildRingingView(nextBell),
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
	// OK and Error carry the outcome of an ordering write back to the page
	// after the POST/redirect/GET hop.
	OK    string
	Error string
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
		Meals:       buildMealViews(meals, now),
		CurrentDate: current,
		PrevDate:    current.AddDate(0, 0, -1),
		NextDate:    current.AddDate(0, 0, 1),
		DateRange:   dateRange,
		OK:          r.URL.Query().Get("ok"),
		Error:       r.URL.Query().Get("error"),
	})
}

// gradesPageDataFull is the view model for grades.html.
type gradesPageDataFull struct {
	Student *studentSummary
	gradesPageView

	// Term/year switcher (feature 3): the term and school year actually
	// used for this render, resolved from the "term"/"year" query params
	// with a fallback to current-year/second-term when either is absent
	// or unrecognised, so a bad or missing param never errors the page.
	SelectedTerm edupage.Term
	TermLabel    string
	IsTermFirst  bool
	IsTermSecond bool
	SelectedYear int
	PrevYear     int
	NextYear     int
}

// defaultGradesTerm is handleGrades' fallback when "term" is absent or
// unrecognised — the same term it used to hardcode.
const defaultGradesTerm = edupage.TermSecond

// parseTermQuery reads the "term" query parameter ("P1" or "P2"), falling
// back to def for anything else (missing, empty, or an unrecognised
// value) rather than erroring the page.
func parseTermQuery(raw string, def edupage.Term) edupage.Term {
	switch edupage.Term(raw) {
	case edupage.TermFirst, edupage.TermSecond:
		return edupage.Term(raw)
	default:
		return def
	}
}

// parseYearQuery reads the "year" query parameter, falling back to def when
// it is missing or not a plausible school year (EduPage itself has existed
// since 2004, so anything outside a generous window is treated as noise
// rather than a real request).
func parseYearQuery(raw string, def int) int {
	if raw == "" {
		return def
	}
	year, err := strconv.Atoi(raw)
	if err != nil || year < 2000 || year > 2100 {
		return def
	}
	return year
}

// handleGrades serves GET /grades, optionally scoped by the "term"
// (P1|P2) and "year" query params so first-term and previous-year grades,
// already reachable via MCP, are reachable from the web UI too.
func (s *Server) handleGrades(w http.ResponseWriter, r *http.Request, sess *Session) {
	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("grades: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	currentYear, err := sess.Client.SchoolYear()
	if err != nil {
		s.logger.Warn("grades: fetch school year failed", "error", err)
	}

	query := r.URL.Query()
	term := parseTermQuery(query.Get("term"), defaultGradesTerm)
	year := parseYearQuery(query.Get("year"), currentYear)

	grades, err := sess.Client.GradesForTerm(year, term)
	if err != nil {
		s.logger.Warn("grades: fetch grades failed", "error", err)
	}

	s.render(w, r, "grades.html", gradesPageDataFull{
		Student:        studentSummaryOrNil(student.Name, studentOK),
		gradesPageView: buildGradesPageView(grades),
		SelectedTerm:   term,
		TermLabel:      termLabel(term),
		IsTermFirst:    term == edupage.TermFirst,
		IsTermSecond:   term == edupage.TermSecond,
		SelectedYear:   year,
		PrevYear:       year - 1,
		NextYear:       year + 1,
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
