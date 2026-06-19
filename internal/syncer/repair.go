package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"time"
	"os"

	"github.com/emersion/go-message/mail"

	"github.com/dougpark/gopher-email/internal/db"
)

// RepairOptions configures the sync/repair walk.
type RepairOptions struct {
	StoragePath string
	DBPath      string
	Verbose     bool
}

// Run walks the storage directory, finds .eml files not recorded in the
// database, parses their headers, and inserts the missing rows.
func Run(ctx context.Context, database *db.DB, opts RepairOptions) error {
	knownPaths, err := database.AllStoragePaths()
	if err != nil {
		return fmt.Errorf("loading known paths: %w", err)
	}

	var added, skipped int

	walkErr := filepath.WalkDir(opts.StoragePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("warning: cannot access %s: %v", path, err)
			return nil // keep walking
		}
		if d.IsDir() || filepath.Ext(path) != ".eml" {
			return nil
		}

		// Check whether this path is already known.
		if _, known := knownPaths[path]; known {
			skipped++
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if opts.Verbose {
			log.Printf("[sync] re-indexing %s", path)
		}

		if err := reindex(database, path); err != nil {
			log.Printf("warning: failed to re-index %s: %v", path, err)
		} else {
			added++
		}
		return nil
	})

	log.Printf("[sync] complete: %d added, %d already indexed", added, skipped)
	return walkErr
}

// reindex reads an .eml file, parses headers, and inserts a new DB record.
func reindex(database *db.DB, path string) error {
	rawEML, err := readFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	headers, parseErr := parseHeaders(rawEML)
	if parseErr != nil {
		log.Printf("warning: partial header parse for %s: %v", path, parseErr)
	}

	date := headers.date
	if date.IsZero() {
		date = time.Now().UTC()
	}

	metaJSON, _ := json.Marshal(headers.allHeaders)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}

	// Use the file path as a stable synthetic ID for re-indexed records.
	syntheticID := "sync:" + path

	exists, err := database.Exists(syntheticID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	return database.Insert(db.EmailArchiveItem{
		ID:          syntheticID,
		SourceType:  "email",
		CreatedAt:   date,
		ProcessedAt: time.Now().UTC(),
		Sender:      headers.sender,
		Subject:     headers.subject,
		StoragePath: path,
		Metadata:    string(metaJSON),
	})
}

type parsedHeaders struct {
	sender     string
	subject    string
	date       time.Time
	allHeaders map[string][]string
}

func parseHeaders(rawEML []byte) (parsedHeaders, error) {
	r, err := mail.CreateReader(bytes.NewReader(rawEML))
	if err != nil {
		return parsedHeaders{}, err
	}
	defer r.Close()

	h := r.Header
	result := parsedHeaders{allHeaders: make(map[string][]string)}

	if addrs, err := h.AddressList("From"); err == nil && len(addrs) > 0 {
		result.sender = addrs[0].String()
	}
	if subj, err := h.Subject(); err == nil {
		result.subject = subj
	}
	if d, err := h.Date(); err == nil {
		result.date = d
	}

	fields := h.Fields()
	for fields.Next() {
		key := fields.Key()
		val, _ := fields.Text()
		result.allHeaders[key] = append(result.allHeaders[key], val)
	}

	return result, nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
