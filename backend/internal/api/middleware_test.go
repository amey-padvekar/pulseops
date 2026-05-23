package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/certainelf/pulseops/backend/internal/api"
)

func TestCORSMiddleware_DefaultOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGIN", "")

	h := api.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("Access-Control-Allow-Methods should be set")
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("Access-Control-Allow-Headers should be set")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestCORSMiddleware_EnvOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGIN", "https://dashboard.example.com")

	h := api.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://dashboard.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "https://dashboard.example.com")
	}
}

func TestCORSMiddleware_PreflightReturnsNoContent(t *testing.T) {
	_ = os.Unsetenv("CORS_ALLOWED_ORIGIN")

	nextCalled := false
	h := api.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/telemetry", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if nextCalled {
		t.Fatal("next handler should not be called for OPTIONS preflight")
	}
}
