package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	if len(decisions.Proxy.Allow) != 0 {
		t.Errorf("decisions.Proxy.Allow = %v, want empty", decisions.Proxy.Allow)
	}
	if len(decisions.Proxy.Deny) != 0 {
		t.Errorf("decisions.Proxy.Deny = %v, want empty", decisions.Proxy.Deny)
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
	if len(decisions.Proxy.Allow) != 0 {
		t.Errorf("decisions.Proxy.Allow = %v, want empty", decisions.Proxy.Allow)
	}
	if len(decisions.Proxy.Deny) != 0 {
		t.Errorf("decisions.Proxy.Deny = %v, want empty", decisions.Proxy.Deny)
	}
}

func TestLoadGlobalDecisions_InvalidYAML(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	invalidYAML := "proxy: [this is not valid yaml\n"
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

	invalidYAML := "proxy: {bad: yaml: content\n"
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

	yamlContent := "proxy:\n  allow:\n    - domain: example.com\nunknown_field: bad\n"
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
		Proxy: DecisionsProxy{
			Allow: []AllowEntry{
				{Domain: "example.com"},
				{Domain: "test.org"},
				{Pattern: "*.example.com"},
			},
		},
	}

	if err := WriteGlobalDecisions(original); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	domains := loaded.AllowedDomains()
	if len(domains) != 2 {
		t.Fatalf("len(AllowedDomains()) = %d, want 2", len(domains))
	}
	if domains[0] != "example.com" || domains[1] != "test.org" {
		t.Errorf("AllowedDomains() = %v, want [example.com test.org]", domains)
	}

	patterns := loaded.AllowedPatterns()
	if len(patterns) != 1 {
		t.Fatalf("len(AllowedPatterns()) = %d, want 1", len(patterns))
	}
	if patterns[0] != "*.example.com" {
		t.Errorf("AllowedPatterns() = %v, want [*.example.com]", patterns)
	}
}

func TestWriteProjectDecisions_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Proxy: DecisionsProxy{
			Allow: []AllowEntry{
				{Domain: "project-specific.com"},
				{Pattern: "*.internal.corp"},
			},
		},
	}

	if err := WriteProjectDecisions("my-project", original); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
	}

	loaded, err := LoadProjectDecisions("my-project")
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	domains := loaded.AllowedDomains()
	if len(domains) != 1 || domains[0] != "project-specific.com" {
		t.Errorf("AllowedDomains() = %v, want [project-specific.com]", domains)
	}
	patterns := loaded.AllowedPatterns()
	if len(patterns) != 1 || patterns[0] != "*.internal.corp" {
		t.Errorf("AllowedPatterns() = %v, want [*.internal.corp]", patterns)
	}
}

func TestWriteGlobalDecisions_CreatesDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	// Verify directory does not exist
	if _, err := os.Stat(decisionDir); !os.IsNotExist(err) {
		t.Fatalf("decision dir should not exist before test: %v", err)
	}

	decisions := &Decisions{Proxy: DecisionsProxy{Allow: []AllowEntry{{Domain: "example.com"}}}}
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

	decisions := &Decisions{Proxy: DecisionsProxy{Allow: []AllowEntry{{Domain: "example.com"}}}}
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

	decisions := &Decisions{Proxy: DecisionsProxy{Allow: []AllowEntry{{Domain: "example.com"}}}}
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
	initial := &Decisions{Proxy: DecisionsProxy{Allow: []AllowEntry{{Domain: "old.com"}}}}
	if err := WriteGlobalDecisions(initial); err != nil {
		t.Fatalf("WriteGlobalDecisions() initial error = %v", err)
	}

	// Write updated decisions
	updated := &Decisions{Proxy: DecisionsProxy{Allow: []AllowEntry{{Domain: "new.com"}, {Domain: "also-new.com"}}}}
	if err := WriteGlobalDecisions(updated); err != nil {
		t.Fatalf("WriteGlobalDecisions() updated error = %v", err)
	}

	// Load and verify updated content
	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	domains := loaded.AllowedDomains()
	if len(domains) != 2 {
		t.Fatalf("len(AllowedDomains()) = %d, want 2", len(domains))
	}
	if domains[0] != "new.com" {
		t.Errorf("AllowedDomains()[0] = %q, want %q", domains[0], "new.com")
	}
	if domains[1] != "also-new.com" {
		t.Errorf("AllowedDomains()[1] = %q, want %q", domains[1], "also-new.com")
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

	if len(loaded.Proxy.Allow) != 0 {
		t.Errorf("loaded.Proxy.Allow = %v, want empty", loaded.Proxy.Allow)
	}
	if len(loaded.Proxy.Deny) != 0 {
		t.Errorf("loaded.Proxy.Deny = %v, want empty", loaded.Proxy.Deny)
	}
}

