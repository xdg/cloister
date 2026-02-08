package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecisionDir_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	dir := DecisionDir()

	want := "/test/config/cloister/decisions"
	if dir != want {
		t.Errorf("DecisionDir() = %q, want %q", dir, want)
	}
}

func TestGlobalDecisionPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := GlobalDecisionPath()

	want := "/test/config/cloister/decisions/global.yaml"
	if path != want {
		t.Errorf("GlobalDecisionPath() = %q, want %q", path, want)
	}
}

func TestProjectDecisionPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := ProjectDecisionPath("my-api")

	want := "/test/config/cloister/decisions/projects/my-api.yaml"
	if path != want {
		t.Errorf("ProjectDecisionPath() = %q, want %q", path, want)
	}
}

func TestLoadGlobalApprovals_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	decisions, err := LoadGlobalApprovals()
	if err != nil {
		t.Fatalf("LoadGlobalApprovals() error = %v", err)
	}

	if decisions == nil {
		t.Fatal("LoadGlobalApprovals() returned nil")
	}
	if len(decisions.Domains) != 0 {
		t.Errorf("decisions.Domains = %v, want empty", decisions.Domains)
	}
	if len(decisions.Patterns) != 0 {
		t.Errorf("decisions.Patterns = %v, want empty", decisions.Patterns)
	}
}

func TestLoadProjectApprovals_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	decisions, err := LoadProjectApprovals("nonexistent")
	if err != nil {
		t.Fatalf("LoadProjectApprovals() error = %v", err)
	}

	if decisions == nil {
		t.Fatal("LoadProjectApprovals() returned nil")
	}
	if len(decisions.Domains) != 0 {
		t.Errorf("decisions.Domains = %v, want empty", decisions.Domains)
	}
	if len(decisions.Patterns) != 0 {
		t.Errorf("decisions.Patterns = %v, want empty", decisions.Patterns)
	}
}

func TestLoadGlobalApprovals_InvalidYAML(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	invalidYAML := "domains: [this is not valid yaml\n"
	if err := os.WriteFile(filepath.Join(decisionDir, "global.yaml"), []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalApprovals()
	if err == nil {
		t.Fatal("LoadGlobalApprovals() expected error for invalid YAML, got nil")
	}
}

func TestLoadProjectApprovals_InvalidYAML(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	projectsDir := filepath.Join(DecisionDir(), "projects")

	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	invalidYAML := "patterns: {bad: yaml: content\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "my-project.yaml"), []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadProjectApprovals("my-project")
	if err == nil {
		t.Fatal("LoadProjectApprovals() expected error for invalid YAML, got nil")
	}
}

func TestLoadGlobalApprovals_UnknownField(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	yamlContent := "domains:\n  - example.com\nunknown_field: bad\n"
	if err := os.WriteFile(filepath.Join(decisionDir, "global.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalApprovals()
	if err == nil {
		t.Fatal("LoadGlobalApprovals() expected error for unknown field, got nil")
	}
}

func TestWriteGlobalApprovals_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Domains:  []string{"example.com", "test.org"},
		Patterns: []string{"*.example.com"},
	}

	if err := WriteGlobalApprovals(original); err != nil {
		t.Fatalf("WriteGlobalApprovals() error = %v", err)
	}

	loaded, err := LoadGlobalApprovals()
	if err != nil {
		t.Fatalf("LoadGlobalApprovals() error = %v", err)
	}

	if len(loaded.Domains) != len(original.Domains) {
		t.Fatalf("len(loaded.Domains) = %d, want %d", len(loaded.Domains), len(original.Domains))
	}
	for i, domain := range original.Domains {
		if loaded.Domains[i] != domain {
			t.Errorf("loaded.Domains[%d] = %q, want %q", i, loaded.Domains[i], domain)
		}
	}

	if len(loaded.Patterns) != len(original.Patterns) {
		t.Fatalf("len(loaded.Patterns) = %d, want %d", len(loaded.Patterns), len(original.Patterns))
	}
	for i, pattern := range original.Patterns {
		if loaded.Patterns[i] != pattern {
			t.Errorf("loaded.Patterns[%d] = %q, want %q", i, loaded.Patterns[i], pattern)
		}
	}
}

func TestWriteProjectApprovals_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Domains:  []string{"project-specific.com"},
		Patterns: []string{"*.internal.corp"},
	}

	if err := WriteProjectApprovals("my-project", original); err != nil {
		t.Fatalf("WriteProjectApprovals() error = %v", err)
	}

	loaded, err := LoadProjectApprovals("my-project")
	if err != nil {
		t.Fatalf("LoadProjectApprovals() error = %v", err)
	}

	if len(loaded.Domains) != 1 || loaded.Domains[0] != "project-specific.com" {
		t.Errorf("loaded.Domains = %v, want [project-specific.com]", loaded.Domains)
	}
	if len(loaded.Patterns) != 1 || loaded.Patterns[0] != "*.internal.corp" {
		t.Errorf("loaded.Patterns = %v, want [*.internal.corp]", loaded.Patterns)
	}
}

