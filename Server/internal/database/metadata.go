package database

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

const metadataSchema = `
-- One row per distinct title query (per library), shared by every file that parses to it.
CREATE TABLE IF NOT EXISTS metadata_lookups (
	kind TEXT NOT NULL,            -- "movie" or "tv"
	library_id TEXT NOT NULL,
	query_key TEXT NOT NULL,
	query_title TEXT NOT NULL,
	query_year INTEGER NOT NULL DEFAULT 0,
	query_country TEXT NOT NULL DEFAULT '',
	tmdb_id INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,          -- "matched", "not_found", "error"
	attempted_at DATETIME NOT NULL,
	PRIMARY KEY (kind, library_id, query_key)
);
CREATE INDEX IF NOT EXISTS idx_lookups_tmdb ON metadata_lookups(kind, tmdb_id);

-- Folders that stand for one title: single-movie folders and TV show folders.
CREATE TABLE IF NOT EXISTS metadata_folders (
	library_id TEXT NOT NULL,
	folder_path TEXT NOT NULL,
	kind TEXT NOT NULL,
	query_key TEXT NOT NULL,
	PRIMARY KEY (library_id, folder_path)
);

CREATE TABLE IF NOT EXISTS movies (
	tmdb_id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	original_title TEXT,
	year INTEGER,
	release_date TEXT,
	overview TEXT,
	tagline TEXT,
	runtime INTEGER,
	genres TEXT,
	imdb_id TEXT,
	poster TEXT,
	backdrop TEXT,
	logo TEXT,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS shows (
	tmdb_id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	original_name TEXT,
	year INTEGER,
	first_air_date TEXT,
	overview TEXT,
	tagline TEXT,
	genres TEXT,
	imdb_id TEXT,
	runtime INTEGER,
	poster TEXT,
	backdrop TEXT,
	logo TEXT,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS seasons (
	show_id INTEGER NOT NULL,
	season_number INTEGER NOT NULL,
	name TEXT,
	overview TEXT,
	poster TEXT,
	status TEXT NOT NULL,          -- "fetched", "not_found", "error"
	fetched_at DATETIME NOT NULL,
	PRIMARY KEY (show_id, season_number)
);

CREATE TABLE IF NOT EXISTS episodes (
	show_id INTEGER NOT NULL,
	season_number INTEGER NOT NULL,
	episode_number INTEGER NOT NULL,
	tmdb_id INTEGER,
	name TEXT,
	overview TEXT,
	air_date TEXT,
	runtime INTEGER,
	still TEXT,
	updated_at DATETIME NOT NULL,
	PRIMARY KEY (show_id, season_number, episode_number)
);

CREATE TABLE IF NOT EXISTS people (
	tmdb_id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	profile TEXT                   -- downloaded headshot (artwork-relative path), or empty
);

-- owner is "movie:<id>", "show:<id>" or "episode:<show>:<season>:<episode>".
CREATE TABLE IF NOT EXISTS credits (
	owner TEXT NOT NULL,
	person_id INTEGER NOT NULL,
	role TEXT NOT NULL,            -- "cast", "guest", "director", "writer", "creator"
	character TEXT,
	ord INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_credits_owner ON credits(owner);
`

// Lookup is the result of matching one title query against TMDB.
type Lookup struct {
	Kind         string
	LibraryID    string
	QueryKey     string
	QueryTitle   string
	QueryYear    int
	QueryCountry string
	TMDBID       int
	Status       string
	AttemptedAt  time.Time
	Manual       bool // set by hand in the web UI; automatic matching never changes it
}

// Movie and Show artwork fields hold paths relative to the configured artwork directory.
type Movie struct {
	TMDBID        int
	Title         string
	OriginalTitle string
	Year          int
	ReleaseDate   string
	Overview      string
	Tagline       string
	Runtime       int
	Genres        string // comma-separated
	IMDBID        string
	Poster        string
	Backdrop      string
	Logo          string
	UpdatedAt     time.Time
}

type Show struct {
	TMDBID       int
	Name         string
	OriginalName string
	Year         int
	FirstAirDate string
	Overview     string
	Tagline      string
	Genres       string // comma-separated
	IMDBID       string
	Runtime      int
	Poster       string
	Backdrop     string
	Logo         string
	UpdatedAt    time.Time
}

type Season struct {
	ShowID       int
	SeasonNumber int
	Name         string
	Overview     string
	Poster       string
	Status       string
	FetchedAt    time.Time
}

type Episode struct {
	ShowID        int
	SeasonNumber  int
	EpisodeNumber int
	TMDBID        int
	Name          string
	Overview      string
	AirDate       string
	Runtime       int
	Still         string
	UpdatedAt     time.Time
}

