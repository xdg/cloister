package guardian

import (
	"context"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/token"
)

// mockDockerOpsRunning is a mock DockerOps that reports the guardian as running.
type mockDockerOpsRunning struct{}

func (mockDockerOpsRunning) CheckDaemon() error              { return nil }
func (mockDockerOpsRunning) EnsureCloisterNetwork() error    { return nil }
func (mockDockerOpsRunning) Run(_ ...string) (string, error) { return "", nil }
func (mockDockerOpsRunning) FindContainerByExactName(_ string) (*docker.ContainerInfo, error) {
	return &docker.ContainerInfo{
		ID:    "fake-id",
		Names: "/" + ContainerName(),
		State: "running",
	}, nil
}

func TestFindTokenForContainer(t *testing.T) {
	registry := newMockRegistry()
	registry.tokens["token-aaa"] = token.Info{CloisterName: "myproject-main"}
	registry.tokens["token-bbb"] = token.Info{CloisterName: "myproject-feature"}

	api := NewAPIServer("127.0.0.1:0", registry)
	if err := api.Start(); err != nil {
		t.Fatalf("failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.Stop(ctx)
	}()

	// Override the API address to point to our test server.
	origAddr := overrideAPIAddr
	overrideAPIAddr = api.ListenAddr()
	defer func() { overrideAPIAddr = origAddr }()

	// Mock Docker ops so IsRunning() returns true.
	SetDockerOps(mockDockerOpsRunning{})
	defer SetDockerOps(nil)

	tests := []struct {
		name          string
		containerName string
		wantToken     string
	}{
		{
			name:          "finds matching container",
			containerName: "myproject-main",
			wantToken:     "token-aaa",
		},
		{
			name:          "finds second container",
			containerName: "myproject-feature",
			wantToken:     "token-bbb",
		},
		{
			name:          "returns empty for unknown container",
			containerName: "unknown-container",
			wantToken:     "",
		},
		{
			name:          "returns empty for empty name",
			containerName: "",
			wantToken:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindTokenForContainer(tt.containerName)
			if got != tt.wantToken {
				t.Errorf("FindTokenForContainer(%q) = %q, want %q", tt.containerName, got, tt.wantToken)
			}
		})
	}
}

func TestFindTokenForContainer_ErrorReturnsEmpty(t *testing.T) {
	// With default Docker ops (no Docker available), ListTokens will fail.
	// FindTokenForContainer should return "" on error.
	SetDockerOps(nil)
	got := FindTokenForContainer("any-container")
	if got != "" {
		t.Errorf("FindTokenForContainer on error = %q, want empty string", got)
	}
}
