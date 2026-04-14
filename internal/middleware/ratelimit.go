package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type perKeyEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // UnixNano
}

// PerKeyLimiter enforces a token-bucket rate limit per key (e.g. per IP).
type PerKeyLimiter struct {
	keys  sync.Map
	rps   rate.Limit
	burst int
	done  chan struct{}
}

func NewPerKeyLimiter(rps float64, burst int) *PerKeyLimiter {
	pl := &PerKeyLimiter{
		rps:   rate.Limit(rps),
		burst: burst,
		done:  make(chan struct{}),
	}
	go pl.cleanup()
	return pl
}

func (pl *PerKeyLimiter) Close() {
	close(pl.done)
}

func (pl *PerKeyLimiter) Allow(key string) (retryAfter time.Duration, ok bool) {
	now := time.Now()
	entry := pl.getOrCreate(key, now)
	entry.lastSeen.Store(now.UnixNano())

	r := entry.limiter.ReserveN(now, 1)
	delay := r.DelayFrom(now)
	if delay == 0 {
		return 0, true
	}
	r.CancelAt(now)
	return delay, false
}

func (pl *PerKeyLimiter) getOrCreate(key string, now time.Time) *perKeyEntry {
	if v, ok := pl.keys.Load(key); ok {
		return v.(*perKeyEntry)
	}
	entry := &perKeyEntry{
		limiter: rate.NewLimiter(pl.rps, pl.burst),
	}
	entry.lastSeen.Store(now.UnixNano())
	if actual, loaded := pl.keys.LoadOrStore(key, entry); loaded {
		return actual.(*perKeyEntry)
	}
	return entry
}

func (pl *PerKeyLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-pl.done:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute).UnixNano()
			pl.keys.Range(func(key, value any) bool {
				if value.(*perKeyEntry).lastSeen.Load() < cutoff {
					pl.keys.Delete(key)
				}
				return true
			})
		}
	}
}

// IPRateLimit returns a Gin middleware that rate-limits by client IP.
func IPRateLimit(pl *PerKeyLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if key == "" {
			key = "unknown"
		}

		retryAfter, ok := pl.Allow(key)
		if !ok {
			secs := int(math.Ceil(retryAfter.Seconds()))
			c.Header("Retry-After", strconv.Itoa(secs))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":      "rate_limit_exceeded",
				"message":    "Too many requests. Please try again later.",
				"retryAfter": secs,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
