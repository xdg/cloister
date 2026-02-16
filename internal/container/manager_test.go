package container

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/docker"
)

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
	var runCalls [][]string

	mock := &mockDockerRunner{
		findContainerByExactNameFn: func(_ string) (*docker.ContainerInfo, error) {
			return nil, nil
		},
		runFunc: func(args ...string) (string, error) {
			runCalls = append(runCalls, args)
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

	if len(runCalls) != 1 {
		t.Fatalf("Expected 1 Run call, got %d", len(runCalls))
	}

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
		findContainerByExactNameFn: func(_ string) (*docker.ContainerInfo, error) {
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

	if err := manager.Stop("cloister-test"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

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
		findContainerByExactNameFn: func(_ string) (*docker.ContainerInfo, error) {
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
		runJSONLinesFunc: func(result any, _ bool, _ ...string) error {
			containers, ok := result.(*[]Info)
			if !ok {
				t.Fatal("Expected *[]Info")
			}
			*containers = []Info{
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

	if containers[0].Name != "cloister-project-main" {
		t.Errorf("First container name = %q, want %q", containers[0].Name, "cloister-project-main")
	}
}
