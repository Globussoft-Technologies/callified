package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRateLimiterBlocksAfterFiveFailures(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, time.July, 24, 16, 0, 0, 0, time.UTC)

	for i := 0; i < loginRateLimitMaxFailures-1; i++ {
		if retryAfter := limiter.registerFailure("203.0.113.9", "agent@example.com", now); retryAfter != 0 {
			t.Fatalf("unexpected retryAfter before limit: %v", retryAfter)
		}
	}

	retryAfter := limiter.registerFailure("203.0.113.9", "agent@example.com", now)
	if retryAfter < 29*time.Second || retryAfter > 30*time.Second {
		t.Fatalf("expected ~30s retryAfter at limit, got %v", retryAfter)
	}

	wait, blocked := limiter.check("203.0.113.9", "agent@example.com", now.Add(5*time.Second))
	if !blocked {
		t.Fatalf("expected limiter to block after five failures")
	}
	if wait < 24*time.Second || wait > 25*time.Second {
		t.Fatalf("expected ~25s remaining wait, got %v", wait)
	}
}

func TestLoginRateLimiterDoesNotBlockBeforeThreshold(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, time.July, 24, 16, 2, 0, 0, time.UTC)

	limiter.registerFailure("203.0.113.12", "agent@example.com", now)

	if wait, blocked := limiter.check("203.0.113.12", "agent@example.com", now.Add(time.Second)); blocked || wait != 0 {
		t.Fatalf("expected no block before threshold, blocked=%v wait=%v", blocked, wait)
	}
}

func TestLoginRateLimiterResetOnSuccess(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, time.July, 24, 16, 5, 0, 0, time.UTC)

	for i := 0; i < loginRateLimitMaxFailures; i++ {
		limiter.registerFailure("203.0.113.10", "user@example.com", now)
	}

	limiter.reset("203.0.113.10", "user@example.com")
	if wait, blocked := limiter.check("203.0.113.10", "user@example.com", now.Add(time.Second)); blocked || wait != 0 {
		t.Fatalf("expected limiter state to clear after reset, blocked=%v wait=%v", blocked, wait)
	}
}

func TestLoginRateLimiterUnlocksAfterWindow(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, time.July, 24, 16, 10, 0, 0, time.UTC)

	for i := 0; i < loginRateLimitMaxFailures; i++ {
		limiter.registerFailure("203.0.113.11", "unlock@example.com", now)
	}

	if wait, blocked := limiter.check("203.0.113.11", "unlock@example.com", now.Add(10*time.Second)); !blocked || wait <= 0 {
		t.Fatalf("expected limiter to remain blocked before window ends, blocked=%v wait=%v", blocked, wait)
	}

	if wait, blocked := limiter.check("203.0.113.11", "unlock@example.com", now.Add(31*time.Second)); blocked || wait != 0 {
		t.Fatalf("expected limiter to unlock after window, blocked=%v wait=%v", blocked, wait)
	}
}

func TestLoginClientIPPrefersForwardedHeaders(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "10.0.0.8:4321"
	r.Header.Set("X-Forwarded-For", "198.51.100.12, 10.0.0.8")
	r.Header.Set("X-Real-IP", "192.0.2.44")

	if got := loginClientIP(r); got != "198.51.100.12" {
		t.Fatalf("expected forwarded IP, got %q", got)
	}
}
