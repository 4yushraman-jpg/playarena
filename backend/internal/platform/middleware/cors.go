package middleware

import "net/http"

// CORS returns a middleware that sets CORS response headers.
//
// allowedOrigins controls which request origins receive an
// Access-Control-Allow-Origin echo. Use explicit domain names in production
// (e.g. "https://app.playarena.com"). Passing []string{"*"} permits any
// origin in development — credentials are NOT sent in that mode because the
// browser rejects Access-Control-Allow-Credentials alongside a wildcard origin.
//
// When a specific origin is matched, Access-Control-Allow-Credentials: true is
// included so browsers forward cookies and Authorization headers on
// cross-origin requests.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	// wildcardOnly: when the only allowed origin is "*" we must not send the
	// credentials header — browsers reject credentials with wildcard ACAO.
	wildcardOnly := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if matchesOrigin(origin, allowedOrigins) {
				// Reflect the exact origin rather than echoing "*" so that
				// browsers honour credentials and the Vary header is correct.
				w.Header().Set("Access-Control-Allow-Origin", origin)

				if !wildcardOnly {
					// Specific-origin match: allow cookies and auth headers.
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
			// Vary tells CDNs and proxies that the response differs by origin.
			w.Header().Add("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func matchesOrigin(origin string, allowed []string) bool {
	if origin == "" {
		return false
	}
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}
