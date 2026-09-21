package web

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// newMessagesTestServer builds a *Server with real parsed templates (so
// s.render — which every non-network branch of these handlers goes
// through — actually works) and no network-touching state.
func newMessagesTestServer(t *testing.T) *Server {
	t.Helper()
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("ParseTemplates: %v", err)
	}
	return &Server{
		templates: tmpl,
		sessions:  NewStore(time.Hour),
		loginRate: newRateLimiter(loginRateLimit, loginRateWindow),
		oauth:     newOAuthStore(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:       time.Now,
	}
}

// newMessagesTestSession returns a session with a live but never-logged-in
// *edupage.Client. Students()/Teachers() read straight from the client's
// in-memory login payload (nil here) rather than making an HTTP call, so
// this is safe to use in handler tests without touching the network —
// unlike SendMessageWithAttachments/CloudUpload, which do make real HTTP
// calls and must never be exercised this way.
func newMessagesTestSession() *Session {
	return &Session{
		ID:          "test-session",
		Client:      edupage.New(0),
		StudentName: "Test Student",
	}
}

func postForm(t *testing.T, h func(w http.ResponseWriter, r *http.Request, sess *Session), sess *Session, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/messages/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h(rec, req, sess)
	return rec
}

func TestHandleMessagesGet_RendersComposeForm(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	req := httptest.NewRequest(http.MethodGet, "/messages", nil)
	rec := httptest.NewRecorder()
	s.handleMessagesGet(rec, req, sess)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Review &amp; send") {
		t.Error("expected the compose form's submit button in the response body")
	}
	if strings.Contains(rec.Body.String(), "value=\"*\"") {
		t.Error("the whole-school recipient must never appear as a selectable option")
	}
}

func TestHandleMessagesGet_ShowsSentBanner(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	req := httptest.NewRequest(http.MethodGet, "/messages?sent=98765", nil)
	rec := httptest.NewRecorder()
	s.handleMessagesGet(rec, req, sess)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "98765") {
		t.Error("expected the sent message's id in the success banner")
	}
}

func TestHandleMessagesSend_RejectsEmptyRecipients(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	rec := postForm(t, s.handleMessagesSend, sess, url.Values{
		"body":   {"hello"},
		"action": {"review"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (validation errors render inline, not as a 500)", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "choose at least one recipient") {
		t.Errorf("expected the empty-recipients error message, got: %s", rec.Body.String())
	}
}

func TestHandleMessagesSend_RejectsEmptyBody(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	rec := postForm(t, s.handleMessagesSend, sess, url.Values{
		"recipient": {"Student1"},
		"body":      {"   "},
		"action":    {"review"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "message text cannot be empty") {
		t.Errorf("expected the empty-body error message, got: %s", rec.Body.String())
	}
}

func TestHandleMessagesSend_RejectsWholeSchoolRecipient(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	rec := postForm(t, s.handleMessagesSend, sess, url.Values{
		"recipient": {"*"},
		"body":      {"hello everyone"},
		"action":    {"review"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "whole school") {
		t.Errorf("expected a whole-school-rejection error message, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Nothing has been sent yet") {
		t.Error("a rejected whole-school request must not reach the confirmation step")
	}
}

func TestHandleMessagesSend_ReviewStepDoesNotSend(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	rec := postForm(t, s.handleMessagesSend, sess, url.Values{
		"recipient": {"Student1"},
		"body":      {"hello"},
		"action":    {"review"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Confirm &amp; send") {
		t.Error("a validated review submission should render the confirmation step")
	}
	if !strings.Contains(body, "Student1") {
		t.Error("the confirmation step should name the recipient (falling back to the raw id when not in the roster)")
	}
}

func TestHandleMessagesSend_EditStepReturnsToCompose(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	rec := postForm(t, s.handleMessagesSend, sess, url.Values{
		"recipient": {"Student1"},
		"body":      {"hello"},
		"action":    {"edit"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Review &amp; send") {
		t.Error("action=edit should render the compose form, not the confirmation step")
	}
}

func TestHandleMessagesSend_InvalidAttachmentsJSON(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	rec := postForm(t, s.handleMessagesSend, sess, url.Values{
		"recipient":        {"Student1"},
		"body":             {"hello"},
		"action":           {"review"},
		"attachments_json": {"not json"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Attachment data was invalid") {
		t.Errorf("expected an invalid-attachment-data error, got: %s", rec.Body.String())
	}
}

func TestHandleMessagesUpload_NoFile(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.Close() // empty multipart body, no "file" part

	req := httptest.NewRequest(http.MethodPost, "/messages/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleMessagesUpload(rec, req, sess)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "No file was uploaded") {
		t.Errorf("expected a no-file error, got: %s", rec.Body.String())
	}
}

func TestHandleMessagesUpload_RejectsOversizedFile(t *testing.T) {
	s := newMessagesTestServer(t)
	sess := newMessagesTestSession()

	// Just over the per-file cap, but comfortably under the MaxBytesReader
	// backstop (cap+512KB), so the request body itself is accepted and the
	// rejection exercised is specifically handleMessagesUpload's own
	// header.Size / validateUploadSize check, not the outer body-size
	// guard.
	oversized := maxMessageUploadBytes + 8192

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "big.bin")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.CopyN(part, zeroReader{}, int64(oversized)); err != nil {
		t.Fatalf("write file contents: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/messages/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleMessagesUpload(rec, req, sess)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "too large") {
		t.Errorf("expected a too-large error, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), strconv.Itoa(maxMessageUploadBytes/(1024*1024))) {
		t.Errorf("expected the error to mention the MB cap, got: %s", rec.Body.String())
	}
}

// zeroReader is an io.Reader that yields an endless stream of zero bytes,
// used to build a large-but-cheap upload body in
// TestHandleMessagesUpload_RejectsOversizedFile without holding the whole
// thing in a []byte at once.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
