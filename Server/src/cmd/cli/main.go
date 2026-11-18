package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
	"golang.org/x/term"
)

// version is the release's, set at build time (-ldflags "-X main.version=..."); "dev" otherwise.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			os.Exit(0)
		case "--version", "-version":
			fmt.Println("WeirdStreamer cli", version)
			os.Exit(0)
		}
	}
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yml"
	}
	// Writes the config, so there's none to load yet.
	if len(os.Args) >= 3 && os.Args[1] == "config" && os.Args[2] == "init" {
		initConfig(os.Args[3:], configPath)
		return
	}

	if err := config.Load(configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	// Ensure database directory exists
	userDBDir := filepath.Dir(config.GlobalConfig.Database.UserPath)
	if err := os.MkdirAll(userDBDir, 0755); err != nil {
		log.Fatalf("Failed to create user database directory: %v", err)
	}

	mediaDBDir := filepath.Dir(config.GlobalConfig.Database.MediaPath)
	if err := os.MkdirAll(mediaDBDir, 0755); err != nil {
		log.Fatalf("Failed to create media database directory: %v", err)
	}

	// Initialize database
	db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	command := os.Args[1]

	switch command {
	case "user":
		handleUserCommand(db, os.Args[2:])
	case "scan":
		handleScanCommand(db)
	case "config":
		handleConfigCommand(os.Args[2:])
	case "trickplay":
		handleTrickplayCommand(db, os.Args[2:])
	case "cleanup":
		handleCleanupCommand(db, os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Media Server CLI")
	fmt.Println("\nUsage:")
	fmt.Println("  cli user create <username> [--group <group>] [--admin] - Create a new user (default: the first group)")
	fmt.Println("  cli user list                                          - List all users")
	fmt.Println("  cli user delete <username>                             - Delete a user")
	fmt.Println("  cli user change-password <username>                    - Change user password")
	fmt.Println("  cli user set-group <username> <group>                  - Set user group")
	fmt.Println("  cli scan                                               - Scan media directories")
	fmt.Println("  cli trickplay [options]                                - Generate trickplay frames")
	fmt.Println("      --video <path>                                     - Generate for a specific video")
	fmt.Println("      --dir <path>                                       - Generate for all videos in a directory")
	fmt.Println("      --library <name>                                   - Generate for a specific media library")
	fmt.Println("      (no options)                                       - Generate for all videos in database")
	fmt.Println("  cli trickplay clean                                    - Remove trickplay images for non-existent paths")
	fmt.Println("  cli cleanup [--force]                                  - Forget files that are gone (thumbnails, frames too)")
	fmt.Println("      --force                                            - Even from a library losing more than half its files")
	fmt.Println("  cli config init <template> [options]                   - Write a new config (CONFIG_PATH, else")
	fmt.Println("                                                           config.yml) from the template")
	fmt.Println("                                                           (config.example.yml); -h for the options")
	fmt.Println("  cli config add-path <path>                             - Add media directory")
	fmt.Println("  cli config list-paths                                  - List media directories")
	fmt.Println("  cli config set-cjk-font <path>                         - Set media.cjkFontPath (\"\" for the default)")
	fmt.Println("  cli config set-subset-command <command>                - Set media.subsetCommand (\"\" for the default)")
}

func handleUserCommand(db *database.DB, args []string) {
	if len(args) < 1 {
		fmt.Println("Missing user subcommand")
		printUsage()
		os.Exit(1)
	}

	subcommand := args[0]

	switch subcommand {
	case "create":
		if len(args) < 2 {
			fmt.Println("Missing username")
			os.Exit(1)
		}
		username := args[1]
		isAdmin := false
		group := ""

		for i := 2; i < len(args); i++ {
			if args[i] == "--admin" {
				isAdmin = true
			} else if args[i] == "--group" && i+1 < len(args) {
				group = args[i+1]
				i++
			}
		}

		if group == "" {
			group = config.GlobalConfig.DefaultGroup() // the first group
		}
		if !config.GlobalConfig.HasGroup(group) {
			fmt.Printf("Error: give the user one of the groups with --group: %s\n", strings.Join(config.GlobalConfig.Groups, ", "))
			os.Exit(1)
		}

		fmt.Print("Enter password: ")
		password, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()

		fmt.Print("Confirm password: ")
		confirmPassword, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()

		if string(password) != string(confirmPassword) {
			fmt.Println("Error: Passwords do not match")
			os.Exit(1)
		}

		user, err := db.CreateUser(username, string(password), isAdmin, group)
		if err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}

		fmt.Printf("User created: %s (ID: %d, Admin: %t, Group: %s)\n", user.Username, user.ID, user.IsAdmin, user.Group)

	case "list":
		users, err := db.ListUsers()
		if err != nil {
			log.Fatalf("Failed to list users: %v", err)
		}

		fmt.Println("Users:")
		for _, user := range users {
			adminStr := ""
			if user.IsAdmin {
				adminStr = " (admin)"
			}
			fmt.Printf("  - %s%s (ID: %d, Group: %s)\n", user.Username, adminStr, user.ID, user.Group)
		}

	case "delete":
		if len(args) < 2 {
			fmt.Println("Missing username")
			os.Exit(1)
		}
		username := args[1]

		if err := db.DeleteUser(username); err != nil {
			log.Fatalf("Failed to delete user: %v", err)
		}

		fmt.Printf("User deleted: %s\n", username)

	case "change-password":
		if len(args) < 2 {
			fmt.Println("Missing username")
			os.Exit(1)
		}
		username := args[1]

		// Verify user exists
		_, err := db.GetUserByUsername(username)
		if err != nil {
			log.Fatalf("User not found: %s", username)
		}

		fmt.Print("Enter new password: ")
		password, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()

		fmt.Print("Confirm new password: ")
		confirmPassword, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()

		if string(password) != string(confirmPassword) {
			fmt.Println("Error: Passwords do not match")
			os.Exit(1)
		}

		if err := db.UpdateUserPassword(username, string(password)); err != nil {
			log.Fatalf("Failed to update password: %v", err)
		}

		fmt.Printf("Password updated for user: %s\n", username)

	case "set-group":
		if len(args) < 3 {
			fmt.Println("Usage: cli user set-group <username> <group>")
			os.Exit(1)
		}
		username := args[1]
		group := args[2]
		if !config.GlobalConfig.HasGroup(group) {
			fmt.Printf("Error: %q isn't a group. Groups: %s\n", group, strings.Join(config.GlobalConfig.Groups, ", "))
			os.Exit(1)
		}

		if err := db.UpdateUser(username, models.UserUpdateRequest{Group: &group}); err != nil {
			log.Fatalf("Failed to set user group: %v", err)
		}

		fmt.Printf("Group updated for user: %s -> %s\n", username, group)

	default:
		fmt.Printf("Unknown user subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func handleScanCommand(db *database.DB) {
	fmt.Println("Scanning media directories...")
	scanner := media.NewScanner(db)

	if err := scanner.ScanMediaPaths(); err != nil {
		log.Fatalf("Failed to scan media: %v", err)
	}

	fmt.Println("Scan completed successfully")
}

func handleTrickplayCommand(db *database.DB, args []string) {
	trickplayGenerator := media.NewTrickplayGenerator(db)

	if len(args) > 0 && args[0] == "clean" {
		fmt.Println("Cleaning up orphaned trickplay images...")
		if err := trickplayGenerator.Clean(); err != nil {
			log.Fatalf("Failed to clean trickplay images: %v", err)
		}
		fmt.Println("Trickplay cleanup completed.")
		return
	}

	// Parse flags
	var videoPath, dirPath, libraryName string
	if len(args) > 0 {
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--video":
				if i+1 < len(args) {
					videoPath = args[i+1]
					i++
				}
			case "--dir":
				if i+1 < len(args) {
					dirPath = args[i+1]
					i++
				}
			case "--library":
				if i+1 < len(args) {
					libraryName = args[i+1]
					i++
				}
			}
		}
	}

	if videoPath != "" {
		if err := trickplayGenerator.Generate(videoPath); err != nil {
			log.Fatalf("Failed to generate trickplay frames for %s: %v", videoPath, err)
		}
	} else if dirPath != "" {
		fmt.Printf("Generating trickplay frames for directory: %s\n", dirPath)
		if err := trickplayGenerator.GenerateForDirectory(dirPath); err != nil {
			log.Fatalf("Failed to generate trickplay frames for directory %s: %v", dirPath, err)
		}
	} else if libraryName != "" {
		fmt.Printf("Generating trickplay frames for library: %s\n", libraryName)
		var libraryPath string
		for _, p := range config.GlobalConfig.Media.Paths {
			if p.Name == libraryName {
				libraryPath = p.Path
				break
			}
		}

		if libraryPath == "" {
			log.Fatalf("Library '%s' not found in config", libraryName)
		}

		if err := trickplayGenerator.GenerateForDirectory(libraryPath); err != nil {
			log.Fatalf("Failed to generate trickplay frames for library %s: %v", libraryName, err)
		}
	} else if len(args) == 0 {
		fmt.Println("Generating trickplay frames for all videos in the database...")
		items, err := db.ListMediaItems("video")
		if err != nil {
			log.Fatalf("Failed to list video items from database: %v", err)
		}

		for _, item := range items {
			if err := trickplayGenerator.Generate(item.Path); err != nil {
				fmt.Printf("Error generating trickplay frames for %s: %v\n", item.Path, err)
			}
		}
	} else {
		fmt.Println("Invalid arguments for trickplay command.")
		printUsage()
	}
}

func handleCleanupCommand(db *database.DB, args []string) {
	force := false
	for _, a := range args {
		if a != "--force" {
			fmt.Printf("Unknown cleanup option: %s\n", a)
			printUsage()
			os.Exit(1)
		}
		force = true
	}
	n, err := media.NewPruner(db).Prune(context.Background(), force)
	if err != nil {
		log.Fatalf("Cleanup failed: %v", err)
	}
	fmt.Printf("Forgot %d file(s) that are gone.\n", n)
}

func handleConfigCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Missing config subcommand")
		printUsage()
		os.Exit(1)
	}

	subcommand := args[0]

	switch subcommand {
	case "add-path":
		if len(args) < 2 {
			fmt.Println("Missing path")
			os.Exit(1)
		}
		path := args[1]

		// Check if path exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("Warning: Path does not exist: %s\n", path)
			fmt.Print("Add anyway? (y/n): ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(response)) != "y" {
				fmt.Println("Cancelled")
				os.Exit(0)
			}
		}

		// Add path to config
		name := filepath.Base(path)
		config.GlobalConfig.Media.Paths = append(config.GlobalConfig.Media.Paths, config.MediaPath{
			Name: name,
			Path: path,
		})

		// Save config
		if err := config.Save(config.Path(), config.GlobalConfig); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}

		fmt.Printf("Added media path: %s\n", path)

	case "list-paths":
		fmt.Println("Media directories:")
		for i, mediaPath := range config.GlobalConfig.Media.Paths {
			typeLabel := ""
			if mediaPath.Type != "" {
				typeLabel = " [Type: " + mediaPath.Type + "]"
			}
			fmt.Printf("  %d. %s (%s) [Groups: %v]%s\n", i+1, mediaPath.Name, mediaPath.Path, mediaPath.Groups, typeLabel)
		}

	case "set-cjk-font", "set-subset-command":
		// The CJK subtitle font and its subsetter (setup.sh sets them when it installs them)
		if len(args) != 2 {
			fmt.Printf("Usage: cli config %s <value>\n", subcommand)
			os.Exit(1)
		}
		key := "cjkFontPath"
		if subcommand == "set-cjk-font" {
			config.GlobalConfig.Media.CJKFontPath = args[1]
		} else {
			key = "subsetCommand"
			config.GlobalConfig.Media.SubsetCommand = args[1]
		}
		if err := config.Save(config.Path(), config.GlobalConfig); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		fmt.Printf("Set media.%s to %q\n", key, args[1])

	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

