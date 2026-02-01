//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/guardian/request"
)

// TestNetworkIsolation_ContainerCanReachGuardian verifies that containers
// on cloister-net can reach the guardian's proxy and request server.
func TestNetworkIsolation_ContainerCanReachGuardian(t *testing.T) {
	containerName := createTestContainer(t, "reach-guardian")

	// Wait for request server port to be ready
	if err := waitForPort(t, containerName, "cloister-guardian", request.DefaultRequestPort, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Test 1: Can reach proxy port 3128
	t.Run("ProxyPort", func(t *testing.T) {
		_, err := execInContainer(t, containerName, "nc", "-z", "-w", "2", "cloister-guardian", "3128")
		if err != nil {
			t.Errorf("Could not connect to proxy port 3128: %v", err)
		}
	})

	// Test 2: Can reach request server port 9998
	t.Run("RequestServerPort", func(t *testing.T) {
		_, err := execInContainer(t, containerName, "nc", "-z", "-w", "2", "cloister-guardian", "9998")
		if err != nil {
			t.Errorf("Could not connect to request server port 9998: %v", err)
		}
	})
}

// TestNetworkIsolation_RequestServerReachable verifies the request server
// responds correctly to requests from containers on cloister-net.
func TestNetworkIsolation_RequestServerReachable(t *testing.T) {
	containerName := createTestContainer(t, "reqserver")

	// Register a test token with the guardian
	testToken := fmt.Sprintf("test-token-%d", time.Now().UnixNano())
	if err := guardian.RegisterToken(testToken, containerName, "test-project"); err != nil {
		t.Fatalf("Failed to register token: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.RevokeToken(testToken)
	})

	// Wait for request server port to be ready
	if err := waitForPort(t, containerName, "cloister-guardian", request.DefaultRequestPort, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Build the curl command to test connectivity
	cmdReq := request.CommandRequest{Cmd: "echo hello", Args: []string{"echo", "hello"}}
	reqBody, _ := json.Marshal(cmdReq)

	output, err := execInContainer(t, containerName,
		"curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", fmt.Sprintf("X-Cloister-Token: %s", testToken),
		"-d", string(reqBody),
		"-w", "\n%{http_code}",
		fmt.Sprintf("http://cloister-guardian:%d/request", request.DefaultRequestPort))

	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			if strings.Contains(cmdErr.Stderr, "not found") {
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

	if statusCode != "200" {
		t.Errorf("Expected status code 200, got %s", statusCode)
		t.Logf("Response body: %s", body)
	}

	// Verify we got a valid JSON response
	var resp request.CommandResponse
	if err := json.NewDecoder(bytes.NewReader([]byte(body))).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response JSON: %v\nBody: %s", err, body)
	}

	// Command should be denied (no approval patterns matched)
	if resp.Status != "denied" {
		t.Errorf("Expected status 'denied', got %q", resp.Status)
	}
}

// TestNetworkIsolation_UnauthorizedRequest verifies that requests
// without a valid token are rejected with 401.
func TestNetworkIsolation_UnauthorizedRequest(t *testing.T) {
	containerName := createTestContainer(t, "unauth")

	cmdReq := request.CommandRequest{Cmd: "echo hello", Args: []string{"echo", "hello"}}
	reqBody, _ := json.Marshal(cmdReq)

	output, err := execInContainer(t, containerName,
		"curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", "X-Cloister-Token: invalid-token-12345",
		"-d", string(reqBody),
		"-w", "\n%{http_code}",
		fmt.Sprintf("http://cloister-guardian:%d/request", request.DefaultRequestPort))

	if err != nil {
		var cmdErr *docker.CommandError
		if errors.As(err, &cmdErr) {
			if strings.Contains(cmdErr.Stderr, "not found") {
				t.Skip("curl not available in container image")
			}
		}
		t.Fatalf("Failed to execute curl in container: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 1 {
		t.Fatalf("Expected at least 1 line (status), got: %q", output)
	}

	statusCode := lines[len(lines)-1]

	if statusCode != "401" {
		t.Errorf("Expected status code 401, got %s", statusCode)
		t.Logf("Full output: %s", output)
	}
}

// TestNetworkIsolation_HostCannotReachRequestServer verifies that the request
// server port (9998) is NOT exposed to the host - it's internal to cloister-net only.
func TestNetworkIsolation_HostCannotReachRequestServer(t *testing.T) {
	// Port 9998 should NOT be exposed to the host.
	// This is a security property: only containers on cloister-net can reach it.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/request", request.DefaultRequestPort))
	if err == nil {
		resp.Body.Close()
		t.Error("Expected connection to localhost:9998 to fail (port should not be exposed to host)")
	}
	// Connection refused or timeout is expected
}

// TestNetworkIsolation_HostCanReachAPIServer verifies that the API server
// port (9997) IS exposed to the host for CLI operations.
func TestNetworkIsolation_HostCanReachAPIServer(t *testing.T) {
	// Port 9997 should be exposed to localhost for CLI token management
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:9997/tokens")
	if err != nil {
		t.Errorf("Expected connection to localhost:9997 to succeed: %v", err)
		return
	}
	defer resp.Body.Close()

	// GET /tokens should return 200 (empty list is fine)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 from /tokens, got %d", resp.StatusCode)
	}
}
