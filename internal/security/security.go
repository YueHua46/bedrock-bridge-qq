package security

import (
	"net/http"
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
	return &Limiter{
		limit:  limit,
		window: window,
		bucket: map[string][]time.Time{},
	}
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	events := l.bucket[key]
	cutoff := now.Add(-l.window)
	kept := events[:0]
	for _, ts := range events {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.limit {
		l.bucket[key] = kept
		return false
	}
	kept = append(kept, now)
	l.bucket[key] = kept
	return true
}

func BearerOK(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	got := r.Header.Get("Authorization")
	if !strings.HasPrefix(got, "Bearer ") {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(got, "Bearer ")) == token
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
