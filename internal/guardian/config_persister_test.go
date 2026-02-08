package guardian

import (
	"os"
	"slices"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestAddDomainToProject_WritesAndReloads(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing domain
	projectName := "test-project"
	initialApprovals := &config.Decisions{
		Domains: []string{"example.com"},
	}
	if err := config.WriteProjectDecisions(projectName, initialApprovals); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new domain
	newDomain := "docs.example.com"
	if err := persister.AddDomainToProject(projectName, newDomain); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify domain was added by reloading approvals
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	// Check that both original and new domain are present
	if !slices.Contains(approvals.Domains, "example.com") {
		t.Error("domain 'example.com' should be present in approvals")
	}
	if !slices.Contains(approvals.Domains, "docs.example.com") {
		t.Error("domain 'docs.example.com' should be present in approvals")
	}

	// Verify total count
	if len(approvals.Domains) != 2 {
		t.Errorf("approvals.Domains length = %d, want 2", len(approvals.Domains))
	}
}

func TestAddDomainToProject_NoDuplicate(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing domain
	projectName := "test-project"
	existingDomain := "example.com"
	initialApprovals := &config.Decisions{
		Domains: []string{existingDomain},
	}
	if err := config.WriteProjectDecisions(projectName, initialApprovals); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same domain again
	if err := persister.AddDomainToProject(projectName, existingDomain); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when domain already exists")
	}

	// Verify only one entry exists
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1 (no duplicate)", len(approvals.Domains))
	}

	if approvals.Domains[0] != existingDomain {
		t.Errorf("approvals.Domains[0] = %q, want %q", approvals.Domains[0], existingDomain)
	}
}

func TestAddDomainToProject_NoReloadNotifier(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-project"

	// Persister with nil ReloadNotifier
	persister := &ConfigPersisterImpl{
		ReloadNotifier: nil,
	}

	// Should not panic
	if err := persister.AddDomainToProject(projectName, "example.com"); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Verify domain was added
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1", len(approvals.Domains))
	}
}

func TestAddDomainToProject_CreatesApprovalFileIfMissing(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Don't create initial approval file - it should be created automatically
	projectName := "nonexistent-project"

	persister := &ConfigPersisterImpl{}

	// Add domain should succeed and create approval file
	newDomain := "example.com"
	if err := persister.AddDomainToProject(projectName, newDomain); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Verify approval file was created
	approvalPath := config.ProjectDecisionPath(projectName)
	if _, err := os.Stat(approvalPath); err != nil {
		t.Fatalf("approval file should have been created: %v", err)
	}

	// Verify domain was added
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1", len(approvals.Domains))
	}

	if approvals.Domains[0] != newDomain {
		t.Errorf("approvals.Domains[0] = %q, want %q", approvals.Domains[0], newDomain)
	}
}

func TestAddDomainToGlobal_WritesAndReloads(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing domain
	initialApprovals := &config.Decisions{
		Domains: []string{"golang.org"},
	}
	if err := config.WriteGlobalDecisions(initialApprovals); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new domain
	newDomain := "docs.golang.org"
	if err := persister.AddDomainToGlobal(newDomain); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify domain was added by reloading approvals
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Check that both original and new domain are present
	if !slices.Contains(approvals.Domains, "golang.org") {
		t.Error("domain 'golang.org' should be present in approvals")
	}
	if !slices.Contains(approvals.Domains, "docs.golang.org") {
		t.Error("domain 'docs.golang.org' should be present in approvals")
	}

	// Verify total count
	if len(approvals.Domains) != 2 {
		t.Errorf("approvals.Domains length = %d, want 2", len(approvals.Domains))
	}
}

func TestAddDomainToGlobal_NoDuplicate(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing domain
	existingDomain := "golang.org"
	initialApprovals := &config.Decisions{
		Domains: []string{existingDomain},
	}
	if err := config.WriteGlobalDecisions(initialApprovals); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same domain again
	if err := persister.AddDomainToGlobal(existingDomain); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when domain already exists")
	}

	// Verify only one entry exists
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1 (no duplicate)", len(approvals.Domains))
	}

	if approvals.Domains[0] != existingDomain {
		t.Errorf("approvals.Domains[0] = %q, want %q", approvals.Domains[0], existingDomain)
	}
}

