package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHelpers(t *testing.T) {
	// Step 1: CreateTestRepo — verify it returns a valid git repo.
	repoPath := CreateTestRepo(t)

	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		t.Fatalf("CreateTestRepo: .git directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("CreateTestRepo: .git is not a directory")
	}

	// Step 2: CreateTestBranch — verify the branch exists.
	branchName := "feature-test"
	CreateTestBranch(t, repoPath, branchName)

	cmd := exec.CommandContext(context.Background(), "git", "branch", "--list", branchName)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list failed: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatalf("CreateTestBranch: branch %q not found in repo", branchName)
	}

	// Step 3: CreateTestWorktree — verify directory exists and is on the correct branch.
	worktreePath := filepath.Join(t.TempDir(), "wt-feature-test")
	CreateTestWorktree(t, repoPath, worktreePath, branchName)

	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("CreateTestWorktree: worktree directory does not exist: %v", err)
	}

	cmd = exec.CommandContext(context.Background(), "git", "branch", "--show-current")
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current failed: %v", err)
	}
	currentBranch := strings.TrimSpace(string(out))
	if currentBranch != branchName {
		t.Fatalf("CreateTestWorktree: worktree on branch %q, want %q", currentBranch, branchName)
	}

	// Step 4: DirtyWorktree — verify git status --porcelain shows changes.
	DirtyWorktree(t, worktreePath)

	cmd = exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain failed: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatalf("DirtyWorktree: git status --porcelain returned no changes")
	}
	if !strings.Contains(string(out), "dirty.txt") {
		t.Errorf("DirtyWorktree: expected dirty.txt in status output, got: %s", out)
	}

	// Step 5: CommitFile — verify the commit exists in the log.
	commitFilename := "committed.txt"
	commitContent := "hello from test\n"
	CommitFile(t, repoPath, commitFilename, commitContent)

	cmd = exec.CommandContext(context.Background(), "git", "log", "--oneline", "--all")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("git log --oneline failed: %v", err)
	}
	if !strings.Contains(string(out), "add "+commitFilename) {
		t.Fatalf("CommitFile: commit for %q not found in log output:\n%s", commitFilename, out)
	}

	// Also verify the file content was written correctly.
	data, err := os.ReadFile(filepath.Join(repoPath, commitFilename))
	if err != nil {
		t.Fatalf("CommitFile: file %q not readable: %v", commitFilename, err)
	}
	if string(data) != commitContent {
		t.Errorf("CommitFile: file content = %q, want %q", data, commitContent)
	}
}
