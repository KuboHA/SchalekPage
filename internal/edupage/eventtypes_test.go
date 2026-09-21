package edupage

import "testing"

func TestEventTypeName(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		want      string
	}{
		{"known raw value", "znamka", "New Grade"},
		{"known snake_case value", "big_exam", "Big Exam"},
		{"case-insensitive match", "ZNAMKA", "New Grade"},
		{"unknown falls back to raw key", "some_unknown_type", "some_unknown_type"},
		{"empty string falls back to itself", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EventTypeName(tc.eventType); got != tc.want {
				t.Errorf("EventTypeName(%q) = %q, want %q", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestEventTypeIcon(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		want      string
	}{
		{"known raw value", "znamka", "fa-graduation-cap"},
		{"known snake_case value", "big_exam", "fa-file-alt"},
		{"case-insensitive match", "SPRAVA", "fa-envelope"},
		{"unknown falls back to fa-bell", "some_unknown_type", "fa-bell"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EventTypeIcon(tc.eventType); got != tc.want {
				t.Errorf("EventTypeIcon(%q) = %q, want %q", tc.eventType, got, tc.want)
			}
		})
	}
}
