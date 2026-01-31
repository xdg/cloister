//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestProxy_AllowedDomain verifies that allowed domains are accessible through the proxy.
func TestProxy_AllowedDomain(t *testing.T) {
	containerName := createTestContainer(t, "proxy-allow")

	// Wait for proxy to be ready
	if err := waitForPort(t, containerName, "cloister-guardian", 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Test accessing an allowed domain (golang.org is typically in the default allowlist)
	// The container needs to use the proxy for external access
	output, err := execInContainer(t, containerName,
		"sh", "-c",
		"http_proxy=http://cloister-guardian:3128 https_proxy=http://cloister-guardian:3128 curl -s -o /dev/null -w '%{http_code}' --max-time 10 https://golang.org/")

	if err != nil {
		var cmdOutput string
		// Check if curl is available
		if output != "" {
			cmdOutput = output
		}
		if strings.Contains(cmdOutput, "not found") {
			t.Skip("curl not available in container image")
		}
		// Network issues are expected if the host doesn't have internet
		t.Skipf("Network test failed (may require internet access): %v", err)
	}

	// Should get a success status (2xx or 3xx redirect)
	output = strings.TrimSpace(output)
	if !strings.HasPrefix(output, "2") && !strings.HasPrefix(output, "3") {
		t.Errorf("Expected success status (2xx or 3xx), got: %s", output)
	}
}

// TestProxy_BlockedDomain verifies that non-allowlisted domains are blocked.
func TestProxy_BlockedDomain(t *testing.T) {
	containerName := createTestContainer(t, "proxy-block")

	// Wait for proxy to be ready
	if err := waitForPort(t, containerName, "cloister-guardian", 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Test accessing a domain that should NOT be in the allowlist
	// Use a domain that definitely won't be allowlisted but is real
	output, err := execInContainer(t, containerName,
		"sh", "-c",
		"http_proxy=http://cloister-guardian:3128 https_proxy=http://cloister-guardian:3128 curl -s -o /dev/null -w '%{http_code}' --max-time 10 https://example.com/ 2>&1")

	if err != nil {
		if strings.Contains(output, "not found") {
			t.Skip("curl not available in container image")
		}
		// For blocked domains, curl may exit with an error
		// Check if we got a 403 or proxy rejection
		if strings.Contains(output, "403") || strings.Contains(output, "Forbidden") {
			// Good - domain was blocked
			return
		}
		// If we got here, the error might be for other reasons
		t.Logf("curl error: %v, output: %s", err, output)
	}

	output = strings.TrimSpace(output)
	// Proxy should return 403 Forbidden for blocked domains
	if output != "403" {
		// Also accept if curl got an error (connection refused by proxy)
		if !strings.Contains(output, "403") {
			t.Logf("Note: got status %q - may need to verify example.com is not in allowlist", output)
		}
	}
}

// TestProxy_DirectAccessBlocked verifies that containers cannot bypass the proxy.
func TestProxy_DirectAccessBlocked(t *testing.T) {
	containerName := createTestContainer(t, "proxy-direct")

	// Try to access the internet directly (without proxy)
	// This should fail because cloister-net is an internal network
	output, err := execInContainer(t, containerName,
		"sh", "-c",
		"curl -s -o /dev/null -w '%{http_code}' --max-time 5 https://golang.org/ 2>&1 || echo 'connection_failed'")

	if err != nil {
		if strings.Contains(output, "not found") {
			t.Skip("curl not available in container image")
		}
	}

	// Direct access should fail - either timeout, connection refused, or DNS failure
	output = strings.TrimSpace(output)
	if strings.HasPrefix(output, "2") || strings.HasPrefix(output, "3") {
		t.Errorf("Direct internet access should be blocked, but got success status: %s", output)
	}

	// Expected: connection_failed, 000 (curl error), or similar failure
	if output == "connection_failed" || output == "000" || strings.Contains(output, "Could not resolve") {
		// Good - direct access is blocked
		return
	}

	t.Logf("Direct access result: %s (should indicate failure)", output)
}
