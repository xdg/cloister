package container

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/docker"
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

func TestManager_Start_Stop(t *testing.T) {
	requireDocker(t)

	projectName := testProjectName()
	branchName := "main"
	containerName := GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Configure the container
	cfg := &Config{
		Project:     projectName,
		Branch:      branchName,
		ProjectPath: tmpDir,
		Image:       "alpine:latest", // Use a small, common image for testing
	}

	manager := NewManager()

	// Test Start
	containerID, err := manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if containerID == "" {
		t.Error("Start() returned empty container ID")
	}

	// Verify container is running
	containers, err := manager.List()
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
	err = manager.Stop(containerName)
	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// Verify container is removed
	containers, err = manager.List()
	if err != nil {
		t.Fatalf("List() after stop failed: %v", err)
	}

	for _, c := range containers {
		if c.Name == containerName {
			t.Errorf("Container %q still exists after Stop()", containerName)
		}
	}
}

func TestManager_Start_AlreadyExists(t *testing.T) {
	requireDocker(t)

	projectName := testProjectName()
	branchName := "exists-test"
	containerName := GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		Project:     projectName,
		Branch:      branchName,
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	// Start the first container
	_, err = manager.Start(cfg)
	if err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	// Try to start again - should return ErrContainerExists
	_, err = manager.Start(cfg)
	if !errors.Is(err, ErrContainerExists) {
		t.Errorf("Second Start() error = %v, want ErrContainerExists", err)
	}
}

func TestManager_Stop_NotFound(t *testing.T) {
	requireDocker(t)

	manager := NewManager()

	// Try to stop a non-existent container
	err := manager.Stop("cloister-nonexistent-container-12345")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("Stop() error = %v, want ErrContainerNotFound", err)
	}
}

func TestManager_List_FiltersCloisterContainers(t *testing.T) {
	requireDocker(t)

	manager := NewManager()

	// List should only return cloister-* containers
	containers, err := manager.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	// All returned containers should have cloister- prefix
	for _, c := range containers {
		if !strings.HasPrefix(c.Name, "cloister-") {
			t.Errorf("List() returned non-cloister container: %q", c.Name)
		}
	}
}

func TestManager_Start_VerifySecuritySettings(t *testing.T) {
	requireDocker(t)

	projectName := testProjectName()
	branchName := "security-test"
	containerName := GenerateContainerName(projectName, branchName)

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		Project:     projectName,
		Branch:      branchName,
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
		Network:     "", // No network for this test
	}

	manager := NewManager()

	containerID, err := manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Inspect container to verify settings
	var inspectResult []struct {
		Config struct {
			User       string `json:"User"`
			WorkingDir string `json:"WorkingDir"`
		} `json:"Config"`
		HostConfig struct {
			CapDrop     []string `json:"CapDrop"`
			SecurityOpt []string `json:"SecurityOpt"`
			NetworkMode string   `json:"NetworkMode"`
		} `json:"HostConfig"`
		Mounts []struct {
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
	}

	output, err := docker.Run("inspect", containerID)
	if err != nil {
		t.Fatalf("docker inspect failed: %v", err)
	}

	// Parse the JSON output manually (docker inspect returns an array)
	if err := parseInspectOutput(output, &inspectResult); err != nil {
		t.Fatalf("Failed to parse inspect output: %v", err)
	}

	if len(inspectResult) == 0 {
		t.Fatal("No inspect results returned")
	}

	inspect := inspectResult[0]

	// Verify working directory
	if inspect.Config.WorkingDir != DefaultWorkDir {
		t.Errorf("WorkingDir = %q, want %q", inspect.Config.WorkingDir, DefaultWorkDir)
	}

	// Verify user ID
	if inspect.Config.User != "1000" {
		t.Errorf("User = %q, want %q", inspect.Config.User, "1000")
	}

	// Verify cap drop
	hasCapDropAll := false
	for _, cap := range inspect.HostConfig.CapDrop {
		if cap == "ALL" {
			hasCapDropAll = true
			break
		}
	}
	if !hasCapDropAll {
		t.Errorf("CapDrop does not include ALL: %v", inspect.HostConfig.CapDrop)
	}

	// Verify security options
	hasNoNewPrivileges := false
	for _, opt := range inspect.HostConfig.SecurityOpt {
		if opt == "no-new-privileges" {
			hasNoNewPrivileges = true
			break
		}
	}
	if !hasNoNewPrivileges {
		t.Errorf("SecurityOpt does not include no-new-privileges: %v", inspect.HostConfig.SecurityOpt)
	}

	// Verify mount
	foundMount := false
	for _, mount := range inspect.Mounts {
		if mount.Destination == DefaultWorkDir {
			foundMount = true
			if mount.Source != tmpDir {
				t.Errorf("Mount source = %q, want %q", mount.Source, tmpDir)
			}
			if !mount.RW {
				t.Error("Mount should be read-write")
			}
			break
		}
	}
	if !foundMount {
		t.Errorf("No mount found at %s", DefaultWorkDir)
	}
}

// parseInspectOutput parses docker inspect JSON output.
func parseInspectOutput(output string, result any) error {
	// docker inspect returns JSON directly, not wrapped in {{json .}}
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}

	// Use standard JSON parsing
	return json.Unmarshal([]byte(output), result)
}
