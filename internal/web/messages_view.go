package web

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// wholeSchoolRecipient is EduPage's broadcast-to-everyone recipient id. The
// compose picker (see messageRecipientOptions) never offers it as a choice,
// and validateMessageRecipients rejects it outright — see the ⚠️ note on
// this page's handlers for why.
const wholeSchoolRecipient = "*"

// maxMessageUploadBytes bounds an attachment upload, matching EduPage's own
// cap (documented at ~50MB). maxMessageUploadMemory is how much of a
// multipart request ParseMultipartForm buffers in memory before spilling
// the rest to a temp file.
const (
	maxMessageUploadBytes  = 50 * 1024 * 1024
	maxMessageUploadMemory = 10 * 1024 * 1024
)

// messageRecipientOption is one selectable entry in the compose page's
// recipient picker.
type messageRecipientOption struct {
	ID   string // e.g. "Student123" or "Teacher45"
	Name string
}

// messageAttachment is a file already uploaded to EduPage's cloud storage
// (via Client.CloudUpload) and pending attachment to the message being
// composed. It round-trips through the compose form as JSON in a hidden
// field — see handlers_messages.go's handleMessagesUpload and
// handleMessagesSend — rather than living in server-side session state, so
// the draft survives nothing more than what the browser holds in the DOM.
type messageAttachment struct {
	CloudID   string `json:"cloudId"`
	File      string `json:"file"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	FileType  string `json:"fileType"`
	URL       string `json:"url"`
}

// messagesPageData is the view model for messages.html.
type messagesPageData struct {
	Student *studentSummary

	StudentOptions []messageRecipientOption
	TeacherOptions []messageRecipientOption

	// Form state, repopulated whenever the page re-renders instead of
	// redirecting: a validation error, or the confirmation step.
	SelectedRecipients map[string]bool
	RecipientNames     []string // resolved display names, for the confirm step
	Body               string
	Attachments        []messageAttachment
	AttachmentsJSON    string // re-serialised, so the confirm form can carry it forward

	Error      string
	Confirming bool

	Sent   bool
	SentID int

	MaxUploadMB int
}

// studentRecipientID and teacherRecipientID build the recipient ids EduPage
// expects (e.g. "Student123"), matching Client.SendMessage's doc comment.
func studentRecipientID(personID int) string { return fmt.Sprintf("Student%d", personID) }
func teacherRecipientID(personID int) string { return fmt.Sprintf("Teacher%d", personID) }

// buildRecipientOptions turns the class roster into the compose picker's
// options, sorted by name. The "*" whole-school broadcast is deliberately
// never included here — see wholeSchoolRecipient.
func buildRecipientOptions(students []edupage.Student, teachers []edupage.Teacher) (studentOpts, teacherOpts []messageRecipientOption) {
	studentOpts = make([]messageRecipientOption, 0, len(students))
	for _, st := range students {
		studentOpts = append(studentOpts, messageRecipientOption{ID: studentRecipientID(st.PersonID), Name: st.Name})
	}
	sort.Slice(studentOpts, func(i, j int) bool { return studentOpts[i].Name < studentOpts[j].Name })

	teacherOpts = make([]messageRecipientOption, 0, len(teachers))
	for _, t := range teachers {
		teacherOpts = append(teacherOpts, messageRecipientOption{ID: teacherRecipientID(t.PersonID), Name: t.Name})
	}
	sort.Slice(teacherOpts, func(i, j int) bool { return teacherOpts[i].Name < teacherOpts[j].Name })

	return studentOpts, teacherOpts
}

// validateMessageRecipients trims and de-duplicates the submitted recipient
// ids, rejecting an empty selection and — defensively, in case a request is
// crafted by hand rather than submitted through the picker — the literal
// "*" whole-school broadcast id. That id is the single most dangerous
// recipient EduPage accepts (it messages every person at the school), so it
// must never be reachable by accident from this page.
func validateMessageRecipients(raw []string) ([]string, error) {
	seen := make(map[string]bool, len(raw))
	cleaned := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if id == wholeSchoolRecipient {
			return nil, fmt.Errorf("sending to the whole school is not available from this page")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		cleaned = append(cleaned, id)
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("choose at least one recipient")
	}
	return cleaned, nil
}

// validateMessageBody rejects an empty or whitespace-only message body. The
// original (untrimmed) text is preserved so intentional leading/trailing
// formatting in a longer message isn't silently stripped.
func validateMessageBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("message text cannot be empty")
	}
	return nil
}

// parseAttachmentsJSON decodes the compose form's hidden "attachments_json"
// field. An empty string decodes to no attachments rather than an error, so
// a message with no files sent is the common case, not an edge case.
func parseAttachmentsJSON(raw string) ([]messageAttachment, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var atts []messageAttachment
	if err := json.Unmarshal([]byte(raw), &atts); err != nil {
		return nil, fmt.Errorf("invalid attachment data: %w", err)
	}
	return atts, nil
}

// encodeAttachmentsJSON is parseAttachmentsJSON's inverse, used to carry the
// attachment list forward across a validation error or the confirm step.
func encodeAttachmentsJSON(atts []messageAttachment) string {
	if len(atts) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(atts)
	if err != nil {
		// atts is built from JSON we ourselves decoded moments earlier, or
		// from a single freshly-uploaded file — there is no realistic way
		// for re-encoding it to fail.
		return "[]"
	}
	return string(encoded)
}

// validateUploadSize rejects an attachment before it is ever sent to
// EduPage, matching the cap enforced by handleMessagesUpload. Kept as a
// pure function so the boundary (exactly at the cap, one byte over, zero)
// is table-testable without materialising a multi-megabyte request body.
func validateUploadSize(size int64) error {
	if size > maxMessageUploadBytes {
		return fmt.Errorf("attachments are capped at %d MB", maxMessageUploadBytes/(1024*1024))
	}
	return nil
}

// attachmentsToCloudFiles converts the compose form's attachment list into
// the edupage.CloudFile slice Client.SendMessageWithAttachments expects.
func attachmentsToCloudFiles(atts []messageAttachment) []edupage.CloudFile {
	files := make([]edupage.CloudFile, 0, len(atts))
	for _, a := range atts {
		files = append(files, edupage.CloudFile{
			CloudID:   a.CloudID,
			Extension: a.Extension,
			FileType:  a.FileType,
			File:      a.File,
			Name:      a.Name,
		})
	}
	return files
}

// resolveRecipientNames maps recipient ids back to display names for the
// confirmation step, so the user sees exactly who a send will reach rather
// than raw ids like "Student482". An id with no match in the roster (stale
// data between page loads) falls back to showing the raw id rather than
// silently dropping it, since hiding a recipient here would be the one
// place that actually matters.
func resolveRecipientNames(recipients []string, studentOpts, teacherOpts []messageRecipientOption) []string {
	lookup := make(map[string]string, len(studentOpts)+len(teacherOpts))
	for _, o := range studentOpts {
		lookup[o.ID] = o.Name
	}
	for _, o := range teacherOpts {
		lookup[o.ID] = o.Name
	}

	names := make([]string, 0, len(recipients))
	for _, id := range recipients {
		if name, ok := lookup[id]; ok {
			names = append(names, name)
		} else {
			names = append(names, id)
		}
	}
	sort.Strings(names)
	return names
}

// selectedRecipientSet turns a recipient id slice into a set, for the
// compose template to check checkboxes back on after a re-render.
func selectedRecipientSet(recipients []string) map[string]bool {
	set := make(map[string]bool, len(recipients))
	for _, id := range recipients {
		set[id] = true
	}
	return set
}

// newMessagesPageData assembles messages.html's view model from the roster
// and whatever form state the current render needs to carry (a fresh
// compose form, a validation error, or the confirm step). It never itself
// talks to EduPage — the caller in handlers_messages.go supplies everything
// it needs so this stays a plain, table-testable function.
func newMessagesPageData(
	student *studentSummary,
	studentOpts, teacherOpts []messageRecipientOption,
	recipients []string,
	body string,
	atts []messageAttachment,
	errMsg string,
	confirming bool,
) messagesPageData {
	data := messagesPageData{
		Student:            student,
		StudentOptions:     studentOpts,
		TeacherOptions:     teacherOpts,
		SelectedRecipients: selectedRecipientSet(recipients),
		Body:               body,
		Attachments:        atts,
		AttachmentsJSON:    encodeAttachmentsJSON(atts),
		Error:              errMsg,
		Confirming:         confirming,
		MaxUploadMB:        maxMessageUploadBytes / (1024 * 1024),
	}
	if confirming {
		data.RecipientNames = resolveRecipientNames(recipients, studentOpts, teacherOpts)
	}
	return data
}
