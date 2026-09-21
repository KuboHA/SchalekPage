package edupage

import (
	"math"
	"testing"
)

func TestSplitGradeData(t *testing.T) {
	cases := []struct {
		raw         string
		wantGradeN  string
		wantComment string
	}{
		{"1 (Great work)", "1", "Great work"},
		{"18", "18", ""},
		{"N (absent, excused)", "N", "absent, excused"},
	}

	for _, tc := range cases {
		gradeN, comment := splitGradeData(tc.raw)
		if gradeN != tc.wantGradeN || comment != tc.wantComment {
			t.Errorf("splitGradeData(%q) = (%q, %q), want (%q, %q)",
				tc.raw, gradeN, comment, tc.wantGradeN, tc.wantComment)
		}
	}
}

func TestExtractStudentViewerJSON(t *testing.T) {
	page := "<script>\n\t\tznamky.znamkyStudentViewer({\"vsetkyZnamky\":[]});\r\n\t\t});\r\n\t\t</script>"

	got, ok := extractStudentViewerJSON(page)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != `{"vsetkyZnamky":[]}` {
		t.Errorf("extractStudentViewerJSON() = %q", got)
	}
}

func TestExtractStudentViewerJSON_NoMatch(t *testing.T) {
	if _, ok := extractStudentViewerJSON("no marker here"); ok {
		t.Error("expected ok=false")
	}
}

