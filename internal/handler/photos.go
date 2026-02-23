package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alissacrane123/mems-backend/internal/middleware"
)

type PhotosHandler struct {
	DB *pgxpool.Pool
}

// UploadPhoto handles POST /api/entries/:entryId/photos
// Accepts a multipart form file, saves it to disk, and creates a photos record.
func (h *PhotosHandler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	entryID := chi.URLParam(r, "entryId")
	userID := middleware.GetUserID(r)

	// Parse the multipart form — 10MB max as per the spec
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	// Get the file from the form field named "file"
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	// Get display_order from the form fields (optional, defaults to 0)
	displayOrder := r.FormValue("display_order")
	if displayOrder == "" {
		displayOrder = "0"
	}

	// Build the file path: {user_id}/{entry_id}/{timestamp}_{filename}
	timestamp := time.Now().UnixMilli()
	ext := filepath.Ext(fileHeader.Filename)
	fileName := fmt.Sprintf("%d%s", timestamp, ext)
	dirPath := fmt.Sprintf("%s/%s/%s", os.Getenv("UPLOADS_PATH"), userID, entryID)

	// Create the directory if it doesn't exist
	// 0755 is the folder permission (owner can read/write/execute, others can read)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload directory")
		return
	}

	// Create the file on disk
	fullPath := fmt.Sprintf("%s/%s", dirPath, fileName)
	dst, err := os.Create(fullPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file")
		return
	}
	defer dst.Close()

	// Copy the uploaded file content to the destination file
	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save file")
		return
	}

	// Insert a record into the photos table
	storedPath := fmt.Sprintf("%s/%s/%s", userID, entryID, fileName)
	var photoID string
	err = h.DB.QueryRow(context.Background(), `
		INSERT INTO photos (entry_id, file_path, display_order)
		VALUES ($1, $2, $3)
		RETURNING id
	`, entryID, storedPath, displayOrder).Scan(&photoID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save photo record")
		return
	}

	// Build the public URL so the frontend can display the image
	publicURL := fmt.Sprintf("%s/uploads/%s", os.Getenv("PUBLIC_URL"), storedPath)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         photoID,
		"entry_id":   entryID,
		"file_path":  storedPath,
		"public_url": publicURL,
	})
}

// GetPhoto handles GET /api/photos/:filePath
// Returns the public URL for a stored photo.
func (h *PhotosHandler) GetPhoto(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("filePath")
	publicURL := fmt.Sprintf("%s/uploads/%s", os.Getenv("PUBLIC_URL"), filePath)

	writeJSON(w, http.StatusOK, map[string]any{
		"public_url": publicURL,
	})
}