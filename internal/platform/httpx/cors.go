package httpx

import (
	"net/http"
	"slices"
)

const (
	corsAllowMethods  = "GET, POST, PUT, PATCH, DELETE"
	corsAllowHeaders  = "Authorization, Content-Type, Idempotency-Key"
	corsExposeHeaders = "Idempotent-Replay, Retry-After, X-Certificate-SHA256, X-File-SHA256, X-File-Rows, Location"
	corsMaxAge        = "600"
)

// CORS lets the listed browser origins (the player web app) call the API. Requests from other origins get no CORS
// headers, so the browser blocks them; requests without an Origin (apps, servers, same-origin) pass untouched. No
// credentials mode: tokens travel in the Authorization header, never in cookies, so there is nothing for another site
// to ride on. An empty list turns CORS off (closed).
func CORS(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Add("Vary", "Origin")
			allowed := slices.Contains(origins, origin)
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if preflight {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				if !allowed {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", corsAllowMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
				h.Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if allowed {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Expose-Headers", corsExposeHeaders)
			}
			next.ServeHTTP(w, r)
		})
	}
}
