package web

import (
	"errors"
	"net/http"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// loginPageData is the view model for login.html.
type loginPageData struct {
	Error string

	// OAuthRequestID carries a pending MCP OAuth authorize request (see
	// oauth.go) through the login form as a hidden field, so a successful
	// login can complete the redirect back to the MCP client.
	OAuthRequestID string
}

// handleIndex serves the login page (GET /), matching app.py's index(). It
// also carries through the "oauth" query param, set by
// handleOAuthAuthorize when an MCP client's authorization request needs the
// user to sign in first.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", loginPageData{OAuthRequestID: r.URL.Query().Get("oauth")})
}

// handleLogin authenticates against EduPage (POST /login). On success it
// creates a server-side session holding the live *edupage.Client — the
// password itself is used only here and never stored, unlike app.py, which
// kept it in the (client-side, signed-but-readable) Flask session cookie
// and re-logged in on every single request.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.loginRate.Allow(clientIP(r)) {
		s.renderStatus(w, r, "login.html", loginPageData{Error: "Too many login attempts. Please wait a few minutes and try again."}, http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, r, "login.html", loginPageData{Error: "Invalid form submission."})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	subdomain := r.FormValue("subdomain")
	oauthRequestID := r.FormValue("oauth")

	client := edupage.New(clientTimeout)
	tf, err := client.Login(username, password, subdomain)
	if err != nil {
		s.render(w, r, "login.html", loginPageData{Error: describeLoginError(err), OAuthRequestID: oauthRequestID})
		return
	}

	sess, err := s.sessions.Create()
	if err != nil {
		s.logger.Error("create session failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sess.Client = client
	sess.Username = username
	sess.Subdomain = subdomain
	sess.StudentName = username // fallback, overridden below on a match
	sess.OAuthRequestID = oauthRequestID

	if students, err := client.Students(); err != nil {
		s.logger.Warn("fetch students at login failed", "error", err)
	} else if student, ok := matchStudentByUsername(students, username); ok {
		sess.StudentName = student.Name
		sess.StudentID = student.PersonID
	}

	setSessionCookie(w, r, sess.ID)

	if tf != nil {
		sess.Pending = tf
		http.Redirect(w, r, "/two_factor", http.StatusSeeOther)
		return
	}
	s.finishLoginRedirect(w, r, sess)
}

// finishLoginRedirect sends the browser wherever it needs to go once a
// session has finished authenticating (2FA included, if any): back to the
// pending MCP OAuth client if this login started there, otherwise to the
// dashboard as usual.
func (s *Server) finishLoginRedirect(w http.ResponseWriter, r *http.Request, sess *Session) {
	if sess.OAuthRequestID != "" {
		reqID := sess.OAuthRequestID
		sess.OAuthRequestID = ""
		if req, ok := s.oauth.takeAuthRequest(reqID); ok {
			s.finishOAuthAuthorize(w, r, req, sess)
			return
		}
		// Pending request expired or was already used; fall through to the
		// normal dashboard redirect rather than stranding the user.
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// describeLoginError renders a login failure the way the login page can
// show it, distinguishing the sentinel errors edupage exposes.
func describeLoginError(err error) string {
	switch {
	case errors.Is(err, edupage.ErrBadCredentials):
		return "Incorrect username or password."
	case errors.Is(err, edupage.ErrCaptcha):
		return "EduPage is asking for a captcha; please try again later."
	case errors.Is(err, edupage.ErrSecondFactorFailed):
		return "Two-factor authentication failed."
	case errors.Is(err, edupage.ErrMissingData):
		return "EduPage returned an unexpected response. Please try again."
	default:
		return err.Error()
	}
}

// handleLogout signs the caller out (POST /logout): it destroys the
// server-side session — so the live *edupage.Client and any pending 2FA
// state are dropped — then clears the session cookie and sends the browser
// back to the login page. Routed as POST-only (never a GET link) so an
// embedded image or prefetch can't trigger it.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, sess *Session) {
	s.sessions.Delete(sess.ID)
	clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// twoFactorPageData is the view model for two_factor.html.
type twoFactorPageData struct {
	Error string
}

// handleTwoFactorGet serves the code-entry page for a login awaiting a
// second factor (GET /two_factor).
func (s *Server) handleTwoFactorGet(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(r)
	if !ok || sess.Pending == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, r, "two_factor.html", twoFactorPageData{})
}

// handleTwoFactorPost completes a pending second-factor login (POST
// /two_factor), porting app.py's two_factor() POST branch — but, unlike it,
// without needing to re-authenticate with the password again.
func (s *Server) handleTwoFactorPost(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(r)
	if !ok || sess.Pending == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, r, "two_factor.html", twoFactorPageData{Error: "Invalid form submission."})
		return
	}
	code := r.FormValue("code")
	if err := sess.Pending.FinishWithCode(code); err != nil {
		s.render(w, r, "two_factor.html", twoFactorPageData{Error: describeLoginError(err)})
		return
	}
	sess.Pending = nil

	if students, err := sess.Client.Students(); err != nil {
		s.logger.Warn("fetch students after 2FA failed", "error", err)
	} else if student, ok := matchStudentByUsername(students, sess.Username); ok {
		sess.StudentName = student.Name
		sess.StudentID = student.PersonID
	}

	s.finishLoginRedirect(w, r, sess)
}
