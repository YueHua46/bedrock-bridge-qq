package security

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	bucket map[string][]time.Time
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	if limit <= 0 {
		limit = 30
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{
		limit:  limit,
		window: window,
		bucket: map[string][]time.Time{},
	}
}

func (l *Limiter) Allow(key string) bool {
	if key == "" {
		return false
	}
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Sweeping expired keys on every call keeps the bucket map bounded even
	// when peers rotate keys (e.g. attacker-sourced player names), preventing
	// unbounded memory growth.
	for k, events := range l.bucket {
		kept := events[:0]
		for _, ts := range events {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		if len(kept) == 0 {
			delete(l.bucket, k)
		} else {
			l.bucket[k] = kept
		}
	}

	events := l.bucket[key]
	if len(events) >= l.limit {
		l.bucket[key] = events
		return false
	}
	events = append(events, now)
	l.bucket[key] = events
	return true
}

// BearerOK reports whether the request carries a Bearer token equal to the
// expected value. Comparison is constant-time to avoid timing oracles.
func BearerOK(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	got := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(got, prefix) {
		return false
	}
	g := []byte(strings.TrimSpace(strings.TrimPrefix(got, prefix)))
	t := []byte(token)
	if len(g) != len(t) {
		return false
	}
	return subtle.ConstantTimeCompare(g, t) == 1
}

// SameOrigin reports whether r originates from a browser context whose
// Origin (or Referer fallback) host matches one of the allowed hosts.
// Empty Origin/Referer is treated as not same-origin to defeat CSRF that
// relies on stripped headers.
func SameOrigin(r *http.Request, allowedHosts []string) bool {
	raw := r.Header.Get("Origin")
	if raw == "" {
		raw = r.Header.Get("Referer")
	}
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	for _, h := range allowedHosts {
		if h == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(u.Host), []byte(h)) == 1 {
			return true
		}
	}
	return false
}

func CleanMessage(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
	if max <= 0 {
		max = 200
	}
	for utf8.RuneCountInString(s) > max {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}
