package handler

import (
	"context"
	"net/http"
	"time"
	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
)

type EntriesHandler struct {
	DB *pgxpool.Pool
}

func (h *EntriesHandler) ListEntries(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "boardId")

	rows, err := h.DB.Query(context.Background(), `
		SELECT 
			e.id, 
			e.content, 
			e.user_id, 
			e.location, 
			e.created_at, 
			p.first_name as created_by_name
		FROM entries e
		JOIN profiles p ON p.id = e.user_id
		WHERE e.board_id = $1
		ORDER BY e.created_at DESC
	`, boardID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch entries")
		return
	}

	entries := []map[string]any{}

	for rows.Next() {
		var id, content, userID, createdByName string
		var location *string // location can be null, so we use a pointer
		var createdAt time.Time

		if err := rows.Scan(&id, &content, &userID, &location, &createdAt, &createdByName); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read entry data")
			return
		}

		entries = append(entries, map[string]any{
			"id":              id,
			"content":         content,
			"created_at":      createdAt,
			"user_id":         userID,
			"created_by_name": createdByName,
			"photos":          []string{}, // placeholder for now
			"location":        location,   // will be null in JSON if location is nil
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *EntriesHandler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "boardId")
	userID := middleware.GetUserID(r)

	var req struct {
		Content  string  `json:"content"`
		Location *string `json:"location"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	var id string
	var createdAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		INSERT INTO entries (user_id, board_id, content, location)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, userID, boardID, req.Content, req.Location).Scan(&id, &createdAt)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create entry")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"board_id":   boardID,
		"user_id":    userID,
		"content":    req.Content,
		"location":   req.Location,
		"created_at": createdAt.Format(time.RFC3339),
	})
}