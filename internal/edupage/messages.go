package edupage

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SendMessage sends a timeline message carrying text to recipients (ids like
// "Student123" or "Teacher45", or the literal "*" for the whole school). It
// returns the id of the newly created timeline item.
//
// ⚠️ This is a write operation: it posts a real message visible to every
// listed recipient. There is no confirmation step here — callers (web/MCP)
// are responsible for asking the user first.
func (c *Client) SendMessage(recipients []string, text string) (int, error) {
	return c.SendMessageWithAttachments(recipients, text, nil)
}

// SendMessageWithAttachments is SendMessage with cloud files (see
// Client.CloudUpload) attached to the message.
//
// The Python reference implementation can upload a file to EduPage's cloud
// storage but always hardcodes attachements="{}" when sending, so an
// uploaded file can never actually be attached to a message there. This
// wires the two together instead, matching the shape EduPage's own web
// client sends: a JSON object mapping each attached file's cloud path to its
// display name.
func (c *Client) SendMessageWithAttachments(recipients []string, text string, files []CloudFile) (int, error) {
	if len(recipients) == 0 {
		return 0, fmt.Errorf("edupage: send message: no recipients given")
	}
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("edupage: send message: text is empty")
	}

	attachments, err := encodeMessageAttachments(files)
	if err != nil {
		return 0, fmt.Errorf("edupage: send message: %w", err)
	}

	body := map[string]string{
		"selectedUser": strings.Join(recipients, ";"),
		"text":         text,
		"attachements": attachments, // sic — matches EduPage's own field name
		"receipt":      "0",
		"typ":          "sprava",
	}

	resp, err := c.PostEncoded("/timeline/?=&akcia=createItem&eqav=1&maxEqav=7", body)
	if err != nil {
		return 0, fmt.Errorf("edupage: send message: %w", err)
	}

	id, err := parseSendMessageResponse(resp)
	if err != nil {
		return 0, fmt.Errorf("edupage: send message: %w", err)
	}
	return id, nil
}

// encodeMessageAttachments builds the JSON object EduPage's createItem
// expects for the "attachements" field: cloud file path -> display name. No
// attachments encodes as the literal empty object, matching what EduPage
// itself sends for a plain text message.
func encodeMessageAttachments(files []CloudFile) (string, error) {
	if len(files) == 0 {
		return "{}", nil
	}

	named := make(map[string]string, len(files))
	for _, f := range files {
		named[f.File] = f.Name
	}

	encoded, err := json.Marshal(named)
	if err != nil {
		return "", fmt.Errorf("encode attachments: %w", err)
	}
	return string(encoded), nil
}

// parseSendMessageResponse parses the (already Client.PostEncoded-decoded)
// response body from createItem into the new timeline item's id.
//
// EduPage signals a rejected message in one of three ways, all with an
// HTTP 200: the literal decoded body "0", a JSON object with a non-empty
// "error" field, or a JSON object whose "changes" array is present but
// empty. On success the new id is at changes[0].timelineid.
func parseSendMessageResponse(body []byte) (int, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "0" {
		return 0, fmt.Errorf("message was rejected")
	}

	var resp struct {
		Error   string           `json:"error"`
		Changes []map[string]any `json:"changes"`
	}
	if err := json.Unmarshal([]byte(trimmed), &resp); err != nil {
		return 0, fmt.Errorf("unexpected response: %w", err)
	}
	if resp.Error != "" {
		return 0, fmt.Errorf("edupage rejected the request: %s", resp.Error)
	}
	if len(resp.Changes) == 0 {
		return 0, fmt.Errorf("message was rejected (no changes reported)")
	}

	id, ok := pickInt(resp.Changes[0]["timelineid"])
	if !ok {
		return 0, fmt.Errorf("%w: response has no timelineid", ErrMissingData)
	}
	return id, nil
}
