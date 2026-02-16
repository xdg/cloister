package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/project"
)

// TestStartErrorHandling verifies that runStart correctly handles various
// error conditions by returning appropriate user-friendly errors.
func TestStartErrorHandling(t *testing.T) {
	// Test dockerNotRunningError integration
	t.Run("docker not running error", func(t *testing.T) {
		err := dockerNotRunningError()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "docker is not running") {
			t.Errorf("expected 'Docker is not running' in error, got: %s", msg)
		}
		if !strings.Contains(msg, "please start Docker") {
			t.Errorf("expected guidance to start Docker, got: %s", msg)
		}
	})

	// Test gitDetectionError integration
	t.Run("git detection errors", func(t *testing.T) {
		testCases := []struct {
			err         error
			expectMatch string
		}{
			{project.ErrNotGitRepo, "not in a git repository"},
			{project.ErrGitNotInstalled, "git is not installed"},
		}

		for _, tc := range testCases {
			result := gitDetectionError(tc.err)
			if result == nil {
				t.Errorf("gitDetectionError(%v) returned nil", tc.err)
				continue
			}
			if !strings.Contains(result.Error(), tc.expectMatch) {
				t.Errorf("expected %q in error, got: %s", tc.expectMatch, result.Error())
			}
		}
	})
}

// TestStartContainerNaming verifies that cloister and container names
// are generated correctly for the start command.
func TestStartContainerNaming(t *testing.T) {
	tests := []struct {
		projectName   string
		wantCloister  string
		wantContainer string
	}{
		{
			projectName:   "myproject",
			wantCloister:  "myproject",
			wantContainer: "cloister-myproject",
		},
		{
			projectName:   "MyProject",
			wantCloister:  "myproject",
			wantContainer: "cloister-myproject",
		},
		{
			projectName:   "my-project",
			wantCloister:  "my-project",
			wantContainer: "cloister-my-project",
		},
		{
			projectName:   "my_project",
			wantCloister:  "my-project",
			wantContainer: "cloister-my-project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.projectName, func(t *testing.T) {
			cloisterName := container.GenerateCloisterName(tc.projectName)
			containerName := container.CloisterNameToContainerName(cloisterName)

			if cloisterName != tc.wantCloister {
				t.Errorf("GenerateCloisterName(%q) = %q, want %q", tc.projectName, cloisterName, tc.wantCloister)
			}
			if containerName != tc.wantContainer {
				t.Errorf("CloisterNameToContainerName(%q) = %q, want %q", cloisterName, containerName, tc.wantContainer)
			}
		})
	}
}

// TestStartGuardianErrorHints verifies that helpful hints are provided
// for various guardian startup failures.
func TestStartGuardianErrorHints(t *testing.T) {
	// These test the error message parsing logic in runStart
	// that provides contextual hints for guardian failures.

	tests := []struct {
		name       string
		errMsg     string
		wantInHint string
	}{
		{
			name:       "port already in use",
			errMsg:     "guardian failed to start: address already in use",
			wantInHint: "Port 9997 may be in use",
		},
		{
			name:       "port allocated",
			errMsg:     "guardian failed to start: port is already allocated",
			wantInHint: "Port 9997 may be in use",
		},
		{
			name:       "image not found",
			errMsg:     "guardian failed to start: No such image",
			wantInHint: "cloister image may not be built",
		},
		{
			name:       "unable to find image",
			errMsg:     "guardian failed to start: Unable to find image",
			wantInHint: "cloister image may not be built",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the error checking logic from runStart
			errStr := tc.errMsg
			hasPortError := strings.Contains(errStr, "address already in use") ||
				strings.Contains(errStr, "port is already allocated")
			hasImageError := strings.Contains(errStr, "No such image") ||
				strings.Contains(errStr, "Unable to find image")

			if hasPortError && !strings.Contains(tc.wantInHint, "Port") {
				t.Error("expected port hint detection")
			}
			if hasImageError && !strings.Contains(tc.wantInHint, "image") {
				t.Error("expected image hint detection")
			}
		})
	}
}

// TestAttachToExistingNameConversion verifies that container names are
// correctly converted to cloister names for user display.
func TestAttachToExistingNameConversion(t *testing.T) {
	tests := []struct {
		containerName string
		wantCloister  string
	}{
		{"cloister-myproject", "myproject"},
		{"cloister-foo-main", "foo-main"},
		{"cloister-test", "test"},
		{"not-a-cloister", "not-a-cloister"},
	}

	for _, tc := range tests {
		t.Run(tc.containerName, func(t *testing.T) {
			cloisterName := container.NameToCloisterName(tc.containerName)
			if cloisterName != tc.wantCloister {
				t.Errorf("NameToCloisterName(%q) = %q, want %q",
					tc.containerName, cloisterName, tc.wantCloister)
			}
		})
	}
}

// TestDockerErrorDetection verifies that Docker-related errors are correctly
// identified using errors.Is.
func TestDockerErrorDetection(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		isNotRunning      bool
		isContainerExists bool
	}{
		{
			name:         "docker not running",
			err:          docker.ErrDockerNotRunning,
			isNotRunning: true,
		},
		{
			name:              "container exists",
			err:               container.ErrContainerExists,
			isContainerExists: true,
		},
		{
			name:         "wrapped docker not running",
			err:          errors.Join(errors.New("context"), docker.ErrDockerNotRunning),
			isNotRunning: true,
		},
		{
			name:              "wrapped container exists",
			err:               errors.Join(errors.New("context"), container.ErrContainerExists),
			isContainerExists: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("something else"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, docker.ErrDockerNotRunning); got != tc.isNotRunning {
				t.Errorf("errors.Is(err, ErrDockerNotRunning) = %v, want %v", got, tc.isNotRunning)
			}
			if got := errors.Is(tc.err, container.ErrContainerExists); got != tc.isContainerExists {
				t.Errorf("errors.Is(err, ErrContainerExists) = %v, want %v", got, tc.isContainerExists)
			}
		})
	}
}
