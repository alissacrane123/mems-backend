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

type NotificationsHandler struct {
	DB *pgxpool.Pool
}

// ListNotifications handles GET /api/notifications
func (h *NotificationsHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	rows, err := h.DB.Query(context.Background(), `
		SELECT id, type, is_read, data, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch notifications")
		return
	}
	defer rows.Close()

	notifications := []map[string]any{}

	for rows.Next() {
		var id, notifType string
		var isRead bool
		var data []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &notifType, &isRead, &data, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read notification data")
			return
		}

		notifications = append(notifications, map[string]any{
			"id":         id,
			"type":       notifType,
			"is_read":    isRead,
			"data":       json.RawMessage(data), // RawMessage prevents double encoding
			"created_at": createdAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, notifications)
}

// CreateNotification handles POST /api/notifications
func (h *NotificationsHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string          `json:"user_id"`
		Type   string          `json:"type"`
		Data   json.RawMessage `json:"data"` // RawMessage lets us pass JSONB directly
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserID == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "user_id and type are required")
		return
	}

	var id string
	var createdAt time.Time

	err := h.DB.QueryRow(context.Background(), `
		INSERT INTO notifications (user_id, type, data)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`, req.UserID, req.Type, []byte(req.Data)).Scan(&id, &createdAt)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create notification")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"type":       req.Type,
		"created_at": createdAt.Format(time.RFC3339),
	})
}

// MarkAsRead handles PATCH /api/notifications/:id/read
func (h *NotificationsHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	_, err := h.DB.Exec(context.Background(), `
		UPDATE notifications
		SET is_read = true
		WHERE id = $1 AND user_id = $2
	`, id, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark notification as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// MarkAllAsRead handles PATCH /api/notifications/read-all
func (h *NotificationsHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	_, err := h.DB.Exec(context.Background(), `
		UPDATE notifications
		SET is_read = true
		WHERE user_id = $1 AND is_read = false
	`, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark all notifications as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// AcceptInvite handles POST /api/notifications/:id/accept
func (h *NotificationsHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	var req struct {
		BoardID string `json:"board_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Check if already a member
	var count int
	err := h.DB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM board_members
		WHERE board_id = $1 AND user_id = $2
	`, req.BoardID, userID).Scan(&count)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check membership")
		return
	}

	// Only add if not already a member
	if count == 0 {
		_, err = h.DB.Exec(context.Background(), `
			INSERT INTO board_members (board_id, user_id, role)
			VALUES ($1, $2, 'member')
		`, req.BoardID, userID)

		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to join board")
			return
		}
	}

	// Mark notification as read
	_, err = h.DB.Exec(context.Background(), `
		UPDATE notifications SET is_read = true WHERE id = $1
	`, id)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update notification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// DeclineInvite handles POST /api/notifications/:id/decline
func (h *NotificationsHandler) DeclineInvite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r)

	_, err := h.DB.Exec(context.Background(), `
		UPDATE notifications SET is_read = true
		WHERE id = $1 AND user_id = $2
	`, id, userID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decline invite")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// CheckInvite handles GET /api/notifications/check-invite
func (h *NotificationsHandler) CheckInvite(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	boardID := r.URL.Query().Get("board_id")

	var count int
	err := h.DB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM notifications
		WHERE user_id = $1
		AND is_read = false
		AND type = 'board_invitation'
		AND data->>'board_id' = $2
	`, userID, boardID).Scan(&count)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check invite")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"exists": count > 0})
}