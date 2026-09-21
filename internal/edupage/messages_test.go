package edupage

import (
	"errors"
	"strings"
	"testing"
)

func TestSendMessage_RejectsEmptyRecipients(t *testing.T) {
	c := New(0)
	if _, err := c.SendMessage(nil, "hello"); err == nil {
		t.Error("expected an error for no recipients")
	}
	if _, err := c.SendMessage([]string{}, "hello"); err == nil {
		t.Error("expected an error for an empty recipient slice")
	}
}

func TestSendMessage_RejectsEmptyText(t *testing.T) {
	c := New(0)
	if _, err := c.SendMessage([]string{"Student123"}, ""); err == nil {
		t.Error("expected an error for empty text")
	}
	if _, err := c.SendMessage([]string{"Student123"}, "   "); err == nil {
		t.Error("expected an error for whitespace-only text")
	}
}

func TestEncodeMessageAttachments(t *testing.T) {
	t.Run("no files encodes as empty object", func(t *testing.T) {
		got, err := encodeMessageAttachments(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "{}" {
			t.Errorf("got %q, want %q", got, "{}")
		}
	})

	t.Run("maps cloud path to display name", func(t *testing.T) {
		files := []CloudFile{
			{File: "cloud/abc123.pdf", Name: "homework.pdf"},
		}
		got, err := encodeMessageAttachments(files)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, `"cloud/abc123.pdf":"homework.pdf"`) {
			t.Errorf("got %q, want it to map the cloud path to the display name", got)
		}
	})
}

func TestParseSendMessageResponse(t *testing.T) {
	t.Run("literal 0 is a failure", func(t *testing.T) {
		if _, err := parseSendMessageResponse([]byte("0")); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("empty body is a failure", func(t *testing.T) {
		if _, err := parseSendMessageResponse([]byte("")); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("HTTP-200-with-error-field is a failure", func(t *testing.T) {
		_, err := parseSendMessageResponse([]byte(`{"error":"no permission","changes":[]}`))
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "no permission") {
			t.Errorf("error %q should mention the EduPage error message", err)
		}
	})

	t.Run("empty changes array is a failure", func(t *testing.T) {
		if _, err := parseSendMessageResponse([]byte(`{"changes":[]}`)); err == nil {
			t.Error("expected an error for an empty changes array")
		}
	})

	t.Run("missing changes field is a failure", func(t *testing.T) {
		if _, err := parseSendMessageResponse([]byte(`{}`)); err == nil {
			t.Error("expected an error when changes is entirely absent")
		}
	})

	t.Run("extracts the new id from changes[0].timelineid", func(t *testing.T) {
		id, err := parseSendMessageResponse([]byte(`{"changes":[{"timelineid":"98765","other":"field"}]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 98765 {
			t.Errorf("id = %d, want 98765", id)
		}
	})

	t.Run("timelineid as a JSON number also works", func(t *testing.T) {
		id, err := parseSendMessageResponse([]byte(`{"changes":[{"timelineid":98765}]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 98765 {
			t.Errorf("id = %d, want 98765", id)
		}
	})

	t.Run("missing timelineid is a failure", func(t *testing.T) {
		if _, err := parseSendMessageResponse([]byte(`{"changes":[{"other":"field"}]}`)); err == nil {
			t.Error("expected an error when timelineid is absent")
		}
	})

	t.Run("invalid JSON is a failure", func(t *testing.T) {
		if _, err := parseSendMessageResponse([]byte("not json")); err == nil {
			t.Error("expected an error for a non-JSON, non-\"0\" body")
		}
	})
}

func TestSendMessageWithAttachments_ErrorsSurfaceErrMissingData(t *testing.T) {
	_, err := parseSendMessageResponse([]byte(`{"changes":[{"other":"field"}]}`))
	if !errors.Is(err, ErrMissingData) {
		t.Errorf("expected ErrMissingData, got %v", err)
	}
}
