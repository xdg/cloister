//go:build e2e

// Package e2e contains end-to-end tests that verify cloister workflows
// with a running guardian. Tests in this package assume the guardian is
// managed by TestMain - they do not start/stop it themselves.
package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// writeTestConfig creates a custom guardian config for e2e tests.
// The config is based on defaults but customized for test requirements:
// - unlisted_domain_behavior: "request_approval" - enables domain approval persistence testing
// - approval_timeout: "3s" - short timeout so blocked-domain tests don't hang
func writeTestConfig() error {
	// Start with production defaults
	cfg := config.DefaultGlobalConfig()

	// Enable domain approval flow so persistence tests can exercise the
	// DomainQueue -> approval -> ConfigPersister path. Using a short
	// timeout (3s) ensures that tests waiting for an unlisted domain
	// to be rejected (e.g., TestProxy_BlockedDomain) do not hang.
	cfg.Proxy.UnlistedDomainBehavior = "request_approval"
	cfg.Proxy.ApprovalTimeout = "3s"

	// Write to XDG_CONFIG_HOME/cloister/config.yaml
	// (XDG_CONFIG_HOME is already set to temp dir by TestMain)
	return config.WriteGlobalConfig(cfg)
}

// TestMain sets up the guardian for all e2e tests and tears it down on exit.
// This allows tests to share a single guardian instance, which is more efficient
// and matches the production model where guardian runs persistently.
func TestMain(m *testing.M) {
	// Isolate XDG directories so tests don't touch ~/.config/cloister or ~/.local/state/cloister
	tempConfigDir, err := os.MkdirTemp("", "cloister-e2e-config-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Could not create temp config dir: %v\n", err)
		os.Exit(0)
	}
	tempStateDir, err := os.MkdirTemp("", "cloister-e2e-state-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Could not create temp state dir: %v\n", err)
		os.RemoveAll(tempConfigDir)
		os.Exit(0)
	}
	os.Setenv("XDG_CONFIG_HOME", tempConfigDir)
	os.Setenv("XDG_STATE_HOME", tempStateDir)

	// Generate unique instance ID for test isolation.
	// This allows tests to run without conflicting with a production guardian
	// or with tests running in other worktrees.
	os.Setenv(guardian.InstanceIDEnvVar, guardian.GenerateInstanceID())

	// Write custom config for e2e tests
	if err := writeTestConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Could not write test config: %v\n", err)
		os.RemoveAll(tempConfigDir)
		os.RemoveAll(tempStateDir)
		os.Exit(0)
	}

	// Check Docker availability first
	if err := docker.CheckDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Docker not available: %v\n", err)
		os.RemoveAll(tempConfigDir)
		os.RemoveAll(tempStateDir)
		os.Exit(0)
	}

	// Set up cloister binary path for executor spawning
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintf(os.Stderr, "SKIP: Could not determine test file location\n")
		os.RemoveAll(tempConfigDir)
		os.RemoveAll(tempStateDir)
		os.Exit(0)
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	binaryPath := filepath.Join(repoRoot, "cloister")
	if _, err := os.Stat(binaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cloister binary not found at %s (run 'make build' first)\n", binaryPath)
		os.RemoveAll(tempConfigDir)
		os.RemoveAll(tempStateDir)
		os.Exit(0)
	}
	os.Setenv(guardian.ExecutableEnvVar, binaryPath)

	// Start the guardian for the test run (no need to stop existing guardian
	// since we have our own isolated instance)
	if err := guardian.EnsureRunning(); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: Could not start guardian: %v\n", err)
		os.RemoveAll(tempConfigDir)
		os.RemoveAll(tempStateDir)
		os.Exit(0)
	}

	// Run tests
	code := m.Run()

	// Cleanup: stop the guardian and remove temp dirs
	if err := guardian.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stop guardian: %v\n", err)
	}
	os.RemoveAll(tempConfigDir)
	os.RemoveAll(tempStateDir)

	os.Exit(code)
}
