package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

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
		var location *string
		var createdAt time.Time

		if err := rows.Scan(&id, &content, &userID, &location, &createdAt, &createdByName); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read entry data")
			return
		}

		// Fetch photos for this entry
		photoRows, err := h.DB.Query(context.Background(), `
        SELECT file_path FROM photos
        WHERE entry_id = $1
        ORDER BY display_order ASC
    `, id)

		photos := []string{}
		if err == nil {
			defer photoRows.Close()
			for photoRows.Next() {
				var filePath string
				if err := photoRows.Scan(&filePath); err == nil {
					// Build the public S3 URL
					publicURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
						os.Getenv("AWS_BUCKET"),
						os.Getenv("AWS_REGION"),
						filePath,
					)
					photos = append(photos, publicURL)
				}
			}
		}

		entries = append(entries, map[string]any{
			"id":              id,
			"content":         content,
			"created_at":      createdAt.Format(time.RFC3339),
			"user_id":         userID,
			"created_by_name": createdByName,
			"location":        location,
			"photos":          photos,
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *EntriesHandler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "boardId")
	userID := middleware.GetUserID(r)

	var req struct {
		Content   string  `json:"content"`
		Location  *string `json:"location"`
		CreatedAt *string `json:"created_at"` // optional, pointer so it can be nil
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	// Use provided created_at or default to now()
	var createdAtValue string
	if req.CreatedAt != nil {
		createdAtValue = *req.CreatedAt
	} else {
		createdAtValue = "now()"
	}

	var id string
	var createdAt time.Time

	err := h.DB.QueryRow(context.Background(), `
    INSERT INTO entries (user_id, board_id, content, location, created_at)
    VALUES ($1, $2, $3, $4, $5::timestamptz)
    RETURNING id, created_at
`, userID, boardID, req.Content, req.Location, createdAtValue).Scan(&id, &createdAt)

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
