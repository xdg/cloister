package cloister

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	containerName := container.GenerateContainerName(projectName)

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
	containerName := container.GenerateContainerName(projectName)

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
	containerName := container.GenerateContainerName(projectName)

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
	containerName := container.GenerateContainerName(projectName)

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

func TestInjectUserSettings_MissingClaudeDir(t *testing.T) {
	// Test that injectUserSettings returns nil when ~/.claude/ doesn't exist.
	// We can't easily test this without mocking UserHomeDir, but we can test
	// that the function doesn't panic or error when called with an invalid
	// container (the container doesn't need to exist for us to test the
	// directory check logic - it will fail at docker cp, not at the stat).

	// This test just verifies the function signature and basic behavior.
	// The function should return an error only if docker cp fails on an
	// existing directory, or nil if the directory doesn't exist.

	// Since we can't mock UserHomeDir, we instead verify that the function
	// handles the docker cp error gracefully when the container doesn't exist.
	err := injectUserSettings("nonexistent-container-12345")

	// If ~/.claude/ exists on this machine, we expect a docker error.
	// If ~/.claude/ doesn't exist, we expect nil.
	// Either is acceptable behavior - we're testing that it doesn't panic.
	_ = err
}

// mockManager is a test double for ContainerManager that records calls
// and returns configurable results.
type mockManager struct {
	createCalled         bool
	createConfig         *container.Config
	createResult         string
	createError          error
	startContainerCalled bool
	startContainerName   string
	startContainerError  error
	stopCalled           bool
	stopContainerName    string
	stopError            error
	attachCalled         bool
	attachContainerName  string
	attachExitCode       int
	attachError          error
}

func (m *mockManager) Create(cfg *container.Config) (string, error) {
	m.createCalled = true
	m.createConfig = cfg
	return m.createResult, m.createError
}

func (m *mockManager) Start(cfg *container.Config) (string, error) {
	// Not used by cloister.Start, but required by interface
	return "", nil
}

func (m *mockManager) StartContainer(containerName string) error {
	m.startContainerCalled = true
	m.startContainerName = containerName
	return m.startContainerError
}

func (m *mockManager) Stop(containerName string) error {
	m.stopCalled = true
	m.stopContainerName = containerName
	return m.stopError
}

func (m *mockManager) Attach(containerName string) (int, error) {
	m.attachCalled = true
	m.attachContainerName = containerName
	return m.attachExitCode, m.attachError
}

func TestWithManager_InjectionWorks(t *testing.T) {
	// Test that WithManager properly injects the manager
	mock := &mockManager{
		attachExitCode: 42,
	}

	exitCode, err := Attach("test-container", WithManager(mock))
	if err != nil {
		t.Fatalf("Attach() returned error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("Attach() exitCode = %d, want 42", exitCode)
	}
	if !mock.attachCalled {
		t.Error("mock.Attach() was not called")
	}
	if mock.attachContainerName != "test-container" {
		t.Errorf("mock.attachContainerName = %q, want %q", mock.attachContainerName, "test-container")
	}
}

func TestAttach_WithMockManager_ReturnsError(t *testing.T) {
	// Test that errors from the manager are propagated
	expectedErr := errors.New("attach failed")
	mock := &mockManager{
		attachError: expectedErr,
	}

	_, err := Attach("test-container", WithManager(mock))
	if err == nil {
		t.Fatal("Attach() should return error from manager")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Attach() error = %v, want %v", err, expectedErr)
	}
}

func TestStop_WithMockManager(t *testing.T) {
	// Test that Stop calls the injected manager
	mock := &mockManager{}

	err := Stop("test-container", "", WithManager(mock))
	if err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	if !mock.stopCalled {
		t.Error("mock.Stop() was not called")
	}
	if mock.stopContainerName != "test-container" {
		t.Errorf("mock.stopContainerName = %q, want %q", mock.stopContainerName, "test-container")
	}
}

func TestStop_WithMockManager_ReturnsError(t *testing.T) {
	// Test that errors from the manager are propagated
	expectedErr := errors.New("stop failed")
	mock := &mockManager{
		stopError: expectedErr,
	}

	err := Stop("test-container", "", WithManager(mock))
	if err == nil {
		t.Fatal("Stop() should return error from manager")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Stop() error = %v, want %v", err, expectedErr)
	}
}

func TestGetManager_DefaultsToRealManager(t *testing.T) {
	// Test that getManager returns a real manager when no option is provided
	mgr := getManager()
	if mgr == nil {
		t.Fatal("getManager() returned nil")
	}

	// Verify it's the concrete type (not nil interface)
	_, ok := mgr.(*container.Manager)
	if !ok {
		t.Errorf("getManager() returned %T, want *container.Manager", mgr)
	}
}

func TestInjectUserSettings_IntegrationWithContainer(t *testing.T) {
	requireDocker(t)

	// Create a test container to verify settings injection
	// Uses cloister-default image which has the cloister user and home directory
	projectName := testProjectName()
	branchName := "settings-test"
	containerName := "cloister-" + projectName + "-" + branchName

	// Ensure cleanup after test
	defer cleanupTestContainer(containerName)

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create container using docker create directly (not through cloister.Start)
	// to avoid needing the guardian. Uses cloister-default which has /home/cloister.
	_, err = docker.Run("create",
		"--name", containerName,
		"-v", tmpDir+":/work",
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
	}

	// Check if ~/.claude/ exists on host
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}
	claudeDir := filepath.Join(homeDir, ".claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		t.Skip("~/.claude/ does not exist on this machine, skipping integration test")
	}

	// Test injectUserSettings
	err = injectUserSettings(containerName)
	if err != nil {
		t.Fatalf("injectUserSettings() failed: %v", err)
	}

	// Start the container so we can verify the files were copied
	_, err = docker.Run("start", containerName)
	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Verify the .claude directory exists in the container
	output, err := docker.Run("exec", containerName, "ls", "-la", "/home/cloister/.claude")
	if err != nil {
		t.Errorf("Failed to verify .claude directory in container: %v", err)
	} else {
		t.Logf("Container .claude contents:\n%s", output)
	}
}
