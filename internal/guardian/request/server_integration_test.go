//go:build integration

package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// requireDocker skips the test if Docker is not available.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// requireGuardian skips the test if guardian cannot be started.
// Registers cleanup to stop the guardian when test completes.
func requireGuardian(t *testing.T) {
	t.Helper()
	requireDocker(t)
	if err := guardian.EnsureRunning(); err != nil {
		t.Skipf("Guardian not available: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.Stop()
	})
}

// cleanupTestContainer removes a test container if it exists.
func cleanupTestContainer(name string) {
	// Best effort cleanup - ignore errors
	_, _ = docker.Run("rm", "-f", name)
}

// TestRequestServer_ReachableFromContainer verifies that a container on cloister-net
// can reach the request server via cloister-guardian:9998.
func TestRequestServer_ReachableFromContainer(t *testing.T) {
	requireGuardian(t)

	// Generate unique container name
	containerName := fmt.Sprintf("cloister-reqtest-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Register a test token with the guardian
	testToken := fmt.Sprintf("test-token-%d", time.Now().UnixNano())
	if err := guardian.RegisterToken(testToken, containerName, "test-project"); err != nil {
		t.Fatalf("Failed to register token: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.RevokeToken(testToken)
	})

	// Create and start a test container on cloister-net
	_, err := docker.Run("run", "-d",
		"--name", containerName,
		"--network", docker.CloisterNetworkName,
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
		}
		t.Fatalf("Failed to create test container: %v", err)
	}

	// Give the container a moment to start
	time.Sleep(100 * time.Millisecond)

	// Build the curl command to test connectivity to request server
	// Uses curl to POST a valid command request to cloister-guardian:9998/request
	cmdReq := CommandRequest{Cmd: "echo hello"}
	reqBody, _ := json.Marshal(cmdReq)

	// Execute curl from inside the container to reach the request server
	output, err := docker.Run("exec", containerName,
		"curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", fmt.Sprintf("X-Cloister-Token: %s", testToken),
		"-d", string(reqBody),
		"-w", "\n%{http_code}",
		"http://cloister-guardian:9998/request")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			// If curl failed, check if it's because curl isn't installed
			if strings.Contains(cmdErr.Stderr, "executable file not found") ||
				strings.Contains(cmdErr.Stderr, "not found") {
				t.Skip("curl not available in container image")
			}
		}
		t.Fatalf("Failed to execute curl in container: %v\nOutput: %s", err, output)
	}

	// Parse response - last line is HTTP status code
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("Expected at least 2 lines (body + status), got: %q", output)
	}

	statusCode := lines[len(lines)-1]
	bodyLines := lines[:len(lines)-1]
	body := strings.Join(bodyLines, "\n")

	// The request server should respond (even if with "not implemented")
	// We expect 501 Not Implemented since command execution is not yet wired up
	if statusCode != "501" {
		t.Errorf("Expected status code 501 (Not Implemented), got %s", statusCode)
		t.Logf("Response body: %s", body)
	}

	// Verify we got a valid JSON response
	var resp CommandResponse
	if err := json.NewDecoder(bytes.NewReader([]byte(body))).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response JSON: %v\nBody: %s", err, body)
	}

	// Verify response indicates "not yet implemented" (expected at this phase)
	if resp.Status != "error" {
		t.Errorf("Expected status 'error', got %q", resp.Status)
	}
	if !strings.Contains(resp.Reason, "not yet implemented") {
		t.Errorf("Expected reason to contain 'not yet implemented', got %q", resp.Reason)
	}
}

// TestRequestServer_UnauthorizedFromContainer verifies that requests
// without a valid token are rejected with 401.
func TestRequestServer_UnauthorizedFromContainer(t *testing.T) {
	requireGuardian(t)

	// Generate unique container name
	containerName := fmt.Sprintf("cloister-reqtest-unauth-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Create and start a test container on cloister-net
	_, err := docker.Run("run", "-d",
		"--name", containerName,
		"--network", docker.CloisterNetworkName,
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
		}
		t.Fatalf("Failed to create test container: %v", err)
	}

	// Give the container a moment to start
	time.Sleep(100 * time.Millisecond)

	// Test with invalid token
	cmdReq := CommandRequest{Cmd: "echo hello"}
	reqBody, _ := json.Marshal(cmdReq)

	output, err := docker.Run("exec", containerName,
		"curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", "X-Cloister-Token: invalid-token-12345",
		"-d", string(reqBody),
		"-w", "\n%{http_code}",
		"http://cloister-guardian:9998/request")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			if strings.Contains(cmdErr.Stderr, "executable file not found") ||
				strings.Contains(cmdErr.Stderr, "not found") {
				t.Skip("curl not available in container image")
			}
		}
		t.Fatalf("Failed to execute curl in container: %v", err)
	}

	// Parse response - last line is HTTP status code
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 1 {
		t.Fatalf("Expected at least 1 line (status), got: %q", output)
	}

	statusCode := lines[len(lines)-1]

	// Should get 401 Unauthorized
	if statusCode != "401" {
		t.Errorf("Expected status code 401, got %s", statusCode)
		t.Logf("Full output: %s", output)
	}
}

