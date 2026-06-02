package storage

// sigv4.go — AWS Signature Version 4 signing for S3 REST API calls.
// Implemented using Go standard library only (crypto/hmac, crypto/sha256,
// encoding/hex, net/url). No external SDK dependency.
//
// Reference: https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
//
// Supports PUT and DELETE requests. Uses UNSIGNED-PAYLOAD for the body hash
// so the request body does not need to be buffered for signing. This is
// supported by S3 for standard (non-streaming SigV4) PUT operations when the
// bucket policy permits it. All S3-compatible services (MinIO, R2) accept it.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	sigV4Algorithm  = "AWS4-HMAC-SHA256"
	sigV4Service    = "s3"
	sigV4Terminator = "aws4_request"
	unsignedPayload = "UNSIGNED-PAYLOAD"
)

// sigV4Signer holds credentials and region for signing.
type sigV4Signer struct {
	accessKey string
	secretKey string
	region    string
}

// sign mutates r by adding the necessary SigV4 authorization headers.
// The caller must have already set Content-Type and the full URL on r.
// The body is treated as UNSIGNED-PAYLOAD — the caller must not change the
// body after calling sign.
func (s *sigV4Signer) sign(r *http.Request) error {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	// Set required headers before building canonical form.
	r.Header.Set("x-amz-date", amzDate)
	r.Header.Set("x-amz-content-sha256", unsignedPayload)
	// host must be lowercase and without port 443/80 for canonical form.
	host := r.URL.Host
	r.Header.Set("host", host)

	// ── 1. Build canonical request ───────────────────────────────────────────
	canonicalURI := r.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalQueryString := canonicalQueryParams(r.URL.RawQuery)

	// Collect headers that will be signed. Must be lowercase and sorted.
	signedHeaders, canonicalHeaders := buildCanonicalHeaders(r)

	canonicalRequest := strings.Join([]string{
		r.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		unsignedPayload,
	}, "\n")

	// ── 2. Build string to sign ──────────────────────────────────────────────
	credentialScope := fmt.Sprintf("%s/%s/%s/%s", dateStamp, s.region, sigV4Service, sigV4Terminator)
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		credentialScope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	// ── 3. Derive signing key ────────────────────────────────────────────────
	signingKey := deriveSigningKey(s.secretKey, dateStamp, s.region, sigV4Service)

	// ── 4. Compute signature ─────────────────────────────────────────────────
	signature := hexHMAC(signingKey, []byte(stringToSign))

	// ── 5. Build Authorization header ────────────────────────────────────────
	auth := fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigV4Algorithm,
		s.accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	r.Header.Set("Authorization", auth)
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func buildCanonicalHeaders(r *http.Request) (signedHeaders, canonicalHeaders string) {
	// Collect header names that must be signed: host + all x-amz-* headers.
	signed := make(map[string]string)
	signed["host"] = r.Header.Get("host")
	for k, vs := range r.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") {
			signed[lk] = strings.TrimSpace(strings.Join(vs, ","))
		}
	}

	names := make([]string, 0, len(signed))
	for k := range signed {
		names = append(names, k)
	}
	sort.Strings(names)

	var hBuf strings.Builder
	for _, n := range names {
		hBuf.WriteString(n)
		hBuf.WriteByte(':')
		hBuf.WriteString(signed[n])
		hBuf.WriteByte('\n')
	}
	return strings.Join(names, ";"), hBuf.String()
}

// canonicalQueryParams sorts query parameters and URL-encodes them per SigV4.
func canonicalQueryParams(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	// Simple sort on key=value pairs (no percent-decode needed for our usage).
	parts := strings.Split(rawQuery, "&")
	sort.Strings(parts)
	return strings.Join(parts, "&")
}

func deriveSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte(sigV4Terminator))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func hexHMAC(key, data []byte) string {
	return hex.EncodeToString(hmacSHA256(key, data))
}

func hexSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
