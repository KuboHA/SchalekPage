package web

import (
	"testing"
	"time"
)

func TestTimeSincePosted(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		ts   time.Time
		want string
	}{
		{"just now", now.Add(-10 * time.Second), "Just now"},
		{"one minute", now.Add(-1 * time.Minute), "1 minute ago"},
		{"five minutes", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"one hour", now.Add(-1 * time.Hour), "1 hour ago"},
		{"three hours", now.Add(-3 * time.Hour), "3 hours ago"},
		{"one day", now.Add(-25 * time.Hour), "1 day ago"},
		{"three days", now.Add(-73 * time.Hour), "3 days ago"},
		{"future timestamp clamps to just now", now.Add(5 * time.Minute), "Just now"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := timeSincePosted(tc.ts, now); got != tc.want {
				t.Errorf("timeSincePosted() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractConfirmCount(t *testing.T) {
	cases := []struct {
		name string
		data any
		want int
	}{
		{"nil", nil, 0},
		{"number", float64(3), 3},
		{"list", []any{1, 2, 3}, 3},
		{"confirm key", map[string]any{"confirmedBy": []any{1, 2}}, 2},
		{"like key nested count", map[string]any{"likes": map[string]any{"count": float64(7)}}, 7},
		{"count field direct", map[string]any{"count": float64(4)}, 4},
		{"irrelevant keys ignored", map[string]any{"title": "hi"}, 0},
		{"prefers max across matching keys", map[string]any{
			"thumbsUp": float64(2),
			"confirm":  []any{1, 2, 3, 4},
		}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractConfirmCount(tc.data); got != tc.want {
				t.Errorf("extractConfirmCount(%v) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

func TestResolveAttachmentURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		base string
		want string
	}{
		{"already absolute", "https://example.com/f.pdf", "https://school.edupage.org", "https://example.com/f.pdf"},
		{"relative with leading slash", "/elearning/f.pdf", "https://school.edupage.org", "https://school.edupage.org/elearning/f.pdf"},
		{"relative without leading slash", "elearning/f.pdf", "https://school.edupage.org", "https://school.edupage.org/elearning/f.pdf"},
		{"no base URL", "elearning/f.pdf", "", "elearning/f.pdf"},
		{"empty", "", "https://school.edupage.org", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveAttachmentURL(tc.raw, tc.base); got != tc.want {
				t.Errorf("resolveAttachmentURL(%q, %q) = %q, want %q", tc.raw, tc.base, got, tc.want)
			}
		})
	}
}

func TestExtractAttachmentsListShape(t *testing.T) {
	data := map[string]any{
		"attachments": []any{
			map[string]any{"name": "Homework.jpg", "url": "/f/homework.jpg"},
			"notes.pdf",
		},
	}
	atts := extractAttachments(data, "https://school.edupage.org")
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %d: %+v", len(atts), atts)
	}
	if !atts[0].IsImage {
		t.Errorf("expected first attachment to be flagged as an image: %+v", atts[0])
	}
	if atts[1].IsImage {
		t.Errorf("expected second attachment not to be flagged as an image: %+v", atts[1])
	}
}

func TestExtractAttachmentsElearningScan(t *testing.T) {
	// A non-empty string value becomes the display name, as in app.py's
	// _scan ("Worksheet" here, not derived from the path). Unlike app.py,
	// which derives is_image purely from that name and would therefore
	// miss this JPEG, looksLikeJPEG falls back to the URL's path
	// extension, so an unambiguous .jpeg attachment still gets flagged as
	// an image even with an opaque display name.
	data := map[string]any{
		"nested": map[string]any{
			"/elearning/materials/sheet.jpeg": "Worksheet",
		},
	}
	atts := extractAttachments(data, "https://school.edupage.org")
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d: %+v", len(atts), atts)
	}
	if atts[0].Name != "Worksheet" {
		t.Errorf("expected filename from value, got %q", atts[0].Name)
	}
	if !atts[0].IsImage {
		t.Errorf("expected .jpeg to be flagged as image via URL fallback")
	}
}

func TestLooksLikeJPEG(t *testing.T) {
	cases := []struct {
		name string
		att  string
		url  string
		want bool
	}{
		{"name has extension", "photo.jpg", "https://school.edupage.org/f/abc123", true},
		{"name opaque, url has extension", "Worksheet", "https://school.edupage.org/elearning/sheet.jpeg", true},
		{"url extension hidden behind query string", "Worksheet", "https://school.edupage.org/elearning/sheet.jpeg?v=2", true},
		{"genuine non-image", "notes.pdf", "https://school.edupage.org/f/notes.pdf", false},
		{"neither name nor url has an image extension", "attachment", "https://school.edupage.org/f/abc123", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeJPEG(tc.att, tc.url); got != tc.want {
				t.Errorf("looksLikeJPEG(%q, %q) = %v, want %v", tc.att, tc.url, got, tc.want)
			}
		})
	}
}

func TestExtractAttachmentsDeduplicates(t *testing.T) {
	data := map[string]any{
		"attachments": []any{
			map[string]any{"name": "a.pdf", "url": "/f/a.pdf"},
			map[string]any{"name": "a-again.pdf", "url": "/f/a.pdf"},
		},
	}
	atts := extractAttachments(data, "https://school.edupage.org")
	if len(atts) != 1 {
		t.Fatalf("expected duplicates to collapse to 1, got %d: %+v", len(atts), atts)
	}
}

func TestGuessParentByPrefix(t *testing.T) {
	idMap := map[int]*notificationView{
		1234:      {},
		123456789: {},
	}
	pid, ok := guessParentByPrefix(123456789, idMap)
	if !ok {
		t.Fatal("expected a parent guess")
	}
	if pid != 1234 {
		t.Errorf("got parent %d, want 1234", pid)
	}

	if _, ok := guessParentByPrefix(42, idMap); ok {
		t.Error("expected no guess for a short id")
	}
}
