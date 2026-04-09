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
	loginThrottleWindow      = 10 * time.Minute
	loginThrottleBlock       = 2 * time.Minute
	loginThrottleMaxFailures = 5
)

type loginThrottle struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

type loginAttempt struct {
	Failures     int
	FirstFailure time.Time
	BlockedUntil time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{
		attempts: make(map[string]loginAttempt),
	}
}

func (l *loginThrottle) retryAfter(key string, now time.Time) (time.Duration, bool) {
	if l == nil || strings.TrimSpace(key) == "" {
		return 0, false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	state, ok := l.attempts[key]
	if !ok {
		return 0, false
	}
	if !state.BlockedUntil.IsZero() && now.Before(state.BlockedUntil) {
		return state.BlockedUntil.Sub(now), true
	}
	if !state.BlockedUntil.IsZero() && !now.Before(state.BlockedUntil) {
		delete(l.attempts, key)
	}
	return 0, false
}

func (l *loginThrottle) registerFailure(key string, now time.Time) time.Duration {
	if l == nil || strings.TrimSpace(key) == "" {
		return 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.attempts[key]
	if state.FirstFailure.IsZero() || now.Sub(state.FirstFailure) > loginThrottleWindow {
		state = loginAttempt{
			Failures:     1,
			FirstFailure: now,
		}
		l.attempts[key] = state
		return 0
	}

	state.Failures++
	if state.Failures >= loginThrottleMaxFailures {
		state.BlockedUntil = now.Add(loginThrottleBlock)
	}
	l.attempts[key] = state
	if state.BlockedUntil.IsZero() {
		return 0
	}
	return state.BlockedUntil.Sub(now)
}

func (l *loginThrottle) reset(key string) {
	if l == nil || strings.TrimSpace(key) == "" {
		return
	}

	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func loginThrottleKey(r *http.Request, username string) string {
	var remoteIP string
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			parts := strings.Split(value, ",")
			value = strings.TrimSpace(parts[0])
		}
		if parsed := net.ParseIP(value); parsed != nil {
			remoteIP = parsed.String()
			break
		}
	}

	if remoteIP == "" {
		host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
		if err == nil {
			remoteIP = host
		}
	}
	if remoteIP == "" {
		remoteIP = "unknown"
	}

	return strings.ToLower(strings.TrimSpace(username)) + "@" + remoteIP
}

func writeRetryAfter(w http.ResponseWriter, delay time.Duration) {
	if delay <= 0 {
		return
	}
	seconds := int(delay.Round(time.Second) / time.Second)
	if seconds <= 0 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}
