package web

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// swScriptPath is where the service worker source lives inside the embedded
// static filesystem. It is served at /sw.js (not /static/sw.js) because a
// service worker's default control scope is the directory it's served
// from — serving it from /static/ would limit it to /static/* and it would
// never see navigations to /dashboard, /grades, and so on.
const swScriptPath = "static/sw.js"

// clientTimeout bounds every request the edupage.Client makes to EduPage.
const clientTimeout = 20 * time.Second

// sessionIdleTimeout is how long a session may sit idle before the reaper
// evicts it (and its live edupage.Client) from memory.
const sessionIdleTimeout = 12 * time.Hour

// Server holds everything an HTTP handler needs: the parsed template set,
// the server-side session store, and a logger. Handlers are methods on
// *Server so they share this state without globals.
type Server struct {
	templates *template.Template
	sessions  *Store
	loginRate *rateLimiter
	oauth     *oauthStore
	logger    *slog.Logger

	// now is the injectable clock used for the 14:30 dashboard rollover and
	// date-fallback logic, so tests don't depend on wall-clock time.
	now func() time.Time
}

// NewServer builds a Server from an already-parsed template set and a
// logger. A session store with the default idle timeout is created
// internally; use it via Sessions() if the caller needs to start the
// reaper.
func NewServer(templates *template.Template, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		templates: templates,
		sessions:  NewStore(sessionIdleTimeout),
		loginRate: newRateLimiter(loginRateLimit, loginRateWindow),
		oauth:     newOAuthStore(),
		logger:    logger,
		now:       time.Now,
	}
}

// handleServiceWorker serves the service worker script from the site root
// so its default scope covers the whole app, not just /static/.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(staticFS, swScriptPath)
	if err != nil {
		s.logger.Error("service worker script not embedded correctly", "error", err)
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Write(data)
}

// Sessions returns the server's session store, e.g. so main() can start its
// background reaper.
func (s *Server) Sessions() *Store { return s.sessions }

// LoginRateLimiter returns the server's /login rate limiter, e.g. so main()
// can start its background reaper.
func (s *Server) LoginRateLimiter() *rateLimiter { return s.loginRate }

// Routes builds the top-level handler: the app's routes plus /static/ file
// serving from the embedded assets.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /two_factor", s.handleTwoFactorGet)
	mux.HandleFunc("POST /two_factor", s.handleTwoFactorPost)
	mux.HandleFunc("GET /dashboard", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("GET /timetable/", s.requireAuth(s.handleTimetable))
	mux.HandleFunc("GET /timetable/{date}", s.requireAuth(s.handleTimetable))
	mux.HandleFunc("GET /lunches/", s.requireAuth(s.handleLunches))
	mux.HandleFunc("GET /lunches/{date}", s.requireAuth(s.handleLunches))
	mux.HandleFunc("GET /grades", s.requireAuth(s.handleGrades))
	mux.HandleFunc("GET /substitutions/", s.requireAuth(s.handleSubstitutions))
	mux.HandleFunc("GET /substitutions/{date}", s.requireAuth(s.handleSubstitutions))

	// Feature 1: logout is POST-only — a GET logout would be triggerable by
	// any embedded image pointing at it.
	mux.HandleFunc("POST /logout", s.requireAuth(s.handleLogout))

	// Features 4/5/6: timeline-derived pages. /notifications/history renders
	// the shared notification-card fragment for the dashboard's "load older".
	mux.HandleFunc("GET /notifications/history", s.requireAuth(s.handleNotificationHistory))
	mux.HandleFunc("GET /homework", s.requireAuth(s.handleHomework))
	mux.HandleFunc("GET /exams", s.requireAuth(s.handleExams))

	// Feature 14: canteen writes. POST-only, and the handlers redirect back
	// to the lunches page so a refresh cannot re-submit an order.
	mux.HandleFunc("POST /lunches/order", s.requireAuth(s.handleLunchOrder))
	mux.HandleFunc("POST /lunches/signoff", s.requireAuth(s.handleLunchSignOff))

	// Features 16/17: messaging and attachment upload, both writes.
	mux.HandleFunc("GET /messages", s.requireAuth(s.handleMessagesGet))
	mux.HandleFunc("POST /messages/send", s.requireAuth(s.handleMessagesSend))
	mux.HandleFunc("POST /messages/upload", s.requireAuth(s.handleMessagesUpload))

	mux.HandleFunc("GET /sw.js", s.handleServiceWorker)

	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleOAuthServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleOAuthProtectedResourceMetadata)
	mux.HandleFunc("POST /oauth/register", s.handleOAuthRegister)
	mux.HandleFunc("GET /oauth/authorize", s.handleOAuthAuthorize)
	mux.HandleFunc("POST /oauth/token", s.handleOAuthToken)
	mux.HandleFunc("POST /mcp", s.handleMCP)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The embedded FS is baked in at build time; this can only fail if
		// the build itself is broken.
		s.logger.Error("static assets not embedded correctly", "error", err)
	} else {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))
	}

	return withRequestTiming(mux)
}
