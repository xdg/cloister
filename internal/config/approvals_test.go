package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApprovalDir_Default(t *testing.T) {
	t.Setenv("CLOISTER_APPROVAL_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	dir := ApprovalDir()

	want := "/test/config/cloister/approvals"
	if dir != want {
		t.Errorf("ApprovalDir() = %q, want %q", dir, want)
	}
}

func TestApprovalDir_EnvOverride(t *testing.T) {
	t.Setenv("CLOISTER_APPROVAL_DIR", "/container/approvals")

	dir := ApprovalDir()

	want := "/container/approvals"
	if dir != want {
		t.Errorf("ApprovalDir() = %q, want %q", dir, want)
	}
}

func TestGlobalApprovalPath(t *testing.T) {
	t.Setenv("CLOISTER_APPROVAL_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := GlobalApprovalPath()

	want := "/test/config/cloister/approvals/global.yaml"
	if path != want {
		t.Errorf("GlobalApprovalPath() = %q, want %q", path, want)
	}
}

func TestProjectApprovalPath(t *testing.T) {
	t.Setenv("CLOISTER_APPROVAL_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := ProjectApprovalPath("my-project")

	want := "/test/config/cloister/approvals/projects/my-project.yaml"
	if path != want {
		t.Errorf("ProjectApprovalPath() = %q, want %q", path, want)
	}
}

func TestLoadGlobalApprovals_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	approvals, err := LoadGlobalApprovals()
	if err != nil {
		t.Fatalf("LoadGlobalApprovals() error = %v", err)
	}

	if approvals == nil {
		t.Fatal("LoadGlobalApprovals() returned nil")
	}
	if len(approvals.Domains) != 0 {
		t.Errorf("approvals.Domains = %v, want empty", approvals.Domains)
	}
	if len(approvals.Patterns) != 0 {
		t.Errorf("approvals.Patterns = %v, want empty", approvals.Patterns)
	}
}

func TestLoadProjectApprovals_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	approvals, err := LoadProjectApprovals("nonexistent")
	if err != nil {
		t.Fatalf("LoadProjectApprovals() error = %v", err)
	}

	if approvals == nil {
		t.Fatal("LoadProjectApprovals() returned nil")
	}
	if len(approvals.Domains) != 0 {
		t.Errorf("approvals.Domains = %v, want empty", approvals.Domains)
	}
	if len(approvals.Patterns) != 0 {
		t.Errorf("approvals.Patterns = %v, want empty", approvals.Patterns)
	}
}

func TestLoadGlobalApprovals_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	approvalDir := filepath.Join(tmpDir, "approvals")
	t.Setenv("CLOISTER_APPROVAL_DIR", approvalDir)

	if err := os.MkdirAll(approvalDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	invalidYAML := "domains: [this is not valid yaml\n"
	if err := os.WriteFile(filepath.Join(approvalDir, "global.yaml"), []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalApprovals()
	if err == nil {
		t.Fatal("LoadGlobalApprovals() expected error for invalid YAML, got nil")
	}
}

func TestLoadProjectApprovals_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	approvalDir := filepath.Join(tmpDir, "approvals")
	projectsDir := filepath.Join(approvalDir, "projects")
	t.Setenv("CLOISTER_APPROVAL_DIR", approvalDir)

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
	tmpDir := t.TempDir()
	approvalDir := filepath.Join(tmpDir, "approvals")
	t.Setenv("CLOISTER_APPROVAL_DIR", approvalDir)

	if err := os.MkdirAll(approvalDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	yamlContent := "domains:\n  - example.com\nunknown_field: bad\n"
	if err := os.WriteFile(filepath.Join(approvalDir, "global.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := LoadGlobalApprovals()
	if err == nil {
		t.Fatal("LoadGlobalApprovals() expected error for unknown field, got nil")
	}
}

func TestWriteGlobalApprovals_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	original := &Approvals{
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
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	original := &Approvals{
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
	tmpDir := t.TempDir()
	approvalDir := filepath.Join(tmpDir, "approvals")
	t.Setenv("CLOISTER_APPROVAL_DIR", approvalDir)

	// Verify directory does not exist
	if _, err := os.Stat(approvalDir); !os.IsNotExist(err) {
		t.Fatalf("approval dir should not exist before test: %v", err)
	}

	approvals := &Approvals{Domains: []string{"example.com"}}
	if err := WriteGlobalApprovals(approvals); err != nil {
		t.Fatalf("WriteGlobalApprovals() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(approvalDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("approval dir should be a directory")
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("approval dir permissions = %o, want 0700", perm)
	}
}

func TestWriteProjectApprovals_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	approvalDir := filepath.Join(tmpDir, "approvals")
	projectsDir := filepath.Join(approvalDir, "projects")
	t.Setenv("CLOISTER_APPROVAL_DIR", approvalDir)

	// Verify directory does not exist
	if _, err := os.Stat(projectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir should not exist before test: %v", err)
	}

	approvals := &Approvals{Domains: []string{"example.com"}}
	if err := WriteProjectApprovals("test-project", approvals); err != nil {
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
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	approvals := &Approvals{Domains: []string{"example.com"}}
	if err := WriteGlobalApprovals(approvals); err != nil {
		t.Fatalf("WriteGlobalApprovals() error = %v", err)
	}

	info, err := os.Stat(GlobalApprovalPath())
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("approval file permissions = %o, want 0600", perm)
	}
}

func TestWriteGlobalApprovals_Overwrites(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	// Write initial approvals
	initial := &Approvals{Domains: []string{"old.com"}}
	if err := WriteGlobalApprovals(initial); err != nil {
		t.Fatalf("WriteGlobalApprovals() initial error = %v", err)
	}

	// Write updated approvals
	updated := &Approvals{Domains: []string{"new.com", "also-new.com"}}
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

func TestWriteGlobalApprovals_EmptyApprovals(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLOISTER_APPROVAL_DIR", filepath.Join(tmpDir, "approvals"))

	// Write empty approvals
	if err := WriteGlobalApprovals(&Approvals{}); err != nil {
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
