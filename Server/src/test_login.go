package main

import (
"fmt"
"log"
"golang.org/x/crypto/bcrypt"
"github.com/weirdstreamer/server/internal/config"
"github.com/weirdstreamer/server/internal/database"
)

func main() {
if err := config.Load("./config.yml"); err != nil {
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

// Test with "admin" password
testPasswords := []string{"admin", "password", "admin123"}

for _, pwd := range testPasswords {
err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(pwd))
if err == nil {
fmt.Printf("✓ Password '%s' matches!\n", pwd)
} else {
fmt.Printf("✗ Password '%s' does not match\n", pwd)
}
}
}
