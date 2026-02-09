package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeAllowlists_Empty(t *testing.T) {
	result := MergeAllowlists(nil, nil)
	if result != nil {
		t.Errorf("MergeAllowlists(nil, nil) = %v, want nil", result)
	}

	result = MergeAllowlists([]AllowEntry{}, []AllowEntry{})
	if result != nil {
		t.Errorf("MergeAllowlists([], []) = %v, want nil", result)
	}
}

func TestMergeAllowlists_GlobalOnly(t *testing.T) {
	global := []AllowEntry{
		{Domain: "example.com"},
		{Domain: "api.example.com"},
	}

	result := MergeAllowlists(global, nil)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Domain != "example.com" {
		t.Errorf("result[0].Domain = %q, want %q", result[0].Domain, "example.com")
	}
	if result[1].Domain != "api.example.com" {
		t.Errorf("result[1].Domain = %q, want %q", result[1].Domain, "api.example.com")
	}
}

func TestMergeAllowlists_ProjectOnly(t *testing.T) {
	project := []AllowEntry{
		{Domain: "project.example.com"},
		{Domain: "docs.project.com"},
	}

	result := MergeAllowlists(nil, project)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Domain != "project.example.com" {
		t.Errorf("result[0].Domain = %q, want %q", result[0].Domain, "project.example.com")
	}
	if result[1].Domain != "docs.project.com" {
		t.Errorf("result[1].Domain = %q, want %q", result[1].Domain, "docs.project.com")
	}
}

func TestMergeAllowlists_Merge(t *testing.T) {
	global := []AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
	}
	project := []AllowEntry{
		{Domain: "custom.example.com"},
		{Domain: "internal.corp.com"},
	}

	result := MergeAllowlists(global, project)
	if len(result) != 4 {
		t.Fatalf("len(result) = %d, want 4", len(result))
	}

	// Verify order: global entries first, then project entries
	expected := []string{"golang.org", "api.anthropic.com", "custom.example.com", "internal.corp.com"}
	for i, want := range expected {
		if result[i].Domain != want {
			t.Errorf("result[%d].Domain = %q, want %q", i, result[i].Domain, want)
		}
	}
}

func TestMergeAllowlists_Dedup(t *testing.T) {
	global := []AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
		{Domain: "shared.example.com"},
	}
	project := []AllowEntry{
		{Domain: "shared.example.com"}, // Duplicate
		{Domain: "golang.org"},         // Duplicate
		{Domain: "unique.project.com"},
	}

	result := MergeAllowlists(global, project)
	if len(result) != 4 {
		t.Fatalf("len(result) = %d, want 4", len(result))
	}

	// Verify duplicates are removed, keeping global order
	expected := []string{"golang.org", "api.anthropic.com", "shared.example.com", "unique.project.com"}
	for i, want := range expected {
		if result[i].Domain != want {
			t.Errorf("result[%d].Domain = %q, want %q", i, result[i].Domain, want)
		}
	}
}

func TestMergeCommandPatterns_Merge(t *testing.T) {
	global := []CommandPattern{
		{Pattern: "^docker compose ps$"},
		{Pattern: "^docker compose logs.*$"},
	}
	project := []CommandPattern{
		{Pattern: "^make test$"},
		{Pattern: "^npm run build$"},
	}

	result := MergeCommandPatterns(global, project)
	if len(result) != 4 {
		t.Fatalf("len(result) = %d, want 4", len(result))
	}

	// Verify order: global patterns first, then project patterns
	expected := []string{"^docker compose ps$", "^docker compose logs.*$", "^make test$", "^npm run build$"}
	for i, want := range expected {
		if result[i].Pattern != want {
			t.Errorf("result[%d].Pattern = %q, want %q", i, result[i].Pattern, want)
		}
	}
}

func TestMergeCommandPatterns_Dedup(t *testing.T) {
	global := []CommandPattern{
		{Pattern: "^docker compose ps$"},
		{Pattern: "^make test$"},
	}
	project := []CommandPattern{
		{Pattern: "^make test$"},         // Duplicate
		{Pattern: "^docker compose ps$"}, // Duplicate
		{Pattern: "^npm run lint$"},
	}

	result := MergeCommandPatterns(global, project)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// Verify duplicates are removed, keeping global order
	expected := []string{"^docker compose ps$", "^make test$", "^npm run lint$"}
	for i, want := range expected {
		if result[i].Pattern != want {
			t.Errorf("result[%d].Pattern = %q, want %q", i, result[i].Pattern, want)
		}
	}
}