type Person struct {
	TMDBID  int
	Name    string
	Profile string
}

type Credit struct {
	Person    Person
	Role      string
	Character string
	Order     int
}

func MovieOwner(id int) string { return "movie:" + itoa(id) }
func ShowOwner(id int) string  { return "show:" + itoa(id) }
func EpisodeOwner(show, season, episode int) string {
	return "episode:" + itoa(show) + ":" + itoa(season) + ":" + itoa(episode)
}

func (db *DB) initMetadataSchema() error {
	if _, err := db.mediaConn.Exec(metadataSchema); err != nil {
		return err
	}
	_, _ = db.mediaConn.Exec("ALTER TABLE metadata_lookups ADD COLUMN manual INTEGER NOT NULL DEFAULT 0")
	return nil
}

// GetLookup returns the lookup for a query, or nil if it was never attempted.
func (db *DB) GetLookup(kind, libraryID, key string) (*Lookup, error) {
	l := &Lookup{}
	err := db.mediaConn.QueryRow(`
		SELECT kind, library_id, query_key, query_title, query_year, query_country, tmdb_id, status, attempted_at, manual
		FROM metadata_lookups WHERE kind = ? AND library_id = ? AND query_key = ?`, kind, libraryID, key).
		Scan(&l.Kind, &l.LibraryID, &l.QueryKey, &l.QueryTitle, &l.QueryYear, &l.QueryCountry, &l.TMDBID, &l.Status, &l.AttemptedAt, &l.Manual)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return l, err
}

