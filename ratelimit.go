package main

import (
	"sync"
	"time"
)

// Limiter is a small in-memory sliding-window rate limiter keyed by IP.
// A single VM serves the app, so in-memory state is sufficient. It mirrors
// the burst+daily shape of the site's KV limiter.
type Limiter struct {
	mu       sync.Mutex
	hits     map[string][]time.Time
	burst    int // max requests within burstWin
	burstWin time.Duration
	day      int // max requests within 24h
}

func NewLimiter(burst int, burstWin time.Duration, day int) *Limiter {
	return &Limiter{
		hits:     map[string][]time.Time{},
		burst:    burst,
		burstWin: burstWin,
		day:      day,
	}
}

// Allow records a hit for ip and reports whether it is within limits. On
// denial it returns a short reason string for display.
func (l *Limiter) Allow(ip string) (bool, string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Prune anything older than 24h.
	cutoff := now.Add(-24 * time.Hour)
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.hits[ip] = kept

	if len(kept) >= l.day {
		return false, "daily limit reached — return tomorrow"
	}
	burstFrom := now.Add(-l.burstWin)
	recent := 0
	for _, t := range kept {
		if t.After(burstFrom) {
			recent++
		}
	}
	if recent >= l.burst {
		return false, "easy — too fast. wait a moment."
	}

	l.hits[ip] = append(l.hits[ip], now)
	return true, ""
}
