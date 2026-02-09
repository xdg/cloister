package config

import (
	"strings"
	"testing"
)

func TestValidateGlobalConfig_Valid(t *testing.T) {
	cfg := &GlobalConfig{
		Proxy: ProxyConfig{
			Listen:                 ":3128",
			ApprovalTimeout:        "60s",
			RateLimit:              120,
			MaxRequestBytes:        10485760,
			UnlistedDomainBehavior: "request_approval",
			Allow: []AllowEntry{
				{Domain: "golang.org"},
				{Domain: "api.anthropic.com"},
			},
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
				{Pattern: "^gh .+$"},
				{Pattern: "^curl .+$"},
			},
		},
		Log: LogConfig{
			Level:  "info",
			Stdout: true,
		},
	}

	err := ValidateGlobalConfig(cfg)
	if err != nil {
		t.Errorf("ValidateGlobalConfig() error = %v, want nil", err)
	}
}

func TestValidateGlobalConfig_Empty(t *testing.T) {
	cfg := &GlobalConfig{}

	err := ValidateGlobalConfig(cfg)
	if err != nil {
		t.Errorf("ValidateGlobalConfig() error = %v, want nil for empty config", err)
	}
}

func TestValidateGlobalConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *GlobalConfig
		wantErr string
	}{
		{
			name: "port too high",
			cfg: &GlobalConfig{
				Proxy: ProxyConfig{Listen: ":99999"},
			},
			wantErr: "proxy.listen: invalid port number 99999",
		},
		{
			name: "port zero",
			cfg: &GlobalConfig{
				Proxy: ProxyConfig{Listen: ":0"},
			},
			wantErr: "proxy.listen: invalid port number 0",
		},
		{
			name: "negative port",
			cfg: &GlobalConfig{
				Request: RequestConfig{Listen: ":-1"},
			},
			wantErr: "request.listen: invalid port",
		},
		{
			name: "non-numeric port",
			cfg: &GlobalConfig{
				Hostexec: HostexecConfig{Listen: "127.0.0.1:abc"},
			},
			wantErr: "hostexec.listen: invalid port",
		},
		{
			name: "missing port",
			cfg: &GlobalConfig{
				Proxy: ProxyConfig{Listen: "localhost"},
			},
			wantErr: "proxy.listen: invalid format",
		},
		{
			name: "empty port after colon",
			cfg: &GlobalConfig{
				Proxy: ProxyConfig{Listen: "localhost:"},
			},
			wantErr: "proxy.listen: invalid port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGlobalConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateGlobalConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateGlobalConfig_InvalidDuration(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *GlobalConfig
		wantErr string
	}{
		{
			name: "invalid proxy approval_timeout",
			cfg: &GlobalConfig{
				Proxy: ProxyConfig{ApprovalTimeout: "notaduration"},
			},
			wantErr: "proxy.approval_timeout: invalid duration",
		},
		{
			name: "invalid request timeout",
			cfg: &GlobalConfig{
				Request: RequestConfig{Timeout: "5 minutes"},
			},
			wantErr: "request.timeout: invalid duration",
		},
		{
			name: "missing unit",
			cfg: &GlobalConfig{
				Proxy: ProxyConfig{ApprovalTimeout: "60"},
			},
			wantErr: "proxy.approval_timeout: invalid duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGlobalConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateGlobalConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateGlobalConfig_InvalidRegex(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *GlobalConfig
		wantErr string
	}{
		{
			name: "invalid auto_approve regex",
			cfg: &GlobalConfig{
				Hostexec: HostexecConfig{
					AutoApprove: []CommandPattern{
						{Pattern: "^valid$"},
						{Pattern: "[invalid"},
					},
				},
			},
			wantErr: "hostexec.auto_approve[1].pattern: invalid regex",
		},
		{
			name: "invalid manual_approve regex",
			cfg: &GlobalConfig{
				Hostexec: HostexecConfig{
					ManualApprove: []CommandPattern{
						{Pattern: "(unclosed"},
					},
				},
			},
			wantErr: "hostexec.manual_approve[0].pattern: invalid regex",
		},
		{
			name: "invalid regex with special chars",
			cfg: &GlobalConfig{
				Hostexec: HostexecConfig{
					AutoApprove: []CommandPattern{
						{Pattern: "(?P<invalid"},
					},
				},
			},
			wantErr: "hostexec.auto_approve[0].pattern: invalid regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGlobalConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateGlobalConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateGlobalConfig_NegativeRateLimit(t *testing.T) {
	cfg := &GlobalConfig{
		Proxy: ProxyConfig{RateLimit: -1},
	}

	err := ValidateGlobalConfig(cfg)
	if err == nil {
		t.Fatal("ValidateGlobalConfig() expected error for negative rate_limit")
	}
	if !strings.Contains(err.Error(), "proxy.rate_limit: must be non-negative") {
		t.Errorf("error = %q, want to mention non-negative rate_limit", err.Error())
	}
}

func TestValidateGlobalConfig_NegativeMaxRequestBytes(t *testing.T) {
	cfg := &GlobalConfig{
		Proxy: ProxyConfig{MaxRequestBytes: -100},
	}

	err := ValidateGlobalConfig(cfg)
	if err == nil {
		t.Fatal("ValidateGlobalConfig() expected error for negative max_request_bytes")
	}
	if !strings.Contains(err.Error(), "proxy.max_request_bytes: must be non-negative") {
		t.Errorf("error = %q, want to mention non-negative max_request_bytes", err.Error())
	}
}

func TestValidateGlobalConfig_InvalidLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{"unknown level", "verbose"},
		{"capitalized", "INFO"},
		{"typo", "wran"},
		{"invalid", "trace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &GlobalConfig{
				Log: LogConfig{Level: tt.level},
			}

			err := ValidateGlobalConfig(cfg)
			if err == nil {
				t.Fatalf("ValidateGlobalConfig() expected error for log level %q", tt.level)
			}
			if !strings.Contains(err.Error(), "log.level: invalid value") {
				t.Errorf("error = %q, want to mention invalid log level", err.Error())
			}
			if !strings.Contains(err.Error(), "debug, info, warn, error") {
				t.Errorf("error = %q, want to list valid options", err.Error())
			}
		})
	}
}

