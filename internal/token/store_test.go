package token

import (
	"os"
	"path/filepath"
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
	if err := store.Save("cloister-test-main", "abc123"); err != nil {
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
	if tokens["abc123"] != "cloister-test-main" {
		t.Errorf("Load returned wrong mapping: %v", tokens)
	}

	// Save another token
	if err := store.Save("cloister-other-dev", "def456"); err != nil {
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
