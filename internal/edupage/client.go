// Package edupage is a Go client for the EduPage school system, ported from
// the Python reference implementation at https://github.com/EdupageAPI/edupage-api.
package edupage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is one logged-in EduPage session. Safe for sequential use by one user.
type Client struct {
	httpClient *http.Client
	jar        *cookiejar.Jar

	subdomain string
	username  string
	data      map[string]any
	gsecHash  string
}

// New returns a client with a cookie jar and the given request timeout.
func New(timeout time.Duration) *Client {
	jar, _ := cookiejar.New(nil) // cookiejar.New never actually errors with a nil Options
	return &Client{
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: timeout,
		},
		jar: jar,
	}
}

// Subdomain returns the resolved school subdomain (may differ from the one
// passed to Login when "login1" auto-resolution was used).
func (c *Client) Subdomain() string { return c.subdomain }

// Username returns the username the client logged in with.
func (c *Client) Username() string { return c.username }

// SessionID returns the current PHPSESSID cookie value.
func (c *Client) SessionID() string {
	if c.subdomain == "" {
		return ""
	}
	u := &url.URL{Scheme: "https", Host: c.subdomain + ".edupage.org"}
	for _, ck := range c.jar.Cookies(u) {
		if ck.Name == "PHPSESSID" {
			return ck.Value
		}
	}
	return ""
}

// Data returns the parsed `userhome(...)` payload from the login page.
func (c *Client) Data() map[string]any { return c.data }

// UserID returns the logged-in user's id (e.g. "Student1234"), from Data.
func (c *Client) UserID() string {
	if c.data == nil {
		return ""
	}
	return asString(c.data["userid"])
}

// GsecHash returns the ASC.gsechash value scraped at login ("" if absent).
func (c *Client) GsecHash() string { return c.gsecHash }

// baseURL returns https://<subdomain>.edupage.org.
func (c *Client) baseURL() string {
	return "https://" + c.subdomain + ".edupage.org"
}

// Get performs a GET against https://<subdomain>.edupage.org<path> and returns
// the raw body. `path` must begin with "/".
func (c *Client) Get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL()+path, nil)
	if err != nil {
		return nil, fmt.Errorf("edupage: build GET %s: %w", path, err)
	}
	return c.do(req)
}

// PostForm posts url-encoded values and returns the raw body.
func (c *Client) PostForm(path string, values url.Values) ([]byte, error) {
	return c.postFormRaw(path, values.Encode())
}

// postFormRaw posts a pre-encoded application/x-www-form-urlencoded body.
func (c *Client) postFormRaw(path, encodedBody string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL()+path, strings.NewReader(encodedBody))
	if err != nil {
		return nil, fmt.Errorf("edupage: build POST %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

// PostEncoded posts `body` wrapped in the EduPage eqap/eqacs/eqaz envelope
// (see compress.go: EncodeRequestBody) and returns the DECODED response body
// (DecodeResponse already applied).
func (c *Client) PostEncoded(path string, body map[string]string) ([]byte, error) {
	raw, err := c.postFormRaw(path, EncodeRequestBody(body))
	if err != nil {
		return nil, err
	}
	decoded, err := DecodeResponse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("edupage: decode response from %s: %w", path, err)
	}
	return []byte(decoded), nil
}

// PostJSON posts a JSON body with Content-Type application/json and returns the
// raw response body.
func (c *Client) PostJSON(path string, payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("edupage: encode JSON payload for %s: %w", path, err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL()+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("edupage: build POST %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

// do executes req using the client's http.Client and returns the raw
// response body. It does not treat non-2xx status codes as errors: EduPage
// returns application-level errors in the body itself, and callers that care
// about status codes have req.Response available via a lower-level API if
// ever needed.
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
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

// hasReloadKey reports whether a raw JSON response body signals a stale
// Client.GsecHash() by carrying a "reload" key in place of the expected
// payload — EduPage's way of signalling this instead of an HTTP error
// status. A "reload" key that is explicitly `false` or `null` does not
// count (mirrors the Python reference's `.get("reload") is not None`, with
// an added allowance for an explicit `false`); any other present value
// does. It returns false, without error, for a body that isn't a JSON
// object at all — callers should have already handled a transport error by
// that point.
func hasReloadKey(body []byte) bool {
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	v, ok := probe["reload"]
	if !ok || v == nil {
		return false
	}
	if b, isBool := v.(bool); isBool {
		return b
	}
	return true
}

// withSessionRecovery runs fetch once. If its result trips detectReload —
// meaning the endpoint reported a stale gsecHash — it transparently re-runs
// Restore to refresh Client.GsecHash() and retries fetch exactly once more.
// If the retry still trips detectReload (or Restore itself fails), it
// returns an error wrapping ErrSessionExpired. fetch is called at most
// twice, however detectReload is defined, so this never recurses.
func (c *Client) withSessionRecovery(fetch func() ([]byte, error), detectReload func([]byte) bool) ([]byte, error) {
	restore := func() error {
		return c.Restore(c.subdomain, c.SessionID(), c.username)
	}
	return retryOnReload(fetch, detectReload, restore)
}

// retryOnReload is the pure retry engine behind Client.withSessionRecovery:
// restore is taken as a parameter (rather than hardcoded to Client.Restore)
// purely so this logic — retry-at-most-once, give up with ErrSessionExpired
// — can be unit tested without a live EduPage session.
func retryOnReload(fetch func() ([]byte, error), detectReload func([]byte) bool, restore func() error) ([]byte, error) {
	body, err := fetch()
	if err != nil {
		return nil, err
	}
	if !detectReload(body) {
		return body, nil
	}

	if err := restore(); err != nil {
		return nil, fmt.Errorf("session expired and could not be refreshed: %w: %v", ErrSessionExpired, err)
	}

	body, err = fetch()
	if err != nil {
		return nil, err
	}
	if detectReload(body) {
		return nil, fmt.Errorf("session still expired after refresh: %w", ErrSessionExpired)
	}
	return body, nil
}

// --- defensive JSON helpers ---
//
// EduPage's JSON responses are famously loose about types: numbers may be
// strings, booleans may be "1"/"0" or omitted, and fields that are normally
// arrays may come back as empty objects ({}) when unpopulated. These helpers
// coerce `any` values from a decoded JSON document without ever panicking.

// asString coerces v to a string. nil becomes "". Numbers are formatted
// without unnecessary trailing zeros/exponents where possible.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(t)
	}
}

// asInt coerces v to an int, tolerating string-encoded numbers and floats.
// Non-numeric or missing values yield 0.
func asInt(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return int(t)
	case int:
		return t
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return 0
		}
		if n, err := strconv.Atoi(t); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

// asFloat coerces v to a float64, tolerating string-encoded numbers.
func asFloat(v any) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return t
	case string:
		t = strings.TrimSpace(t)
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// asMap coerces v to a map[string]any, returning nil if v is not a JSON
// object (e.g. it's an empty array "[]", which EduPage sometimes sends in
// place of an empty object).
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// asBool coerces v to a bool, tolerating "1"/"0"/"true"/"false" strings and
// numeric 1/0.
func asBool(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "true", "y", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
