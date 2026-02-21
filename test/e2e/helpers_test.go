//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// testContainerInfo holds info about a test container with optional proxy auth.
type testContainerInfo struct {
	Name    string // Container name
	Token   string // Proxy auth token (empty if unauthenticated)
	Project string // Unique project name for token registration
}

// createTestContainer creates a container on cloister-net for testing.
// The container runs sleep infinity and is cleaned up when the test ends.
// Returns the container name.
func createTestContainer(t *testing.T, suffix string) string {
	t.Helper()

	containerName := fmt.Sprintf("cloister-e2e-%s-%d", suffix, time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = docker.Run("rm", "-f", containerName)
	})

	_, err := docker.Run("run", "-d",
		"--name", containerName,
		"--network", docker.CloisterNetworkName,
		container.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create test container with %s: %v", container.DefaultImage(), err)
	}

	// Give the container a moment to start
	time.Sleep(100 * time.Millisecond)

	return containerName
}

// createAuthenticatedTestContainer creates a container with a registered proxy token.
// The token is registered with the guardian and cleaned up when the test ends.
// Returns container info including the token for proxy authentication.
func createAuthenticatedTestContainer(t *testing.T, suffix string) testContainerInfo {
	t.Helper()

	containerName := createTestContainer(t, suffix)

	// Generate a unique project name per test to avoid cross-test state pollution
	project := fmt.Sprintf("e2e-%s-%d", suffix, time.Now().UnixNano())

	// Register a token for this container
	token := fmt.Sprintf("test-token-%s-%d", suffix, time.Now().UnixNano())
	if err := guardian.RegisterTokenFull(token, containerName, project, ""); err != nil {
		t.Fatalf("Failed to register token: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.RevokeToken(token)
		// Clean up project decisions file if it was created
		_ = os.Remove(config.ProjectDecisionPath(project))
	})

	return testContainerInfo{Name: containerName, Token: token, Project: project}
}

// saveGlobalDecisions saves the current global decisions file content and
// restores it when the test completes. Tests that modify global decisions
// must call this at the top to avoid polluting other tests.
func saveGlobalDecisions(t *testing.T) {
	t.Helper()

	path := config.GlobalDecisionPath()
	original, readErr := os.ReadFile(path)

	t.Cleanup(func() {
		if readErr != nil {
			// File didn't exist before; remove it
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, original, 0o644)
		}
	})
}

// waitForPort waits for a port to become reachable from inside a container.
// Returns nil on success, error on timeout.
func waitForPort(t *testing.T, containerName, host string, port int) error {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// Use nc -z for port scanning (zero-I/O mode, just checks if port is open)
		_, err := docker.Run("exec", containerName,
			"nc", "-z", "-w", "1", host, fmt.Sprintf("%d", port))
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s:%d to be reachable", host, port)
}

// execInContainer runs a command inside a container and returns the output.
func execInContainer(t *testing.T, containerName string, cmd ...string) (string, error) {
	t.Helper()
	args := append([]string{"exec", containerName}, cmd...)
	return docker.Run(args...)
}