// initConfig writes a new config to path: the template's settings (config.example.yml) and
// comments, with the values given as options, a new JWT secret, and no proxies trusted. The TMDB
// token comes from $WEIRDSTREAMER_TMDB_TOKEN, so it isn't on the command line. An existing config
// is left alone.
func initConfig(args []string, path string) {
	fs := flag.NewFlagSet("config init", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: cli config init <template> [options]")
		fs.PrintDefaults()
	}
	name := fs.String("name", "", "the server's name, which the apps show (default: the template's)")
	port := fs.Int("port", 0, "the port to listen on (default: the template's)")
	host := fs.String("host", "0.0.0.0", "the address to listen on")
	cert := fs.String("cert", "", "HTTPS certificate file (with -key); without them, plain HTTP")
	key := fs.String("key", "", "HTTPS private key file")
	var libraries []string
	fs.Func("library", `a library, "name|folder|type" (type: movie, tv or none); repeat for more. `+
		"Every group may see it. Without any, the config has none (add them in Settings)", func(v string) error {
		libraries = append(libraries, v)
		return nil
	})
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fs.Usage()
		os.Exit(2)
	}
	template := args[0]
	fs.Parse(args[1:])
	if (*cert == "") != (*key == "") {
		log.Fatal("Give both -cert and -key, or neither")
	}
	if _, err := os.Stat(path); err == nil {
		log.Fatalf("%s already exists: edit it, or move it aside first", path)
	}

	if err := config.Load(template); err != nil {
		log.Fatalf("Reading the template: %v", err)
	}
	c := *config.GlobalConfig
	if *name != "" {
		c.Server.Name = *name
	}
	if *port != 0 {
		c.Server.Port = *port
	}
	c.Server.Host = *host
	c.Server.CertFile, c.Server.KeyFile = *cert, *key
	c.Streaming.EnableHTTP2 = *cert != ""
	c.Server.TrustedProxies = []string{} // none until there's a proxy in front
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal(err)
	}
	c.Auth.JWTSecret = hex.EncodeToString(secret)
	c.Metadata.TMDBAccessToken = os.Getenv("WEIRDSTREAMER_TMDB_TOKEN")
	c.Media.Paths = []config.MediaPath{}
	for _, l := range libraries {
		parts := strings.Split(l, "|")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
			log.Fatalf(`-library %q: want "name|folder|type"`, l)
		}
		p := config.MediaPath{Name: parts[0], Path: parts[1], Groups: append([]string(nil), c.Groups...)}
		if t := parts[2]; t != "" && t != "none" {
			p.Type = t
		}
		c.Media.Paths = append(c.Media.Paths, p)
	}
	if err := c.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}
	if err := config.SaveFrom(template, path, &c); err != nil {
		log.Fatalf("Writing %s: %v", path, err)
	}
	if err := config.Load(path); err != nil {
		log.Fatalf("The written %s doesn't load: %v", path, err)
	}
	fmt.Printf("Wrote %s\n", path)
}
