// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"fmt"
	"os"
	"path/filepath"
)

// HostSocketDir returns the directory for the hostexec socket on the host.
// This is ~/.local/share/cloister.
func HostSocketDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "cloister"), nil
}

// HostSocketPath returns the path to the hostexec socket on the host.
// This is ~/.local/share/cloister/hostexec.sock.
func HostSocketPath() (string, error) {
	dir, err := HostSocketDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hostexec.sock"), nil
}

// ContainerSocketPath is the path to the hostexec socket inside the guardian container.
const ContainerSocketPath = "/var/run/hostexec.sock"

// SharedSecretEnvVar is the environment variable name for the shared secret.
const SharedSecretEnvVar = "CLOISTER_SHARED_SECRET"
