package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// messagesRoster fetches the student/teacher roster and the navbar's
// student summary in one place, since every messages.html render needs
// both. Both calls are answered from the cached login payload already
// sitting in memory (see edupage.Client.Students/Teachers), so there is no
// extra HTTP round trip here.
func (s *Server) messagesRoster(sess *Session) (student *studentSummary, studentOpts, teacherOpts []messageRecipientOption) {
	students, err := sess.Client.Students()
	if err != nil {
		s.logger.Warn("messages: fetch students failed", "error", err)
	}
	teachers, err := sess.Client.Teachers()
	if err != nil {
		s.logger.Warn("messages: fetch teachers failed", "error", err)
	}

	resolved, ok := resolveStudent(students, sess.StudentID, sess.StudentName)
	studentOpts, teacherOpts = buildRecipientOptions(students, teachers)
	return studentSummaryOrNil(resolved.Name, ok), studentOpts, teacherOpts
}

// handleMessagesGet serves GET /messages: an empty compose form, or — after
// handleMessagesSend's post-send redirect — a success banner naming the new
// timeline id.
func (s *Server) handleMessagesGet(w http.ResponseWriter, r *http.Request, sess *Session) {
	student, studentOpts, teacherOpts := s.messagesRoster(sess)

	data := newMessagesPageData(student, studentOpts, teacherOpts, nil, "", nil, "", false)

	if raw := r.URL.Query().Get("sent"); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil {
			data.Sent = true
			data.SentID = id
		}
	}

	s.render(w, r, "messages.html", data)
}

// handleMessagesSend serves POST /messages/send, the compose form's submit
// target for all three steps of sending a message: the initial "Review &
// Send", the confirmation step's "Confirm & Send", and its "Back to edit".
// Which step a submission is depends solely on its "action" field
// ("review", "confirm", or "edit") — never on any state held server-side —
// because the only thing this page carries between steps is what the
// browser resubmits in hidden fields.
//
// ⚠️ This sends a real message to real people at a real school, so
// SendMessageWithAttachments is reachable only via action=="confirm", and
// only after recipients/body/attachments are validated *again* at that
// point — never trusting that an earlier "review" step already checked
// them, in case the confirm form was resubmitted with tampered fields.
func (s *Server) handleMessagesSend(w http.ResponseWriter, r *http.Request, sess *Session) {
	student, studentOpts, teacherOpts := s.messagesRoster(sess)

	if err := r.ParseForm(); err != nil {
		s.render(w, r, "messages.html", newMessagesPageData(student, studentOpts, teacherOpts, nil, "", nil, "Invalid form submission.", false))
		return
	}

	rawRecipients := r.Form["recipient"]
	body := r.FormValue("body")
	action := r.FormValue("action")

	atts, attErr := parseAttachmentsJSON(r.FormValue("attachments_json"))

	// "Back to edit": return to the compose view with whatever the browser
	// still has in its hidden fields, no validation, no scolding — the user
	// asked to change something, not to submit.
	if action == "edit" {
		data := newMessagesPageData(student, studentOpts, teacherOpts, rawRecipients, body, atts, "", false)
		s.render(w, r, "messages.html", data)
		return
	}

	if attErr != nil {
		data := newMessagesPageData(student, studentOpts, teacherOpts, rawRecipients, body, nil,
			"Attachment data was invalid — please re-attach your files and try again.", false)
		s.render(w, r, "messages.html", data)
		return
	}

	recipients, err := validateMessageRecipients(rawRecipients)
	if err != nil {
		data := newMessagesPageData(student, studentOpts, teacherOpts, rawRecipients, body, atts, err.Error(), false)
		s.render(w, r, "messages.html", data)
		return
	}
	if err := validateMessageBody(body); err != nil {
		data := newMessagesPageData(student, studentOpts, teacherOpts, recipients, body, atts, err.Error(), false)
		s.render(w, r, "messages.html", data)
		return
	}

	if action != "confirm" {
		// "Review & Send" (or anything else): validation passed, but this
		// is not yet the confirmed submission. Show exactly who this will
		// reach and stop — nothing has been sent.
		data := newMessagesPageData(student, studentOpts, teacherOpts, recipients, body, atts, "", true)
		s.render(w, r, "messages.html", data)
		return
	}

	id, err := sess.Client.SendMessageWithAttachments(recipients, body, attachmentsToCloudFiles(atts))
	if err != nil {
		s.logger.Warn("messages: send failed", "error", err)
		data := newMessagesPageData(student, studentOpts, teacherOpts, recipients, body, atts,
			"Could not send the message: "+err.Error(), false)
		s.render(w, r, "messages.html", data)
		return
	}

	// POST/redirect/GET: a reload of the success page never re-sends.
	http.Redirect(w, r, fmt.Sprintf("/messages?sent=%d", id), http.StatusSeeOther)
}

