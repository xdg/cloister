//go:build integration

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
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &Config{
		Project:     projectName,
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	containerID, err := manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if containerID == "" {
		t.Error("Start() returned empty container ID")
	}

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

	if err := manager.Stop(containerName); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

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
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &Config{
		Project:     projectName,
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	if _, err = manager.Start(cfg); err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	_, err = manager.Start(cfg)
	if !errors.Is(err, ErrContainerExists) {
		t.Errorf("Second Start() error = %v, want ErrContainerExists", err)
	}
}

func TestManager_Stop_NotFound(t *testing.T) {
	requireDocker(t)

	manager := NewManager()
	err := manager.Stop("cloister-nonexistent-container-12345")
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("Stop() error = %v, want ErrContainerNotFound", err)
	}
}

func TestManager_List_FiltersCloisterContainers(t *testing.T) {
	requireDocker(t)

	manager := NewManager()
	containers, err := manager.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

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
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &Config{
		Project:     projectName,
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
		Network:     "",
	}

	manager := NewManager()

	containerID, err := manager.Start(cfg)
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

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

	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &inspectResult); err != nil {
		t.Fatalf("Failed to parse inspect output: %v", err)
	}

	if len(inspectResult) == 0 {
		t.Fatal("No inspect results returned")
	}

	inspect := inspectResult[0]

	if inspect.Config.WorkingDir != DefaultWorkDir {
		t.Errorf("WorkingDir = %q, want %q", inspect.Config.WorkingDir, DefaultWorkDir)
	}

	if inspect.Config.User != "1000" {
		t.Errorf("User = %q, want %q", inspect.Config.User, "1000")
	}

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

func TestManager_ContainerStatus_NonExistent(t *testing.T) {
	requireDocker(t)

	manager := NewManager()
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
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &Config{
		Project:     projectName,
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	if _, err = manager.Start(cfg); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

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
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &Config{
		Project:     projectName,
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	if _, err = manager.Create(cfg); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

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
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &Config{
		Project:     projectName,
		Branch:      "main",
		ProjectPath: tmpDir,
		Image:       "alpine:latest",
	}

	manager := NewManager()

	if _, err = manager.Start(cfg); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	exists, running, err := manager.ContainerStatus(containerName)
	if err != nil {
		t.Fatalf("ContainerStatus() error = %v", err)
	}

	if !exists {
		t.Error("ContainerStatus() exists = false, but container was just started")
	}
	if !running {
		t.Error("ContainerStatus() running = false, but container should be running")
	}

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
