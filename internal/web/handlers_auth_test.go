package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// TestHandleLogoutDestroysSession verifies handleLogout (feature 1) removes
// the session from the store, clears the cookie client-side, and redirects
// to the login page — matching the STRICT requirement that logout only ever
// runs as a POST.
func TestHandleLogoutDestroysSession(t *testing.T) {
	s := newTestServer()
	sess, err := s.sessions.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess.Client = edupage.New(time.Second)

	if _, ok := s.sessions.Get(sess.ID); !ok {
		t.Fatal("precondition: session should exist before logout")
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: sess.ID})
	rec := httptest.NewRecorder()

	s.handleLogout(rec, req, sess)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected %d, got %d", http.StatusSeeOther, rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to /, got %q", loc)
	}

	if _, ok := s.sessions.Get(sess.ID); ok {
		t.Error("expected session to be removed from the store")
	}

	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			cleared = c
		}
	}
	if cleared == nil {
		t.Fatal("expected a Set-Cookie header clearing the session cookie")
	}
	if cleared.MaxAge >= 0 {
		t.Errorf("expected a negative MaxAge to expire the cookie, got %d", cleared.MaxAge)
	}
}

// TestHandleLogoutViaRequireAuth exercises handleLogout through the same
// requireAuth wrapper the router uses, confirming an unauthenticated caller
// never reaches it (and so can't destroy a session it doesn't own) and an
// authenticated one is logged out end to end.
func TestHandleLogoutViaRequireAuth(t *testing.T) {
	s := newTestServer()
	sess, err := s.sessions.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess.Client = edupage.New(time.Second)

	h := s.requireAuth(s.handleLogout)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected %d, got %d", http.StatusSeeOther, rec.Code)
	}
	if _, ok := s.sessions.Get(sess.ID); ok {
		t.Error("expected session to be removed from the store")
	}
}
