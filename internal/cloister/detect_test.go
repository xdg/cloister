package cloister

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/container"
)

func TestDetectName(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Save and restore originals.
		origDetectGitRoot := detectGitRoot
		origProjectName := projectName
		t.Cleanup(func() {
			detectGitRoot = origDetectGitRoot
			projectName = origProjectName
		})

		detectGitRoot = func(path string) (string, error) {
			if path != "." {
				t.Errorf("expected path %q, got %q", ".", path)
			}
			return "/fake/git/root", nil
		}
		projectName = func(gitRoot string) (string, error) {
			if gitRoot != "/fake/git/root" {
				t.Errorf("expected gitRoot %q, got %q", "/fake/git/root", gitRoot)
			}
			return "myproject", nil
		}

		name, err := DetectName()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := container.GenerateCloisterName("myproject")
		if name != expected {
			t.Errorf("expected %q, got %q", expected, name)
		}
	})

	t.Run("git root detection fails", func(t *testing.T) {
		origDetectGitRoot := detectGitRoot
		t.Cleanup(func() { detectGitRoot = origDetectGitRoot })

		detectGitRoot = func(path string) (string, error) {
			return "", errors.New("not a git repo")
		}

		_, err := DetectName()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got := err.Error(); got != "detect git root: not a git repo" {
			t.Errorf("unexpected error message: %s", got)
		}
	})

	t.Run("project name detection fails", func(t *testing.T) {
		origDetectGitRoot := detectGitRoot
		origProjectName := projectName
		t.Cleanup(func() {
			detectGitRoot = origDetectGitRoot
			projectName = origProjectName
		})

		detectGitRoot = func(path string) (string, error) {
			return "/fake/root", nil
		}
		projectName = func(gitRoot string) (string, error) {
			return "", errors.New("no remote configured")
		}

		_, err := DetectName()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got := err.Error(); got != "determine project name: no remote configured" {
			t.Errorf("unexpected error message: %s", got)
		}
	})
}
