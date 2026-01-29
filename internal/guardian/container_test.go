package guardian

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/docker"
)

// requireDocker skips the test if Docker is not available.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// cleanupGuardian removes the guardian container if it exists.
func cleanupGuardian() {
	// Best effort cleanup - ignore errors
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
}