// uploadResponse is handleMessagesUpload's JSON response shape, for both
// the success and error cases (distinguished by OK).
type uploadResponse struct {
	OK         bool               `json:"ok"`
	Error      string             `json:"error,omitempty"`
	Attachment *messageAttachment `json:"attachment,omitempty"`
}

func writeUploadResponse(w http.ResponseWriter, status int, resp uploadResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleMessagesUpload serves POST /messages/upload: an AJAX endpoint the
// compose page's JS calls immediately when a file is picked, before the
// message itself is sent. It uploads straight to EduPage's cloud storage
// (Client.CloudUpload) and returns the resulting attachment as JSON; the
// page folds it into the compose form's "attachments_json" hidden field, so
// no server-side draft state is kept for it (see messageAttachment's doc
// comment).
//
// ⚠️ Uploaded files are hosted permanently and are publicly readable by
// anyone who has the URL — EduPage does not gate cloud storage behind auth.
// The page states this next to the file input; nothing here can enforce
// that a user actually reads it, so the handler itself refuses nothing on
// privacy grounds beyond what the UI already warned about.
func (s *Server) handleMessagesUpload(w http.ResponseWriter, r *http.Request, sess *Session) {
	maxMB := strconv.Itoa(maxMessageUploadBytes / (1024 * 1024))

	// A little slack over the hard cap for multipart framing overhead
	// (boundaries, headers), so a file exactly at the cap doesn't get
	// rejected by request-body overhead alone.
	r.Body = http.MaxBytesReader(w, r.Body, maxMessageUploadBytes+512*1024)

	if err := r.ParseMultipartForm(maxMessageUploadMemory); err != nil {
		writeUploadResponse(w, http.StatusBadRequest, uploadResponse{
			Error: "Could not read the uploaded file — it may be larger than the " + maxMB + " MB limit.",
		})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeUploadResponse(w, http.StatusBadRequest, uploadResponse{Error: "No file was uploaded."})
		return
	}
	defer file.Close()

	if err := validateUploadSize(header.Size); err != nil {
		writeUploadResponse(w, http.StatusBadRequest, uploadResponse{
			Error: "\"" + header.Filename + "\" is too large — attachments are capped at " + maxMB + " MB.",
		})
		return
	}

	cf, err := sess.Client.CloudUpload(header.Filename, file)
	if err != nil {
		s.logger.Warn("messages: upload failed", "error", err)
		writeUploadResponse(w, http.StatusBadGateway, uploadResponse{Error: "Upload failed: " + err.Error()})
		return
	}

	att := messageAttachment{
		CloudID:   cf.CloudID,
		File:      cf.File,
		Name:      cf.Name,
		Extension: cf.Extension,
		FileType:  cf.FileType,
		URL:       cf.URL(sess.Client),
	}
	writeUploadResponse(w, http.StatusOK, uploadResponse{OK: true, Attachment: &att})
}
