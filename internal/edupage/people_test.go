package edupage

import "testing"

func newTestClient(data map[string]any) *Client {
	return &Client{data: data}
}

func TestStudentsFaithfulMapping(t *testing.T) {
	c := newTestClient(map[string]any{
		"dbi": map[string]any{
			"students": map[string]any{
				"1234": map[string]any{
					"firstname":     "Jan",
					"lastname":      "Novak",
					"gender":        "M",
					"datefrom":      "2019-09-02",
					"classid":       "42",
					"numberinclass": "7",
				},
				// A student with the "triedaid" variant instead of "classid",
				// and numeric (not string) numberinclass, must still parse.
				"5678": map[string]any{
					"firstname":     "Eva",
					"lastname":      "Kovacova",
					"gender":        "F",
					"triedaid":      float64(43),
					"numberinclass": float64(3),
				},
				// Bogus / empty entries must be skipped, not panic.
				"": map[string]any{"firstname": "ghost"},
			},
		},
	})

	students, err := c.Students()
	if err != nil {
		t.Fatalf("Students(): %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("got %d students, want 2: %+v", len(students), students)
	}

	byID := map[int]Student{}
	for _, s := range students {
		byID[s.PersonID] = s
	}

	jan, ok := byID[1234]
	if !ok {
		t.Fatalf("student 1234 missing")
	}
	if jan.Name != "Jan Novak" {
		t.Errorf("Name = %q", jan.Name)
	}
	if jan.ClassID != 42 || jan.NumberInClass != 7 {
		t.Errorf("ClassID/NumberInClass = %d/%d", jan.ClassID, jan.NumberInClass)
	}
	if jan.InSchoolSince == nil || jan.InSchoolSince.Year() != 2019 {
		t.Errorf("InSchoolSince = %v", jan.InSchoolSince)
	}

	eva, ok := byID[5678]
	if !ok {
		t.Fatalf("student 5678 missing")
	}
	if eva.ClassID != 43 {
		t.Errorf("ClassID (via triedaid fallback) = %d, want 43", eva.ClassID)
	}
	if eva.NumberInClass != 3 {
		t.Errorf("NumberInClass = %d, want 3", eva.NumberInClass)
	}
	if eva.InSchoolSince != nil {
		t.Errorf("InSchoolSince should be nil when absent, got %v", eva.InSchoolSince)
	}
}

func TestStudentsNoDBI(t *testing.T) {
	c := newTestClient(map[string]any{})
	students, err := c.Students()
	if err != nil {
		t.Fatalf("Students(): %v", err)
	}
	if students != nil {
		t.Errorf("expected nil students when dbi is absent, got %+v", students)
	}
}

func TestStudentsNotLoggedIn(t *testing.T) {
	c := &Client{}
	_, err := c.Students()
	if err == nil {
		t.Fatal("expected ErrNotLoggedIn")
	}
}

func TestTeachersResolveClassroomShort(t *testing.T) {
	c := newTestClient(map[string]any{
		"dbi": map[string]any{
			"teachers": map[string]any{
				"9": map[string]any{
					"firstname":   "Maria",
					"lastname":    "Horvathova",
					"gender":      "F",
					"classroomid": "77",
				},
			},
			"classrooms": map[string]any{
				"77": map[string]any{"name": "Classroom 77", "short": "C77"},
			},
		},
	})

	teachers, err := c.Teachers()
	if err != nil {
		t.Fatalf("Teachers(): %v", err)
	}
	if len(teachers) != 1 {
		t.Fatalf("got %d teachers, want 1", len(teachers))
	}
	if teachers[0].ClassroomID != "C77" {
		t.Errorf("ClassroomID = %q, want %q", teachers[0].ClassroomID, "C77")
	}
}

func TestSchoolYear(t *testing.T) {
	c := newTestClient(map[string]any{
		"dp": map[string]any{"year": float64(2024)},
	})
	year, err := c.SchoolYear()
	if err != nil {
		t.Fatalf("SchoolYear(): %v", err)
	}
	if year != 2024 {
		t.Errorf("SchoolYear() = %d, want 2024", year)
	}
}

func TestSchoolYearMissingDP(t *testing.T) {
	c := newTestClient(map[string]any{})
	_, err := c.SchoolYear()
	if err == nil {
		t.Fatal("expected an error when dp is missing")
	}
}

func TestClassesFieldMapping(t *testing.T) {
	c := newTestClient(map[string]any{
		"dbi": map[string]any{
			"classes": map[string]any{
				"3": map[string]any{
					"name":        "3.A",
					"short":       "3A",
					"teacherid":   "10",
					"teacher2id":  "11",
					"grade":       "3",
					"classroomid": "5",
				},
			},
		},
	})

	classes, err := c.Classes()
	if err != nil {
		t.Fatalf("Classes(): %v", err)
	}
	if len(classes) != 1 {
		t.Fatalf("got %d classes, want 1", len(classes))
	}
	cl := classes[0]
	if cl.ClassID != 3 || cl.Name != "3.A" || cl.Short != "3A" || cl.TeacherID != 10 ||
		cl.Teacher2ID != 11 || cl.Grade != 3 || cl.ClassroomID != "5" {
		t.Errorf("unexpected class mapping: %+v", cl)
	}
}
