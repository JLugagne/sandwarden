package app

import (
	"sync"
	"time"
)

const (
	reconcileRetryBase = time.Minute
	reconcileRetryMax  = 15 * time.Minute
)

type backoff struct {
	base     time.Duration
	max      time.Duration
	mu       sync.Mutex
	failures map[string]int
	retryAt  map[string]time.Time
}

func newBackoff(base, max time.Duration) *backoff {
	return &backoff{base: base, max: max, failures: map[string]int{}, retryAt: map[string]time.Time{}}
}

func (b *backoff) ready(key string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !now.Before(b.retryAt[key])
}

func (b *backoff) fail(key string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures[key]++
	delay := b.max
	if n := b.failures[key]; n <= 16 {
		delay = min(b.base<<(n-1), b.max)
	}
	b.retryAt[key] = now.Add(delay)
}

func (b *backoff) succeed(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.failures, key)
	delete(b.retryAt, key)
}

func (b *backoff) retain(seen map[string]bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range b.failures {
		if !seen[key] {
			delete(b.failures, key)
			delete(b.retryAt, key)
		}
	}
}
