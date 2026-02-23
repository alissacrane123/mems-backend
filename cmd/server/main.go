package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/alissacrane123/mems-backend/internal/handler"
	"github.com/alissacrane123/mems-backend/internal/middleware"
	"github.com/alissacrane123/mems-backend/internal/storage"
)

func main() {
	godotenv.Load()

	db, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	fmt.Println("Connected to database!")

	s3Client, err := storage.NewS3Client()
	if err != nil {
		log.Fatal("Failed to connect to S3:", err)
	}

	authHandler := &handler.AuthHandler{DB: db}

	usersHandler := &handler.UsersHandler{DB: db}
	boardsHandler := &handler.BoardsHandler{DB: db}
	membersHandler := &handler.MembersHandler{DB: db}
	entriesHandler := &handler.EntriesHandler{DB: db}
	photosHandler := &handler.PhotosHandler{DB: db, S3: s3Client}
	notificationsHandler := &handler.NotificationsHandler{DB: db}

	r := chi.NewRouter()

	// CORS middleware — must be added before any routes
	r.Use(cors.Handler(cors.Options{
		// AllowedOrigins is your frontend URL — only this origin can make requests
		AllowedOrigins: []string{os.Getenv("FRONTEND_URL")},

		// AllowedMethods lists which HTTP methods the frontend can use
		AllowedMethods: []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},

		// AllowedHeaders lets the frontend send these headers
		AllowedHeaders: []string{"Accept", "Content-Type", "Authorization"},

		// AllowCredentials must be true for cookies to work cross-origin
		AllowCredentials: true,

		// MaxAge is how long the browser caches the CORS response (in seconds)
		MaxAge: 300,
	}))

	// Health check — public, no auth needed
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Public auth routes — no middleware
	r.Post("/api/auth/signup", authHandler.Signup)
	r.Post("/api/auth/signin", authHandler.Signin)
	r.Get("/api/boards/invite/{code}", boardsHandler.GetBoardByInviteCode)
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))

	// Protected routes — middleware runs first on all of these
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		// auth
		r.Post("/api/auth/signout", authHandler.Signout)
		r.Get("/api/auth/session", authHandler.Session)

		// users
		r.Get("/api/users/me", usersHandler.Me)
		r.Post("/api/users/lookup-by-email", usersHandler.LookupByEmail)

		// boards
		r.Get("/api/boards", boardsHandler.ListBoards)
		r.Post("/api/boards", boardsHandler.CreateBoard)
		r.Get("/api/boards/{id}", boardsHandler.GetBoard)

		// members
		r.Get("/api/boards/{id}/members", membersHandler.ListMembers)
		r.Get("/api/boards/{id}/members/count", membersHandler.CountMembers)
		r.Post("/api/boards/{id}/members", membersHandler.CreateMember)
		r.Get("/api/boards/{id}/members/check", membersHandler.CheckIsMember)
		r.Get("/api/boards/{boardId}/entries", entriesHandler.ListEntries)
		r.Post("/api/boards/{boardId}/entries", entriesHandler.CreateEntry)

		// photos
		r.Post("/api/entries/{entryId}/photos", photosHandler.UploadPhoto)
		r.Get("/api/photos", photosHandler.GetPhoto)

		// notifications
		r.Get("/api/notifications", notificationsHandler.ListNotifications)
		r.Post("/api/notifications", notificationsHandler.CreateNotification)
		r.Patch("/api/notifications/read-all", notificationsHandler.MarkAllAsRead)
		r.Patch("/api/notifications/{id}/read", notificationsHandler.MarkAsRead)
		r.Post("/api/notifications/{id}/accept", notificationsHandler.AcceptInvite)
		r.Post("/api/notifications/{id}/decline", notificationsHandler.DeclineInvite)
		r.Get("/api/notifications/check-invite", notificationsHandler.CheckInvite)
	})

	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", r)
}
