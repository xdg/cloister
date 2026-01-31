//go:build e2e

// Package e2e contains end-to-end tests that verify cloister workflows
// with a running guardian. Tests in this package assume the guardian is
// managed by TestMain - they do not start/stop it themselves.
package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// TestMain sets up the guardian for all e2e tests and tears it down on exit.
// This allows tests to share a single guardian instance, which is more efficient
// and matches the production model where guardian runs persistently.
func TestMain(m *testing.M) {
	// Check Docker availability first
	if err := docker.CheckDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Docker not available: %v\n", err)
		os.Exit(0)
	}

	// Set up cloister binary path for executor spawning
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintf(os.Stderr, "SKIP: Could not determine test file location\n")
		os.Exit(0)
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	binaryPath := filepath.Join(repoRoot, "cloister")
	if _, err := os.Stat(binaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cloister binary not found at %s (run 'make build' first)\n", binaryPath)
		os.Exit(0)
	}
	os.Setenv(guardian.ExecutableEnvVar, binaryPath)

	// Ensure clean state - stop any existing guardian
	_ = guardian.Stop()

	// Start the guardian for the test run
	if err := guardian.EnsureRunning(); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Could not start guardian: %v\n", err)
		os.Exit(0)
	}

	// Run tests
	code := m.Run()

	// Cleanup: stop the guardian
	if err := guardian.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stop guardian: %v\n", err)
	}

	os.Exit(code)
}
