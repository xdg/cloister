package executor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/testutil"
)

func TestDaemonState_SaveLoadRemove(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Test saving state
	state := &executor.DaemonState{
		PID:        12345,
		Secret:     "test-secret-abc",
		SocketPath: "/tmp/test.sock",
	}

	if err := executor.SaveDaemonState(state); err != nil {
		t.Fatalf("executor.SaveDaemonState() error: %v", err)
	}

	// Test loading state
	loaded, err := executor.LoadDaemonState()
	if err != nil {
		t.Fatalf("executor.LoadDaemonState() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("executor.LoadDaemonState() returned nil")
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
	if err := executor.RemoveDaemonState(); err != nil {
		t.Fatalf("executor.RemoveDaemonState() error: %v", err)
	}

	// Verify state is gone
	loaded, err = executor.LoadDaemonState()
	if err != nil {
		t.Fatalf("executor.LoadDaemonState() after remove error: %v", err)
	}
	if loaded != nil {
		t.Error("Expected nil state after removal")
	}
}

func TestDaemonState_LoadNonExistent(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Loading non-existent state should return nil, no error
	state, err := executor.LoadDaemonState()
	if err != nil {
		t.Fatalf("executor.LoadDaemonState() error: %v", err)
	}
	if state != nil {
		t.Error("Expected nil state for non-existent file")
	}
}

func TestIsDaemonRunning_NilState(t *testing.T) {
	if executor.IsDaemonRunning(nil) {
		t.Error("Expected false for nil state")
	}
}

func TestIsDaemonRunning_ZeroPID(t *testing.T) {
	state := &executor.DaemonState{PID: 0}
	if executor.IsDaemonRunning(state) {
		t.Error("Expected false for zero PID")
	}
}

func TestIsDaemonRunning_CurrentProcess(t *testing.T) {
	// Our own process should be running
	state := &executor.DaemonState{PID: os.Getpid()}
	if !executor.IsDaemonRunning(state) {
		t.Error("Expected true for current process PID")
	}
}

func TestIsDaemonRunning_NonExistentProcess(t *testing.T) {
	// Use a very high PID that likely doesn't exist
	state := &executor.DaemonState{PID: 999999999}
	if executor.IsDaemonRunning(state) {
		t.Error("Expected false for non-existent PID")
	}
}

func TestStopDaemon_NilState(t *testing.T) {
	// Should not error on nil state
	if err := executor.StopDaemon(nil); err != nil {
		t.Errorf("executor.StopDaemon(nil) error: %v", err)
	}
}

func TestStopDaemon_ZeroPID(t *testing.T) {
	state := &executor.DaemonState{PID: 0}
	if err := executor.StopDaemon(state); err != nil {
		t.Errorf("executor.StopDaemon(zero PID) error: %v", err)
	}
}

func TestCleanupStaleState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tmpDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpDir)

	// Create a stale state file with a non-existent PID
	state := &executor.DaemonState{
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
	if err := executor.SaveDaemonState(state); err != nil {
		t.Fatalf("executor.SaveDaemonState() error: %v", err)
	}

	// Cleanup should remove both state and socket
	if err := executor.CleanupStaleState(); err != nil {
		t.Fatalf("executor.CleanupStaleState() error: %v", err)
	}

	// Verify state is gone
	loaded, _ := executor.LoadDaemonState()
	if loaded != nil {
		t.Error("Expected state to be cleaned up")
	}

	// Verify socket is gone
	if _, err := os.Stat(state.SocketPath); !os.IsNotExist(err) {
		t.Error("Expected socket file to be cleaned up")
	}
}

func TestGetPIDString(t *testing.T) {
	pidStr := executor.GetPIDString()
	if pidStr == "" {
		t.Error("executor.GetPIDString() returned empty string")
	}
	if pidStr == "0" {
		t.Error("executor.GetPIDString() returned 0")
	}
}

func TestDaemonStatePath_Production(t *testing.T) {
	t.Setenv(executor.InstanceIDEnvVar, "")

	path, err := executor.DaemonStatePath()
	if err != nil {
		t.Fatalf("executor.DaemonStatePath() error: %v", err)
	}
	if filepath.Base(path) != "executor.json" {
		t.Errorf("executor.DaemonStatePath() = %q, want basename executor.json", path)
	}
}

func TestDaemonStatePath_TestInstance(t *testing.T) {
	t.Setenv(executor.InstanceIDEnvVar, "abc123")

	path, err := executor.DaemonStatePath()
	if err != nil {
		t.Fatalf("executor.DaemonStatePath() error: %v", err)
	}
	if filepath.Base(path) != "executor-abc123.json" {
		t.Errorf("executor.DaemonStatePath() = %q, want basename executor-abc123.json", path)
	}
}

func TestDaemonStateDir_Default(t *testing.T) {
	// Clear XDG_STATE_HOME to test default behavior
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := executor.DaemonStateDir()
	if err != nil {
		t.Fatalf("executor.DaemonStateDir() error: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}

	expected := filepath.Join(home, ".local", "state", "cloister")
	if dir != expected {
		t.Errorf("executor.DaemonStateDir() = %q, want %q", dir, expected)
	}
}

func TestDaemonStateDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	dir, err := executor.DaemonStateDir()
	if err != nil {
		t.Fatalf("executor.DaemonStateDir() error: %v", err)
	}

	expected := "/custom/state/cloister"
	if dir != expected {
		t.Errorf("executor.DaemonStateDir() = %q, want %q", dir, expected)
	}
}
