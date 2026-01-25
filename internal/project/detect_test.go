package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectGitRoot_CurrentRepo(t *testing.T) {
	// This test runs in the cloister repo itself
	root, err := DetectGitRoot("")
	if err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Fatalf("DetectGitRoot failed: %v", err)
	}

	// Should return an absolute path
	if !filepath.IsAbs(root) {
		t.Errorf("expected absolute path, got %q", root)
	}

	// Should contain "cloister" in the path (the repo name)
	if !strings.Contains(root, "cloister") {
		t.Errorf("expected path to contain 'cloister', got %q", root)
	}

	// The .git directory should exist at the root
	gitDir := filepath.Join(root, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("expected .git directory at %q", gitDir)
	}
}

func TestDetectGitRoot_SubDirectory(t *testing.T) {
	// Get the current repo root first
	root, err := DetectGitRoot("")
	if err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Fatalf("DetectGitRoot failed: %v", err)
	}

	// Now try detecting from a subdirectory
	subDir := filepath.Join(root, "internal", "project")
	subRoot, err := DetectGitRoot(subDir)
	if err != nil {
		t.Fatalf("DetectGitRoot from subdirectory failed: %v", err)
	}

	if subRoot != root {
		t.Errorf("expected root %q from subdirectory, got %q", root, subRoot)
	}
}

func TestDetectGitRoot_NotGitRepo(t *testing.T) {
	// /tmp should not be a git repo
	tmpDir := os.TempDir()
	_, err := DetectGitRoot(tmpDir)

	if err == nil {
		t.Fatal("expected error for non-git directory")
	}

	if !errors.Is(err, ErrNotGitRepo) {
		// It might also fail if the temp dir itself is inside a git repo
		// (e.g., on some CI systems), so we check the error message
		if !strings.Contains(err.Error(), "not a git repository") {
			t.Errorf("expected ErrNotGitRepo, got: %v", err)
		}
	}
}

func TestDetectBranch_CurrentRepo(t *testing.T) {
	root, err := DetectGitRoot("")
	if err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Skip("not in a git repo")
	}

	branch, err := DetectBranch(root)
	if err != nil {
		t.Fatalf("DetectBranch failed: %v", err)
	}

	// Branch should be non-empty
	if branch == "" {
		t.Error("expected non-empty branch name")
	}

	// Branch should not contain newlines or spaces
	if strings.ContainsAny(branch, " \n\r\t") {
		t.Errorf("branch name contains whitespace: %q", branch)
	}
}

func TestDetectBranch_NotGitRepo(t *testing.T) {
	tmpDir := os.TempDir()
	_, err := DetectBranch(tmpDir)

	if err == nil {
		t.Fatal("expected error for non-git directory")
	}

	// Should return ErrNotGitRepo or a GitError with relevant message
	if !errors.Is(err, ErrNotGitRepo) {
		var gitErr *GitError
		if !errors.As(err, &gitErr) {
			// Might still be valid if temp is in a git repo on some systems
			if !strings.Contains(err.Error(), "not a git repository") {
				t.Errorf("expected git-related error, got: %v", err)
			}
		}
	}
}

func TestProjectName_CurrentRepo(t *testing.T) {
	root, err := DetectGitRoot("")
	if err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Skip("not in a git repo")
	}

	name, err := ProjectName(root)
	if err != nil {
		t.Fatalf("ProjectName failed: %v", err)
	}

	// Project name should be "cloister" (either from remote or directory)
	if name != "cloister" {
		t.Errorf("expected project name 'cloister', got %q", name)
	}
}

