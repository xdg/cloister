//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/guardian"
)

// TestProxy_AllowedDomain verifies that allowed domains are accessible through the proxy.
func TestProxy_AllowedDomain(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "proxy-allow")
	guardianHost := guardian.ContainerName()

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Test accessing an allowed domain (golang.org is in the default allowlist).
	// Use %{http_connect} to verify the proxy allows the CONNECT (200),
	// then %{http_code} for the actual HTTP response through the tunnel.
	// Authentication is required: --proxy-user :token (empty username, token as password).
	output, err := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -s -o /dev/null -w '%{http_connect}:%{http_code}' --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+" --max-time 10 https://golang.org/")

	if err != nil {
		if strings.Contains(output, "not found") {
			t.Skip("curl not available in container image")
		}
		// Network issues are expected if the host doesn't have internet
		t.Skipf("Network test failed (may require internet access): %v", err)
	}

	output = strings.TrimSpace(output)
	parts := strings.SplitN(output, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("Unexpected output format: %q", output)
	}

	connectCode, httpCode := parts[0], parts[1]

	// CONNECT should succeed (200)
	if connectCode != "200" {
		t.Errorf("Expected CONNECT response 200 for allowed domain, got: %s", connectCode)
	}

	// Final HTTP response should be success (2xx or 3xx redirect)
	if !strings.HasPrefix(httpCode, "2") && !strings.HasPrefix(httpCode, "3") {
		t.Errorf("Expected HTTP success status (2xx or 3xx), got: %s", httpCode)
	}
}

// TestProxy_BlockedDomain verifies that non-allowlisted domains are blocked.
// With unlisted_domain_behavior: "request_approval", unlisted domains are queued
// for approval rather than immediately rejected. The request blocks until the
// approval timeout (3s in test config) expires, then returns 403.
func TestProxy_BlockedDomain(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "proxy-block")
	guardianHost := guardian.ContainerName()

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Test accessing a domain that should NOT be in the allowlist.
	// Note: curl's %{http_connect} returns empty when CONNECT fails with 403/407 because
	// curl treats it as a transport error (exit 56). We use verbose output and parse
	// the actual HTTP response line instead.
	// Authentication is required: --proxy-user :token (empty username, token as password).
	// --max-time is set to 15s to allow for the 3s approval timeout plus overhead.
	output, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+" --max-time 15 https://example.com/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")

	if strings.Contains(output, "not found") {
		t.Skip("curl not available in container image")
	}

	output = strings.TrimSpace(output)
	// Proxy should return 403 Forbidden on CONNECT for blocked domains
	// (after approval timeout expires, since we're in request_approval mode)
	if output != "403" {
		t.Errorf("Expected CONNECT response 403 for blocked domain, got: %q", output)
	}
}

// TestProxy_UnauthenticatedRequest verifies that requests without valid token get 407.
func TestProxy_UnauthenticatedRequest(t *testing.T) {
	// Use unauthenticated container (no token registered with guardian)
	containerName := createTestContainer(t, "proxy-unauth")
	guardianHost := guardian.ContainerName()

	// Wait for proxy to be ready
	if err := waitForPort(t, containerName, guardianHost, 3128); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Test accessing an allowed domain WITHOUT authentication.
	// Note: curl's %{http_connect} returns empty when CONNECT fails with 407 because
	// curl treats it as a transport error. We use verbose output and parse
	// the actual HTTP response line instead.
	output, _ := execInContainer(t, containerName,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --max-time 10 https://golang.org/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")

	if strings.Contains(output, "not found") {
		t.Skip("curl not available in container image")
	}

	output = strings.TrimSpace(output)
	// Proxy should return 407 Proxy Authentication Required for unauthenticated requests
	if output != "407" {
		t.Errorf("Expected CONNECT response 407 for unauthenticated request, got: %q", output)
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
}
