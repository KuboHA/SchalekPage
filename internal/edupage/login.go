package edupage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TwoFactor carries the state needed to finish a 2FA login.
type TwoFactor struct {
	client *Client

	authEndpoint string // "gu"
	authToken    string // "au"
	csrfToken    string // "csrfauth"

	code string // set once IsConfirmed() returns true
}

// Login authenticates. It returns a non-nil *TwoFactor when the account
// requires a second factor; in that case the login is NOT yet complete.
// subdomain "login1" triggers portal auto-resolution.
func (c *Client) Login(username, password, subdomain string) (*TwoFactor, error) {
	resp, err := c.loginWithRPC(username, password, subdomain)
	if err != nil {
		return nil, err
	}

	if resp == nil {
		resp, err = c.loginLegacy(username, password, subdomain)
		if err != nil {
			return nil, err
		}
	}

	return c.finishLogin(resp, subdomain, username)
}

// loginWithRPC mirrors the RPC login process (mainlogin.js):
//  1. GET  /login/?cmd=MainLogin
//  2. POST /login/?cmd=MainLogin&akcia=getToken
//  3. POST /login/?cmd=MainLogin&akcia=login
//
// It returns (nil, nil) - not an error - whenever any step "soft fails" the
// way the Python reference does (bad status code, missing token, rejected
// csrf token, ...); the caller then falls back to the legacy login flow.
// A non-nil error is only returned for transport-level failures.
func (c *Client) loginWithRPC(username, password, subdomain string) (*http.Response, error) {
	base := "https://" + subdomain + ".edupage.org"

	warmup, err := c.httpClient.Get(base + "/login/?cmd=MainLogin")
	if err != nil {
		return nil, fmt.Errorf("edupage: rpc login warmup request: %w", err)
	}
	io.Copy(io.Discard, warmup.Body)
	warmup.Body.Close()
	if warmup.StatusCode != http.StatusOK {
		return nil, nil
	}

	tokenParams, err := json.Marshal(map[string]any{"username": username, "edupage": ""})
	if err != nil {
		return nil, fmt.Errorf("edupage: encode getToken params: %w", err)
	}
	tokenBody := EncodeRequestBody(map[string]string{"rpcparams": string(tokenParams)})

	tokenRaw, err := c.rawPost(base+"/login/?cmd=MainLogin&akcia=getToken", tokenBody)
	if err != nil {
		return nil, fmt.Errorf("edupage: rpc getToken request: %w", err)
	}
	if tokenRaw == nil {
		return nil, nil
	}

	tokenResp := parseRPCResponse(string(tokenRaw))
	token := ""
	if tokenResp != nil {
		token = asString(tokenResp["token"])
	}
	if token == "" {
		return nil, nil
	}

	loginParams, err := json.Marshal(map[string]any{
		"username":  username,
		"password":  password,
		"userToken": token,
		"edupage":   "",
		"ctxt":      "",
		"tu":        nil,
		"gu":        nil,
		"au":        nil,
	})
	if err != nil {
		return nil, fmt.Errorf("edupage: encode login params: %w", err)
	}
	loginBody := EncodeRequestBody(map[string]string{"rpcparams": string(loginParams)})

	loginRaw, err := c.rawPost(base+"/login/?cmd=MainLogin&akcia=login", loginBody)
	if err != nil {
		return nil, fmt.Errorf("edupage: rpc login request: %w", err)
	}
	if loginRaw == nil {
		return nil, nil
	}

	loginResp := parseRPCResponse(string(loginRaw))
	if loginResp == nil {
		return nil, nil
	}

	errID := ""
	if errObj := asMap(loginResp["err"]); errObj != nil {
		errID = asString(errObj["error_id"])
	}
	redirectURL := asString(loginResp["redirectUrl"])

	if errID == "invalid_token" || redirectURL == "" {
		return nil, nil
	}

	target, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("edupage: parse base url: %w", err)
	}
	ref, err := url.Parse(redirectURL)
	if err != nil {
		return nil, nil // malformed redirect URL: treat as a soft failure
	}

	finalResp, err := c.httpClient.Get(target.ResolveReference(ref).String())
	if err != nil {
		return nil, fmt.Errorf("edupage: follow rpc login redirect: %w", err)
	}
	return finalResp, nil
}

