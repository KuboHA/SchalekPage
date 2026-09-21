package web

import (
	"context"
	"testing"
	"time"
)

func TestStoreCreateAndGet(t *testing.T) {
	st := NewStore(time.Hour)
	sess, err := st.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session id")
	}

	got, ok := st.Get(sess.ID)
	if !ok {
		t.Fatal("expected to find session")
	}
	if got != sess {
		t.Fatal("expected the same session pointer back")
	}
}

func TestStoreGetUnknown(t *testing.T) {
	st := NewStore(time.Hour)
	if _, ok := st.Get("nope"); ok {
		t.Fatal("expected unknown id to miss")
	}
}

func TestStoreExpiryOnGet(t *testing.T) {
	now := time.Now()
	st := NewStore(time.Minute)
	st.now = func() time.Time { return now }

	sess, err := st.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Still within the idle window.
	now = now.Add(30 * time.Second)
	if _, ok := st.Get(sess.ID); !ok {
		t.Fatal("expected session to still be valid")
	}

	// Advance past the idle window from the last access.
	now = now.Add(2 * time.Minute)
	if _, ok := st.Get(sess.ID); ok {
		t.Fatal("expected session to have expired")
	}
	if _, ok := st.Get(sess.ID); ok {
		t.Fatal("expected session to remain expired (and be evicted)")
	}
}

func TestStoreGetRefreshesLastSeen(t *testing.T) {
	now := time.Now()
	st := NewStore(time.Minute)
	st.now = func() time.Time { return now }

	sess, _ := st.Create()

	// Touch the session just before it would expire, repeatedly.
	for i := 0; i < 3; i++ {
		now = now.Add(45 * time.Second)
		if _, ok := st.Get(sess.ID); !ok {
			t.Fatalf("iteration %d: expected session to still be alive", i)
		}
	}
}

func TestStoreDelete(t *testing.T) {
	st := NewStore(time.Hour)
	sess, _ := st.Create()
	st.Delete(sess.ID)
	if _, ok := st.Get(sess.ID); ok {
		t.Fatal("expected session to be gone after Delete")
	}
}

func TestStoreReaper(t *testing.T) {
	now := time.Now()
	st := NewStore(50 * time.Millisecond)
	st.now = func() time.Time { return now }

	sess, _ := st.Create()
	now = now.Add(time.Second) // well past idle timeout

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.StartReaper(ctx, 10*time.Millisecond)

	deadline := time.After(2 * time.Second)
	for {
		if st.Len() == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("reaper never evicted expired session %s", sess.ID)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSessionAuthenticated(t *testing.T) {
	sess := &Session{}
	if sess.Authenticated() {
		t.Fatal("expected empty session to be unauthenticated")
	}
}
