//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/version"
)

// testContainerInfo holds info about a test container with optional proxy auth.
type testContainerInfo struct {
	Name  string // Container name
	Token string // Proxy auth token (empty if unauthenticated)
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
		version.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create test container with %s: %v", version.DefaultImage(), err)
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

	// Register a token for this container
	token := fmt.Sprintf("test-token-%s-%d", suffix, time.Now().UnixNano())
	if err := guardian.RegisterToken(token, containerName, "test-project"); err != nil {
		t.Fatalf("Failed to register token: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.RevokeToken(token)
	})

	return testContainerInfo{Name: containerName, Token: token}
}

// waitForPort waits for a port to become reachable from inside a container.
// Returns nil on success, error on timeout.
func waitForPort(t *testing.T, containerName, host string, port int, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)
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
