package edupage

import "testing"

func TestAsString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"already a string", "already a string"},
		{float64(42), "42"},
		{float64(3.5), "3.5"},
		{true, "1"},
		{false, "0"},
	}
	for _, tc := range cases {
		if got := asString(tc.in); got != tc.want {
			t.Errorf("asString(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAsInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{nil, 0},
		{float64(7), 7},
		{"7", 7},
		{"  7  ", 7},
		{"not a number", 0},
		{"", 0},
		{"3.0", 3},
	}
	for _, tc := range cases {
		if got := asInt(tc.in); got != tc.want {
			t.Errorf("asInt(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestAsMapRejectsNonObjects(t *testing.T) {
	if asMap(nil) != nil {
		t.Error("asMap(nil) should be nil")
	}
	if asMap([]any{}) != nil {
		t.Error("asMap of an array (EduPage's empty-object-as-array quirk) should be nil, not panic")
	}
	if asMap("a string") != nil {
		t.Error("asMap of a string should be nil")
	}
	m := map[string]any{"k": "v"}
	if got := asMap(m); got == nil || got["k"] != "v" {
		t.Error("asMap should pass through a real object")
	}
}

func TestAsBool(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{float64(1), true},
		{float64(0), false},
		{"1", true},
		{"0", false},
		{"true", true},
		{"garbage", false},
	}
	for _, tc := range cases {
		if got := asBool(tc.in); got != tc.want {
			t.Errorf("asBool(%#v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSessionIDNoSubdomainYet(t *testing.T) {
	c := New(0)
	if got := c.SessionID(); got != "" {
		t.Errorf("SessionID() before login = %q, want empty", got)
	}
}
