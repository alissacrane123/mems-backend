// This file handles user-related endpoints.
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
)

// UsersHandler holds the database connection pool.
// Same pattern as AuthHandler — struct with DB, methods for each endpoint.
type UsersHandler struct {
	DB *pgxpool.Pool
}

// Me handles GET /api/users/me
// Returns the current user's profile using the user ID from the auth cookie.
func (h *UsersHandler) Me(w http.ResponseWriter, r *http.Request) {
	// Get the user ID that the middleware attached to the request context
	userID := middleware.GetUserID(r)
	userEmail := middleware.GetUserEmail(r)

	// Query the profiles table for this user's name
	var firstName, lastName string
	err := h.DB.QueryRow(context.Background(),
		`SELECT first_name, last_name FROM profiles WHERE id = $1`,
		userID,
	).Scan(&firstName, &lastName)

	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         userID,
		"email":      userEmail,
		"first_name": firstName,
		"last_name":  lastName,
	})
}

// lookupByEmailRequest defines the request body shape for email lookup.
type lookupByEmailRequest struct {
	Email string `json:"email"`
}

// LookupByEmail handles POST /api/users/lookup-by-email
// Used to find a user's ID by their email — for example when inviting someone to a board.
func (h *UsersHandler) LookupByEmail(w http.ResponseWriter, r *http.Request) {
	// Decode the request body
	var req lookupByEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Look up the user in the users table by email
	var userID string
	err := h.DB.QueryRow(context.Background(),
		`SELECT id FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID)

	if err != nil {
		// User not found — return exists: false instead of an error
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"exists":  false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"exists":  true,
		"userId":  userID,
	})
}