package guardian

import (
	"path/filepath"
	"testing"
)

func TestHostSocketPath(t *testing.T) {
	path, err := HostSocketPath()
	if err != nil {
		t.Fatalf("HostSocketPath() error: %v", err)
	}

	// Verify path ends with expected filename
	if filepath.Base(path) != "hostexec.sock" {
		t.Errorf("Expected socket filename 'hostexec.sock', got %q", filepath.Base(path))
	}

	// Verify path contains expected directory
	if !filepath.IsAbs(path) {
		t.Errorf("Expected absolute path, got %q", path)
	}
}

func TestSharedSecretEnvVar(t *testing.T) {
	if SharedSecretEnvVar != "CLOISTER_SHARED_SECRET" {
		t.Errorf("Expected env var name CLOISTER_SHARED_SECRET, got %q", SharedSecretEnvVar)
	}
}

func TestContainerSocketPath(t *testing.T) {
	if ContainerSocketPath != "/var/run/hostexec.sock" {
		t.Errorf("Expected container socket path /var/run/hostexec.sock, got %q", ContainerSocketPath)
	}
}
