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

// TestMissingTeachersFromHTML covers the pure half of SubstitutionDay: it
// resolves names embedded in an already-fetched viewer fragment against the
// login payload's teacher table, so one fetch can feed both views.
func TestMissingTeachersFromHTML(t *testing.T) {
	c := newTestClient(map[string]any{
		"dbi": map[string]any{
			"teachers": map[string]any{
				"1": map[string]any{"firstname": "Jane", "lastname": "Doe"},
				"2": map[string]any{"firstname": "John", "lastname": "Smith"},
				"3": map[string]any{"firstname": "Ann", "lastname": "Lee"},
			},
		},
	})

	html := `<div class="header">` + missingTeachersMarker +
		`Missing teachers: Jane Doe, John Smith + Ann Lee  (illness)</span></div>`

	got, err := c.missingTeachersFromHTML(html)
	if err != nil {
		t.Fatalf("missingTeachersFromHTML: %v", err)
	}
	want := []string{"Jane Doe", "John Smith", "Ann Lee"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %v", got, want)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}

	t.Run("empty fragment", func(t *testing.T) {
		got, err := c.missingTeachersFromHTML("   ")
		if got != nil || err != nil {
			t.Errorf("got (%#v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("unknown teacher", func(t *testing.T) {
		html := `<div class="header">` + missingTeachersMarker + `Missing teachers: Nobody Here</span></div>`
		if _, err := c.missingTeachersFromHTML(html); err == nil {
			t.Error("expected an error for an unknown teacher")
		}
	})
}

func TestParseMissingTeacherNames(t *testing.T) {
	t.Run("single and co-taught names, with annotation dropped", func(t *testing.T) {
		html := `<div class="header">` + missingTeachersMarker +
			`Missing teachers: Jane Doe, John Smith + Ann Lee  (illness)</span></div>`

		names, ok := parseMissingTeacherNames(html)
		if !ok {
			t.Fatal("expected ok=true")
		}
		want := []string{"Jane Doe", "John Smith", "Ann Lee"}
		if len(names) != len(want) {
			t.Fatalf("got %#v, want %#v", names, want)
		}
		for i, n := range want {
			if names[i] != n {
				t.Errorf("names[%d] = %q, want %q", i, names[i], n)
			}
		}
	})

	t.Run("empty header line", func(t *testing.T) {
		html := `<div class="header">` + missingTeachersMarker + `</span></div>`
		if _, ok := parseMissingTeacherNames(html); ok {
			t.Error("expected ok=false")
		}
	})

	t.Run("no marker at all", func(t *testing.T) {
		if _, ok := parseMissingTeacherNames("<html>nothing here</html>"); ok {
			t.Error("expected ok=false")
		}
	})
}

// TestSubstitutionHTML_ReloadDetection exercises the same reload-detection
// helper substitutionHTML relies on (via Client.withSessionRecovery), since
// the endpoint itself can't be hit without a live EduPage session. See
// TestRetryOnReload_* in client_test.go for the retry-count/give-up
// behavior this builds on.
func TestSubstitutionHTML_ReloadDetection(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"normal substitution payload", `{"r":"<div>...</div>"}`, false},
		{"stale session", `{"reload":true}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasReloadKey([]byte(tc.body)); got != tc.want {
				t.Errorf("hasReloadKey(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
