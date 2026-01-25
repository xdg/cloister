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

func TestIsRunning_WhenNotExists(t *testing.T) {
	requireDocker(t)

	// Ensure guardian is not running
	cleanupGuardian()

	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error: %v", err)
	}
	if running {
		t.Error("IsRunning() = true, want false when container doesn't exist")
	}
}

func TestEnsureRunning_Idempotent(t *testing.T) {
	requireDocker(t)

	// Ensure clean state
	cleanupGuardian()
	defer cleanupGuardian()

	// First call should start the guardian
	// Note: This test requires the cloister:latest image to exist.
	// For testing purposes, we only check the logic works, not that
	// the image actually runs correctly.
	err := EnsureRunning()
	if err != nil {
		// If the image doesn't exist, skip this test
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Test requires cloister:latest image: %v", err)
		}
		t.Fatalf("First EnsureRunning() error: %v", err)
	}

	// Second call should be a no-op
	err = EnsureRunning()
	if err != nil {
		t.Fatalf("Second EnsureRunning() error: %v", err)
	}

	// Guardian should be running
	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error: %v", err)
	}
	if !running {
		t.Error("Guardian should be running after EnsureRunning()")
	}
}

func TestStop_WhenNotRunning(t *testing.T) {
	requireDocker(t)

	// Ensure guardian is not running
	cleanupGuardian()

	// Stop should be idempotent
	err := Stop()
	if err != nil {
		t.Fatalf("Stop() error when not running: %v", err)
	}
}

func TestStart_ReturnsErrorWhenAlreadyRunning(t *testing.T) {
	requireDocker(t)

	// Ensure clean state
	cleanupGuardian()
	defer cleanupGuardian()

	// Start the guardian
	err := Start()
	if err != nil {
		// If the image doesn't exist, skip this test
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Test requires cloister:latest image: %v", err)
		}
		t.Fatalf("First Start() error: %v", err)
	}

	// Second start should return error
	err = Start()
	if !errors.Is(err, ErrGuardianAlreadyRunning) {
		t.Errorf("Second Start() = %v, want ErrGuardianAlreadyRunning", err)
	}
}

func TestGetContainerState(t *testing.T) {
	requireDocker(t)

	// Ensure guardian is not running
	cleanupGuardian()

	// Should return nil when container doesn't exist
	state, err := getContainerState()
	if err != nil {
		t.Fatalf("getContainerState() error: %v", err)
	}
	if state != nil {
		t.Error("getContainerState() should return nil when container doesn't exist")
	}
}
