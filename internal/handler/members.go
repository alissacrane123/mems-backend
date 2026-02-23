package handler

import (
	"context"
	"net/http"
	"encoding/json"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MembersHandler struct {
	DB *pgxpool.Pool
}

func (h *MembersHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	// chi.URLParam reads the :id part from the URL path
	boardID := chi.URLParam(r, "id")

	rows, err := h.DB.Query(context.Background(), `
		SELECT id, user_id, role, joined_at
		FROM board_members
		WHERE board_id = $1
	`, boardID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch members")
		return
	}

	boardMembers := []map[string]any{}

	for rows.Next() {
		var id, userId, role string
		var joinedAt time.Time

		if err := rows.Scan(&id, &userId, &role, &joinedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read member data")
			return
		}

		boardMembers = append(boardMembers, map[string]any{
			"id":        id,
			"user_id":   userId,
			"role":      role,
			"joined_at": joinedAt,
		})
	}

	writeJSON(w, http.StatusOK, boardMembers)
}


func (h *MembersHandler) CountMembers(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")

	var count int

	err := h.DB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM board_members WHERE board_id = $1
	`, boardID).Scan(&count)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count members")
		return
	}	

	writeJSON(w, http.StatusOK, map[string]any{
		"count": count,
	})
}

func (h *MembersHandler) CreateMember(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserID == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "user_id and role are required")
		return
	}

	var id, joinedAt string
	err := h.DB.QueryRow(context.Background(), `
		INSERT INTO board_members (board_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id, joined_at
	`, boardID, req.UserID, req.Role).Scan(&id, &joinedAt)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        id,
		"user_id":   req.UserID,
		"role":      req.Role,
		"joined_at": joinedAt,
	})
}

func (h *MembersHandler) CheckIsMember(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")
	userID := r.URL.Query().Get("user_id")

	var count int
	err := h.DB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM board_members
		WHERE board_id = $1 AND user_id = $2
	`, boardID, userID).Scan(&count)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check membership")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"is_member": count > 0,
	})
}