// rawPost issues a form-urlencoded POST with a body that is already fully
// encoded (e.g. by EncodeRequestBody), returning (nil, nil) on a non-200
// status the way the Python reference treats it as a soft failure.
func (c *Client) rawPost(rawURL, encodedBody string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(encodedBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	return body, nil
}

// parseRPCResponse decodes the eqz:/eqwd: envelope and parses the resulting
// JSON, returning nil (never an error) on any failure - matching the Python
// reference's `except: return None` behaviour, since a malformed RPC
// response is an expected "try the next thing" condition rather than a
// fatal error.
func parseRPCResponse(text string) map[string]any {
	if text == "" {
		return nil
	}
	decoded, err := DecodeResponse(text)
	if err != nil {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(decoded), &v); err != nil {
		return nil
	}
	return v
}

// loginLegacy performs the fallback edubarLogin.php + csrftoken login flow
// used when the RPC login flow above soft-fails.
func (c *Client) loginLegacy(username, password, subdomain string) (*http.Response, error) {
	requestURL := fmt.Sprintf("https://%s.edupage.org/login/?cmd=MainLogin", subdomain)

	getResp, err := c.httpClient.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("edupage: legacy login page request: %w", err)
	}
	body, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("edupage: read legacy login page: %w", err)
	}

	csrfToken, ok := between(string(body), `"csrftoken":"`, `"`)
	if !ok {
		return nil, fmt.Errorf("edupage: %w: EduPage did not provide a login token", ErrBadCredentials)
	}

	params := url.Values{
		"csrfauth": {csrfToken},
		"username": {username},
		"password": {password},
	}

	postURL := fmt.Sprintf("https://%s.edupage.org/login/edubarLogin.php", subdomain)
	req, err := http.NewRequest(http.MethodPost, postURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("edupage: build legacy login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("edupage: legacy login request: %w", err)
	}

	finalURL := resp.Request.URL.String()
	if strings.Contains(finalURL, "cap=1") || strings.Contains(finalURL, "lerr=b43b43") {
		resp.Body.Close()
		return nil, ErrCaptcha
	}
	if strings.Contains(finalURL, "bad=1") {
		resp.Body.Close()
		return nil, ErrBadCredentials
	}

	return resp, nil
}

// finishLogin parses the final login response, resolving 2FA if necessary.
func (c *Client) finishLogin(resp *http.Response, subdomain, username string) (*TwoFactor, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edupage: read login response: %w", err)
	}
	data := string(body)

	if subdomain == "login1" {
		host := resp.Request.URL.Hostname()
		if idx := strings.Index(host, "."); idx != -1 {
			subdomain = host[:idx]
		} else {
			subdomain = host
		}
	}

	c.subdomain = subdomain
	c.username = username

	if !strings.Contains(resp.Request.URL.String(), "twofactor") {
		if err := c.parseLoginData(data); err != nil {
			return nil, err
		}
		return nil, nil
	}

	requestURL := fmt.Sprintf("https://%s.edupage.org/login/twofactor?sn=1", c.subdomain)
	tfResp, err := c.httpClient.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("edupage: two-factor page request: %w", err)
	}
	defer tfResp.Body.Close()
	tfBody, err := io.ReadAll(tfResp.Body)
	if err != nil {
		return nil, fmt.Errorf("edupage: read two-factor page: %w", err)
	}

	csrfauth, ok1 := between(string(tfBody), `csrfauth" value="`, `"`)
	au, ok2 := between(string(tfBody), `au" value="`, `"`)
	gu, ok3 := between(string(tfBody), `gu" value="`, `"`)
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("edupage: %w: EduPage did not provide two-factor fields", ErrBadCredentials)
	}

	return &TwoFactor{
		client:       c,
		authEndpoint: gu,
		authToken:    au,
		csrfToken:    csrfauth,
	}, nil
}

// parseLoginData extracts the `userhome({...});` JSON payload and the
// ASC.gsechash scrape from an EduPage login/user page.
func (c *Client) parseLoginData(data string) error {
	afterMarker, ok := splitAfter(data, "userhome(")
	if !ok {
		return fmt.Errorf("edupage: %w: EduPage did not return login data", ErrBadCredentials)
	}

	// Mirrors Python's `.rsplit(");", 2)[0]`: drop everything from the
	// second-to-last ");" onward, discarding trailing script statements.
	jsonString := rsplitFirst(afterMarker, ");", 2)
	jsonString = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(jsonString)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonString), &parsed); err != nil {
		return fmt.Errorf("edupage: %w: could not parse login data: %v", ErrBadCredentials, err)
	}

	c.data = parsed

	if hash, ok := between(data, `ASC.gsechash="`, `"`); ok {
		c.gsecHash = hash
	} else {
		c.gsecHash = ""
	}

	return nil
}

