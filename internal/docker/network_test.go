//go:build integration

package docker

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestNetworkExists_BridgeNetwork(t *testing.T) {
	// The bridge network should always exist in a standard Docker installation
	exists, err := NetworkExists("bridge")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if !exists {
		t.Log("bridge network not found (may be normal in some Docker configurations)")
	}
}

func TestNetworkExists_NonExistent(t *testing.T) {
	exists, err := NetworkExists("cloister-test-nonexistent-network-12345")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if exists {
		t.Error("expected non-existent network to return false")
	}
}

func TestInspectNetwork_Bridge(t *testing.T) {
	info, err := InspectNetwork("bridge")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
			if strings.Contains(cmdErr.Stderr, "No such network") {
				t.Skip("bridge network not found (unusual Docker configuration)")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "bridge" {
		t.Errorf("expected name 'bridge', got %q", info.Name)
	}
	if info.Driver != "bridge" {
		t.Errorf("expected driver 'bridge', got %q", info.Driver)
	}
	// Standard bridge network is not internal
	if info.Internal {
		t.Error("expected bridge network to not be internal")
	}
}

func TestInspectNetwork_NonExistent(t *testing.T) {
	_, err := InspectNetwork("cloister-test-nonexistent-network-12345")
	if err == nil {
		t.Fatal("expected error for non-existent network")
	}

	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		var execErr *exec.Error
		if errors.As(cmdErr.Err, &execErr) {
			t.Skip("Docker CLI not installed")
		}
		// Check for daemon not running vs network not found
		if strings.Contains(cmdErr.Stderr, "Cannot connect to the Docker daemon") {
			t.Skip("Docker daemon not running")
		}
		// Expected: "No such network" or "network not found" error
		if !strings.Contains(cmdErr.Stderr, "No such network") && !strings.Contains(cmdErr.Stderr, "not found") {
			t.Errorf("expected 'No such network' error, got stderr: %s", cmdErr.Stderr)
		}
		return
	}

	// If not a CommandError, check if Docker daemon isn't running
	if errors.Is(err, ErrDockerNotRunning) {
		t.Skip("Docker daemon not running")
	}

	t.Logf("got expected error type: %T - %v", err, err)
}

func TestEnsureNetwork_CreateAndCleanup(t *testing.T) {
	testNetworkName := "cloister-test-network"

	// Cleanup function to ensure we don't leave test artifacts
	cleanup := func() {
		// Ignore errors from cleanup - network might not exist
		_, _ = Run("network", "rm", testNetworkName)
	}
	cleanup() // Clean up any previous test run
	defer cleanup()

	// Test creating an internal network
	err := EnsureNetwork(testNetworkName, true)
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("failed to create network: %v", err)
	}

	// Verify network exists
	exists, err := NetworkExists(testNetworkName)
	if err != nil {
		t.Fatalf("failed to check network existence: %v", err)
	}
	if !exists {
		t.Fatal("network should exist after EnsureNetwork")
	}

	// Verify internal flag is set correctly
	info, err := InspectNetwork(testNetworkName)
	if err != nil {
		t.Fatalf("failed to inspect network: %v", err)
	}
	if !info.Internal {
		t.Error("expected network to be internal")
	}

	// Test idempotency - calling EnsureNetwork again should succeed
	err = EnsureNetwork(testNetworkName, true)
	if err != nil {
		t.Errorf("EnsureNetwork should be idempotent: %v", err)
	}
}

func TestEnsureNetwork_ConfigMismatch(t *testing.T) {
	testNetworkName := "cloister-test-network-mismatch"

	// Cleanup function
	cleanup := func() {
		_, _ = Run("network", "rm", testNetworkName)
	}
	cleanup()
	defer cleanup()

	// Create a non-internal network first
	err := EnsureNetwork(testNetworkName, false)
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("failed to create network: %v", err)
	}

	// Verify it's not internal
	info, err := InspectNetwork(testNetworkName)
	if err != nil {
		t.Fatalf("failed to inspect network: %v", err)
	}
	if info.Internal {
		t.Error("expected network to NOT be internal")
	}

	// Now try to ensure it as internal - should return ErrNetworkConfigMismatch
	err = EnsureNetwork(testNetworkName, true)
	if err == nil {
		t.Fatal("expected ErrNetworkConfigMismatch when internal flag differs")
	}
	if !errors.Is(err, ErrNetworkConfigMismatch) {
		t.Errorf("expected ErrNetworkConfigMismatch, got: %v", err)
	}
}

func TestCloisterNetworkName(t *testing.T) {
	// Verify the constant has the expected value
	if CloisterNetworkName != "cloister-net" {
		t.Errorf("expected CloisterNetworkName to be 'cloister-net', got %q", CloisterNetworkName)
	}
}

func TestEnsureCloisterNetwork(t *testing.T) {
	// This test creates the actual cloister-net network, which may persist
	// We don't clean it up since it's the production network name
	// Instead, we just verify the function works correctly

	err := EnsureCloisterNetwork()
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("failed to ensure cloister network: %v", err)
	}

	// Verify network exists and is internal
	exists, err := NetworkExists(CloisterNetworkName)
	if err != nil {
		t.Fatalf("failed to check network existence: %v", err)
	}
	if !exists {
		t.Fatal("cloister network should exist after EnsureCloisterNetwork")
	}

	info, err := InspectNetwork(CloisterNetworkName)
	if err != nil {
		t.Fatalf("failed to inspect cloister network: %v", err)
	}
	if !info.Internal {
		t.Error("cloister network must be internal to prevent external access")
	}

	// Verify idempotency
	err = EnsureCloisterNetwork()
	if err != nil {
		t.Errorf("EnsureCloisterNetwork should be idempotent: %v", err)
	}
}

func TestNetworkExists_ExactMatch(t *testing.T) {
	// Test that NetworkExists uses exact matching, not substring matching
	// First check if "bridge" exists
	exists, err := NetworkExists("bridge")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) || strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if !exists {
		t.Skip("bridge network not found, cannot test exact matching")
	}

	// "bri" should not match "bridge"
	exists, err = NetworkExists("bri")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("NetworkExists should use exact matching, but 'bri' matched something")
	}

	// "bridgefoo" should not match "bridge"
	exists, err = NetworkExists("bridgefoo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("NetworkExists should use exact matching, but 'bridgefoo' matched something")
	}
}
