// OAuth 2.1 authorization server, minimal enough to satisfy the MCP
// "Authorization" spec for a remote HTTP server: RFC 7591 dynamic client
// registration, RFC 8414 / RFC 9728 metadata discovery, and the
// authorization-code + PKCE grant. There is no separate account system —
// "signing in" to an OAuth client is the same EduPage username/password
// (+ optional 2FA) form the browser dashboard already uses; a successful
// login is bound to an opaque access token instead of (or alongside) the
// session cookie.
package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// oauthCodeTTL and oauthTokenTTL bound how long an authorization code and an
// issued access token remain valid. Tokens match the session idle timeout
// since they are only useful as long as the underlying session (and its live
// edupage.Client) still exists.
const (
	oauthCodeTTL  = 5 * time.Minute
	oauthTokenTTL = sessionIdleTimeout
)

// oauthClient is a dynamically registered MCP client (RFC 7591). Only public
// clients are supported: authentication at the token endpoint relies on PKCE
// (RFC 7636), not a client secret.
type oauthClient struct {
	ID           string
	RedirectURIs []string
	Name         string
}

// oauthAuthRequest is a pending GET /oauth/authorize request, stashed while
// the user completes the EduPage login form (and 2FA, if required).
type oauthAuthRequest struct {
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	CreatedAt           time.Time
}

// oauthCode is an issued, single-use authorization code.
type oauthCode struct {
	ClientID            string
	RedirectURI         string
	SessionID           string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
}

// oauthToken is an issued access token, bound to a session id.
type oauthToken struct {
	SessionID string
	ExpiresAt time.Time
}

// oauthStore holds all in-memory OAuth state. Like Store, it is lost on
// restart, matching the app's existing "no persistent storage" design.
type oauthStore struct {
	mu      sync.Mutex
	clients map[string]*oauthClient
	pending map[string]*oauthAuthRequest
	codes   map[string]*oauthCode
	tokens  map[string]*oauthToken
	now     func() time.Time
}

func newOAuthStore() *oauthStore {
	return &oauthStore{
		clients: make(map[string]*oauthClient),
		pending: make(map[string]*oauthAuthRequest),
		codes:   make(map[string]*oauthCode),
		tokens:  make(map[string]*oauthToken),
		now:     time.Now,
	}
}

// randomToken returns a random, URL-safe identifier with byteLen bytes of
// entropy before encoding.
func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("web: generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (st *oauthStore) registerClient(redirectURIs []string, name string) (*oauthClient, error) {
	id, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	c := &oauthClient{ID: id, RedirectURIs: redirectURIs, Name: name}

	st.mu.Lock()
	st.clients[id] = c
	st.mu.Unlock()

	return c, nil
}

func (st *oauthStore) getClient(id string) (*oauthClient, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.clients[id]
	return c, ok
}

// stashAuthRequest stores a pending authorize request and returns its id.
func (st *oauthStore) stashAuthRequest(req *oauthAuthRequest) (string, error) {
	id, err := randomToken(24)
	if err != nil {
		return "", err
	}
	st.mu.Lock()
	st.pending[id] = req
	st.mu.Unlock()
	return id, nil
}

// takeAuthRequest looks up and removes a pending authorize request. It
// returns false if the id is unknown or has expired.
func (st *oauthStore) takeAuthRequest(id string) (*oauthAuthRequest, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	req, ok := st.pending[id]
	if !ok {
		return nil, false
	}
	delete(st.pending, id)
	if st.now().Sub(req.CreatedAt) > oauthCodeTTL {
		return nil, false
	}
	return req, true
}

// issueCode mints a one-time authorization code bound to sessionID.
func (st *oauthStore) issueCode(req *oauthAuthRequest, sessionID string) (string, error) {
	code, err := randomToken(24)
	if err != nil {
		return "", err
	}
	st.mu.Lock()
	st.codes[code] = &oauthCode{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		SessionID:           sessionID,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		ExpiresAt:           st.now().Add(oauthCodeTTL),
	}
	st.mu.Unlock()
	return code, nil
}

// redeemCode looks up and deletes a code (single use), returning it if valid
// and unexpired.
func (st *oauthStore) redeemCode(code string) (*oauthCode, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.codes[code]
	if !ok {
		return nil, false
	}
	delete(st.codes, code)
	if st.now().After(c.ExpiresAt) {
		return nil, false
	}
	return c, true
}

// issueToken mints an access token bound to sessionID.
func (st *oauthStore) issueToken(sessionID string) (string, time.Time, error) {
	tok, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := st.now().Add(oauthTokenTTL)
	st.mu.Lock()
	st.tokens[tok] = &oauthToken{SessionID: sessionID, ExpiresAt: expiresAt}
	st.mu.Unlock()
	return tok, expiresAt, nil
}

// sessionIDForToken resolves a bearer token to a session id, if valid and
// unexpired.
func (st *oauthStore) sessionIDForToken(token string) (string, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	t, ok := st.tokens[token]
	if !ok {
		return "", false
	}
	if st.now().After(t.ExpiresAt) {
		delete(st.tokens, token)
		return "", false
	}
	return t.SessionID, true
}

