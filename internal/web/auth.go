package web

import (
	"net/http"
	"strings"
	"time"
)

// isRequestSecure reports whether the request reached us over HTTPS, either
// directly (r.TLS) or as reported by a terminating reverse proxy via the
// conventional X-Forwarded-Proto header. Without this, a deployment behind
// nginx/Caddy would always see r.TLS == nil and never set the Secure cookie
// flag, even though the browser is talking to it over HTTPS.
func isRequestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// setSessionCookie writes the HttpOnly, SameSite=Lax session cookie.
func setSessionCookie(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isRequestSecure(r),
		MaxAge:   int(sessionIdleTimeout / time.Second),
	})
}

// clearSessionCookie expires the session cookie client-side.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// sessionFromRequest looks up the caller's session via its cookie, if any.
func (s *Server) sessionFromRequest(r *http.Request) (*Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	return s.sessions.Get(c.Value)
}

// requireAuth wraps a handler so that requests without a fully
// authenticated session are redirected to the login page ("/"), matching
// app.py's `if 'subdomain' not in session ...: return redirect(url_for('index'))`
// guard repeated at the top of every protected route.
func (s *Server) requireAuth(h func(w http.ResponseWriter, r *http.Request, sess *Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.sessionFromRequest(r)
		if !ok || !sess.Authenticated() {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		h(w, r, sess)
	}
}
