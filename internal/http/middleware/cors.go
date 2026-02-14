package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig holds CORS configuration options.
type CORSConfig struct {
	// AllowedOrigins is a list of origins that are allowed to make requests.
	// Use "*" to allow all origins.
	AllowedOrigins []string
	// AllowedMethods is a list of HTTP methods allowed.
	AllowedMethods []string
	// AllowedHeaders is a list of headers that can be used in the request.
	AllowedHeaders []string
	// ExposedHeaders is a list of headers that can be read by the client.
	ExposedHeaders []string
	// AllowCredentials indicates whether credentials are allowed.
	AllowCredentials bool
	// MaxAge is the maximum age (in seconds) for preflight cache.
	MaxAge int
}

// DefaultCORSConfig returns a permissive CORS configuration suitable for development.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORS returns a CORS middleware with default configuration.
func CORS() func(http.Handler) http.Handler {
	return CORSWithConfig(DefaultCORSConfig())
}

// CORSWithConfig returns a CORS middleware with custom configuration.
func CORSWithConfig(config CORSConfig) func(http.Handler) http.Handler {
	allowedMethods := strings.Join(config.AllowedMethods, ", ")
	allowedHeaders := strings.Join(config.AllowedHeaders, ", ")
	exposedHeaders := strings.Join(config.ExposedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if origin != "" {
				allowed := false
				for _, o := range config.AllowedOrigins {
					if o == "*" || o == origin {
						allowed = true
						break
					}
				}

				if allowed {
					if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					} else {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Add("Vary", "Origin")
					}

					if config.AllowCredentials {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}

					if exposedHeaders != "" {
						w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)
					}
				}
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
