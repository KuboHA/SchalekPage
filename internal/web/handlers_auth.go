package web

import (
	"errors"
	"net/http"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// loginPageData is the view model for login.html.
type loginPageData struct {
	Error string
}

// handleIndex serves the login page (GET /), matching app.py's index().
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", loginPageData{})
}

// handleLogin authenticates against EduPage (POST /login). On success it
// creates a server-side session holding the live *edupage.Client — the
// password itself is used only here and never stored, unlike app.py, which
// kept it in the (client-side, signed-but-readable) Flask session cookie
// and re-logged in on every single request.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, r, "login.html", loginPageData{Error: "Invalid form submission."})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	subdomain := r.FormValue("subdomain")

	client := edupage.New(clientTimeout)
	tf, err := client.Login(username, password, subdomain)
	if err != nil {
		s.render(w, r, "login.html", loginPageData{Error: describeLoginError(err)})
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

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
