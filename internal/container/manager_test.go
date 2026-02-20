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

func TestManager_HasRunningCloister(t *testing.T) {
	tests := []struct {
		name        string
		containers  []Info
		projectName string
		want        string
	}{
		{
			name: "matches running container with branch suffix",
			containers: []Info{
				{Name: "/cloister-myproj-main", State: "running"},
			},
			projectName: "myproj",
			want:        "myproj-main",
		},
		{
			name: "matches exact project name",
			containers: []Info{
				{Name: "/cloister-myproj", State: "running"},
			},
			projectName: "myproj",
			want:        "myproj",
		},
		{
			name: "skips guardian",
			containers: []Info{
				{Name: "/cloister-guardian", State: "running"},
			},
			projectName: "guardian",
			want:        "",
		},
		{
			name: "skips non-running containers",
			containers: []Info{
				{Name: "/cloister-myproj-main", State: "exited"},
			},
			projectName: "myproj",
			want:        "",
		},
		{
			name: "no match for different project",
			containers: []Info{
				{Name: "/cloister-other-main", State: "running"},
			},
			projectName: "myproj",
			want:        "",
		},
		{
			name:        "no containers at all",
			containers:  []Info{},
			projectName: "myproj",
			want:        "",
		},
		{
			name: "does not match partial project name",
			containers: []Info{
				{Name: "/cloister-myproject-main", State: "running"},
			},
			projectName: "myproj",
			want:        "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockDockerRunner{
				runJSONLinesFunc: func(result any, _ bool, _ ...string) error {
					containers, ok := result.(*[]Info)
					if !ok {
						t.Fatal("Expected *[]Info")
					}
					*containers = tc.containers
					return nil
				},
			}
			mgr := NewManagerWithRunner(mock)
			got, err := mgr.HasRunningCloister(tc.projectName)
			if err != nil {
				t.Fatalf("HasRunningCloister() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("HasRunningCloister(%q) = %q, want %q", tc.projectName, got, tc.want)
			}
		})
	}
}

func TestManager_HasRunningCloister_ListError(t *testing.T) {
	mock := &mockDockerRunner{
		runJSONLinesFunc: func(_ any, _ bool, _ ...string) error {
			return errors.New("docker not available")
		},
	}
	mgr := NewManagerWithRunner(mock)
	got, err := mgr.HasRunningCloister("myproj")
	if err != nil {
		t.Fatalf("HasRunningCloister() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("HasRunningCloister() = %q, want empty string on List error", got)
	}
}
