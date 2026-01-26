package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/project"
)

func TestProjectCmd_HasSubcommands(t *testing.T) {
	// Verify project command exists and has expected subcommands
	subCmds := projectCmd.Commands()
	if len(subCmds) == 0 {
		t.Fatal("project command should have subcommands")
	}

	expected := map[string]bool{
		"list":   false,
		"show":   false,
		"edit":   false,
		"remove": false,
	}

	for _, cmd := range subCmds {
		if _, ok := expected[cmd.Name()]; ok {
			expected[cmd.Name()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing subcommand: %s", name)
		}
	}
}

func TestProjectList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cmd := &cobra.Command{}
	err := runProjectList(cmd, nil)
	if err != nil {
		t.Fatalf("runProjectList() error = %v", err)
	}
	// Should succeed with "No registered projects." message
}

func TestProjectList_WithProjects(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create registry with test data
	reg := &project.Registry{
		Projects: []project.RegistryEntry{
			{
				Name:     "test-project",
				Root:     "/home/user/repos/test-project",
				Remote:   "git@github.com:user/test-project.git",
				LastUsed: time.Now(),
			},
		},
	}
	if err := project.SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	cmd := &cobra.Command{}
	err := runProjectList(cmd, nil)
	if err != nil {
		t.Fatalf("runProjectList() error = %v", err)
	}
}

func TestProjectShow_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cmd := &cobra.Command{}
	err := runProjectShow(cmd, []string{"non-existent"})
	if err == nil {
		t.Fatal("expected error for non-existent project")
	}
}

func TestProjectShow_Found(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create registry with test data
	reg := &project.Registry{
		Projects: []project.RegistryEntry{
			{
				Name:     "test-project",
				Root:     "/home/user/repos/test-project",
				Remote:   "git@github.com:user/test-project.git",
				LastUsed: time.Now(),
			},
		},
	}
	if err := project.SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	cmd := &cobra.Command{}
	err := runProjectShow(cmd, []string{"test-project"})
	if err != nil {
		t.Fatalf("runProjectShow() error = %v", err)
	}
}

func TestProjectRemove_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cmd := &cobra.Command{}
	err := runProjectRemove(cmd, []string{"non-existent"})
	if err == nil {
		t.Fatal("expected error for non-existent project")
	}
}

func TestProjectRemove_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create registry with test data
	reg := &project.Registry{
		Projects: []project.RegistryEntry{
			{
				Name:     "to-remove",
				Root:     "/home/user/repos/to-remove",
				Remote:   "git@github.com:user/to-remove.git",
				LastUsed: time.Now(),
			},
		},
	}
	if err := project.SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Reset flag to default
	projectRemoveConfig = false

	cmd := &cobra.Command{}
	err := runProjectRemove(cmd, []string{"to-remove"})
	if err != nil {
		t.Fatalf("runProjectRemove() error = %v", err)
	}

	// Verify project was removed
	loadedReg, err := project.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	if loadedReg.FindByName("to-remove") != nil {
		t.Error("project should have been removed")
	}
}

func TestProjectRemove_WithConfigFlag(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create registry with test data
	reg := &project.Registry{
		Projects: []project.RegistryEntry{
			{
				Name:     "to-remove",
				Root:     "/home/user/repos/to-remove",
				Remote:   "git@github.com:user/to-remove.git",
				LastUsed: time.Now(),
			},
		},
	}
	if err := project.SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Create a project config file
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}
	configPath := filepath.Join(projectsDir, "to-remove.yaml")
	if err := os.WriteFile(configPath, []byte("remote: test"), 0600); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Set flag to remove config
	projectRemoveConfig = true
	defer func() { projectRemoveConfig = false }()

	cmd := &cobra.Command{}
	err := runProjectRemove(cmd, []string{"to-remove"})
	if err != nil {
		t.Fatalf("runProjectRemove() error = %v", err)
	}

	// Verify project was removed from registry
	loadedReg, err := project.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	if loadedReg.FindByName("to-remove") != nil {
		t.Error("project should have been removed from registry")
	}

	// Verify config file was deleted
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("config file should have been deleted")
	}
}

func TestProjectRemoveCmd_HasConfigFlag(t *testing.T) {
	flag := projectRemoveCmd.Flags().Lookup("config")
	if flag == nil {
		t.Fatal("remove command should have --config flag")
	}

	if flag.DefValue != "false" {
		t.Errorf("--config flag default should be false, got %q", flag.DefValue)
	}
}

func TestProjectListCmd_HasAlias(t *testing.T) {
	aliases := projectListCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "ls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("list command should have 'ls' alias")
	}
}

func TestProjectRemoveCmd_HasAlias(t *testing.T) {
	aliases := projectRemoveCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "rm" {
			found = true
			break
		}
	}
	if !found {
		t.Error("remove command should have 'rm' alias")
	}
}
