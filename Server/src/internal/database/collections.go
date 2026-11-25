package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Collections admins make in a movie library: a name, an optional description and the library's
// titles in it (by kind and TMDB ID, so they follow the library's files wherever its folder is).

const userCollectionsSchema = `
CREATE TABLE IF NOT EXISTS user_collections (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	library_id TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS user_collections_library ON user_collections (library_id);

CREATE TABLE IF NOT EXISTS user_collection_titles (
	collection_id INTEGER NOT NULL,
	kind TEXT NOT NULL,
	tmdb_id INTEGER NOT NULL,
	added_at DATETIME NOT NULL,
	PRIMARY KEY (collection_id, kind, tmdb_id)
);
`

// UserCollection is a collection an admin made; Titles are its library titles, in no order.
type UserCollection struct {
	ID          int64
	LibraryID   string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Titles      []CollectionTitle
}

// CollectionTitle is a library title in a collection: "movie" (or "show", a TV movie) and its TMDB ID.
type CollectionTitle struct {
	Kind   string `json:"kind"`
	TMDBID int    `json:"tmdb_id"`
}

// ErrNoUserCollection is returned for a collection that isn't in the library.
var ErrNoUserCollection = errors.New("no such collection in this library")

func (db *DB) initUserCollections() error {
	_, err := db.mediaConn.Exec(userCollectionsSchema)
	return err
}

// UserCollections lists a library's collections with their titles, by name.
func (db *DB) UserCollections(libraryID string) ([]UserCollection, error) {
	rows, err := db.mediaConn.Query(`SELECT id, library_id, name, description, created_at, updated_at
		FROM user_collections WHERE library_id = ? ORDER BY name COLLATE NOCASE, id`, libraryID)
	if err != nil {
		return nil, err
	}
	var out []UserCollection
	for rows.Next() {
		var c UserCollection
		if err := rows.Scan(&c.ID, &c.LibraryID, &c.Name, &c.Description, &c.CreatedAt, &c.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Titles, err = db.collectionTitles(out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// UserCollection returns one of a library's collections with its titles.
func (db *DB) UserCollection(libraryID string, id int64) (*UserCollection, error) {
	c := &UserCollection{}
	err := db.mediaConn.QueryRow(`SELECT id, library_id, name, description, created_at, updated_at
		FROM user_collections WHERE id = ? AND library_id = ?`, id, libraryID).
		Scan(&c.ID, &c.LibraryID, &c.Name, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoUserCollection
	}
	if err != nil {
		return nil, err
	}
	c.Titles, err = db.collectionTitles(id)
	return c, err
}

func (db *DB) collectionTitles(id int64) ([]CollectionTitle, error) {
	rows, err := db.mediaConn.Query(`SELECT kind, tmdb_id FROM user_collection_titles WHERE collection_id = ? ORDER BY added_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CollectionTitle
	for rows.Next() {
		var t CollectionTitle
		if err := rows.Scan(&t.Kind, &t.TMDBID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateUserCollection makes a collection in a library with these titles, and returns its ID.
func (db *DB) CreateUserCollection(libraryID, name, description string, titles []CollectionTitle) (int64, error) {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	res, err := tx.Exec(`INSERT INTO user_collections (library_id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		libraryID, strings.TrimSpace(name), strings.TrimSpace(description), now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := addCollectionTitles(tx, id, titles, now); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateUserCollection changes a library's collection: its name and description when given (nil
// leaves them), and adds and removes titles.
func (db *DB) UpdateUserCollection(libraryID string, id int64, name, description *string, add, remove []CollectionTitle) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_collections WHERE id = ? AND library_id = ?`, id, libraryID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNoUserCollection
	}
	now := time.Now().UTC()
	if name != nil {
		if _, err := tx.Exec(`UPDATE user_collections SET name = ? WHERE id = ?`, strings.TrimSpace(*name), id); err != nil {
			return err
		}
	}
	if description != nil {
		if _, err := tx.Exec(`UPDATE user_collections SET description = ? WHERE id = ?`, strings.TrimSpace(*description), id); err != nil {
			return err
		}
	}
	if err := addCollectionTitles(tx, id, add, now); err != nil {
		return err
	}
	for _, t := range remove {
		if _, err := tx.Exec(`DELETE FROM user_collection_titles WHERE collection_id = ? AND kind = ? AND tmdb_id = ?`,
			id, t.Kind, t.TMDBID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE user_collections SET updated_at = ? WHERE id = ?`, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

func addCollectionTitles(tx *sql.Tx, id int64, titles []CollectionTitle, at time.Time) error {
	for _, t := range titles {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO user_collection_titles (collection_id, kind, tmdb_id, added_at) VALUES (?, ?, ?, ?)`,
			id, t.Kind, t.TMDBID, at); err != nil {
			return err
		}
	}
	return nil
}

// DeleteUserCollection removes a library's collection (its titles stay in the library).
func (db *DB) DeleteUserCollection(libraryID string, id int64) error {
	res, err := db.mediaConn.Exec(`DELETE FROM user_collections WHERE id = ? AND library_id = ?`, id, libraryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoUserCollection
	}
	_, err = db.mediaConn.Exec(`DELETE FROM user_collection_titles WHERE collection_id = ?`, id)
	return err
}
