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

CREATE TABLE IF NOT EXISTS run_stats (
	run_id                  TEXT PRIMARY KEY,
	run_type                TEXT NOT NULL,
	started_at              DATETIME NOT NULL,
	finished_at             DATETIME NOT NULL,
	duration_ms             INTEGER NOT NULL,
	status                  TEXT NOT NULL,
	inbound_label           TEXT,
	archive_label           TEXT,
	fetched_count           INTEGER NOT NULL DEFAULT 0,
	processed_ok_count      INTEGER NOT NULL DEFAULT 0,
	skipped_exists_count    INTEGER NOT NULL DEFAULT 0,
	failed_count            INTEGER NOT NULL DEFAULT 0,
	label_swap_error_count  INTEGER NOT NULL DEFAULT 0,
	message                 TEXT
);

CREATE TABLE IF NOT EXISTS system_status (
	id              INTEGER PRIMARY KEY CHECK (id = 1),
	last_run        DATETIME,
	last_status     TEXT,
	emails_fetched  INTEGER NOT NULL DEFAULT 0,
	emails_ingested INTEGER NOT NULL DEFAULT 0,
	total_archived  INTEGER NOT NULL DEFAULT 0,
	message         TEXT
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

// RunStat stores summary counters for a single command run.
type RunStat struct {
	RunID               string
	RunType             string
	StartedAt           time.Time
	FinishedAt          time.Time
	DurationMs          int64
	Status              string
	InboundLabel        string
	ArchiveLabel        string
	FetchedCount        int
	ProcessedOKCount    int
	SkippedExistsCount  int
	FailedCount         int
	LabelSwapErrorCount int
	Message             string
}

// SystemStatus is a one-row snapshot of the latest run.
type SystemStatus struct {
	LastRun        time.Time
	LastStatus     string
	EmailsFetched  int
	EmailsIngested int
	TotalArchived  int
	Message        string
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

	if _, err := conn.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	if _, err := conn.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("setting busy timeout: %w", err)
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

// CountArchived returns total rows in the archive table.
func (d *DB) CountArchived() (int, error) {
	var total int
	err := d.conn.QueryRow("SELECT COUNT(1) FROM email_archive_items").Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("counting archived rows: %w", err)
	}
	return total, nil
}

// InsertRunStat writes one run summary row.
func (d *DB) InsertRunStat(stat RunStat) error {
	const q = `
INSERT INTO run_stats
    (run_id, run_type, started_at, finished_at, duration_ms, status, inbound_label, archive_label,
     fetched_count, processed_ok_count, skipped_exists_count, failed_count, label_swap_error_count, message)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.conn.Exec(q,
		stat.RunID,
		stat.RunType,
		stat.StartedAt.UTC().Format(time.RFC3339),
		stat.FinishedAt.UTC().Format(time.RFC3339),
		stat.DurationMs,
		stat.Status,
		stat.InboundLabel,
		stat.ArchiveLabel,
		stat.FetchedCount,
		stat.ProcessedOKCount,
		stat.SkippedExistsCount,
		stat.FailedCount,
		stat.LabelSwapErrorCount,
		stat.Message,
	)
	if err != nil {
		return fmt.Errorf("inserting run stat %q: %w", stat.RunID, err)
	}
	return nil
}

// UpsertSystemStatus updates the single-row latest status snapshot.
func (d *DB) UpsertSystemStatus(status SystemStatus) error {
	const q = `
INSERT INTO system_status
    (id, last_run, last_status, emails_fetched, emails_ingested, total_archived, message)
VALUES
    (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    last_run = excluded.last_run,
    last_status = excluded.last_status,
    emails_fetched = excluded.emails_fetched,
    emails_ingested = excluded.emails_ingested,
    total_archived = excluded.total_archived,
    message = excluded.message`

	_, err := d.conn.Exec(q,
		status.LastRun.UTC().Format(time.RFC3339),
		status.LastStatus,
		status.EmailsFetched,
		status.EmailsIngested,
		status.TotalArchived,
		status.Message,
	)
	if err != nil {
		return fmt.Errorf("upserting system status: %w", err)
	}
	return nil
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
