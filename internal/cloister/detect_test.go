package cloister

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/container"
)

// saveDetectVars saves all package-level detection variables and returns a
// cleanup function that restores them. Call at the top of each subtest.
func saveDetectVars(t *testing.T) {
	t.Helper()
	origDetectGitRoot := detectGitRoot
	origProjectName := projectName
	origLoadCloisterRegistry := loadCloisterRegistry
	origGetWorkingDir := getWorkingDir
	t.Cleanup(func() {
		detectGitRoot = origDetectGitRoot
		projectName = origProjectName
		loadCloisterRegistry = origLoadCloisterRegistry
		getWorkingDir = origGetWorkingDir
	})
}

// mockNoRegistryMatch configures the registry loader to return an empty
// registry so registry-based detection always falls through to project-based.
func mockNoRegistryMatch() {
	loadCloisterRegistry = func() (*Registry, error) {
		return &Registry{}, nil
	}
	getWorkingDir = func() (string, error) {
		return "/unrelated/path", nil
	}
}

func TestDetectName(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saveDetectVars(t)
		mockNoRegistryMatch()

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
		saveDetectVars(t)
		mockNoRegistryMatch()

		detectGitRoot = func(_ string) (string, error) {
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
		saveDetectVars(t)
		mockNoRegistryMatch()

		detectGitRoot = func(_ string) (string, error) {
			return "/fake/root", nil
		}
		projectName = func(_ string) (string, error) {
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

	t.Run("registry_worktree_match", func(t *testing.T) {
		saveDetectVars(t)

		loadCloisterRegistry = func() (*Registry, error) {
			reg := &Registry{
				Cloisters: []RegistryEntry{
					{
						CloisterName: "myproject-feature",
						ProjectName:  "myproject",
						Branch:       "feature",
						HostPath:     "/fake/worktree/path",
						IsWorktree:   true,
					},
				},
			}
			return reg, nil
		}
		getWorkingDir = func() (string, error) {
			return "/fake/worktree/path/subdir", nil
		}

		name, err := DetectName()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "myproject-feature" {
			t.Errorf("expected %q, got %q", "myproject-feature", name)
		}
	})

	t.Run("registry_main_match", func(t *testing.T) {
		saveDetectVars(t)

		loadCloisterRegistry = func() (*Registry, error) {
			reg := &Registry{
				Cloisters: []RegistryEntry{
					{
						CloisterName: "cloister-myproject",
						ProjectName:  "myproject",
						HostPath:     "/fake/main/checkout",
						IsWorktree:   false,
					},
				},
			}
			return reg, nil
		}
		getWorkingDir = func() (string, error) {
			return "/fake/main/checkout", nil
		}

		name, err := DetectName()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "cloister-myproject" {
			t.Errorf("expected %q, got %q", "cloister-myproject", name)
		}
	})

	t.Run("registry_no_match_falls_back", func(t *testing.T) {
		saveDetectVars(t)

		loadCloisterRegistry = func() (*Registry, error) {
			reg := &Registry{
				Cloisters: []RegistryEntry{
					{
						CloisterName: "cloister-other",
						ProjectName:  "other",
						HostPath:     "/some/other/path",
					},
				},
			}
			return reg, nil
		}
		getWorkingDir = func() (string, error) {
			return "/fake/git/root/subdir", nil
		}
		detectGitRoot = func(_ string) (string, error) {
			return "/fake/git/root", nil
		}
		projectName = func(_ string) (string, error) {
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

	t.Run("registry_load_error_falls_back", func(t *testing.T) {
		saveDetectVars(t)

		loadCloisterRegistry = func() (*Registry, error) {
			return nil, errors.New("disk error")
		}
		getWorkingDir = func() (string, error) {
			return "/fake/git/root", nil
		}
		detectGitRoot = func(_ string) (string, error) {
			return "/fake/git/root", nil
		}
		projectName = func(_ string) (string, error) {
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
}
