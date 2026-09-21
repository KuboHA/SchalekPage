package edupage

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MyTimetable returns the logged-in user's own timetable ("my timetable") for
// day. It returns (nil, nil) when EduPage simply has no plan for that day.
//
// NOTE: unlike the Python reference's Timetables.get_my_timetable, this does
// not follow a parent account's "selected child" (there is no entry point
// for that in this client), so it always fetches the logged-in account's own
// plan.
func (c *Client) MyTimetable(day time.Time) (*Timetable, error) {
	plan, err := c.dayPlan(day)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, nil
	}

	subjects, err := subjectsByID(c)
	if err != nil {
		return nil, err
	}
	teachers, err := teachersByID(c)
	if err != nil {
		return nil, err
	}
	classrooms, err := classroomsByID(c)
	if err != nil {
		return nil, err
	}

	lessons := parseTimetablePlan(plan, day, subjects, teachers, classrooms)
	return &Timetable{Date: day, Lessons: lessons}, nil
}

// subjectsByID resolves the school's subjects once and indexes them by
// SubjectID, so per-lesson/per-grade lookups are O(1) map reads instead of a
// linear scan through c.Subjects() for every lesson.
func subjectsByID(c *Client) (map[int]Subject, error) {
	list, err := c.Subjects()
	if err != nil {
		return nil, fmt.Errorf("edupage: resolve subjects: %w", err)
	}
	byID := make(map[int]Subject, len(list))
	for _, s := range list {
		byID[s.SubjectID] = s
	}
	return byID, nil
}

// teachersByID resolves the school's teachers once and indexes them by
// PersonID; see subjectsByID.
func teachersByID(c *Client) (map[int]Teacher, error) {
	list, err := c.Teachers()
	if err != nil {
		return nil, fmt.Errorf("edupage: resolve teachers: %w", err)
	}
	byID := make(map[int]Teacher, len(list))
	for _, t := range list {
		byID[t.PersonID] = t
	}
	return byID, nil
}

// classroomsByID resolves the school's classrooms once and indexes them by
// ClassroomID; see subjectsByID.
func classroomsByID(c *Client) (map[string]Classroom, error) {
	list, err := c.Classrooms()
	if err != nil {
		return nil, fmt.Errorf("edupage: resolve classrooms: %w", err)
	}
	byID := make(map[string]Classroom, len(list))
	for _, cr := range list {
		byID[cr.ClassroomID] = cr
	}
	return byID, nil
}

// dayPlan fetches and decodes the raw "plan" array for day from EduPage's
// day-plan endpoint. It mirrors Timetables.__get_date_plan from the Python
// reference: first a GET to scrape a "gpid"/"gsh" pair out of an HTML page,
// then a POST whose JS-ish response embeds the actual plan as JSON.
//
// It returns (nil, nil) when the response doesn't contain data for day.
func (c *Client) dayPlan(day time.Time) ([]any, error) {
	csrfBody, err := c.Get("/dashboard/eb.php?mode=ttday")
	if err != nil {
		return nil, fmt.Errorf("edupage: fetch timetable page: %w", err)
	}
	csrfText := string(csrfBody)

	gpidStr, ok := extractBetween(csrfText, "gpid=", "&")
	if !ok {
		return nil, fmt.Errorf("edupage: timetable: could not find gpid in response")
	}
	gsh, ok := extractBetween(csrfText, "gsh=", `"`)
	if !ok {
		return nil, fmt.Errorf("edupage: timetable: could not find gsh in response")
	}

	gpid, err := strconv.Atoi(strings.TrimSpace(gpidStr))
	if err != nil {
		return nil, fmt.Errorf("edupage: timetable: invalid gpid %q: %w", gpidStr, err)
	}

	values := url.Values{
		"gpid":    {strconv.Itoa(gpid + 1)},
		"gsh":     {gsh},
		"action":  {"loadData"},
		"user":    {c.UserID()},
		"changes": {"{}"},
		"date":    {day.Format(eduDateLayout)},
		"dateto":  {day.Format(eduDateLayout)},
		"_LJSL":   {"4096"},
	}

	body, err := c.PostForm("/gcall", values)
	if err != nil {
		return nil, fmt.Errorf("edupage: fetch timetable data: %w", err)
	}

	jsonPart, ok := extractDayPlanJSON(string(body), c.UserID())
	if !ok {
		// Response didn't have the expected shape; treat as "no data".
		return nil, nil
	}

	var payload struct {
		Dates map[string]struct {
			Plan []any `json:"plan"`
		} `json:"dates"`
	}
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return nil, fmt.Errorf("edupage: decode timetable payload: %w", err)
	}

	dayData, ok := payload.Dates[day.Format(eduDateLayout)]
	if !ok {
		return nil, nil
	}

	return dayData.Plan, nil
}