func TestValidateGlobalConfig_ValidLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := &GlobalConfig{
				Log: LogConfig{Level: level},
			}

			err := ValidateGlobalConfig(cfg)
			if err != nil {
				t.Errorf("ValidateGlobalConfig() error = %v for valid log level %q", err, level)
			}
		})
	}
}

func TestValidateGlobalConfig_ValidListenFormats(t *testing.T) {
	tests := []struct {
		name   string
		listen string
	}{
		{"port only", ":8080"},
		{"localhost with port", "localhost:8080"},
		{"ip with port", "127.0.0.1:9999"},
		{"ipv6 with port", "[::1]:8080"},
		{"min port", ":1"},
		{"max port", ":65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &GlobalConfig{
				Proxy: ProxyConfig{Listen: tt.listen},
			}

			err := ValidateGlobalConfig(cfg)
			if err != nil {
				t.Errorf("ValidateGlobalConfig() error = %v for listen %q", err, tt.listen)
			}
		})
	}
}

func TestValidateGlobalConfig_ValidDurations(t *testing.T) {
	durations := []string{"1s", "30s", "5m", "1h", "1h30m", "500ms", "2h45m30s"}

	for _, d := range durations {
		t.Run(d, func(t *testing.T) {
			cfg := &GlobalConfig{
				Proxy:   ProxyConfig{ApprovalTimeout: d},
				Request: RequestConfig{Timeout: d},
			}

			err := ValidateGlobalConfig(cfg)
			if err != nil {
				t.Errorf("ValidateGlobalConfig() error = %v for duration %q", err, d)
			}
		})
	}
}

func TestValidateProjectConfig_Valid(t *testing.T) {
	cfg := &ProjectConfig{
		Remote: "git@github.com:xdg/my-api.git",
		Root:   "~/repos/my-api",
		Refs:   []string{"~/repos/shared-lib"},
		Proxy: ProjectProxyConfig{
			Allow: []AllowEntry{
				{Domain: "internal-docs.company.com"},
			},
		},
		Hostexec: ProjectHostexecConfig{
			AutoApprove: []CommandPattern{
				{Pattern: "^make test$"},
				{Pattern: "^./scripts/lint\\.sh$"},
			},
		},
	}

	err := ValidateProjectConfig(cfg)
	if err != nil {
		t.Errorf("ValidateProjectConfig() error = %v, want nil", err)
	}
}

