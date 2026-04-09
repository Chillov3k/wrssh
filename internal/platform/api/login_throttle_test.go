package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginThrottleBlocksAfterRepeatedFailures(t *testing.T) {
	throttle := newLoginThrottle()
	now := time.Unix(1_700_000_000, 0)
	key := "alice@127.0.0.1"

	for i := 0; i < loginThrottleMaxFailures-1; i++ {
		if blockedFor := throttle.registerFailure(key, now); blockedFor != 0 {
			t.Fatalf("unexpected block before threshold: %v", blockedFor)
		}
	}

	blockedFor := throttle.registerFailure(key, now)
	if blockedFor <= 0 {
		t.Fatal("expected login attempts to become blocked")
	}

	if retryAfter, blocked := throttle.retryAfter(key, now.Add(time.Second)); !blocked || retryAfter <= 0 {
		t.Fatal("expected retryAfter to report an active block")
	}
}

func TestLoginThrottleResetClearsBlock(t *testing.T) {
	throttle := newLoginThrottle()
	now := time.Unix(1_700_000_000, 0)
	key := "alice@127.0.0.1"

	for i := 0; i < loginThrottleMaxFailures; i++ {
		throttle.registerFailure(key, now)
	}
	throttle.reset(key)

	if retryAfter, blocked := throttle.retryAfter(key, now.Add(time.Second)); blocked || retryAfter != 0 {
		t.Fatalf("expected reset to clear login throttle state, got blocked=%v retryAfter=%v", blocked, retryAfter)
	}
}

func TestLoginThrottleKeyUsesForwardedClientIP(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/auth/login", nil)
	request.RemoteAddr = "172.20.0.10:12345"
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 172.20.0.2")

	key := loginThrottleKey(request, "Alice")
	if key != "alice@203.0.113.10" {
		t.Fatalf("unexpected throttle key: %s", key)
	}
}
