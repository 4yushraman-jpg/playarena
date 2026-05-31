package middleware

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Recovery catches panics, logs a stack trace, and returns a 500 response.
// Delegates to chi's built-in Recoverer.
var Recovery func(http.Handler) http.Handler = chimw.Recoverer
