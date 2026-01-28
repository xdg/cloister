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
	containerName := GenerateContainerName(projectName)

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
		Branch:      "main",
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
	containerName := GenerateContainerName(projectName)

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
		Branch:      "main",
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
	containerName := GenerateContainerName(projectName)

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
		Branch:      "main",
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

func TestManager_ContainerStatus_NonExistent(t *testing.T) {
	requireDocker(t)

	manager := NewManager()

	// Test with a container that doesn't exist
	exists, running, err := manager.ContainerStatus("cloister-nonexistent-container-12345")
	if err != nil {
		t.Fatalf("ContainerStatus() error = %v, want nil", err)
	}
	if exists {
		t.Error("ContainerStatus() exists = true, want false for non-existent container")
	}
	if running {
		t.Error("ContainerStatus() running = true, want false for non-existent container")
	}
}

func TestManager_ContainerStatus_Running(t *testing.T) {
	requireDocker(t)

	projectName := testProjectName()
	containerName := GenerateContainerName(projectName)

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
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	// Start a container
	_, err = manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Test ContainerStatus for running container
	exists, running, err := manager.ContainerStatus(containerName)
	if err != nil {
		t.Fatalf("ContainerStatus() error = %v, want nil", err)
	}
	if !exists {
		t.Error("ContainerStatus() exists = false, want true for running container")
	}
	if !running {
		t.Error("ContainerStatus() running = false, want true for running container")
	}
}

func TestManager_ContainerStatus_Stopped(t *testing.T) {
	requireDocker(t)

	projectName := testProjectName()
	containerName := GenerateContainerName(projectName)

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
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	// Create container without starting it
	_, err = manager.Create(cfg)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Test ContainerStatus for created but not started container
	exists, running, err := manager.ContainerStatus(containerName)
	if err != nil {
		t.Fatalf("ContainerStatus() error = %v, want nil", err)
	}
	if !exists {
		t.Error("ContainerStatus() exists = false, want true for created container")
	}
	if running {
		t.Error("ContainerStatus() running = true, want false for stopped container")
	}
}

func TestManager_ContainerStatus_SingleDockerCall(t *testing.T) {
	requireDocker(t)

	projectName := testProjectName()
	containerName := GenerateContainerName(projectName)

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
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	// Start a container
	_, err = manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Verify that ContainerStatus returns both values correctly in a single call
	// This test documents that both exists and running come from the same underlying call
	exists, running, err := manager.ContainerStatus(containerName)
	if err != nil {
		t.Fatalf("ContainerStatus() error = %v", err)
	}

	// Verify consistency: for a running container, both should be true
	if !exists {
		t.Error("ContainerStatus() exists = false, but container was just started")
	}
	if !running {
		t.Error("ContainerStatus() running = false, but container should be running")
	}

	// Verify that the old methods still work and return consistent results
	existsOnly, err := manager.containerExists(containerName)
	if err != nil {
		t.Fatalf("containerExists() error = %v", err)
	}
	if existsOnly != exists {
		t.Errorf("containerExists() = %v, but ContainerStatus() returned exists = %v", existsOnly, exists)
	}

	runningOnly, err := manager.IsRunning(containerName)
	if err != nil {
		t.Fatalf("IsRunning() error = %v", err)
	}
	if runningOnly != running {
		t.Errorf("IsRunning() = %v, but ContainerStatus() returned running = %v", runningOnly, running)
	}
}

// mockDockerRunner is a test double for DockerRunner.
type mockDockerRunner struct {
	runFunc                    func(args ...string) (string, error)
	runJSONLinesFunc           func(result any, strict bool, args ...string) error
	findContainerByExactNameFn func(name string) (*docker.ContainerInfo, error)
	startContainerFunc         func(name string) error
}

func (m *mockDockerRunner) Run(args ...string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(args...)
	}
	return "", nil
}

func (m *mockDockerRunner) RunJSONLines(result any, strict bool, args ...string) error {
	if m.runJSONLinesFunc != nil {
		return m.runJSONLinesFunc(result, strict, args...)
	}
	return nil
}

