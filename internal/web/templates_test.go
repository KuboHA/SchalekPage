package web

import "testing"

// TestTemplatesParse ensures every embedded template compiles under
// html/template with the registered FuncMap. It depends on
// internal/edupage providing EventTypeIcon/EventTypeName (agent B).
func TestTemplatesParse(t *testing.T) {
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("ParseTemplates: %v", err)
	}

	want := []string{
		"login.html",
		"two_factor.html",
		"dashboard.html",
		"timetable.html",
		"lunches.html",
		"grades.html",
		"substitutions.html",
	}
	for _, name := range want {
		if tmpl.Lookup(name) == nil {
			t.Errorf("template %q was not registered", name)
		}
	}
}
