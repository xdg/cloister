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

func TestLoadGlobalDecisions_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	decisions, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	if decisions == nil {
		t.Fatal("LoadGlobalDecisions() returned nil")
	}
	if len(decisions.Domains) != 0 {
		t.Errorf("decisions.Domains = %v, want empty", decisions.Domains)
	}
	if len(decisions.Patterns) != 0 {
		t.Errorf("decisions.Patterns = %v, want empty", decisions.Patterns)
	}
	if len(decisions.DeniedDomains) != 0 {
		t.Errorf("decisions.DeniedDomains = %v, want empty", decisions.DeniedDomains)
	}
	if len(decisions.DeniedPatterns) != 0 {
		t.Errorf("decisions.DeniedPatterns = %v, want empty", decisions.DeniedPatterns)
	}
}

func TestLoadProjectDecisions_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	decisions, err := LoadProjectDecisions("nonexistent")
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if decisions == nil {
		t.Fatal("LoadProjectDecisions() returned nil")
	}
	if len(decisions.Domains) != 0 {
		t.Errorf("decisions.Domains = %v, want empty", decisions.Domains)
	}
	if len(decisions.Patterns) != 0 {
		t.Errorf("decisions.Patterns = %v, want empty", decisions.Patterns)
	}
	if len(decisions.DeniedDomains) != 0 {
		t.Errorf("decisions.DeniedDomains = %v, want empty", decisions.DeniedDomains)
	}
	if len(decisions.DeniedPatterns) != 0 {
		t.Errorf("decisions.DeniedPatterns = %v, want empty", decisions.DeniedPatterns)
	}
}

func TestLoadGlobalDecisions_InvalidYAML(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	invalidYAML := "domains: [this is not valid yaml\n"
	if err := os.WriteFile(filepath.Join(decisionDir, "global.yaml"), []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalDecisions()
	if err == nil {
		t.Fatal("LoadGlobalDecisions() expected error for invalid YAML, got nil")
	}
}

func TestLoadProjectDecisions_InvalidYAML(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	projectsDir := filepath.Join(DecisionDir(), "projects")

	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	invalidYAML := "patterns: {bad: yaml: content\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "my-project.yaml"), []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadProjectDecisions("my-project")
	if err == nil {
		t.Fatal("LoadProjectDecisions() expected error for invalid YAML, got nil")
	}
}

func TestLoadGlobalDecisions_UnknownField(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	yamlContent := "domains:\n  - example.com\nunknown_field: bad\n"
	if err := os.WriteFile(filepath.Join(decisionDir, "global.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalDecisions()
	if err == nil {
		t.Fatal("LoadGlobalDecisions() expected error for unknown field, got nil")
	}
}

func TestWriteGlobalDecisions_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Domains:  []string{"example.com", "test.org"},
		Patterns: []string{"*.example.com"},
	}

	if err := WriteGlobalDecisions(original); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
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

func TestWriteProjectDecisions_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Domains:  []string{"project-specific.com"},
		Patterns: []string{"*.internal.corp"},
	}

	if err := WriteProjectDecisions("my-project", original); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
	}

	loaded, err := LoadProjectDecisions("my-project")
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if len(loaded.Domains) != 1 || loaded.Domains[0] != "project-specific.com" {
		t.Errorf("loaded.Domains = %v, want [project-specific.com]", loaded.Domains)
	}
	if len(loaded.Patterns) != 1 || loaded.Patterns[0] != "*.internal.corp" {
		t.Errorf("loaded.Patterns = %v, want [*.internal.corp]", loaded.Patterns)
	}
}

func TestWriteGlobalDecisions_CreatesDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	// Verify directory does not exist
	if _, err := os.Stat(decisionDir); !os.IsNotExist(err) {
		t.Fatalf("decision dir should not exist before test: %v", err)
	}

	decisions := &Decisions{Domains: []string{"example.com"}}
	if err := WriteGlobalDecisions(decisions); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
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

func TestWriteProjectDecisions_CreatesDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	projectsDir := filepath.Join(DecisionDir(), "projects")

	// Verify directory does not exist
	if _, err := os.Stat(projectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir should not exist before test: %v", err)
	}

	decisions := &Decisions{Domains: []string{"example.com"}}
	if err := WriteProjectDecisions("test-project", decisions); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
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

func TestWriteGlobalDecisions_FilePermissions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	decisions := &Decisions{Domains: []string{"example.com"}}
	if err := WriteGlobalDecisions(decisions); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
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

