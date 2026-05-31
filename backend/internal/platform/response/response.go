package response

import (
	"encoding/json"
	"net/http"
)

// Write marshals v as JSON and writes it to w with the given HTTP status code.
// Content-Type is set automatically. If marshalling fails, a 500 body is written.
func Write(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// Error writes a JSON error body: {"error": message}.
func Error(w http.ResponseWriter, status int, message string) {
	Write(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}
