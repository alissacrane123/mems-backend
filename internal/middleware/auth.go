// This package contains middleware — functions that run before HTTP handlers.
// Middleware is useful for things like auth checks, logging, and CORS
// that need to happen on every request without copy-pasting code everywhere.
package middleware

import (
	"context"
	"net/http"

	// Replace with your actual module name
	"github.com/alissacrane123/mems-backend/internal/auth"
)

// contextKey is a custom type for context keys.
// Using a custom type prevents collisions with other packages
// that might also store things in context using plain strings.
type contextKey string

// UserIDKey and UserEmailKey are the keys we use to store
// the user's info in the request context after validating the token.
const (
	UserIDKey    contextKey = "user_id"
	UserEmailKey contextKey = "user_email"
)

// RequireAuth is the middleware function.
// It takes the next handler as an argument and returns a new handler —
// this is the standard Go middleware pattern.
func RequireAuth(next http.Handler) http.Handler {
	// http.HandlerFunc converts our function into an http.Handler
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to read the "token" cookie
		cookie, err := r.Cookie("token")
		if err != nil {
			// Cookie is missing — user is not logged in
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}

		// Validate the token and extract the claims
		claims, err := auth.ValidateToken(cookie.Value)
		if err != nil {
			// Token is invalid or expired
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// Token is valid — attach the user's ID and email to the request context
		// so any handler down the chain can access them without re-reading the token.
		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)

		// Call the next handler, passing the updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID is a helper to read the user ID from the context inside a handler.
// Usage: userID := middleware.GetUserID(r)
func GetUserID(r *http.Request) string {
	val, _ := r.Context().Value(UserIDKey).(string)
	return val
}

// GetUserEmail is a helper to read the user email from the context inside a handler.
func GetUserEmail(r *http.Request) string {
	val, _ := r.Context().Value(UserEmailKey).(string)
	return val
}