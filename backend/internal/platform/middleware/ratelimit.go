package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipTTL is the idle period after which a per-IP entry is pruned from the map.
// An IP that has made no requests for this duration has its limiter discarded;
// the next request from that IP starts a fresh bucket.
const ipTTL = 10 * time.Minute

// ipEntry holds a token-bucket limiter and the timestamp of the last request
// from the associated IP address.
type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter enforces per-IP token-bucket rate limits.
//
// Each unique client IP gets its own limiter, created lazily on first request.
// A background goroutine prunes entries that have been idle for ipTTL, bounding
// memory growth to O(active unique IPs) rather than O(all historical IPs).
//
// Concurrency: all map operations are protected by a sync.Mutex.
// rate.Limiter is itself goroutine-safe; we only hold the mutex long enough to
// look up or create the entry, then release it before calling Allow().
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipEntry
	r        rate.Limit
	b        int
	done     chan struct{}
}

// NewIPRateLimiter creates a rate limiter allowing r sustained requests per
// second with a burst of b. The caller must call Stop() when the limiter is
// no longer needed to release the background cleanup goroutine.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	l := &IPRateLimiter{
		limiters: make(map[string]*ipEntry),
		r:        r,
		b:        b,
		done:     make(chan struct{}),
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
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.limiters[ip]
	if !ok {
		e = &ipEntry{limiter: rate.NewLimiter(l.r, l.b)}
		l.limiters[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// cleanup runs on the given interval and removes entries that have been idle
// for longer than ipTTL. It exits when the done channel is closed.
func (l *IPRateLimiter) cleanup(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			l.mu.Lock()
			for ip, e := range l.limiters {
				if time.Since(e.lastSeen) > ipTTL {
					delete(l.limiters, ip)
				}
			}
			l.mu.Unlock()
		case <-l.done:
			return
		}
	}
}

// Middleware returns an HTTP middleware that rejects requests from IPs that
// have exhausted their token bucket with HTTP 429 Too Many Requests.
//
// The middleware must be mounted after chimw.RealIP so that r.RemoteAddr
// already reflects the true client IP from X-Forwarded-For / X-Real-IP.
func (l *IPRateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !l.get(ip).Allow() {
				w.Header().Set("Content-Type", "application/json")
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
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the host portion of r.RemoteAddr, which chimw.RealIP
// has already normalised to the real client address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