// verifyPKCE checks a code_verifier against the stored code_challenge.
// Only S256 is accepted; "plain" is rejected, matching the MCP spec's
// requirement that public clients use S256.
func verifyPKCE(method, challenge, verifier string) bool {
	if method != "S256" || challenge == "" || verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// externalBaseURL reconstructs the scheme+host the caller reached us on, so
// the OAuth metadata documents can advertise absolute URLs without needing a
// configured public hostname. It trusts X-Forwarded-Proto/Host the same way
// isRequestSecure does, for deployments behind a reverse proxy.
func externalBaseURL(r *http.Request) string {
	scheme := "http"
	if isRequestSecure(r) {
		scheme = "https"
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	return scheme + "://" + host
}

// --- RFC 8414 / RFC 9728 metadata ---

func (s *Server) handleOAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	base := externalBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"edupage"},
	})
}

func (s *Server) handleOAuthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	base := externalBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 base + "/mcp",
		"authorization_servers":    []string{base},
		"bearer_methods_supported": []string{"header"},
	})
}

// --- RFC 7591 dynamic client registration ---

type registerRequest struct {
	RedirectURIs []string `json:"redirect_uris"`
	ClientName   string   `json:"client_name"`
}

func (s *Server) handleOAuthRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client_metadata"})
		return
	}
	if len(req.RedirectURIs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri", "error_description": "redirect_uris is required"})
		return
	}
	for _, u := range req.RedirectURIs {
		if _, err := url.Parse(u); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect_uri"})
			return
		}
	}

	client, err := s.oauth.registerClient(req.RedirectURIs, req.ClientName)
	if err != nil {
		s.logger.Error("oauth: register client failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ID,
		"redirect_uris":              client.RedirectURIs,
		"client_name":                client.Name,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
	})
}

// --- authorize ---

// handleOAuthAuthorize serves GET /oauth/authorize: it validates the client
// and redirect_uri, then either bounces the browser to the login page (via
// its "oauth" continuation param) or, if the caller already has an
// authenticated session cookie, completes the redirect immediately.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")

	client, ok := s.oauth.getClient(clientID)
	if !ok {
		http.Error(w, "unknown client_id", http.StatusBadRequest)
		return
	}
	validRedirect := false
	for _, u := range client.RedirectURIs {
		if u == redirectURI {
			validRedirect = true
			break
		}
	}
	if !validRedirect {
		http.Error(w, "redirect_uri does not match a registered value", http.StatusBadRequest)
		return
	}

	if q.Get("response_type") != "code" {
		redirectOAuthError(w, r, redirectURI, q.Get("state"), "unsupported_response_type", "")
		return
	}
	challenge := q.Get("code_challenge")
	method := q.Get("code_challenge_method")
	if challenge == "" || method != "S256" {
		redirectOAuthError(w, r, redirectURI, q.Get("state"), "invalid_request", "PKCE with S256 is required")
		return
	}

	req := &oauthAuthRequest{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		State:               q.Get("state"),
		CodeChallenge:       challenge,
		CodeChallengeMethod: method,
		CreatedAt:           s.now(),
	}

	if sess, ok := s.sessionFromRequest(r); ok && sess.Authenticated() {
		s.finishOAuthAuthorize(w, r, req, sess)
		return
	}

	reqID, err := s.oauth.stashAuthRequest(req)
	if err != nil {
		s.logger.Error("oauth: stash auth request failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/?oauth="+url.QueryEscape(reqID), http.StatusSeeOther)
}

// finishOAuthAuthorize mints a code for an already-authenticated session and
// redirects back to the client's redirect_uri.
func (s *Server) finishOAuthAuthorize(w http.ResponseWriter, r *http.Request, req *oauthAuthRequest, sess *Session) {
	code, err := s.oauth.issueCode(req, sess.ID)
	if err != nil {
		s.logger.Error("oauth: issue code failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	dest, err := url.Parse(req.RedirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	qs := dest.Query()
	qs.Set("code", code)
	if req.State != "" {
		qs.Set("state", req.State)
	}
	dest.RawQuery = qs.Encode()
	http.Redirect(w, r, dest.String(), http.StatusSeeOther)
}

func redirectOAuthError(w http.ResponseWriter, r *http.Request, redirectURI, state, errCode, desc string) {
	dest, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, errCode, http.StatusBadRequest)
		return
	}
	qs := dest.Query()
	qs.Set("error", errCode)
	if desc != "" {
		qs.Set("error_description", desc)
	}
	if state != "" {
		qs.Set("state", state)
	}
	dest.RawQuery = qs.Encode()
	http.Redirect(w, r, dest.String(), http.StatusSeeOther)
}

// --- token ---

func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	if r.FormValue("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}

	code, ok := s.oauth.redeemCode(r.FormValue("code"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	if code.ClientID != r.FormValue("client_id") || code.RedirectURI != r.FormValue("redirect_uri") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	if !verifyPKCE(code.CodeChallengeMethod, code.CodeChallenge, r.FormValue("code_verifier")) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "PKCE verification failed"})
		return
	}

	if _, ok := s.sessions.Get(code.SessionID); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "session no longer exists"})
		return
	}

	token, expiresAt, err := s.oauth.issueToken(code.SessionID)
	if err != nil {
		s.logger.Error("oauth: issue token failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(time.Until(expiresAt).Seconds()),
		"scope":        "edupage",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
