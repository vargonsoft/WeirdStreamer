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
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/backup"
	"github.com/weirdstreamer/server/internal/clientip"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/handlers"
	"github.com/weirdstreamer/server/internal/logs"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/metadata"
	"github.com/weirdstreamer/server/internal/tlscert"
)

// @title Weird Streamer API
// @version 1.0
// @description High-performance Weird Streamer server with authentication
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@example.com

// @license.name AGPL-3.0-or-later
// @license.url https://www.gnu.org/licenses/agpl-3.0.html

// @host localhost:9000
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// version is the release's, set at build time (-ldflags "-X main.version=..."); "dev" otherwise.
var version = "dev"

const serverUsage = `Usage: server [-h | --help | --version]

Runs the WeirdStreamer server with the config in ./config.yml (or the CONFIG_PATH
environment variable). Its command-line options:
  -h, --help     show this help and exit
  --version      show the version and exit

The admin commands are in the cli program (run it with no arguments to list them).
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Print(serverUsage)
			os.Exit(0)
		case "--version", "-version":
			fmt.Println("WeirdStreamer server", version)
			os.Exit(0)
		default:
			fmt.Fprint(os.Stderr, serverUsage)
			os.Exit(2)
		}
	}
	privateUmask()
	media.UseBundledTools()
	// A stop signal (a restart) cancels ctx: the background jobs stop and the server shuts down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yml"
	}

	if err := config.Load(configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := config.GlobalConfig.CheckJWTSecret(); err != nil {
		log.Fatalf("Refusing to start: %v. Set it in %s to a long random string, for example the output of: openssl rand -hex 32", err, configPath)
	}
	clientip.SetTrusted(config.GlobalConfig.Server.TrustedProxies)
	// auth.log and access.log, beside live.sh's server-*.log
	if err := logs.Open(config.GlobalConfig.Server.LogsPath); err != nil {
		log.Printf("Warning: could not open the auth and access logs: %v", err)
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
	if config.GlobalConfig.Media.TranscodeCachePath == "" {
		config.GlobalConfig.Media.TranscodeCachePath = filepath.Join(filepath.Dir(filepath.Clean(config.GlobalConfig.Media.ThumbnailPath)), "transcodes")
	}
	media.DetectHardware() // GPUs and their encoders, tried in the background for Settings
	if err := media.SetTranscodeCacheDir(config.GlobalConfig.Media.TranscodeCachePath); err != nil {
		log.Printf("Warning: could not prepare the transcode cache: %v", err)
	}
	if config.GlobalConfig.Media.DownloadPath == "" {
		config.GlobalConfig.Media.DownloadPath = filepath.Join(filepath.Dir(filepath.Clean(config.GlobalConfig.Media.ThumbnailPath)), "downloads")
	}
	if config.GlobalConfig.Media.TempPath == "" {
		config.GlobalConfig.Media.TempPath = filepath.Join(filepath.Dir(filepath.Clean(config.GlobalConfig.Media.ThumbnailPath)), "tmp")
	}
	// Fixed copies of damaged videos, and the originals they replaced: beside the thumbnails by default
	if config.GlobalConfig.Media.RepairPath == "" {
		config.GlobalConfig.Media.RepairPath = filepath.Join(filepath.Dir(filepath.Clean(config.GlobalConfig.Media.ThumbnailPath)), "repairs")
	}
	if config.GlobalConfig.Media.TrashPath == "" {
		config.GlobalConfig.Media.TrashPath = filepath.Join(filepath.Dir(filepath.Clean(config.GlobalConfig.Media.ThumbnailPath)), "trash")
	}
	if err := media.SetTempDir(config.GlobalConfig.Media.TempPath); err != nil {
		log.Printf("Warning: could not prepare the temp folder %s: %v", config.GlobalConfig.Media.TempPath, err)
	}
	keepDataPrivate(config.GlobalConfig)
	if err := handlers.WriteLocalKey(filepath.Join(filepath.Dir(configPath), "local.key")); err != nil {
		log.Printf("Warning: could not write local.key (scripts/restart-live.sh can't see the streams): %v", err)
	}

	// Initialize database
	db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	media.SetDiscStore(db)
	// Users can only be given defined groups; ones from before (or a group removed from
	// the config by hand) see nothing until moved to one.
	if users, err := db.ListUsers(); err == nil {
		for _, u := range users {
			if !config.GlobalConfig.HasGroup(u.Group) {
				log.Printf("Warning: user %q is in the group %q, which isn't defined in the config's groups", u.Username, u.Group)
			}
		}
	}

	// Initialize services
	authService := auth.NewService(db)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	pairingHandler := handlers.NewPairingHandler(authService)
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
	// Videos' styled (ASS/SSA) subtitle tracks, copied out of them for whole windows and downloads.
	mediaHandler.StartAssCache(ctx)
	mediaHandler.StartRepairs(ctx)
	// New files, found every hour (and on Scan Media).
	go mediaHandler.StartScans(ctx)
	backup.Start(ctx, db, version) // on the schedule Settings sets
	go media.CountStorage()        // the data folders' sizes, for Settings → Jobs
	// When files arrived, for Recently Added: their time on disk (new ones are dated as they're saved).
	go func() {
		if err := db.DateMediaAdded(); err != nil {
			log.Printf("Warning: could not date the media files: %v", err)
		}
	}()
	// A token or library change saved through Settings starts a sync without a restart
	configHandler.OnUpdate(func() { go metadataService.SyncAll(ctx) })

	// Setup router
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/api/auth/login", authHandler.Login).Methods("POST")
	r.HandleFunc("/api/auth/logout", authHandler.Logout).Methods("POST")
	// TV sign-in with a short code: the TV makes one and polls it until a user claims it
	r.HandleFunc("/api/auth/pairing", pairingHandler.Create).Methods("POST")
	r.HandleFunc("/api/auth/pairing/{code}", pairingHandler.Poll).Methods("GET")
	// What the apps show for a server before signing in to it
	r.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": config.GlobalConfig.Server.Name, "version": version})
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

	// A signed-in user enters a TV's code, and the TV signs in as them
	api.HandleFunc("/pairing/claim", pairingHandler.Claim).Methods("POST")

	api.HandleFunc("/media", mediaHandler.ListMedia).Methods("GET")
	api.HandleFunc("/media/browse", mediaHandler.BrowseDirectory).Methods("GET")
	api.HandleFunc("/media/stream", mediaHandler.StreamMedia).Methods("GET")
	api.HandleFunc("/media/metadata", mediaHandler.GetMediaMetadata).Methods("GET")
	api.HandleFunc("/media/details", mediaHandler.GetMediaDetails).Methods("GET")
	api.HandleFunc("/library", mediaHandler.GetLibrary).Methods("GET")
	api.HandleFunc("/library/title", mediaHandler.GetLibraryTitle).Methods("GET")
	api.HandleFunc("/library/new", mediaHandler.NewAdditions).Methods("GET")
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
	api.HandleFunc("/media/upcoming", mediaHandler.Upcoming).Methods("GET")
	api.HandleFunc("/media/subtitle-offset", mediaHandler.SetSubtitleOffset).Methods("PUT")
	api.HandleFunc("/media/segments", mediaHandler.SetSegment).Methods("PUT")
	api.HandleFunc("/library/autoskip", mediaHandler.SetAutoSkip).Methods("PUT")
	api.HandleFunc("/media/subtitles", mediaHandler.GetSubtitles).Methods("GET")
	api.HandleFunc("/media/font", mediaHandler.GetFont).Methods("GET")
	api.HandleFunc("/media/subtitle-font", mediaHandler.GetSubtitleFont).Methods("POST")
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
	api.HandleFunc("/media/trickplay", mediaHandler.GetTrickplayInfoByPath).Methods("GET")
	api.HandleFunc("/media/{path}", mediaHandler.GetMediaInfo).Methods("GET")

	// Config endpoints (admin only)
	api.HandleFunc("/config", configHandler.GetConfig).Methods("GET")
	api.HandleFunc("/config", configHandler.UpdateConfig).Methods("PUT")
	api.HandleFunc("/config/folders", configHandler.ListFolders).Methods("GET")
	api.HandleFunc("/admin/certificate", configHandler.Certificate).Methods("GET")
	api.HandleFunc("/admin/hardware", configHandler.Hardware).Methods("GET")
	api.HandleFunc("/admin/backups", configHandler.Backups).Methods("GET")
	api.HandleFunc("/admin/backups", configHandler.BackUpNow).Methods("POST")
	api.HandleFunc("/admin/damaged", mediaHandler.DamagedVideos).Methods("GET")
	api.HandleFunc("/admin/damaged/replace", mediaHandler.ReplaceDamaged).Methods("POST")
	api.HandleFunc("/admin/damaged/fix", mediaHandler.FixDamaged).Methods("POST")
	api.HandleFunc("/admin/damaged/play", mediaHandler.PlayDamaged).Methods("POST")
	api.HandleFunc("/admin/trash/empty", mediaHandler.EmptyTrash).Methods("POST")
	api.HandleFunc("/groups", configHandler.GetAvailableGroups).Methods("GET")
	api.HandleFunc("/groups", configHandler.CreateGroup).Methods("POST")
	api.HandleFunc("/groups/{name}", configHandler.RenameGroup).Methods("PUT")
	api.HandleFunc("/groups/{name}/default", configHandler.MakeDefaultGroup).Methods("PUT")
	api.HandleFunc("/groups/{name}", configHandler.DeleteGroup).Methods("DELETE")

	// User management endpoints
	api.HandleFunc("/users", userHandler.ListUsers).Methods("GET")
	api.HandleFunc("/users", userHandler.CreateUser).Methods("POST")
	api.HandleFunc("/users", userHandler.UpdateUser).Methods("PUT")
	api.HandleFunc("/users", userHandler.DeleteUser).Methods("DELETE")
	api.HandleFunc("/users/change-password", userHandler.ChangePassword).Methods("POST")
	api.HandleFunc("/account", userHandler.GetAccount).Methods("GET")
	api.HandleFunc("/account", userHandler.UpdateAccount).Methods("PUT")

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
		// The app (20 MB) only for those signed in: anyone else fetching it over and over would use
		// up the server's upload. Signed-in pages and the apps' update checks fetch it with their token.
		r.PathPrefix("/downloads/").Handler(authHandler.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r)
		})))
		r.PathPrefix("/").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r)
		}))
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", config.GlobalConfig.Server.Host, config.GlobalConfig.Server.Port)
	log.Printf("Starting server on %s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: logs.Access(securityHeaders(limitBodies(r))),
		// A client that connects and never finishes its request's headers, or leaves a connection
		// open doing nothing, doesn't hold it for good. (No limit on a response: streams are long.)
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
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
				// Warned about in the log and Settings from 30 days before it expires
				if notAfter, certErr := tlscert.NotAfter(certFile); certErr == nil {
					tlscert.Watch(ctx, certFile, notAfter)
				} else {
					log.Printf("[TLS] Can't read %s's expiry: %v", certFile, certErr)
				}
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

// maxRequestBody is the most a request may send. The largest the apps send (Settings, a phone's
// progress played offline) are a few hundred KB; nothing is uploaded.
const maxRequestBody = 8 << 20

// securityHeaders sets the browser protections on every response: the content policy (below),
// no framing by other sites, where a hidden page could lead a signed-in user into clicks they
// didn't mean (clickjacking; X-Frame-Options is for older browsers), no guessing a file's type
// from its contents (nosniff), and no page address sent to other sites.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy := contentPolicy
		if strings.HasPrefix(r.URL.Path, "/vendor/jassub/") {
			policy = jassubPolicy
		}
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// contentPolicy lets the web UI load and run only the server's own files: no inline or other
// sites' scripts, so text slipped into a page can't run. Styles may be inline (the pages app.js
// builds have style attributes); images and media may also be blob: and data: URLs. WebAssembly
// (JASSUB's libass) is allowed, and no site may frame the pages.
const contentPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; " +
	"font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
	"form-action 'self'; frame-ancestors 'none'"

// jassubPolicy is for JASSUB's files: its worker (which follows its own script's policy) builds
// functions with new Function (Emscripten's embind), so it may also evaluate code.
const jassubPolicy = "default-src 'self'; script-src 'self' 'unsafe-eval' 'wasm-unsafe-eval'; " +
	"object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

// limitBodies refuses request bodies past maxRequestBody, so no one (signing in needs no account)
// can make the server hold an endless one in memory.
func limitBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		next.ServeHTTP(w, r)
	})
}