// IsConfirmed polls whether the login was confirmed on a device. When it
// returns true, Finish may be called.
func (tf *TwoFactor) IsConfirmed() (bool, error) {
	body, err := tf.client.PostForm("/login/twofactor?akcia=checkIfConfirmed", url.Values{})
	if err != nil {
		return false, fmt.Errorf("edupage: check two-factor confirmation: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return false, fmt.Errorf("edupage: %w: invalid response checking two-factor confirmation", ErrMissingData)
	}

	status := asString(data["status"])
	if status == "fail" {
		return false, nil
	}
	if status != "ok" {
		return false, fmt.Errorf("edupage: %w: invalid response from EduPage: %v", ErrMissingData, data)
	}

	tf.code = asString(data["data"])
	return true, nil
}

// ResendNotifications re-sends the 2FA push notification.
func (tf *TwoFactor) ResendNotifications() error {
	body, err := tf.client.PostForm("/login/twofactor?akcia=resendNotifs", url.Values{})
	if err != nil {
		return fmt.Errorf("edupage: resend two-factor notifications: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil || asString(data["status"]) != "ok" {
		return fmt.Errorf("edupage: failed to resend notifications: %s", string(body))
	}
	return nil
}

// Finish completes a device-confirmed 2FA login.
func (tf *TwoFactor) Finish() error {
	if tf.code == "" {
		return fmt.Errorf("edupage: %w: not confirmed (call IsConfirmed first)", ErrBadCredentials)
	}
	return tf.finish(tf.code)
}

// FinishWithCode completes a 2FA login using an emailed/app code.
func (tf *TwoFactor) FinishWithCode(code string) error {
	return tf.finish(code)
}

func (tf *TwoFactor) finish(code string) error {
	params := url.Values{
		"csrfauth": {tf.csrfToken},
		"t2fasec":  {code},
		"2fNoSave": {"y"},
		"2fform":   {"1"},
		"gu":       {tf.authEndpoint},
		"au":       {tf.authToken},
	}

	body, err := tf.client.PostForm("/login/edubarLogin.php", params)
	if err != nil {
		return fmt.Errorf("edupage: finish two-factor login: %w", err)
	}

	if !strings.Contains(string(body), "window.location = gu;") {
		return ErrSecondFactorFailed
	}

	sessionID := tf.client.SessionID()
	return tf.client.Restore(tf.client.subdomain, sessionID, tf.client.username)
}

// Restore re-attaches a client to an existing server session, so a web request
// can be served without re-entering the password. It sets the PHPSESSID cookie
// and re-fetches /user to repopulate Data.
func (c *Client) Restore(subdomain, sessionID, username string) error {
	c.subdomain = subdomain
	c.username = username

	u := &url.URL{Scheme: "https", Host: subdomain + ".edupage.org"}
	c.jar.SetCookies(u, []*http.Cookie{{Name: "PHPSESSID", Value: sessionID}})

	body, err := c.Get("/user")
	if err != nil {
		return fmt.Errorf("edupage: restore session: %w", err)
	}

	if err := c.parseLoginData(string(body)); err != nil {
		return fmt.Errorf("edupage: %w: invalid session id: %v", ErrBadCredentials, err)
	}
	return nil
}

// between returns the substring of s between the first occurrence of start
// and the following occurrence of end, and whether both were found.
func between(s, start, end string) (string, bool) {
	after, ok := splitAfter(s, start)
	if !ok {
		return "", false
	}
	idx := strings.Index(after, end)
	if idx == -1 {
		return "", false
	}
	return after[:idx], true
}

// splitAfter returns the part of s following the first occurrence of sep,
// and whether sep was found.
func splitAfter(s, sep string) (string, bool) {
	idx := strings.Index(s, sep)
	if idx == -1 {
		return "", false
	}
	return s[idx+len(sep):], true
}

// rsplitFirst mimics Python's str.rsplit(sep, maxsplit)[0]: it splits s from
// the right at most maxsplit times and returns the remainder (i.e.
// everything before the maxsplit-th-from-the-end occurrence of sep).
func rsplitFirst(s, sep string, maxsplit int) string {
	for i := 0; i < maxsplit; i++ {
		idx := strings.LastIndex(s, sep)
		if idx == -1 {
			break
		}
		s = s[:idx]
	}
	return s
}