func TestWriteGlobalApprovals_CreatesDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	// Verify directory does not exist
	if _, err := os.Stat(decisionDir); !os.IsNotExist(err) {
		t.Fatalf("decision dir should not exist before test: %v", err)
	}

	decisions := &Decisions{Domains: []string{"example.com"}}
	if err := WriteGlobalApprovals(decisions); err != nil {
		t.Fatalf("WriteGlobalApprovals() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(decisionDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("decision dir should be a directory")
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("decision dir permissions = %o, want 0700", perm)
	}
}

func TestWriteProjectApprovals_CreatesDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	projectsDir := filepath.Join(DecisionDir(), "projects")

	// Verify directory does not exist
	if _, err := os.Stat(projectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir should not exist before test: %v", err)
	}

	decisions := &Decisions{Domains: []string{"example.com"}}
	if err := WriteProjectApprovals("test-project", decisions); err != nil {
		t.Fatalf("WriteProjectApprovals() error = %v", err)
	}

	// Verify projects directory was created
	info, err := os.Stat(projectsDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("projects dir should be a directory")
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("projects dir permissions = %o, want 0700", perm)
	}
}

func TestWriteGlobalApprovals_FilePermissions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	decisions := &Decisions{Domains: []string{"example.com"}}
	if err := WriteGlobalApprovals(decisions); err != nil {
		t.Fatalf("WriteGlobalApprovals() error = %v", err)
	}

	info, err := os.Stat(GlobalDecisionPath())
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("decision file permissions = %o, want 0600", perm)
	}
}

func TestWriteGlobalApprovals_Overwrites(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Write initial decisions
	initial := &Decisions{Domains: []string{"old.com"}}
	if err := WriteGlobalApprovals(initial); err != nil {
		t.Fatalf("WriteGlobalApprovals() initial error = %v", err)
	}

	// Write updated decisions
	updated := &Decisions{Domains: []string{"new.com", "also-new.com"}}
	if err := WriteGlobalApprovals(updated); err != nil {
		t.Fatalf("WriteGlobalApprovals() updated error = %v", err)
	}

	// Load and verify updated content
	loaded, err := LoadGlobalApprovals()
	if err != nil {
		t.Fatalf("LoadGlobalApprovals() error = %v", err)
	}

	if len(loaded.Domains) != 2 {
		t.Fatalf("len(loaded.Domains) = %d, want 2", len(loaded.Domains))
	}
	if loaded.Domains[0] != "new.com" {
		t.Errorf("loaded.Domains[0] = %q, want %q", loaded.Domains[0], "new.com")
	}
	if loaded.Domains[1] != "also-new.com" {
		t.Errorf("loaded.Domains[1] = %q, want %q", loaded.Domains[1], "also-new.com")
	}
}

func TestWriteGlobalApprovals_EmptyDecisions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Write empty decisions
	if err := WriteGlobalApprovals(&Decisions{}); err != nil {
		t.Fatalf("WriteGlobalApprovals() error = %v", err)
	}

	// Load and verify
	loaded, err := LoadGlobalApprovals()
	if err != nil {
		t.Fatalf("LoadGlobalApprovals() error = %v", err)
	}

	if len(loaded.Domains) != 0 {
		t.Errorf("loaded.Domains = %v, want empty", loaded.Domains)
	}
	if len(loaded.Patterns) != 0 {
		t.Errorf("loaded.Patterns = %v, want empty", loaded.Patterns)
	}
}