func TestAddDomainToGlobal_NoReloadNotifier(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Persister with nil ReloadNotifier
	persister := &ConfigPersisterImpl{
		ReloadNotifier: nil,
	}

	// Should not panic
	if err := persister.AddDomainToGlobal("example.com"); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Verify domain was added
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1", len(approvals.Domains))
	}

	if approvals.Domains[0] != "example.com" {
		t.Errorf("approvals.Domains[0] = %q, want %q", approvals.Domains[0], "example.com")
	}
}

func TestAddDomainToGlobal_CreatesApprovalFileIfMissing(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Don't create initial approval file
	persister := &ConfigPersisterImpl{}

	// Add domain should succeed
	newDomain := "example.com"
	if err := persister.AddDomainToGlobal(newDomain); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Verify approval file was created
	approvalPath := config.GlobalDecisionPath()
	if _, err := os.Stat(approvalPath); err != nil {
		t.Fatalf("approval file should have been created: %v", err)
	}

	// Verify domain was added
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1", len(approvals.Domains))
	}

	if approvals.Domains[0] != newDomain {
		t.Errorf("approvals.Domains[0] = %q, want %q", approvals.Domains[0], newDomain)
	}
}

func TestAddDomainToProject_MultipleAdditions(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	projectName := "test-project"

	persister := &ConfigPersisterImpl{}

	// Add multiple domains
	domains := []string{"api.example.com", "docs.example.com", "cdn.example.com"}
	for _, domain := range domains {
		if err := persister.AddDomainToProject(projectName, domain); err != nil {
			t.Fatalf("AddDomainToProject(%q) error = %v", domain, err)
		}
	}

	// Verify all domains were added
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if len(approvals.Domains) != len(domains) {
		t.Errorf("approvals.Domains length = %d, want %d", len(approvals.Domains), len(domains))
	}

	// Verify each domain is present
	for _, domain := range domains {
		if !slices.Contains(approvals.Domains, domain) {
			t.Errorf("domain %q should be present in approvals", domain)
		}
	}
}

// Tests for pattern persistence

