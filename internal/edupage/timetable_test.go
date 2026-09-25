package edupage

import (
	"errors"
	"testing"
	"time"
)

func TestIsOnlineLesson(t *testing.T) {
	if (Lesson{}).IsOnlineLesson() {
		t.Error("empty lesson should not be an online lesson")
	}
	if !(Lesson{OnlineLesson: "https://example.com/join"}).IsOnlineLesson() {
		t.Error("lesson with an online lesson link should be an online lesson")
	}
}

func TestSignIntoLesson_NotOnline(t *testing.T) {
	c := New(0)
	if _, err := c.SignIntoLesson(Lesson{}); err == nil {
		t.Error("expected an error for a non-online lesson")
	}
}

func TestSignIntoLesson_NoSubject(t *testing.T) {
	c := New(0)
	l := Lesson{OnlineLesson: "https://example.com/join"}
	if _, err := c.SignIntoLesson(l); !errors.Is(err, ErrMissingData) {
		t.Errorf("expected ErrMissingData, got %v", err)
	}
}

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

func TestParseTimetablePlan_CancelledEventGroupFlags(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	plan := []any{
		// Ordinary lesson: no "type"/"removed"/"main" keys at all. Must NOT
		// be flagged cancelled or event, even though a naive "type == \"\""
		// check on a missing key would wrongly mark it cancelled.
		map[string]any{
			"uniperiod":  "1",
			"starttime":  "08:00",
			"endtime":    "08:45",
			"groupnames": []any{"1.A/sk1", "", "1.A/sk2"},
		},
		// Explicitly removed.
		map[string]any{
			"uniperiod": "2",
			"starttime": "08:50",
			"endtime":   "09:35",
			"removed":   true,
		},
		// type == "absent".
		map[string]any{
			"uniperiod": "3",
			"starttime": "09:50",
			"endtime":   "10:35",
			"type":      "absent",
		},
		// type == "" (present but empty, as opposed to missing).
		map[string]any{
			"uniperiod": "4",
			"starttime": "10:45",
			"endtime":   "11:30",
			"type":      "",
		},
		// type == "event".
		map[string]any{
			"uniperiod": "5",
			"starttime": "11:40",
			"endtime":   "12:25",
			"type":      "event",
		},
		// type == "out".
		map[string]any{
			"uniperiod": "6",
			"starttime": "12:35",
			"endtime":   "13:20",
			"type":      "out",
		},
		// main is truthy.
		map[string]any{
			"uniperiod": "7",
			"starttime": "13:30",
			"endtime":   "14:15",
			"main":      true,
		},
	}

	lessons := parseTimetablePlan(plan, day, nil, nil, nil)
	if len(lessons) != 7 {
		t.Fatalf("got %d lessons, want 7: %#v", len(lessons), lessons)
	}

	ordinary := lessons[0]
	if ordinary.IsCancelled || ordinary.IsEvent {
		t.Errorf("ordinary lesson: IsCancelled=%v IsEvent=%v, want both false", ordinary.IsCancelled, ordinary.IsEvent)
	}
	if want := []string{"1.A/sk1", "1.A/sk2"}; len(ordinary.Groups) != len(want) || ordinary.Groups[0] != want[0] || ordinary.Groups[1] != want[1] {
		t.Errorf("Groups = %#v, want %#v (empty entries filtered)", ordinary.Groups, want)
	}

	for i, name := range []string{"removed=true", `type="absent"`, `type=""`} {
		l := lessons[i+1]
		if !l.IsCancelled {
			t.Errorf("lesson %d (%s): IsCancelled = false, want true", i+1, name)
		}
		if l.IsEvent {
			t.Errorf("lesson %d (%s): IsEvent = true, want false", i+1, name)
		}
	}

	for i, name := range []string{`type="event"`, `type="out"`, "main=true"} {
		l := lessons[i+4]
		if !l.IsEvent {
			t.Errorf("lesson %d (%s): IsEvent = false, want true", i+4, name)
		}
		if l.IsCancelled {
			t.Errorf("lesson %d (%s): IsCancelled = true, want false", i+4, name)
		}
	}
}

func TestIsLessonType(t *testing.T) {
	cases := []struct {
		name string
		item map[string]any
		want string
		ok   bool
	}{
		{"missing key never matches, even want=\"\"", map[string]any{}, "", false},
		{"missing key never matches a non-empty want", map[string]any{}, "absent", false},
		{"exact match", map[string]any{"type": "absent"}, "absent", true},
		{"present empty string matches want=\"\"", map[string]any{"type": ""}, "", true},
		{"non-string type never matches", map[string]any{"type": float64(1)}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLessonType(tc.item, tc.want); got != tc.ok {
				t.Errorf("isLessonType(%#v, %q) = %v, want %v", tc.item, tc.want, got, tc.ok)
			}
		})
	}
}

func TestTruthy(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{nil, false},
		{false, false},
		{true, true},
		{float64(0), false},
		{float64(1), true},
		{"", false},
		{"0", false},
		{"anything else", true},
		{map[string]any{}, false},
		{map[string]any{"k": "v"}, true},
		{[]any{}, false},
		{[]any{1}, true},
	}
	for _, tc := range cases {
		if got := truthy(tc.in); got != tc.want {
			t.Errorf("truthy(%#v) = %v, want %v", tc.in, got, tc.want)
		}
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
