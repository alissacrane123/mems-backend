package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
)

type NotesHandler struct {
	DB *pgxpool.Pool
}

// ListNotes handles GET /api/notes
func (h *NotesHandler) ListNotes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	folderID := r.URL.Query().Get("folder_id")

	var rows pgx.Rows
	var err error

	if folderID != "" {
		// Return notes in the specified folder
		rows, err = h.DB.Query(context.Background(), `
            SELECT id, title, content, folder_id, created_at, updated_at
            FROM notes
            WHERE user_id = $1 AND folder_id = $2
            ORDER BY updated_at DESC
        `, userID, folderID)
	} else {
		// Return unfiled notes only
		rows, err = h.DB.Query(context.Background(), `
            SELECT id, title, content, folder_id, created_at, updated_at
            FROM notes
            WHERE user_id = $1 AND folder_id IS NULL
            ORDER BY updated_at DESC
        `, userID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch notes")
		return
	}
	defer rows.Close()

	notes := []map[string]any{}

	for rows.Next() {
		var id, title string
		var content, noteFolderID *string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &title, &content, &noteFolderID, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read note data")
			return
		}

		notes = append(notes, map[string]any{
			"id":         id,
			"title":      title,
			"content":    content,
			"folder_id":  noteFolderID,
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
		Title    string  `json:"title"`
		Content  *string `json:"content"`
		FolderID *string `json:"folder_id"`
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
		INSERT INTO notes (user_id, title, content, folder_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at, folder_id
	`, userID, req.Title, req.Content, req.FolderID).Scan(&id, &createdAt, &updatedAt, &req.FolderID)

	if err != nil {
		log.Printf("failed to fetch notes: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create note")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"title":      req.Title,
		"content":    req.Content,
		"folder_id":  req.FolderID,
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
	var folderID *string
	var createdAt, updatedAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		SELECT title, content, created_at, updated_at, folder_id
		FROM notes
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&title, &content, &createdAt, &updatedAt, &folderID)

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
		"folder_id":  folderID,
	})
}

// UpdateNote handles PATCH /api/notes/:id
func (h *NotesHandler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	var req struct {
		Title    *string `json:"title"`
		Content  *string `json:"content"`
		FolderID *string `json:"folder_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var title string
	var content *string
	var updatedAt time.Time
	var folderID *string

	// Only update fields that were provided
	err := h.DB.QueryRow(context.Background(), `
		UPDATE notes
		SET
			title = COALESCE($1, title),
			content = COALESCE($2, content),
			folder_id = COALESCE($3, folder_id)
		WHERE id = $4 AND user_id = $5
		RETURNING title, content, updated_at, folder_id
	`, req.Title, req.Content, req.FolderID, id, userID).Scan(&title, &content, &updatedAt, &folderID)

	if err != nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"title":      title,
		"content":    content,
		"updated_at": updatedAt.Format(time.RFC3339),
		"folder_id":  folderID,
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
