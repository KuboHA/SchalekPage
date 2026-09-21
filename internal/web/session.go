// Package web implements the HTTP layer for SchalekPage: handlers, server-side
// sessions, and HTML rendering of the pages that used to be Jinja2 templates.
package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/KuboHA/SchalekPage/internal/edupage"
)

// CookieName is the name of the HttpOnly cookie that carries the session id.
const CookieName = "sp_session"

// Session is one logged-in (or logging-in) user's server-side state. Unlike
// the Python reference implementation, the password never lives here and a
// live *edupage.Client is kept in memory instead of being reconstructed (and
// re-authenticated) on every request.
type Session struct {
	ID string

	// Client is the live, logged-in EduPage client. Nil while a login is
	// still pending a second factor.
	Client *edupage.Client

	// Pending holds the two-factor handshake state between the initial
	// POST /login and the code/confirmation submitted to /two_factor. It is
	// nil once authentication is complete.
	Pending *edupage.TwoFactor

	// Username is the login username, used as a fallback match against the
	// student roster when StudentID is zero.
	Username string

	// Subdomain is the school subdomain the user authenticated against.
	Subdomain string

	// StudentName and StudentID identify the resolved student record, if
	// any was found at login time.
	StudentName string
	StudentID   int

	createdAt time.Time
	lastSeen  time.Time
}

// Authenticated reports whether the session has completed login (i.e. is not
// waiting on a second factor).
func (s *Session) Authenticated() bool {
	return s.Client != nil && s.Pending == nil
}

// Store is an in-memory, concurrency-safe session store keyed by session id.
// Sessions idle for longer than the configured idle timeout are evicted by
// the background reaper (see StartReaper) and on lookup.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	idle     time.Duration
	now      func() time.Time
}

// NewStore returns an empty session store that expires sessions idle for
// longer than idleTimeout.
func NewStore(idleTimeout time.Duration) *Store {
	return &Store{
		sessions: make(map[string]*Session),
		idle:     idleTimeout,
		now:      time.Now,
	}
}

// newSessionID generates a random, URL-safe 32-byte session identifier.
func newSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("web: generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Create allocates and stores a new, empty session.
func (st *Store) Create() (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	now := st.now()
	sess := &Session{ID: id, createdAt: now, lastSeen: now}

	st.mu.Lock()
	st.sessions[id] = sess
	st.mu.Unlock()

	return sess, nil
}

// Get returns the session for id, refreshing its idle timer. It returns
// false if the id is unknown or the session has expired.
func (st *Store) Get(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	sess, ok := st.sessions[id]
	if !ok {
		return nil, false
	}
	now := st.now()
	if st.idle > 0 && now.Sub(sess.lastSeen) > st.idle {
		delete(st.sessions, id)
		return nil, false
	}
	sess.lastSeen = now
	return sess, true
}

// Delete removes a session (used on logout or a failed second factor restart).
func (st *Store) Delete(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, id)
}

// Len reports the number of currently stored sessions (including ones that
// may be expired but not yet reaped). Mainly useful for tests and metrics.
func (st *Store) Len() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.sessions)
}

// reap deletes all sessions idle for longer than the store's idle timeout.
func (st *Store) reap() {
	if st.idle <= 0 {
		return
	}
	now := st.now()
	st.mu.Lock()
	defer st.mu.Unlock()
	for id, sess := range st.sessions {
		if now.Sub(sess.lastSeen) > st.idle {
			delete(st.sessions, id)
		}
	}
}

// StartReaper runs a background goroutine that periodically evicts expired
// sessions until ctx is cancelled.
func (st *Store) StartReaper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st.reap()
			}
		}
	}()
}