func TestExtractProjectName(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		want      string
	}{
		{
			name:      "SSH URL with .git",
			remoteURL: "git@github.com:xdg/cloister.git",
			want:      "cloister",
		},
		{
			name:      "SSH URL without .git",
			remoteURL: "git@github.com:xdg/cloister",
			want:      "cloister",
		},
		{
			name:      "HTTPS URL with .git",
			remoteURL: "https://github.com/xdg/cloister.git",
			want:      "cloister",
		},
		{
			name:      "HTTPS URL without .git",
			remoteURL: "https://github.com/xdg/cloister",
			want:      "cloister",
		},
		{
			name:      "SSH protocol URL",
			remoteURL: "ssh://git@github.com/xdg/cloister.git",
			want:      "cloister",
		},
		{
			name:      "GitLab URL",
			remoteURL: "git@gitlab.com:company/project.git",
			want:      "project",
		},
		{
			name:      "Bitbucket URL",
			remoteURL: "https://bitbucket.org/user/myrepo.git",
			want:      "myrepo",
		},
		{
			name:      "Nested path (monorepo style)",
			remoteURL: "https://github.com/org/team/project.git",
			want:      "project",
		},
		{
			name:      "Self-hosted GitLab",
			remoteURL: "git@git.company.com:team/service-api.git",
			want:      "service-api",
		},
		{
			name:      "Azure DevOps HTTPS",
			remoteURL: "https://dev.azure.com/org/project/_git/repo",
			want:      "repo",
		},
		{
			name:      "Empty URL",
			remoteURL: "",
			want:      "",
		},
		{
			name:      "Malformed URL",
			remoteURL: "not-a-url",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractProjectName(tt.remoteURL)
			if got != tt.want {
				t.Errorf("extractProjectName(%q) = %q, want %q", tt.remoteURL, got, tt.want)
			}
		})
	}
}

func TestGitError_Error(t *testing.T) {
	tests := []struct {
		name     string
		gitErr   GitError
		contains []string
	}{
		{
			name: "with stderr",
			gitErr: GitError{
				Command: "status",
				Args:    []string{"status"},
				Stderr:  "fatal: not a git repository",
				Err:     errors.New("exit status 128"),
			},
			contains: []string{"git status failed", "exit status 128", "stderr:", "not a git repository"},
		},
		{
			name: "without stderr",
			gitErr: GitError{
				Command: "fetch",
				Args:    []string{"fetch", "origin"},
				Stderr:  "",
				Err:     errors.New("exit status 1"),
			},
			contains: []string{"git fetch failed", "exit status 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.gitErr.Error()
			for _, s := range tt.contains {
				if !strings.Contains(msg, s) {
					t.Errorf("error message %q should contain %q", msg, s)
				}
			}
		})
	}
}

func TestGitError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	gitErr := &GitError{
		Command: "test",
		Err:     underlying,
	}

	if !errors.Is(gitErr, underlying) {
		t.Error("GitError should unwrap to underlying error")
	}
}

func TestProjectName_FallbackToDirectory(t *testing.T) {
	// Create a temporary git repo without a remote
	tmpDir, err := os.MkdirTemp("", "test-project-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	if _, err := runGit(tmpDir, "init"); err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Fatalf("git init failed: %v", err)
	}

	// Get project name (should fall back to directory name)
	name, err := ProjectName(tmpDir)
	if err != nil {
		t.Fatalf("ProjectName failed: %v", err)
	}

	// Should use the temp directory name (starts with "test-project-")
	if !strings.HasPrefix(name, "test-project-") {
		t.Errorf("expected project name starting with 'test-project-', got %q", name)
	}
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors can be detected with errors.Is
	t.Run("ErrNotGitRepo", func(t *testing.T) {
		wrappedErr := ErrNotGitRepo
		if !errors.Is(wrappedErr, ErrNotGitRepo) {
			t.Error("ErrNotGitRepo should be detectable with errors.Is")
		}
	})

	t.Run("ErrGitNotInstalled", func(t *testing.T) {
		wrappedErr := ErrGitNotInstalled
		if !errors.Is(wrappedErr, ErrGitNotInstalled) {
			t.Error("ErrGitNotInstalled should be detectable with errors.Is")
		}
	})
}
