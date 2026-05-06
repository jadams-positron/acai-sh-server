package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

// RateLimitSpec specifies a rate limit window.
type RateLimitSpec struct {
	Requests      int
	WindowSeconds int
}

// Limiter is the rate-limiter contract.
type Limiter interface {
	Allow(endpointKey, tokenID string, spec RateLimitSpec, now time.Time) bool
}

// InProcessLimiter is the default Limiter — sync.Map keyed by
// (endpoint, tokenID, bucket) → atomic counter. Buckets in earlier windows
// are pruned lazily.
type InProcessLimiter struct {
	buckets sync.Map // map[string]*atomic.Int64
}

// NewInProcessLimiter returns a fresh in-process limiter.
func NewInProcessLimiter() *InProcessLimiter { return &InProcessLimiter{} }

// Allow implements Limiter.
func (l *InProcessLimiter) Allow(endpointKey, tokenID string, spec RateLimitSpec, now time.Time) bool {
	if spec.Requests <= 0 || spec.WindowSeconds <= 0 {
		return true
	}
	bucket := now.Unix() / int64(spec.WindowSeconds)
	key := fmt.Sprintf("%s:%s:%d", endpointKey, tokenID, bucket)

	val, _ := l.buckets.LoadOrStore(key, new(atomic.Int64))
	counter, ok := val.(*atomic.Int64)
	if !ok {
		return true
	}
	count := counter.Add(1)

	if count%100 == 0 {
		l.pruneOlder(bucket)
	}

	return count <= int64(spec.Requests)
}

func (l *InProcessLimiter) pruneOlder(currentBucket int64) {
	l.buckets.Range(func(k, _ any) bool {
		ks, _ := k.(string)
		// Bucket number is the trailing segment after the last ':'.
		i := strings.LastIndex(ks, ":")
		if i < 0 {
			return true
		}
		b, err := strconv.ParseInt(ks[i+1:], 10, 64)
		if err == nil && b < currentBucket {
			l.buckets.Delete(ks)
		}
		return true
	})
}

// RateLimit returns middleware that consults limiter for each request, keyed
// by the path + the authenticated token ID (or "anonymous").
func RateLimit(specForPath func(path string) RateLimitSpec, limiter Limiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			spec := specForPath(c.Request().URL.Path)
			tokenID := "anonymous"
			if t := TokenFromEcho(c); t != nil {
				tokenID = t.ID
			}
			if !limiter.Allow(c.Request().URL.Path, tokenID, spec, time.Now()) {
				return apierror.WriteAppErrorEcho(c, http.StatusTooManyRequests, "Rate limit exceeded", "")
			}
			return next(c)
		}
	}
}
