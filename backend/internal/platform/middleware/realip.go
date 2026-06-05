package middleware

import (
	"net"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// TrustedRealIP returns a middleware that rewrites r.RemoteAddr from
// X-Forwarded-For / X-Real-IP only when the direct TCP connection originates
// from a CIDR in trustedCIDRs, preventing rate-limit bypass via spoofed
// forwarding headers from untrusted clients.
//
// When trustedCIDRs is empty, the middleware falls back to chimw.RealIP
// (unconditional header processing) for backward compatibility with deployments
// that do not configure TRUSTED_PROXY_CIDRS. Set TRUSTED_PROXY_CIDRS in
// production to the load balancer's egress IP range to enable enforcement.
func TrustedRealIP(trustedCIDRs []string) func(http.Handler) http.Handler {
	if len(trustedCIDRs) == 0 {
		// Backward-compatible fallback: no trusted CIDRs configured.
		return chimw.RealIP
	}

	nets := parseCIDRs(trustedCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isTrustedProxy(r.RemoteAddr, nets) {
				// Connection from a known proxy — delegate to chi's RealIP to
				// rewrite RemoteAddr from the forwarding headers.
				chimw.RealIP(next).ServeHTTP(w, r)
				return
			}
			// Untrusted connection — ignore forwarding headers entirely.
			next.ServeHTTP(w, r)
		})
	}
}

func parseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil && n != nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func isTrustedProxy(remoteAddr string, nets []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
