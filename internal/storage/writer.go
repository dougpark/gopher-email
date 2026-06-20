package storage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// WriteResult holds the outcome of a successful write.
type WriteResult struct {
	Path     string // relative path under storageRoot, e.g. storage/2026/06/19/<uuid>.eml
	Checksum string // hex-encoded SHA-256 of the file contents
}

// Write saves rawEML under <storageRoot>/YYYY/MM/DD/<uuid>.eml.
// It returns the relative path and SHA-256 checksum of the written file.
func Write(storageRoot string, rawEML []byte, date time.Time) (WriteResult, error) {
	dir := filepath.Join(
		storageRoot,
		date.UTC().Format("2006"),
		date.UTC().Format("01"),
		date.UTC().Format("02"),
	)

	if err := os.MkdirAll(dir, 0750); err != nil {
		return WriteResult{}, fmt.Errorf("creating storage directory %q: %w", dir, err)
	}

	filename := uuid.New().String() + ".eml"
	absPath := filepath.Join(dir, filename)

	if err := os.WriteFile(absPath, rawEML, 0640); err != nil {
		return WriteResult{}, fmt.Errorf("writing eml file %q: %w", absPath, err)
	}

	sum := sha256.Sum256(rawEML)
	return WriteResult{
		Path:     absPath,
		Checksum: fmt.Sprintf("%x", sum),
	}, nil
}

// Exists returns true if the given path exists on the filesystem.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
