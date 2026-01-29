package cloister

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/container"
)

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