func TestValidateProjectConfig_Empty(t *testing.T) {
	cfg := &ProjectConfig{}

	err := ValidateProjectConfig(cfg)
	if err != nil {
		t.Errorf("ValidateProjectConfig() error = %v, want nil for empty config", err)
	}
}

func TestValidateProjectConfig_InvalidRegex(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ProjectConfig
		wantErr string
	}{
		{
			name: "unclosed bracket",
			cfg: &ProjectConfig{
				Hostexec: ProjectHostexecConfig{
					AutoApprove: []CommandPattern{
						{Pattern: "[invalid"},
					},
				},
			},
			wantErr: "hostexec.auto_approve[0].pattern: invalid regex",
		},
		{
			name: "unclosed group",
			cfg: &ProjectConfig{
				Hostexec: ProjectHostexecConfig{
					AutoApprove: []CommandPattern{
						{Pattern: "^valid$"},
						{Pattern: "(unclosed"},
					},
				},
			},
			wantErr: "hostexec.auto_approve[1].pattern: invalid regex",
		},
		{
			name: "invalid escape sequence",
			cfg: &ProjectConfig{
				Hostexec: ProjectHostexecConfig{
					AutoApprove: []CommandPattern{
						{Pattern: "\\k<invalid>"},
					},
				},
			},
			wantErr: "hostexec.auto_approve[0].pattern: invalid regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateProjectConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateProjectConfig_EmptyPattern(t *testing.T) {
	// Empty patterns should be valid (they're no-ops)
	cfg := &ProjectConfig{
		Hostexec: ProjectHostexecConfig{
			AutoApprove: []CommandPattern{
				{Pattern: ""},
				{Pattern: "^valid$"},
			},
		},
	}

	err := ValidateProjectConfig(cfg)
	if err != nil {
		t.Errorf("ValidateProjectConfig() error = %v, want nil for empty pattern", err)
	}
}

func TestValidateProjectConfig_ManualApproveValid(t *testing.T) {
	cfg := &ProjectConfig{
		Hostexec: ProjectHostexecConfig{
			ManualApprove: []CommandPattern{
				{Pattern: "^./deploy\\.sh.*$"},
				{Pattern: "^terraform apply.*$"},
			},
		},
	}

	err := ValidateProjectConfig(cfg)
	if err != nil {
		t.Errorf("ValidateProjectConfig() error = %v, want nil", err)
	}
}

func TestValidateProjectConfig_ManualApproveInvalidRegex(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ProjectConfig
		wantErr string
	}{
		{
			name: "unclosed bracket in manual_approve",
			cfg: &ProjectConfig{
				Hostexec: ProjectHostexecConfig{
					ManualApprove: []CommandPattern{
						{Pattern: "[invalid"},
					},
				},
			},
			wantErr: "hostexec.manual_approve[0].pattern: invalid regex",
		},
		{
			name: "unclosed group in manual_approve",
			cfg: &ProjectConfig{
				Hostexec: ProjectHostexecConfig{
					ManualApprove: []CommandPattern{
						{Pattern: "^valid$"},
						{Pattern: "(unclosed"},
					},
				},
			},
			wantErr: "hostexec.manual_approve[1].pattern: invalid regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateProjectConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateAgentConfig_ValidAuthMethods(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AgentConfig
	}{
		{
			name: "empty auth_method is valid",
			cfg:  &AgentConfig{},
		},
		{
			name: "token auth_method with token",
			cfg:  &AgentConfig{AuthMethod: "token", Token: "my-token"},
		},
		{
			name: "api_key auth_method with api_key",
			cfg:  &AgentConfig{AuthMethod: "api_key", APIKey: "sk-ant-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentConfig(tt.cfg, "agents.claude")
			if err != nil {
				t.Errorf("ValidateAgentConfig() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateAgentConfig_InvalidAuthMethod(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *AgentConfig
		wantErr string
	}{
		{
			name:    "unknown auth_method",
			cfg:     &AgentConfig{AuthMethod: "oauth"},
			wantErr: "agents.claude.auth_method: invalid value \"oauth\", must be one of: token, api_key",
		},
		{
			name:    "typo in auth_method",
			cfg:     &AgentConfig{AuthMethod: "tokens"},
			wantErr: "agents.claude.auth_method: invalid value \"tokens\", must be one of: token, api_key",
		},
		{
			name:    "capitalized auth_method",
			cfg:     &AgentConfig{AuthMethod: "TOKEN"},
			wantErr: "agents.claude.auth_method: invalid value \"TOKEN\", must be one of: token, api_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentConfig(tt.cfg, "agents.claude")
			if err == nil {
				t.Fatal("ValidateAgentConfig() expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateAgentConfig_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *AgentConfig
		wantErr string
	}{
		{
			name:    "token auth_method without token",
			cfg:     &AgentConfig{AuthMethod: "token"},
			wantErr: "agents.claude.token: required when auth_method is \"token\"",
		},
		{
			name:    "token auth_method with empty token",
			cfg:     &AgentConfig{AuthMethod: "token", Token: ""},
			wantErr: "agents.claude.token: required when auth_method is \"token\"",
		},
		{
			name:    "api_key auth_method without api_key",
			cfg:     &AgentConfig{AuthMethod: "api_key"},
			wantErr: "agents.claude.api_key: required when auth_method is \"api_key\"",
		},
		{
			name:    "api_key auth_method with empty api_key",
			cfg:     &AgentConfig{AuthMethod: "api_key", APIKey: ""},
			wantErr: "agents.claude.api_key: required when auth_method is \"api_key\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentConfig(tt.cfg, "agents.claude")
			if err == nil {
				t.Fatal("ValidateAgentConfig() expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateAgentConfigWarnings_NoAuth(t *testing.T) {
	cfg := &AgentConfig{}
	warnings := ValidateAgentConfigWarnings(cfg, "agents.claude", nil)

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	expected := "agents.claude: no authentication configured (auth_method not set)"
	if warnings[0] != expected {
		t.Errorf("warning = %q, want %q", warnings[0], expected)
	}
}

func TestValidateAgentConfigWarnings_NoAuthWithHostEnv(t *testing.T) {
	cfg := &AgentConfig{}
	// Simulate host having ANTHROPIC_API_KEY set
	warnings := ValidateAgentConfigWarnings(cfg, "agents.claude", []string{"some-api-key"})

	if len(warnings) != 0 {
		t.Errorf("expected no warnings when host env vars present, got %v", warnings)
	}
}

func TestValidateAgentConfigWarnings_AuthConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AgentConfig
	}{
		{
			name: "token auth_method",
			cfg:  &AgentConfig{AuthMethod: "token", Token: "my-token"},
		},
		{
			name: "api_key auth_method",
			cfg:  &AgentConfig{AuthMethod: "api_key", APIKey: "sk-ant-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := ValidateAgentConfigWarnings(tt.cfg, "agents.claude", nil)
			if len(warnings) != 0 {
				t.Errorf("expected no warnings when auth configured, got %v", warnings)
			}
		})
	}
}

func TestValidateGlobalConfig_InvalidAgentConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *GlobalConfig
		wantErr string
	}{
		{
			name: "invalid auth_method in agent config",
			cfg: &GlobalConfig{
				Agents: map[string]AgentConfig{
					"claude": {AuthMethod: "invalid"},
				},
			},
			wantErr: "agents.claude.auth_method: invalid value \"invalid\"",
		},
		{
			name: "token auth without token field",
			cfg: &GlobalConfig{
				Agents: map[string]AgentConfig{
					"claude": {AuthMethod: "token"},
				},
			},
			wantErr: "agents.claude.token: required when auth_method is \"token\"",
		},
		{
			name: "api_key auth without api_key field",
			cfg: &GlobalConfig{
				Agents: map[string]AgentConfig{
					"codex": {AuthMethod: "api_key"},
				},
			},
			wantErr: "agents.codex.api_key: required when auth_method is \"api_key\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGlobalConfig(tt.cfg)
			if err == nil {
				t.Fatal("ValidateGlobalConfig() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateGlobalConfig_ValidAgentConfig(t *testing.T) {
	cfg := &GlobalConfig{
		Agents: map[string]AgentConfig{
			"claude": {
				AuthMethod: "token",
				Token:      "my-oauth-token",
			},
			"codex": {
				AuthMethod: "api_key",
				APIKey:     "sk-openai-123",
			},
		},
	}

	err := ValidateGlobalConfig(cfg)
	if err != nil {
		t.Errorf("ValidateGlobalConfig() error = %v, want nil", err)
	}
}