func TestMergeCommandPatterns_Empty(t *testing.T) {
	result := MergeCommandPatterns(nil, nil)
	if result != nil {
		t.Errorf("MergeCommandPatterns(nil, nil) = %v, want nil", result)
	}

	result = MergeCommandPatterns([]CommandPattern{}, []CommandPattern{})
	if result != nil {
		t.Errorf("MergeCommandPatterns([], []) = %v, want nil", result)
	}
}

func TestMergeDenylists_Empty(t *testing.T) {
	result := MergeDenylists(nil, nil)
	if result != nil {
		t.Errorf("MergeDenylists(nil, nil) = %v, want nil", result)
	}

	result = MergeDenylists([]AllowEntry{}, []AllowEntry{})
	if result != nil {
		t.Errorf("MergeDenylists([], []) = %v, want nil", result)
	}
}

func TestMergeDenylists_Merge(t *testing.T) {
	global := []AllowEntry{
		{Domain: "evil.com"},
		{Domain: "malware.net"},
	}
	project := []AllowEntry{
		{Domain: "banned.example.com"},
		{Domain: "phishing.org"},
	}

	result := MergeDenylists(global, project)
	if len(result) != 4 {
		t.Fatalf("len(result) = %d, want 4", len(result))
	}

	// Verify order: global entries first, then project entries
	expected := []string{"evil.com", "malware.net", "banned.example.com", "phishing.org"}
	for i, want := range expected {
		if result[i].Domain != want {
			t.Errorf("result[%d].Domain = %q, want %q", i, result[i].Domain, want)
		}
	}
}

func TestMergeDenylists_WithPatterns(t *testing.T) {
	global := []AllowEntry{
		{Domain: "evil.com"},
		{Pattern: "*.malware.net"},
	}
	project := []AllowEntry{
		{Domain: "banned.example.com"},
	}

	result := MergeDenylists(global, project)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	if result[0].Domain != "evil.com" {
		t.Errorf("result[0].Domain = %q, want %q", result[0].Domain, "evil.com")
	}
	if result[1].Pattern != "*.malware.net" {
		t.Errorf("result[1].Pattern = %q, want %q", result[1].Pattern, "*.malware.net")
	}
	if result[2].Domain != "banned.example.com" {
		t.Errorf("result[2].Domain = %q, want %q", result[2].Domain, "banned.example.com")
	}
}

func TestMergeDenylists_Dedup(t *testing.T) {
	global := []AllowEntry{
		{Domain: "evil.com"},
		{Domain: "banned.example.com"},
	}
	project := []AllowEntry{
		{Domain: "evil.com"},           // Duplicate
		{Domain: "project-banned.com"}, // Unique
	}

	result := MergeDenylists(global, project)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	expected := []string{"evil.com", "banned.example.com", "project-banned.com"}
	for i, want := range expected {
		if result[i].Domain != want {
			t.Errorf("result[%d].Domain = %q, want %q", i, result[i].Domain, want)
		}
	}
}

func TestResolveConfig_GlobalOnly(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// No project name, should return global-only config
	cfg, err := ResolveConfig("")
	if err != nil {
		t.Fatalf("ResolveConfig(\"\") error = %v", err)
	}

	// Verify global values are present
	if cfg.ProxyListen != ":3128" {
		t.Errorf("cfg.ProxyListen = %q, want %q", cfg.ProxyListen, ":3128")
	}
	if cfg.Agent != "claude" {
		t.Errorf("cfg.Agent = %q, want %q", cfg.Agent, "claude")
	}
	if cfg.RateLimit != 120 {
		t.Errorf("cfg.RateLimit = %d, want %d", cfg.RateLimit, 120)
	}

	// Verify allowlist contains default entries
	if len(cfg.Allow) == 0 {
		t.Error("cfg.Allow should not be empty")
	}

	// Verify project-specific fields are empty
	if cfg.ProjectName != "" {
		t.Errorf("cfg.ProjectName = %q, want empty", cfg.ProjectName)
	}
	if cfg.ProjectRoot != "" {
		t.Errorf("cfg.ProjectRoot = %q, want empty", cfg.ProjectRoot)
	}
}

