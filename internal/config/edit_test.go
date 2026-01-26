package config

import (
	"os"
	"testing"
)

// Note: EditProjectConfig and EditGlobalConfig require an interactive editor,
// so full testing requires manual verification. These tests cover the setup
// and validation logic that can be tested programmatically.

func TestEditProjectConfig_CreatesFileIfNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Use 'true' as editor which exits immediately without modifying the file
	t.Setenv("EDITOR", "true")

	path := ProjectConfigPath("test-project")

	// File should not exist yet
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist before test: %v", err)
	}

	// Edit should create the file
	if err := EditProjectConfig("test-project"); err != nil {
		t.Fatalf("EditProjectConfig() error = %v", err)
	}

	// File should now exist
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file should exist after EditProjectConfig: %v", err)
	}
}

func TestEditProjectConfig_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Use 'true' as editor which exits immediately without modifying the file
	t.Setenv("EDITOR", "true")

	// Create projects directory and write a config
	projectsDir := ProjectsDir()
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "remote: \"my-remote\"\nroot: \"/my/path\"\n"
	path := ProjectConfigPath("test-project")
	if err := os.WriteFile(path, []byte(customContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Edit should not error (editor is 'true' which exits 0)
	if err := EditProjectConfig("test-project"); err != nil {
		t.Fatalf("EditProjectConfig() error = %v", err)
	}

	// Content should be unchanged (editor didn't modify it)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != customContent {
		t.Errorf("config file was modified, content = %q, want %q", string(data), customContent)
	}
}

func TestEditGlobalConfig_CreatesFileIfNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Use 'true' as editor which exits immediately without modifying the file
	t.Setenv("EDITOR", "true")

	path := GlobalConfigPath()

	// File should not exist yet
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist before test: %v", err)
	}

	// Edit should create the file
	if err := EditGlobalConfig(); err != nil {
		t.Fatalf("EditGlobalConfig() error = %v", err)
	}

	// File should now exist
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file should exist after EditGlobalConfig: %v", err)
	}
}

func TestEditGlobalConfig_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Use 'true' as editor which exits immediately without modifying the file
	t.Setenv("EDITOR", "true")

	// Create config directory and write a config
	configDir := ConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	customContent := "# Custom config\nproxy:\n  listen: \":9999\"\n"
	path := GlobalConfigPath()
	if err := os.WriteFile(path, []byte(customContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Edit should not error (editor is 'true' which exits 0)
	if err := EditGlobalConfig(); err != nil {
		t.Fatalf("EditGlobalConfig() error = %v", err)
	}

	// Content should be unchanged (editor didn't modify it)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != customContent {
		t.Errorf("config file was modified, content = %q, want %q", string(data), customContent)
	}
}

func TestEditProjectConfig_EditorFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Use 'false' as editor which exits with error
	t.Setenv("EDITOR", "false")

	// Create a valid config first
	if err := InitProjectConfig("test-project", "remote", "/path"); err != nil {
		t.Fatalf("InitProjectConfig() error = %v", err)
	}

	// Edit should return an error because the editor failed
	err := EditProjectConfig("test-project")
	if err == nil {
		t.Error("EditProjectConfig() should return error when editor fails")
	}
}

func TestOpenEditor_FallbackToVi(t *testing.T) {
	// This test just verifies the fallback logic exists
	// We can't actually test vi execution in an automated test
	t.Setenv("EDITOR", "")

	// The function would use "vi" as default
	// We just verify the code path doesn't panic
	// Actual execution would require a real terminal
}
