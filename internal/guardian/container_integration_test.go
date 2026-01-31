//go:build integration

package guardian

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/executor"
)

// requireDocker skips the test if Docker is not available.
// Note: We can't use testutil.RequireDocker here because testutil imports guardian,
// which would create an import cycle.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// requireCloisterBinary ensures the cloister binary is built and sets CLOISTER_EXECUTABLE.
// Skips the test if the binary doesn't exist.
func requireCloisterBinary(t *testing.T) {
	t.Helper()

	// Find repo root (go up from this file's directory)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("Could not determine test file location")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	binaryPath := filepath.Join(repoRoot, "cloister")

	if _, err := os.Stat(binaryPath); err != nil {
		t.Skipf("cloister binary not found at %s (run 'make build' first)", binaryPath)
	}

	// Set env var for StartExecutor to use
	t.Setenv(ExecutableEnvVar, binaryPath)
}

// cleanupGuardian removes the guardian container and executor if they exist.
func cleanupGuardian() {
	// Best effort cleanup - ignore errors
	_ = StopExecutor()
	_, _ = docker.Run("stop", ContainerName)
	_, _ = docker.Run("rm", ContainerName)
}

// requireCleanGuardianState ensures no guardian is running and registers cleanup.
// Skips the test if guardian is unexpectedly running (another package may be using it).
func requireCleanGuardianState(t *testing.T) {
	t.Helper()
	requireDocker(t)
	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error: %v", err)
	}
	if running {
		t.Skip("Skipping: guardian is already running (parallel test conflict)")
	}
	t.Cleanup(cleanupGuardian)
}

// TestGuardian_WhenNotRunning tests guardian behavior when no container exists.
func TestGuardian_WhenNotRunning(t *testing.T) {
	requireCleanGuardianState(t)

	t.Run("IsRunning_ReturnsFalse", func(t *testing.T) {
		running, err := IsRunning()
		if err != nil {
			t.Fatalf("IsRunning() error: %v", err)
		}
		if running {
			t.Error("IsRunning() = true, want false when container doesn't exist")
		}
	})

	t.Run("GetContainerState_ReturnsNil", func(t *testing.T) {
		state, err := getContainerState()
		if err != nil {
			t.Fatalf("getContainerState() error: %v", err)
		}
		if state != nil {
			t.Error("getContainerState() should return nil when container doesn't exist")
		}
	})

	t.Run("Stop_IsIdempotent", func(t *testing.T) {
		if err := Stop(); err != nil {
			t.Fatalf("Stop() error when not running: %v", err)
		}
	})
}

// TestGuardian_Lifecycle tests guardian start/stop behavior.
func TestGuardian_Lifecycle(t *testing.T) {
	requireCleanGuardianState(t)
	requireCloisterBinary(t)

	// Start the guardian (required for all subtests)
	err := EnsureRunning()
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Test requires cloister:latest image: %v", err)
		}
		t.Fatalf("EnsureRunning() error: %v", err)
	}

	t.Run("IsRunning_ReturnsTrue", func(t *testing.T) {
		running, err := IsRunning()
		if err != nil {
			t.Fatalf("IsRunning() error: %v", err)
		}
		if !running {
			t.Error("Guardian should be running after EnsureRunning()")
		}
	})

	t.Run("EnsureRunning_IsIdempotent", func(t *testing.T) {
		if err := EnsureRunning(); err != nil {
			t.Fatalf("Second EnsureRunning() error: %v", err)
		}
	})

	t.Run("Start_ReturnsErrorWhenRunning", func(t *testing.T) {
		err := Start()
		if !errors.Is(err, ErrGuardianAlreadyRunning) {
			t.Errorf("Start() = %v, want ErrGuardianAlreadyRunning", err)
		}
	})

	t.Run("Executor_SocketExists", func(t *testing.T) {
		socketPath, err := HostSocketPath()
		if err != nil {
			t.Fatalf("HostSocketPath() error: %v", err)
		}
		info, err := os.Stat(socketPath)
		if err != nil {
			t.Errorf("Socket file should exist at %s: %v", socketPath, err)
			return
		}
		if info.Mode().Type()&os.ModeSocket == 0 {
			t.Errorf("Expected socket file, got mode %v", info.Mode())
		}
	})

	t.Run("Executor_DaemonRunning", func(t *testing.T) {
		state, err := executor.LoadDaemonState()
		if err != nil {
			t.Fatalf("LoadDaemonState() error: %v", err)
		}
		if state == nil {
			t.Fatal("Expected daemon state to exist after guardian start")
		}
		if !executor.IsDaemonRunning(state) {
			t.Errorf("Executor daemon should be running (PID %d)", state.PID)
		}
	})

	// Stop guardian and verify cleanup
	t.Run("Stop_CleansUpExecutor", func(t *testing.T) {
		// Get socket path before stopping
		socketPath, err := HostSocketPath()
		if err != nil {
			t.Fatalf("HostSocketPath() error: %v", err)
		}

		// Stop the guardian
		if err := Stop(); err != nil {
			t.Fatalf("Stop() error: %v", err)
		}

		// Verify guardian is stopped
		running, err := IsRunning()
		if err != nil {
			t.Fatalf("IsRunning() error: %v", err)
		}
		if running {
			t.Error("Guardian should not be running after Stop()")
		}

		// Verify executor state is cleaned up
		state, err := executor.LoadDaemonState()
		if err != nil {
			t.Fatalf("LoadDaemonState() error: %v", err)
		}
		if state != nil && executor.IsDaemonRunning(state) {
			t.Errorf("Executor daemon should not be running after Stop() (PID %d)", state.PID)
		}

		// Verify socket is removed
		if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
			t.Errorf("Socket file should be removed after Stop(): %v", err)
		}
	})
}
