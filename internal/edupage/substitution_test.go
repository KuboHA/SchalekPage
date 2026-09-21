package edupage

import "testing"

func TestFormatLessonN(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"3", "3"},
		{"3 - 4", "3-4"},
		{"3.", "3"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := formatLessonN(tc.raw); got != tc.want {
			t.Errorf("formatLessonN(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestParseChangeAction(t *testing.T) {
	cases := []struct {
		raw  string
		want ChangeAction
	}{
		{"change", ActionChange},
		{"add", ActionAddition},
		{"remove", ActionDeletion},
		{" Change ", ActionChange},
		{"unknownaction", ChangeAction("unknownaction")},
	}

	for _, tc := range cases {
		if got := parseChangeAction(tc.raw); got != tc.want {
			t.Errorf("parseChangeAction(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestParseSubstitutionHTML(t *testing.T) {
	// This mirrors the shape the Python reference's string-splitting expects
	// *after* its fixed tag replacements would have run: each row is
	// `{action}">{lessonN}</span>{title}</span>`, where `">`  is turned into
	// an extra "</span>" delimiter by parseSubstitutionHTML itself.
	html := `<div class="header">missing teachers etc.</div>` +
		substClassDelim + `1.A` + substRowsMarker +
		substRowDelim + `change">3</span>Math substituted teacher</span>` +
		substRowDelim + `add">4 - 5</span>New lesson added</span>` +
		substClassDelim + `2.B` + substRowsMarker +
		substRowDelim + `remove">1</span>Lesson cancelled</span>` +
		substFooterDelim + `footer junk`

	changes := parseSubstitutionHTML(html)
	if len(changes) != 3 {
		t.Fatalf("got %d changes, want 3: %#v", len(changes), changes)
	}

	if changes[0].ChangeClass != "1.A" || changes[0].LessonN != "3" || changes[0].Action != ActionChange {
		t.Errorf("changes[0] = %#v", changes[0])
	}
	if changes[1].LessonN != "4-5" || changes[1].Action != ActionAddition {
		t.Errorf("changes[1] = %#v", changes[1])
	}
	if changes[2].ChangeClass != "2.B" || changes[2].Action != ActionDeletion {
		t.Errorf("changes[2] = %#v", changes[2])
	}
}

func TestParseSubstitutionHTML_NoSections(t *testing.T) {
	if got := parseSubstitutionHTML("<html>nothing here</html>"); got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
}
