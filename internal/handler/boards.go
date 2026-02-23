// This file handles all board-related endpoints.
package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
)

// BoardsHandler holds the database connection pool.
type BoardsHandler struct {
	DB *pgxpool.Pool
}

// generateInviteCode creates a random URL-safe string for board invite codes.
// This replicates what Supabase did automatically with gen_random_bytes().
func generateInviteCode() (string, error) {
	// Make a slice of 8 random bytes
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// Encode to base64url (URL-safe, no +/ characters)
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ListBoards handles GET /api/boards
// Returns all boards the current user is a member of,
// including their role and the member count for each board.
func (h *BoardsHandler) ListBoards(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	// This query joins boards with board_members to get the user's role,
	// and counts total members for each board.
	rows, err := h.DB.Query(context.Background(), `
		SELECT 
			b.id,
			b.name,
			b.description,
			b.invite_code,
			bm.role,
			(SELECT COUNT(*) FROM board_members WHERE board_id = b.id) as member_count
		FROM boards b
		JOIN board_members bm ON bm.board_id = b.id
		WHERE bm.user_id = $1
		ORDER BY b.created_at DESC
	`, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch boards")
		return
	}
	// defer rows.Close() ensures the database rows are cleaned up when
	// the function exits, even if something goes wrong partway through.
	defer rows.Close()

	// Build a slice (like an array) of boards to return
	// We use []map[string]any so we don't need to define a struct for this response
	boards := []map[string]any{}

	// rows.Next() advances through each row returned by the query
	for rows.Next() {
		var id, name, role string
		var description *string // pointer because description can be NULL in the DB
		var inviteCode string
		var memberCount int

		// Scan reads the current row's values into our variables
		if err := rows.Scan(&id, &name, &description, &inviteCode, &role, &memberCount); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read board data")
			return
		}

		boards = append(boards, map[string]any{
			"id":           id,
			"name":         name,
			"description":  description,
			"invite_code":  inviteCode,
			"role":         role,
			"member_count": memberCount,
		})
	}

	writeJSON(w, http.StatusOK, boards)
}

// CreateBoard handles POST /api/boards
// Creates a new board and automatically adds the creator as owner.
func (h *BoardsHandler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Generate a unique invite code for the board
	inviteCode, err := generateInviteCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate invite code")
		return
	}

	// Insert the board and get back its new ID
	var boardID string
	err = h.DB.QueryRow(context.Background(), `
		INSERT INTO boards (name, description, invite_code, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.Name, req.Description, inviteCode, userID).Scan(&boardID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create board")
		return
	}

	// Add the creator as owner — replicates the Supabase trigger
	_, err = h.DB.Exec(context.Background(), `
		INSERT INTO board_members (board_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, boardID, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add board owner")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          boardID,
		"name":        req.Name,
		"description": req.Description,
		"invite_code": inviteCode,
		"created_by":  userID,
	})
}

// GetBoard handles GET /api/boards/:id
// Returns a single board — only if the current user is a member.
func (h *BoardsHandler) GetBoard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	// chi.URLParam reads the :id part from the URL path
	boardID := chi.URLParam(r, "id")

	var id, name, role string
	var description *string
	var inviteCode string
	var memberCount int

	err := h.DB.QueryRow(context.Background(), `
		SELECT
			b.id,
			b.name,
			b.description,
			b.invite_code,
			bm.role,
			(SELECT COUNT(*) FROM board_members WHERE board_id = b.id) as member_count
		FROM boards b
		JOIN board_members bm ON bm.board_id = b.id
		WHERE b.id = $1 AND bm.user_id = $2
	`, boardID, userID).Scan(&id, &name, &description, &inviteCode, &role, &memberCount)

	if err != nil {
		// Either board doesn't exist or user is not a member — return 404 either way
		// We don't want to reveal whether the board exists to non-members
		writeError(w, http.StatusNotFound, "board not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"name":         name,
		"description":  description,
		"invite_code":  inviteCode,
		"role":         role,
		"member_count": memberCount,
	})
}

// GetBoardByInviteCode handles GET /api/boards/invite/:code
// Public endpoint — returns basic board info so users can preview before joining.
func (h *BoardsHandler) GetBoardByInviteCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var id, name string
	var description *string
	var memberCount int

	err := h.DB.QueryRow(context.Background(), `
		SELECT
			b.id,
			b.name,
			b.description,
			(SELECT COUNT(*) FROM board_members WHERE board_id = b.id) as member_count
		FROM boards b
		WHERE b.invite_code = $1
	`, code).Scan(&id, &name, &description, &memberCount)

	if err != nil {
		writeError(w, http.StatusNotFound, "invalid invite code")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"name":         name,
		"description":  description,
		"member_count": memberCount,
	})
}