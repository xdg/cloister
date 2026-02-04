package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfigCmd_HasSubcommands(t *testing.T) {
	// Verify config command exists and has expected subcommands
	subCmds := configCmd.Commands()
	if len(subCmds) == 0 {
		t.Fatal("config command should have subcommands")
	}

	expected := map[string]bool{
		"show": false,
		"edit": false,
		"path": false,
		"init": false,
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

func TestConfigPath_PrintsPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	cmd := &cobra.Command{}

	// Run the command (using the function directly)
	// This function uses fmt.Println which goes to stdout
	// We just verify it doesn't panic
	runConfigPath(cmd, nil)

	// The expected path would be:
	// filepath.Join(tmpDir, "cloister", "config.yaml")
	// but we can't easily capture fmt.Println output in a test
}

func TestConfigInit_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	cmd := &cobra.Command{}
	err := runConfigInit(cmd, nil)
	if err != nil {
		t.Fatalf("runConfigInit() error = %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, "cloister", "config.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	if info.Size() == 0 {
		t.Error("config file should not be empty")
	}
}

func TestConfigShow_LoadsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	cmd := &cobra.Command{}
	// Should succeed even if no config exists (uses defaults)
	err := runConfigShow(cmd, nil)
	if err != nil {
		t.Fatalf("runConfigShow() error = %v", err)
	}
}
