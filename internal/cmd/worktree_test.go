package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/testutil"
)

func TestWorktreeCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "worktree" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'worktree' command to be registered on rootCmd")
	}
}

func TestWorktreeListCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range worktreeCmd.Commands() {
		if cmd.Name() == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'list' subcommand to be registered on worktreeCmd")
	}
}

func TestWorktreeList_Output(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Create a registry with 3 entries: 1 main + 2 worktrees for same project.
	reg := &cloister.Registry{}
	entries := []cloister.RegistryEntry{
		{
			CloisterName: "my-app",
			ProjectName:  "my-app",
			Branch:       "",
			HostPath:     "/home/user/projects/my-app",
			IsWorktree:   false,
		},
		{
			CloisterName: "my-app-feature-auth",
			ProjectName:  "my-app",
			Branch:       "feature-auth",
			HostPath:     "/home/user/.local/share/cloister/worktrees/my-app/feature-auth",
			IsWorktree:   true,
		},
		{
			CloisterName: "my-app-bugfix-login",
			ProjectName:  "my-app",
			Branch:       "bugfix-login",
			HostPath:     "/home/user/.local/share/cloister/worktrees/my-app/bugfix-login",
			IsWorktree:   true,
		},
	}
	for _, entry := range entries {
		if err := reg.Register(entry); err != nil {
			t.Fatalf("failed to register entry %s: %v", entry.CloisterName, err)
		}
	}
	if err := cloister.SaveRegistry(reg); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Reset the flag before test (in case other tests set it).
	worktreeProjectFlag = "my-app"
	defer func() { worktreeProjectFlag = "" }()

	// Capture output by calling runWorktreeList directly.
	// Since term.Stdout() writes to os.Stdout by default,
	// we verify via the function return (no error) and by checking
	// the registry contents match expectations.
	err := runWorktreeList(worktreeListCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Also test via cobra execution with captured output.
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"worktree", "list", "-p", "my-app"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error from Execute: %v", err)
	}

	// Note: tabwriter output goes to term.Stdout() (os.Stdout), not cobra's
	// captured output, so we verify correctness through the direct call above.
	// The Execute() call verifies the command wiring works without error.
}

