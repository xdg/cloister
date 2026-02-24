package worktree_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/testutil"
	"github.com/xdg/cloister/internal/worktree"
)

// currentBranch returns the current branch name for the repo at the given path.
func currentBranch(t *testing.T, path string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get current branch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestCreate(t *testing.T) {
	t.Run("creates branch and worktree when branch does not exist", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		wtPath := filepath.Join(t.TempDir(), "feature-wt")

		err := worktree.Create(repo, wtPath, "feature-new")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Verify the directory exists.
		info, err := os.Stat(wtPath)
		if err != nil {
			t.Fatalf("worktree dir not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("worktree path is not a directory")
		}

		// Verify the worktree is on the correct branch.
		branch := currentBranch(t, wtPath)
		if branch != "feature-new" {
			t.Errorf("got branch %q, want %q", branch, "feature-new")
		}
	})

	t.Run("succeeds when branch already exists", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		testutil.CreateTestBranch(t, repo, "existing-branch")
		wtPath := filepath.Join(t.TempDir(), "existing-wt")

		err := worktree.Create(repo, wtPath, "existing-branch")
		if err != nil {
			t.Fatalf("Create with existing branch: %v", err)
		}

		branch := currentBranch(t, wtPath)
		if branch != "existing-branch" {
			t.Errorf("got branch %q, want %q", branch, "existing-branch")
		}
	})
}

func TestIsDirty(t *testing.T) {
	t.Run("clean worktree returns false", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		testutil.CreateTestBranch(t, repo, "clean-branch")
		wtPath := filepath.Join(t.TempDir(), "clean-wt")

		err := worktree.Create(repo, wtPath, "clean-branch")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		dirty, err := worktree.IsDirty(wtPath)
		if err != nil {
			t.Fatalf("IsDirty: %v", err)
		}
		if dirty {
			t.Error("expected clean worktree to not be dirty")
		}
	})

	t.Run("dirty worktree returns true", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		testutil.CreateTestBranch(t, repo, "dirty-branch")
		wtPath := filepath.Join(t.TempDir(), "dirty-wt")

		err := worktree.Create(repo, wtPath, "dirty-branch")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		testutil.DirtyWorktree(t, wtPath)

		dirty, err := worktree.IsDirty(wtPath)
		if err != nil {
			t.Fatalf("IsDirty: %v", err)
		}
		if !dirty {
			t.Error("expected dirty worktree to be dirty")
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("refuses to remove dirty worktree without force", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		wtPath := filepath.Join(t.TempDir(), "dirty-remove-wt")

		err := worktree.Create(repo, wtPath, "dirty-remove")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		testutil.DirtyWorktree(t, wtPath)

		err = worktree.Remove(wtPath, false)
		if err == nil {
			t.Fatal("expected error removing dirty worktree without force")
		}
		if !strings.Contains(err.Error(), "uncommitted changes") {
			t.Errorf("error should mention uncommitted changes, got: %v", err)
		}
	})

	t.Run("removes dirty worktree with force", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		wtPath := filepath.Join(t.TempDir(), "force-remove-wt")

		err := worktree.Create(repo, wtPath, "force-remove")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		testutil.DirtyWorktree(t, wtPath)

		err = worktree.Remove(wtPath, true)
		if err != nil {
			t.Fatalf("Remove with force: %v", err)
		}

		// Verify the directory no longer exists.
		if _, serr := os.Stat(wtPath); !os.IsNotExist(serr) {
			t.Error("worktree directory should be removed")
		}
	})
}

func TestIsWorktree(t *testing.T) {
	t.Run("main checkout returns false", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)

		if worktree.IsWorktree(repo) {
			t.Error("main checkout should not be identified as a worktree")
		}
	})

	t.Run("worktree returns true", func(t *testing.T) {
		repo := testutil.CreateTestRepo(t)
		wtPath := filepath.Join(t.TempDir(), "is-wt")

		err := worktree.Create(repo, wtPath, "wt-check")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if !worktree.IsWorktree(wtPath) {
			t.Error("worktree should be identified as a worktree")
		}
	})

	t.Run("non-git directory returns false", func(t *testing.T) {
		dir := t.TempDir()
		if worktree.IsWorktree(dir) {
			t.Error("non-git directory should not be identified as a worktree")
		}
	})
}
