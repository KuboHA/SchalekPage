package edupage

import (
	"testing"
	"time"
)

func TestCombineDayTime(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		hhmm string
		want time.Time
	}{
		{"normal time", "07:55", time.Date(2024, 3, 15, 7, 55, 0, 0, time.UTC)},
		{"midnight sentinel 24:00 becomes 23:59", "24:00", time.Date(2024, 3, 15, 23, 59, 0, 0, time.UTC)},
		{"empty string", "", time.Time{}},
		{"malformed", "not-a-time", time.Time{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := combineDayTime(day, tc.hhmm)
			if !got.Equal(tc.want) {
				t.Errorf("combineDayTime(%q) = %v, want %v", tc.hhmm, got, tc.want)
			}
		})
	}
}

func TestExtractCurriculum(t *testing.T) {
	cases := []struct {
		name  string
		flags any
		want  string
	}{
		{
			name:  "dp0.note_wd wins",
			flags: map[string]any{"dp0": map[string]any{"note_wd": "Read chapter 4"}},
			want:  "Read chapter 4",
		},
		{
			name:  "falls back to event.name",
			flags: map[string]any{"event": map[string]any{"name": "School trip"}},
			want:  "School trip",
		},
		{
			name:  "no flags",
			flags: nil,
			want:  "",
		},
		{
			name:  "flags present but empty",
			flags: map[string]any{},
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractCurriculum(tc.flags); got != tc.want {
				t.Errorf("extractCurriculum() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTimetablePlan(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	plan := []any{
		// A real lesson.
		map[string]any{
			"uniperiod":    "3",
			"starttime":    "09:50",
			"endtime":      "10:35",
			"subjectid":    float64(7),
			"teacherids":   []any{float64(11), float64(12)},
			"classroomids": []any{"204"},
			"ol_url":       "https://meet.example/abc",
			"flags":        map[string]any{"dp0": map[string]any{"note_wd": "Ch. 5 exercises"}},
		},
		// A placeholder "add lesson" row that must be skipped.
		map[string]any{
			"header": []any{map[string]any{"cmd": "addlesson_t"}},
		},
		// A row with an empty header (falsy in Python) that must be skipped.
		map[string]any{
			"header": []any{},
		},
	}

	subjects := map[int]Subject{7: {SubjectID: 7, Name: "Mathematics", Short: "MAT"}}
	teachers := map[int]Teacher{
		11: {PersonID: 11, Name: "Jane Teacher"},
		// 12 intentionally absent from the table.
	}
	classrooms := map[string]Classroom{"204": {ClassroomID: "204", Name: "Room 204", Short: "204"}}

	lessons := parseTimetablePlan(plan, day, subjects, teachers, classrooms)
	if len(lessons) != 1 {
		t.Fatalf("got %d lessons, want 1: %#v", len(lessons), lessons)
	}

	l := lessons[0]
	if l.Period == nil || *l.Period != 3 {
		t.Errorf("Period = %v, want 3", l.Period)
	}
	if !l.StartTime.Equal(time.Date(2024, 3, 15, 9, 50, 0, 0, time.UTC)) {
		t.Errorf("StartTime = %v", l.StartTime)
	}
	if l.Subject == nil || l.Subject.SubjectID != 7 || l.Subject.Name != "Mathematics" || l.Subject.Short != "MAT" {
		t.Errorf("Subject = %#v, want resolved Mathematics/MAT", l.Subject)
	}
	if len(l.Teachers) != 2 || l.Teachers[0].PersonID != 11 || l.Teachers[0].Name != "Jane Teacher" {
		t.Errorf("Teachers[0] = %#v, want resolved PersonID 11 / Jane Teacher", l.Teachers)
	}
	if len(l.Teachers) != 2 || l.Teachers[1].PersonID != 12 || l.Teachers[1].Name != "" {
		// Teacher 12 has no entry in the lookup table: keep the id, leave the
		// name blank, and do not drop the lesson.
		t.Errorf("Teachers[1] = %#v, want unresolved PersonID 12 with empty Name", l.Teachers)
	}
	if len(l.Classrooms) != 1 || l.Classrooms[0].ClassroomID != "204" || l.Classrooms[0].Name != "Room 204" {
		t.Errorf("Classrooms = %#v, want resolved Room 204", l.Classrooms)
	}
	if l.OnlineLesson != "https://meet.example/abc" {
		t.Errorf("OnlineLesson = %q", l.OnlineLesson)
	}
	if l.Curriculum != "Ch. 5 exercises" {
		t.Errorf("Curriculum = %q", l.Curriculum)
	}
}

func TestParseTimetablePlan_UnresolvedIDsKeepStubs(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	plan := []any{
		map[string]any{
			"uniperiod":    "1",
			"starttime":    "08:00",
			"endtime":      "08:45",
			"subjectid":    float64(99),
			"teacherids":   []any{float64(88)},
			"classroomids": []any{"X1"},
		},
	}

	// Empty lookup tables: nothing resolves.
	lessons := parseTimetablePlan(plan, day, map[int]Subject{}, map[int]Teacher{}, map[string]Classroom{})
	if len(lessons) != 1 {
		t.Fatalf("got %d lessons, want 1", len(lessons))
	}

	l := lessons[0]
	if l.Subject == nil || l.Subject.SubjectID != 99 || l.Subject.Name != "" {
		t.Errorf("Subject = %#v, want unresolved SubjectID 99 with empty Name", l.Subject)
	}
	if len(l.Teachers) != 1 || l.Teachers[0].PersonID != 88 || l.Teachers[0].Name != "" {
		t.Errorf("Teachers = %#v, want unresolved PersonID 88 with empty Name", l.Teachers)
	}
	if len(l.Classrooms) != 1 || l.Classrooms[0].ClassroomID != "X1" || l.Classrooms[0].Name != "" {
		t.Errorf("Classrooms = %#v, want unresolved ClassroomID X1 with empty Name", l.Classrooms)
	}
}

func TestExtractDayPlanJSON(t *testing.T) {
	text := `some_prefix_junk("Student123",{"dates":{"2024-03-15":{"plan":[]}}},[1,2,3])`

	got, ok := extractDayPlanJSON(text, "Student123")
	if !ok {
		t.Fatal("expected ok=true")
	}

	want := `{"dates":{"2024-03-15":{"plan":[]}}}`
	if got != want {
		t.Errorf("extractDayPlanJSON() = %q, want %q", got, want)
	}
}

func TestExtractDayPlanJSON_NoMatch(t *testing.T) {
	if _, ok := extractDayPlanJSON("nothing useful here", "Student123"); ok {
		t.Error("expected ok=false when the marker is absent")
	}
}
