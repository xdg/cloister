package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitRun executes a git command in the given directory, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// CreateTestRepo creates a bare-minimum git repo in t.TempDir() with an
// initial commit. Returns the repo path. The repo has git user.name and
// user.email configured so commits work in CI environments without global
// git config.
func CreateTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.name", "Test User")
	gitRun(t, dir, "config", "user.email", "test@example.com")

	readme := filepath.Join(dir, "README")
	if err := os.WriteFile(readme, []byte("test repo\n"), 0o640); err != nil {
		t.Fatalf("write README: %v", err)
	}

	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial commit")

	return dir
}

// CreateTestBranch creates a branch in the given repo without checking it out.
// Useful for testing "branch already exists" vs "branch needs creation" paths.
func CreateTestBranch(t *testing.T, repoPath, branchName string) {
	t.Helper()
	gitRun(t, repoPath, "branch", branchName)
}

// CreateTestWorktree creates a git worktree at worktreePath for the given
// branch. The branch must already exist (use CreateTestBranch first).
func CreateTestWorktree(t *testing.T, repoPath, worktreePath, branch string) {
	t.Helper()
	gitRun(t, repoPath, "worktree", "add", worktreePath, branch)
}

// CommitFile writes a file with the given content to the repo, stages it, and
// commits it. Useful for advancing history in tests.
func CommitFile(t *testing.T, repoPath, filename, content string) {
	t.Helper()

	filePath := filepath.Join(repoPath, filename)

	if dir := filepath.Dir(filePath); dir != repoPath {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir parents for %s: %v", filename, err)
		}
	}

	if err := os.WriteFile(filePath, []byte(content), 0o640); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}

	gitRun(t, repoPath, "add", filename)
	gitRun(t, repoPath, "commit", "-m", "add "+filename)
}

// DirtyWorktree creates an uncommitted, untracked file in the given worktree
// directory. This is useful for testing dirty-check refusal logic where
// `git status --porcelain` should report untracked changes.
func DirtyWorktree(t *testing.T, worktreePath string) {
	t.Helper()

	dirty := filepath.Join(worktreePath, "dirty.txt")
	if err := os.WriteFile(dirty, []byte("uncommitted changes\n"), 0o640); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
}