// extractDayPlanJSON pulls the JSON object out of the gcall response, which
// looks like: `..."<userid>",{"dates": {...}},[...]` — i.e. it is not valid
// JSON on its own, but contains one embedded as a JS literal.
func extractDayPlanJSON(text, userID string) (string, bool) {
	marker := userID + `",`
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}
	rest := text[idx+len(marker):]

	end := strings.LastIndex(rest, ",[")
	if end < 0 {
		return "", false
	}

	return rest[:end], true
}

// parseTimetablePlan converts a raw "plan" array (as returned by EduPage) for
// the given day into a slice of Lesson values. Placeholder "add lesson" rows
// are skipped, as in the Python reference.
//
// subjects, teachers and classrooms are ID-keyed lookup tables (see
// subjectsByID/teachersByID/classroomsByID) used to resolve names. When an id
// referenced by a lesson has no match in the corresponding table, the
// resulting Subject/Teacher/Classroom keeps the id but has an empty name —
// it is never dropped and never causes an error.
func parseTimetablePlan(
	plan []any,
	day time.Time,
	subjects map[int]Subject,
	teachers map[int]Teacher,
	classrooms map[string]Classroom,
) []Lesson {
	lessons := make([]Lesson, 0, len(plan))

	for _, raw := range plan {
		item := pickMap(raw)
		if item == nil {
			continue
		}

		if headerRaw, has := item["header"]; has {
			header := pickSlice(headerRaw)
			skip := len(header) == 0
			if !skip {
				if first := pickMap(header[0]); first != nil && pickString(first["cmd"]) == "addlesson_t" {
					skip = true
				}
			}
			if skip {
				continue
			}
		}

		lesson := Lesson{
			StartTime:    combineDayTime(day, pickString(item["starttime"])),
			EndTime:      combineDayTime(day, pickString(item["endtime"])),
			OnlineLesson: pickString(item["ol_url"]),
			Curriculum:   extractCurriculum(item["flags"]),
		}

		if periodStr := pickString(item["uniperiod"]); isDigits(periodStr) {
			if p, err := strconv.Atoi(periodStr); err == nil {
				lesson.Period = &p
			}
		}

		if subjectID, ok := pickInt(item["subjectid"]); ok {
			if s, found := subjects[subjectID]; found {
				subject := s
				lesson.Subject = &subject
			} else {
				lesson.Subject = &Subject{SubjectID: subjectID}
			}
		}

		for _, raw := range pickSlice(item["classroomids"]) {
			id := strings.TrimSpace(pickString(raw))
			if id == "" {
				continue
			}
			if cr, found := classrooms[id]; found {
				lesson.Classrooms = append(lesson.Classrooms, cr)
			} else {
				lesson.Classrooms = append(lesson.Classrooms, Classroom{ClassroomID: id})
			}
		}

		for _, raw := range pickSlice(item["teacherids"]) {
			id, ok := pickInt(raw)
			if !ok {
				continue
			}
			if t, found := teachers[id]; found {
				lesson.Teachers = append(lesson.Teachers, t)
			} else {
				lesson.Teachers = append(lesson.Teachers, Teacher{PersonID: id})
			}
		}

		lessons = append(lessons, lesson)
	}

	return lessons
}

// combineDayTime combines day's date with an EduPage "HH:MM" time-of-day
// string into a full time.Time. EduPage represents midnight-at-end-of-day as
// "24:00", which is normalized to 23:59, as in the Python reference. It
// returns the zero time.Time when hhmm is empty or malformed.
func combineDayTime(day time.Time, hhmm string) time.Time {
	if hhmm == "" {
		return time.Time{}
	}
	hhmm = strings.ReplaceAll(hhmm, "24:00", "23:59")

	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return time.Time{}
	}

	h, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, errM := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errH != nil || errM != nil {
		return time.Time{}
	}

	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
}

// extractCurriculum ports the Python reference's curriculum lookup:
// flags.dp0.note_wd, falling back to flags.event.name.
func extractCurriculum(flagsRaw any) string {
	flags := pickMap(flagsRaw)
	if flags == nil {
		return ""
	}
	if dp0 := pickMap(flags["dp0"]); dp0 != nil {
		if note := pickString(dp0["note_wd"]); note != "" {
			return note
		}
	}
	if event := pickMap(flags["event"]); event != nil {
		if name := pickString(event["name"]); name != "" {
			return name
		}
	}
	return ""
}
