package web

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// periodView is a presentation-ready lesson/period, used by both the
// dashboard's mini timetable and the full /timetable page.
type periodView struct {
	Period      string
	HasPeriod   bool
	TimeDisplay string // "08:00 - 08:45", or "Cancelled" when times are missing
	StartTime   string // "08:00", empty when missing
	Subject     string // display name; empty means "no subject"
	HasSubject  bool
	Classrooms  string
	Teachers    string

	// IsCancelled marks a lesson EduPage removed or flagged absent. A
	// cancelled lesson must never render like one that is going ahead.
	IsCancelled bool
	// IsEvent marks a calendar entry (trip, meeting, ...) rather than a lesson.
	IsEvent bool
	Groups  string
}

func joinTeacherNames(teachers []edupage.Teacher) string {
	names := make([]string, len(teachers))
	for i, t := range teachers {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

func joinClassroomNames(rooms []edupage.Classroom) string {
	names := make([]string, len(rooms))
	for i, r := range rooms {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}

// subjectDisplayName returns a subject's display name, preferring Name over
// Short, exactly as app.py's fallback chain did for the (non-string) case.
func subjectDisplayName(s *edupage.Subject) string {
	if s == nil {
		return ""
	}
	if s.Name != "" {
		return s.Name
	}
	return s.Short
}

// buildPeriodView converts a Lesson into a periodView. titleCased controls
// whether the subject name is rendered in Title Case (as timetable.html
// does via the `|title` filter) or Sentence case (as dashboard.html does
// via Python's `.capitalize()`).
func buildPeriodView(l edupage.Lesson, titleCased bool) periodView {
	pv := periodView{
		Classrooms:  joinClassroomNames(l.Classrooms),
		Teachers:    joinTeacherNames(l.Teachers),
		IsCancelled: l.IsCancelled,
		IsEvent:     l.IsEvent,
		Groups:      strings.Join(l.Groups, ", "),
	}
	if l.Period != nil {
		pv.Period = strconv.Itoa(*l.Period)
		pv.HasPeriod = true
	}
	if !l.StartTime.IsZero() && !l.EndTime.IsZero() {
		pv.TimeDisplay = l.StartTime.Format("15:04") + " - " + l.EndTime.Format("15:04")
		pv.StartTime = l.StartTime.Format("15:04")
	} else {
		pv.TimeDisplay = "Cancelled"
	}
	if name := subjectDisplayName(l.Subject); name != "" {
		pv.HasSubject = true
		if titleCased {
			pv.Subject = titleCase(name)
		} else {
			pv.Subject = capitalizeFirst(name)
		}
	}
	return pv
}

func buildPeriodViews(lessons []edupage.Lesson, titleCased bool) []periodView {
	out := make([]periodView, len(lessons))
	for i, l := range lessons {
		out[i] = buildPeriodView(l, titleCased)
	}
	return out
}

// capitalizeFirst upper-cases the first rune and lower-cases the rest,
// matching Python's str.capitalize().
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// titleCase upper-cases the first rune of every space-separated word,
// matching Jinja's `|title` filter closely enough for subject names.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		lw := []rune(strings.ToLower(w))
		lw[0] = unicode.ToUpper(lw[0])
		words[i] = string(lw)
	}
	return strings.Join(words, " ")
}

// substitutionView is a presentation-ready timetable change.
type substitutionView struct {
	LessonN string
	Title   string
	Action  string
}

// buildSubstitutionViews normalizes and filters raw timetable changes to
// the student's own class, porting app.py's normalized_changes loop.
// className is the student's resolved class short name/name; when empty, no
// filtering is applied (matching app.py's `if class_name and ...`).
func buildSubstitutionViews(changes []edupage.TimetableChange, className string) []substitutionView {
	var out []substitutionView
	for _, ch := range changes {
		if className != "" && ch.ChangeClass != className {
			continue
		}
		out = append(out, substitutionView{
			LessonN: ch.LessonN,
			Title:   ch.Title,
			Action:  string(ch.Action),
		})
	}
	return out
}

// ringingView is a presentation-ready bell time for the timetable gutter.
type ringingView struct {
	Type   string
	Label  string
	Time   string
	Period string
}

// buildRingingView renders the next bell, or nil when the school published
// no bell schedule. It costs no network request — the schedule is already in
// the cached login payload.
func buildRingingView(rt *edupage.RingingTime) *ringingView {
	if rt == nil {
		return nil
	}
	label := "Next lesson starts"
	if rt.Type == edupage.RingingBreak {
		label = "Next break starts"
	}
	return &ringingView{
		Type:   string(rt.Type),
		Label:  label,
		Time:   rt.Time.Format("15:04"),
		Period: rt.Period,
	}
}
