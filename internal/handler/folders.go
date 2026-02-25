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

type FoldersHandler struct {
	DB *pgxpool.Pool
}

// ListFolders handles GET /api/folders
// Returns all folders for the current user, structured as a flat list.
// The frontend can use parent_id to build the tree structure.
func (h *FoldersHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := h.DB.Query(context.Background(), `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders
		WHERE user_id = $1
		ORDER BY name ASC
	`, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch folders")
		return
	}
	defer rows.Close()

	folders := []map[string]any{}

	for rows.Next() {
		var id, name string
		var parentID *string // pointer because parent_id can be null (root folder)
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &parentID, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read folder data")
			return
		}

		folders = append(folders, map[string]any{
			"id":         id,
			"name":       name,
			"parent_id":  parentID,
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, folders)
}

// CreateFolder handles POST /api/folders
func (h *FoldersHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"` // optional — nil means root folder
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var id string
	var createdAt, updatedAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		INSERT INTO folders (user_id, name, parent_id)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`, userID, req.Name, req.ParentID).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create folder")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"name":       req.Name,
		"parent_id":  req.ParentID,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	})
}

// GetFolder handles GET /api/folders/:id
// Returns a folder and its direct notes and subfolders.
func (h *FoldersHandler) GetFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	// Get the folder itself
	var name string
	var parentID *string
	var createdAt, updatedAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		SELECT name, parent_id, created_at, updated_at
		FROM folders
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&name, &parentID, &createdAt, &updatedAt)

	if err != nil {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}

	// Get direct subfolders
	subRows, err := h.DB.Query(context.Background(), `
		SELECT id, name, parent_id, created_at, updated_at
		FROM folders
		WHERE parent_id = $1 AND user_id = $2
		ORDER BY name ASC
	`, id, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch subfolders")
		return
	}
	defer subRows.Close()

	subfolders := []map[string]any{}
	for subRows.Next() {
		var subID, subName string
		var subParentID *string
		var subCreatedAt, subUpdatedAt time.Time

		if err := subRows.Scan(&subID, &subName, &subParentID, &subCreatedAt, &subUpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read subfolder data")
			return
		}

		subfolders = append(subfolders, map[string]any{
			"id":         subID,
			"name":       subName,
			"parent_id":  subParentID,
			"created_at": subCreatedAt.Format(time.RFC3339),
			"updated_at": subUpdatedAt.Format(time.RFC3339),
		})
	}

	// Get direct notes in this folder
	noteRows, err := h.DB.Query(context.Background(), `
		SELECT id, title, content, created_at, updated_at
		FROM notes
		WHERE folder_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
	`, id, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch notes")
		return
	}
	defer noteRows.Close()

	notes := []map[string]any{}
	for noteRows.Next() {
		var noteID, title string
		var content *string
		var noteCreatedAt, noteUpdatedAt time.Time

		if err := noteRows.Scan(&noteID, &title, &content, &noteCreatedAt, &noteUpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read note data")
			return
		}

		notes = append(notes, map[string]any{
			"id":         noteID,
			"title":      title,
			"content":    content,
			"created_at": noteCreatedAt.Format(time.RFC3339),
			"updated_at": noteUpdatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"name":       name,
		"parent_id":  parentID,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
		"subfolders": subfolders,
		"notes":      notes,
	})
}

// RenameFolder handles PATCH /api/folders/:id
func (h *FoldersHandler) RenameFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	var req struct {
		Name     *string `json:"name"`
		ParentID *string `json:"parent_id"` // allows moving folder to a new parent
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var name string
	var parentID *string
	var updatedAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		UPDATE folders
		SET
			name = COALESCE($1, name),
			parent_id = COALESCE($2, parent_id)
		WHERE id = $3 AND user_id = $4
		RETURNING name, parent_id, updated_at
	`, req.Name, req.ParentID, id, userID).Scan(&name, &parentID, &updatedAt)

	if err != nil {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"name":       name,
		"parent_id":  parentID,
		"updated_at": updatedAt.Format(time.RFC3339),
	})
}

// DeleteFolder handles DELETE /api/folders/:id
// mode=delete — deletes folder, all subfolders, and all notes recursively
// mode=move-up — moves contents up one level then deletes the folder
func (h *FoldersHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)
	mode := r.URL.Query().Get("mode")

	if mode == "" {
		mode = "delete" // default to hard delete
	}

	// First verify the folder exists and belongs to the user
	var parentID *string
	err := h.DB.QueryRow(context.Background(), `
		SELECT parent_id FROM folders WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&parentID)

	if err != nil {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}

	if mode == "move-up" {
		// Use a recursive CTE to find ALL descendant folder IDs
		// This handles unlimited nesting depth
		rows, err := h.DB.Query(context.Background(), `
			WITH RECURSIVE descendants AS (
				SELECT id FROM folders WHERE id = $1
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN descendants d ON f.parent_id = d.id
			)
			SELECT id FROM descendants WHERE id != $1
		`, id)

		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to find subfolders")
			return
		}
		defer rows.Close()

		// Collect all descendant folder IDs
		var descendantIDs []string
		for rows.Next() {
			var descID string
			if err := rows.Scan(&descID); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to read subfolder data")
				return
			}
			descendantIDs = append(descendantIDs, descID)
		}

		// Move all notes in this folder and all subfolders to parent (or root if no parent)
		_, err = h.DB.Exec(context.Background(), `
			UPDATE notes
			SET folder_id = $1
			WHERE folder_id = $2 OR folder_id = ANY($3::uuid[])
		`, parentID, id, descendantIDs)

		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to move notes")
			return
		}

		// Move direct subfolders up to parent (or root)
		_, err = h.DB.Exec(context.Background(), `
			UPDATE folders SET parent_id = $1 WHERE parent_id = $2 AND user_id = $3
		`, parentID, id, userID)

		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to move subfolders")
			return
		}
	}

	// Delete the folder — in delete mode the CASCADE handles subfolders and notes
	// In move-up mode the folder is now empty so this just deletes the folder itself
	_, err = h.DB.Exec(context.Background(), `
		DELETE FROM folders WHERE id = $1 AND user_id = $2
	`, id, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete folder")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}