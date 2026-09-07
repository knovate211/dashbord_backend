// Package auth provides JWT validation middleware for the API gateway.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	pkgauth "github.com/knovate211/pkg/auth"
)

type contextKey string

const (
	CtxUserID contextKey = "user_id"
	CtxRole   contextKey = "role"
)

// Auth returns an HTTP middleware that validates JWTs and injects user claims into the request context.
// Requests to public paths (listed in publicPaths) bypass authentication entirely.
// Requests to optional paths get JWT parsed if present, but are not blocked if token is missing.
func Auth(validator *pkgauth.Validator, log *zap.Logger, publicPaths ...string) func(http.Handler) http.Handler {
	publicSet := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		publicSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow public paths with no token required
			if publicSet[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Extract token
			token := r.Header.Get("Authorization")
			if token == "" {
				token = r.URL.Query().Get("token") // fallback for WebSocket
			}

			// For /api/graphql: parse token if present but don't block if missing
			// (individual resolvers enforce auth per-field)
			if r.URL.Path == "/api/graphql" {
				var uid string
				if token != "" {
					claims, err := validator.Validate(strings.TrimPrefix(token, "Bearer "))
					if err == nil {
						ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
						ctx = context.WithValue(ctx, CtxRole, claims.Role)
						r.Header.Set("X-User-ID", claims.UserID)
						r.Header.Set("X-User-Role", claims.Role)
						r = r.WithContext(ctx)
						uid = claims.UserID
					} else {
						log.Warn("graphql token validation failed", zap.Error(err))
					}
				} else {
					log.Warn("graphql request with missing authorization token")
				}
				log.Info("GraphQL Request", zap.String("user_id", uid))
				next.ServeHTTP(w, r)
				return
			}

			if token == "" {
				http.Error(w, `{"error":"missing authorization token"}`, http.StatusUnauthorized)
				return
			}

			// Validate
			claims, err := validator.Validate(strings.TrimPrefix(token, "Bearer "))
			if err != nil {
				log.Warn("invalid token", zap.Error(err))
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Inject into context
			ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
			ctx = context.WithValue(ctx, CtxRole, claims.Role)

			// Also set headers for downstream services (WebSocket, etc.)
			r.Header.Set("X-User-ID", claims.UserID)
			r.Header.Set("X-User-Role", claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxUserID).(string)
	return v
}

// RoleFromContext extracts the user role from the request context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxRole).(string)
	return v
}

// CORS returns a middleware that adds CORS headers for the GraphQL endpoint.
func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	// Parse allowed origins once at middleware initialization for O(1) lookups
	var originsMap map[string]bool
	allowAll := false

	origins := strings.Split(allowedOrigins, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
		if origins[i] == "*" {
			allowAll = true
		}
	}

	if !allowAll {
		originsMap = make(map[string]bool, len(origins))
		for _, o := range origins {
			if o != "" {
				originsMap[o] = true
			}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				var isAllowed bool
				if allowAll {
					isAllowed = true
				} else {
					isAllowed = originsMap[origin]
				}

				if isAllowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// When Access-Control-Allow-Credentials is true, the Access-Control-Allow-Origin
					// cannot be wildcard "*", so we dynamically reflect the allowed origin.
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			// PATCH is used by the admin and recruiter REST surfaces (updating a
			// user's role, editing a test, grading a written answer, recording a
			// shortlist decision). Omitting it here fails the browser preflight
			// even though the route itself works.
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
