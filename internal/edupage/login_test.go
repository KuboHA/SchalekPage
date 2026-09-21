package edupage

import "testing"

func TestParseLoginData(t *testing.T) {
	raw := "some junk before" +
		`userhome({"userid":"Student1234","dp":{"year":2023}});` +
		`ASC.gsechash="abc123";` +
		"trailing junk"

	c := &Client{}
	if err := c.parseLoginData(raw); err != nil {
		t.Fatalf("parseLoginData: %v", err)
	}

	if got := c.UserID(); got != "Student1234" {
		t.Errorf("UserID() = %q, want %q", got, "Student1234")
	}
	if got := c.GsecHash(); got != "abc123" {
		t.Errorf("GsecHash() = %q, want %q", got, "abc123")
	}
	dp := asMap(c.Data()["dp"])
	if dp == nil || asInt(dp["year"]) != 2023 {
		t.Errorf("dp.year not parsed correctly: %v", dp)
	}
}

func TestParseLoginDataWithTrailingScriptTagOnly(t *testing.T) {
	// Real EduPage pages typically have a single userhome(...) call, possibly
	// followed by unrelated markup with no further ");" occurrences.
	raw := "userhome({\"userid\":\"Teacher1\"});\n</script>"

	c := &Client{}
	if err := c.parseLoginData(raw); err != nil {
		t.Fatalf("parseLoginData: %v", err)
	}
	if got := c.UserID(); got != "Teacher1" {
		t.Errorf("UserID() = %q, want %q", got, "Teacher1")
	}
}

func TestParseLoginDataMissingMarker(t *testing.T) {
	c := &Client{}
	err := c.parseLoginData("no userhome call here")
	if err == nil {
		t.Fatal("expected an error when userhome( marker is missing")
	}
}

func TestParseLoginDataNoGsecHash(t *testing.T) {
	c := &Client{}
	if err := c.parseLoginData(`userhome({"userid":"Student1"});`); err != nil {
		t.Fatalf("parseLoginData: %v", err)
	}
	if got := c.GsecHash(); got != "" {
		t.Errorf("GsecHash() = %q, want empty", got)
	}
}

func TestBetween(t *testing.T) {
	s := `<input name="csrfauth" value="tok123">`
	got, ok := between(s, `csrfauth" value="`, `"`)
	if !ok || got != "tok123" {
		t.Errorf("between() = %q, %v; want %q, true", got, ok, "tok123")
	}

	_, ok = between(s, "notfound", `"`)
	if ok {
		t.Error("expected ok=false when start marker is absent")
	}
}

func TestRsplitFirst(t *testing.T) {
	cases := []struct {
		s, sep string
		n      int
		want   string
	}{
		{"a);b);c);d", ");", 2, "a);b"},
		{"a);b", ");", 5, "a"},
		{"noseparator", ");", 2, "noseparator"},
		{"a);b);c", ");", 0, "a);b);c"},
	}
	for _, tc := range cases {
		if got := rsplitFirst(tc.s, tc.sep, tc.n); got != tc.want {
			t.Errorf("rsplitFirst(%q, %q, %d) = %q, want %q", tc.s, tc.sep, tc.n, got, tc.want)
		}
	}
}