// TestRequestServer_PortReachable verifies basic TCP connectivity to port 9998.
func TestRequestServer_PortReachable(t *testing.T) {
	requireGuardian(t)

	// Generate unique container name
	containerName := fmt.Sprintf("cloister-reqtest-port-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Create and start a test container on cloister-net
	_, err := docker.Run("run", "-d",
		"--name", containerName,
		"--network", docker.CloisterNetworkName,
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
		}
		t.Fatalf("Failed to create test container: %v", err)
	}

	// Give the container a moment to start
	time.Sleep(100 * time.Millisecond)

	// Use nc (netcat) to verify port 9998 is open on cloister-guardian
	// This is a simpler connectivity test that doesn't require curl
	output, err := docker.Run("exec", containerName,
		"sh", "-c",
		"echo '' | nc -w 2 cloister-guardian 9998 && echo 'connected' || echo 'failed'")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			// If nc isn't available, try using timeout + bash
			if strings.Contains(cmdErr.Stderr, "not found") {
				// Fall back to /dev/tcp if available
				output, err = docker.Run("exec", containerName,
					"bash", "-c",
					"timeout 2 bash -c 'echo > /dev/tcp/cloister-guardian/9998' && echo connected || echo failed")
				if err != nil {
					t.Skipf("Neither nc nor /dev/tcp available in container")
				}
			} else {
				t.Fatalf("Failed to check port: %v", err)
			}
		}
	}

	if !strings.Contains(output, "connected") {
		t.Errorf("Could not connect to cloister-guardian:9998 from container")
		t.Logf("Output: %s", output)
	}
}

// TestRequestServer_HTTPGetReturns405 verifies GET requests to /request return 405.
func TestRequestServer_HTTPGetReturns405(t *testing.T) {
	requireGuardian(t)

	// Generate unique container name
	containerName := fmt.Sprintf("cloister-reqtest-get-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Register a test token with the guardian
	testToken := fmt.Sprintf("test-token-get-%d", time.Now().UnixNano())
	if err := guardian.RegisterToken(testToken, containerName, "test-project"); err != nil {
		t.Fatalf("Failed to register token: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.RevokeToken(testToken)
	})

	// Create and start a test container on cloister-net
	_, err := docker.Run("run", "-d",
		"--name", containerName,
		"--network", docker.CloisterNetworkName,
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
		}
		t.Fatalf("Failed to create test container: %v", err)
	}

	// Give the container a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send GET request (should be rejected - POST only)
	output, err := docker.Run("exec", containerName,
		"curl", "-s",
		"-X", "GET",
		"-H", fmt.Sprintf("X-Cloister-Token: %s", testToken),
		"-w", "\n%{http_code}",
		"http://cloister-guardian:9998/request")
	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			if strings.Contains(cmdErr.Stderr, "executable file not found") ||
				strings.Contains(cmdErr.Stderr, "not found") {
				t.Skip("curl not available in container image")
			}
		}
		t.Fatalf("Failed to execute curl in container: %v", err)
	}

	// Parse response - last line is HTTP status code
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 1 {
		t.Fatalf("Expected at least 1 line (status), got: %q", output)
	}

	statusCode := lines[len(lines)-1]

	// Should get 405 Method Not Allowed (only POST is allowed)
	if statusCode != "405" {
		t.Errorf("Expected status code 405, got %s", statusCode)
		t.Logf("Full output: %s", output)
	}
}

// TestRequestServer_ReachableViaHTTPClient tests using Go's http.Client
// from the host to verify the guardian's internal request server is wired up.
// This complements the container-based tests by verifying the server from the host side.
func TestRequestServer_HostCannotReachPort9998(t *testing.T) {
	requireGuardian(t)

	// Port 9998 should NOT be exposed to the host (it's internal to cloister-net).
	// This test verifies security: only containers on cloister-net can reach it.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:9998/request")
	if err == nil {
		resp.Body.Close()
		t.Error("Expected connection to localhost:9998 to fail (port should not be exposed to host)")
	}
	// Connection refused or timeout is expected - port 9998 is internal only
}
