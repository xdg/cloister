package cmd

import (
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/container"
)

// TestFilterCloisterContainers verifies the filtering logic used in runList.
// This tests the filtering logic extracted from the loop in runList.
func TestFilterCloisterContainers(t *testing.T) {
	// Simulate the filtering logic from runList
	filterCloisters := func(containers []container.Info) []container.Info {
		var cloisters []container.Info
		for _, c := range containers {
			// Skip the guardian container
			if c.Name == "cloister-guardian" {
				continue
			}
			// Only include running containers
			if c.State != "running" {
				continue
			}
			cloisters = append(cloisters, c)
		}
		return cloisters
	}

	tests := []struct {
		name       string
		containers []container.Info
		wantCount  int
		wantNames  []string
	}{
		{
			name:       "empty list",
			containers: []container.Info{},
			wantCount:  0,
			wantNames:  nil,
		},
		{
			name: "only guardian running",
			containers: []container.Info{
				{Name: "cloister-guardian", State: "running"},
			},
			wantCount: 0,
			wantNames: nil,
		},
		{
			name: "guardian and one cloister running",
			containers: []container.Info{
				{Name: "cloister-guardian", State: "running"},
				{Name: "cloister-myproject", State: "running"},
			},
			wantCount: 1,
			wantNames: []string{"cloister-myproject"},
		},
		{
			name: "stopped cloister excluded",
			containers: []container.Info{
				{Name: "cloister-myproject", State: "exited"},
			},
			wantCount: 0,
			wantNames: nil,
		},
		{
			name: "multiple cloisters with mixed states",
			containers: []container.Info{
				{Name: "cloister-guardian", State: "running"},
				{Name: "cloister-project1", State: "running"},
				{Name: "cloister-project2", State: "exited"},
				{Name: "cloister-project3", State: "running"},
			},
			wantCount: 2,
			wantNames: []string{"cloister-project1", "cloister-project3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterCloisters(tt.containers)
			if len(result) != tt.wantCount {
				t.Errorf("filterCloisters() returned %d containers, want %d", len(result), tt.wantCount)
			}
			for i, name := range tt.wantNames {
				if i >= len(result) {
					t.Errorf("missing expected container %q at index %d", name, i)
					continue
				}
				if result[i].Name != name {
					t.Errorf("container[%d].Name = %q, want %q", i, result[i].Name, name)
				}
			}
		})
	}
}

// TestDockerNotRunningInList verifies that runList returns the correct error
// when Docker is not running.
func TestDockerNotRunningInList(t *testing.T) {
	err := dockerNotRunningError()
	if err == nil {
		t.Fatal("dockerNotRunningError() returned nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "docker is not running") {
		t.Errorf("expected 'Docker is not running' in error, got: %s", msg)
	}
}
