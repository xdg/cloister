package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDaemonState_SaveLoadRemove(t *testing.T) {
	// Create temp directory for state file
	tmpDir, err := os.MkdirTemp("", "daemon-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override the state directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test saving state
	state := &DaemonState{
		PID:        12345,
		Secret:     "test-secret-abc",
		SocketPath: "/tmp/test.sock",
	}

	if err := SaveDaemonState(state); err != nil {
		t.Fatalf("SaveDaemonState() error: %v", err)
	}

	// Test loading state
	loaded, err := LoadDaemonState()
	if err != nil {
		t.Fatalf("LoadDaemonState() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadDaemonState() returned nil")
	}
	if loaded.PID != state.PID {
		t.Errorf("PID: got %d, want %d", loaded.PID, state.PID)
	}
	if loaded.Secret != state.Secret {
		t.Errorf("Secret: got %q, want %q", loaded.Secret, state.Secret)
	}
	if loaded.SocketPath != state.SocketPath {
		t.Errorf("SocketPath: got %q, want %q", loaded.SocketPath, state.SocketPath)
	}

	// Test removing state
	if err := RemoveDaemonState(); err != nil {
		t.Fatalf("RemoveDaemonState() error: %v", err)
	}

	// Verify state is gone
	loaded, err = LoadDaemonState()
	if err != nil {
		t.Fatalf("LoadDaemonState() after remove error: %v", err)
	}
	if loaded != nil {
		t.Error("Expected nil state after removal")
	}
}

func TestDaemonState_LoadNonExistent(t *testing.T) {
	// Create temp directory with no state file
	tmpDir, err := os.MkdirTemp("", "daemon-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Loading non-existent state should return nil, no error
	state, err := LoadDaemonState()
	if err != nil {
		t.Fatalf("LoadDaemonState() error: %v", err)
	}
	if state != nil {
		t.Error("Expected nil state for non-existent file")
	}
}

func TestIsDaemonRunning_NilState(t *testing.T) {
	if IsDaemonRunning(nil) {
		t.Error("Expected false for nil state")
	}
}

func TestIsDaemonRunning_ZeroPID(t *testing.T) {
	state := &DaemonState{PID: 0}
	if IsDaemonRunning(state) {
		t.Error("Expected false for zero PID")
	}
}

func TestIsDaemonRunning_CurrentProcess(t *testing.T) {
	// Our own process should be running
	state := &DaemonState{PID: os.Getpid()}
	if !IsDaemonRunning(state) {
		t.Error("Expected true for current process PID")
	}
}

func TestIsDaemonRunning_NonExistentProcess(t *testing.T) {
	// Use a very high PID that likely doesn't exist
	state := &DaemonState{PID: 999999999}
	if IsDaemonRunning(state) {
		t.Error("Expected false for non-existent PID")
	}
}

func TestStopDaemon_NilState(t *testing.T) {
	// Should not error on nil state
	if err := StopDaemon(nil); err != nil {
		t.Errorf("StopDaemon(nil) error: %v", err)
	}
}

func TestStopDaemon_ZeroPID(t *testing.T) {
	state := &DaemonState{PID: 0}
	if err := StopDaemon(state); err != nil {
		t.Errorf("StopDaemon(zero PID) error: %v", err)
	}
}

func TestCleanupStaleState(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "daemon-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create a stale state file with a non-existent PID
	state := &DaemonState{
		PID:        999999999, // Very unlikely to exist
		Secret:     "stale-secret",
		SocketPath: filepath.Join(tmpDir, "stale.sock"),
	}

	// Create the fake socket file
	f, err := os.Create(state.SocketPath)
	if err != nil {
		t.Fatalf("Failed to create fake socket: %v", err)
	}
	f.Close()

	// Save the stale state
	if err := SaveDaemonState(state); err != nil {
		t.Fatalf("SaveDaemonState() error: %v", err)
	}

	// Cleanup should remove both state and socket
	if err := CleanupStaleState(); err != nil {
		t.Fatalf("CleanupStaleState() error: %v", err)
	}

	// Verify state is gone
	loaded, _ := LoadDaemonState()
	if loaded != nil {
		t.Error("Expected state to be cleaned up")
	}

	// Verify socket is gone
	if _, err := os.Stat(state.SocketPath); !os.IsNotExist(err) {
		t.Error("Expected socket file to be cleaned up")
	}
}

func TestGetPIDString(t *testing.T) {
	pidStr := GetPIDString()
	if pidStr == "" {
		t.Error("GetPIDString() returned empty string")
	}
	if pidStr == "0" {
		t.Error("GetPIDString() returned 0")
	}
}

func TestDaemonStatePath_Production(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")

	path, err := DaemonStatePath()
	if err != nil {
		t.Fatalf("DaemonStatePath() error: %v", err)
	}
	if filepath.Base(path) != "executor.json" {
		t.Errorf("DaemonStatePath() = %q, want basename executor.json", path)
	}
}

func TestDaemonStatePath_TestInstance(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "abc123")

	path, err := DaemonStatePath()
	if err != nil {
		t.Fatalf("DaemonStatePath() error: %v", err)
	}
	if filepath.Base(path) != "executor-abc123.json" {
		t.Errorf("DaemonStatePath() = %q, want basename executor-abc123.json", path)
	}
}
