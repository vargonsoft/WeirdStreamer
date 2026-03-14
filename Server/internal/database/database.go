package database

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/weirdstreamer/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	userConn  *sql.DB
	mediaConn *sql.DB
}

func New(userDBPath, mediaDBPath string) (*DB, error) {
	userConn, err := sql.Open("sqlite3", userDBPath)
	if err != nil {
		return nil, err
	}

	mediaConn, err := sql.Open("sqlite3", mediaDBPath)
	if err != nil {
		userConn.Close()
		return nil, err
	}

	db := &DB{
		userConn:  userConn,
		mediaConn: mediaConn,
	}
	if err := db.initSchema(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) initSchema() error {
	userSchema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		is_admin INTEGER DEFAULT 0,
		user_group TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	mediaSchema := `
	CREATE TABLE IF NOT EXISTS media_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		size INTEGER NOT NULL,
		mime_type TEXT NOT NULL,
		thumbnail_url TEXT,
		duration REAL,
		width INTEGER,
		height INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_media_type ON media_items(type);
	CREATE INDEX IF NOT EXISTS idx_media_path ON media_items(path);
	`

	if _, err := db.userConn.Exec(userSchema); err != nil {
		return err
	}

	// Migration: add user_group column if it doesn't exist
	_, _ = db.userConn.Exec("ALTER TABLE users ADD COLUMN user_group TEXT")

	if _, err := db.mediaConn.Exec(mediaSchema); err != nil {
		return err
	}

	return nil
}

func (db *DB) Close() error {
	if err := db.userConn.Close(); err != nil {
		return err
	}
	return db.mediaConn.Close()
}

// User operations
func (db *DB) CreateUser(username, password string, isAdmin bool, group string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	result, err := db.userConn.Exec(
		"INSERT INTO users (username, password_hash, is_admin, user_group) VALUES (?, ?, ?, ?)",
		username, string(hash), isAdmin, group,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &models.User{
		ID:           id,
		Username:     username,
		PasswordHash: string(hash),
		IsAdmin:      isAdmin,
		Group:        group,
		CreatedAt:    time.Now(),
	}, nil
}

func (db *DB) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	err := db.userConn.QueryRow(
		"SELECT id, username, password_hash, is_admin, user_group, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin, &user.Group, &user.CreatedAt)

	if err != nil {
		return nil, err
	}
	return user, nil
}

func (db *DB) GetUserByID(id int64) (*models.User, error) {
	user := &models.User{}
	err := db.userConn.QueryRow(
		"SELECT id, username, password_hash, is_admin, user_group, created_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin, &user.Group, &user.CreatedAt)

	if err != nil {
		return nil, err
	}
	return user, nil
}

func (db *DB) ListUsers() ([]models.User, error) {
	rows, err := db.userConn.Query("SELECT id, username, is_admin, user_group, created_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.IsAdmin, &user.Group, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (db *DB) UpdateUser(username string, req models.UserUpdateRequest) error {
	if req.IsAdmin != nil && req.Group != nil {
		_, err := db.userConn.Exec("UPDATE users SET is_admin = ?, user_group = ? WHERE username = ?", *req.IsAdmin, *req.Group, username)
		return err
	} else if req.IsAdmin != nil {
		_, err := db.userConn.Exec("UPDATE users SET is_admin = ? WHERE username = ?", *req.IsAdmin, username)
		return err
	} else if req.Group != nil {
		_, err := db.userConn.Exec("UPDATE users SET user_group = ? WHERE username = ?", *req.Group, username)
		return err
	}
	return nil
}

func (db *DB) DeleteUser(username string) error {
	_, err := db.userConn.Exec("DELETE FROM users WHERE username = ?", username)
	return err
}

func (db *DB) UpdateUserPassword(username, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = db.userConn.Exec("UPDATE users SET password_hash = ? WHERE username = ?", string(hash), username)
	return err
}

// Media operations
func (db *DB) SaveMediaItem(item *models.MediaItem) error {
	result, err := db.mediaConn.Exec(`
		INSERT OR REPLACE INTO media_items 
		(path, name, type, size, mime_type, thumbnail_url, duration, width, height, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, item.Path, item.Name, item.Type, item.Size, item.MimeType, item.ThumbnailURL, item.Duration, item.Width, item.Height)

	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	item.ID = id
	return nil
}

func (db *DB) GetMediaItem(path string) (*models.MediaItem, error) {
	item := &models.MediaItem{}
	err := db.mediaConn.QueryRow(`
		SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, created_at, updated_at
		FROM media_items WHERE path = ?
	`, path).Scan(&item.ID, &item.Path, &item.Name, &item.Type, &item.Size, &item.MimeType,
		&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return item, nil
}

func (db *DB) GetMediaItemByID(id int64) (*models.MediaItem, error) {
	item := &models.MediaItem{}
	err := db.mediaConn.QueryRow(`
		SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, created_at, updated_at
		FROM media_items WHERE id = ?
	`, id).Scan(&item.ID, &item.Path, &item.Name, &item.Type, &item.Size, &item.MimeType,
		&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return item, nil
}

func (db *DB) ListMediaItems(mediaType string) ([]models.MediaItem, error) {
	var rows *sql.Rows
	var err error

	if mediaType != "" {
		rows, err = db.mediaConn.Query(`
			SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, created_at, updated_at
			FROM media_items WHERE type = ? ORDER BY name
		`, mediaType)
	} else {
		rows, err = db.mediaConn.Query(`
			SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, created_at, updated_at
			FROM media_items ORDER BY name
		`)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.MediaItem
	for rows.Next() {
		var item models.MediaItem
		if err := rows.Scan(&item.ID, &item.Path, &item.Name, &item.Type, &item.Size, &item.MimeType,
			&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (db *DB) DeleteMediaItem(path string) error {
	_, err := db.mediaConn.Exec("DELETE FROM media_items WHERE path = ?", path)
	return err
}
