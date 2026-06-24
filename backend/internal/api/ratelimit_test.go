package api

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsUpToLimitThenBlocks(t *testing.T) {
	now := time.Unix(0, 0)
	rl := newRateLimiter(3, time.Minute)
	rl.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := rl.allow("1.2.3.4"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, retry := rl.allow("1.2.3.4")
	if ok {
		t.Fatal("4th request should be blocked")
	}
	if retry <= 0 || retry > time.Minute {
		t.Errorf("expected a positive retry-after within the window, got %v", retry)
	}
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	now := time.Unix(0, 0)
	rl := newRateLimiter(1, time.Minute)
	rl.now = func() time.Time { return now }

	if ok, _ := rl.allow("ip"); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := rl.allow("ip"); ok {
		t.Fatal("second request in window should be blocked")
	}
	now = now.Add(time.Minute + time.Second) // window elapses
	if ok, _ := rl.allow("ip"); !ok {
		t.Fatal("request after window reset should be allowed")
	}
}

func TestRateLimiter_IsolatesClients(t *testing.T) {
	now := time.Unix(0, 0)
	rl := newRateLimiter(1, time.Minute)
	rl.now = func() time.Time { return now }

	if ok, _ := rl.allow("a"); !ok {
		t.Fatal("client a first request should be allowed")
	}
	if ok, _ := rl.allow("b"); !ok {
		t.Fatal("client b must not be throttled by client a's usage")
	}
}
