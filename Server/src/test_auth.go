package main

import (
"fmt"
"log"
"github.com/weirdstreamer/server/internal/config"
"github.com/weirdstreamer/server/internal/database"
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

users, err := db.ListUsers()
if err != nil {
log.Fatalf("Failed to list users: %v", err)
}

fmt.Printf("Found %d users:\n", len(users))
for _, user := range users {
fmt.Printf("  - %s (ID: %d, Admin: %t)\n", user.Username, user.ID, user.IsAdmin)
}

// Try to get admin user
user, err := db.GetUserByUsername("admin")
if err != nil {
log.Fatalf("Failed to get admin user: %v", err)
}

fmt.Printf("\nAdmin user details:\n")
fmt.Printf("  ID: %d\n", user.ID)
fmt.Printf("  Username: %s\n", user.Username)
fmt.Printf("  IsAdmin: %t\n", user.IsAdmin)
fmt.Printf("  PasswordHash length: %d\n", len(user.PasswordHash))
}