func TestAddPatternToProject_WritesAndReloads(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing domain
	projectName := "test-project"
	initialApprovals := &config.Decisions{
		Domains: []string{"example.com"},
	}
	if err := config.WriteProjectDecisions(projectName, initialApprovals); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new pattern
	newPattern := "*.example.com"
	if err := persister.AddPatternToProject(projectName, newPattern); err != nil {
		t.Fatalf("AddPatternToProject() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify pattern was added by reloading approvals
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	// Check that domain is still present and pattern was added
	if !slices.Contains(approvals.Domains, "example.com") {
		t.Error("domain 'example.com' should be present in approvals")
	}
	if !slices.Contains(approvals.Patterns, "*.example.com") {
		t.Error("pattern '*.example.com' should be present in approvals")
	}

	// Verify counts
	if len(approvals.Domains) != 1 {
		t.Errorf("approvals.Domains length = %d, want 1", len(approvals.Domains))
	}
	if len(approvals.Patterns) != 1 {
		t.Errorf("approvals.Patterns length = %d, want 1", len(approvals.Patterns))
	}
}

func TestAddPatternToProject_NoDuplicate(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing pattern
	projectName := "test-project"
	existingPattern := "*.example.com"
	initialApprovals := &config.Decisions{
		Patterns: []string{existingPattern},
	}
	if err := config.WriteProjectDecisions(projectName, initialApprovals); err != nil {
		t.Fatalf("WriteProjectDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same pattern again
	if err := persister.AddPatternToProject(projectName, existingPattern); err != nil {
		t.Fatalf("AddPatternToProject() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when pattern already exists")
	}

	// Verify only one entry exists
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	if len(approvals.Patterns) != 1 {
		t.Errorf("approvals.Patterns length = %d, want 1 (no duplicate)", len(approvals.Patterns))
	}

	if approvals.Patterns[0] != existingPattern {
		t.Errorf("approvals.Patterns[0] = %q, want %q", approvals.Patterns[0], existingPattern)
	}
}

func TestAddPatternToProject_InvalidPattern(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	persister := &ConfigPersisterImpl{}

	tests := []struct {
		name    string
		pattern string
	}{
		{"empty pattern", ""},
		{"missing asterisk prefix", "example.com"},
		{"whitespace", "*.example .com"},
		{"too short", "*."},
		{"overly broad *.com", "*.com"},
		{"overly broad *.org", "*.org"},
		{"overly broad *.net", "*.net"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := persister.AddPatternToProject("test-project", tc.pattern)
			if err == nil {
				t.Errorf("AddPatternToProject(%q) should return error", tc.pattern)
			}
		})
	}
}

func TestAddPatternToGlobal_WritesAndReloads(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing domain
	initialApprovals := &config.Decisions{
		Domains: []string{"golang.org"},
	}
	if err := config.WriteGlobalDecisions(initialApprovals); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Add new pattern
	newPattern := "*.googleapis.com"
	if err := persister.AddPatternToGlobal(newPattern); err != nil {
		t.Fatalf("AddPatternToGlobal() error = %v", err)
	}

	// Verify reload notifier was called
	if !reloadCalled {
		t.Error("ReloadNotifier should have been called")
	}

	// Verify pattern was added by reloading approvals
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Check that domain is still present and pattern was added
	if !slices.Contains(approvals.Domains, "golang.org") {
		t.Error("domain 'golang.org' should be present in approvals")
	}
	if !slices.Contains(approvals.Patterns, "*.googleapis.com") {
		t.Error("pattern '*.googleapis.com' should be present in approvals")
	}
}

func TestAddPatternToGlobal_NoDuplicate(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Set up initial approval file with one existing pattern
	existingPattern := "*.googleapis.com"
	initialApprovals := &config.Decisions{
		Patterns: []string{existingPattern},
	}
	if err := config.WriteGlobalDecisions(initialApprovals); err != nil {
		t.Fatalf("WriteGlobalDecisions() error = %v", err)
	}

	// Track reload calls
	reloadCalled := false
	persister := &ConfigPersisterImpl{
		ReloadNotifier: func() {
			reloadCalled = true
		},
	}

	// Try to add the same pattern again
	if err := persister.AddPatternToGlobal(existingPattern); err != nil {
		t.Fatalf("AddPatternToGlobal() error = %v", err)
	}

	// Reload notifier should NOT be called since no write occurred
	if reloadCalled {
		t.Error("ReloadNotifier should NOT have been called when pattern already exists")
	}

	// Verify only one entry exists
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	if len(approvals.Patterns) != 1 {
		t.Errorf("approvals.Patterns length = %d, want 1 (no duplicate)", len(approvals.Patterns))
	}

	if approvals.Patterns[0] != existingPattern {
		t.Errorf("approvals.Patterns[0] = %q, want %q", approvals.Patterns[0], existingPattern)
	}
}

// Tests verifying static config files remain unchanged after persistence operations

func TestAddDomainToProject_StaticConfigUnchanged(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a static project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{
				{Domain: "static.example.com"},
			},
		},
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Read static config file contents before persistence
	configPath := config.ProjectConfigPath(projectName)
	beforeBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Add domain via persister (should go to approval file, not config)
	if err := persister.AddDomainToProject(projectName, "new.example.com"); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	// Read static config file contents after persistence
	afterBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Verify static config is identical
	if string(beforeBytes) != string(afterBytes) {
		t.Error("static project config file should remain unchanged after persistence")
	}

	// Verify the domain went to approval file
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}
	if !slices.Contains(approvals.Domains, "new.example.com") {
		t.Error("domain 'new.example.com' should be present in approval file")
	}
}

func TestAddDomainToGlobal_StaticConfigUnchanged(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a static global config
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
			Allow: []config.AllowEntry{
				{Domain: "static.golang.org"},
			},
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Read static config file contents before persistence
	configPath := config.GlobalConfigPath()
	beforeBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Add domain via persister (should go to approval file, not config)
	if err := persister.AddDomainToGlobal("new.golang.org"); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	// Read static config file contents after persistence
	afterBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Verify static config is identical
	if string(beforeBytes) != string(afterBytes) {
		t.Error("static global config file should remain unchanged after persistence")
	}

	// Verify the domain went to approval file
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}
	if !slices.Contains(approvals.Domains, "new.golang.org") {
		t.Error("domain 'new.golang.org' should be present in approval file")
	}
}

