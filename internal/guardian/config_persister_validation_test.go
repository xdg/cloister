package guardian

import (
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestAddDomainToProject_EmptyDomain(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Try to add empty domain
	err := persister.AddDomainToProject(projectName, "")
	if err == nil {
		t.Fatal("AddDomainToProject() should return error for empty domain")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestAddDomainToProject_WhitespaceDomain(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a test project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Test various whitespace scenarios
	invalidDomains := []string{
		"example.com ",
		" example.com",
		"exam ple.com",
		"example.com\n",
		"example.com\t",
	}

	for _, domain := range invalidDomains {
		err := persister.AddDomainToProject(projectName, domain)
		if err == nil {
			t.Errorf("AddDomainToProject(%q) should return error for domain with whitespace", domain)
		}
		if !strings.Contains(err.Error(), "whitespace") {
			t.Errorf("error should mention 'whitespace' for domain %q, got: %v", domain, err)
		}
	}
}

func TestAddDomainToGlobal_EmptyDomain(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	persister := &ConfigPersisterImpl{}

	// Try to add empty domain
	err := persister.AddDomainToGlobal("")
	if err == nil {
		t.Fatal("AddDomainToGlobal() should return error for empty domain")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestAddDomainToGlobal_WhitespaceDomain(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	persister := &ConfigPersisterImpl{}

	// Test various whitespace scenarios
	invalidDomains := []string{
		"golang.org ",
		" golang.org",
		"go lang.org",
		"golang.org\n",
		"golang.org\t",
	}

	for _, domain := range invalidDomains {
		err := persister.AddDomainToGlobal(domain)
		if err == nil {
			t.Errorf("AddDomainToGlobal(%q) should return error for domain with whitespace", domain)
		}
		if !strings.Contains(err.Error(), "whitespace") {
			t.Errorf("error should mention 'whitespace' for domain %q, got: %v", domain, err)
		}
	}
}

func TestReloadNotifier_PanicSafety(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-project"

	// Create persister with panicking notifier
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			panic("intentional test panic")
		},
	}

	// This should not panic - the panic should be caught
	err := persister.AddDomainToProject(projectName, "example.com")
	if err != nil {
		t.Fatalf("AddDomainToProject() error = %v, expected success despite panic", err)
	}

	// Verify domain was still added despite the panic
	approvals, err := config.LoadProjectApprovals(projectName)
	if err != nil {
		t.Fatalf("LoadProjectApprovals() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1", len(approvals.Domains))
	}

	if approvals.Domains[0] != "example.com" {
		t.Errorf("approvals.Domains[0] = %q, want 'example.com'", approvals.Domains[0])
	}
}

func TestReloadNotifier_GlobalPanicSafety(t *testing.T) {
	// Setup isolated config environment
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create persister with panicking notifier
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			panic("intentional test panic")
		},
	}

	// This should not panic - the panic should be caught
	err := persister.AddDomainToGlobal("example.com")
	if err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v, expected success despite panic", err)
	}

	// Verify domain was still added despite the panic
	approvals, err := config.LoadGlobalApprovals()
	if err != nil {
		t.Fatalf("LoadGlobalApprovals() error = %v", err)
	}

	// Check that example.com was added
	found := false
	for _, d := range approvals.Domains {
		if d == "example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Error("example.com should be present in approvals despite panic")
	}
}