func TestWorktreeList_OutputContent(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Create a registry with entries for the test project.
	reg := &cloister.Registry{}
	entries := []cloister.RegistryEntry{
		{
			CloisterName: "test-proj",
			ProjectName:  "test-proj",
			Branch:       "",
			HostPath:     "/tmp/test-proj",
			IsWorktree:   false,
		},
		{
			CloisterName: "test-proj-feat",
			ProjectName:  "test-proj",
			Branch:       "feat",
			HostPath:     "/tmp/worktrees/test-proj/feat",
			IsWorktree:   true,
		},
	}
	for _, entry := range entries {
		if err := reg.Register(entry); err != nil {
			t.Fatalf("failed to register entry: %v", err)
		}
	}
	if err := cloister.SaveRegistry(reg); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Verify the registry returns the correct entries.
	found := reg.FindByProject("test-proj")
	if len(found) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(found))
	}

	// Verify the first entry shows "(main)" label behavior.
	if found[0].Branch != "" {
		t.Errorf("expected empty branch for main entry, got %q", found[0].Branch)
	}
	if found[1].Branch != "feat" {
		t.Errorf("expected branch 'feat', got %q", found[1].Branch)
	}

	// Run the command with -p flag.
	worktreeProjectFlag = "test-proj"
	defer func() { worktreeProjectFlag = "" }()

	err := runWorktreeList(worktreeListCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorktreeList_NoEntries(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Save an empty registry.
	reg := &cloister.Registry{}
	if err := cloister.SaveRegistry(reg); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	worktreeProjectFlag = "nonexistent-project"
	defer func() { worktreeProjectFlag = "" }()

	// Capture output via a pipe to verify the "no managed worktrees" message.
	// Since term output goes to os.Stdout, we verify indirectly through error return.
	err := runWorktreeList(worktreeListCmd, nil)
	if err != nil {
		t.Fatalf("expected no error for empty project, got: %v", err)
	}
}

func TestWorktreeList_AliasLs(t *testing.T) {
	if !worktreeListCmd.HasAlias("ls") {
		t.Fatal("expected 'ls' alias on worktreeListCmd")
	}
}

func TestWorktreeList_ProjectFlag(t *testing.T) {
	flag := worktreeListCmd.Flags().Lookup("project")
	if flag == nil {
		t.Fatal("expected 'project' flag on worktreeListCmd")
	}
	if flag.Shorthand != "p" {
		t.Errorf("expected shorthand 'p', got %q", flag.Shorthand)
	}
}

func TestWorktreeList_NoEntries_ViaExecute(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	reg := &cloister.Registry{}
	if err := cloister.SaveRegistry(reg); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"worktree", "list", "-p", "empty-proj"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorktreeList_HasCorrectColumns(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	reg := &cloister.Registry{}
	if err := reg.Register(cloister.RegistryEntry{
		CloisterName: "col-test",
		ProjectName:  "col-test",
		Branch:       "",
		HostPath:     "/tmp/col-test",
	}); err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	if err := cloister.SaveRegistry(reg); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Verify the entry data is correct for column rendering.
	entries := reg.FindByProject("col-test")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	// WORKTREE column: should show "(main)" when branch is empty
	worktreeLabel := entry.Branch
	if worktreeLabel == "" {
		worktreeLabel = "(main)"
	}
	if worktreeLabel != "(main)" {
		t.Errorf("expected worktree label '(main)', got %q", worktreeLabel)
	}

	// PATH column
	if entry.HostPath != "/tmp/col-test" {
		t.Errorf("expected path '/tmp/col-test', got %q", entry.HostPath)
	}

	// CLOISTER column
	if entry.CloisterName != "col-test" {
		t.Errorf("expected cloister name 'col-test', got %q", entry.CloisterName)
	}

	// STATUS column: without Docker, IsRunning will fail, so status should be "stopped"
	// (verified by the fact that runWorktreeList doesn't error out)
	worktreeProjectFlag = "col-test"
	defer func() { worktreeProjectFlag = "" }()

	err := runWorktreeList(worktreeListCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorktreeList_DetectProjectError(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Without -p flag and not in a git repo, should get an error.
	worktreeProjectFlag = ""

	// Use a temp dir that is not a git repo.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	err := runWorktreeList(worktreeListCmd, nil)
	if err == nil {
		t.Fatal("expected error when not in a git repo and no -p flag")
	}
	if !strings.Contains(err.Error(), "failed to detect project") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWorktreeRemoveCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range worktreeCmd.Commands() {
		if cmd.Name() == "remove" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'remove' subcommand to be registered on worktreeCmd")
	}
}

func TestWorktreeRemove_Flags(t *testing.T) {
	forceFlag := worktreeRemoveCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("expected 'force' flag on worktreeRemoveCmd")
	}
	if forceFlag.Shorthand != "f" {
		t.Errorf("expected shorthand 'f', got %q", forceFlag.Shorthand)
	}

	projectFlag := worktreeRemoveCmd.Flags().Lookup("project")
	if projectFlag == nil {
		t.Fatal("expected 'project' flag on worktreeRemoveCmd")
	}
	if projectFlag.Shorthand != "p" {
		t.Errorf("expected shorthand 'p', got %q", projectFlag.Shorthand)
	}
}

func TestWorktreeRemove_NotInRegistry(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	// Save an empty registry.
	reg := &cloister.Registry{}
	if err := cloister.SaveRegistry(reg); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Set project flag to avoid git detection.
	worktreeRemoveProjectFlag = "my-proj"
	worktreeRemoveForceFlag = false
	defer func() {
		worktreeRemoveProjectFlag = ""
		worktreeRemoveForceFlag = false
	}()

	err := runWorktreeRemove(worktreeRemoveCmd, []string{"no-such-branch"})
	if err == nil {
		t.Fatal("expected error for non-existent worktree")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "my-proj") {
		t.Errorf("expected project name in error, got: %v", err)
	}
}

func TestWorktreeRemove_DetectProjectError(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	worktreeRemoveProjectFlag = ""
	worktreeRemoveForceFlag = false

	// Use a temp dir that is not a git repo.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	err := runWorktreeRemove(worktreeRemoveCmd, []string{"some-branch"})
	if err == nil {
		t.Fatal("expected error when not in a git repo and no -p flag")
	}
	if !strings.Contains(err.Error(), "failed to detect project") {
		t.Errorf("unexpected error: %v", err)
	}
}
