package web

import (
	"bytes"
	"net/http"
)

// render executes the named template into a buffer first (so a mid-render
// template error never leaves a half-written response), then writes it out.
// On failure it logs the error and serves a plain 500 instead of panicking
// or letting the handler crash the process.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		s.logger.Error("template render failed", "template", name, "path", r.URL.Path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
