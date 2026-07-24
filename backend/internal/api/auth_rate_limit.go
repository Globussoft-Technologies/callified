package api

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	loginRateLimitMaxFailures = 5
	loginRateLimitWindow      = 30 * time.Second
	loginRateLimitEntryTTL    = 10 * time.Minute
)

type loginRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginRateLimitEntry
}

type loginRateLimitEntry struct {
	failures     int
	blockedUntil time.Time
	lastSeen     time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		entries: make(map[string]*loginRateLimitEntry),
	}
}

func (l *loginRateLimiter) check(ip, email string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	remaining := time.Duration(0)
	blocked := false
	for _, key := range []string{loginRateLimitIPKey(ip), loginRateLimitEmailKey(email)} {
		if key == "" {
			continue
		}
		entry, ok := l.entries[key]
		if !ok {
			continue
		}
		if now.Sub(entry.lastSeen) > loginRateLimitEntryTTL {
			delete(l.entries, key)
			continue
		}
		entry.lastSeen = now
		if !entry.blockedUntil.IsZero() && !entry.blockedUntil.After(now) {
			entry.failures = 0
			entry.blockedUntil = time.Time{}
			continue
		}
		if entry.blockedUntil.IsZero() || !entry.blockedUntil.After(now) {
			continue
		}
		wait := entry.blockedUntil.Sub(now)
		if wait > remaining {
			remaining = wait
		}
		blocked = true
	}
	return remaining, blocked
}

func (l *loginRateLimiter) registerFailure(ip, email string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	remaining := time.Duration(0)
	for _, key := range []string{loginRateLimitIPKey(ip), loginRateLimitEmailKey(email)} {
		if key == "" {
			continue
		}
		entry, ok := l.entries[key]
		if !ok || now.Sub(entry.lastSeen) > loginRateLimitEntryTTL {
			entry = &loginRateLimitEntry{}
			l.entries[key] = entry
		}
		if !entry.blockedUntil.IsZero() && !entry.blockedUntil.After(now) {
			entry.failures = 0
			entry.blockedUntil = time.Time{}
		}
		entry.failures++
		entry.lastSeen = now
		if entry.failures >= loginRateLimitMaxFailures {
			entry.blockedUntil = now.Add(loginRateLimitWindow)
			wait := entry.blockedUntil.Sub(now)
			if wait > remaining {
				remaining = wait
			}
		}
	}
	return remaining
}

func (l *loginRateLimiter) reset(ip, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, loginRateLimitIPKey(ip))
	delete(l.entries, loginRateLimitEmailKey(email))
}

func loginRateLimitIPKey(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	return "ip:" + ip
}

func loginRateLimitEmailKey(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ""
	}
	return "email:" + email
}

func loginClientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func writeLoginRateLimitError(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int((retryAfter + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":               "Too many failed login attempts. Try again after 30 seconds.",
		"retry_after_seconds": secs,
	})
}