func (m *mockDockerRunner) FindContainerByExactName(name string) (*docker.ContainerInfo, error) {
	if m.findContainerByExactNameFn != nil {
		return m.findContainerByExactNameFn(name)
	}
	return nil, nil
}

func (m *mockDockerRunner) StartContainer(name string) error {
	if m.startContainerFunc != nil {
		return m.startContainerFunc(name)
	}
	return nil
}

func TestManager_WithMockRunner_Start(t *testing.T) {
	// Track docker calls
	var runCalls [][]string

	mock := &mockDockerRunner{
		findContainerByExactNameFn: func(name string) (*docker.ContainerInfo, error) {
			// Container doesn't exist yet
			return nil, nil
		},
		runFunc: func(args ...string) (string, error) {
			runCalls = append(runCalls, args)
			// Return a fake container ID
			return "abc123def456", nil
		},
	}

	manager := NewManagerWithRunner(mock)

	cfg := &Config{
		Project:     "test-project",
		Branch:      "main",
		ProjectPath: "/tmp/test",
		Image:       "test-image:latest",
	}

	containerID, err := manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if containerID != "abc123def456" {
		t.Errorf("Start() = %q, want %q", containerID, "abc123def456")
	}

	// Verify docker run was called
	if len(runCalls) != 1 {
		t.Fatalf("Expected 1 Run call, got %d", len(runCalls))
	}

	// Verify the command starts with "run" and "-d"
	if runCalls[0][0] != "run" {
		t.Errorf("Expected first arg to be 'run', got %q", runCalls[0][0])
	}
	if runCalls[0][1] != "-d" {
		t.Errorf("Expected second arg to be '-d', got %q", runCalls[0][1])
	}
}

func TestManager_WithMockRunner_Stop(t *testing.T) {
	var runCalls [][]string

	mock := &mockDockerRunner{
		findContainerByExactNameFn: func(name string) (*docker.ContainerInfo, error) {
			// Container exists
			return &docker.ContainerInfo{
				ID:    "abc123",
				Names: "cloister-test",
				State: "running",
			}, nil
		},
		runFunc: func(args ...string) (string, error) {
			runCalls = append(runCalls, args)
			return "", nil
		},
	}

	manager := NewManagerWithRunner(mock)

	err := manager.Stop("cloister-test")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify stop and rm were called
	if len(runCalls) != 2 {
		t.Fatalf("Expected 2 Run calls (stop + rm), got %d", len(runCalls))
	}

	if runCalls[0][0] != "stop" {
		t.Errorf("Expected first call to be 'stop', got %q", runCalls[0][0])
	}
	if runCalls[1][0] != "rm" {
		t.Errorf("Expected second call to be 'rm', got %q", runCalls[1][0])
	}
}

func TestManager_WithMockRunner_ContainerNotFound(t *testing.T) {
	mock := &mockDockerRunner{
		findContainerByExactNameFn: func(name string) (*docker.ContainerInfo, error) {
			// Container doesn't exist
			return nil, nil
		},
	}

	manager := NewManagerWithRunner(mock)

	err := manager.Stop("nonexistent")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("Stop() error = %v, want ErrContainerNotFound", err)
	}
}

func TestManager_WithMockRunner_List(t *testing.T) {
	mock := &mockDockerRunner{
		runJSONLinesFunc: func(result any, strict bool, args ...string) error {
			containers, ok := result.(*[]ContainerInfo)
			if !ok {
				t.Fatal("Expected *[]ContainerInfo")
			}
			*containers = []ContainerInfo{
				{ID: "abc123", Name: "/cloister-project-main", State: "running"},
				{ID: "def456", Name: "/cloister-other-dev", State: "exited"},
			}
			return nil
		},
	}

	manager := NewManagerWithRunner(mock)

	containers, err := manager.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(containers) != 2 {
		t.Fatalf("List() returned %d containers, want 2", len(containers))
	}

	// Verify names have leading slash stripped
	if containers[0].Name != "cloister-project-main" {
		t.Errorf("First container name = %q, want %q", containers[0].Name, "cloister-project-main")
	}
}
