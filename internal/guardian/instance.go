// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
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
	_, _ = rand.Read(b)
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

// FindFreePort asks the OS for an available TCP port.
// This is used for dynamic port allocation in test instances.
//
// Note: There is an inherent TOCTOU (time-of-check to time-of-use) race between
// when this function returns and when the caller binds to the port. Another
// process could theoretically grab the port in between. This is a known
// limitation of ephemeral port allocation and is acceptable for test isolation
// where port collisions are rare.
func FindFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected address type: %T", l.Addr())
	}
	return addr.Port, nil
}

// DefaultTokenAPIPort is the default port for the guardian token management API.
const DefaultTokenAPIPort = 9997

// DefaultApprovalPort is the default port for the guardian approval web UI.
const DefaultApprovalPort = 9999

// GuardianPorts returns the ports to use for the guardian token API and approval server.
// For production (no instance ID), returns fixed ports 9997 and 9999.
// For test instances, allocates dynamic ports.
func GuardianPorts() (tokenPort, approvalPort int, err error) {
	if InstanceID() == "" {
		return DefaultTokenAPIPort, DefaultApprovalPort, nil
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
