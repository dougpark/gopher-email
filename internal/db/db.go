package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS email_archive_items (
    id           TEXT PRIMARY KEY,
    source_type  TEXT    NOT NULL DEFAULT 'email',
    created_at   DATETIME,
    processed_at DATETIME,
    sender       TEXT,
    subject      TEXT,
    storage_path TEXT,
    metadata     JSON
);
`

// EmailArchiveItem mirrors the email_archive_items table row.
type EmailArchiveItem struct {
	ID          string
	SourceType  string
	CreatedAt   time.Time
	ProcessedAt time.Time
	Sender      string
	Subject     string
	StoragePath string
	Metadata    string // JSON-encoded header map
}

// DB wraps the SQLite connection.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db %q: %w", path, err)
	}

	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Exists returns true if a record with the given Gmail message ID already exists.
func (d *DB) Exists(id string) (bool, error) {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(1) FROM email_archive_items WHERE id = ?", id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking existence of %q: %w", id, err)
	}
	return count > 0, nil
}

// Insert writes a new EmailArchiveItem to the database.
func (d *DB) Insert(item EmailArchiveItem) error {
	const q = `
INSERT INTO email_archive_items
    (id, source_type, created_at, processed_at, sender, subject, storage_path, metadata)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.conn.Exec(q,
		item.ID,
		item.SourceType,
		item.CreatedAt.UTC().Format(time.RFC3339),
		item.ProcessedAt.UTC().Format(time.RFC3339),
		item.Sender,
		item.Subject,
		item.StoragePath,
		item.Metadata,
	)
	if err != nil {
		return fmt.Errorf("inserting item %q: %w", item.ID, err)
	}
	return nil
}

// Delete removes a record by ID. Used as a best-effort rollback.
func (d *DB) Delete(id string) error {
	_, err := d.conn.Exec("DELETE FROM email_archive_items WHERE id = ?", id)
	return err
}

// AllStoragePaths returns every storage_path currently in the database.
func (d *DB) AllStoragePaths() (map[string]struct{}, error) {
	rows, err := d.conn.Query("SELECT storage_path FROM email_archive_items WHERE storage_path IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("querying storage paths: %w", err)
	}
	defer rows.Close()

	paths := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths[p] = struct{}{}
	}
	return paths, rows.Err()
}
