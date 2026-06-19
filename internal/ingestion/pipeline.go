package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
	"os"

	"github.com/emersion/go-message/mail"

	"github.com/dougpark/gopher-email/internal/db"
	"github.com/dougpark/gopher-email/internal/gmail"
	"github.com/dougpark/gopher-email/internal/storage"
)

// Pipeline orchestrates the atomic ingestion of a single Gmail message.
type Pipeline struct {
	db           *db.DB
	gmail        *gmail.Client
	storageRoot  string
	inboundLabel string
	archiveLabel string
	verbose      bool
}

// New creates a Pipeline with all required dependencies.
func New(database *db.DB, gmailClient *gmail.Client, storageRoot, inboundLabel, archiveLabel string, verbose bool) *Pipeline {
	return &Pipeline{
		db:           database,
		gmail:        gmailClient,
		storageRoot:  storageRoot,
		inboundLabel: inboundLabel,
		archiveLabel: archiveLabel,
		verbose:      verbose,
	}
}

// Process runs the full atomic ingestion for a single Gmail message ID.
// If any step fails before the label swap, the message stays in the inbound
// label so the next run will retry it.
func (p *Pipeline) Process(ctx context.Context, msgID string) error {
	// Step 1 — Idempotency check.
	exists, err := p.db.Exists(msgID)
	if err != nil {
		return fmt.Errorf("db.Exists: %w", err)
	}
	if exists {
		p.logf("skipping %s (already processed)", msgID)
		return nil
	}

	// Step 2 — Fetch raw RFC 2822 bytes from Gmail.
	p.logf("fetching %s", msgID)
	rawEML, err := p.gmail.GetRaw(ctx, msgID)
	if err != nil {
		return fmt.Errorf("GetRaw: %w", err)
	}

	// Step 3 — Parse headers with go-message/mail.
	headers, err := parseHeaders(rawEML)
	if err != nil {
		// Non-fatal: continue ingestion with whatever we parsed.
		log.Printf("warning: partial header parse for %s: %v", msgID, err)
	}

	date := headers.date
	if date.IsZero() {
		date = time.Now().UTC()
	}

	// Step 4 — Write .eml to filesystem.
	result, err := storage.Write(p.storageRoot, rawEML, date)
	if err != nil {
		return fmt.Errorf("storage.Write: %w", err)
	}
	p.logf("wrote %s → %s", msgID, result.Path)

	// Step 5 — Build metadata JSON (all headers).
	metaJSON, err := json.Marshal(headers.allHeaders)
	if err != nil {
		metaJSON = []byte("{}")
	}
	// Embed checksum in metadata.
	var metaMap map[string]interface{}
	_ = json.Unmarshal(metaJSON, &metaMap)
	if metaMap == nil {
		metaMap = make(map[string]interface{})
	}
	metaMap["_checksum_sha256"] = result.Checksum
	metaJSON, _ = json.Marshal(metaMap)

	// Step 6 — Insert into SQLite.
	item := db.EmailArchiveItem{
		ID:          msgID,
		SourceType:  "email",
		CreatedAt:   date,
		ProcessedAt: time.Now().UTC(),
		Sender:      headers.sender,
		Subject:     headers.subject,
		StoragePath: result.Path,
		Metadata:    string(metaJSON),
	}
	if err := p.db.Insert(item); err != nil {
		// Attempt to clean up the written file to avoid orphans.
		_ = removeFile(result.Path)
		return fmt.Errorf("db.Insert: %w", err)
	}

	// Step 7 — Verify file still on disk, then swap labels.
	if !storage.Exists(result.Path) {
		_ = p.db.Delete(msgID) // best-effort rollback
		return fmt.Errorf("file missing after write: %s", result.Path)
	}

	if err := p.gmail.BatchModify(ctx, []string{msgID}, []string{p.inboundLabel}, []string{p.archiveLabel}); err != nil {
		// SQLite row and file are present; label swap failed. Log and leave in
		// inbound label — the next run's idempotency check prevents duplication.
		log.Printf("warning: label swap failed for %s (will retry next run): %v", msgID, err)
		return nil
	}

	p.logf("processed %s OK", msgID)
	return nil
}

// parsedHeaders holds the fields we care about from the email headers.
type parsedHeaders struct {
	sender     string
	subject    string
	date       time.Time
	allHeaders map[string][]string
}

// parseHeaders extracts key headers and all header fields from raw RFC 2822 bytes.
func parseHeaders(rawEML []byte) (parsedHeaders, error) {
	r, err := mail.CreateReader(bytes.NewReader(rawEML))
	if err != nil {
		return parsedHeaders{}, fmt.Errorf("creating mail reader: %w", err)
	}
	defer r.Close()

	h := r.Header
	result := parsedHeaders{
		allHeaders: make(map[string][]string),
	}

	// Extract well-known fields.
	if addrs, err := h.AddressList("From"); err == nil && len(addrs) > 0 {
		result.sender = addrs[0].String()
	}
	if subj, err := h.Subject(); err == nil {
		result.subject = subj
	}
	if d, err := h.Date(); err == nil {
		result.date = d
	}

	// Capture all header fields as-is for the metadata JSON column.
	fields := h.Fields()
	for fields.Next() {
		key := fields.Key()
		val, _ := fields.Text()
		result.allHeaders[key] = append(result.allHeaders[key], val)
	}

	return result, nil
}

func (p *Pipeline) logf(format string, args ...any) {
	if p.verbose {
		log.Printf("[ingestion] "+format, args...)
	}
}

// removeFile is a best-effort file removal used for cleanup on DB insert failure.
func removeFile(path string) error {
	return os.Remove(path)
}
