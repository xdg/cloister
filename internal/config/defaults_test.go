package config

import (
	"testing"
)

func TestDefaultGlobalConfig_Valid(t *testing.T) {
	cfg := DefaultGlobalConfig()

	err := ValidateGlobalConfig(cfg)
	if err != nil {
		t.Errorf("DefaultGlobalConfig() produces invalid config: %v", err)
	}
}

func TestDefaultGlobalConfig_ContainsDomains(t *testing.T) {
	cfg := DefaultGlobalConfig()

	// Build a set of domains for easy lookup
	domains := make(map[string]bool)
	for _, entry := range cfg.Proxy.Allow {
		domains[entry.Domain] = true
	}

	// Verify AI API domains are present
	aiAPIs := []string{
		"api.anthropic.com",
		"api.openai.com",
		"generativelanguage.googleapis.com",
	}
	for _, domain := range aiAPIs {
		if !domains[domain] {
			t.Errorf("DefaultGlobalConfig() missing AI API domain %q", domain)
		}
	}

	// Verify package registry domains are present
	registries := []string{
		"registry.npmjs.org",
		"proxy.golang.org",
		"sum.golang.org",
		"pypi.org",
		"files.pythonhosted.org",
		"crates.io",
		"static.crates.io",
	}
	for _, domain := range registries {
		if !domains[domain] {
			t.Errorf("DefaultGlobalConfig() missing package registry domain %q", domain)
		}
	}

	// Verify documentation site domains are present
	docSites := []string{
		"golang.org",
		"pkg.go.dev",
		"go.dev",
		"docs.rs",
		"doc.rust-lang.org",
		"docs.python.org",
		"developer.mozilla.org",
		"devdocs.io",
		"stackoverflow.com",
		"man7.org",
		"linux.die.net",
	}
	for _, domain := range docSites {
		if !domains[domain] {
			t.Errorf("DefaultGlobalConfig() missing documentation site domain %q", domain)
		}
	}
}

func TestDefaultGlobalConfig_AllFields(t *testing.T) {
	cfg := DefaultGlobalConfig()

	// Proxy fields
	if cfg.Proxy.Listen == "" {
		t.Error("DefaultGlobalConfig() Proxy.Listen is empty")
	}
	if len(cfg.Proxy.Allow) == 0 {
		t.Error("DefaultGlobalConfig() Proxy.Allow is empty")
	}
	if cfg.Proxy.UnlistedDomainBehavior == "" {
		t.Error("DefaultGlobalConfig() Proxy.UnlistedDomainBehavior is empty")
	}
	if cfg.Proxy.ApprovalTimeout == "" {
		t.Error("DefaultGlobalConfig() Proxy.ApprovalTimeout is empty")
	}
	if cfg.Proxy.RateLimit == 0 {
		t.Error("DefaultGlobalConfig() Proxy.RateLimit is zero")
	}
	if cfg.Proxy.MaxRequestBytes == 0 {
		t.Error("DefaultGlobalConfig() Proxy.MaxRequestBytes is zero")
	}

	// Request fields
	if cfg.Request.Listen == "" {
		t.Error("DefaultGlobalConfig() Request.Listen is empty")
	}
	if cfg.Request.Timeout == "" {
		t.Error("DefaultGlobalConfig() Request.Timeout is empty")
	}

	// Hostexec fields
	if cfg.Hostexec.Listen == "" {
		t.Error("DefaultGlobalConfig() Hostexec.Listen is empty")
	}
	if len(cfg.Hostexec.AutoApprove) == 0 {
		t.Error("DefaultGlobalConfig() Hostexec.AutoApprove is empty")
	}
	if len(cfg.Hostexec.ManualApprove) == 0 {
		t.Error("DefaultGlobalConfig() Hostexec.ManualApprove is empty")
	}

	// Devcontainer fields
	if !cfg.Devcontainer.Enabled {
		t.Error("DefaultGlobalConfig() Devcontainer.Enabled is false")
	}
	if len(cfg.Devcontainer.Features.Allow) == 0 {
		t.Error("DefaultGlobalConfig() Devcontainer.Features.Allow is empty")
	}
	if len(cfg.Devcontainer.BlockedMounts) == 0 {
		t.Error("DefaultGlobalConfig() Devcontainer.BlockedMounts is empty")
	}

	// Agents
	if len(cfg.Agents) == 0 {
		t.Error("DefaultGlobalConfig() Agents is empty")
	}
	for name, agent := range cfg.Agents {
		if agent.Command == "" {
			t.Errorf("DefaultGlobalConfig() Agent %q has empty Command", name)
		}
		if len(agent.Env) == 0 {
			t.Errorf("DefaultGlobalConfig() Agent %q has empty Env", name)
		}
	}

	// Verify expected agents are present
	expectedAgents := []string{"claude", "codex", "gemini"}
	for _, name := range expectedAgents {
		if _, ok := cfg.Agents[name]; !ok {
			t.Errorf("DefaultGlobalConfig() missing agent %q", name)
		}
	}

	// Defaults fields
	if cfg.Defaults.Image == "" {
		t.Error("DefaultGlobalConfig() Defaults.Image is empty")
	}
	if cfg.Defaults.Shell == "" {
		t.Error("DefaultGlobalConfig() Defaults.Shell is empty")
	}
	if cfg.Defaults.User == "" {
		t.Error("DefaultGlobalConfig() Defaults.User is empty")
	}
	if cfg.Defaults.Agent == "" {
		t.Error("DefaultGlobalConfig() Defaults.Agent is empty")
	}

	// Log fields
	if cfg.Log.File == "" {
		t.Error("DefaultGlobalConfig() Log.File is empty")
	}
	if cfg.Log.Level == "" {
		t.Error("DefaultGlobalConfig() Log.Level is empty")
	}
	if cfg.Log.PerCloisterDir == "" {
		t.Error("DefaultGlobalConfig() Log.PerCloisterDir is empty")
	}
}

func TestDefaultGlobalConfig_ClaudeSkipPerms(t *testing.T) {
	cfg := DefaultGlobalConfig()

	claude, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatal("DefaultGlobalConfig() missing claude agent")
	}

	if claude.SkipPerms == nil {
		t.Fatal("DefaultGlobalConfig() claude.SkipPerms is nil, expected true")
	}
	if !*claude.SkipPerms {
		t.Error("DefaultGlobalConfig() claude.SkipPerms is false, expected true")
	}
}

func TestDefaultProjectConfig(t *testing.T) {
	cfg := DefaultProjectConfig()

	// Verify it returns a valid empty config
	err := ValidateProjectConfig(cfg)
	if err != nil {
		t.Errorf("DefaultProjectConfig() produces invalid config: %v", err)
	}

	// Verify it is empty (minimal)
	if cfg.Remote != "" {
		t.Error("DefaultProjectConfig() Remote is not empty")
	}
	if cfg.Root != "" {
		t.Error("DefaultProjectConfig() Root is not empty")
	}
	if len(cfg.Refs) != 0 {
		t.Error("DefaultProjectConfig() Refs is not empty")
	}
	if len(cfg.Proxy.Allow) != 0 {
		t.Error("DefaultProjectConfig() Proxy.Allow is not empty")
	}
	if len(cfg.Commands.AutoApprove) != 0 {
		t.Error("DefaultProjectConfig() Commands.AutoApprove is not empty")
	}
}
