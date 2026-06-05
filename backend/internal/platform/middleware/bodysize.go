package middleware

import (
	"net/http"
)

// BodySizeLimit returns a middleware that caps request body reads at maxBytes.
// Requests whose bodies exceed the limit are rejected with 413 Request Entity
// Too Large — the JSON decoder returns *http.MaxBytesError, which
// validator.DecodeJSON maps to validator.ErrBodyTooLarge, and the handler maps
// that sentinel to http.StatusRequestEntityTooLarge.
//
// Apply this middleware per route group. Do NOT apply it to media upload
// routes — those enforce their own 10 MB limit inside the handler.
//
// Recommended value for JSON endpoints: 64 * 1024 (64 KB).
func BodySizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