func TestWriteGlobalDecisions_Overwrites(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Write initial decisions
	initial := &Decisions{Domains: []string{"old.com"}}
	if err := WriteGlobalDecisions(initial); err != nil {
		t.Fatalf("WriteGlobalDecisions() initial error = %v", err)
	}

	// Write updated decisions
	updated := &Decisions{Domains: []string{"new.com", "also-new.com"}}
	if err := WriteGlobalDecisions(updated); err != nil {
		t.Fatalf("WriteGlobalDecisions() updated error = %v", err)
	}

	// Load and verify updated content
	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
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

func TestWriteGlobalDecisions_EmptyDecisions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Write empty decisions
	if err := WriteGlobalDecisions(&Decisions{}); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	// Load and verify
	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	if len(loaded.Domains) != 0 {
		t.Errorf("loaded.Domains = %v, want empty", loaded.Domains)
	}
	if len(loaded.Patterns) != 0 {
		t.Errorf("loaded.Patterns = %v, want empty", loaded.Patterns)
	}
	if len(loaded.DeniedDomains) != 0 {
		t.Errorf("loaded.DeniedDomains = %v, want empty", loaded.DeniedDomains)
	}
	if len(loaded.DeniedPatterns) != 0 {
		t.Errorf("loaded.DeniedPatterns = %v, want empty", loaded.DeniedPatterns)
	}
}

func TestLoadDecisions_AllFourFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	yamlContent := `domains:
  - example.com
  - test.org
patterns:
  - "*.example.com"
denied_domains:
  - evil.com
  - malware.net
denied_patterns:
  - "*.evil.com"
`
	if err := os.WriteFile(filepath.Join(decisionDir, "global.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	decisions, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Verify domains
	if len(decisions.Domains) != 2 {
		t.Fatalf("len(Domains) = %d, want 2", len(decisions.Domains))
	}
	if decisions.Domains[0] != "example.com" || decisions.Domains[1] != "test.org" {
		t.Errorf("Domains = %v, want [example.com test.org]", decisions.Domains)
	}

	// Verify patterns
	if len(decisions.Patterns) != 1 {
		t.Fatalf("len(Patterns) = %d, want 1", len(decisions.Patterns))
	}
	if decisions.Patterns[0] != "*.example.com" {
		t.Errorf("Patterns = %v, want [*.example.com]", decisions.Patterns)
	}

	// Verify denied_domains
	if len(decisions.DeniedDomains) != 2 {
		t.Fatalf("len(DeniedDomains) = %d, want 2", len(decisions.DeniedDomains))
	}
	if decisions.DeniedDomains[0] != "evil.com" || decisions.DeniedDomains[1] != "malware.net" {
		t.Errorf("DeniedDomains = %v, want [evil.com malware.net]", decisions.DeniedDomains)
	}

	// Verify denied_patterns
	if len(decisions.DeniedPatterns) != 1 {
		t.Fatalf("len(DeniedPatterns) = %d, want 1", len(decisions.DeniedPatterns))
	}
	if decisions.DeniedPatterns[0] != "*.evil.com" {
		t.Errorf("DeniedPatterns = %v, want [*.evil.com]", decisions.DeniedPatterns)
	}
}

func TestMigrateDecisionDir_MigratesOldDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	oldDir := ConfigDir() + "approvals"
	if err := os.MkdirAll(oldDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Write a test file in the old directory
	testContent := []byte("domains:\n  - example.com\n")
	if err := os.WriteFile(filepath.Join(oldDir, "global.yaml"), testContent, 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	migrated, err := MigrateDecisionDir()
	if err != nil {
		t.Fatalf("MigrateDecisionDir() error = %v", err)
	}
	if !migrated {
		t.Error("MigrateDecisionDir() = false, want true")
	}

	// Verify old directory is gone
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old approvals/ directory should not exist after migration, err = %v", err)
	}

	// Verify new directory exists with the file
	newDir := DecisionDir()
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("os.Stat(decisions/) error = %v", err)
	}
	if !info.IsDir() {
		t.Error("decisions/ should be a directory")
	}

	data, err := os.ReadFile(filepath.Join(newDir, "global.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != string(testContent) {
		t.Errorf("file content = %q, want %q", string(data), string(testContent))
	}
}

func TestMigrateDecisionDir_NoOldDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create the cloister config dir but not approvals/
	if err := os.MkdirAll(ConfigDir(), 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	migrated, err := MigrateDecisionDir()
	if err != nil {
		t.Fatalf("MigrateDecisionDir() error = %v", err)
	}
	if migrated {
		t.Error("MigrateDecisionDir() = true, want false (no old dir)")
	}
}

func TestMigrateDecisionDir_BothExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	oldDir := ConfigDir() + "approvals"
	newDir := DecisionDir()

	if err := os.MkdirAll(oldDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll(approvals) error = %v", err)
	}
	if err := os.MkdirAll(newDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll(decisions) error = %v", err)
	}

	// Write distinct files so we can verify neither was clobbered
	if err := os.WriteFile(filepath.Join(oldDir, "old.yaml"), []byte("old"), 0600); err != nil {
		t.Fatalf("os.WriteFile(old) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "new.yaml"), []byte("new"), 0600); err != nil {
		t.Fatalf("os.WriteFile(new) error = %v", err)
	}

	migrated, err := MigrateDecisionDir()
	if err != nil {
		t.Fatalf("MigrateDecisionDir() error = %v", err)
	}
	if migrated {
		t.Error("MigrateDecisionDir() = true, want false (both dirs exist)")
	}

	// Verify old directory still exists
	if _, err := os.Stat(oldDir); err != nil {
		t.Errorf("old approvals/ directory should still exist, err = %v", err)
	}

	// Verify new directory still has its original file
	data, err := os.ReadFile(filepath.Join(newDir, "new.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile(new) error = %v", err)
	}
	if string(data) != "new" {
		t.Errorf("decisions/new.yaml content = %q, want %q", string(data), "new")
	}
}

func TestMigrateDecisionDir_PreservesContents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	oldDir := ConfigDir() + "approvals"
	projectsDir := filepath.Join(oldDir, "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll(projects) error = %v", err)
	}

	globalContent := []byte("domains:\n  - example.com\n  - test.org\n")
	projectContent := []byte("domains:\n  - project.dev\npatterns:\n  - \"*.internal.corp\"\n")

	if err := os.WriteFile(filepath.Join(oldDir, "global.yaml"), globalContent, 0600); err != nil {
		t.Fatalf("os.WriteFile(global) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "test.yaml"), projectContent, 0600); err != nil {
		t.Fatalf("os.WriteFile(project) error = %v", err)
	}

	migrated, err := MigrateDecisionDir()
	if err != nil {
		t.Fatalf("MigrateDecisionDir() error = %v", err)
	}
	if !migrated {
		t.Error("MigrateDecisionDir() = false, want true")
	}

	newDir := DecisionDir()

	// Verify global.yaml preserved
	data, err := os.ReadFile(filepath.Join(newDir, "global.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile(global) error = %v", err)
	}
	if string(data) != string(globalContent) {
		t.Errorf("global.yaml content = %q, want %q", string(data), string(globalContent))
	}

	// Verify projects/test.yaml preserved
	data, err = os.ReadFile(filepath.Join(newDir, "projects", "test.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile(project) error = %v", err)
	}
	if string(data) != string(projectContent) {
		t.Errorf("projects/test.yaml content = %q, want %q", string(data), string(projectContent))
	}

	// Verify old directory is gone
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old approvals/ directory should not exist, err = %v", err)
	}
}

