package cloister

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// testProjectName generates a unique test project name.
func testProjectName() string {
	return "test-" + time.Now().Format("20060102-150405")
}

// cleanupTestContainer removes a test container if it exists.
func cleanupTestContainer(name string) {
	// Best effort cleanup - ignore errors
	_, _ = docker.Run("rm", "-f", name)
}

// requireDocker skips the test if Docker is not available.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// requireGuardian skips the test if guardian cannot be started.
func requireGuardian(t *testing.T) {
	t.Helper()
	requireDocker(t)
	if err := guardian.EnsureRunning(); err != nil {
		t.Skipf("Guardian not available: %v", err)
	}
}

func TestStartOptions_Fields(t *testing.T) {
	// Test that StartOptions has expected fields
	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "myproject",
		BranchName:  "main",
		Image:       "custom:latest",
	}

	if opts.ProjectPath != "/path/to/project" {
		t.Errorf("ProjectPath = %q, want %q", opts.ProjectPath, "/path/to/project")
	}
	if opts.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", opts.ProjectName, "myproject")
	}
	if opts.BranchName != "main" {
		t.Errorf("BranchName = %q, want %q", opts.BranchName, "main")
	}
	if opts.Image != "custom:latest" {
		t.Errorf("Image = %q, want %q", opts.Image, "custom:latest")
	}
}

func TestStart_Stop_Integration(t *testing.T) {
	requireGuardian(t)

	projectName := testProjectName()
	branchName := "main"
	containerName := container.GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := StartOptions{
		ProjectPath: tmpDir,
		ProjectName: projectName,
		BranchName:  branchName,
		Image:       "alpine:latest",
	}

	// Test Start
	containerID, tok, err := Start(opts)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if containerID == "" {
		t.Error("Start() returned empty container ID")
	}
	if tok == "" {
		t.Error("Start() returned empty token")
	}
	if len(tok) != 64 {
		t.Errorf("Start() returned token of length %d, want 64", len(tok))
	}

	// Verify container is running
	mgr := container.NewManager()
	containers, err := mgr.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.Name == containerName {
			found = true
			if c.State != "running" {
				t.Errorf("Expected container state 'running', got %q", c.State)
			}
			break
		}
	}
	if !found {
		t.Errorf("Container %q not found in List()", containerName)
	}

	// Test Stop
	err = Stop(containerName, tok)
	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// Verify container is removed
	containers, err = mgr.List()
	if err != nil {
		t.Fatalf("List() after stop failed: %v", err)
	}

	for _, c := range containers {
		if c.Name == containerName {
			t.Errorf("Container %q still exists after Stop()", containerName)
		}
	}
}

func TestStart_InjectsEnvVars(t *testing.T) {
	requireGuardian(t)

	projectName := testProjectName()
	branchName := "env-test"
	containerName := container.GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := StartOptions{
		ProjectPath: tmpDir,
		ProjectName: projectName,
		BranchName:  branchName,
		Image:       "alpine:latest",
	}

	// Start the container
	containerID, tok, err := Start(opts)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() { _ = Stop(containerName, tok) }()

	// Inspect container to verify environment variables
	var inspectResult []struct {
		Config struct {
			Env []string `json:"Env"`
		} `json:"Config"`
	}

	output, err := docker.Run("inspect", containerID)
	if err != nil {
		t.Fatalf("docker inspect failed: %v", err)
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &inspectResult); err != nil {
		t.Fatalf("Failed to parse inspect output: %v", err)
	}

	if len(inspectResult) == 0 {
		t.Fatal("No inspect results returned")
	}

	envVars := inspectResult[0].Config.Env
	envMap := make(map[string]string)
	for _, e := range envVars {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Verify CLOISTER_TOKEN
	if tokenVal, ok := envMap["CLOISTER_TOKEN"]; !ok {
		t.Error("CLOISTER_TOKEN not set in container")
	} else if tokenVal != tok {
		t.Errorf("CLOISTER_TOKEN = %q, want %q", tokenVal, tok)
	}

	// Verify HTTP_PROXY
	if proxyVal, ok := envMap["HTTP_PROXY"]; !ok {
		t.Error("HTTP_PROXY not set in container")
	} else {
		expectedPrefix := "http://token:" + tok + "@cloister-guardian:3128"
		if proxyVal != expectedPrefix {
			t.Errorf("HTTP_PROXY = %q, want %q", proxyVal, expectedPrefix)
		}
	}

	// Verify HTTPS_PROXY
	if proxyVal, ok := envMap["HTTPS_PROXY"]; !ok {
		t.Error("HTTPS_PROXY not set in container")
	} else {
		expectedPrefix := "http://token:" + tok + "@cloister-guardian:3128"
		if proxyVal != expectedPrefix {
			t.Errorf("HTTPS_PROXY = %q, want %q", proxyVal, expectedPrefix)
		}
	}

	// Verify lowercase variants
	if _, ok := envMap["http_proxy"]; !ok {
		t.Error("http_proxy not set in container")
	}
	if _, ok := envMap["https_proxy"]; !ok {
		t.Error("https_proxy not set in container")
	}
}

func TestStart_ConnectsToCloisterNet(t *testing.T) {
	requireGuardian(t)

	projectName := testProjectName()
	branchName := "network-test"
	containerName := container.GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := StartOptions{
		ProjectPath: tmpDir,
		ProjectName: projectName,
		BranchName:  branchName,
		Image:       "alpine:latest",
	}

	// Start the container
	containerID, tok, err := Start(opts)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() { _ = Stop(containerName, tok) }()

	// Inspect container to verify network
	var inspectResult []struct {
		NetworkSettings struct {
			Networks map[string]struct {
				NetworkID string `json:"NetworkID"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}

	output, err := docker.Run("inspect", containerID)
	if err != nil {
		t.Fatalf("docker inspect failed: %v", err)
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &inspectResult); err != nil {
		t.Fatalf("Failed to parse inspect output: %v", err)
	}

	if len(inspectResult) == 0 {
		t.Fatal("No inspect results returned")
	}

	networks := inspectResult[0].NetworkSettings.Networks
	if _, ok := networks[docker.CloisterNetworkName]; !ok {
		t.Errorf("Container not connected to %s, connected networks: %v",
			docker.CloisterNetworkName, networks)
	}
}

func TestStop_WithEmptyToken(t *testing.T) {
	requireGuardian(t)

	projectName := testProjectName()
	branchName := "empty-token"
	containerName := container.GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := StartOptions{
		ProjectPath: tmpDir,
		ProjectName: projectName,
		BranchName:  branchName,
		Image:       "alpine:latest",
	}

	// Start the container
	_, tok, err := Start(opts)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Save the token for later revocation if needed
	_ = tok

	// Stop with empty token - should still stop the container
	err = Stop(containerName, "")
	if err != nil {
		t.Fatalf("Stop() with empty token failed: %v", err)
	}

	// Verify container is removed
	mgr := container.NewManager()
	containers, err := mgr.List()
	if err != nil {
		t.Fatalf("List() after stop failed: %v", err)
	}

	for _, c := range containers {
		if c.Name == containerName {
			t.Errorf("Container %q still exists after Stop()", containerName)
		}
	}
}

func TestStop_NonExistentContainer(t *testing.T) {
	requireDocker(t)

	// Stop a non-existent container - should return error
	err := Stop("cloister-nonexistent-12345", "sometoken")
	if err == nil {
		t.Error("Stop() with non-existent container should return error")
	}
}
