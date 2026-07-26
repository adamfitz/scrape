// Package database provides a pure data layer for SQLite storage.
// It contains no normalisation logic — that belongs in the pipeline layer.
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// MediaType identifies a category of media.
type MediaType int

const (
	MediaTypeManga MediaType = iota
	MediaTypeAnime
	MediaTypeLightNovel
	MediaTypeWebNovel
	MediaTypeWebtoon
)

// AllMediaTypes returns every supported MediaType.
// Used by pipeline and maintenance commands to iterate all tables.
func AllMediaTypes() []MediaType {
	return []MediaType{
		MediaTypeManga,
		MediaTypeAnime,
		MediaTypeLightNovel,
		MediaTypeWebNovel,
		MediaTypeWebtoon,
	}
}

// TableName returns the SQLite table name for the media type.
// Used by all database queries to target the correct table.
func (t MediaType) TableName() string {
	switch t {
	case MediaTypeManga:
		return "manga"
	case MediaTypeAnime:
		return "anime"
	case MediaTypeLightNovel:
		return "lightnovel"
	case MediaTypeWebNovel:
		return "webnovel"
	case MediaTypeWebtoon:
		return "webtoons"
	default:
		return "manga"
	}
}

// Media represents a single title-bearing record, shared across all media types.
type Media struct {
	ID          int64
	Type        MediaType
	Title       string
	AltTitle    string // JSON — structured or flat format
	Author      string
	Description string
	CoverURL    string
	URL         string
	SourceID    string // MangaDex ID, AniList ID, etc.
	Language    string // Original language code (e.g. "en", "ja")
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DB wraps a SQLite database connection.
type DB struct {
	conn *sql.DB
	path string
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{conn: conn, path: path}
	if err := db.EnsureSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Path returns the database file path, used for display in stats/backup output.
func (db *DB) Path() string {
	return db.path
}

// EnsureSchema creates all tables and indexes if they don't exist.
// Called once by Open on database creation.
func (db *DB) EnsureSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS manga (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT NOT NULL,
		alt_title   TEXT,
		author      TEXT,
		description TEXT,
		cover_url   TEXT,
		url         TEXT,
		source_id   TEXT UNIQUE,
		language    TEXT,
		status      TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS observation (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		media_id    INTEGER NOT NULL,
		media_type  TEXT NOT NULL,
		progress    TEXT,
		title       TEXT,
		url         TEXT,
		status      TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS bookmarks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		media_id    INTEGER NOT NULL,
		media_type  TEXT NOT NULL,
		chapter_id  INTEGER,
		note        TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS anime (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT NOT NULL,
		alt_title   TEXT,
		author      TEXT,
		description TEXT,
		cover_url   TEXT,
		url         TEXT,
		source_id   TEXT UNIQUE,
		language    TEXT,
		status      TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS lightnovel (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT NOT NULL,
		alt_title   TEXT,
		author      TEXT,
		description TEXT,
		cover_url   TEXT,
		url         TEXT,
		volumes     INTEGER,
		source_id   TEXT UNIQUE,
		language    TEXT,
		status      TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS webnovel (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT NOT NULL,
		alt_title   TEXT,
		author      TEXT,
		description TEXT,
		cover_url   TEXT,
		url         TEXT,
		source_id   TEXT UNIQUE,
		language    TEXT,
		status      TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS webtoons (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT NOT NULL,
		alt_title   TEXT,
		author      TEXT,
		description TEXT,
		cover_url   TEXT,
		url         TEXT,
		source_id   TEXT UNIQUE,
		language    TEXT,
		status      TEXT,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS title_index (
		media_type     TEXT NOT NULL,
		media_id       INTEGER NOT NULL,
		normalised     TEXT NOT NULL,
		original_title TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (media_id) REFERENCES manga(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_title_index_lookup
		ON title_index(media_type, normalised);
	CREATE INDEX IF NOT EXISTS idx_title_index_media
		ON title_index(media_id);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// AllMedia fetches every row for the given media type.
// Used for in-memory matching in the pipeline layer.
// Selects only columns common to all media tables; media-specific fields
// (author, description, cover_url) are populated only for manga.
func (db *DB) AllMedia(mediaType MediaType) ([]Media, error) {
	table := mediaType.TableName()

	// All tables share: id, title, alt_title, author, description, cover_url,
	// url, source_id, status, created_at, updated_at.
	query := fmt.Sprintf(
		`SELECT id, title, alt_title, url, source_id, language, status, created_at, updated_at,
		        author, description, cover_url
		 FROM %s`, table)

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []Media
	for rows.Next() {
		var m Media
		m.Type = mediaType
		if err := rows.Scan(&m.ID, &m.Title, &m.AltTitle, &m.URL, &m.SourceID,
			&m.Language, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.Author, &m.Description, &m.CoverURL); err != nil {
			continue
		}
		all = append(all, m)
	}
	return all, nil
}

// GetMediaBySourceID performs an exact lookup by source-specific ID
// (e.g. MangaDex ID, AniList ID). Searches across all media tables.
func (db *DB) GetMediaBySourceID(id string) (*Media, error) {
	for _, mt := range AllMediaTypes() {
		table := mt.TableName()
		query := fmt.Sprintf(
			`SELECT id, title, alt_title, url, source_id, language, status, created_at, updated_at,
			        author, description, cover_url
			 FROM %s WHERE source_id = ?`, table)

		row := db.conn.QueryRow(query, id)
		var m Media
		m.Type = mt
		err := row.Scan(&m.ID, &m.Title, &m.AltTitle, &m.URL, &m.SourceID,
			&m.Language, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.Author, &m.Description, &m.CoverURL)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("query %s by source_id: %w", table, err)
		}
		return &m, nil
	}
	return nil, nil
}

// InsertMedia inserts a new record and returns the generated ID.
// Used by UpsertMedia when no existing record matches the SourceID.
func (db *DB) InsertMedia(m *Media) (int64, error) {
	table := m.Type.TableName()
	query := fmt.Sprintf(
		`INSERT INTO %s (title, alt_title, author, description, cover_url, url, source_id, language, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, table)

	result, err := db.conn.Exec(query,
		m.Title, m.AltTitle, m.Author, m.Description, m.CoverURL, m.URL, m.SourceID, m.Language, m.Status)
	if err != nil {
		return 0, fmt.Errorf("insert %s: %w", table, err)
	}
	return result.LastInsertId()
}

// UpdateMedia updates an existing record by ID.
func (db *DB) UpdateMedia(m *Media) error {
	table := m.Type.TableName()
	query := fmt.Sprintf(
		`UPDATE %s SET title=?, alt_title=?, author=?, description=?, cover_url=?, url=?, source_id=?, language=?, status=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`, table)

	_, err := db.conn.Exec(query,
		m.Title, m.AltTitle, m.Author, m.Description, m.CoverURL, m.URL, m.SourceID, m.Language, m.Status, m.ID)
	if err != nil {
		return fmt.Errorf("update %s: %w", table, err)
	}
	return nil
}

// UpsertMedia inserts or updates a record. If SourceID is set and a matching
// record exists, it is updated; otherwise a new record is inserted.
func (db *DB) UpsertMedia(m *Media) (int64, error) {
	if m.SourceID != "" {
		existing, err := db.GetMediaBySourceID(m.SourceID)
		if err != nil {
			return 0, err
		}
		if existing != nil {
			m.ID = existing.ID
			m.Type = existing.Type
			return existing.ID, db.UpdateMedia(m)
		}
	}
	return db.InsertMedia(m)
}

// Backup creates a backup of the database using VACUUM INTO.
func (db *DB) Backup(dest string) error {
	_, err := db.conn.Exec(fmt.Sprintf("VACUUM INTO '%s'", dest))
	if err != nil {
		return fmt.Errorf("backup database: %w", err)
	}
	return nil
}

// Vacuum compacts the database.
func (db *DB) Vacuum() error {
	_, err := db.conn.Exec("VACUUM")
	return err
}

// IntegrityCheck runs PRAGMA integrity_check and returns the result.
func (db *DB) IntegrityCheck() (string, error) {
	var result string
	err := db.conn.QueryRow("PRAGMA integrity_check").Scan(&result)
	return result, err
}

// Stats returns record counts for every table in the database.
// Used by the `maintenance stats` command.
func (db *DB) Stats() (map[string]int, error) {
	tables := []string{"manga", "observation", "bookmarks", "anime", "lightnovel", "webnovel", "webtoons"}
	stats := make(map[string]int, len(tables))

	for _, table := range tables {
		var count int
		err := db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		stats[table] = count
	}

	return stats, nil
}

// --- Title Index ---

// IndexTitle adds a single normalised title to the index.
func (db *DB) IndexTitle(mediaType MediaType, mediaID int64, normalised, original string) error {
	table := mediaType.TableName()
	_, err := db.conn.Exec(
		`INSERT OR REPLACE INTO title_index (media_type, media_id, normalised, original_title)
		 VALUES (?, ?, ?, ?)`,
		table, mediaID, normalised, original)
	return err
}

// IndexTitles replaces all index entries for a record with the given normalised titles.
func (db *DB) IndexTitles(mediaType MediaType, mediaID int64, titles map[string]string) error {
	table := mediaType.TableName()
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM title_index WHERE media_type = ? AND media_id = ?`, table, mediaID); err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO title_index (media_type, media_id, normalised, original_title)
		 VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for normalised, original := range titles {
		if _, err := stmt.Exec(table, mediaID, normalised, original); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ClearTitleIndex removes all index entries for a single media record.
func (db *DB) ClearTitleIndex(mediaType MediaType, mediaID int64) error {
	table := mediaType.TableName()
	_, err := db.conn.Exec(
		`DELETE FROM title_index WHERE media_type = ? AND media_id = ?`,
		table, mediaID)
	return err
}

// RebuildTitleIndex clears all index entries for a media type.
// The pipeline layer is responsible for re-populating the index.
func (db *DB) RebuildTitleIndex(mediaType MediaType) error {
	table := mediaType.TableName()
	_, err := db.conn.Exec(`DELETE FROM title_index WHERE media_type = ?`, table)
	return err
}

// TitleIndexSize returns the number of entries in the title index for one media type.
// Used by ensureTitleIndex to decide whether a rebuild is needed.
func (db *DB) TitleIndexSize(mediaType MediaType) (int, error) {
	table := mediaType.TableName()
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM title_index WHERE media_type = ?`, table).Scan(&count)
	return count, err
}

// TitleIndexSizeTotal returns the total index entries across all media types.
func (db *DB) TitleIndexSizeTotal() (int, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM title_index`).Scan(&count)
	return count, err
}

// QueryTitleIndex looks up a normalised title in the index and returns the media ID.
func (db *DB) QueryTitleIndex(mediaType MediaType, normalised string) (int64, error) {
	table := mediaType.TableName()
	var mediaID int64
	err := db.conn.QueryRow(
		`SELECT media_id FROM title_index WHERE media_type = ? AND normalised = ? LIMIT 1`,
		table, normalised).Scan(&mediaID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return mediaID, err
}

// BatchQueryTitleIndex looks up multiple normalised titles in the index.
// Returns a map of normalised title to media ID for all matches.
func (db *DB) BatchQueryTitleIndex(mediaType MediaType, normalised []string) (map[string]int64, error) {
	if len(normalised) == 0 {
		return nil, nil
	}
	table := mediaType.TableName()

	// Build placeholders for IN clause
	placeholders := make([]string, len(normalised))
	args := make([]interface{}, 0, len(normalised)+1)
	args = append(args, table)
	for i, n := range normalised {
		placeholders[i] = "?"
		args = append(args, n)
	}
	query := fmt.Sprintf(
		`SELECT normalised, media_id FROM title_index WHERE media_type = ? AND normalised IN (%s)`,
		strings.Join(placeholders, ","))

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var norm string
		var id int64
		if err := rows.Scan(&norm, &id); err != nil {
			continue
		}
		result[norm] = id
	}
	return result, nil
}

// GetMediaByID fetches a media record by primary key.
func (db *DB) GetMediaByID(mediaType MediaType, id int64) (*Media, error) {
	table := mediaType.TableName()
	query := fmt.Sprintf(
		`SELECT id, title, alt_title, url, source_id, language, status, created_at, updated_at,
		        author, description, cover_url
		 FROM %s WHERE id = ?`, table)

	var m Media
	m.Type = mediaType
	err := db.conn.QueryRow(query, id).Scan(
		&m.ID, &m.Title, &m.AltTitle, &m.URL, &m.SourceID,
		&m.Language, &m.Status, &m.CreatedAt, &m.UpdatedAt,
		&m.Author, &m.Description, &m.CoverURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get %s by id: %w", table, err)
	}
	return &m, nil
}
