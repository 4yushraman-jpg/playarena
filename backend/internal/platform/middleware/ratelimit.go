package middleware

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// ipTTL is the idle period after which a per-IP entry is pruned from the map.
// An IP that has made no requests for this duration has its limiter discarded;
// the next request from that IP starts a fresh bucket.
const ipTTL = 10 * time.Minute

// ipEntry holds a token-bucket limiter and the timestamp of the last request
// from the associated IP address. lastSeen is an atomic int64 (Unix nanoseconds)
// so that concurrent requests can update it without acquiring a lock.
type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // unix nanoseconds; updated on each request via Store
}

// IPRateLimiter enforces per-IP token-bucket rate limits.
//
// Each unique client IP gets its own limiter, created lazily on first request.
// A background goroutine prunes entries that have been idle for ipTTL, bounding
// memory growth to O(active unique IPs) rather than O(all historical IPs).
//
// Concurrency: the limiter map is a sync.Map — reads are lock-free and writes
// (new IP registration) use fine-grained internal locks. The cleanup goroutine
// uses sync.Map.Range, which does not hold a global lock for the full iteration,
// eliminating the O(n) mutex-hold stall that occurred with a plain map+Mutex.
type IPRateLimiter struct {
	limiters sync.Map // key: string IP, value: *ipEntry
	r        rate.Limit
	b        int
	done     chan struct{}
}

// NewIPRateLimiter creates a rate limiter allowing r sustained requests per
// second with a burst of b. The caller must call Stop() when the limiter is
// no longer needed to release the background cleanup goroutine.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	l := &IPRateLimiter{
		r:    r,
		b:    b,
		done: make(chan struct{}),
	}
	go l.cleanup(5 * time.Minute)
	return l
}

// Stop signals the background cleanup goroutine to exit. Safe to call once;
// subsequent calls are no-ops.
func (l *IPRateLimiter) Stop() {
	select {
	case <-l.done:
		// Already stopped.
	default:
		close(l.done)
	}
}

// get returns the limiter for ip, creating one if this is the first request
// from that address. It also stamps the entry's lastSeen so the cleanup
// goroutine does not prune active IPs.
func (l *IPRateLimiter) get(ip string) *rate.Limiter {
	now := time.Now().UnixNano()

	if v, ok := l.limiters.Load(ip); ok {
		e := v.(*ipEntry)
		e.lastSeen.Store(now)
		return e.limiter
	}

	// New IP — create an entry and race to store it. LoadOrStore returns the
	// winner; if another goroutine stored first we use that entry instead.
	e := &ipEntry{limiter: rate.NewLimiter(l.r, l.b)}
	e.lastSeen.Store(now)
	actual, _ := l.limiters.LoadOrStore(ip, e)
	winner := actual.(*ipEntry)
	winner.lastSeen.Store(now)
	return winner.limiter
}

// cleanup runs on the given interval and removes entries that have been idle
// for longer than ipTTL. sync.Map.Range does not hold a global mutex for the
// full iteration, so this goroutine does not block concurrent rate-limit checks.
func (l *IPRateLimiter) cleanup(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			cutoff := time.Now().Add(-ipTTL).UnixNano()
			l.limiters.Range(func(k, v any) bool {
				if v.(*ipEntry).lastSeen.Load() < cutoff {
					l.limiters.Delete(k)
				}
				return true
			})
		case <-l.done:
			return
		}
	}
}

// Middleware returns an HTTP middleware that rejects requests from IPs that
// have exhausted their token bucket with HTTP 429 Too Many Requests.
//
// The middleware must be mounted after TrustedRealIP (or chimw.RealIP) so that
// r.RemoteAddr already reflects the true client IP.
func (l *IPRateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !l.get(ip).Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WriteMiddleware returns an HTTP middleware that applies rate limiting only to
// write requests (POST, PUT, PATCH, DELETE). GET and HEAD requests pass through
// unconditionally so read-heavy routes are not throttled.
//
// Use this for domain write endpoints. For route groups where all methods should
// be rate-limited (e.g. the /auth subtree), use Middleware() instead.
func (l *IPRateLimiter) WriteMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				ip := clientIP(r)
				if !l.get(ip).Allow() {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the host portion of r.RemoteAddr, which TrustedRealIP
// (or chimw.RealIP) has already normalised to the real client address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
