package config

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// DefaultGlobalConfig returns a GlobalConfig with all defaults populated.
// These defaults provide a secure baseline configuration for cloister.
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

				// Package registries
				{Domain: "registry.npmjs.org"},
				{Domain: "proxy.golang.org"},
				{Domain: "sum.golang.org"},
				{Domain: "pypi.org"},
				{Domain: "files.pythonhosted.org"},
				{Domain: "crates.io"},
				{Domain: "static.crates.io"},

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
				{Pattern: "^docker compose ps$"},
				{Pattern: "^docker compose logs.*$"},
			},
			ManualApprove: []CommandPattern{
				// Dev environment lifecycle
				{Pattern: "^docker compose (up|down|restart|build).*$"},
				// External tools requiring credentials
				{Pattern: "^gh .+$"},
				{Pattern: "^jira .+$"},
				{Pattern: "^aws .+$"},
				{Pattern: "^gcloud .+$"},
				// Network access with full path visibility
				{Pattern: "^curl .+$"},
				{Pattern: "^wget .+$"},
			},
		},
		Devcontainer: DevcontainerConfig{
			Enabled: true,
			Features: FeaturesConfig{
				Allow: []string{
					"ghcr.io/devcontainers/features/*",
					"ghcr.io/devcontainers-contrib/features/*",
				},
			},
			BlockedMounts: []string{
				"~/.ssh",
				"~/.aws",
				"~/.config/gcloud",
				"~/.gnupg",
				"~/.config/gh",
				"/var/run/docker.sock",
			},
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
