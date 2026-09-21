package edupage

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GradesForTerm returns the logged-in student's grades for the given school
// year and term. It returns an empty slice and a nil error when EduPage has
// no grades recorded yet for that term.
//
// Grade.Subject and Grade.Teacher are resolved from the school's DBI tables
// (via subjectsByID/teachersByID); an id with no match in those tables keeps
// the id but leaves the name empty rather than being dropped or erroring.
func (c *Client) GradesForTerm(year int, term Term) ([]Grade, error) {
	path := fmt.Sprintf(
		"/znamky/?what=studentviewer&znamky_yearid=%d&nadobdobie=%s",
		year, url.QueryEscape(string(term)),
	)

	body, err := c.PostForm(path, url.Values{})
	if err != nil {
		return nil, fmt.Errorf("edupage: fetch grades: %w", err)
	}

	jsonPart, ok := extractStudentViewerJSON(string(body))
	if !ok {
		return nil, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return nil, fmt.Errorf("edupage: decode grades payload: %w", err)
	}

	rawGrades := pickSlice(payload["vsetkyZnamky"])
	if len(rawGrades) == 0 {
		return nil, nil
	}

	eventsRoot := pickMap(payload["vsetkyUdalosti"])
	details := pickMap(eventsRoot["edupage"])

	subjects, err := subjectsByID(c)
	if err != nil {
		return nil, err
	}
	teachers, err := teachersByID(c)
	if err != nil {
		return nil, err
	}

	grades := make([]Grade, 0, len(rawGrades))
	for _, raw := range rawGrades {
		g := pickMap(raw)
		if g == nil {
			continue
		}

		grade, ok := parseGrade(g, details, subjects, teachers)
		if ok {
			grades = append(grades, grade)
		}
	}

	return grades, nil
}

// extractStudentViewerJSON pulls the JSON object out of the /znamky/ page's
// `...znamkyStudentViewer(<json>);...` inline script, mirroring
// Grades.__parse_grade_data from the Python reference.
func extractStudentViewerJSON(page string) (string, bool) {
	const marker = ".znamkyStudentViewer("
	idx := strings.Index(page, marker)
	if idx < 0 {
		return "", false
	}
	rest := page[idx+len(marker):]

	end := strings.Index(rest, ");")
	if end < 0 {
		return "", false
	}

	return rest[:end], true
}

// parseGrade converts one raw entry from "vsetkyZnamky" (plus its associated
// details from "vsetkyUdalosti.edupage") into a Grade. ok is false when the
// entry is missing required identifying data and should be skipped, as in
// the Python reference. subjects and teachers are ID-keyed lookup tables
// (see subjectsByID/teachersByID) used to resolve Subject/Teacher names.
func parseGrade(g, details map[string]any, subjects map[int]Subject, teachers map[int]Teacher) (Grade, bool) {
	eventIDStr := strings.TrimSpace(pickString(g["udalostid"]))
	if eventIDStr == "" {
		return Grade{}, false
	}
	eventID, err := strconv.Atoi(eventIDStr)
	if err != nil {
		return Grade{}, false
	}

	detail := pickMap(details[eventIDStr])

	subjectIDRaw, has := detail["PredmetID"]
	if !has {
		return Grade{}, false
	}
	if s, ok := subjectIDRaw.(string); ok && s == "vsetky" {
		return Grade{}, false
	}
	subjectID, ok := pickInt(subjectIDRaw)
	if !ok {
		return Grade{}, false
	}

	var gradeDate time.Time
	if ds := pickString(g["datum"]); ds != "" {
		if t, err := time.Parse(eduTimeLayout, ds); err == nil {
			gradeDate = t
		}
	}

	subjectName := subjects[subjectID].Name

	teacherID, hasTeacherID := pickInt(detail["UcitelID"])
	var teacherName string
	if hasTeacherID {
		teacherName = teachers[teacherID].Name
	}

	gradeType := pickString(detail["p_typ_udalosti"])

	var maxPoints, weight float64
	var hasMaxPoints bool
	switch gradeType {
	case "1":
		// Normal 1-5 grade: only an importance weight, no max points.
		if w, ok := pickFloat(detail["p_vaha"]); ok {
			weight = w / 20
		}
	case "2":
		// Points grade.
		if mp, ok := pickFloat(detail["p_vaha"]); ok {
			maxPoints = mp
			hasMaxPoints = true
		}
	case "3":
		// Percentage grade.
		if mp, ok := pickFloat(detail["p_vaha_body"]); ok {
			maxPoints = mp
			hasMaxPoints = true
		}
		if w, ok := pickFloat(detail["p_vaha"]); ok {
			weight = w / 20
		}
	}

	gradeN, comment := splitGradeData(pickString(g["data"]))

	verbal := true
	var percent float64
	if numeric, err := strconv.ParseFloat(gradeN, 64); err == nil {
		verbal = false
		switch {
		case maxPoints > 0:
			percent = math.Round(numeric/maxPoints*100*100) / 100
		case hasMaxPoints && maxPoints == 0:
			percent = math.Inf(1)
		}
	}

	return Grade{
		GradeN:    gradeN,
		Comment:   comment,
		Date:      gradeDate,
		Subject:   subjectName,
		SubjectID: subjectID,
		Teacher:   teacherName,
		TeacherID: teacherID,
		Title:     pickString(detail["p_meno"]),
		MaxPoints: maxPoints,
		Percent:   percent,
		Verbal:    verbal,
		Weight:    weight,
		EventID:   eventID,
	}, true
}

// splitGradeData splits a raw grade "data" value such as "18 (great job)"
// into its grade value and comment, mirroring the Python reference's
// `grade.get("data").split(" (", 1)` handling.
func splitGradeData(raw string) (gradeN, comment string) {
	idx := strings.Index(raw, " (")
	if idx < 0 {
		return raw, ""
	}

	gradeN = raw[:idx]
	rest := raw[idx+2:]
	if end := strings.LastIndex(rest, ")"); end >= 0 {
		comment = rest[:end]
	}

	return gradeN, comment
}
