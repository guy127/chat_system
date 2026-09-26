package httpx

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// KeyedLimiter is a token bucket per key (IP, member ID, ...), with idle
// buckets evicted in the background until ctx is cancelled.
type KeyedLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	buckets map[string]*bucket
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

func NewKeyedLimiter(ctx context.Context, limit rate.Limit, burst int) *KeyedLimiter {
	l := &KeyedLimiter{limit: limit, burst: burst, buckets: map[string]*bucket{}}
	go l.evict(ctx, 5*time.Minute)
	return l
}

func (l *KeyedLimiter) Allow(key string) bool {
	l.mu.Lock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[key] = b
	}
	b.seen = time.Now()
	l.mu.Unlock()
	return b.lim.Allow()
}

func (l *KeyedLimiter) evict(ctx context.Context, idle time.Duration) {
	t := time.NewTicker(idle)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			l.mu.Lock()
			for k, b := range l.buckets {
				if now.Sub(b.seen) > idle {
					delete(l.buckets, k)
				}
			}
			l.mu.Unlock()
		}
	}
}
