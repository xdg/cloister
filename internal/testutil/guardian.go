package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// RequireCloisterBinary ensures the cloister binary is built and sets CLOISTER_EXECUTABLE.
// Skips the test if the binary doesn't exist.
func RequireCloisterBinary(t *testing.T) {
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
	t.Setenv(guardian.ExecutableEnvVar, binaryPath)
}

// RequireGuardian ensures the guardian is running and registers cleanup.
// This is for integration tests that manage their own guardian lifecycle.
// Generates a unique instance ID so tests don't conflict with production or other tests.
// Skips the test if the guardian cannot be started.
func RequireGuardian(t *testing.T) {
	t.Helper()
	RequireDocker(t)
	RequireCloisterBinary(t)

	// Generate unique instance ID for test isolation
	t.Setenv(guardian.InstanceIDEnvVar, guardian.GenerateInstanceID())

	if err := guardian.EnsureRunning(); err != nil {
		t.Skipf("Guardian not available: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.Stop()
	})
}

// RequireCleanGuardianState ensures no guardian is running and registers cleanup.
// Generates a unique instance ID so tests operate on an isolated guardian.
// Use this for tests that need exclusive control over guardian lifecycle.
func RequireCleanGuardianState(t *testing.T) {
	t.Helper()
	RequireDocker(t)

	// Generate unique instance ID for test isolation.
	// This means IsRunning() will check for our isolated instance, not production.
	t.Setenv(guardian.InstanceIDEnvVar, guardian.GenerateInstanceID())

	running, err := guardian.IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error: %v", err)
	}
	if running {
		// This should not happen with instance isolation, but check anyway
		t.Skip("Skipping: guardian is already running (parallel test conflict)")
	}
	t.Cleanup(func() {
		_, _ = CleanupGuardian()
	})
}

// CleanupGuardian stops the executor and removes the guardian container if they exist.
// Returns any output and error from the docker commands.
func CleanupGuardian() (string, error) {
	_ = guardian.StopExecutor()
	_, _ = docker.Run("stop", guardian.ContainerName())
	return docker.Run("rm", guardian.ContainerName())
}
