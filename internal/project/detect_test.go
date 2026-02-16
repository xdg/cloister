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

	// The .git directory should exist at the root
	gitDir := filepath.Join(root, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("expected .git directory at %q", gitDir)
	}

	// Verify the detected root is actually a git repository by checking
	// that we can successfully run git commands in it
	if _, err := runGit(root, "rev-parse", "--git-dir"); err != nil {
		t.Errorf("detected root %q is not a valid git repository: %v", root, err)
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

	name, err := Name(root)
	if err != nil {
		t.Fatalf("Name failed: %v", err)
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
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize git repo
	if _, err := runGit(tmpDir, "init"); err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Fatalf("git init failed: %v", err)
	}

	// Get project name (should fall back to directory name)
	name, err := Name(tmpDir)
	if err != nil {
		t.Fatalf("Name failed: %v", err)
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

func TestDetectProject_Valid(t *testing.T) {
	// This test runs in the cloister repo itself
	info, err := DetectProject("")
	if err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Fatalf("DetectProject failed: %v", err)
	}

	// Should return the project info
	if info == nil {
		t.Fatal("expected non-nil Info")
	}

	// Name should be "cloister"
	if info.Name != "cloister" {
		t.Errorf("expected project name 'cloister', got %q", info.Name)
	}

	// Root should be an absolute path
	if !filepath.IsAbs(info.Root) {
		t.Errorf("expected absolute path for Root, got %q", info.Root)
	}

	// Verify the detected root is a valid git repository
	gitDir := filepath.Join(info.Root, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("expected .git directory at %q", gitDir)
	}

	// Branch should be non-empty
	if info.Branch == "" {
		t.Error("expected non-empty Branch")
	}

	// Remote should contain "cloister" (the cloister repo has a remote)
	if !strings.Contains(info.Remote, "cloister") {
		t.Errorf("expected Remote to contain 'cloister', got %q", info.Remote)
	}
}

func TestDetectProject_Subdirectory(t *testing.T) {
	// Get the current repo root first
	root, err := DetectGitRoot("")
	if err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Skip("not in a git repo")
	}

	// Detect from a subdirectory
	subDir := filepath.Join(root, "internal", "project")
	info, err := DetectProject(subDir)
	if err != nil {
		t.Fatalf("DetectProject from subdirectory failed: %v", err)
	}

	// Should return the same root
	if info.Root != root {
		t.Errorf("expected Root %q from subdirectory, got %q", root, info.Root)
	}

	// Name should still be "cloister"
	if info.Name != "cloister" {
		t.Errorf("expected project name 'cloister', got %q", info.Name)
	}
}

func TestDetectProject_NoRemote(t *testing.T) {
	// Create a temporary git repo without a remote
	tmpDir, err := os.MkdirTemp("", "test-no-remote-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize git repo
	if _, err := runGit(tmpDir, "init"); err != nil {
		if errors.Is(err, ErrGitNotInstalled) {
			t.Skip("git not installed")
		}
		t.Fatalf("git init failed: %v", err)
	}

	// Configure user for commit
	if _, err := runGit(tmpDir, "config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}
	if _, err := runGit(tmpDir, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}

	// Create an initial commit so we have a branch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if _, err := runGit(tmpDir, "add", "test.txt"); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if _, err := runGit(tmpDir, "commit", "-m", "initial commit"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Detect project
	info, err := DetectProject(tmpDir)
	if err != nil {
		t.Fatalf("DetectProject failed: %v", err)
	}

	// Remote should be empty
	if info.Remote != "" {
		t.Errorf("expected empty Remote, got %q", info.Remote)
	}

	// Name should be the directory name (starts with "test-no-remote-")
	if !strings.HasPrefix(info.Name, "test-no-remote-") {
		t.Errorf("expected project name starting with 'test-no-remote-', got %q", info.Name)
	}

	// Branch should be non-empty (probably "master" or "main")
	if info.Branch == "" {
		t.Error("expected non-empty Branch")
	}
}

func TestDetectProject_NotGitRepo(t *testing.T) {
	// Create a temporary directory that is not a git repo
	tmpDir, err := os.MkdirTemp("", "test-not-git-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	_, err = DetectProject(tmpDir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}

	if !errors.Is(err, ErrNotGitRepo) {
		t.Errorf("expected ErrNotGitRepo, got: %v", err)
	}
}

func TestGetRemoteURL(t *testing.T) {
	t.Run("with remote", func(t *testing.T) {
		root, err := DetectGitRoot("")
		if err != nil {
			if errors.Is(err, ErrGitNotInstalled) {
				t.Skip("git not installed")
			}
			t.Skip("not in a git repo")
		}

		remote := GetRemoteURL(root)
		// The cloister repo should have a remote
		if remote == "" {
			t.Error("expected non-empty remote URL for cloister repo")
		}
		if !strings.Contains(remote, "cloister") {
			t.Errorf("expected remote to contain 'cloister', got %q", remote)
		}
	})

	t.Run("without remote", func(t *testing.T) {
		// Create a temporary git repo without a remote
		tmpDir, err := os.MkdirTemp("", "test-remote-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Initialize git repo
		if _, err := runGit(tmpDir, "init"); err != nil {
			if errors.Is(err, ErrGitNotInstalled) {
				t.Skip("git not installed")
			}
			t.Fatalf("git init failed: %v", err)
		}

		remote := GetRemoteURL(tmpDir)
		if remote != "" {
			t.Errorf("expected empty remote, got %q", remote)
		}
	})

	t.Run("not a git repo", func(t *testing.T) {
		// Create a temporary directory that is not a git repo
		tmpDir, err := os.MkdirTemp("", "test-not-repo-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		remote := GetRemoteURL(tmpDir)
		// Should return empty string (not an error)
		if remote != "" {
			t.Errorf("expected empty remote for non-git dir, got %q", remote)
		}
	})
}
