package web

import (
	"testing"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func TestStudentRecipientID(t *testing.T) {
	cases := []struct {
		id   int
		want string
	}{
		{123, "Student123"},
		{0, "Student0"},
		{9999, "Student9999"},
	}
	for _, tc := range cases {
		if got := studentRecipientID(tc.id); got != tc.want {
			t.Errorf("studentRecipientID(%d) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestTeacherRecipientID(t *testing.T) {
	cases := []struct {
		id   int
		want string
	}{
		{45, "Teacher45"},
		{0, "Teacher0"},
	}
	for _, tc := range cases {
		if got := teacherRecipientID(tc.id); got != tc.want {
			t.Errorf("teacherRecipientID(%d) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestBuildRecipientOptions(t *testing.T) {
	students := []edupage.Student{
		{PersonID: 2, Name: "Beta Student"},
		{PersonID: 1, Name: "Alpha Student"},
	}
	teachers := []edupage.Teacher{
		{PersonID: 20, Name: "Zed Teacher"},
		{PersonID: 10, Name: "Anna Teacher"},
	}

	studentOpts, teacherOpts := buildRecipientOptions(students, teachers)

	if len(studentOpts) != 2 || studentOpts[0].ID != "Student1" || studentOpts[0].Name != "Alpha Student" {
		t.Errorf("studentOpts[0] = %+v, want Student1/Alpha Student first (sorted by name)", studentOpts[0])
	}
	if studentOpts[1].ID != "Student2" {
		t.Errorf("studentOpts[1].ID = %q, want Student2", studentOpts[1].ID)
	}

	if len(teacherOpts) != 2 || teacherOpts[0].ID != "Teacher10" || teacherOpts[0].Name != "Anna Teacher" {
		t.Errorf("teacherOpts[0] = %+v, want Teacher10/Anna Teacher first (sorted by name)", teacherOpts[0])
	}
	if teacherOpts[1].ID != "Teacher20" {
		t.Errorf("teacherOpts[1].ID = %q, want Teacher20", teacherOpts[1].ID)
	}
}

func TestBuildRecipientOptions_Empty(t *testing.T) {
	studentOpts, teacherOpts := buildRecipientOptions(nil, nil)
	if len(studentOpts) != 0 || len(teacherOpts) != 0 {
		t.Errorf("expected empty slices, got %d students, %d teachers", len(studentOpts), len(teacherOpts))
	}
}

func TestValidateMessageRecipients(t *testing.T) {
	cases := []struct {
		name    string
		raw     []string
		wantErr bool
		wantIDs []string
	}{
		{"nil input rejected", nil, true, nil},
		{"empty slice rejected", []string{}, true, nil},
		{"only whitespace rejected", []string{"  ", ""}, true, nil},
		{"whole school rejected even alone", []string{"*"}, true, nil},
		{"whole school rejected even mixed with real recipients", []string{"Student1", "*"}, true, nil},
		{"single valid recipient", []string{"Student123"}, false, []string{"Student123"}},
		{"trims whitespace", []string{"  Student123  "}, false, []string{"Student123"}},
		{"de-duplicates", []string{"Student1", "Student1", "Teacher2"}, false, []string{"Student1", "Teacher2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateMessageRecipients(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %v", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %v, want %v", got, tc.wantIDs)
			}
			for i := range got {
				if got[i] != tc.wantIDs[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tc.wantIDs[i])
				}
			}
		})
	}
}

func TestValidateMessageBody(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"empty rejected", "", true},
		{"whitespace only rejected", "   \n\t  ", true},
		{"non-empty accepted", "Hello!", false},
		{"leading/trailing whitespace around real text accepted", "  Hello!  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMessageBody(tc.body)
			if tc.wantErr && err == nil {
				t.Errorf("expected an error for %q", tc.body)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.body, err)
			}
		})
	}
}

func TestValidateUploadSize(t *testing.T) {
	cases := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"zero bytes ok", 0, false},
		{"small file ok", 1024, false},
		{"exactly at the cap ok", maxMessageUploadBytes, false},
		{"one byte over the cap rejected", maxMessageUploadBytes + 1, true},
		{"far over the cap rejected", maxMessageUploadBytes * 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUploadSize(tc.size)
			if tc.wantErr && err == nil {
				t.Errorf("size %d: expected an error", tc.size)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("size %d: unexpected error: %v", tc.size, err)
			}
		})
	}
}

