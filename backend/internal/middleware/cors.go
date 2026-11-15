package middleware

import (
	"net/http"
	"slices"
)

// CORS allows requests from the configured origins ("*" allows any origin).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := slices.Contains(allowedOrigins, "*")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || slices.Contains(allowedOrigins, origin)) {
				h.Set("Access-Control-Allow-Origin", origin)
				if !allowAll { // never allow credentials for wildcard origins
					h.Set("Access-Control-Allow-Credentials", "true")
				}
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
			}

			// Preflight request: answer directly.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