func TestWriteDecisions_AllFourFields_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Domains:        []string{"example.com", "test.org"},
		Patterns:       []string{"*.example.com"},
		DeniedDomains:  []string{"evil.com", "malware.net"},
		DeniedPatterns: []string{"*.evil.com", "*.bad.org"},
	}

	if err := WriteGlobalDecisions(original); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Verify domains round-trip
	if len(loaded.Domains) != len(original.Domains) {
		t.Fatalf("len(Domains) = %d, want %d", len(loaded.Domains), len(original.Domains))
	}
	for i, d := range original.Domains {
		if loaded.Domains[i] != d {
			t.Errorf("Domains[%d] = %q, want %q", i, loaded.Domains[i], d)
		}
	}

	// Verify patterns round-trip
	if len(loaded.Patterns) != len(original.Patterns) {
		t.Fatalf("len(Patterns) = %d, want %d", len(loaded.Patterns), len(original.Patterns))
	}
	for i, p := range original.Patterns {
		if loaded.Patterns[i] != p {
			t.Errorf("Patterns[%d] = %q, want %q", i, loaded.Patterns[i], p)
		}
	}

	// Verify denied_domains round-trip
	if len(loaded.DeniedDomains) != len(original.DeniedDomains) {
		t.Fatalf("len(DeniedDomains) = %d, want %d", len(loaded.DeniedDomains), len(original.DeniedDomains))
	}
	for i, d := range original.DeniedDomains {
		if loaded.DeniedDomains[i] != d {
			t.Errorf("DeniedDomains[%d] = %q, want %q", i, loaded.DeniedDomains[i], d)
		}
	}

	// Verify denied_patterns round-trip
	if len(loaded.DeniedPatterns) != len(original.DeniedPatterns) {
		t.Fatalf("len(DeniedPatterns) = %d, want %d", len(loaded.DeniedPatterns), len(original.DeniedPatterns))
	}
	for i, p := range original.DeniedPatterns {
		if loaded.DeniedPatterns[i] != p {
			t.Errorf("DeniedPatterns[%d] = %q, want %q", i, loaded.DeniedPatterns[i], p)
		}
	}
}
