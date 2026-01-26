package main

import (
"fmt"
"log"
"golang.org/x/crypto/bcrypt"
"github.com/weirdstreamer/server/internal/config"
"github.com/weirdstreamer/server/internal/database"
"github.com/weirdstreamer/server/internal/auth"
)

func main() {
if err := config.Load("./config.json"); err != nil {
log.Fatalf("Failed to load config: %v", err)
}

db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
if err != nil {
log.Fatalf("Failed to initialize database: %v", err)
}
defer db.Close()

user, err := db.GetUserByUsername("admin")
if err != nil {
log.Fatalf("Failed to get admin user: %v", err)
}

// Test password
err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("admin"))
if err == nil {
fmt.Println("✓ Password 'admin' matches in database!")
} else {
fmt.Printf("✗ Password 'admin' does NOT match: %v\n", err)
}

// Now test full authentication flow
authService := auth.NewService(db)
response, err := authService.Authenticate("admin", "admin")
if err != nil {
fmt.Printf("✗ Authentication failed: %v\n", err)
} else {
fmt.Printf("✓ Authentication successful! Token: %s...\n", response.Token[:20])
fmt.Printf("  User ID: %d\n", response.User.ID)
fmt.Printf("  Username: %s\n", response.User.Username)
fmt.Printf("  Is Admin: %t\n", response.User.IsAdmin)
}
}
