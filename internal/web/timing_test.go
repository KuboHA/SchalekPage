package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerTimingHeader(t *testing.T) {
	h := withRequestTiming(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setServerTiming(w, r, 2*time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get("Server-Timing")
	if !strings.Contains(got, "total;dur=") || !strings.Contains(got, "render;dur=") {
		t.Fatalf("Server-Timing = %q, want both total and render metrics", got)
	}
}

// TestServerTimingWithoutWrapper ensures setServerTiming stays a no-op when a
// handler is called without the timing middleware (e.g. directly by a test),
// rather than panicking or emitting a bogus zero-duration header.
func TestServerTimingWithoutWrapper(t *testing.T) {
	rec := httptest.NewRecorder()
	setServerTiming(rec, httptest.NewRequest(http.MethodGet, "/", nil), time.Millisecond)

	if got := rec.Header().Get("Server-Timing"); got != "" {
		t.Errorf("Server-Timing = %q, want empty without the timing middleware", got)
	}
}