func TestParseGrade(t *testing.T) {
	t.Run("percentage grade", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "555",
			"datum":     "2024-02-10 00:00:00",
			"data":      "18 (nice)",
		}
		details := map[string]any{
			"555": map[string]any{
				"p_meno":         "Chapter 3 test",
				"PredmetID":      "42",
				"UcitelID":       "9",
				"p_typ_udalosti": "3",
				"p_vaha_body":    "20",
				"p_vaha":         "40",
			},
		}

		grade, ok := parseGrade(g, details, nil, nil)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if grade.EventID != 555 {
			t.Errorf("EventID = %d, want 555", grade.EventID)
		}
		if grade.SubjectID != 42 || grade.TeacherID != 9 {
			t.Errorf("SubjectID/TeacherID = %d/%d, want 42/9", grade.SubjectID, grade.TeacherID)
		}
		if grade.GradeN != "18" || grade.Comment != "nice" {
			t.Errorf("GradeN/Comment = %q/%q, want 18/nice", grade.GradeN, grade.Comment)
		}
		if grade.Verbal {
			t.Error("Verbal = true, want false for a numeric grade")
		}
		if grade.MaxPoints != 20 {
			t.Errorf("MaxPoints = %v, want 20", grade.MaxPoints)
		}
		wantPercent := 90.0
		if grade.Percent != wantPercent {
			t.Errorf("Percent = %v, want %v", grade.Percent, wantPercent)
		}
		if grade.Weight != 2 {
			t.Errorf("Weight = %v, want 2", grade.Weight)
		}
	})

	t.Run("classic 1-5 grade has no max points", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "1",
			"datum":     "2024-02-10 00:00:00",
			"data":      "1",
		}
		details := map[string]any{
			"1": map[string]any{
				"p_meno":         "Oral exam",
				"PredmetID":      "3",
				"p_typ_udalosti": "1",
				"p_vaha":         "40",
			},
		}

		grade, ok := parseGrade(g, details, nil, nil)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if grade.MaxPoints != 0 {
			t.Errorf("MaxPoints = %v, want 0", grade.MaxPoints)
		}
		if grade.Weight != 2 {
			t.Errorf("Weight = %v, want 2", grade.Weight)
		}
		if grade.Verbal {
			t.Error("Verbal = true, want false for grade \"1\"")
		}
	})

	t.Run("verbal grade", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "2",
			"datum":     "2024-02-10 00:00:00",
			"data":      "absent (sick)",
		}
		details := map[string]any{
			"2": map[string]any{
				"p_meno":         "Quiz",
				"PredmetID":      "3",
				"p_typ_udalosti": "1",
			},
		}

		grade, ok := parseGrade(g, details, nil, nil)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !grade.Verbal {
			t.Error("Verbal = false, want true for a non-numeric grade")
		}
	})

	t.Run("points grade with zero max points is infinite percent", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "3",
			"datum":     "2024-02-10 00:00:00",
			"data":      "0",
		}
		details := map[string]any{
			"3": map[string]any{
				"p_meno":         "Bonus",
				"PredmetID":      "3",
				"p_typ_udalosti": "2",
				"p_vaha":         "0",
			},
		}

		grade, ok := parseGrade(g, details, nil, nil)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !math.IsInf(grade.Percent, 1) {
			t.Errorf("Percent = %v, want +Inf", grade.Percent)
		}
	})

	t.Run("subject id vsetky is skipped", func(t *testing.T) {
		g := map[string]any{"udalostid": "4", "data": "1"}
		details := map[string]any{
			"4": map[string]any{"PredmetID": "vsetky"},
		}
		if _, ok := parseGrade(g, details, nil, nil); ok {
			t.Error("expected ok=false for PredmetID == \"vsetky\"")
		}
	})

	t.Run("missing udalostid is skipped", func(t *testing.T) {
		if _, ok := parseGrade(map[string]any{}, map[string]any{}, nil, nil); ok {
			t.Error("expected ok=false for missing udalostid")
		}
	})

	t.Run("resolves subject and teacher names", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "10",
			"datum":     "2024-02-10 00:00:00",
			"data":      "1",
		}
		details := map[string]any{
			"10": map[string]any{
				"p_meno":         "Oral exam",
				"PredmetID":      "3",
				"UcitelID":       "9",
				"p_typ_udalosti": "1",
			},
		}
		subjects := map[int]Subject{3: {SubjectID: 3, Name: "Slovak", Short: "SVK"}}
		teachers := map[int]Teacher{9: {PersonID: 9, Name: "Mrs. Novak"}}

		grade, ok := parseGrade(g, details, subjects, teachers)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if grade.Subject != "Slovak" {
			t.Errorf("Subject = %q, want %q", grade.Subject, "Slovak")
		}
		if grade.Teacher != "Mrs. Novak" {
			t.Errorf("Teacher = %q, want %q", grade.Teacher, "Mrs. Novak")
		}
	})

	t.Run("unresolved subject/teacher ids keep the id with an empty name", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "11",
			"datum":     "2024-02-10 00:00:00",
			"data":      "1",
		}
		details := map[string]any{
			"11": map[string]any{
				"p_meno":         "Oral exam",
				"PredmetID":      "77",
				"UcitelID":       "88",
				"p_typ_udalosti": "1",
			},
		}

		// Empty lookup tables: nothing resolves.
		grade, ok := parseGrade(g, details, map[int]Subject{}, map[int]Teacher{})
		if !ok {
			t.Fatal("expected ok=true")
		}
		if grade.SubjectID != 77 || grade.Subject != "" {
			t.Errorf("SubjectID/Subject = %d/%q, want 77/\"\"", grade.SubjectID, grade.Subject)
		}
		if grade.TeacherID != 88 || grade.Teacher != "" {
			t.Errorf("TeacherID/Teacher = %d/%q, want 88/\"\"", grade.TeacherID, grade.Teacher)
		}
	})

	t.Run("missing teacher id leaves TeacherID and Teacher zero", func(t *testing.T) {
		g := map[string]any{
			"udalostid": "12",
			"datum":     "2024-02-10 00:00:00",
			"data":      "1",
		}
		details := map[string]any{
			"12": map[string]any{
				"p_meno":         "Oral exam",
				"PredmetID":      "3",
				"p_typ_udalosti": "1",
			},
		}
		teachers := map[int]Teacher{0: {PersonID: 0, Name: "Should not be used"}}

		grade, ok := parseGrade(g, details, map[int]Subject{}, teachers)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if grade.TeacherID != 0 || grade.Teacher != "" {
			t.Errorf("TeacherID/Teacher = %d/%q, want 0/\"\" when UcitelID is absent", grade.TeacherID, grade.Teacher)
		}
	})
}
