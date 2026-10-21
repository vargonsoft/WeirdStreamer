package main

import (
	"context"
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
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/metadata"
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
	if config.GlobalConfig.Media.ArtworkPath == "" {
		config.GlobalConfig.Media.ArtworkPath = "./artwork"
	}
	os.MkdirAll(config.GlobalConfig.Media.ArtworkPath, 0755)
	// Encodes for offline downloads; by default next to the thumbnails, not in /tmp (often RAM-backed).
	if config.GlobalConfig.Media.DownloadPath == "" {
		config.GlobalConfig.Media.DownloadPath = filepath.Join(filepath.Dir(filepath.Clean(config.GlobalConfig.Media.ThumbnailPath)), "downloads")
	}

	// Initialize database
	db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	media.SetDiscStore(db)

	// Initialize services
	authService := auth.NewService(db)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	mediaHandler := handlers.NewMediaHandler(db, authService)
	mediaHandler.SetDownloads(media.NewDownloadManager(config.GlobalConfig.Media.DownloadPath))
	configHandler := handlers.NewConfigHandler(db)
	userHandler := handlers.NewUserHandler(db)

	// TMDB metadata for "movie" and "tv" libraries, looked up in the background
	metadataService := metadata.NewService(db, mediaHandler.NotifyMetadataReady)
	mediaHandler.SetMetadata(metadataService)
	metadataService.Start(context.Background())
	if config.GlobalConfig.Metadata.TMDBAccessToken == "" {
		for _, lib := range config.GlobalConfig.Media.Paths {
			if lib.Type != "" {
				log.Printf("Warning: library %q has type %q but metadata.tmdbAccessToken is not set; lookups are disabled", lib.Name, lib.Type)
			}
		}
	}
	go metadataService.SyncAll(context.Background())
	// A token or library change saved through Settings starts a sync without a restart
	configHandler.OnUpdate(func() { go metadataService.SyncAll(context.Background()) })

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
	// Active stream count for tools on this machine (loopback only; no login, no file names)
	r.HandleFunc("/local/streams", mediaHandler.LocalStreamSummary).Methods("GET")

	api := r.PathPrefix("/api").Subrouter()
	api.Use(authHandler.AuthMiddleware)

	api.HandleFunc("/media", mediaHandler.ListMedia).Methods("GET")
	api.HandleFunc("/media/browse", mediaHandler.BrowseDirectory).Methods("GET")
	api.HandleFunc("/media/stream", mediaHandler.StreamMedia).Methods("GET")
	api.HandleFunc("/media/metadata", mediaHandler.GetMediaMetadata).Methods("GET")
	api.HandleFunc("/media/details", mediaHandler.GetMediaDetails).Methods("GET")
	api.HandleFunc("/library", mediaHandler.GetLibrary).Methods("GET")
	api.HandleFunc("/library/title", mediaHandler.GetLibraryTitle).Methods("GET")
	api.HandleFunc("/playback/watched", mediaHandler.SetWatched).Methods("POST")
	api.HandleFunc("/playback/sync", mediaHandler.SyncPlayback).Methods("POST")
	api.HandleFunc("/playback/continue", mediaHandler.ContinueWatching).Methods("GET")
	api.HandleFunc("/playback/continue", mediaHandler.RemoveFromContinue).Methods("DELETE")
	api.HandleFunc("/media/match", mediaHandler.GetMatch).Methods("GET")
	api.HandleFunc("/media/match", mediaHandler.SetMatch).Methods("PUT")
	api.HandleFunc("/media/match", mediaHandler.ResetMatch).Methods("DELETE")
	api.HandleFunc("/streams/progress", mediaHandler.ReportStreamProgress).Methods("POST")
	api.HandleFunc("/media/disc", mediaHandler.GetDisc).Methods("GET")
	api.HandleFunc("/downloads/options", mediaHandler.DownloadOptions).Methods("GET")
	api.HandleFunc("/downloads", mediaHandler.CreateDownload).Methods("POST")
	api.HandleFunc("/downloads/{id}", mediaHandler.GetDownload).Methods("GET")
	api.HandleFunc("/downloads/{id}", mediaHandler.DeleteDownload).Methods("DELETE")
	api.HandleFunc("/downloads/{id}/file", mediaHandler.DownloadFile).Methods("GET", "HEAD")
	api.HandleFunc("/admin/streams", mediaHandler.ListStreams).Methods("GET")
	api.HandleFunc("/media/match/search", mediaHandler.SearchMatch).Methods("GET")
	api.HandleFunc("/media/match/posters", mediaHandler.GetMatchPosters).Methods("GET")
	api.HandleFunc("/media/match/poster", mediaHandler.SetMatchPoster).Methods("PUT")
	api.HandleFunc("/media/match/seasons", mediaHandler.MatchSeasons).Methods("GET")
	api.HandleFunc("/media/match/episodes", mediaHandler.MatchEpisodes).Methods("GET")
	api.HandleFunc("/media/match/episode", mediaHandler.SetEpisodeMapping).Methods("PUT")
	api.HandleFunc("/media/match/episode", mediaHandler.ClearEpisodeMapping).Methods("DELETE")
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

	// Downloaded TMDB artwork: /artwork/{movie|tv}/{id}/{file} and /artwork/person/{file}
	artwork := r.PathPrefix("/artwork").Subrouter()
	artwork.Use(authHandler.AuthMiddleware)
	artwork.HandleFunc("/{kind:movie|tv}/{id:[0-9]+}/{file}", mediaHandler.ServeArtwork).Methods("GET", "HEAD")
	artwork.HandleFunc("/{kind:person}/{file}", mediaHandler.ServeArtwork).Methods("GET", "HEAD")

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
