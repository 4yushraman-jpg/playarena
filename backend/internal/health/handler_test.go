package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/health"
)

func TestLive_AlwaysReturns200(t *testing.T) {
	h := health.New(nil) // nil pool — liveness must not touch DB

	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rr := httptest.NewRecorder()
	h.Live(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("expected status=alive, got %q", body["status"])
	}
}
