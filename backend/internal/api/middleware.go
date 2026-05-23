package api

import (
	"net/http"
	"os"
	"strings"
)

const defaultAllowedOrigin = "http://localhost:5173"

func allowedOrigin() string {
	origin := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGIN"))
	if origin == "" {
		return defaultAllowedOrigin
	}
	return origin
}

// CORSMiddleware applies CORS headers for the configured frontend origin and
// handles preflight requests.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := allowedOrigin()
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type, Authorization, X-PulseOps-Request-ID, X-PulseOps-Request-Attempt, X-PulseOps-Device-ID",
		)
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
