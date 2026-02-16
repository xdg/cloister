// Package testutil provides shared test helpers for cloister tests.
package testutil

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/docker"
)

// RequireDocker skips the test if Docker is not available.
func RequireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// CleanupContainer removes a container if it exists.
// Safe to call even if the container doesn't exist.
func CleanupContainer(name string) {
	// Best effort cleanup - container may not exist
	if _, err := docker.Run("rm", "-f", name); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup container %s: %v\n", name, err)
	}
}

// UniqueContainerName generates a unique container name with the given prefix.
// Useful for avoiding collisions in parallel tests.
func UniqueContainerName(prefix string) string {
	return fmt.Sprintf("cloister-%s-%d", prefix, time.Now().UnixNano())
}

// TestProjectName generates a unique test project name.
func TestProjectName() string {
	return "test-" + time.Now().Format("20060102-150405")
}

// TestContainerName generates a unique test container name with the given suffix.
func TestContainerName(suffix string) string {
	return "cloister-test-" + suffix + "-" + time.Now().Format("150405")
}

// IsolateXDGDirs sets XDG_CONFIG_HOME and XDG_STATE_HOME to temp directories.
// This prevents tests from reading or writing to the real ~/.config/cloister
// or ~/.local/state/cloister directories.
func IsolateXDGDirs(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}