func TestLoadDecisions_AllFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	decisionDir := DecisionDir()

	if err := os.MkdirAll(decisionDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	yamlContent := `proxy:
  allow:
    - domain: example.com
    - domain: test.org
    - pattern: "*.example.com"
  deny:
    - domain: evil.com
    - domain: malware.net
    - pattern: "*.evil.com"
`
	if err := os.WriteFile(filepath.Join(decisionDir, "global.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	decisions, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Verify allowed domains
	domains := decisions.AllowedDomains()
	if len(domains) != 2 {
		t.Fatalf("len(AllowedDomains()) = %d, want 2", len(domains))
	}
	if domains[0] != "example.com" || domains[1] != "test.org" {
		t.Errorf("AllowedDomains() = %v, want [example.com test.org]", domains)
	}

	// Verify allowed patterns
	patterns := decisions.AllowedPatterns()
	if len(patterns) != 1 {
		t.Fatalf("len(AllowedPatterns()) = %d, want 1", len(patterns))
	}
	if patterns[0] != "*.example.com" {
		t.Errorf("AllowedPatterns() = %v, want [*.example.com]", patterns)
	}

	// Verify denied domains
	deniedDomains := decisions.DeniedDomains()
	if len(deniedDomains) != 2 {
		t.Fatalf("len(DeniedDomains()) = %d, want 2", len(deniedDomains))
	}
	if deniedDomains[0] != "evil.com" || deniedDomains[1] != "malware.net" {
		t.Errorf("DeniedDomains() = %v, want [evil.com malware.net]", deniedDomains)
	}

	// Verify denied patterns
	deniedPatterns := decisions.DeniedPatterns()
	if len(deniedPatterns) != 1 {
		t.Fatalf("len(DeniedPatterns()) = %d, want 1", len(deniedPatterns))
	}
	if deniedPatterns[0] != "*.evil.com" {
		t.Errorf("DeniedPatterns() = %v, want [*.evil.com]", deniedPatterns)
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
	testContent := []byte("proxy:\n  allow:\n    - domain: example.com\n")
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

	globalContent := []byte("proxy:\n  allow:\n    - domain: example.com\n    - domain: test.org\n")
	projectContent := []byte("proxy:\n  allow:\n    - domain: project.dev\n    - pattern: \"*.internal.corp\"\n")

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

func TestWriteDecisions_AllFields_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Proxy: DecisionsProxy{
			Allow: []AllowEntry{
				{Domain: "example.com"},
				{Domain: "test.org"},
				{Pattern: "*.example.com"},
			},
			Deny: []AllowEntry{
				{Domain: "evil.com"},
				{Domain: "malware.net"},
				{Pattern: "*.evil.com"},
				{Pattern: "*.bad.org"},
			},
		},
	}

	if err := WriteGlobalDecisions(original); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	loaded, err := LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Verify allowed domains round-trip
	domains := loaded.AllowedDomains()
	if len(domains) != 2 {
		t.Fatalf("len(AllowedDomains()) = %d, want 2", len(domains))
	}
	for i, d := range []string{"example.com", "test.org"} {
		if domains[i] != d {
			t.Errorf("AllowedDomains()[%d] = %q, want %q", i, domains[i], d)
		}
	}

	// Verify allowed patterns round-trip
	patterns := loaded.AllowedPatterns()
	if len(patterns) != 1 {
		t.Fatalf("len(AllowedPatterns()) = %d, want 1", len(patterns))
	}
	if patterns[0] != "*.example.com" {
		t.Errorf("AllowedPatterns()[0] = %q, want %q", patterns[0], "*.example.com")
	}

	// Verify denied domains round-trip
	deniedDomains := loaded.DeniedDomains()
	if len(deniedDomains) != 2 {
		t.Fatalf("len(DeniedDomains()) = %d, want 2", len(deniedDomains))
	}
	for i, d := range []string{"evil.com", "malware.net"} {
		if deniedDomains[i] != d {
			t.Errorf("DeniedDomains()[%d] = %q, want %q", i, deniedDomains[i], d)
		}
	}

	// Verify denied patterns round-trip
	deniedPatterns := loaded.DeniedPatterns()
	if len(deniedPatterns) != 2 {
		t.Fatalf("len(DeniedPatterns()) = %d, want 2", len(deniedPatterns))
	}
	for i, p := range []string{"*.evil.com", "*.bad.org"} {
		if deniedPatterns[i] != p {
			t.Errorf("DeniedPatterns()[%d] = %q, want %q", i, deniedPatterns[i], p)
		}
	}
}

// --- New tests for Phase 6.4 ---

func TestDecisions_EmptyMarshal(t *testing.T) {
	// Empty Decisions should marshal to empty/minimal YAML without spurious keys
	d := &Decisions{}
	data, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	out := strings.TrimSpace(string(data))
	if out != "{}" {
		t.Errorf("empty Decisions marshaled to %q, want %q", out, "{}")
	}
}

func TestDecisions_RoundTripNewFormat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	original := &Decisions{
		Proxy: DecisionsProxy{
			Allow: []AllowEntry{
				{Domain: "allowed.com"},
				{Pattern: "*.allowed.net"},
			},
			Deny: []AllowEntry{
				{Domain: "denied.com"},
				{Pattern: "*.denied.net"},
			},
		},
	}

	// Marshal
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	// Unmarshal back
	var loaded Decisions
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	// Verify allow entries
	if len(loaded.Proxy.Allow) != 2 {
		t.Fatalf("len(Proxy.Allow) = %d, want 2", len(loaded.Proxy.Allow))
	}
	if loaded.Proxy.Allow[0].Domain != "allowed.com" {
		t.Errorf("Proxy.Allow[0].Domain = %q, want %q", loaded.Proxy.Allow[0].Domain, "allowed.com")
	}
	if loaded.Proxy.Allow[1].Pattern != "*.allowed.net" {
		t.Errorf("Proxy.Allow[1].Pattern = %q, want %q", loaded.Proxy.Allow[1].Pattern, "*.allowed.net")
	}

	// Verify deny entries
	if len(loaded.Proxy.Deny) != 2 {
		t.Fatalf("len(Proxy.Deny) = %d, want 2", len(loaded.Proxy.Deny))
	}
	if loaded.Proxy.Deny[0].Domain != "denied.com" {
		t.Errorf("Proxy.Deny[0].Domain = %q, want %q", loaded.Proxy.Deny[0].Domain, "denied.com")
	}
	if loaded.Proxy.Deny[1].Pattern != "*.denied.net" {
		t.Errorf("Proxy.Deny[1].Pattern = %q, want %q", loaded.Proxy.Deny[1].Pattern, "*.denied.net")
	}
}

func TestDecisions_HelperMethods(t *testing.T) {
	d := &Decisions{
		Proxy: DecisionsProxy{
			Allow: []AllowEntry{
				{Domain: "a.com"},
				{Pattern: "*.a.com"},
				{Domain: "b.com"},
				{Pattern: "*.b.com"},
			},
			Deny: []AllowEntry{
				{Domain: "x.com"},
				{Pattern: "*.x.com"},
				{Domain: "y.com"},
			},
		},
	}

	// AllowedDomains
	ad := d.AllowedDomains()
	if len(ad) != 2 || ad[0] != "a.com" || ad[1] != "b.com" {
		t.Errorf("AllowedDomains() = %v, want [a.com b.com]", ad)
	}

	// AllowedPatterns
	ap := d.AllowedPatterns()
	if len(ap) != 2 || ap[0] != "*.a.com" || ap[1] != "*.b.com" {
		t.Errorf("AllowedPatterns() = %v, want [*.a.com *.b.com]", ap)
	}

	// DeniedDomains
	dd := d.DeniedDomains()
	if len(dd) != 2 || dd[0] != "x.com" || dd[1] != "y.com" {
		t.Errorf("DeniedDomains() = %v, want [x.com y.com]", dd)
	}

	// DeniedPatterns
	dp := d.DeniedPatterns()
	if len(dp) != 1 || dp[0] != "*.x.com" {
		t.Errorf("DeniedPatterns() = %v, want [*.x.com]", dp)
	}
}

func TestDecisions_HelperMethods_Empty(t *testing.T) {
	d := &Decisions{}

	if ad := d.AllowedDomains(); ad != nil {
		t.Errorf("AllowedDomains() on empty = %v, want nil", ad)
	}
	if ap := d.AllowedPatterns(); ap != nil {
		t.Errorf("AllowedPatterns() on empty = %v, want nil", ap)
	}
	if dd := d.DeniedDomains(); dd != nil {
		t.Errorf("DeniedDomains() on empty = %v, want nil", dd)
	}
	if dp := d.DeniedPatterns(); dp != nil {
		t.Errorf("DeniedPatterns() on empty = %v, want nil", dp)
	}
}

func TestDecisions_HelperMethods_PatternsOnly(t *testing.T) {
	// Entries with only patterns should not appear in domain helpers and vice versa
	d := &Decisions{
		Proxy: DecisionsProxy{
			Allow: []AllowEntry{
				{Pattern: "*.only-pattern.com"},
			},
			Deny: []AllowEntry{
				{Pattern: "*.deny-pattern.com"},
			},
		},
	}

	if ad := d.AllowedDomains(); len(ad) != 0 {
		t.Errorf("AllowedDomains() = %v, want empty", ad)
	}
	if dd := d.DeniedDomains(); len(dd) != 0 {
		t.Errorf("DeniedDomains() = %v, want empty", dd)
	}
	if ap := d.AllowedPatterns(); len(ap) != 1 || ap[0] != "*.only-pattern.com" {
		t.Errorf("AllowedPatterns() = %v, want [*.only-pattern.com]", ap)
	}
	if dp := d.DeniedPatterns(); len(dp) != 1 || dp[0] != "*.deny-pattern.com" {
		t.Errorf("DeniedPatterns() = %v, want [*.deny-pattern.com]", dp)
	}
}
