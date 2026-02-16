// Package token provides token generation, validation, and persistence.
package token //nolint:revive // intentional: does not conflict at import path level

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xdg/cloister/internal/config"
)

// tokenFile is the JSON structure for persisted tokens.
type tokenFile struct {
	Token    string `json:"token"`
	Project  string `json:"project,omitempty"`
	Worktree string `json:"worktree,omitempty"`
}

// DefaultTokenDir returns the default directory for token storage.
// This is config.Dir() + "tokens", which respects XDG_CONFIG_HOME.
func DefaultTokenDir() (string, error) {
	return config.Dir() + "tokens", nil
}

// Store handles persistent token storage using one file per cloister.
// File names are cloister names, file contents are tokens.
type Store struct {
	dir string
}

// NewStore creates a token store at the given directory.
// Creates the directory if it doesn't exist.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create token directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Save persists a token for a cloister.
// Overwrites any existing token for the same cloister name.
//
// Deprecated: Use SaveFull to include the worktree path.
func (s *Store) Save(cloisterName, token, projectName string) error {
	return s.SaveFull(cloisterName, token, projectName, "")
}

// SaveFull persists a token for a cloister with all metadata.
// Overwrites any existing token for the same cloister name.
func (s *Store) SaveFull(cloisterName, token, projectName, worktreePath string) error {
	path := filepath.Join(s.dir, cloisterName)
	data, err := json.Marshal(tokenFile{Token: token, Project: projectName, Worktree: worktreePath})
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
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
// Returns a map of token -> Info (cloister name and project name).
// Backward compatible: plain text files are treated as token-only (no project).
func (s *Store) Load() (map[string]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Info), nil
		}
		return nil, fmt.Errorf("failed to read token directory: %w", err)
	}

	tokens := make(map[string]Info)
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

		// Try to parse as JSON first (new format)
		var tf tokenFile
		if err := json.Unmarshal(data, &tf); err == nil && tf.Token != "" {
			tokens[tf.Token] = Info{
				CloisterName: cloisterName,
				ProjectName:  tf.Project,
				WorktreePath: tf.Worktree,
			}
			continue
		}

		// Fall back to plain text (old format: file contains just the token)
		token := strings.TrimSpace(string(data))
		if token != "" {
			tokens[token] = Info{CloisterName: cloisterName}
		}
	}

	return tokens, nil
}

// Dir returns the token storage directory.
func (s *Store) Dir() string {
	return s.dir
}
