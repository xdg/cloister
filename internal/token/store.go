// Package token provides token generation, validation, and persistence.
package token

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultTokenDir returns the default directory for token storage.
// This is ~/.config/cloister/tokens on the host.
func DefaultTokenDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "cloister", "tokens"), nil
}

// Store handles persistent token storage using one file per cloister.
// File names are cloister names, file contents are tokens.
type Store struct {
	dir string
}

// NewStore creates a token store at the given directory.
// Creates the directory if it doesn't exist.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create token directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Save persists a token for a cloister.
// Overwrites any existing token for the same cloister name.
func (s *Store) Save(cloisterName, token string) error {
	path := filepath.Join(s.dir, cloisterName)
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}
	return nil
}

// Remove deletes the token for a cloister.
// Returns nil if the token file doesn't exist (idempotent).
func (s *Store) Remove(cloisterName string) error {
	path := filepath.Join(s.dir, cloisterName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove token: %w", err)
	}
	return nil
}

// Load reads all persisted tokens.
// Returns a map of token -> cloister name.
func (s *Store) Load() (map[string]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("failed to read token directory: %w", err)
	}

	tokens := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		cloisterName := entry.Name()
		path := filepath.Join(s.dir, cloisterName)

		data, err := os.ReadFile(path)
		if err != nil {
			// Skip unreadable files
			continue
		}

		token := strings.TrimSpace(string(data))
		if token != "" {
			tokens[token] = cloisterName
		}
	}

	return tokens, nil
}

// Dir returns the token storage directory.
func (s *Store) Dir() string {
	return s.dir
}
