package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseDir_Default(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir, err := BaseDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(tmp, "cloister", "worktrees")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestBaseDir_CreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir, err := BaseDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("path is not a directory")
	}
}

func TestBaseDir_RespectsXDGDataHome(t *testing.T) {
	tmp := t.TempDir()
	customData := filepath.Join(tmp, "custom-data")
	t.Setenv("XDG_DATA_HOME", customData)

	dir, err := BaseDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(customData, "cloister", "worktrees")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestDir_ConstructsCorrectPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir, err := Dir("myproject", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(tmp, "cloister", "worktrees", "myproject", "main")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestDir_SanitizesBranchName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	tests := []struct {
		branch     string
		wantSuffix string
	}{
		{"feature/auth", "feature-auth"},
		{"feature/deep/nested", "feature-deep-nested"},
		{"UPPER-case", "upper-case"},
		{"special@chars!", "special-chars"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			dir, err := Dir("proj", tt.branch)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := filepath.Join(tmp, "cloister", "worktrees", "proj", tt.wantSuffix)
			if dir != want {
				t.Errorf("got %q, want %q", dir, want)
			}
		})
	}
}

func TestDir_DoesNotCreateDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir, err := Dir("myproject", "somebranch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = os.Stat(dir)
	if !os.IsNotExist(err) {
		t.Error("Dir should not create the project/branch directory")
	}
}
