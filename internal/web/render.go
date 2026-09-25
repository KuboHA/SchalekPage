package web

import (
	"bytes"
	"net/http"
	"time"
)

// render executes the named template into a buffer first (so a mid-render
// template error never leaves a half-written response), then writes it out.
// On failure it logs the error and serves a plain 500 instead of panicking
// or letting the handler crash the process.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	s.renderStatus(w, r, name, data, http.StatusOK)
}

// renderStatus is render with an explicit status code, for responses (like a
// rate-limited login) that need something other than 200 OK. The status must
// be set via WriteHeader after headers but before the body, so it can't just
// be a WriteHeader call bolted on by the caller.
func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, name string, data any, status int) {
	var buf bytes.Buffer
	renderStart := time.Now()
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		s.logger.Error("template render failed", "template", name, "path", r.URL.Path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	setServerTiming(w, r, time.Since(renderStart))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
