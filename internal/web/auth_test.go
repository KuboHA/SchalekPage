package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

func newTestServer() *Server {
	return &Server{
		sessions: NewStore(time.Hour),
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:      time.Now,
	}
}

func TestRequireAuthRedirectsWithoutCookie(t *testing.T) {
	s := newTestServer()
	called := false
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request, sess *Session) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if called {
		t.Fatal("handler should not run without a session")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected %d, got %d", http.StatusSeeOther, rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to /, got %q", loc)
	}
}

func TestRequireAuthRedirectsForPendingSession(t *testing.T) {
	s := newTestServer()
	sess, err := s.sessions.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Client is left nil, so Authenticated() is false regardless of
	// Pending — this covers the "awaiting 2FA" state.

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: sess.ID})
	rec := httptest.NewRecorder()

	called := false
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request, sess *Session) {
		called = true
	})
	h(rec, req)

	if called {
		t.Fatal("handler should not run for an unauthenticated (pending) session")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected %d, got %d", http.StatusSeeOther, rec.Code)
	}
}

func TestRequireAuthPassesThroughAuthenticatedSession(t *testing.T) {
	s := newTestServer()
	sess, err := s.sessions.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// A non-nil Client flips Authenticated() to true; edupage.New performs
	// no network I/O, so it's safe to use directly in a test.
	sess.Pending = nil
	sess.Client = edupage.New(time.Second)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: sess.ID})
	rec := httptest.NewRecorder()

	called := false
	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request, got *Session) {
		called = true
		if got != sess {
			t.Error("expected the same session to be passed through")
		}
	})
	h(rec, req)

	if !called {
		t.Fatal("expected handler to run for an authenticated session")
	}
}

func TestRequireAuthUnknownCookie(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "does-not-exist"})
	rec := httptest.NewRecorder()

	h := s.requireAuth(func(w http.ResponseWriter, r *http.Request, sess *Session) {
		t.Fatal("handler should not run for an unknown session id")
	})
	h(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected %d, got %d", http.StatusSeeOther, rec.Code)
	}
}