func (db *DB) SaveLookup(l *Lookup) error {
	_, err := db.mediaConn.Exec(`
		INSERT OR REPLACE INTO metadata_lookups
		(kind, library_id, query_key, query_title, query_year, query_country, tmdb_id, status, attempted_at, manual)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.Kind, l.LibraryID, l.QueryKey, l.QueryTitle, l.QueryYear, l.QueryCountry, l.TMDBID, l.Status, l.AttemptedAt, l.Manual)
	return err
}

// DeleteLookup forgets a lookup, so the title is matched automatically again.
func (db *DB) DeleteLookup(kind, libraryID, key string) error {
	_, err := db.mediaConn.Exec(`DELETE FROM metadata_lookups WHERE kind = ? AND library_id = ? AND query_key = ?`, kind, libraryID, key)
	return err
}

// LibrariesForTMDB lists the libraries holding a file matched to the given movie or show.
func (db *DB) LibrariesForTMDB(kind string, tmdbID int) ([]string, error) {
	rows, err := db.mediaConn.Query(`SELECT DISTINCT library_id FROM metadata_lookups
		WHERE kind = ? AND tmdb_id = ? AND status = 'matched'`, kind, tmdbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var libs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		libs = append(libs, id)
	}
	return libs, rows.Err()
}

// MatchedTMDBIDs returns every movie or show ID still referenced by a lookup.
func (db *DB) MatchedTMDBIDs(kind string) (map[int]bool, error) {
	rows, err := db.mediaConn.Query(`SELECT DISTINCT tmdb_id FROM metadata_lookups WHERE kind = ? AND status = 'matched'`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// PruneLibraryMetadata removes lookups and folder mappings for a library that no file uses any
// more. keepKeys and keepFolders are what a full walk of the library found.
func (db *DB) PruneLibraryMetadata(kind, libraryID string, keepKeys, keepFolders map[string]bool) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := pruneColumn(tx, `SELECT query_key FROM metadata_lookups WHERE kind = ? AND library_id = ?`,
		`DELETE FROM metadata_lookups WHERE kind = ? AND library_id = ? AND query_key = ?`, kind, libraryID, keepKeys); err != nil {
		return err
	}
	if err := pruneColumn(tx, `SELECT folder_path FROM metadata_folders WHERE kind = ? AND library_id = ?`,
		`DELETE FROM metadata_folders WHERE kind = ? AND library_id = ? AND folder_path = ?`, kind, libraryID, keepFolders); err != nil {
		return err
	}
	return tx.Commit()
}

func pruneColumn(tx *sql.Tx, selectQuery, deleteQuery, kind, libraryID string, keep map[string]bool) error {
	rows, err := tx.Query(selectQuery, kind, libraryID)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		if !keep[v] {
			stale = append(stale, v)
		}
	}
	rows.Close()
	for _, v := range stale {
		if _, err := tx.Exec(deleteQuery, kind, libraryID, v); err != nil {
			return err
		}
	}
	return nil
}

// SetFolderKey records that a folder stands for one title.
func (db *DB) SetFolderKey(libraryID, folderPath, kind, key string) error {
	_, err := db.mediaConn.Exec(`INSERT OR REPLACE INTO metadata_folders (library_id, folder_path, kind, query_key)
		VALUES (?, ?, ?, ?)`, libraryID, folderPath, kind, key)
	return err
}

// FolderKeys returns the lookup keys recorded for the given folders of one library.
func (db *DB) FolderKeys(libraryID string, folders []string) (map[string]string, error) {
	keys := map[string]string{}
	if len(folders) == 0 {
		return keys, nil
	}
	args := []any{libraryID}
	for _, f := range folders {
		args = append(args, f)
	}
	rows, err := db.mediaConn.Query(`SELECT folder_path, query_key FROM metadata_folders
		WHERE library_id = ? AND folder_path IN (`+placeholders(len(folders))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var folder, key string
		if err := rows.Scan(&folder, &key); err != nil {
			return nil, err
		}
		keys[folder] = key
	}
	return keys, rows.Err()
}

func (db *DB) GetMovie(id int) (*Movie, error) {
	m := &Movie{}
	err := db.mediaConn.QueryRow(`SELECT tmdb_id, title, original_title, year, release_date, overview, tagline,
		runtime, genres, imdb_id, poster, backdrop, logo, updated_at FROM movies WHERE tmdb_id = ?`, id).
		Scan(&m.TMDBID, &m.Title, &m.OriginalTitle, &m.Year, &m.ReleaseDate, &m.Overview, &m.Tagline,
			&m.Runtime, &m.Genres, &m.IMDBID, &m.Poster, &m.Backdrop, &m.Logo, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (db *DB) GetShow(id int) (*Show, error) {
	s := &Show{}
	err := db.mediaConn.QueryRow(`SELECT tmdb_id, name, original_name, year, first_air_date, overview, tagline,
		genres, imdb_id, runtime, poster, backdrop, logo, updated_at FROM shows WHERE tmdb_id = ?`, id).
		Scan(&s.TMDBID, &s.Name, &s.OriginalName, &s.Year, &s.FirstAirDate, &s.Overview, &s.Tagline,
			&s.Genres, &s.IMDBID, &s.Runtime, &s.Poster, &s.Backdrop, &s.Logo, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (db *DB) GetSeason(showID, season int) (*Season, error) {
	s := &Season{}
	err := db.mediaConn.QueryRow(`SELECT show_id, season_number, COALESCE(name, ''), COALESCE(overview, ''),
		COALESCE(poster, ''), status, fetched_at FROM seasons WHERE show_id = ? AND season_number = ?`, showID, season).
		Scan(&s.ShowID, &s.SeasonNumber, &s.Name, &s.Overview, &s.Poster, &s.Status, &s.FetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (db *DB) GetEpisode(showID, season, episode int) (*Episode, error) {
	e := &Episode{}
	err := db.mediaConn.QueryRow(`SELECT show_id, season_number, episode_number, tmdb_id, name, overview,
		air_date, runtime, still, updated_at FROM episodes
		WHERE show_id = ? AND season_number = ? AND episode_number = ?`, showID, season, episode).
		Scan(&e.ShowID, &e.SeasonNumber, &e.EpisodeNumber, &e.TMDBID, &e.Name, &e.Overview,
			&e.AirDate, &e.Runtime, &e.Still, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// SaveMovie stores a movie and replaces its credits.
func (db *DB) SaveMovie(m *Movie, credits []Credit) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO movies (tmdb_id, title, original_title, year, release_date,
		overview, tagline, runtime, genres, imdb_id, poster, backdrop, logo, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TMDBID, m.Title, m.OriginalTitle, m.Year, m.ReleaseDate, m.Overview, m.Tagline, m.Runtime,
		m.Genres, m.IMDBID, m.Poster, m.Backdrop, m.Logo, m.UpdatedAt); err != nil {
		return err
	}
	if err := replaceCredits(tx, MovieOwner(m.TMDBID), credits); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveShow stores a show and replaces its credits.
func (db *DB) SaveShow(s *Show, credits []Credit) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO shows (tmdb_id, name, original_name, year, first_air_date,
		overview, tagline, genres, imdb_id, runtime, poster, backdrop, logo, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.TMDBID, s.Name, s.OriginalName, s.Year, s.FirstAirDate, s.Overview, s.Tagline, s.Genres,
		s.IMDBID, s.Runtime, s.Poster, s.Backdrop, s.Logo, s.UpdatedAt); err != nil {
		return err
	}
	if err := replaceCredits(tx, ShowOwner(s.TMDBID), credits); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveSeason stores a season with its episodes and their credits (keyed by episode number).
func (db *DB) SaveSeason(s *Season, episodes []Episode, credits map[int][]Credit) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO seasons (show_id, season_number, name, overview, poster, status, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.ShowID, s.SeasonNumber, s.Name, s.Overview, s.Poster, s.Status, s.FetchedAt); err != nil {
		return err
	}
	for _, e := range episodes {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO episodes (show_id, season_number, episode_number, tmdb_id,
			name, overview, air_date, runtime, still, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ShowID, e.SeasonNumber, e.EpisodeNumber, e.TMDBID, e.Name, e.Overview, e.AirDate, e.Runtime,
			e.Still, e.UpdatedAt); err != nil {
			return err
		}
		if err := replaceCredits(tx, EpisodeOwner(e.ShowID, e.SeasonNumber, e.EpisodeNumber), credits[e.EpisodeNumber]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveSeasonStatus records a failed or empty season fetch without touching its episodes.
func (db *DB) SaveSeasonStatus(showID, season int, status string, at time.Time) error {
	_, err := db.mediaConn.Exec(`INSERT INTO seasons (show_id, season_number, status, fetched_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(show_id, season_number) DO UPDATE SET status = excluded.status, fetched_at = excluded.fetched_at`,
		showID, season, status, at)
	return err
}

// DeleteTitle removes a movie or show (with its seasons, episodes and credits), so matching it
// again fetches everything fresh. People are kept: they're shared with other titles.
func (db *DB) DeleteTitle(kind string, id int) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var stmts []string
	var args [][]any
	if kind == "movie" {
		stmts = []string{`DELETE FROM movies WHERE tmdb_id = ?`, `DELETE FROM credits WHERE owner = ?`}
		args = [][]any{{id}, {MovieOwner(id)}}
	} else {
		stmts = []string{`DELETE FROM shows WHERE tmdb_id = ?`, `DELETE FROM seasons WHERE show_id = ?`,
			`DELETE FROM episodes WHERE show_id = ?`, `DELETE FROM credits WHERE owner = ? OR owner LIKE ?`}
		args = [][]any{{id}, {id}, {id}, {ShowOwner(id), "episode:" + itoa(id) + ":%"}}
	}
	for i, stmt := range stmts {
		if _, err := tx.Exec(stmt, args[i]...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteSeasons forgets a show's fetched seasons so they're fetched again.
func (db *DB) DeleteSeasons(showID int) error {
	_, err := db.mediaConn.Exec(`DELETE FROM seasons WHERE show_id = ?`, showID)
	return err
}

// Credits returns an owner's credits in billing order, with each person's details.
func (db *DB) Credits(owner string) ([]Credit, error) {
	rows, err := db.mediaConn.Query(`SELECT c.role, c.character, c.ord, p.tmdb_id, p.name, p.profile
		FROM credits c JOIN people p ON p.tmdb_id = c.person_id WHERE c.owner = ? ORDER BY c.role, c.ord`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var credits []Credit
	for rows.Next() {
		var c Credit
		var character, profile sql.NullString
		if err := rows.Scan(&c.Role, &character, &c.Order, &c.Person.TMDBID, &c.Person.Name, &profile); err != nil {
			return nil, err
		}
		c.Character, c.Person.Profile = character.String, profile.String
		credits = append(credits, c)
	}
	return credits, rows.Err()
}

// PersonProfile returns a person's downloaded headshot path, if any.
func (db *DB) PersonProfile(id int) (string, error) {
	var profile sql.NullString
	err := db.mediaConn.QueryRow(`SELECT profile FROM people WHERE tmdb_id = ?`, id).Scan(&profile)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return profile.String, err
}

func replaceCredits(tx *sql.Tx, owner string, credits []Credit) error {
	if _, err := tx.Exec(`DELETE FROM credits WHERE owner = ?`, owner); err != nil {
		return err
	}
	for _, c := range credits {
		// Keep an existing headshot when this credit didn't download one.
		if _, err := tx.Exec(`INSERT INTO people (tmdb_id, name, profile) VALUES (?, ?, ?)
			ON CONFLICT(tmdb_id) DO UPDATE SET name = excluded.name,
			profile = CASE WHEN excluded.profile != '' THEN excluded.profile ELSE people.profile END`,
			c.Person.TMDBID, c.Person.Name, c.Person.Profile); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO credits (owner, person_id, role, character, ord) VALUES (?, ?, ?, ?, ?)`,
			owner, c.Person.TMDBID, c.Role, c.Character, c.Order); err != nil {
			return err
		}
	}
	return nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
