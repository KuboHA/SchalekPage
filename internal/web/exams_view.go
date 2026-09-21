// Feature 6: a forward-looking exams & tests view. Like homework, EduPage
// has no dedicated exams module — these arrive as timeline events too, but
// (unlike homework) the event's own timestamp is the exam date itself, so no
// oldVals digging is needed here.
package web

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// examEventTypes is the set of raw EduPage timeline event types that
// represent an exam or test, matched case-insensitively against
// edupage.TimelineEvent.EventType.
var examEventTypes = map[string]bool{
	"bexam":          true,
	"oexam":          true,
	"rexam":          true,
	"pexam":          true,
	"sexam":          true,
	"testing":        true,
	"testpridelenie": true,
	"testvysledok":   true,
}

// isExamEventType reports whether eventType is one of the raw EduPage
// timeline types that represent an exam or test.
func isExamEventType(eventType string) bool {
	return examEventTypes[strings.ToLower(eventType)]
}

// examItemView is a presentation-ready exam/test timeline entry.
type examItemView struct {
	EventID    int
	Title      string
	EventType  string
	TypeName   string
	IconClass  string
	AuthorName string
	Date       time.Time
}

// examsPageView is the fully precomputed view model for exams.html: upcoming
// exams sorted soonest-first, and past ones sorted most-recent-first, kept
// visually apart so a stale exam never reads as still ahead.
type examsPageView struct {
	Upcoming []examItemView
	Past     []examItemView
}

// buildExamsPageView filters events down to exam/test types and splits them
// into upcoming vs past relative to now, using each event's own Timestamp as
// the exam date. Events with no parseable timestamp are skipped outright:
// there's nothing meaningful to sort them by.
func buildExamsPageView(events []edupage.TimelineEvent, now time.Time) examsPageView {
	today := truncateToDate(now)

	var view examsPageView
	for _, e := range events {
		if !isExamEventType(e.EventType) {
			continue
		}
		if e.Timestamp.IsZero() {
			continue
		}

		item := examItemView{
			EventID:    e.EventID,
			Title:      e.Text,
			EventType:  e.EventType,
			TypeName:   edupage.EventTypeName(strings.ToLower(e.EventType)),
			IconClass:  edupage.EventTypeIcon(strings.ToLower(e.EventType)),
			AuthorName: e.AuthorName,
			Date:       e.Timestamp,
		}

		if !truncateToDate(e.Timestamp).Before(today) {
			view.Upcoming = append(view.Upcoming, item)
		} else {
			view.Past = append(view.Past, item)
		}
	}

	sort.SliceStable(view.Upcoming, func(i, j int) bool {
		return view.Upcoming[i].Date.Before(view.Upcoming[j].Date)
	})
	sort.SliceStable(view.Past, func(i, j int) bool {
		return view.Past[i].Date.After(view.Past[j].Date)
	})

	return view
}

// examsPageData is the view model for exams.html.
type examsPageData struct {
	Student *studentSummary
	examsPageView
}

// handleExams serves GET /exams: a forward-looking view of upcoming exams
// and tests, filtered from the timeline feed.
func (s *Server) handleExams(w http.ResponseWriter, r *http.Request, sess *Session) {
	now := s.now()

	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("exams: fetch students failed", "error", err)
	}
	student, studentOK := resolveStudent(students, sess.StudentID, sess.StudentName)

	events, err := sess.Client.Notifications()
	if err != nil {
		s.logger.Warn("exams: fetch notifications failed", "error", err)
	}

	s.render(w, r, "exams.html", examsPageData{
		Student:       studentSummaryOrNil(student.Name, studentOK),
		examsPageView: buildExamsPageView(events, now),
	})
}
