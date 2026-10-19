package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/handlers"
)

// @title Media Streaming Server API
// @version 1.0
// @description High-performance media streaming server with authentication
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@example.com

// @license.name AGPL-3.0-or-later
// @license.url https://www.gnu.org/licenses/agpl-3.0.html

// @host localhost:8080
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.json"
	}

	if err := config.Load(configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure directories exist
	os.MkdirAll(filepath.Dir(config.GlobalConfig.Database.UserPath), 0755)
	os.MkdirAll(filepath.Dir(config.GlobalConfig.Database.MediaPath), 0755)
	os.MkdirAll(config.GlobalConfig.Media.ThumbnailPath, 0755)
	if config.GlobalConfig.Media.TrickplayPath != "" {
		os.MkdirAll(config.GlobalConfig.Media.TrickplayPath, 0755)
	}

	// Initialize database
	db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize services
	authService := auth.NewService(db)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	mediaHandler := handlers.NewMediaHandler(db, authService)
	configHandler := handlers.NewConfigHandler(db)
	userHandler := handlers.NewUserHandler(db)

	// Setup router
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/api/auth/login", authHandler.Login).Methods("POST")
	r.HandleFunc("/api/auth/logout", authHandler.Logout).Methods("POST")
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Protected API routes
	api := r.PathPrefix("/api").Subrouter()
	api.Use(authHandler.AuthMiddleware)

	api.HandleFunc("/media", mediaHandler.ListMedia).Methods("GET")
	api.HandleFunc("/media/browse", mediaHandler.BrowseDirectory).Methods("GET")
	api.HandleFunc("/media/stream", mediaHandler.StreamMedia).Methods("GET")
	api.HandleFunc("/media/metadata", mediaHandler.GetMediaMetadata).Methods("GET")
	api.HandleFunc("/media/scan", mediaHandler.ScanMedia).Methods("POST")
	api.HandleFunc("/media/regenerate-thumbnails", mediaHandler.RegenerateThumbnails).Methods("POST")
	api.HandleFunc("/media/thumbnail-updates", mediaHandler.ThumbnailUpdates).Methods("GET")
	api.HandleFunc("/media/trickplay/{id}", mediaHandler.GetTrickplayInfo).Methods("GET")
	api.HandleFunc("/media/{path}", mediaHandler.GetMediaInfo).Methods("GET")

	// Config endpoints (admin only)
	api.HandleFunc("/config", configHandler.GetConfig).Methods("GET")
	api.HandleFunc("/config", configHandler.UpdateConfig).Methods("PUT")
	api.HandleFunc("/groups", configHandler.GetAvailableGroups).Methods("GET")

	// User management endpoints
	api.HandleFunc("/users", userHandler.ListUsers).Methods("GET")
	api.HandleFunc("/users", userHandler.CreateUser).Methods("POST")
	api.HandleFunc("/users", userHandler.UpdateUser).Methods("PUT")
	api.HandleFunc("/users", userHandler.DeleteUser).Methods("DELETE")
	api.HandleFunc("/users/change-password", userHandler.ChangePassword).Methods("POST")

	// Thumbnails and trickplay frames require login and are checked against the file's library
	thumbs := r.PathPrefix("/thumbnails").Subrouter()
	thumbs.Use(authHandler.AuthMiddleware)
	thumbs.HandleFunc("/{filename:.*}", mediaHandler.ServeThumbnail).Methods("GET", "HEAD")

	trickplay := r.PathPrefix("/trickplay").Subrouter()
	trickplay.Use(authHandler.AuthMiddleware)
	trickplay.HandleFunc("/{hash}/{frame}", mediaHandler.ServeTrickplayFrame).Methods("GET", "HEAD")

	webDir := "./web"
	if _, err := os.Stat(webDir); err == nil {
		// no-cache makes browsers revalidate (a cheap 304) instead of running a stale web UI after an update
		fileServer := http.FileServer(http.Dir(webDir))
		r.PathPrefix("/").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r)
		}))
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", config.GlobalConfig.Server.Host, config.GlobalConfig.Server.Port)
	log.Printf("Starting server on %s", addr)

	// Check if HTTP/2 is enabled and certificates exist
	if config.GlobalConfig.Streaming.EnableHTTP2 {
		certFile := "cert.pem"
		keyFile := "key.pem"

		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			log.Printf("Warning: %s not found, falling back to HTTP", certFile)
			log.Fatal(http.ListenAndServe(addr, r))
		} else if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			log.Printf("Warning: %s not found, falling back to HTTP", keyFile)
			log.Fatal(http.ListenAndServe(addr, r))
		} else {
			log.Printf("Using HTTPS/HTTP2")
			log.Fatal(http.ListenAndServeTLS(addr, certFile, keyFile, r))
		}
	} else {
		log.Fatal(http.ListenAndServe(addr, r))
	}
}
