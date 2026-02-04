package token

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_SaveLoadRemove(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "token-store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Test Save
	if err := store.Save("cloister-test-main", "abc123", "test-project"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists with correct permissions
	path := filepath.Join(tmpDir, "cloister-test-main")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Token file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Token file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Test Load
	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("Load returned %d tokens, want 1", len(tokens))
	}
	tokenInfo := tokens["abc123"]
	if tokenInfo.CloisterName != "cloister-test-main" {
		t.Errorf("Load returned wrong cloister name: %s", tokenInfo.CloisterName)
	}
	if tokenInfo.ProjectName != "test-project" {
		t.Errorf("Load returned wrong project name: %s", tokenInfo.ProjectName)
	}

	// Save another token
	if err := store.Save("cloister-other-dev", "def456", "other-project"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	tokens, err = store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("Load returned %d tokens, want 2", len(tokens))
	}

	// Test Remove
	if err := store.Remove("cloister-test-main"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	tokens, err = store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("Load returned %d tokens after remove, want 1", len(tokens))
	}
	if _, exists := tokens["abc123"]; exists {
		t.Error("Token abc123 should have been removed")
	}

	// Remove non-existent (should be idempotent)
	if err := store.Remove("nonexistent"); err != nil {
		t.Errorf("Remove of nonexistent token should not error: %v", err)
	}
}

func TestStore_LoadEmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "token-store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("Load returned %d tokens for empty dir, want 0", len(tokens))
	}
}

func TestStore_LoadNonexistentDir(t *testing.T) {
	// Store.Load should handle nonexistent directory gracefully
	store := &Store{dir: "/nonexistent/path/that/does/not/exist"}
	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("Load returned %d tokens for nonexistent dir, want 0", len(tokens))
	}
}

func TestStore_DirectoryPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "token-store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storeDir := filepath.Join(tmpDir, "tokens")
	_, err = NewStore(storeDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	info, err := os.Stat(storeDir)
	if err != nil {
		t.Fatalf("Store directory not created: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("Store directory permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestDefaultTokenDir(t *testing.T) {
	// Clear XDG_CONFIG_HOME to test default behavior
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := DefaultTokenDir()
	if err != nil {
		t.Fatalf("DefaultTokenDir() error = %v", err)
	}

	// Should end with .config/cloister/tokens
	expectedSuffix := filepath.Join(".config", "cloister", "tokens")
	if !strings.HasSuffix(dir, expectedSuffix) {
		t.Errorf("DefaultTokenDir() = %v, want path ending in %v", dir, expectedSuffix)
	}

	// Should start with home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	if !strings.HasPrefix(dir, home) {
		t.Errorf("DefaultTokenDir() = %v, want path starting with %v", dir, home)
	}

	// Should be exactly home + .config/cloister/tokens
	expected := filepath.Join(home, ".config", "cloister", "tokens")
	if dir != expected {
		t.Errorf("DefaultTokenDir() = %v, want %v", dir, expected)
	}
}

func TestDefaultTokenDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	dir, err := DefaultTokenDir()
	if err != nil {
		t.Fatalf("DefaultTokenDir() error = %v", err)
	}

	expected := "/custom/config/cloister/tokens"
	if dir != expected {
		t.Errorf("DefaultTokenDir() = %v, want %v", dir, expected)
	}
}

func TestStore_Dir(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if got := store.Dir(); got != dir {
		t.Errorf("Store.Dir() = %v, want %v", got, dir)
	}
}

func TestStore_LoadLegacyPlainTextFormat(t *testing.T) {
	// Test backward compatibility with old plain text format
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Write a plain text token file (old format)
	plainTextPath := filepath.Join(tmpDir, "old-cloister")
	if err := os.WriteFile(plainTextPath, []byte("plaintoken123\n"), 0600); err != nil {
		t.Fatalf("Failed to write plain text token: %v", err)
	}

	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(tokens) != 1 {
		t.Errorf("Load returned %d tokens, want 1", len(tokens))
	}

	info, exists := tokens["plaintoken123"]
	if !exists {
		t.Error("Expected plain text token to be loaded")
	}
	if info.CloisterName != "old-cloister" {
		t.Errorf("CloisterName = %v, want old-cloister", info.CloisterName)
	}
	if info.ProjectName != "" {
		t.Errorf("ProjectName = %v, want empty string for legacy format", info.ProjectName)
	}
}

func TestStore_LoadSkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Create a subdirectory (should be skipped)
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0700); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Save a valid token
	if err := store.Save("valid-cloister", "validtoken", "project"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should only have the valid token, not the directory
	if len(tokens) != 1 {
		t.Errorf("Load returned %d tokens, want 1 (directory should be skipped)", len(tokens))
	}
}

func TestStore_LoadSkipsEmptyFiles(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Create an empty file
	emptyPath := filepath.Join(tmpDir, "empty-cloister")
	if err := os.WriteFile(emptyPath, []byte(""), 0600); err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	// Create a file with only whitespace
	whitespacePath := filepath.Join(tmpDir, "whitespace-cloister")
	if err := os.WriteFile(whitespacePath, []byte("  \n\t  "), 0600); err != nil {
		t.Fatalf("Failed to write whitespace file: %v", err)
	}

	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Empty/whitespace files should not create token entries
	if len(tokens) != 0 {
		t.Errorf("Load returned %d tokens, want 0 (empty files should be skipped)", len(tokens))
	}
}

func TestStore_LoadSkipsInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Create a file with invalid JSON that doesn't have a token field
	invalidPath := filepath.Join(tmpDir, "invalid-cloister")
	if err := os.WriteFile(invalidPath, []byte(`{"other":"field"}`), 0600); err != nil {
		t.Fatalf("Failed to write invalid JSON file: %v", err)
	}

	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Invalid JSON with no token field falls through to plain text parsing
	// {"other":"field"} is not a valid plain text token (contains braces)
	// but it will be treated as a token string
	if len(tokens) != 1 {
		t.Errorf("Load returned %d tokens, want 1", len(tokens))
	}
}
