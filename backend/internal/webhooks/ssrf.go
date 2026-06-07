// Package webhooks implements SSRF protection for webhook URL validation.
//
// Two layers of protection:
//  1. Registration-time: ValidateURL rejects non-HTTPS schemes and known-private
//     hostnames/CIDRs before the URL is persisted.
//  2. Delivery-time: SSRFSafeTransport wraps http.Transport with a custom
//     DialContext that resolves DNS and verifies all resolved IPs are public
//     before establishing the connection, preventing DNS rebinding attacks.
package webhooks

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// blockedCIDRs is the set of IP ranges that must never receive webhook deliveries.
// Covers: loopback, RFC1918 private, RFC4193 ULA, link-local, APIPA, and
// cloud metadata service addresses.
var blockedCIDRs []*net.IPNet

func init() {
	ranges := []string{
		// IPv4 loopback
		"127.0.0.0/8",
		// IPv4 private (RFC1918)
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		// IPv4 link-local / APIPA
		"169.254.0.0/16",
		// IPv4 shared address space (RFC6598) — used by some cloud metadata services
		"100.64.0.0/10",
		// IPv4 loopback extended
		"0.0.0.0/8",
		// IPv6 loopback
		"::1/128",
		// IPv6 ULA (RFC4193)
		"fc00::/7",
		// IPv6 link-local
		"fe80::/10",
	}
	for _, r := range ranges {
		_, cidr, err := net.ParseCIDR(r)
		if err != nil {
			panic("ssrf: failed to parse CIDR " + r + ": " + err.Error())
		}
		blockedCIDRs = append(blockedCIDRs, cidr)
	}
}

// isPublicIP reports whether ip is a publicly routable address.
// Returns false for loopback, private, link-local, and metadata service IPs.
func isPublicIP(ip net.IP) bool {
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return false
		}
	}
	return true
}

// ValidateURL validates a webhook URL at registration time.
//
// Rules (all must pass):
//   - Scheme must be "https" (case-insensitive)
//   - Host must not be an IP literal in a blocked CIDR
//   - Host must not be "localhost" or any loopback hostname
func ValidateURL(rawURL string) error {
	if rawURL == "" {
		return ErrURLRequired
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}

	if !strings.EqualFold(u.Scheme, "https") {
		return ErrInvalidURL
	}

	host := u.Hostname()
	if host == "" {
		return ErrInvalidURL
	}

	// Reject well-known loopback hostnames.
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return ErrSSRFBlocked
	}

	// If host is an IP literal, check it immediately without DNS resolution.
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return ErrSSRFBlocked
		}
	}
	// Hostname-based targets are further protected by SSRFSafeTransport at
	// delivery time (DNS rebinding protection via DialContext IP verification).

	return nil
}

// SSRFSafeTransport returns an http.Transport whose DialContext resolves the
// target hostname and verifies that all resolved IPs are publicly routable
// before allowing the connection. This prevents DNS rebinding attacks where a
// hostname is registered with a public IP but later resolves to a private one.
//
// The returned transport uses a 10-second dial timeout and a 30-second TLS
// handshake timeout, both matching PlayArena's outbound HTTP conventions.
func SSRFSafeTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	t := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("ssrf: invalid address %q: %w", addr, err)
			}

			// Resolve hostname to IPs. net.DefaultResolver uses the OS resolver
			// which follows /etc/resolv.conf. We do not need to customise the
			// resolver here — the goal is to verify the resolved IPs before
			// dialling, which already defeats DNS rebinding.
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("ssrf: DNS lookup failed for %q: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("ssrf: no IPs resolved for %q", host)
			}

			// All resolved IPs must be public.
			for _, ia := range ips {
				if !isPublicIP(ia.IP) {
					return nil, fmt.Errorf("ssrf: %q resolves to a private IP (%s), delivery blocked", host, ia.IP)
				}
			}

			// Dial using the first resolved public IP directly (no re-resolution).
			resolvedAddr := net.JoinHostPort(ips[0].IP.String(), port)
			return dialer.DialContext(ctx, network, resolvedAddr)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
	return t
}
