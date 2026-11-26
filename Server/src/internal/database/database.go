package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	userConn  *sql.DB
	mediaConn *sql.DB
}

func New(userDBPath, mediaDBPath string) (*DB, error) {
	// WAL plus a busy timeout lets background metadata/thumbnail writers run alongside reads
	// instead of failing with "database is locked".
	const dsnOptions = "?_journal_mode=WAL&_busy_timeout=5000"
	userConn, err := sql.Open("sqlite3", userDBPath+dsnOptions)
	if err != nil {
		return nil, err
	}

	mediaConn, err := sql.Open("sqlite3", mediaDBPath+dsnOptions)
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
	` + mediaAddedSchema

	if _, err := db.userConn.Exec(userSchema); err != nil {
		return err
	}

	// Migration: add user_group column if it doesn't exist
	_, _ = db.userConn.Exec("ALTER TABLE users ADD COLUMN user_group TEXT")
	_, _ = db.userConn.Exec("ALTER TABLE users ADD COLUMN audio_language TEXT NOT NULL DEFAULT ''")
	_, _ = db.userConn.Exec("ALTER TABLE users ADD COLUMN subtitle_language TEXT NOT NULL DEFAULT ''")
	_, _ = db.userConn.Exec("ALTER TABLE users ADD COLUMN subtitles_on INTEGER NOT NULL DEFAULT 0")
	if err := db.initSessionSchema(); err != nil {
		return err
	}

	if _, err := db.mediaConn.Exec(mediaSchema); err != nil {
		return err
	}

	// path_hash maps thumbnail and trickplay file names back to their media item, so access to
	// them can be checked against the item's library.
	_, _ = db.mediaConn.Exec("ALTER TABLE media_items ADD COLUMN path_hash TEXT")
	// A video's format line ("4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"): "" not read yet, "?" none
	_, _ = db.mediaConn.Exec("ALTER TABLE media_items ADD COLUMN format TEXT NOT NULL DEFAULT ''")
	if _, err := db.mediaConn.Exec("CREATE INDEX IF NOT EXISTS idx_media_path_hash ON media_items(path_hash)"); err != nil {
		return err
	}
	if err := db.backfillPathHashes(); err != nil {
		return err
	}
	if err := db.initMetadataSchema(); err != nil {
		return err
	}
	if err := db.initUserCollections(); err != nil {
		return err
	}
	if err := db.initGuide(); err != nil {
		return err
	}
	if err := db.initPersonDetails(); err != nil {
		return err
	}
	if err := db.initDiscSchema(); err != nil {
		return err
	}
	if err := db.initPlaybackSchema(); err != nil {
		return err
	}
	if err := db.initSegmentSchema(); err != nil {
		return err
	}

	return nil
}

func (db *DB) backfillPathHashes() error {
	rows, err := db.mediaConn.Query("SELECT id, path FROM media_items WHERE path_hash IS NULL")
	if err != nil {
		return err
	}
	type row struct {
		id   int64
		path string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.path); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, r)
	}
	rows.Close()
	if len(pending) == 0 {
		return nil
	}

	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("UPDATE media_items SET path_hash = ? WHERE id = ?")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range pending {
		if _, err := stmt.Exec(config.CacheKey(r.path), r.id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) Close() error {
	if err := db.userConn.Close(); err != nil {
		return err
	}
	return db.mediaConn.Close()
}

// ErrBadUsername refuses a username with characters an email address can't have.
var ErrBadUsername = errors.New("a username can have only letters, digits and these: . @ ! # $ % & ' * + - / = ? ^ _ ` { | } ~ (at most 254)")

// ValidUsername reports whether name uses only the characters of an email address (ASCII letters
// and digits, and . @ ! # $ % & ' * + - / = ? ^ _ ` { | } ~), at most 254 of them.
func ValidUsername(name string) bool {
	if name == "" || len(name) > 254 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune(".@!#$%&'*+-/=?^_`{|}~", c)) {
			return false
		}
	}
	return true
}

