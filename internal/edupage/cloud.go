package edupage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// CloudFile is a file uploaded to EduPage's cloud storage via
// Client.CloudUpload, as returned by /timeline/?akcia=uploadAtt.
//
// ⚠️ Uploaded files are publicly readable by anyone with the URL (see
// CloudFile.URL) — EduPage does not gate cloud storage behind auth. Callers
// must make that plain to the user before uploading anything sensitive.
type CloudFile struct {
	CloudID   string
	Extension string
	FileType  string
	// File is the cloud-relative path EduPage identifies this upload by.
	// It's also what SendMessageWithAttachments keys the "attachements"
	// object on.
	File string
	Name string
}

// URL returns the public download URL for f on c's school.
func (f CloudFile) URL(c *Client) string {
	if f.File == "" {
		return ""
	}
	if strings.HasPrefix(f.File, "http://") || strings.HasPrefix(f.File, "https://") {
		return f.File
	}

	path := f.File
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL() + path
}

// CloudUpload uploads the contents read from r, named filename, to EduPage's
// cloud storage. The returned CloudFile can be passed to
// Client.SendMessageWithAttachments to attach it to a message.
//
// ⚠️ This is a write operation with no undo: EduPage exposes no delete API
// used by this client, and the resulting file is publicly readable by URL.
func (c *Client) CloudUpload(filename string, r io.Reader) (*CloudFile, error) {
	contentType, body, err := buildUploadAttRequest(filename, r)
	if err != nil {
		return nil, fmt.Errorf("edupage: cloud upload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL()+"/timeline/?akcia=uploadAtt", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("edupage: cloud upload: build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	// Uploads move far more data than any other call, and EduPage accepts
	// files up to ~50MB. The shared client timeout is tuned for small JSON
	// requests and would abort a large upload on a slow link partway
	// through, so this one request gets a longer deadline of its own.
	resp, err := c.doWithTimeout(req, cloudUploadTimeout)
	if err != nil {
		return nil, fmt.Errorf("edupage: cloud upload: %w", err)
	}

	file, err := parseCloudUploadResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("edupage: cloud upload: %w", err)
	}
	return file, nil
}

// buildUploadAttRequest builds a multipart/form-data body with the uploaded
// file under the field name "att", matching what EduPage's own web client
// sends to /timeline/?akcia=uploadAtt. Factored out from CloudUpload so the
// multipart framing is testable without a network round trip.
func buildUploadAttRequest(filename string, r io.Reader) (contentType string, body []byte, err error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("att", filename)
	if err != nil {
		return "", nil, fmt.Errorf("create multipart field: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", nil, fmt.Errorf("write file contents: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", nil, fmt.Errorf("close multipart writer: %w", err)
	}

	return w.FormDataContentType(), buf.Bytes(), nil
}

// parseCloudUploadResponse parses the JSON response body from uploadAtt into
// a *CloudFile. EduPage reports failure via a "status" field that isn't
// "ok" (optionally alongside an "error" message).
func parseCloudUploadResponse(body []byte) (*CloudFile, error) {
	// EduPage nests the uploaded file's descriptor under "data". The fields
	// are also accepted at the top level because older/other deployments put
	// them there; reading only the top level yields a CloudFile with every
	// field empty, which silently attaches a broken reference to a message.
	type cloudFileFields struct {
		CloudID   any    `json:"cloudid"`
		Extension string `json:"extension"`
		Type      string `json:"type"`
		File      string `json:"file"`
		Name      string `json:"name"`
	}
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   *cloudFileFields
		cloudFileFields
	}
	if err := json.Unmarshal(bytes.TrimSpace(body), &resp); err != nil {
		return nil, fmt.Errorf("unexpected response: %w", err)
	}
	if resp.Status != "ok" {
		if resp.Error != "" {
			return nil, fmt.Errorf("edupage rejected the upload: %s", resp.Error)
		}
		return nil, fmt.Errorf("edupage rejected the upload (status %q)", resp.Status)
	}

	fields := resp.cloudFileFields
	if resp.Data != nil {
		fields = *resp.Data
	}

	file := &CloudFile{
		CloudID:   pickString(fields.CloudID),
		Extension: fields.Extension,
		FileType:  fields.Type,
		File:      fields.File,
		Name:      fields.Name,
	}
	// A "successful" upload that produced no usable reference is a failure as
	// far as callers are concerned — attaching it would reference nothing.
	if file.CloudID == "" && file.File == "" {
		return nil, fmt.Errorf("edupage reported success but returned no file reference")
	}
	return file, nil
}

// cloudUploadTimeout bounds a single file upload. It is deliberately much
// larger than the shared per-request timeout because a 50MB attachment on a
// slow connection is a legitimate, slow-but-healthy request rather than a
// hung one.
const cloudUploadTimeout = 10 * time.Minute

// doWithTimeout runs req against a shallow copy of the client's HTTP client
// with a different timeout, so one unusually long request cannot change the
// deadline every other request runs under. The cookie jar is shared, so the
// session travels with it.
func (c *Client) doWithTimeout(req *http.Request, timeout time.Duration) ([]byte, error) {
	slow := *c.httpClient
	slow.Timeout = timeout

	resp, err := slow.Do(req)
	if err != nil {
		return nil, fmt.Errorf("edupage: request to %s failed: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edupage: read response from %s: %w", req.URL.Path, err)
	}
	return body, nil
}
