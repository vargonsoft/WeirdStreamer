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
	mediaHandler := handlers.NewMediaHandler(db)
	configHandler := handlers.NewConfigHandler()
	userHandler := handlers.NewUserHandler(db)

	// Setup router
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/api/auth/login", authHandler.Login).Methods("POST")
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
	api.HandleFunc("/media/{path}", mediaHandler.GetMediaInfo).Methods("GET")

	// Config endpoints (admin only)
	api.HandleFunc("/config", configHandler.GetConfig).Methods("GET")
	api.HandleFunc("/config", configHandler.UpdateConfig).Methods("PUT")

	// User management endpoints
	api.HandleFunc("/users", userHandler.ListUsers).Methods("GET")
	api.HandleFunc("/users", userHandler.CreateUser).Methods("POST")
	api.HandleFunc("/users", userHandler.DeleteUser).Methods("DELETE")
	api.HandleFunc("/users/change-password", userHandler.ChangePassword).Methods("POST")

	// Thumbnail serving (no auth required for thumbnails)
	r.HandleFunc("/thumbnails/{filename:.*}", mediaHandler.ServeThumbnail).Methods("GET")

	// Serve web client
	webDir := "./web"
	if _, err := os.Stat(webDir); err == nil {
		r.PathPrefix("/").Handler(http.FileServer(http.Dir(webDir)))
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