// User operations
func (db *DB) CreateUser(username, password string, isAdmin bool, group string) (*models.User, error) {
	if !ValidUsername(username) {
		return nil, ErrBadUsername
	}
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

// UsersInGroup lists the usernames of a group's users.
func (db *DB) UsersInGroup(group string) ([]string, error) {
	rows, err := db.userConn.Query("SELECT username FROM users WHERE user_group = ? ORDER BY username", group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// RenameGroup moves a group's users to its new name.
func (db *DB) RenameGroup(from, to string) error {
	_, err := db.userConn.Exec("UPDATE users SET user_group = ? WHERE user_group = ?", to, from)
	return err
}

func (db *DB) DeleteUser(username string) error {
	for _, q := range []string{"DELETE FROM playback WHERE user_id = (SELECT id FROM users WHERE username = ?)",
		"DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = ?)"} {
		if _, err := db.userConn.Exec(q, username); err != nil {
			return err
		}
	}
	_, err := db.userConn.Exec("DELETE FROM users WHERE username = ?", username)
	return err
}

// UpdateUserPassword sets a user's password and ends their sessions, except keepSession (the one
// that changed it; "" ends them all).
func (db *DB) UpdateUserPassword(username, newPassword, keepSession string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user, err := db.GetUserByUsername(username)
	if err != nil {
		return err
	}
	if _, err = db.userConn.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(hash), user.ID); err != nil {
		return err
	}
	return db.EndUserSessions(user.ID, keepSession)
}

// SetMediaFormat stores a known file's format line, keeping the rest (and its ID).
func (db *DB) SetMediaFormat(path, format string) error {
	_, err := db.mediaConn.Exec(`UPDATE media_items SET format = ? WHERE path = ?`, format, path)
	return err
}

// Media operations
func (db *DB) SaveMediaItem(item *models.MediaItem) error {
	result, err := db.mediaConn.Exec(`
		INSERT OR REPLACE INTO media_items 
		(path, name, type, size, mime_type, thumbnail_url, duration, width, height, format, path_hash, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, item.Path, item.Name, item.Type, item.Size, item.MimeType, item.ThumbnailURL, item.Duration, item.Width, item.Height, item.Format, config.CacheKey(item.Path))

	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	item.ID = id
	return db.recordArrival(item.Path)
}

func (db *DB) GetMediaItem(path string) (*models.MediaItem, error) {
	item := &models.MediaItem{}
	err := db.mediaConn.QueryRow(`
		SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, format, created_at, updated_at
		FROM media_items WHERE path = ?
	`, path).Scan(&item.ID, &item.Path, &item.Name, &item.Type, &item.Size, &item.MimeType,
		&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.Format, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return item, nil
}

// FolderPreview gives the thumbnail of a photo or video under the folder dir (a disk path) to
// show on it: of the first 200 with thumbnails in path order, the one nearest the folder (its own
// files before its subfolders'), or "" when it has none yet.
func (db *DB) FolderPreview(dir string) string {
	url, _ := db.FolderPreviewItem(dir)
	return url
}

// FolderPreviewItem is FolderPreview with the file (a disk path) the thumbnail is of.
func (db *DB) FolderPreviewItem(dir string) (url, path string) {
	lo, hi := under(dir)
	err := db.mediaConn.QueryRow(`
		SELECT thumbnail_url, path FROM (
			SELECT path, thumbnail_url FROM media_items
			WHERE path > ? AND path < ? AND thumbnail_url != '' AND type IN ('image', 'video')
			ORDER BY path LIMIT 200)
		ORDER BY length(path) - length(replace(path, ?, '')), path LIMIT 1
	`, lo, hi, sep).Scan(&url, &path) // nearest first: the fewest separators
	if err != nil {
		return "", ""
	}
	return url, path
}

func (db *DB) GetMediaItemByID(id int64) (*models.MediaItem, error) {
	item := &models.MediaItem{}
	err := db.mediaConn.QueryRow(`
		SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, format, created_at, updated_at
		FROM media_items WHERE id = ?
	`, id).Scan(&item.ID, &item.Path, &item.Name, &item.Type, &item.Size, &item.MimeType,
		&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.Format, &item.CreatedAt, &item.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return item, nil
}

// GetMediaItemByPathHash finds the media item whose thumbnail/trickplay files use hash.
func (db *DB) GetMediaItemByPathHash(hash string) (*models.MediaItem, error) {
	item := &models.MediaItem{}
	err := db.mediaConn.QueryRow(`
		SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, format, created_at, updated_at
		FROM media_items WHERE path_hash = ?
	`, hash).Scan(&item.ID, &item.Path, &item.Name, &item.Type, &item.Size, &item.MimeType,
		&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.Format, &item.CreatedAt, &item.UpdatedAt)

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
			SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, format, created_at, updated_at
			FROM media_items WHERE type = ? ORDER BY name
		`, mediaType)
	} else {
		rows, err = db.mediaConn.Query(`
			SELECT id, path, name, type, size, mime_type, thumbnail_url, duration, width, height, format, created_at, updated_at
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
			&item.ThumbnailURL, &item.Duration, &item.Width, &item.Height, &item.Format, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (db *DB) DeleteMediaItem(path string) error {
	_, err := db.mediaConn.Exec("DELETE FROM media_items WHERE path = ?", path)
	if err == nil {
		_, err = db.mediaConn.Exec("DELETE FROM media_added WHERE path = ?", path)
	}
	return err
}

// Languages are a user's track choices: the languages they prefer for audio and for subtitles
// (ISO 639-1 codes, "" for the file's default), and whether subtitles start on.
type Languages struct {
	Audio       string `json:"audio_language"`
	Subtitle    string `json:"subtitle_language"`
	SubtitlesOn bool   `json:"subtitles_on"`
}

// UserLanguages are the user's track choices.
func (db *DB) UserLanguages(id int64) Languages {
	var l Languages
	_ = db.userConn.QueryRow("SELECT audio_language, subtitle_language, subtitles_on FROM users WHERE id = ?", id).
		Scan(&l.Audio, &l.Subtitle, &l.SubtitlesOn)
	return l
}

// SetUserLanguages keeps the user's track choices.
func (db *DB) SetUserLanguages(id int64, l Languages) error {
	_, err := db.userConn.Exec("UPDATE users SET audio_language = ?, subtitle_language = ?, subtitles_on = ? WHERE id = ?",
		l.Audio, l.Subtitle, l.SubtitlesOn, id)
	return err
}
