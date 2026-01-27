package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.json"
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
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Media Server CLI")
	fmt.Println("\nUsage:")
	fmt.Println("  cli user create <username> [--admin]  - Create a new user")
	fmt.Println("  cli user list                         - List all users")
	fmt.Println("  cli user delete <username>            - Delete a user")
	fmt.Println("  cli user change-password <username>   - Change user password")
	fmt.Println("  cli scan                              - Scan media directories")
	fmt.Println("  cli config add-path <path>            - Add media directory")
	fmt.Println("  cli config list-paths                 - List media directories")
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
		isAdmin := len(args) > 2 && args[2] == "--admin"

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

		user, err := db.CreateUser(username, string(password), isAdmin)
		if err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}

		fmt.Printf("User created: %s (ID: %d, Admin: %t)\n", user.Username, user.ID, user.IsAdmin)

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
			fmt.Printf("  - %s%s (ID: %d)\n", user.Username, adminStr, user.ID)
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
		configPath := os.Getenv("CONFIG_PATH")
		if configPath == "" {
			configPath = "./config.json"
		}

		if err := config.Save(configPath, config.GlobalConfig); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}

		fmt.Printf("Added media path: %s\n", path)

	case "list-paths":
		fmt.Println("Media directories:")
		for i, mediaPath := range config.GlobalConfig.Media.Paths {
			fmt.Printf("  %d. %s (%s)\n", i+1, mediaPath.Name, mediaPath.Path)
		}

	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}
