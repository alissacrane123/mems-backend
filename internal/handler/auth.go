// This file handles all auth-related HTTP requests (signup, signin, signout, session).
// Each function here maps to one API endpoint.
package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	// The dot before the package path means we're importing from our own module.
	// Replace "github.com/yourusername/mems-backend" with whatever you used in go mod init.
	"github.com/alissacrane123/mems-backend/internal/auth"
)

// AuthHandler holds the database connection pool.
// A pool manages multiple DB connections efficiently - better than one single connection.
// All our handler methods will live on this struct so they can access the DB.
type AuthHandler struct {
	DB *pgxpool.Pool
}

// --- Helper functions ---

// writeJSON is a helper to send a JSON response.
// w is the response writer, status is the HTTP status code (e.g. 200, 400),
// and v is any value that will be converted to JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError is a helper to send a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// --- Signup ---

// signupRequest defines the shape of the request body for signup.
type signupRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Signup handles POST /api/auth/signup
// It creates a new user in the users table and a profile in the profiles table.
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	// Decode the JSON request body into our signupRequest struct
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Basic validation
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Hash the password using bcrypt - never store plain text passwords!
	// The 12 is the "cost" - higher = slower to hash but more secure.
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Insert the user into the users table and get back their new ID.
	// $1, $2 are placeholders for the arguments - this prevents SQL injection.
	var userID string
	err = h.DB.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		req.Email, string(passwordHash),
	).Scan(&userID) // Scan reads the returned value into our userID variable

	if err != nil {
		// If the email already exists, the UNIQUE constraint will cause an error
		if strings.Contains(err.Error(), "unique") {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Also create a profiles row for this user (replicates the Supabase trigger)
	_, err = h.DB.Exec(context.Background(),
		`INSERT INTO profiles (id, first_name, last_name) VALUES ($1, $2, $3)`,
		userID, req.FirstName, req.LastName,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}

	// Generate a JWT token for the new user
	token, err := auth.GenerateToken(userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Replace the writeJSON response in both Signup and Signin with this:
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteNoneMode, // changed from Lax to None
		MaxAge:   7 * 24 * 60 * 60,
		Path:     "/",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]string{"id": userID, "email": req.Email},
	})
}

// --- Signin ---

// signinRequest defines the shape of the request body for signin.
type signinRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Signin handles POST /api/auth/signin
func (h *AuthHandler) Signin(w http.ResponseWriter, r *http.Request) {
	var req signinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	log.Printf("Signin attempt for email: %s", req.Email)

	var userID, passwordHash string
	err := h.DB.QueryRow(context.Background(),
		`SELECT id, password_hash FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &passwordHash)

	if err != nil {
		log.Printf("User lookup failed: %v", err)
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	log.Printf("Found user: %s hash: %s", userID, passwordHash)

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		log.Printf("Password mismatch: %v", err)
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Generate a JWT token
	token, err := auth.GenerateToken(userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	// Replace the writeJSON response in both Signup and Signin with this:
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteNoneMode, // changed from Lax to None
		MaxAge:   7 * 24 * 60 * 60,
		Path:     "/",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]string{"id": userID, "email": req.Email},
	})
}

// --- Signout ---

func (h *AuthHandler) Signout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "token",
		Value:  "",
		MaxAge: -1, // tells the browser to delete it immediately
		Path:   "/",
	})
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// --- Session ---

// Session handles GET /api/auth/session
// It reads the JWT from the cookie instead of the Authorization header.
func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	// Read the "token" cookie from the request
	// r.Cookie returns an error if the cookie doesn't exist
	cookie, err := r.Cookie("token")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Validate the token value from the cookie
	claims, err := auth.ValidateToken(cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// Return the user info from the token claims
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]string{
			"id":    claims.UserID,
			"email": claims.Email,
		},
		"expires_at": time.Now().Add(7 * 24 * time.Hour),
	})
}
