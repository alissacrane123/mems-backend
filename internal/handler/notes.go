package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
)

type NotesHandler struct {
	DB *pgxpool.Pool
}

// ListNotes handles GET /api/notes
func (h *NotesHandler) ListNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := h.DB.Query(context.Background(), `
		SELECT id, title, content, created_at, updated_at
		FROM notes
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch notes")
		return
	}
	defer rows.Close()

	notes := []map[string]any{}

	for rows.Next() {
		var id, title string
		var content *string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &title, &content, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read note data")
			return
		}

		notes = append(notes, map[string]any{
			"id":         id,
			"title":      title,
			"content":    content,
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, notes)
}

// CreateNote handles POST /api/notes
func (h *NotesHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Title   string  `json:"title"`
		Content *string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Default title if not provided
	if req.Title == "" {
		req.Title = "Untitled Note"
	}

	var id string
	var createdAt, updatedAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		INSERT INTO notes (user_id, title, content)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`, userID, req.Title, req.Content).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create note")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"title":      req.Title,
		"content":    req.Content,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	})
}

// GetNote handles GET /api/notes/:id
func (h *NotesHandler) GetNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	var title string
	var content *string
	var createdAt, updatedAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		SELECT title, content, created_at, updated_at
		FROM notes
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&title, &content, &createdAt, &updatedAt)

	if err != nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"title":      title,
		"content":    content,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	})
}

// UpdateNote handles PATCH /api/notes/:id
func (h *NotesHandler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	var req struct {
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var title string
	var content *string
	var updatedAt time.Time

	// Only update fields that were provided
	err := h.DB.QueryRow(context.Background(), `
		UPDATE notes
		SET
			title = COALESCE($1, title),
			content = COALESCE($2, content)
		WHERE id = $3 AND user_id = $4
		RETURNING title, content, updated_at
	`, req.Title, req.Content, id, userID).Scan(&title, &content, &updatedAt)

	if err != nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"title":      title,
		"content":    content,
		"updated_at": updatedAt.Format(time.RFC3339),
	})
}

// DeleteNote handles DELETE /api/notes/:id
func (h *NotesHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	result, err := h.DB.Exec(context.Background(), `
		DELETE FROM notes
		WHERE id = $1 AND user_id = $2
	`, id, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete note")
		return
	}

	// RowsAffected tells us if the note actually existed and was deleted
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}