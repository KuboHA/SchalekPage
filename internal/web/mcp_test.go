package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// newAuthenticatedSession creates a session that looks authenticated
// (Client != nil, Pending == nil) without talking to real EduPage.
func newAuthenticatedSession(t *testing.T, s *Server) *Session {
	t.Helper()
	sess, err := s.sessions.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess.Client = edupage.New(0)
	sess.Username = "teststudent"
	sess.Subdomain = "testschool"
	return sess
}

func TestMCPRequiresBearerToken(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	rec := httptest.NewRecorder()

	s.handleMCP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if www := rec.Header().Get("WWW-Authenticate"); !strings.Contains(www, "oauth-protected-resource") {
		t.Fatalf("expected WWW-Authenticate to point at protected-resource metadata, got %q", www)
	}
}

// oauthFlow drives dynamic client registration, /oauth/authorize (against an
// already-authenticated session, so no login form is involved) and the
// token exchange, returning the minted access token.
func oauthFlow(t *testing.T, s *Server, sess *Session) string {
	t.Helper()

	regBody := `{"redirect_uris":["https://client.example/callback"],"client_name":"test client"}`
	regReq := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(regBody))
	regRec := httptest.NewRecorder()
	s.handleOAuthRegister(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", regRec.Code, regRec.Body.String())
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(regRec.Body.Bytes(), &reg); err != nil {
		t.Fatalf("decode register response: %v", err)
	}

	verifier := "test-code-verifier-0123456789-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authURL := "/oauth/authorize?" + url.Values{
		"client_id":             {reg.ClientID},
		"redirect_uri":          {"https://client.example/callback"},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
	}.Encode()

	authReq := httptest.NewRequest(http.MethodGet, authURL, nil)
	authReq.AddCookie(&http.Cookie{Name: CookieName, Value: sess.ID})
	authRec := httptest.NewRecorder()
	s.handleOAuthAuthorize(authRec, authReq)

	if authRec.Code != http.StatusSeeOther {
		t.Fatalf("authorize: expected redirect, got %d: %s", authRec.Code, authRec.Body.String())
	}
	loc, err := url.Parse(authRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect location: %v", err)
	}
	if got := loc.Query().Get("state"); got != "xyz" {
		t.Fatalf("expected state to round-trip, got %q", got)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("expected an authorization code in redirect, got %q", loc)
	}

	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {reg.ClientID},
		"redirect_uri":  {"https://client.example/callback"},
		"code_verifier": {verifier},
	}
	tokenReq := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRec := httptest.NewRecorder()
	s.handleOAuthToken(tokenRec, tokenReq)

	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token: expected 200, got %d: %s", tokenRec.Code, tokenRec.Body.String())
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokenResp); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if tokenResp.AccessToken == "" {
		t.Fatalf("expected a non-empty access token")
	}
	return tokenResp.AccessToken
}

func TestOAuthFlowThenMCPToolsList(t *testing.T) {
	s := newTestServer()
	sess := newAuthenticatedSession(t, s)
	token := oauthFlow(t, s, sess)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	s.handleMCP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp jsonrpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be an object, got %T", resp.Result)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("expected a non-empty tools list, got %v", result["tools"])
	}
}

func TestOAuthCodeIsSingleUse(t *testing.T) {
	s := newTestServer()
	sess := newAuthenticatedSession(t, s)

	verifier := "test-code-verifier-0123456789-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	client, err := s.oauth.registerClient([]string{"https://client.example/callback"}, "test")
	if err != nil {
		t.Fatalf("registerClient: %v", err)
	}
	req := &oauthAuthRequest{
		ClientID:            client.ID,
		RedirectURI:         "https://client.example/callback",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	}
	code, err := s.oauth.issueCode(req, sess.ID)
	if err != nil {
		t.Fatalf("issueCode: %v", err)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {client.ID},
		"redirect_uri":  {"https://client.example/callback"},
		"code_verifier": {verifier},
	}.Encode()

	firstReq := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form))
	firstReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	first := httptest.NewRecorder()
	s.handleOAuthToken(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first redemption: expected 200, got %d: %s", first.Code, first.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form))
	secondReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	second := httptest.NewRecorder()
	s.handleOAuthToken(second, secondReq)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second redemption: expected 400 (code already used), got %d: %s", second.Code, second.Body.String())
	}
}
