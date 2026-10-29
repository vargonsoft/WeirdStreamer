package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/handlers"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/metadata"
)

// @title Weird Streamer API
// @version 1.0
// @description High-performance Weird Streamer server with authentication
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
	// A stop signal (a restart) cancels ctx: the background jobs stop and the server shuts down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	media.SetDiscStore(db)
	// Users can only be given defined groups; ones from before (or a group removed from
	// config.json by hand) see nothing until moved to one.
	if users, err := db.ListUsers(); err == nil {
		for _, u := range users {
			if !config.GlobalConfig.HasGroup(u.Group) {
				log.Printf("Warning: user %q is in the group %q, which isn't defined in config.json's groups", u.Username, u.Group)
			}
		}
	}

	// Initialize services
	authService := auth.NewService(db)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	mediaHandler := handlers.NewMediaHandler(db, authService)
	mediaHandler.SetDownloads(media.NewDownloadManager(config.GlobalConfig.Media.DownloadPath))
	configHandler := handlers.NewConfigHandler(db)
	userHandler := handlers.NewUserHandler(db, authService)

	// TMDB metadata for "movie" and "tv" libraries, looked up in the background
	metadataService := metadata.NewService(db, mediaHandler.NotifyMetadataReady)
	mediaHandler.SetMetadata(metadataService)
	metadataService.Start(ctx)
	if config.GlobalConfig.Metadata.TMDBAccessToken == "" {
		for _, lib := range config.GlobalConfig.Media.Paths {
			if lib.Type != "" {
				log.Printf("Warning: library %q has type %q but metadata.tmdbAccessToken is not set; lookups are disabled", lib.Name, lib.Type)
			}
		}
	}
	go metadataService.SyncAll(ctx)
	// Intros and credits, for the players' Skip buttons, found in the background.
	mediaHandler.StartSegmentAnalysis(ctx)
	// Records of files that are gone (removed, or in a library no longer configured), forgotten.
	mediaHandler.StartCleanup(ctx)
	// Trickplay frames (the pictures over the seek bars) for videos without them.
	mediaHandler.StartTrickplay(ctx)
	go media.CountStorage() // the data folders' sizes, for Settings → Jobs
	// A token or library change saved through Settings starts a sync without a restart
	configHandler.OnUpdate(func() { go metadataService.SyncAll(ctx) })

	// Setup router
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/api/auth/login", authHandler.Login).Methods("POST")
	r.HandleFunc("/api/auth/logout", authHandler.Logout).Methods("POST")
	// What the apps show for a server before signing in to it
	r.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": config.GlobalConfig.Server.Name})
	}).Methods("GET")
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
	api.HandleFunc("/media/codecs", mediaHandler.GetCodecs).Methods("GET")
	api.HandleFunc("/media/seekpoint", mediaHandler.GetSeekPoint).Methods("GET")
	api.HandleFunc("/media/segments", mediaHandler.GetSegments).Methods("GET")
	api.HandleFunc("/media/subtitle-offset", mediaHandler.SetSubtitleOffset).Methods("PUT")
	api.HandleFunc("/media/segments", mediaHandler.SetSegment).Methods("PUT")
	api.HandleFunc("/library/autoskip", mediaHandler.SetAutoSkip).Methods("PUT")
	api.HandleFunc("/media/subtitles", mediaHandler.GetSubtitles).Methods("GET")
	api.HandleFunc("/media/subtitles/search", mediaHandler.SearchSubtitles).Methods("GET")
	api.HandleFunc("/media/subtitles/download", mediaHandler.DownloadSubtitle).Methods("POST")
	api.HandleFunc("/downloads/options", mediaHandler.DownloadOptions).Methods("GET")
	api.HandleFunc("/downloads", mediaHandler.CreateDownload).Methods("POST")
	api.HandleFunc("/downloads/{id}", mediaHandler.GetDownload).Methods("GET")
	api.HandleFunc("/downloads/{id}", mediaHandler.DeleteDownload).Methods("DELETE")
	api.HandleFunc("/downloads/{id}/file", mediaHandler.DownloadFile).Methods("GET", "HEAD")
	api.HandleFunc("/admin/streams", mediaHandler.ListStreams).Methods("GET")
	api.HandleFunc("/admin/jobs", mediaHandler.ListJobs).Methods("GET")
	api.HandleFunc("/admin/jobs/{id}/{action}", mediaHandler.ControlJob).Methods("POST")
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
	api.HandleFunc("/groups", configHandler.CreateGroup).Methods("POST")
	api.HandleFunc("/groups/{name}", configHandler.RenameGroup).Methods("PUT")
	api.HandleFunc("/groups/{name}", configHandler.DeleteGroup).Methods("DELETE")

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
		// The Android app (web/downloads, from Android/deploy-release.sh --web): installable as a
		// package, not a zip
		mime.AddExtensionType(".apk", "application/vnd.android.package-archive")
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

	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		var err error
		// Check if HTTP/2 is enabled and certificates exist
		if config.GlobalConfig.Streaming.EnableHTTP2 {
			certFile, keyFile := config.GlobalConfig.Server.TLSFiles()

			if _, statErr := os.Stat(certFile); os.IsNotExist(statErr) {
				log.Printf("Warning: %s not found, falling back to HTTP", certFile)
				err = srv.ListenAndServe()
			} else if _, statErr := os.Stat(keyFile); os.IsNotExist(statErr) {
				log.Printf("Warning: %s not found, falling back to HTTP", keyFile)
				err = srv.ListenAndServe()
			} else {
				log.Printf("Using HTTPS/HTTP2")
				err = srv.ListenAndServeTLS(certFile, keyFile)
			}
		} else {
			err = srv.ListenAndServe()
		}
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdown(srv, db)
}

// shutdown stops the server cleanly: requests get a few seconds to finish (streams, which never
// do, are then cut), the ffmpeg runs it started are stopped, and the databases are closed. It
// stays well inside the 10 seconds restart-live.sh waits.
func shutdown(srv *http.Server, db *database.DB) {
	log.Printf("Shutting down")
	wait, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(wait); err != nil {
		srv.Close()
	}
	media.StopChildren(2 * time.Second)
	if err := db.Close(); err != nil {
		log.Printf("Closing the databases: %v", err)
	}
	log.Printf("Stopped")
}
