// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/guardian/request"
)

// InstanceIDEnvVar is the environment variable used to set a unique instance ID
// for test isolation. When set, the guardian container name and executor state
// file are suffixed with this ID, allowing multiple test runs to operate
// concurrently without conflicts.
const InstanceIDEnvVar = "CLOISTER_INSTANCE_ID"

// InstanceID returns the current instance ID from the environment.
// Returns empty string for production usage (when env var is not set).
//
// Thread safety: This function reads an environment variable which should be
// set once at program/test startup and not modified during execution. While
// os.Getenv is thread-safe, concurrent modification via os.Setenv could cause
// races. For test isolation, set CLOISTER_INSTANCE_ID before starting any
// goroutines that may call this function.
func InstanceID() string {
	return os.Getenv(InstanceIDEnvVar)
}

// GenerateInstanceID creates a random 6-character hex ID for test isolation.
func GenerateInstanceID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%06x", time.Now().UnixNano()&0xFFFFFF)
	}
	return hex.EncodeToString(b)
}

// ContainerName returns the guardian container name for the current instance.
// For production (no instance ID), returns "cloister-guardian".
// For test instances, returns "cloister-guardian-<id>".
func ContainerName() string {
	if id := InstanceID(); id != "" {
		return "cloister-guardian-" + id
	}
	return "cloister-guardian"
}

// Host returns the guardian's network hostname. The container name doubles as
// the DNS hostname on the cloister-net Docker network.
func Host() string {
	return ContainerName()
}

// ProxyEnvVars returns environment variables for configuring a container
// to use the guardian proxy with the given token for authentication.
//
// The returned slice contains:
//   - CLOISTER_TOKEN: the authentication token
//   - CLOISTER_GUARDIAN_HOST: the guardian hostname (for hostexec and other tools)
//   - HTTP_PROXY: proxy URL with embedded credentials
//   - HTTPS_PROXY: same proxy URL (for tools that check HTTPS_PROXY)
//   - http_proxy: lowercase variant for compatibility
//   - https_proxy: lowercase variant for compatibility
//   - NO_PROXY: hosts that bypass the proxy (guardian, localhost)
//   - no_proxy: lowercase variant for compatibility
//   - CLOISTER_REQUEST_PORT: port for hostexec requests (default 9998)
//
// The proxy URL format is: http://token:$token@$host:$port
// Using "token" as the username and the actual token as the password.
func ProxyEnvVars(tok, guardianHost string) []string {
	if guardianHost == "" {
		guardianHost = Host()
	}

	proxyURL := fmt.Sprintf("http://token:%s@%s:%d", tok, guardianHost, DefaultProxyPort)
	noProxy := fmt.Sprintf("%s,localhost,127.0.0.1,::1,::", guardianHost)

	return []string{
		"CLOISTER_TOKEN=" + tok,
		"CLOISTER_GUARDIAN_HOST=" + guardianHost,
		"CLOISTER_REQUEST_PORT=" + fmt.Sprintf("%d", request.DefaultRequestPort),
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
		"NO_PROXY=" + noProxy,
		"no_proxy=" + noProxy,
	}
}

// FindFreePort asks the OS for an available TCP port.
// This is used for dynamic port allocation in test instances.
//
// Note: There is an inherent TOCTOU (time-of-check to time-of-use) race between
// when this function returns and when the caller binds to the port. Another
// process could theoretically grab the port in between. This is a known
// limitation of ephemeral port allocation and is acceptable for test isolation
// where port collisions are rare.
func FindFreePort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen on tcp: %w", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			clog.Warn("failed to close listener for port discovery: %v", err)
		}
	}()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected address type: %T", l.Addr())
	}
	return addr.Port, nil
}

// DefaultApprovalPort is the default port for the guardian approval web UI.
const DefaultApprovalPort = 9999

// Ports returns the ports to use for the guardian token API and approval server.
// For production (no instance ID), returns fixed ports 9997 and 9999.
// For test instances, allocates dynamic ports.
func Ports() (tokenPort, approvalPort int, err error) {
	if InstanceID() == "" {
		return DefaultAPIPort, DefaultApprovalPort, nil
	}
	tokenPort, err = FindFreePort()
	if err != nil {
		return 0, 0, err
	}
	approvalPort, err = FindFreePort()
	if err != nil {
		return 0, 0, err
	}
	return tokenPort, approvalPort, nil
}
