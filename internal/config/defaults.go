package config

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// DefaultGlobalConfig returns a GlobalConfig with all defaults populated.
// These defaults provide a secure baseline configuration for cloister.
//
// Security philosophy for hostexec patterns:
//   - AutoApprove: Only read-only commands with no side effects
//   - ManualApprove: Specific patterns for common operations; broad patterns
//     like "aws .+" or "curl .+" are intentionally excluded because they
//     could be used for data exfiltration or credential abuse
//   - Network access should go through the proxy allowlist, not hostexec
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Proxy: ProxyConfig{
			Listen: ":3128",
			Allow: []AllowEntry{
				// Documentation sites
				{Domain: "golang.org"},
				{Domain: "pkg.go.dev"},
				{Domain: "go.dev"},
				{Domain: "docs.rs"},
				{Domain: "doc.rust-lang.org"},
				{Domain: "docs.python.org"},
				{Domain: "developer.mozilla.org"},
				{Domain: "devdocs.io"},
				{Domain: "stackoverflow.com"},
				{Domain: "man7.org"},
				{Domain: "linux.die.net"},
				{Domain: "cppreference.com"},
				{Domain: "en.cppreference.com"},
				{Domain: "typescriptlang.org"},
				{Domain: "nodejs.org"},
				{Domain: "docs.docker.com"},
				{Domain: "kubernetes.io"},
				{Domain: "ruby-doc.org"},
				{Domain: "docs.npmjs.com"},

				// Package registries
				{Domain: "registry.npmjs.org"},
				{Domain: "proxy.golang.org"},
				{Domain: "sum.golang.org"},
				{Domain: "pypi.org"},
				{Domain: "files.pythonhosted.org"},
				{Domain: "crates.io"},
				{Domain: "static.crates.io"},
				{Domain: "rubygems.org"},
				{Domain: "index.rubygems.org"},
				{Domain: "yarnpkg.com"},

				// AI provider APIs
				{Domain: "api.anthropic.com"},
				{Domain: "api.openai.com"},
				{Domain: "generativelanguage.googleapis.com"},
			},
			UnlistedDomainBehavior: "request_approval",
			ApprovalTimeout:        "60s",
			RateLimit:              120,
			MaxRequestBytes:        10485760, // 10MB
		},
		Request: RequestConfig{
			Listen:  ":9998",
			Timeout: "5m",
		},
		Hostexec: HostexecConfig{
			Listen: "127.0.0.1:9999",
			AutoApprove: []CommandPattern{
				// Read-only container inspection (safe, no side effects)
				{Pattern: "^docker ps.*$"},
			},
			ManualApprove: []CommandPattern{
				// GitHub CLI - read-only operations only
				// Patterns like "gh pr create" excluded: could exfiltrate data in PR body
				{Pattern: `^gh pr (view|list|status|checks|diff)( .+)?$`},
				{Pattern: `^gh issue (view|list)( .+)?$`},
				{Pattern: `^gh repo view( .+)?$`},
				{Pattern: `^gh run (list|view|watch)( .+)?$`},
			},
			// Intentionally excluded from defaults (users can add to project config):
			// - curl/wget: Network access should use proxy allowlist, not hostexec
			// - aws/gcloud: Too broad; could exfiltrate data or create resources
			// - docker compose: Project-specific; add narrower patterns if needed
		},
		Agents: map[string]AgentConfig{
			"claude": {
				Command: "claude",
				Env: []string{
					"ANTHROPIC_*",
					"CLAUDE_*",
				},
				SkipPerms: boolPtr(true),
			},
			"codex": {
				Command: "codex",
				Env: []string{
					"OPENAI_API_KEY",
				},
			},
			"gemini": {
				Command: "gemini",
				Env: []string{
					"GOOGLE_API_KEY",
					"GEMINI_*",
				},
			},
		},
		Defaults: DefaultsConfig{
			// Image intentionally empty - signals use of version-matched image
			Shell: "/bin/bash",
			User:  "cloister",
			Agent: "claude",
		},
		Log: LogConfig{
			File:           "~/.local/share/cloister/audit.log",
			Stdout:         true,
			Level:          "info",
			PerCloister:    true,
			PerCloisterDir: "~/.local/share/cloister/logs/",
		},
	}
}

// DefaultProjectConfig returns an empty ProjectConfig. Projects start with
// minimal configuration and are populated when a project is detected or
// explicitly configured.
func DefaultProjectConfig() *ProjectConfig {
	return &ProjectConfig{}
}