func TestResolveConfig_WithProject(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create project config directory
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Create project config with additional allowlist and commands
	projectContent := `
remote: "git@github.com:example/myproject.git"
root: "/projects/myproject"
refs:
  - "/docs/api-spec"
proxy:
  allow:
    - domain: "custom.myproject.com"
    - domain: "internal.corp.net"
hostexec:
  auto_approve:
    - pattern: "^make test$"
    - pattern: "^npm run build$"
`
	projectPath := filepath.Join(projectsDir, "myproject.yaml")
	if err := os.WriteFile(projectPath, []byte(projectContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := ResolveConfig("myproject")
	if err != nil {
		t.Fatalf("ResolveConfig(\"myproject\") error = %v", err)
	}

	// Verify global values are still present
	if cfg.ProxyListen != ":3128" {
		t.Errorf("cfg.ProxyListen = %q, want %q", cfg.ProxyListen, ":3128")
	}
	if cfg.Agent != "claude" {
		t.Errorf("cfg.Agent = %q, want %q", cfg.Agent, "claude")
	}

	// Verify project-specific fields
	if cfg.ProjectName != "myproject" {
		t.Errorf("cfg.ProjectName = %q, want %q", cfg.ProjectName, "myproject")
	}
	if cfg.ProjectRoot != "/projects/myproject" {
		t.Errorf("cfg.ProjectRoot = %q, want %q", cfg.ProjectRoot, "/projects/myproject")
	}
	if cfg.ProjectRemote != "git@github.com:example/myproject.git" {
		t.Errorf("cfg.ProjectRemote = %q, want %q", cfg.ProjectRemote, "git@github.com:example/myproject.git")
	}
	if len(cfg.ProjectRefs) != 1 || cfg.ProjectRefs[0] != "/docs/api-spec" {
		t.Errorf("cfg.ProjectRefs = %v, want %v", cfg.ProjectRefs, []string{"/docs/api-spec"})
	}

	// Verify merged allowlist contains both global and project entries
	// Global has defaults like "golang.org", "api.anthropic.com"
	// Project adds "custom.myproject.com", "internal.corp.net"
	foundGolang := false
	foundCustom := false
	for _, entry := range cfg.Allow {
		if entry.Domain == "golang.org" {
			foundGolang = true
		}
		if entry.Domain == "custom.myproject.com" {
			foundCustom = true
		}
	}
	if !foundGolang {
		t.Error("cfg.Allow should contain global entry 'golang.org'")
	}
	if !foundCustom {
		t.Error("cfg.Allow should contain project entry 'custom.myproject.com'")
	}

	// Verify merged auto_approve patterns
	// Global has "^docker ps.*$"
	// Project adds "^make test$", "^npm run build$"
	foundDockerPs := false
	foundMakeTest := false
	for _, pattern := range cfg.AutoApprove {
		if pattern.Pattern == "^docker ps.*$" {
			foundDockerPs = true
		}
		if pattern.Pattern == "^make test$" {
			foundMakeTest = true
		}
	}
	if !foundDockerPs {
		t.Error("cfg.AutoApprove should contain global pattern '^docker ps.*$'")
	}
	if !foundMakeTest {
		t.Error("cfg.AutoApprove should contain project pattern '^make test$'")
	}
}

func TestResolveConfig_ProjectNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Project doesn't exist, should use default (empty) project config
	cfg, err := ResolveConfig("nonexistent-project")
	if err != nil {
		t.Fatalf("ResolveConfig(\"nonexistent-project\") error = %v", err)
	}

	// Verify global values are present
	if cfg.ProxyListen != ":3128" {
		t.Errorf("cfg.ProxyListen = %q, want %q", cfg.ProxyListen, ":3128")
	}

	// Verify project name is set even though config doesn't exist
	if cfg.ProjectName != "nonexistent-project" {
		t.Errorf("cfg.ProjectName = %q, want %q", cfg.ProjectName, "nonexistent-project")
	}

	// Verify project-specific fields are empty (default project config)
	if cfg.ProjectRoot != "" {
		t.Errorf("cfg.ProjectRoot = %q, want empty", cfg.ProjectRoot)
	}
	if cfg.ProjectRemote != "" {
		t.Errorf("cfg.ProjectRemote = %q, want empty", cfg.ProjectRemote)
	}

	// Verify allowlist is global-only (no project additions)
	// Since project config is empty, merged list equals global list
	if len(cfg.Allow) == 0 {
		t.Error("cfg.Allow should contain global default entries")
	}

	// Verify no project-specific domains were added
	for _, entry := range cfg.Allow {
		// All entries should be from the global defaults
		if entry.Domain == "custom.myproject.com" {
			t.Error("cfg.Allow should not contain project-specific domains for nonexistent project")
		}
	}
}

func TestResolveConfig_MergeDedup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create project config directory
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Create project config with overlapping entries
	projectContent := `
remote: "git@github.com:example/dedup.git"
proxy:
  allow:
    - domain: "golang.org"
    - domain: "unique.project.com"
hostexec:
  auto_approve:
    - pattern: "^docker ps.*$"
    - pattern: "^project-specific$"
`
	projectPath := filepath.Join(projectsDir, "dedup-test.yaml")
	if err := os.WriteFile(projectPath, []byte(projectContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := ResolveConfig("dedup-test")
	if err != nil {
		t.Fatalf("ResolveConfig(\"dedup-test\") error = %v", err)
	}

	// Count occurrences of "golang.org" in allowlist
	golangCount := 0
	for _, entry := range cfg.Allow {
		if entry.Domain == "golang.org" {
			golangCount++
		}
	}
	if golangCount != 1 {
		t.Errorf("'golang.org' appears %d times in allowlist, want 1", golangCount)
	}

	// Count occurrences of "^docker ps.*$" in auto_approve
	dockerPsCount := 0
	for _, pattern := range cfg.AutoApprove {
		if pattern.Pattern == "^docker ps.*$" {
			dockerPsCount++
		}
	}
	if dockerPsCount != 1 {
		t.Errorf("'^docker ps.*$' appears %d times in auto_approve, want 1", dockerPsCount)
	}
}

func TestResolveConfig_ManualApprovePatterns(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create project config directory
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Create project config with manual_approve patterns
	projectContent := `
remote: "git@github.com:example/manual-test.git"
root: "/projects/manual-test"
hostexec:
  manual_approve:
    - pattern: "^./deploy\\.sh.*$"
    - pattern: "^terraform apply.*$"
`
	projectPath := filepath.Join(projectsDir, "manual-test.yaml")
	if err := os.WriteFile(projectPath, []byte(projectContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := ResolveConfig("manual-test")
	if err != nil {
		t.Fatalf("ResolveConfig(\"manual-test\") error = %v", err)
	}

	// Verify merged manual_approve patterns
	// Global has specific gh patterns like "^gh pr (view|list|status|checks|diff)( .+)?$"
	// Project adds "^./deploy\\.sh.*$", "^terraform apply.*$"
	foundGlobalGhPr := false
	foundProjectDeploy := false
	foundProjectTerraform := false
	for _, pattern := range cfg.ManualApprove {
		if pattern.Pattern == `^gh pr (view|list|status|checks|diff)( .+)?$` {
			foundGlobalGhPr = true
		}
		if pattern.Pattern == "^./deploy\\.sh.*$" {
			foundProjectDeploy = true
		}
		if pattern.Pattern == "^terraform apply.*$" {
			foundProjectTerraform = true
		}
	}
	if !foundGlobalGhPr {
		t.Error("cfg.ManualApprove should contain global pattern '^gh pr (view|list|status|checks|diff)( .+)?$'")
	}
	if !foundProjectDeploy {
		t.Error("cfg.ManualApprove should contain project pattern '^./deploy\\.sh.*$'")
	}
	if !foundProjectTerraform {
		t.Error("cfg.ManualApprove should contain project pattern '^terraform apply.*$'")
	}
}

func TestResolveConfig_ManualApproveDedup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create project config directory
	projectsDir := filepath.Join(tmpDir, "cloister", "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	// Create project config with overlapping manual_approve pattern
	projectContent := `
remote: "git@github.com:example/dedup-manual.git"
hostexec:
  manual_approve:
    - pattern: "^gh pr (view|list|status|checks|diff)( .+)?$"
    - pattern: "^project-specific-manual$"
`
	projectPath := filepath.Join(projectsDir, "dedup-manual.yaml")
	if err := os.WriteFile(projectPath, []byte(projectContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := ResolveConfig("dedup-manual")
	if err != nil {
		t.Fatalf("ResolveConfig(\"dedup-manual\") error = %v", err)
	}

	// Count occurrences of "^gh pr (view|list|status|checks|diff)( .+)?$" in manual_approve
	ghPrCount := 0
	for _, pattern := range cfg.ManualApprove {
		if pattern.Pattern == `^gh pr (view|list|status|checks|diff)( .+)?$` {
			ghPrCount++
		}
	}
	if ghPrCount != 1 {
		t.Errorf("'^gh pr (view|list|status|checks|diff)( .+)?$' appears %d times in manual_approve, want 1", ghPrCount)
	}

	// Verify project-specific pattern was added
	foundProjectSpecific := false
	for _, pattern := range cfg.ManualApprove {
		if pattern.Pattern == "^project-specific-manual$" {
			foundProjectSpecific = true
		}
	}
	if !foundProjectSpecific {
		t.Error("cfg.ManualApprove should contain project pattern '^project-specific-manual$'")
	}
}

func TestResolveConfig_GlobalOnlyManualApprove(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// No project name, should return global-only config
	cfg, err := ResolveConfig("")
	if err != nil {
		t.Fatalf("ResolveConfig(\"\") error = %v", err)
	}

	// Verify global manual_approve patterns are present
	// Default global config has specific gh patterns (read-only operations only)
	if len(cfg.ManualApprove) == 0 {
		t.Error("cfg.ManualApprove should not be empty for global config")
	}

	foundGhPr := false
	foundGhIssue := false
	for _, pattern := range cfg.ManualApprove {
		if pattern.Pattern == `^gh pr (view|list|status|checks|diff)( .+)?$` {
			foundGhPr = true
		}
		if pattern.Pattern == `^gh issue (view|list)( .+)?$` {
			foundGhIssue = true
		}
	}
	if !foundGhPr {
		t.Error("cfg.ManualApprove should contain global pattern '^gh pr (view|list|status|checks|diff)( .+)?$'")
	}
	if !foundGhIssue {
		t.Error("cfg.ManualApprove should contain global pattern '^gh issue (view|list)( .+)?$'")
	}
}

func TestMergeAllowlists_PatternOnlyEntries(t *testing.T) {
	global := []AllowEntry{
		{Pattern: "*.foo.com"},
	}
	project := []AllowEntry{
		{Pattern: "*.bar.com"},
	}

	result := MergeAllowlists(global, project)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Pattern != "*.foo.com" {
		t.Errorf("result[0].Pattern = %q, want %q", result[0].Pattern, "*.foo.com")
	}
	if result[1].Pattern != "*.bar.com" {
		t.Errorf("result[1].Pattern = %q, want %q", result[1].Pattern, "*.bar.com")
	}
}

func TestMergeAllowlists_MixedDomainAndPattern(t *testing.T) {
	global := []AllowEntry{
		{Domain: "a.com"},
		{Pattern: "*.b.com"},
	}
	project := []AllowEntry{
		{Domain: "a.com"},    // Duplicate domain
		{Pattern: "*.c.com"}, // Unique pattern
	}

	result := MergeAllowlists(global, project)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// a.com (deduped), *.b.com, *.c.com
	if result[0].Domain != "a.com" {
		t.Errorf("result[0].Domain = %q, want %q", result[0].Domain, "a.com")
	}
	if result[1].Pattern != "*.b.com" {
		t.Errorf("result[1].Pattern = %q, want %q", result[1].Pattern, "*.b.com")
	}
	if result[2].Pattern != "*.c.com" {
		t.Errorf("result[2].Pattern = %q, want %q", result[2].Pattern, "*.c.com")
	}
}

func TestMergeDenylists_PatternOnlyEntries(t *testing.T) {
	global := []AllowEntry{
		{Pattern: "*.evil.com"},
	}
	project := []AllowEntry{
		{Pattern: "*.malicious.net"},
	}

	result := MergeDenylists(global, project)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Pattern != "*.evil.com" {
		t.Errorf("result[0].Pattern = %q, want %q", result[0].Pattern, "*.evil.com")
	}
	if result[1].Pattern != "*.malicious.net" {
		t.Errorf("result[1].Pattern = %q, want %q", result[1].Pattern, "*.malicious.net")
	}
}

func TestMergeDenylists_MixedDomainAndPattern(t *testing.T) {
	global := []AllowEntry{
		{Domain: "evil.com"},
		{Pattern: "*.malware.net"},
	}
	project := []AllowEntry{
		{Domain: "evil.com"},        // Duplicate domain
		{Pattern: "*.phishing.org"}, // Unique pattern
	}

	result := MergeDenylists(global, project)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// evil.com (deduped), *.malware.net, *.phishing.org
	if result[0].Domain != "evil.com" {
		t.Errorf("result[0].Domain = %q, want %q", result[0].Domain, "evil.com")
	}
	if result[1].Pattern != "*.malware.net" {
		t.Errorf("result[1].Pattern = %q, want %q", result[1].Pattern, "*.malware.net")
	}
	if result[2].Pattern != "*.phishing.org" {
		t.Errorf("result[2].Pattern = %q, want %q", result[2].Pattern, "*.phishing.org")
	}
}

func TestResolveConfig_WithDeny(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Write global config with a deny entry
	globalDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(globalDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	globalContent := `
proxy:
  deny:
    - domain: "evil.com"
    - pattern: "*.malware.net"
`
	globalPath := filepath.Join(globalDir, "config.yaml")
	if err := os.WriteFile(globalPath, []byte(globalContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Create project config with additional deny entries
	projectsDir := filepath.Join(globalDir, "projects")
	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	projectContent := `
remote: "git@github.com:example/denytest.git"
proxy:
  deny:
    - domain: "banned.example.com"
    - domain: "evil.com"
`
	projectPath := filepath.Join(projectsDir, "denytest.yaml")
	if err := os.WriteFile(projectPath, []byte(projectContent), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// Test global-only (no project)
	cfg, err := ResolveConfig("")
	if err != nil {
		t.Fatalf("ResolveConfig(\"\") error = %v", err)
	}
	if len(cfg.Deny) != 2 {
		t.Fatalf("global-only cfg.Deny len = %d, want 2", len(cfg.Deny))
	}
	if cfg.Deny[0].Domain != "evil.com" {
		t.Errorf("cfg.Deny[0].Domain = %q, want %q", cfg.Deny[0].Domain, "evil.com")
	}
	if cfg.Deny[1].Pattern != "*.malware.net" {
		t.Errorf("cfg.Deny[1].Pattern = %q, want %q", cfg.Deny[1].Pattern, "*.malware.net")
	}

	// Test with project (merged + deduped)
	cfg, err = ResolveConfig("denytest")
	if err != nil {
		t.Fatalf("ResolveConfig(\"denytest\") error = %v", err)
	}

	// Global has evil.com + *.malware.net, project has banned.example.com + evil.com (dup)
	// Merged should be 3 entries (evil.com deduped)
	if len(cfg.Deny) != 3 {
		t.Fatalf("merged cfg.Deny len = %d, want 3", len(cfg.Deny))
	}

	// Verify order: global first, then unique project entries
	expected := []struct {
		domain  string
		pattern string
	}{
		{domain: "evil.com"},
		{pattern: "*.malware.net"},
		{domain: "banned.example.com"},
	}
	for i, want := range expected {
		if want.domain != "" && cfg.Deny[i].Domain != want.domain {
			t.Errorf("cfg.Deny[%d].Domain = %q, want %q", i, cfg.Deny[i].Domain, want.domain)
		}
		if want.pattern != "" && cfg.Deny[i].Pattern != want.pattern {
			t.Errorf("cfg.Deny[%d].Pattern = %q, want %q", i, cfg.Deny[i].Pattern, want.pattern)
		}
	}
}
