package handler

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
	"github.com/alissacrane123/mems-backend/internal/storage"
)

type PhotosHandler struct {
	DB *pgxpool.Pool
	S3 *storage.S3Client
}

// UploadPhoto handles POST /api/entries/:entryId/photos
func (h *PhotosHandler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "entryId")
	userID := middleware.GetUserID(r)

	// Parse the multipart form — 10MB max
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	// Get the file from the form
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	// Get the file extension (e.g. "jpg", "png")
	ext := strings.TrimPrefix(filepath.Ext(fileHeader.Filename), ".")
	if ext == "" {
		writeError(w, http.StatusBadRequest, "file must have an extension")
		return
	}

	// Get display_order from form fields
	displayOrder := r.FormValue("display_order")
	if displayOrder == "" {
		displayOrder = "0"
	}

	// Upload to S3
	filePath, publicURL, err := h.S3.UploadFile(userID, entryID, ext, file)
	if err != nil {
		fmt.Println("S3 upload error:", err) // temporary log
		writeError(w, http.StatusInternalServerError, "failed to upload photo")
		return
	}

	// Save record to database
	var photoID string
	err = h.DB.QueryRow(context.Background(), `
		INSERT INTO photos (entry_id, file_path, display_order)
		VALUES ($1, $2, $3)
		RETURNING id
	`, entryID, filePath, displayOrder).Scan(&photoID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save photo record")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         photoID,
		"entry_id":   entryID,
		"file_path":  filePath,
		"public_url": publicURL,
	})
}

// GetPhoto handles GET /api/photos
func (h *PhotosHandler) GetPhoto(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("filePath")

	publicURL := "https://" + r.Host + "/uploads/" + filePath

	writeJSON(w, http.StatusOK, map[string]any{
		"public_url": publicURL,
	})
}