func TestAddPatternToProject_StaticConfigUnchanged(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a static project config
	projectName := "test-project"
	initialCfg := &config.ProjectConfig{
		Remote: "git@github.com:example/repo.git",
		Root:   "/test/path",
		Proxy: config.ProjectProxyConfig{
			Allow: []config.AllowEntry{
				{Domain: "static.example.com"},
			},
		},
	}
	if err := config.WriteProjectConfig(projectName, initialCfg, false); err != nil {
		t.Fatalf("WriteProjectConfig() error = %v", err)
	}

	// Read static config file contents before persistence
	configPath := config.ProjectConfigPath(projectName)
	beforeBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Add pattern via persister (should go to approval file, not config)
	if err := persister.AddPatternToProject(projectName, "*.example.com"); err != nil {
		t.Fatalf("AddPatternToProject() error = %v", err)
	}

	// Read static config file contents after persistence
	afterBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Verify static config is identical
	if string(beforeBytes) != string(afterBytes) {
		t.Error("static project config file should remain unchanged after persistence")
	}

	// Verify the pattern went to approval file
	approvals, err := config.LoadProjectDecisions(projectName)
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}
	if !slices.Contains(approvals.Patterns, "*.example.com") {
		t.Error("pattern '*.example.com' should be present in approval file")
	}
}

func TestAddPatternToGlobal_StaticConfigUnchanged(t *testing.T) {
	// Setup isolated config and approval environments
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create a static global config
	initialCfg := &config.GlobalConfig{
		Proxy: config.ProxyConfig{
			Listen: ":3128",
			Allow: []config.AllowEntry{
				{Domain: "static.golang.org"},
			},
		},
	}
	if err := config.WriteGlobalConfig(initialCfg); err != nil {
		t.Fatalf("WriteGlobalConfig() error = %v", err)
	}

	// Read static config file contents before persistence
	configPath := config.GlobalConfigPath()
	beforeBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	persister := &ConfigPersisterImpl{}

	// Add pattern via persister (should go to approval file, not config)
	if err := persister.AddPatternToGlobal("*.googleapis.com"); err != nil {
		t.Fatalf("AddPatternToGlobal() error = %v", err)
	}

	// Read static config file contents after persistence
	afterBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Verify static config is identical
	if string(beforeBytes) != string(afterBytes) {
		t.Error("static global config file should remain unchanged after persistence")
	}

	// Verify the pattern went to approval file
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}
	if !slices.Contains(approvals.Patterns, "*.googleapis.com") {
		t.Error("pattern '*.googleapis.com' should be present in approval file")
	}
}

func TestAddDomainToProject_StripsPort(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	persister := &ConfigPersisterImpl{}

	// Add domain with port (as CONNECT requests provide)
	if err := persister.AddDomainToProject("test-project", "example.com:443"); err != nil {
		t.Fatalf("AddDomainToProject() error = %v", err)
	}

	approvals, err := config.LoadProjectDecisions("test-project")
	if err != nil {
		t.Fatalf("LoadProjectDecisions() error = %v", err)
	}

	// Domain should be stored without port
	if !slices.Contains(approvals.Domains, "example.com") {
		t.Errorf("expected 'example.com' (without port), got: %v", approvals.Domains)
	}
	if slices.Contains(approvals.Domains, "example.com:443") {
		t.Error("domain should not include port")
	}
}

func TestAddDomainToGlobal_StripsPort(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	persister := &ConfigPersisterImpl{}

	// Add domain with port
	if err := persister.AddDomainToGlobal("api.example.com:443"); err != nil {
		t.Fatalf("AddDomainToGlobal() error = %v", err)
	}

	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("LoadGlobalDecisions() error = %v", err)
	}

	// Domain should be stored without port
	if !slices.Contains(approvals.Domains, "api.example.com") {
		t.Errorf("expected 'api.example.com' (without port), got: %v", approvals.Domains)
	}
}

func TestValidatePatternSafety(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		shouldErr bool
	}{
		{"valid 3-component suffix", "*.example.com", false},
		{"valid 2-component suffix", "*.co.uk", false},
		{"valid multi-level", "*.api.example.com", false},
		{"invalid overly broad *.com", "*.com", true},
		{"invalid overly broad *.org", "*.org", true},
		{"invalid overly broad *.net", "*.net", true},
		{"invalid empty", "", true},
		{"invalid no prefix", "example.com", true},
		{"invalid whitespace", "*.example .com", true},
		{"invalid too short", "*.", true},
		{"invalid single component suffix", "*.localhost", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePattern(tc.pattern)
			if tc.shouldErr && err == nil {
				t.Errorf("validatePattern(%q) should return error", tc.pattern)
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("validatePattern(%q) should not return error, got: %v", tc.pattern, err)
			}
		})
	}
}
