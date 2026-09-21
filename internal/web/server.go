package web

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

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
		logger:    logger,
		now:       time.Now,
	}
}

// Sessions returns the server's session store, e.g. so main() can start its
// background reaper.
func (s *Server) Sessions() *Store { return s.sessions }

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

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The embedded FS is baked in at build time; this can only fail if
		// the build itself is broken.
		s.logger.Error("static assets not embedded correctly", "error", err)
	} else {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))
	}

	return mux
}
