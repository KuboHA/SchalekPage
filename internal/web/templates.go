package web

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// StaticFS returns the embedded static asset filesystem, rooted so that
// "static/css/..." paths resolve, ready to be served under /static/.
func StaticFS() embed.FS { return staticFS }

// funcMap returns the html/template functions available to every page:
// date/time formatting (Go's html/template has no strftime), the
// EduPage event-type icon lookup, and small string helpers that replace
// Jinja filters used in the original templates.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"dateYMD":      func(t time.Time) string { return t.Format(dateLayout) },
		"dateLong":     func(t time.Time) string { return t.Format("Monday, 02 January 2006") },
		"dateTime":     func(t time.Time) string { return t.Format("15:04") },
		"dateShortDay": func(t time.Time) string { return t.Format("Mon") },
		"dateDay":      func(t time.Time) string { return t.Format("02") },
		"dateMon":      func(t time.Time) string { return t.Format("Jan") },
		"dateDMY":      func(t time.Time) string { return t.Format("02 Jan 2006") },
		"eventTypeIcon": func(eventType string) string {
			return edupage.EventTypeIcon(strings.ToLower(eventType))
		},
		"eventTypeName": func(eventType string) string {
			return edupage.EventTypeName(strings.ToLower(eventType))
		},
		"menuOrdered": menuOrdered,
		"add":         func(a, b int) int { return a + b },
		"sub":         func(a, b int) int { return a - b },
	}
}

// ParseTemplates parses every internal/web/templates/*.html file once,
// registering funcMap for use in all of them.
func ParseTemplates() (*template.Template, error) {
	t, err := template.New("web").Funcs(funcMap()).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: parse templates: %w", err)
	}
	return t, nil
}