func TestParseAttachmentsJSON(t *testing.T) {
	t.Run("empty string decodes to no attachments", func(t *testing.T) {
		atts, err := parseAttachmentsJSON("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if atts != nil {
			t.Errorf("got %v, want nil", atts)
		}
	})

	t.Run("whitespace-only decodes to no attachments", func(t *testing.T) {
		atts, err := parseAttachmentsJSON("   ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if atts != nil {
			t.Errorf("got %v, want nil", atts)
		}
	})

	t.Run("invalid JSON is rejected", func(t *testing.T) {
		if _, err := parseAttachmentsJSON("not json"); err == nil {
			t.Error("expected an error for invalid JSON")
		}
	})

	t.Run("decodes a well-formed attachment list", func(t *testing.T) {
		raw := `[{"cloudId":"abc","file":"cloud/abc.pdf","name":"homework.pdf","extension":"pdf","fileType":"document","url":"https://school.edupage.org/cloud/abc.pdf"}]`
		atts, err := parseAttachmentsJSON(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(atts) != 1 {
			t.Fatalf("got %d attachments, want 1", len(atts))
		}
		want := messageAttachment{
			CloudID:   "abc",
			File:      "cloud/abc.pdf",
			Name:      "homework.pdf",
			Extension: "pdf",
			FileType:  "document",
			URL:       "https://school.edupage.org/cloud/abc.pdf",
		}
		if atts[0] != want {
			t.Errorf("got %+v, want %+v", atts[0], want)
		}
	})
}

func TestEncodeAttachmentsJSON_RoundTrips(t *testing.T) {
	atts := []messageAttachment{
		{CloudID: "1", File: "cloud/1.png", Name: "photo.png", Extension: "png", FileType: "image", URL: "https://x/cloud/1.png"},
		{CloudID: "2", File: "cloud/2.pdf", Name: "notes.pdf", Extension: "pdf", FileType: "document", URL: "https://x/cloud/2.pdf"},
	}
	encoded := encodeAttachmentsJSON(atts)
	decoded, err := parseAttachmentsJSON(encoded)
	if err != nil {
		t.Fatalf("unexpected error decoding round-tripped JSON: %v", err)
	}
	if len(decoded) != len(atts) {
		t.Fatalf("got %d attachments, want %d", len(decoded), len(atts))
	}
	for i := range atts {
		if decoded[i] != atts[i] {
			t.Errorf("decoded[%d] = %+v, want %+v", i, decoded[i], atts[i])
		}
	}
}

func TestEncodeAttachmentsJSON_Empty(t *testing.T) {
	if got := encodeAttachmentsJSON(nil); got != "[]" {
		t.Errorf("encodeAttachmentsJSON(nil) = %q, want %q", got, "[]")
	}
	if got := encodeAttachmentsJSON([]messageAttachment{}); got != "[]" {
		t.Errorf("encodeAttachmentsJSON([]) = %q, want %q", got, "[]")
	}
}

func TestAttachmentsToCloudFiles(t *testing.T) {
	atts := []messageAttachment{
		{CloudID: "1", File: "cloud/1.png", Name: "photo.png", Extension: "png", FileType: "image", URL: "https://x/cloud/1.png"},
	}
	files := attachmentsToCloudFiles(atts)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	want := edupage.CloudFile{CloudID: "1", File: "cloud/1.png", Name: "photo.png", Extension: "png", FileType: "image"}
	if files[0] != want {
		t.Errorf("got %+v, want %+v (note: the display-only URL is not part of edupage.CloudFile)", files[0], want)
	}
}

func TestAttachmentsToCloudFiles_Empty(t *testing.T) {
	if files := attachmentsToCloudFiles(nil); len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestResolveRecipientNames(t *testing.T) {
	studentOpts := []messageRecipientOption{{ID: "Student1", Name: "Zed Student"}}
	teacherOpts := []messageRecipientOption{{ID: "Teacher1", Name: "Anna Teacher"}}

	t.Run("resolves and sorts known ids", func(t *testing.T) {
		names := resolveRecipientNames([]string{"Student1", "Teacher1"}, studentOpts, teacherOpts)
		want := []string{"Anna Teacher", "Zed Student"}
		if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
			t.Errorf("got %v, want %v", names, want)
		}
	})

	t.Run("falls back to the raw id for an unknown recipient", func(t *testing.T) {
		names := resolveRecipientNames([]string{"Student999"}, studentOpts, teacherOpts)
		if len(names) != 1 || names[0] != "Student999" {
			t.Errorf("got %v, want [Student999]", names)
		}
	})
}

func TestSelectedRecipientSet(t *testing.T) {
	set := selectedRecipientSet([]string{"Student1", "Teacher2"})
	if !set["Student1"] || !set["Teacher2"] {
		t.Errorf("expected both ids selected, got %v", set)
	}
	if set["Student999"] {
		t.Error("unrelated id should not be selected")
	}
}

func TestNewMessagesPageData_ComposeState(t *testing.T) {
	data := newMessagesPageData(nil, nil, nil, []string{"Student1"}, "hello", nil, "", false)
	if data.Confirming {
		t.Error("expected Confirming = false")
	}
	if !data.SelectedRecipients["Student1"] {
		t.Error("expected Student1 to be selected")
	}
	if data.RecipientNames != nil {
		t.Errorf("compose state should not resolve recipient names, got %v", data.RecipientNames)
	}
	if data.AttachmentsJSON != "[]" {
		t.Errorf("AttachmentsJSON = %q, want []", data.AttachmentsJSON)
	}
}

func TestNewMessagesPageData_ConfirmState(t *testing.T) {
	studentOpts := []messageRecipientOption{{ID: "Student1", Name: "Alpha Student"}}
	data := newMessagesPageData(nil, studentOpts, nil, []string{"Student1"}, "hello", nil, "", true)
	if !data.Confirming {
		t.Error("expected Confirming = true")
	}
	if len(data.RecipientNames) != 1 || data.RecipientNames[0] != "Alpha Student" {
		t.Errorf("RecipientNames = %v, want [Alpha Student]", data.RecipientNames)
	}
}
