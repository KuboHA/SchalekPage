package web

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimit(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("attempt %d should have been allowed", i+1)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("4th attempt within the window should have been denied")
	}
}

func TestRateLimiterTracksKeysIndependently(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("first attempt for 1.2.3.4 should be allowed")
	}
	if !rl.Allow("5.6.7.8") {
		t.Fatal("first attempt for a different key should be allowed")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("second attempt for 1.2.3.4 should be denied")
	}
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	now := time.Now()
	rl.now = func() time.Time { return now }

	if !rl.Allow("1.2.3.4") {
		t.Fatal("first attempt should be allowed")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("second attempt within the window should be denied")
	}

	now = now.Add(2 * time.Minute)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("attempt after the window elapsed should be allowed")
	}
}

func TestRateLimiterReap(t *testing.T) {
	rl := newRateLimiter(1, 50*time.Millisecond)
	rl.Allow("1.2.3.4")
	if len(rl.buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(rl.buckets))
	}

	ctx, cancel := context.WithCancel(context.Background())
	rl.StartReaper(ctx.Done(), 10*time.Millisecond)
	defer cancel()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rl.mu.Lock()
		n := len(rl.buckets)
		rl.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected reaper to evict the expired bucket")
}
